package workspace

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/axgiroud312-byte/prism-local-browser/internal/kernel"
)

func batchExecutionError(err error) *Error {
	var classified *kernel.Problem
	if errors.As(err, &classified) {
		return kernelFailure(err).Error
	}
	return storageFailure(err).Error
}

func (s *Service) finishBatch(task *batchTask, cause error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.batchTasks[task.planID] != task {
		return
	}
	task.finalState = "completed"
	if cause != nil {
		if errors.Is(cause, context.Canceled) {
			if s.closed || s.closeRequested.Load() {
				task.finalState = "failed"
				task.finalCause = &Error{Code: "APPLICATION_INTERRUPTED", Message: "应用退出中断批次；已提交项保留，未执行项不会自动重做。", Retryable: true}
			} else {
				task.finalState = "cancelled"
				task.finalCause = &Error{Code: "OPERATION_CANCELLED", Message: "只取消此批次剩余项；已提交项保留，可明确继续未完成项。", Retryable: true}
			}
		} else {
			task.finalState = "failed"
			task.finalCause = batchExecutionError(cause)
		}
	}
	_ = s.flushOneBatchFinal(task)
}

// A commit error is resolved from the journal first. This path only saves
// terminal observations, never repeats identity allocation, directories or DB
// environment mutations. It also handles a transaction whose outcome was unknown.
func (s *Service) flushOneBatchFinal(task *batchTask) error {
	plan, err := readBatchPlan(s.db, task.planID)
	if err == nil && plan.OperationID != task.operationID {
		delete(s.batchTasks, task.planID)
		return nil
	}
	operation := task.last
	if err == nil {
		plan.State, plan.Error = task.finalState, task.finalCause
		if plan.State == "completed" && (plan.Completed != plan.Total || plan.Failed != 0) {
			plan.State = "failed"
			plan.Error = &Error{Code: "BATCH_PARTIAL_FAILED", Message: "仅已提交项目计完成；其余失败和未执行项保留，重试不会重复已完成环境。", Retryable: true}
		}
		plan.Sequence++
		operation = batchOperation(plan)
		operation.Stage = "finished"
		operation.BatchReport.FinishedAt = timestamp()
		tx, beginErr := s.db.Begin()
		if beginErr != nil {
			err = beginErr
		} else {
			err = persistBatchPlan(tx, plan, operation)
			if err == nil {
				_, err = tx.Exec("INSERT OR IGNORE INTO activities(id,created_at,action,target,detail) VALUES(?,?,?,?,?)", operation.ID, timestamp(), "持久批次 "+plan.Kind, plan.ID, "逐项事务记录完成/失败/未执行；取消和重开不重复已完成项，不复制登录数据。")
			}
			if err == nil && s.options.BeforeCommit != nil {
				err = s.options.BeforeCommit()
			}
			if err == nil {
				err = tx.Commit()
			} else {
				_ = tx.Rollback()
			}
		}
	}
	if err != nil {
		operation.State, operation.Stage, operation.PersistencePending, operation.Error = "failed", "storage-pending", true, storageFailure(err).Error
		task.pending = &operation
		return err
	}
	delete(s.batchTasks, task.planID)
	return nil
}
func (s *Service) flushBatchPersistence() {
	for _, pending := range s.batchAcceptances {
		_, _ = s.resolveBatchAcceptance(pending)
	}
	for _, task := range s.batchTasks {
		if task.pending != nil {
			_ = s.flushOneBatchFinal(task)
		}
	}
}
func (s *Service) pendingBatchOperation(operationID string) *Operation {
	if pending := s.pendingBatchAcceptance(operationID); pending != nil {
		return pending
	}
	for _, task := range s.batchTasks {
		if task.operationID == operationID && task.pending != nil {
			return task.pending
		}
	}
	return nil
}

func (s *Service) listBatchOperations() ([]Operation, error) {
	rows, err := s.db.Query(`SELECT result_json FROM operations WHERE json_extract(result_json,'$.kind') LIKE 'batch-%' ORDER BY rowid DESC LIMIT 30`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	operations := []Operation{}
	for rows.Next() {
		var text string
		var operation Operation
		if err = rows.Scan(&text); err != nil {
			return nil, err
		}
		if json.Unmarshal([]byte(text), &operation) != nil || operation.BatchReport == nil {
			return nil, errors.New("invalid batch operation")
		}
		if pending := s.pendingBatchOperation(operation.ID); pending != nil {
			operation = *pending
		}
		operations = append(operations, operation)
	}
	return operations, rows.Err()
}

func (s *Service) recoverBatchTasks() error {
	rows, err := s.db.Query(`SELECT id FROM batch_plans WHERE state IN ('accepted','running')`)
	if err != nil {
		return err
	}
	ids := []string{}
	for rows.Next() {
		var planID string
		if err = rows.Scan(&planID); err != nil {
			rows.Close()
			return err
		}
		ids = append(ids, planID)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	for _, planID := range ids {
		plan, err := readBatchPlan(s.db, planID)
		if err != nil {
			return err
		}
		task := &batchTask{planID: plan.ID, operationID: plan.OperationID, last: batchOperation(plan), finalState: "failed", finalCause: &Error{Code: "APPLICATION_INTERRUPTED", Message: "上次批次因应用中断；已提交项按持久日志保留，未执行/已准备项需明确继续，不自动复制或分配。", Retryable: true}}
		if err = s.flushOneBatchFinal(task); err != nil {
			return err
		}
	}
	return nil
}
