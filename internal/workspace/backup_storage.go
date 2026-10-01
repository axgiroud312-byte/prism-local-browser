package workspace

import (
	"context"
	"encoding/json"
	"errors"
	"os"

	"github.com/axgiroud312-byte/prism-local-browser/internal/backup"
)

func (s *Service) initializeBackups() error {
	var version int
	if err := s.db.QueryRow("PRAGMA user_version").Scan(&version); err != nil {
		return err
	}
	if version == 6 {
		tx, err := s.db.Begin()
		if err != nil {
			return err
		}
		defer tx.Rollback()
		if _, err = tx.Exec(`CREATE TABLE backup_exports(operation_id TEXT PRIMARY KEY REFERENCES operations(id),destination TEXT NOT NULL,temporary TEXT NOT NULL,targets_json TEXT NOT NULL,created_at TEXT NOT NULL,phase TEXT NOT NULL)`); err != nil {
			return err
		}
		for _, statement := range []string{
			`CREATE TABLE environment_data_state(environment_id TEXT PRIMARY KEY REFERENCES environments(id) ON DELETE CASCADE,state TEXT NOT NULL CHECK(state IN ('never-initialized','directory-prepared','runtime-claimed','legacy-unconfirmed')))`,
			`INSERT INTO environment_data_state(environment_id,state) SELECT id,'legacy-unconfirmed' FROM environments`,
			`CREATE TRIGGER monotonic_environment_data_state BEFORE UPDATE ON environment_data_state WHEN NEW.environment_id<>OLD.environment_id OR (OLD.state='runtime-claimed' AND NEW.state<>'runtime-claimed') OR (OLD.state='directory-prepared' AND NEW.state NOT IN ('directory-prepared','runtime-claimed')) OR (OLD.state='legacy-unconfirmed' AND NEW.state NOT IN ('legacy-unconfirmed','runtime-claimed')) BEGIN SELECT RAISE(ABORT,'data initialization fact cannot be reset'); END`,
		} {
			if _, err = tx.Exec(statement); err != nil {
				return err
			}
		}
		if _, err = tx.Exec("PRAGMA user_version=7"); err != nil {
			return err
		}
		if err = tx.Commit(); err != nil {
			return err
		}
	}
	rows, err := s.db.Query(`SELECT operation_id,destination,temporary,targets_json,created_at,phase FROM backup_exports LIMIT 0`)
	if err != nil {
		return err
	}
	if err = rows.Close(); err != nil {
		return err
	}
	rows, err = s.db.Query(`SELECT environment_id,state FROM environment_data_state LIMIT 0`)
	if err != nil {
		return err
	}
	return rows.Close()
}

func (s *Service) persistBackup(task *backupTask, operation Operation, phase string) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	encoded, err := json.Marshal(operation)
	if err != nil {
		return err
	}
	result, err := tx.Exec("UPDATE operations SET result_json=? WHERE id=?", string(encoded), operation.ID)
	if err != nil {
		return err
	}
	if count, _ := result.RowsAffected(); count != 1 {
		return errors.New("backup operation journal missing")
	}
	result, err = tx.Exec("UPDATE backup_exports SET phase=? WHERE operation_id=?", phase, operation.ID)
	if err != nil {
		return err
	}
	if count, _ := result.RowsAffected(); count != 1 {
		return errors.New("backup publication journal missing")
	}
	if operation.State == "completed" || operation.State == "failed" || operation.State == "cancelled" {
		detail := "完整备份已核对并发布；不代表登录状态跨用户便携或恢复已验收。"
		if operation.State != "completed" {
			detail = "备份未确认发布完成；原浏览数据未替换，临时包不是成功备份。"
		}
		if _, err = tx.Exec(`INSERT OR IGNORE INTO activities(id,created_at,action,target,detail) VALUES(?,?,?,?,?)`, operation.ID, timestamp(), "导出完整本机备份", operation.BackupReport.Name, detail); err != nil {
			return err
		}
	}
	if s.options.BeforeCommit != nil {
		if err = s.options.BeforeCommit(); err != nil {
			return err
		}
	}
	if err = tx.Commit(); err != nil {
		var text, observedPhase string
		if s.db.QueryRow(`SELECT o.result_json,b.phase FROM operations o JOIN backup_exports b ON b.operation_id=o.id WHERE o.id=?`, operation.ID).Scan(&text, &observedPhase) == nil && text == string(encoded) && observedPhase == phase {
			return nil
		}
	}
	return err
}

func (s *Service) releaseBackupUses(task *backupTask) {
	for _, target := range task.targets {
		if s.backupUses[target.ID] != task {
			continue
		}
		delete(s.backupUses, target.ID)
		if s.batchUses[target.ID] == nil && !s.runtimeOwnsProfileUse(target.ID) {
			delete(s.profileUses, target.ID)
		}
	}
}
func (s *Service) flushOneBackupFinal(task *backupTask) error {
	if !task.finalPending {
		return nil
	}
	if err := s.persistBackup(task, task.final, task.final.State); err != nil {
		return err
	}
	task.operation = task.final
	task.finalPending = false
	s.releaseBackupUses(task)
	delete(s.backupTasks, task.operation.ID)
	return nil
}
func (s *Service) flushBackupPersistence() {
	for _, task := range s.backupTasks {
		if task.acceptancePending {
			_ = s.confirmBackupAcceptance(task)
		}
		if task.finalPending {
			_ = s.flushOneBackupFinal(task)
		}
	}
}
func (s *Service) pendingBackupOperation(operationID string) *Operation {
	task := s.backupTasks[operationID]
	if task == nil {
		return nil
	}
	operation := copyBackupOperation(task.operation)
	if task.acceptancePending {
		operation.State, operation.Stage, operation.PersistencePending = "failed", "acceptance-pending", true
		operation.Error = &Error{Code: "STORAGE_WRITE_FAILED", Message: "备份受理结果待核实，尚未复制数据；只读取或重发原请求，不能另建任务。", Retryable: true}
	}
	if task.finalPending {
		operation = copyBackupOperation(task.final)
		operation.State, operation.Stage, operation.PersistencePending = "failed", "storage-pending", true
		operation.Error = &Error{Code: "STORAGE_WRITE_FAILED", Message: "备份观测待保存；已发布事实及原摘要保留，重读只保存结果，不重新复制或发布。", Retryable: true}
	}
	return &operation
}

func (s *Service) finishBackup(task *backupTask, cause error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.backupTasks[task.operation.ID] != task {
		return
	}
	operation := copyBackupOperation(task.operation)
	operation.BackupReport.Sequence++
	operation.State, operation.Stage = "completed", "completed"
	if cause != nil || !operation.BackupReport.Published {
		operation.State, operation.Stage = "failed", "interrupted"
		operation.Error = backupFailure(cause)
		if errors.Is(cause, context.Canceled) && !s.closed && !s.closeRequested.Load() {
			operation.State = "cancelled"
			operation.Error = &Error{Code: "OPERATION_CANCELLED", Message: "只取消本次导出；原数据未替换，临时文件不是完成包，已关闭环境不会自动重开。", Retryable: true}
		}
	}
	task.final = operation
	task.finalPending = true
	_ = s.flushOneBackupFinal(task)
}

func (s *Service) recoverBackupExports() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.recoverBackupExportsLocked()
}

func (s *Service) recoverBackupExportsLocked() error {
	rows, err := s.db.Query(`SELECT b.operation_id,b.destination,b.temporary,b.targets_json,b.created_at,b.phase,o.result_json FROM backup_exports b JOIN operations o ON o.id=b.operation_id WHERE b.phase NOT IN ('completed','failed','cancelled')`)
	if err != nil {
		return err
	}
	tasks := []*backupTask{}
	for rows.Next() {
		task := &backupTask{}
		var targets, text, phase string
		if err = rows.Scan(&task.requestID, &task.destination, &task.temporary, &targets, &task.createdAt, &phase, &text); err != nil {
			rows.Close()
			return err
		}
		if decode(json.RawMessage(targets), &task.targets) != nil || decode(json.RawMessage(text), &task.operation) != nil || task.operation.ID != task.requestID || task.operation.Kind != "backup-export" || task.operation.BackupReport == nil {
			rows.Close()
			return errors.New("invalid backup journal")
		}
		final, temp, pathErr := backup.OutputPaths(s.root, task.destination, task.operation.ID)
		if pathErr != nil || final != task.destination || temp != task.temporary {
			rows.Close()
			return errors.New("invalid backup publication paths")
		}
		task.recovery = phase == "publishing"
		tasks = append(tasks, task)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	recovery := []*backupTask{}
	// Finish every fallible synchronous initialization before any goroutine can
	// touch backupTasks. A failure returns with no recovery worker to abandon.
	for _, task := range tasks {
		s.backupTasks[task.operation.ID] = task
		if task.recovery {
			recovery = append(recovery, task)
		} else {
			operation := copyBackupOperation(task.operation)
			operation.State, operation.Stage = "failed", "interrupted"
			operation.BackupReport.Sequence++
			operation.Error = &Error{Code: "APPLICATION_INTERRUPTED", Message: "应用退出中断导出；临时文件不是完成备份，原浏览数据未替换，未自动重做。", Retryable: true}
			task.final, task.finalPending = operation, true
			if err = s.flushOneBackupFinal(task); err != nil {
				return err
			}
		}
	}
	for _, task := range recovery {
		ctx, cancel := context.WithCancel(context.Background())
		task.cancel = cancel
		s.workers.Add(1)
		go func(ctx context.Context, task *backupTask) {
			defer s.workers.Done()
			digest, err := backup.PublishedDigest(ctx, task.destination)
			if err == nil && digest == task.operation.BackupReport.ArchiveSHA256 && digest != "" {
				s.mu.Lock()
				task.operation.BackupReport.Published = true
				s.mu.Unlock()
				s.finishBackup(task, nil)
			} else {
				message := "退出打断了备份发布；正式文件未能按原摘要确认，没有重新复制、发布或删除任何文件。"
				if os.IsNotExist(err) {
					message = "退出发生在发布前；临时文件保留但不是成功备份，没有自动重做。"
				}
				s.finishBackup(task, &Error{Code: "BACKUP_PUBLICATION_UNCONFIRMED", Message: message, Retryable: true})
			}
		}(ctx, task)
	}
	return nil
}

func (s *Service) listBackupOperations() ([]Operation, []BackupRecord, error) {
	rows, err := s.db.Query(`SELECT o.result_json,b.created_at FROM operations o JOIN backup_exports b ON b.operation_id=o.id ORDER BY o.rowid DESC LIMIT 30`)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()
	operations := []Operation{}
	records := []BackupRecord{}
	seen := map[string]bool{}
	for rows.Next() {
		var text, created string
		var operation Operation
		if err = rows.Scan(&text, &created); err != nil {
			return nil, nil, err
		}
		if json.Unmarshal([]byte(text), &operation) != nil || operation.BackupReport == nil {
			return nil, nil, errors.New("invalid backup report")
		}
		if pending := s.pendingBackupOperation(operation.ID); pending != nil {
			operation = *pending
		}
		operations = append(operations, operation)
		seen[operation.ID] = true
		if operation.State == "completed" && !operation.PersistencePending && operation.BackupReport.Published {
			report := operation.BackupReport
			records = append(records, BackupRecord{ID: operation.ID, OperationID: operation.ID, Name: report.Name, CreatedAt: created, Scope: report.Scope, EnvironmentCount: report.EnvironmentCount, ArchiveSHA256: report.ArchiveSHA256, ManifestSHA256: report.ManifestSHA256})
		}
	}
	for operationID := range s.backupTasks {
		if !seen[operationID] {
			operations = append(operations, *s.pendingBackupOperation(operationID))
		}
	}
	return operations, records, rows.Err()
}

// Error messages are deliberately independent of file names, Cookie contents,
// protected credential bytes and the user's absolute output path.
func (e *Error) Error() string { return e.Message }
