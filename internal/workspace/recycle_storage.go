package workspace

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"

	"github.com/axgiroud312-byte/prism-local-browser/internal/backup"
)

// Both export and the trusted import schema use exactly this projection.
const nativeV1SchemaProjection = "DROP TABLE IF EXISTS migration_kernel_refs; DROP TABLE IF EXISTS kernel_migrations; DROP TABLE IF EXISTS kernel_default; DROP TABLE IF EXISTS recycle_jobs; DROP TABLE IF EXISTS environment_trash; DROP TABLE IF EXISTS restore_jobs; PRAGMA user_version=7"

func (s *Service) initializeRecycle() error {
	var version int
	if err := s.db.QueryRow("PRAGMA user_version").Scan(&version); err != nil {
		return err
	}
	if version == 8 {
		tx, err := s.db.Begin()
		if err != nil {
			return err
		}
		defer tx.Rollback()
		for _, statement := range []string{
			`CREATE TABLE environment_trash(environment_id TEXT PRIMARY KEY REFERENCES environments(id),trash_id TEXT NOT NULL UNIQUE,entry_json TEXT NOT NULL)`,
			`CREATE TABLE recycle_jobs(operation_id TEXT PRIMARY KEY REFERENCES operations(id),plan_json TEXT NOT NULL,plan_sha256 TEXT NOT NULL,phase TEXT NOT NULL,request_id TEXT NOT NULL UNIQUE,signature TEXT NOT NULL)`,
			`PRAGMA user_version=9`,
		} {
			if _, err := tx.Exec(statement); err != nil {
				return err
			}
		}
		if err := tx.Commit(); err != nil {
			return err
		}
	}
	for _, statement := range []string{"SELECT environment_id,trash_id,entry_json FROM environment_trash LIMIT 0", "SELECT operation_id,plan_json,plan_sha256,phase,request_id,signature FROM recycle_jobs LIMIT 0"} {
		rows, err := s.db.Query(statement)
		if err != nil {
			return err
		}
		if err := rows.Close(); err != nil {
			return err
		}
	}
	return nil
}

func recyclePlanHash(encoded []byte) string {
	sum := sha256.Sum256(encoded)
	return hex.EncodeToString(sum[:])
}
func copyRecyclePlan(plan recyclePlan) recyclePlan {
	encoded, _ := json.Marshal(plan)
	var copy recyclePlan
	json.Unmarshal(encoded, &copy)
	return copy
}

func recycleBaseline(query interface {
	Query(string, ...any) (*sql.Rows, error)
}, environmentID string) (string, error) {
	return restoreAffectedBaseline(query, restorePlan{Environments: []restoreStoredEnvironment{{Manifest: backup.Environment{ID: environmentID}}}})
}
func readRecycleEntry(query profileQuery, trashID string) (recycleEntry, error) {
	var entry recycleEntry
	var text, environmentID string
	err := query.QueryRow("SELECT environment_id,entry_json FROM environment_trash WHERE trash_id=?", trashID).Scan(&environmentID, &text)
	if err != nil {
		return entry, err
	}
	if err := backup.DecodeJSON([]byte(text), &entry); err != nil {
		return entry, err
	}
	if entry.Lock != nil && (!entry.Item.DataPresent || entry.Lock.Identity == (backup.TreeIdentity{}) || entry.Lock.Size < 0 || !backup.Hash(entry.Lock.SHA256)) {
		return entry, errors.New("invalid retained ownership file")
	}
	ref, err := dataReference(environmentID)
	if err != nil || entry.Item.EnvironmentID != environmentID || entry.TrashID != trashID || entry.Item.ID != trashID || entry.Reference != ref || !backup.CanonicalID(trashID) || !backup.Hash(entry.Baseline) || entry.Item.Revision < 1 || !validRestoreInventory(entry.Files) || entry.Item.DataPresent && entry.Identity == (backup.TreeIdentity{}) || !entry.Item.DataPresent && (entry.Identity != (backup.TreeIdentity{}) || len(entry.Files) != 0) {
		return entry, errors.New("invalid recycle entry")
	}
	return entry, nil
}
func saveRecycleJournal(tx *sql.Tx, plan recyclePlan, op Operation, phase string) error {
	p, err := json.Marshal(plan)
	if err != nil {
		return err
	}
	o, err := json.Marshal(op)
	if err != nil {
		return err
	}
	for _, query := range []struct {
		sql  string
		args []any
	}{
		{"UPDATE recycle_jobs SET plan_json=?,plan_sha256=?,phase=? WHERE operation_id=?", []any{string(p), recyclePlanHash(p), phase, op.ID}},
		{"UPDATE operations SET result_json=? WHERE id=?", []any{string(o), op.ID}},
	} {
		result, err := tx.Exec(query.sql, query.args...)
		if err != nil {
			return err
		}
		if count, err := result.RowsAffected(); err != nil || count != 1 {
			return errors.New("recycle journal missing")
		}
	}
	return nil
}
func (s *Service) persistRecycle(plan recyclePlan, op Operation, phase string) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := saveRecycleJournal(tx, plan, op, phase); err != nil {
		return err
	}
	if s.options.BeforeCommit != nil {
		if err := s.options.BeforeCommit(); err != nil {
			return err
		}
	}
	if err = tx.Commit(); err != nil {
		var p, o, storedPhase string
		wantP, _ := json.Marshal(plan)
		wantO, _ := json.Marshal(op)
		if s.db.QueryRow("SELECT j.plan_json,j.phase,o.result_json FROM recycle_jobs j JOIN operations o ON o.id=j.operation_id WHERE j.operation_id=?", op.ID).Scan(&p, &storedPhase, &o) == nil && p == string(wantP) && o == string(wantO) && phase == storedPhase {
			return nil
		}
	}
	return err
}
func (s *Service) readRecycleJournal(operationID string) (recyclePlan, Operation, string, error) {
	var plan recyclePlan
	var op Operation
	var p, o, phase, hash, requestID, signature, requestSignature, receipt string
	err := s.db.QueryRow("SELECT j.plan_json,j.phase,o.result_json,j.plan_sha256,j.request_id,j.signature,r.signature,r.result_json FROM recycle_jobs j LEFT JOIN operations o ON o.id=j.operation_id LEFT JOIN requests r ON r.id=j.request_id WHERE j.operation_id=?", operationID).Scan(&p, &phase, &o, &hash, &requestID, &signature, &requestSignature, &receipt)
	if err != nil {
		return plan, op, phase, err
	}
	if recyclePlanHash([]byte(p)) != hash {
		return plan, op, phase, errors.New("recycle plan digest differs")
	}
	if err := backup.DecodeJSON([]byte(p), &plan); err != nil {
		return plan, op, phase, err
	}
	if err := backup.DecodeJSON([]byte(o), &op); err != nil {
		return plan, op, phase, err
	}
	if plan.ID != operationID {
		return plan, op, phase, errors.New("recycle operation identity differs")
	}
	if err := validateRecycleJournal(plan, op, phase, p); err != nil {
		return plan, op, phase, err
	}
	if err := recycleReceipt(plan, op, requestID, signature, requestSignature, receipt); err != nil {
		return plan, op, phase, err
	}
	return plan, op, phase, nil
}
func (s *Service) listRecycleOperations() ([]Operation, error) {
	rows, err := s.db.Query("SELECT o.result_json FROM operations o JOIN recycle_jobs j ON j.operation_id=o.id ORDER BY o.rowid DESC LIMIT 30")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []Operation{}
	for rows.Next() {
		var text string
		var op Operation
		if err := rows.Scan(&text); err != nil {
			return nil, err
		}
		if err := json.Unmarshal([]byte(text), &op); err != nil {
			return nil, err
		}
		if s.recycleTask != nil && s.recycleTask.operation.ID == op.ID {
			op = copyRecycleOperation(s.recycleTask.operation)
		}
		result = append(result, op)
	}
	return result, rows.Err()
}
