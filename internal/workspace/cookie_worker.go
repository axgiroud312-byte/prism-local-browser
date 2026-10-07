package workspace

import (
	"context"
	"errors"
	"time"

	"github.com/axgiroud312-byte/prism-local-browser/internal/cookies"
)

func cookieError(err error) *Error {
	var classified *cookies.Error
	if errors.As(err, &classified) {
		return &Error{Code: classified.Code, Message: classified.Message, Retryable: classified.Retryable}
	}
	return &Error{Code: "COOKIE_CONTROL_FAILED", Message: "本次受控Cookie操作失败，不回显原始通道内容；已完成项保留，未改写其他目标。", Retryable: true}
}

func (s *Service) cookieTaskReady(task *cookieImportTask) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	slot := task.slot
	if s.closed || s.closeRequested.Load() || s.backupUses[task.operation.EnvironmentID] != nil || s.runtimeSlots[task.operation.EnvironmentID] != slot || slot.process == nil || slot.session.SessionID != task.operation.SessionID || slot.session.State != "running" || slot.session.PersistencePending || slot.session.NeedsReconcile || slot.session.NetworkFault != nil || runtimeStopPending(slot) {
		return false
	}
	_, revision, _, err := s.readEnvironment(task.operation.EnvironmentID)
	return err == nil && revision == task.operation.CookieReport.Revision
}

func copyCookieOperation(operation Operation) Operation {
	if operation.CookieReport != nil {
		report := *operation.CookieReport
		report.Items = append([]CookieItemResult(nil), report.Items...)
		operation.CookieReport = &report
	}
	operation.CompletedIDs = append([]string{}, operation.CompletedIDs...)
	return operation
}

func (s *Service) publishCookieProgress(task *cookieImportTask, operation Operation) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.cookieTasks[operation.EnvironmentID] == task {
		operation.CancelRequested = task.operation.CancelRequested
		task.operation = copyCookieOperation(operation)
		s.cookieResults[operation.ID] = copyCookieOperation(operation)
	}
}

func recountCookieReport(report *CookieImportReport) {
	report.VerifiedCount, report.WrittenCount, report.AlreadyMatchedCount, report.FailedCount, report.SkippedCount, report.UnconfirmedCount = 0, 0, 0, 0, 0, 0
	for _, item := range report.Items {
		switch item.Status {
		case "verified":
			report.VerifiedCount++
			report.WrittenCount++
		case "already-matched":
			report.VerifiedCount++
			report.AlreadyMatchedCount++
		case "unknown":
			report.FailedCount++
			report.UnconfirmedCount++
		case "failed":
			report.FailedCount++
		case "expired", "cancelled":
			report.SkippedCount++
		}
	}
}

func (s *Service) runCookieImport(ctx context.Context, task *cookieImportTask) {
	defer s.workers.Done()
	defer task.cancel()
	s.mu.Lock()
	operation := copyCookieOperation(task.operation)
	s.mu.Unlock()
	report := operation.CookieReport
	operation.State, operation.Stage = "running", "reading-current"
	s.publishCookieProgress(task, operation)
	if report.Policy == "replace-all" {
		if ctx.Err() != nil || !s.cookieTaskReady(task) {
			report.ClearState = "not-attempted"
		} else {
			operation.Stage = "clearing-explicitly"
			s.publishCookieProgress(task, operation)
			bounded, cancel := context.WithTimeout(ctx, 10*time.Second)
			err := task.transport.ClearCookies(bounded)
			cancel()
			if err != nil {
				operation.Error = cookieError(err)
				report.ClearState = "unknown"
			} else {
				report.ClearState = "verified-empty"
			}
		}
	}
	for index := range report.Items {
		item := &report.Items[index]
		if ctx.Err() != nil {
			item.Status, item.ErrorCode, item.Message = "cancelled", "OPERATION_CANCELLED", "后续行未发送；已核对和结果未知的条目保留。"
			continue
		}
		if !s.cookieTaskReady(task) {
			item.Status, item.ErrorCode, item.Message = "failed", "COOKIE_SESSION_CHANGED", "指定会话/修订不再可安全控制，未改写后来会话。"
			if operation.Error == nil {
				operation.Error = &Error{Code: item.ErrorCode, Message: item.Message, Retryable: true}
			}
			continue
		}
		if report.Policy == "replace-all" && report.ClearState != "verified-empty" {
			item.Status, item.ErrorCode, item.Message = "failed", "COOKIE_CLEAR_UNCONFIRMED", "未确认清空，未发送本行写入；重试不会自动再次清空。"
			continue
		}
		value := task.values[item.Index]
		if value.Expires != nil && *value.Expires <= float64(time.Now().UnixNano())/1e9 {
			item.Status, item.Expired, item.ErrorCode, item.Message = "expired", true, "COOKIE_EXPIRED", "执行时已过期，本行跳过，不删除同键旧Cookie或续期。"
			continue
		}
		operation.Stage = "writing-and-reading-back"
		s.publishCookieProgress(task, operation)
		bounded, cancel := context.WithTimeout(ctx, 10*time.Second)
		result, err := task.transport.ApplyCookie(bounded, value)
		cancel()
		if err != nil {
			classified := cookieError(err)
			item.Status, item.ErrorCode, item.Message = "failed", classified.Code, classified.Message
			if classified.Code == "COOKIE_WRITE_UNCONFIRMED" {
				item.Status = "unknown"
			}
			if operation.Error == nil {
				operation.Error = classified
			}
		} else if result.Status == "verified" || result.Status == "already-matched" {
			item.Status = result.Status
		} else {
			item.Status, item.ErrorCode, item.Message = "unknown", "COOKIE_WRITE_UNCONFIRMED", "内核没有给出可核对结果，不能计为真实成功。"
		}
		if !s.cookieTaskReady(task) && operation.Error == nil {
			operation.Error = &Error{Code: "COOKIE_SESSION_CHANGED", Message: "导入期间指定会话退出或变更，已有读回事实保留；未继续写入其他会话。", Retryable: true}
		}
		recountCookieReport(report)
		s.publishCookieProgress(task, operation)
	}
	recountCookieReport(report)
	report.FinishedAt = timestamp()
	operation.State, operation.Stage = "completed", "verified"
	if report.FailedCount != 0 || report.SkippedCount != 0 || operation.Error != nil {
		operation.State, operation.Stage = "failed", "partial-or-unconfirmed"
		if operation.Error == nil {
			operation.Error = &Error{Code: "COOKIE_IMPORT_INCOMPLETE", Message: "仅计入逐条读回通过项，部分条目未导入；请查看逐项结果。", Retryable: true}
		}
	}
	if ctx.Err() != nil {
		operation.State, operation.Stage = "cancelled", "cancelled"
	}
	if report.VerifiedCount == operation.Total && operation.State == "completed" {
		operation.CompletedIDs = []string{operation.EnvironmentID}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	operation.CancelRequested = task.operation.CancelRequested
	for index := range task.values {
		delete(task.values, index)
	}
	task.values = nil
	task.operation = copyCookieOperation(operation)
	s.cookiePending[operation.ID] = copyCookieOperation(operation)
	_ = s.flushOneCookieWrite(operation)
}

func (s *Service) cancelCookieOperation(operation Operation) Result {
	if operation.State != "accepted" && operation.State != "running" || operation.PersistencePending {
		return success(operation, operation.ID)
	}
	if task := s.cookieTasks[operation.EnvironmentID]; task != nil && task.operation.ID == operation.ID {
		operation = copyCookieOperation(task.operation)
		operation.CancelRequested = true
		task.operation = operation
		s.cookieResults[operation.ID] = copyCookieOperation(operation)
		task.cancel()
	}
	return success(operation, operation.ID)
}

func (s *Service) cancelRuntimeCookies(environmentID string) {
	if task := s.cookieTasks[environmentID]; task != nil {
		task.cancel()
	}
}
