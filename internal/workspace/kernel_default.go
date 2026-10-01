package workspace

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
)

type KernelDefault struct {
	KernelID string `json:"kernelId"`
	Revision int64  `json:"revision"`
}
type KernelDefaultRequest struct {
	KernelID         string `json:"kernelId"`
	ExpectedRevision int64  `json:"expectedRevision"`
	RequestID        string `json:"requestId"`
}

func (s *Service) initializeMigrations() error {
	var version int
	if err := s.db.QueryRow("PRAGMA user_version").Scan(&version); err != nil {
		return err
	}
	if version == 9 {
		tx, err := s.db.Begin()
		if err != nil {
			return err
		}
		defer tx.Rollback()
		for _, statement := range []string{
			`CREATE TABLE kernel_default(singleton INTEGER PRIMARY KEY CHECK(singleton=1),kernel_id TEXT NOT NULL REFERENCES kernels(id),revision INTEGER NOT NULL CHECK(revision>0))`,
			`INSERT INTO kernel_default(singleton,kernel_id,revision) VALUES(1,'kernel-pending',1)`,
			`CREATE TABLE kernel_migrations(operation_id TEXT PRIMARY KEY REFERENCES operations(id),plan_json TEXT NOT NULL,plan_sha256 TEXT NOT NULL,phase TEXT NOT NULL,committed INTEGER NOT NULL DEFAULT 0 CHECK(committed IN (0,1)),request_id TEXT NOT NULL UNIQUE,signature TEXT NOT NULL)`,
			`CREATE TABLE migration_kernel_refs(operation_id TEXT NOT NULL REFERENCES kernel_migrations(operation_id),kernel_id TEXT NOT NULL REFERENCES kernels(id),PRIMARY KEY(operation_id,kernel_id))`,
			`PRAGMA user_version=10`,
		} {
			if _, err := tx.Exec(statement); err != nil {
				return err
			}
		}
		if err := tx.Commit(); err != nil {
			return err
		}
	}
	for _, statement := range []string{"SELECT singleton,kernel_id,revision FROM kernel_default LIMIT 0", "SELECT operation_id,plan_json,plan_sha256,phase,committed,request_id,signature FROM kernel_migrations LIMIT 0", "SELECT operation_id,kernel_id FROM migration_kernel_refs LIMIT 0"} {
		rows, err := s.db.Query(statement)
		if err != nil {
			return err
		}
		if err := rows.Close(); err != nil {
			return err
		}
	}
	_, err := s.readKernelDefault()
	return err
}

func (s *Service) readKernelDefault() (KernelDefault, error) {
	var result KernelDefault
	err := s.db.QueryRow("SELECT kernel_id,revision FROM kernel_default WHERE singleton=1").Scan(&result.KernelID, &result.Revision)
	if err == nil && result.Revision < 1 {
		err = errors.New("invalid default kernel revision")
	}
	return result, err
}

func (s *Service) setKernelDefault(input KernelDefaultRequest) Result {
	if strings.TrimSpace(input.RequestID) == "" || len(input.RequestID) > 128 || input.ExpectedRevision < 1 {
		return failure("VALIDATION_FAILED", "请选择明确的默认构建和当前修订。", false)
	}
	encoded, _ := json.Marshal(input)
	sum := sha256.Sum256(append([]byte("Kernel.SetDefault:"), encoded...))
	signature := hex.EncodeToString(sum[:])
	var oldSignature, oldJSON string
	err := s.db.QueryRow("SELECT signature,result_json FROM requests WHERE id=?", input.RequestID).Scan(&oldSignature, &oldJSON)
	if err == nil {
		if oldSignature != signature {
			return failure("REQUEST_ID_REUSED", "原请求不能更换默认构建。", false)
		}
		var prior Result
		if json.Unmarshal([]byte(oldJSON), &prior) != nil {
			return storageFailure(errors.New("default receipt invalid"))
		}
		return prior
	}
	if err != sql.ErrNoRows {
		return storageFailure(err)
	}
	if s.kernelTask != nil {
		return failure("PROFILE_BUSY", "请等待当前内核维护任务完成。", true)
	}
	if input.KernelID != PendingKernelID {
		if _, err := savedKernelFrom(s.db, input.KernelID, true); err != nil {
			return kernelFailure(err)
		}
	}
	tx, err := s.db.Begin()
	if err != nil {
		return storageFailure(err)
	}
	defer tx.Rollback()
	result, err := tx.Exec("UPDATE kernel_default SET kernel_id=?,revision=revision+1 WHERE singleton=1 AND revision=?", input.KernelID, input.ExpectedRevision)
	if err != nil {
		return storageFailure(err)
	}
	if count, err := result.RowsAffected(); err != nil || count != 1 {
		return failure("REVISION_CONFLICT", "默认选择已变化，请重新读取后选择。", true)
	}
	response := success(KernelDefault{KernelID: input.KernelID, Revision: input.ExpectedRevision + 1}, "")
	receipt, _ := json.Marshal(response)
	if _, err := tx.Exec("INSERT INTO requests(id,signature,result_json) VALUES(?,?,?)", input.RequestID, signature, string(receipt)); err != nil {
		return storageFailure(err)
	}
	if s.options.BeforeCommit != nil {
		if err := s.options.BeforeCommit(); err != nil {
			return storageFailure(err)
		}
	}
	if err := tx.Commit(); err != nil {
		var actual string
		if s.db.QueryRow("SELECT result_json FROM requests WHERE id=? AND signature=?", input.RequestID, signature).Scan(&actual) != nil || actual != string(receipt) {
			return failure("DEFAULT_RESULT_UNCONFIRMED", "默认选择保存尚未核实，请读取或重发原请求。", true)
		}
	}
	return response
}

func (s *Service) kernelRetentionCount(kernelID string) (int, error) {
	var count int
	err := s.db.QueryRow(`SELECT (SELECT COUNT(*) FROM fingerprints WHERE kernel_id=?)+(SELECT COUNT(*) FROM fingerprint_revisions WHERE kernel_id=?)+(SELECT COUNT(*) FROM kernel_default WHERE kernel_id=?)+(SELECT COUNT(*) FROM migration_kernel_refs WHERE kernel_id=?)`, kernelID, kernelID, kernelID, kernelID).Scan(&count)
	return count, err
}
