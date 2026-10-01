package workspace

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/axgiroud312-byte/prism-local-browser/internal/proxy"
)

func (s *Service) beginProxyCheck(input ProxyTarget, record ProxyView, ref, signature string) Result {
	if active := s.proxyChecks[record.ID]; active != nil {
		operation := active.operation
		if overlay, exists := s.proxyResults[operation.ID]; exists {
			operation = overlay
		}
		result := success(map[string]any{"status": "accepted", "operation": operation}, operation.ID)
		tx, err := s.db.Begin()
		if err != nil {
			return storageFailure(err)
		}
		defer tx.Rollback()
		if err := s.commitProxyTransaction(tx, input.RequestID, signature, result, ""); err != nil {
			return storageFailure(err)
		}
		return result
	}
	operation := Operation{ID: id(), Kind: "proxy-check", State: "accepted", Stage: "queued", ProxyID: record.ID, Total: 1, CompletedIDs: []string{}}
	result := success(map[string]any{"status": "accepted", "operation": operation}, operation.ID)
	tx, err := s.db.Begin()
	if err != nil {
		return storageFailure(err)
	}
	defer tx.Rollback()
	encoded, _ := json.Marshal(operation)
	if _, err := tx.Exec("INSERT INTO operations(id,result_json) VALUES(?,?)", operation.ID, string(encoded)); err != nil {
		return storageFailure(err)
	}
	if err := s.commitProxyTransaction(tx, input.RequestID, signature, result, ""); err != nil {
		return storageFailure(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
	task := &proxyCheckTask{operation: operation, cancel: cancel, revision: record.Revision}
	s.proxyChecks[record.ID] = task
	s.proxyResults[operation.ID] = operation
	s.workers.Add(1)
	go s.runProxyCheck(ctx, task, record, ref)
	return result
}

func (s *Service) runProxyCheck(ctx context.Context, task *proxyCheckTask, record ProxyView, ref string) {
	defer s.workers.Done()
	defer task.cancel()
	select {
	case s.proxyCheckGate <- struct{}{}:
		defer func() { <-s.proxyCheckGate }()
	case <-ctx.Done():
		observed := &proxy.CheckError{Code: "OPERATION_CANCELLED", Message: "检查在等待网络调度期间取消，未发起请求。", Retryable: true}
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			observed.Code, observed.Message = "PROXY_CHECK_TIMEOUT", "检查在等待网络调度期间超过时限，未发起请求；请稍后重试。"
		}
		s.finishProxyCheck(task, proxy.Report{Mode: "native", AdapterVersion: proxy.CheckVersion, StartedAt: timestamp(), FinishedAt: timestamp(), Steps: []proxy.Step{}, Error: observed})
		return
	}
	s.mu.Lock()
	protected, err := s.readProtectedProxyCredentials(ref)
	s.mu.Unlock()
	var credentials *proxy.Credentials
	if err == nil {
		credentials, err = s.decodeProtectedProxyCredentials(ref, protected)
	}
	if credentials != nil {
		defer func() { credentials.Username, credentials.Password = "", "" }()
	}
	if ctx.Err() != nil {
		observed := &proxy.CheckError{Code: "OPERATION_CANCELLED", Message: "本次检查在读取受保护凭据期间取消，未发起网络请求。", Retryable: true}
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			observed.Code, observed.Message = "PROXY_CHECK_TIMEOUT", "本次检查在读取凭据期间超过时限，未发起网络请求。"
		}
		s.finishProxyCheck(task, proxy.Report{Mode: "native", AdapterVersion: proxy.CheckVersion, StartedAt: timestamp(), FinishedAt: timestamp(), Steps: []proxy.Step{}, Error: observed})
		return
	}
	s.mu.Lock()
	if err == nil {
		task.operation.State, task.operation.Stage = "running", "validation"
		s.proxyResults[task.operation.ID] = task.operation
	}
	s.mu.Unlock()
	if err != nil {
		s.finishProxyCheck(task, proxy.Report{Mode: "native", AdapterVersion: proxy.CheckVersion, StartedAt: timestamp(), FinishedAt: timestamp(), Steps: []proxy.Step{{Stage: "credentials", Status: "failed", Time: timestamp(), Message: "当前Windows用户无法读取受保护认证，未发起网络请求。"}}, Error: &proxy.CheckError{Code: "CREDENTIALS_UNAVAILABLE", Message: "本机认证读取失败；请明确替换凭据，没有无认证或直连回退。", Retryable: true}})
		return
	}
	progress := func(step proxy.Step) {
		s.mu.Lock()
		defer s.mu.Unlock()
		if s.proxyChecks[record.ID] != task || s.proxyPending[task.operation.ID] != nil {
			return
		}
		task.operation.State, task.operation.Stage = "running", step.Stage
		if task.operation.ProxyReport == nil {
			task.operation.ProxyReport = &proxy.Report{Mode: "native", AdapterVersion: proxy.CheckVersion, ProxyID: record.ID, Revision: record.Revision, StartedAt: step.Time, Steps: []proxy.Step{}}
		}
		// Copies keep prior bridge snapshots immutable while the worker advances.
		report := *task.operation.ProxyReport
		report.Steps = append(append([]proxy.Step(nil), report.Steps...), step)
		task.operation.ProxyReport = &report
		s.proxyResults[task.operation.ID] = task.operation
	}
	check := s.options.CheckProxy
	if check == nil {
		check = func(ctx context.Context, config proxy.Configuration, credentials *proxy.Credentials, progress func(proxy.Step)) proxy.Report {
			return proxy.Check(ctx, config, credentials, proxy.CheckOptions{}, progress)
		}
	}
	report := check(ctx, record.Configuration, credentials, progress)
	if credentials != nil {
		credentials.Username, credentials.Password = "", ""
	}
	s.finishProxyCheck(task, report)
}

func (s *Service) finishProxyCheck(task *proxyCheckTask, report proxy.Report) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.proxyChecks[task.operation.ProxyID] != task {
		return
	}
	if report.Mode != "native" {
		report = proxy.Report{Mode: "native", AdapterVersion: proxy.CheckVersion, StartedAt: timestamp(), FinishedAt: timestamp(), Steps: []proxy.Step{}, Error: &proxy.CheckError{Code: "CAPABILITY_UNSUPPORTED", Message: "非原生报告不能作为实际代理结果。", Retryable: false}}
	}
	report.ProxyID, report.Revision = task.operation.ProxyID, task.revision
	operation := task.operation
	operation.Stage, operation.State, operation.Error, operation.PersistencePending, operation.ProxyReport = "completed", "completed", nil, false, &report
	operation.CompletedIDs = []string{operation.ProxyID}
	if report.Error != nil {
		operation.Stage, operation.State, operation.CompletedIDs = "failed", "failed", []string{}
		operation.Error = &Error{Code: report.Error.Code, Message: report.Error.Message, Retryable: report.Error.Retryable}
		if report.Error.Code == "OPERATION_CANCELLED" {
			operation.State, operation.Stage = "cancelled", "cancelled"
		}
	}
	s.proxyPending[operation.ID] = &proxyCheckWrite{operation: operation, report: report}
	s.flushProxyCheck(operation.ID)
}

func (s *Service) flushProxyPersistence() {
	for operationID := range s.proxyPending {
		s.flushProxyCheck(operationID)
	}
}
func (s *Service) flushProxyCheck(operationID string) {
	pending := s.proxyPending[operationID]
	if pending == nil {
		return
	}
	if err := s.commitProxyCheck(pending); err != nil {
		overlay := pending.operation
		overlay.State, overlay.Stage, overlay.Error, overlay.PersistencePending = "failed", "storage-pending", storageFailure(err).Error, true
		s.proxyResults[operationID] = overlay
		return
	}
	delete(s.proxyPending, operationID)
	delete(s.proxyResults, operationID)
	if task := s.proxyChecks[pending.operation.ProxyID]; task != nil && task.operation.ID == operationID {
		delete(s.proxyChecks, pending.operation.ProxyID)
	}
}
func (s *Service) commitProxyCheck(pending *proxyCheckWrite) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	encodedReport, _ := json.Marshal(pending.report)
	updated, err := tx.Exec("UPDATE proxy_config SET check_json=? WHERE proxy_id=? AND revision=?", string(encodedReport), pending.operation.ProxyID, pending.report.Revision)
	if err != nil {
		return err
	}
	operation := pending.operation
	operation.PersistencePending = false
	if count, err := updated.RowsAffected(); err != nil {
		return err
	} else if count != 1 {
		operation.State, operation.Stage, operation.CompletedIDs = "failed", "stale-configuration", []string{}
		operation.Error = &Error{Code: "REVISION_CONFLICT", Message: "检查期间代理配置已改变，本次观测没有覆盖新修订。", Retryable: true}
	}
	encodedOperation, _ := json.Marshal(operation)
	if _, err := tx.Exec("UPDATE operations SET result_json=? WHERE id=?", string(encodedOperation), operation.ID); err != nil {
		return err
	}
	detail := "本次连接、TLS和隧道请求及实际出口已核对；不代表浏览器代理通道或断线保护通过。"
	if operation.Error != nil {
		detail = operation.Error.Message
	}
	if _, err := tx.Exec("INSERT INTO activities(id,created_at,action,target,detail) VALUES(?,?,?,?,?)", operation.ID, timestamp(), "检查代理", operation.ProxyID, detail); err != nil {
		return err
	}
	if s.options.BeforeCommit != nil {
		if err := s.options.BeforeCommit(); err != nil {
			return err
		}
	}
	return tx.Commit()
}
func (s *Service) cancelProxyOperation(operation Operation) Result {
	if task := s.proxyChecks[operation.ProxyID]; task != nil && task.operation.ID == operation.ID {
		task.cancel()
		if s.proxyPending[operation.ID] == nil {
			task.operation.CancelRequested = true
			s.proxyResults[operation.ID] = task.operation
			operation = task.operation
		}
	}
	return success(operation, operation.ID)
}

func (s *Service) recoverProxyChecks() error {
	rows, err := s.db.Query("SELECT result_json FROM operations WHERE json_extract(result_json,'$.kind')='proxy-check' AND (json_extract(result_json,'$.state') IN ('accepted','running') OR json_extract(result_json,'$.persistencePending')=1)")
	if err != nil {
		return err
	}
	operations := []Operation{}
	for rows.Next() {
		var text string
		if err := rows.Scan(&text); err != nil {
			rows.Close()
			return err
		}
		var operation Operation
		if json.Unmarshal([]byte(text), &operation) != nil {
			rows.Close()
			return errors.New("invalid proxy operation")
		}
		operations = append(operations, operation)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	if len(operations) == 0 {
		return nil
	}
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, operation := range operations {
		operation.State, operation.Stage, operation.PersistencePending, operation.ProxyReport = "failed", "application-interrupted", false, nil
		operation.Error = &Error{Code: "APPLICATION_INTERRUPTED", Message: "上次检查随应用退出而中断，没有自动重发网络请求；请重新检查。", Retryable: true}
		encoded, _ := json.Marshal(operation)
		if _, err := tx.Exec("UPDATE operations SET result_json=? WHERE id=?", string(encoded), operation.ID); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *Service) listProxyOperations() ([]Operation, error) {
	rows, err := s.db.Query("SELECT result_json FROM operations WHERE json_extract(result_json,'$.kind')='proxy-check' ORDER BY rowid DESC LIMIT 50")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	operations := []Operation{}
	for rows.Next() {
		var encoded string
		if err := rows.Scan(&encoded); err != nil {
			return nil, err
		}
		var operation Operation
		if json.Unmarshal([]byte(encoded), &operation) != nil {
			return nil, errors.New("invalid saved proxy operation")
		}
		if current, exists := s.proxyResults[operation.ID]; exists {
			operation = current
		}
		operations = append(operations, operation)
	}
	return operations, rows.Err()
}
