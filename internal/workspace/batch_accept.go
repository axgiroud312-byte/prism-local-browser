package workspace

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"strings"
	"time"
)

func (s *Service) acceptBatch(method, planID, operationID, requestID string) Result {
	if strings.TrimSpace(requestID) == "" {
		return failure("VALIDATION_FAILED", "缺少批次请求标识。", false)
	}
	encoded, _ := json.Marshal(struct{ Method, PlanID, OperationID string }{method, planID, operationID})
	digest := sha256.Sum256(encoded)
	signature := hex.EncodeToString(digest[:])
	var priorSignature, priorJSON string
	err := s.db.QueryRow("SELECT signature,result_json FROM requests WHERE id=?", requestID).Scan(&priorSignature, &priorJSON)
	if err == nil {
		if signature != priorSignature {
			return failure("REQUEST_ID_REUSED", "请求标识已用于其他批次内容，未重复执行。", false)
		}
		var prior Result
		if json.Unmarshal([]byte(priorJSON), &prior) != nil {
			return failure("STORAGE_READ_FAILED", "批次请求记录损坏。", true)
		}
		observed, observeErr := s.observeBatchAcceptance(prior)
		if observeErr != nil {
			return s.deferBatchAcceptance(requestID, signature, prior, observeErr)
		}
		return observed
	}
	if err != sql.ErrNoRows {
		return storageFailure(err)
	}
	if method == "Batch.Retry" {
		var text string
		if err = s.db.QueryRow("SELECT result_json FROM operations WHERE id=?", operationID).Scan(&text); err != nil {
			return failure("NOT_FOUND", "批次任务不存在。", true)
		}
		var operation Operation
		if json.Unmarshal([]byte(text), &operation) != nil || operation.BatchReport == nil || !strings.HasPrefix(operation.Kind, "batch-") {
			return failure("VALIDATION_FAILED", "只能继续指定持久批次，不重放其他任务。", false)
		}
		planID = operation.BatchReport.PlanID
	}
	plan, err := readBatchPlan(s.db, planID)
	if err != nil {
		return failure("NOT_FOUND", "批次计划不存在。", true)
	}
	for _, pending := range s.batchAcceptances {
		if pending.planID == plan.ID {
			return failure("BATCH_BUSY", "此计划前一受理的提交结果尚未核实，不能用另一请求重复执行。", true)
		}
	}
	if task := s.batchTasks[planID]; task != nil {
		return failure("BATCH_BUSY", "本计划仍执行或结果待保存，不能重复受理。", true)
	}
	if method == "Batch.Commit" {
		if plan.State != "preview" {
			return failure("BATCH_ALREADY_ACCEPTED", "这个计划已受理；请查询原任务或明确继续剩余项。", true)
		}
		expires, err := time.Parse(time.RFC3339Nano, plan.ExpiresAt)
		if err != nil || !time.Now().Before(expires) {
			return failure("PREVIEW_EXPIRED", "批次预览已过期，请重新查看计划。", true)
		}
	} else if plan.OperationID != operationID || plan.State != "cancelled" && plan.State != "failed" {
		return failure("REVISION_CONFLICT", "计划已被继续或已完成，旧任务不能再重复执行。", true)
	}
	plan.OperationID, plan.State, plan.CancelRequested, plan.Cursor, plan.Failed, plan.AttemptCompleted, plan.Error = id(), "accepted", false, 0, 0, 0, nil
	plan.Sequence++
	operation := batchOperation(plan)
	operation.Stage = "queued"
	result := success(map[string]any{"status": "accepted", "operation": operation}, operation.ID)
	tx, err := s.db.Begin()
	if err != nil {
		return storageFailure(err)
	}
	defer tx.Rollback()
	if method == "Batch.Retry" {
		if _, err = tx.Exec(`UPDATE batch_items SET state=CASE WHEN identity_json IS NULL THEN 'not-executed' ELSE 'prepared' END,error_json=NULL WHERE plan_id=? AND state='failed'`, plan.ID); err != nil {
			return storageFailure(err)
		}
	}
	encoded, _ = json.Marshal(operation)
	if _, err = tx.Exec("INSERT INTO operations(id,result_json) VALUES(?,?)", operation.ID, string(encoded)); err != nil {
		return storageFailure(err)
	}
	if err = persistBatchPlan(tx, plan, operation); err != nil {
		return storageFailure(err)
	}
	encoded, _ = json.Marshal(result)
	if _, err = tx.Exec("INSERT INTO requests(id,signature,result_json) VALUES(?,?,?)", requestID, signature, string(encoded)); err != nil {
		return storageFailure(err)
	}
	if s.options.BeforeCommit != nil {
		if err = s.options.BeforeCommit(); err != nil {
			return storageFailure(err)
		}
	}
	if err = tx.Commit(); err != nil {
		return s.deferBatchAcceptance(requestID, signature, result, err)
	}
	s.startAcceptedBatch(plan, operation)
	return result
}

func (s *Service) cancelBatchOperation(operation Operation) Result {
	if operation.BatchReport == nil {
		return failure("VALIDATION_FAILED", "批次报告无效。", false)
	}
	plan, err := readBatchPlan(s.db, operation.BatchReport.PlanID)
	if err != nil {
		return storageFailure(err)
	}
	if plan.OperationID != operation.ID || plan.State != "accepted" && plan.State != "running" {
		return success(operation, operation.ID)
	}
	if task := s.batchTasks[plan.ID]; task != nil && task.pending != nil {
		return success(*task.pending, operation.ID)
	}
	plan.CancelRequested = true
	plan.Sequence++
	operation = batchOperation(plan)
	operation.Stage = "cancel-requested"
	tx, err := s.db.Begin()
	if err != nil {
		return storageFailure(err)
	}
	defer tx.Rollback()
	if err = persistBatchPlan(tx, plan, operation); err != nil {
		return storageFailure(err)
	}
	if s.options.BeforeCommit != nil {
		if err = s.options.BeforeCommit(); err != nil {
			return storageFailure(err)
		}
	}
	if err = tx.Commit(); err != nil {
		return storageFailure(err)
	}
	if task := s.batchTasks[plan.ID]; task != nil {
		task.cancel()
	}
	return success(operation, operation.ID)
}
