package workspace

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
)

func (s *Service) retryRestoreFinalization(operationID string) Result {
	task := s.restoreTask
	if task == nil {
		var text string
		var op Operation
		if s.db.QueryRow("SELECT result_json FROM operations WHERE id=?", operationID).Scan(&text) != nil || json.Unmarshal([]byte(text), &op) != nil || op.Kind != "backup-restore" {
			return failure("NOT_FOUND", "没有此恢复任务。", false)
		}
		return success(op, op.ID)
	}
	if task.operation.ID != operationID {
		return failure("PROFILE_BUSY", "只能核对当前受保护恢复任务。", true)
	}
	if task.acceptancePending {
		return failure("RESTORE_INCOMPLETE", "请核实原受理请求，尚未执行目录恢复。", true)
	}
	if task.running {
		return success(copyRestoreOperation(task.operation), operationID)
	}
	if task.finalPending != nil {
		return success(copyRestoreOperation(task.operation), operationID)
	}
	if task.bootstrapOutcome != nil {
		s.scheduleRestoreBootstrap(task)
		return success(copyRestoreOperation(task.operation), operationID)
	}
	if !task.recoveryAvailable {
		return failure("RESTORE_INCOMPLETE", "重开后的恢复计划需要启动恢复流程核对，当前保持保护。", true)
	}
	op := copyRestoreOperation(task.operation)
	op.State = "running"
	op.Stage = "recovering"
	op.RestoreReport.Sequence++
	op.PersistencePending = true
	if err := s.persistRestore(task, op, "recovering"); err != nil {
		return s.restoreUnconfirmed(task, err)
	}
	task.operation = op
	task.phase = "recovering"
	task.running = true
	plan := task.plan
	var cause error = errors.New("retry incomplete restore")
	if op.CancelRequested {
		cause = context.Canceled
	}
	s.workers.Add(1)
	go func() { defer s.workers.Done(); s.finishRestoreExecution(task, plan, cause) }()
	return success(op, operationID)
}

func (s *Service) initializeRestores() error {
	var version int
	if err := s.db.QueryRow("PRAGMA user_version").Scan(&version); err != nil {
		return err
	}
	if version == 7 {
		tx, err := s.db.Begin()
		if err != nil {
			return err
		}
		defer tx.Rollback()
		if _, err = tx.Exec(`CREATE TABLE restore_jobs(operation_id TEXT PRIMARY KEY REFERENCES operations(id),plan_json TEXT NOT NULL,phase TEXT NOT NULL,committed INTEGER NOT NULL DEFAULT 0 CHECK(committed IN (0,1)),request_id TEXT NOT NULL UNIQUE,signature TEXT NOT NULL)`); err != nil {
			return err
		}
		if _, err = tx.Exec("PRAGMA user_version=8"); err != nil {
			return err
		}
		if err = tx.Commit(); err != nil {
			return err
		}
	}
	rows, err := s.db.Query("SELECT operation_id,plan_json,phase,committed,request_id,signature FROM restore_jobs LIMIT 0")
	if err != nil {
		return err
	}
	return rows.Close()
}

func saveRestoreJournal(tx *sql.Tx, plan restorePlan, op Operation, phase string) error {
	planJSON, err := json.Marshal(plan)
	if err != nil {
		return err
	}
	opJSON, err := json.Marshal(op)
	if err != nil {
		return err
	}
	result, err := tx.Exec("UPDATE restore_jobs SET plan_json=?,phase=? WHERE operation_id=?", string(planJSON), phase, op.ID)
	if err != nil {
		return err
	}
	if count, _ := result.RowsAffected(); count != 1 {
		return errors.New("restore journal missing")
	}
	result, err = tx.Exec("UPDATE operations SET result_json=? WHERE id=?", string(opJSON), op.ID)
	if err != nil {
		return err
	}
	if count, _ := result.RowsAffected(); count != 1 {
		return errors.New("restore operation missing")
	}
	return nil
}
func (s *Service) persistRestore(task *restoreTask, op Operation, phase string) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err = saveRestoreJournal(tx, task.plan, op, phase); err != nil {
		return err
	}
	if s.options.BeforeCommit != nil {
		if err = s.options.BeforeCommit(); err != nil {
			return err
		}
	}
	if err = tx.Commit(); err != nil {
		var text, storedPhase string
		expected, _ := json.Marshal(op)
		if s.db.QueryRow("SELECT o.result_json,r.phase FROM restore_jobs r JOIN operations o ON o.id=r.operation_id WHERE r.operation_id=?", op.ID).Scan(&text, &storedPhase) == nil && text == string(expected) && storedPhase == phase {
			return nil
		}
	}
	return err
}
func (s *Service) restoreCommitted(operationID string) (bool, error) {
	var committed bool
	err := s.db.QueryRow("SELECT committed FROM restore_jobs WHERE operation_id=?", operationID).Scan(&committed)
	return committed, err
}

// Retry only the already-observed terminal write. No archive read, stop, copy,
// directory rename or configuration replay occurs in this path.
func (s *Service) flushRestorePersistence() {
	task := s.restoreTask
	if task != nil && !task.running && task.bootstrapReady && task.bootstrapOutcome != nil {
		s.finishRestoreBootstrap(task)
		return
	}
	if task == nil || task.running || task.acceptancePending || task.finalPending == nil {
		return
	}
	op := copyRestoreOperation(*task.finalPending)
	if err := s.persistRestore(task, op, task.finalPhase); err != nil {
		return
	}
	task.operation = op
	task.phase = task.finalPhase
	task.finalPending = nil
	s.releaseRestore(task, op.RestoreReport.Committed)
}

func (s *Service) listRestoreOperations() ([]Operation, error) {
	rows, err := s.db.Query("SELECT o.result_json FROM operations o JOIN restore_jobs r ON r.operation_id=o.id ORDER BY o.rowid DESC LIMIT 30")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []Operation{}
	for rows.Next() {
		var text string
		var op Operation
		if err = rows.Scan(&text); err != nil {
			return nil, err
		}
		if err = json.Unmarshal([]byte(text), &op); err != nil {
			return nil, err
		}
		if s.restoreTask != nil && s.restoreTask.operation.ID == op.ID {
			op = copyRestoreOperation(s.restoreTask.operation)
		}
		result = append(result, op)
	}
	return result, rows.Err()
}

// Load all candidates before starting any worker: one journal owns the global
// switch barrier. Multiple or malformed candidates are retained, never replayed.
func (s *Service) loadInterruptedRestore() error {
	rows, err := s.db.Query("SELECT o.result_json,r.plan_json,r.phase,r.committed,r.request_id,r.signature,q.signature,q.result_json FROM restore_jobs r LEFT JOIN operations o ON o.id=r.operation_id LEFT JOIN requests q ON q.id=r.request_id WHERE r.phase NOT IN ('finalized','rolled-back','rejected')")
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		if s.restoreTask != nil {
			return restoreRecoveryCountError(2)
		}
		var opJSON, planJSON, phase, requestID, signature, requestSignature, requestJSON string
		var committed bool
		if err = rows.Scan(&opJSON, &planJSON, &phase, &committed, &requestID, &signature, &requestSignature, &requestJSON); err != nil {
			return err
		}
		task, err := s.restoreRecoveryRecord(opJSON, planJSON, phase, requestID, signature, requestSignature, requestJSON, committed)
		if err != nil {
			return err
		}
		s.restoreTask = task
	}
	return rows.Err()
}
