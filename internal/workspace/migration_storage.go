package workspace

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"

	"github.com/axgiroud312-byte/prism-local-browser/internal/backup"
)

func migrationProblem(err error) Result {
	var safe *Error
	if errors.As(err, &safe) {
		return Result{Mode: "native", Error: safe}
	}
	return failure("MIGRATION_INCOMPLETE", "迁移结果尚未完整核对，原备份与工作副本保留；请释放占用、修复存储后核对原任务。", true)
}
func migrationSignature(input MigrationRequest) string {
	b, _ := json.Marshal(input)
	h := sha256.Sum256(append([]byte("Migration.Prepare:"), b...))
	return hex.EncodeToString(h[:])
}
func saveMigration(tx *sql.Tx, plan migrationPlan, op Operation, phase string) error {
	b, err := json.Marshal(plan)
	if err != nil {
		return err
	}
	hash := sha256.Sum256(b)
	o, err := json.Marshal(op)
	if err != nil {
		return err
	}
	r, err := tx.Exec("UPDATE kernel_migrations SET plan_json=?,plan_sha256=?,phase=?,committed=? WHERE operation_id=?", string(b), hex.EncodeToString(hash[:]), phase, plan.Committed, plan.ID)
	if err != nil {
		return err
	}
	n, err := r.RowsAffected()
	if err != nil || n != 1 {
		return errors.New("migration journal missing")
	}
	_, err = tx.Exec("UPDATE operations SET result_json=? WHERE id=?", string(o), op.ID)
	return err
}
func (s *Service) persistMigration(plan migrationPlan, op Operation, phase string) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err = saveMigration(tx, plan, op, phase); err != nil {
		return err
	}
	if s.options.BeforeCommit != nil {
		if err = s.options.BeforeCommit(); err != nil {
			return err
		}
	}
	return s.commitMigrationTransaction(tx)
}

func (s *Service) commitMigrationTransaction(tx *sql.Tx) error {
	if s.options.CommitMigration != nil {
		return s.options.CommitMigration(tx)
	}
	return tx.Commit()
}
func (s *Service) readMigrationJournal(id string) (migrationPlan, Operation, string, error) {
	var plan migrationPlan
	var op Operation
	var raw, hash, phase, rawOp, request, signature, requestSignature, receipt string
	var committed bool
	err := s.db.QueryRow(`SELECT m.plan_json,m.plan_sha256,m.phase,m.committed,m.request_id,m.signature,o.result_json,r.signature,r.result_json FROM kernel_migrations m JOIN operations o ON o.id=m.operation_id JOIN requests r ON r.id=m.request_id WHERE m.operation_id=?`, id).Scan(&raw, &hash, &phase, &committed, &request, &signature, &rawOp, &requestSignature, &receipt)
	if err != nil {
		return plan, op, phase, err
	}
	sum := sha256.Sum256([]byte(raw))
	if hex.EncodeToString(sum[:]) != hash || decode(json.RawMessage(raw), &plan) != nil || decode(json.RawMessage(rawOp), &op) != nil {
		return plan, op, phase, errors.New("migration journal invalid")
	}
	canonical, _ := json.Marshal(plan)
	r := op.MigrationReport
	if string(canonical) != raw || plan.Version != 1 || plan.ID != id || !backup.CanonicalID(id) || !backup.CanonicalID(plan.WorkID) || plan.WorkID == plan.Environment.ID || op.ID != id || op.Kind != "migration" || op.Total != 1 || r == nil || r.Mode != "native" || r.Sequence < 1 || r.RequestID != request || r.PreviewID != plan.Preview.PreviewID || r.EnvironmentID != plan.Environment.ID || op.EnvironmentID != plan.Environment.ID || r.Seed != plan.Environment.Seed || plan.Committed != committed || r.Committed != committed || r.BackupVerified != plan.BackupVerified || r.ArchiveSHA256 != plan.BackupSHA256 || r.TrialExited != plan.TrialExited {
		return plan, op, phase, errors.New("migration journal identity differs")
	}
	if !map[string]bool{"accepted": true, "backup-ready": true, "copy-ready": true, "trial-starting": true, "trial-running": true, "ready": true, "prepared": true, "committed": true, "finished": true}[phase] {
		return plan, op, phase, errors.New("migration phase invalid")
	}
	if plan.Preview.Mode != "native" || !backup.CanonicalID(plan.Preview.PreviewID) || plan.Preview.EnvironmentID != plan.Environment.ID || plan.Preview.Name != plan.Environment.Name || plan.Preview.After.ConfigRevision != plan.Preview.Before.ConfigRevision+1 || !profileMatchesConfiguration(plan.Preview.Before, plan.Environment.Configuration) || plan.LaunchPermitted && plan.WorkIdentity == (backup.TreeIdentity{}) || plan.LaunchNoProcess && (!plan.LaunchPermitted || plan.PID != 0 || !plan.TrialExited) || plan.Prepared && plan.WorkIdentity != plan.Move.NewIdentity {
		return plan, op, phase, errors.New("migration frozen relationship differs")
	}
	expectedConfig := plan.Environment.Configuration
	expectedConfig.CoreID = plan.Preview.After.KernelID
	if !profileMatchesConfiguration(plan.Preview.After, expectedConfig) || checkProfileEvidence(s.db, plan.Preview.Before) != nil || checkProfileEvidence(s.db, plan.Preview.After) != nil {
		return plan, op, phase, errors.New("migration normalized profiles differ")
	}
	ref, e := dataReference(plan.Environment.ID)
	workRef, w := dataReference(plan.WorkID)
	if e != nil || w != nil || plan.Target.ID != plan.Environment.ID || plan.Target.Reference != ref || plan.Move.ID != plan.Environment.ID || plan.Move.Live != ref || plan.Move.Incoming != workRef || plan.Move.Previous != migrationStageRef(id)+"/previous" || !backup.Hash(plan.Baseline) || !backup.CanonicalID(plan.FingerprintID) || checkProfile(plan.Preview.Before) != nil || checkProfile(plan.Preview.After) != nil || plan.Preview.Before.Seed != plan.Preview.After.Seed || plan.Preview.Before.Seed != plan.Environment.Seed || plan.Preview.Before.KernelID == plan.Preview.After.KernelID || r.OldKernelID != plan.Preview.Before.KernelID || r.NewKernelID != plan.Preview.After.KernelID || !profileMatchesConfiguration(plan.Preview.Before, plan.Environment.Configuration) || plan.Preview.ExpectedRevision < 1 {
		return plan, op, phase, errors.New("migration frozen inputs invalid")
	}
	if plan.BackupVerified && !backup.Hash(plan.BackupSHA256) || plan.Committed && (!plan.Prepared || !backup.Hash(plan.CommittedBaseline)) || !plan.Committed && plan.CommittedBaseline != "" || plan.Prepared && (!plan.BackupVerified || !plan.TrialExited || plan.Move.NewIdentity == (backup.TreeIdentity{}) || !validRestoreInventory(plan.Move.NewFiles)) || !validRestoreInventory(plan.Move.OldFiles) || plan.Move.OldPresent && plan.Move.OldIdentity == (backup.TreeIdentity{}) || !plan.Move.OldPresent && (plan.Move.OldIdentity != (backup.TreeIdentity{}) || len(plan.Move.OldFiles) != 0) || plan.LaunchPermitted && !backup.CanonicalID(plan.SessionID) {
		return plan, op, phase, errors.New("migration decisions invalid")
	}
	var prior Result
	if signature != requestSignature || signature != migrationSignature(MigrationRequest{PreviewID: r.PreviewID, Confirm: true, RequestID: request}) || json.Unmarshal([]byte(receipt), &prior) != nil || !prior.OK || prior.Mode != "native" || prior.OperationID != id {
		return plan, op, phase, errors.New("migration receipt invalid")
	}
	return plan, op, phase, nil
}
func (s *Service) listMigrations() ([]Operation, error) {
	rows, err := s.db.Query("SELECT result_json FROM operations WHERE json_extract(result_json,'$.kind')='migration' ORDER BY rowid DESC LIMIT 20")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	ops := []Operation{}
	for rows.Next() {
		var text string
		var op Operation
		if err = rows.Scan(&text); err != nil {
			return nil, err
		}
		if decode(json.RawMessage(text), &op) != nil {
			return nil, errors.New("migration operation invalid")
		}
		if s.migrationTask != nil && s.migrationTask.operation.ID == op.ID {
			op = copyMigrationOperation(s.migrationTask.operation)
		}
		ops = append(ops, op)
	}
	return ops, rows.Err()
}
