package workspace

import "encoding/json"

func (s *Service) flushOneCookieWrite(operation Operation) error {
	if err := s.commitCookieObservation(operation); err != nil {
		pending := copyCookieOperation(operation)
		pending.State, pending.Stage, pending.PersistencePending, pending.Error = "failed", "storage-pending", true, storageFailure(err).Error
		s.cookieResults[operation.ID] = pending
		s.profileUses[operation.EnvironmentID] = true
		return err
	}
	delete(s.cookiePending, operation.ID)
	delete(s.cookieResults, operation.ID)
	if task := s.cookieTasks[operation.EnvironmentID]; task != nil && task.operation.ID == operation.ID {
		delete(s.cookieTasks, operation.EnvironmentID)
	}
	if !s.runtimeOwnsProfileUse(operation.EnvironmentID) {
		delete(s.profileUses, operation.EnvironmentID)
	}
	return nil
}

func (s *Service) flushCookiePersistence() {
	for _, operation := range s.cookiePending {
		_ = s.flushOneCookieWrite(operation)
	}
}

func (s *Service) commitCookieObservation(operation Operation) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	operation.PersistencePending = false
	encoded, err := json.Marshal(operation)
	if err != nil {
		return err
	}
	if _, err = tx.Exec("UPDATE operations SET result_json=? WHERE id=?", string(encoded), operation.ID); err != nil {
		return err
	}
	if _, err = tx.Exec("INSERT OR IGNORE INTO activities(id,created_at,action,target,detail) VALUES(?,?,?,?,?)", operation.ID, timestamp(), "导入指定环境Cookie", operation.EnvironmentID, "仅逐条读回核对计成功；Cookie值不入日志，未改写其他环境。"); err != nil {
		return err
	}
	if s.options.BeforeCommit != nil {
		if err := s.options.BeforeCommit(); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *Service) listCookieOperations() ([]Operation, error) {
	rows, err := s.db.Query("SELECT result_json FROM operations WHERE json_extract(result_json,'$.kind')='cookie-import' ORDER BY rowid DESC")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	operations := []Operation{}
	for rows.Next() {
		var encoded string
		var operation Operation
		if err := rows.Scan(&encoded); err != nil {
			return nil, err
		}
		if err := json.Unmarshal([]byte(encoded), &operation); err != nil {
			return nil, err
		}
		if observed, exists := s.cookieResults[operation.ID]; exists {
			operation = observed
		}
		operations = append(operations, operation)
	}
	return operations, rows.Err()
}

func (s *Service) recoverCookieImports() error {
	operations, err := s.listCookieOperations()
	if err != nil {
		return err
	}
	for _, operation := range operations {
		if operation.State != "accepted" && operation.State != "running" {
			continue
		}
		operation.State, operation.Stage, operation.Error = "failed", "interrupted", &Error{Code: "APPLICATION_INTERRUPTED", Message: "应用在Cookie导入期间关闭；可能已有写入，未重放秘密或自动再次清空。请重新预览、先核对同键后重试。", Retryable: true}
		if report := operation.CookieReport; report != nil {
			for index := range report.Items {
				if report.Items[index].Status == "pending" {
					report.Items[index].Status, report.Items[index].ErrorCode, report.Items[index].Message = "unknown", "APPLICATION_INTERRUPTED", "尚未持久化的写入结果未知，不能计为成功或假称无副作用。"
				}
			}
			if report.ClearState == "pending" {
				report.ClearState = "unknown"
			}
			recountCookieReport(report)
			report.FinishedAt = timestamp()
		}
		if err := s.commitCookieObservation(operation); err != nil {
			return err
		}
	}
	return nil
}
