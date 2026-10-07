package workspace

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
)

func acceptedBatchOperation(result Result) (Operation, error) {
	encoded, err := json.Marshal(result.Data)
	if err != nil {
		return Operation{}, err
	}
	var data struct {
		Status    string    `json:"status"`
		Operation Operation `json:"operation"`
	}
	if json.Unmarshal(encoded, &data) != nil || !result.OK || result.Mode != "native" || data.Status != "accepted" || data.Operation.ID != result.OperationID || data.Operation.BatchReport == nil || data.Operation.Kind != "batch-"+data.Operation.BatchReport.Kind {
		return Operation{}, errors.New("invalid batch acceptance")
	}
	return data.Operation, nil
}

// Called under the service mutex only after a durable acceptance is confirmed.
// A request replay can attach the missing original worker, never a second one.
func (s *Service) startAcceptedBatch(plan batchPlan, operation Operation) {
	if plan.OperationID != operation.ID || plan.State != "accepted" && plan.State != "running" || s.batchTasks[plan.ID] != nil {
		return
	}
	if s.closed || s.closeRequested.Load() {
		task := &batchTask{planID: plan.ID, operationID: operation.ID, last: operation, finalState: "failed", finalCause: &Error{Code: "APPLICATION_INTERRUPTED", Message: "已核实受理但应用正在退出；未自动执行剩余项，可重开后明确继续。", Retryable: true}}
		s.batchTasks[plan.ID] = task
		_ = s.flushOneBatchFinal(task)
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	task := &batchTask{planID: plan.ID, operationID: operation.ID, last: operation, cancel: cancel}
	s.batchTasks[plan.ID] = task
	s.workers.Add(1)
	go s.runBatch(ctx, task)
}

func (s *Service) observeBatchAcceptance(result Result) (Result, error) {
	accepted, err := acceptedBatchOperation(result)
	if err != nil {
		return Result{}, err
	}
	plan, err := readBatchPlan(s.db, accepted.BatchReport.PlanID)
	if err != nil {
		return Result{}, err
	}
	var text string
	if err = s.db.QueryRow(`SELECT result_json FROM operations WHERE id=?`, accepted.ID).Scan(&text); err != nil {
		return Result{}, err
	}
	var operation Operation
	if json.Unmarshal([]byte(text), &operation) != nil || operation.ID != accepted.ID || operation.BatchReport == nil || operation.BatchReport.PlanID != plan.ID || operation.Kind != "batch-"+plan.Kind {
		return Result{}, errors.New("batch acceptance journal mismatch")
	}
	s.startAcceptedBatch(plan, operation)
	if task := s.batchTasks[plan.ID]; task != nil && task.operationID == operation.ID && task.pending != nil {
		operation = *task.pending
	}
	return success(map[string]any{"status": "accepted", "operation": operation}, operation.ID), nil
}

func (s *Service) deferBatchAcceptance(requestID, signature string, result Result, cause error) Result {
	operation, err := acceptedBatchOperation(result)
	if err != nil {
		return storageFailure(err)
	}
	pending := &batchAcceptance{requestID: requestID, signature: signature, planID: operation.BatchReport.PlanID, operation: operation, cause: cause}
	s.batchAcceptances[requestID] = pending
	observed, err := s.resolveBatchAcceptance(pending)
	if err == nil {
		return observed
	}
	rejected := storageFailure(err)
	rejected.OperationID = operation.ID
	return rejected
}

func (s *Service) resolveBatchAcceptance(pending *batchAcceptance) (Result, error) {
	var signature, text string
	err := s.db.QueryRow(`SELECT signature,result_json FROM requests WHERE id=?`, pending.requestID).Scan(&signature, &text)
	if err == sql.ErrNoRows {
		delete(s.batchAcceptances, pending.requestID)
		return Result{}, pending.cause
	}
	if err != nil {
		return Result{}, err
	}
	var result Result
	if signature != pending.signature || json.Unmarshal([]byte(text), &result) != nil || result.OperationID != pending.operation.ID {
		return Result{}, errors.New("unconfirmed batch request identity")
	}
	observed, err := s.observeBatchAcceptance(result)
	if err != nil {
		return Result{}, err
	}
	delete(s.batchAcceptances, pending.requestID)
	return observed, nil
}

func (s *Service) pendingBatchAcceptance(operationID string) *Operation {
	for _, pending := range s.batchAcceptances {
		if pending.operation.ID != operationID {
			continue
		}
		operation := pending.operation
		operation.State, operation.Stage, operation.PersistencePending = "failed", "acceptance-pending", true
		operation.Error = &Error{Code: "STORAGE_WRITE_FAILED", Message: "此批次受理的提交结果待核实，尚未调度副作用；请用原请求标识重读，不能重复创建计划。", Retryable: true}
		return &operation
	}
	return nil
}
