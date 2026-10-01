package workspace

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/axgiroud312-byte/prism-local-browser/internal/backup"
)

func (s *Service) acceptRestore(input RestoreRequest) Result {
	if input.PreviewID == "" || !backup.Hash(input.ArchiveSHA256) || strings.TrimSpace(input.RequestID) == "" || len(input.RequestID) > 128 {
		return failure("VALIDATION_FAILED", "恢复需要原预览、包摘要和请求标识，不接受路径或配置覆盖。", false)
	}
	encoded, _ := json.Marshal(input)
	sum := sha256.Sum256(append([]byte("Backup.ApplyRestore:"), encoded...))
	signature := hex.EncodeToString(sum[:])
	if task := s.restoreTask; task != nil && task.acceptancePending && task.operation.RestoreReport.RequestID == input.RequestID {
		if signature != task.signature {
			return failure("REQUEST_ID_REUSED", "原恢复确认不能改变。", false)
		}
		if err := s.confirmRestoreAcceptance(task, signature); err != nil {
			return s.restoreUnconfirmed(task, err)
		}
		return success(map[string]any{"status": "accepted", "operation": copyRestoreOperation(task.operation)}, task.operation.ID)
	}
	var priorSignature, text string
	err := s.db.QueryRow("SELECT signature,result_json FROM requests WHERE id=?", input.RequestID).Scan(&priorSignature, &text)
	if err == nil {
		if signature != priorSignature {
			return failure("REQUEST_ID_REUSED", "同一请求不能更换恢复包或影响确认。", false)
		}
		var prior Result
		if json.Unmarshal([]byte(text), &prior) != nil || prior.OperationID == "" {
			return failure("STORAGE_READ_FAILED", "原恢复受理无法核对；未重新执行。", true)
		}
		if task := s.restoreTask; task != nil && task.operation.ID == prior.OperationID {
			if task.acceptancePending {
				if err = s.confirmRestoreAcceptance(task, signature); err != nil {
					return s.restoreUnconfirmed(task, err)
				}
			}
			return success(map[string]any{"status": "accepted", "operation": copyRestoreOperation(task.operation)}, prior.OperationID)
		}
		var op Operation
		if s.db.QueryRow("SELECT result_json FROM operations WHERE id=?", prior.OperationID).Scan(&text) != nil || json.Unmarshal([]byte(text), &op) != nil || op.Kind != "backup-restore" {
			return failure("STORAGE_READ_FAILED", "原恢复结果无法读取；未重新执行。", true)
		}
		return success(map[string]any{"status": "accepted", "operation": op}, op.ID)
	}
	if err != sql.ErrNoRows {
		return storageFailure(err)
	}
	if s.restoreTask != nil {
		return failure("RESTORE_INCOMPLETE", "已有恢复操作受理或结果待核对；只读取原任务，不创建第二个目录写入者。", true)
	}
	d := s.restorePreview
	if d == nil || d.preview.PreviewID != input.PreviewID || time.Now().After(d.expires) || d.preview.ArchiveSHA256 != input.ArchiveSHA256 {
		return failure("PREVIEW_EXPIRED", "恢复预览已失效或摘要不同，请重新只读预检。", true)
	}
	if !d.preview.CanRestore {
		return failure("BACKUP_INVALID", "身份冲突或精确内核缺失尚未解决，未进入恢复。", false)
	}
	if d.preview.OverwriteCount > 0 && !input.ConfirmOverwrite || d.preview.CredentialReentryCount > 0 && !input.AcknowledgeCredentials {
		return failure("VALIDATION_FAILED", "请明确确认覆盖影响及凭据需要重新输入的范围。", false)
	}
	if s.recycleTask != nil || s.restorePreflight != nil || s.kernelTask != nil || len(s.batchTasks) > 0 || len(s.batchAcceptances) > 0 || len(s.backupTasks) > 0 || len(s.cookieTasks) > 0 || len(s.proxyChecks) > 0 || len(s.proxyPending) > 0 || len(s.cookiePending) > 0 || len(s.runtimePending) > 0 {
		return failure("PROFILE_BUSY", "当前有维护、批次、Cookie或观测待保存任务；先完成它，再恢复。", true)
	}
	baseline, err := restoreBaseline(s.db)
	if err != nil {
		return storageFailure(err)
	}
	if baseline != d.baseline {
		return failure("REVISION_CONFLICT", "预检后当前配置已变化，请重新查看恢复影响。", true)
	}
	plan := restorePlan{JournalVersion: 3, ID: id(), Source: d.path, Baseline: d.baseline, ArchiveSHA256: input.ArchiveSHA256, Environments: []restoreStoredEnvironment{}, Proxies: []restoreStoredProxy{}, KernelMapping: d.kernelMapping, Moves: []restoreMove{}}
	for index, item := range d.data.environments {
		if slot := s.runtimeSlots[item.manifest.ID]; slot != nil {
			if slot.session.NeedsReconcile || slot.session.PersistencePending {
				return failure("SESSION_IDENTITY_UNCONFIRMED", "受影响会话退出状态未核对，不能切换数据。", true)
			}
			if s.runtimeOwnsProfileUse(item.manifest.ID) && !input.StopRunning {
				return failure("PROFILE_BUSY", "需明确允许正常停止受影响环境；不自动强制结束。", true)
			}
		}
		if s.profileUses[item.manifest.ID] && !s.runtimeOwnsProfileUse(item.manifest.ID) {
			return failure("PROFILE_BUSY", "受影响环境处于其他维护中，未抢占目录。", true)
		}
		plan.Environments = append(plan.Environments, restoreStoredEnvironment{Manifest: item.manifest, Environment: item.environment, Code: item.code, DataState: item.dataState, History: item.history, ExistingRevision: d.impacts[index].CurrentRevision})
	}
	for _, item := range d.data.proxies {
		plan.Proxies = append(plan.Proxies, restoreStoredProxy{ID: item.ID, Ref: item.Ref, Configuration: item.Configuration, Protected: item.Protected, Revision: item.Revision})
	}
	op := Operation{ID: plan.ID, Kind: "backup-restore", State: "accepted", Stage: "accepted", Total: len(plan.Environments), CompletedIDs: []string{}, RestoreReport: &RestoreReport{Mode: "native", RequestID: input.RequestID, PreviewID: input.PreviewID, ArchiveSHA256: input.ArchiveSHA256, Sequence: 1, EnvironmentCount: len(plan.Environments), Protected: true, CredentialReentryCount: d.preview.CredentialReentryCount}}
	task := &restoreTask{operation: op, plan: plan, phase: "accepted"}
	result := success(map[string]any{"status": "accepted", "operation": op}, op.ID)
	resultJSON, _ := json.Marshal(result)
	opJSON, _ := json.Marshal(op)
	task.acceptanceOperation = string(opJSON)
	task.signature = signature
	planJSON, _ := json.Marshal(plan)
	tx, err := s.db.Begin()
	if err != nil {
		return storageFailure(err)
	}
	defer tx.Rollback()
	if _, err = tx.Exec("INSERT INTO operations(id,result_json) VALUES(?,?)", op.ID, string(opJSON)); err == nil {
		_, err = tx.Exec("INSERT INTO restore_jobs(operation_id,plan_json,phase,request_id,signature) VALUES(?,?,?,?,?)", op.ID, string(planJSON), "accepted", input.RequestID, signature)
	}
	if err == nil {
		_, err = tx.Exec("INSERT INTO requests(id,signature,result_json) VALUES(?,?,?)", input.RequestID, signature, string(resultJSON))
	}
	if err == nil && s.options.BeforeCommit != nil {
		err = s.options.BeforeCommit()
	}
	if err != nil {
		return storageFailure(err)
	}
	err = tx.Commit()
	s.restoreTask = task
	s.restorePreview = nil
	if err != nil {
		task.acceptancePending = true
		if err = s.confirmRestoreAcceptance(task, signature); err != nil {
			return s.restoreUnconfirmed(task, err)
		}
	} else {
		s.startRestore(task)
	}
	return success(map[string]any{"status": "accepted", "operation": copyRestoreOperation(task.operation)}, op.ID)
}

func (s *Service) confirmRestoreAcceptance(task *restoreTask, signature string) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var savedSignature, plan, op, requestSignature, requestJSON string
	err = tx.QueryRow("SELECT r.signature,r.plan_json,o.result_json,q.signature,q.result_json FROM restore_jobs r JOIN operations o ON o.id=r.operation_id JOIN requests q ON q.id=r.request_id WHERE r.operation_id=? AND r.request_id=?", task.plan.ID, task.operation.RestoreReport.RequestID).Scan(&savedSignature, &plan, &op, &requestSignature, &requestJSON)
	if err == sql.ErrNoRows {
		var count int
		if err = tx.QueryRow("SELECT (SELECT COUNT(*) FROM restore_jobs WHERE operation_id=? OR request_id=?)+(SELECT COUNT(*) FROM operations WHERE id=?)+(SELECT COUNT(*) FROM requests WHERE id=?)", task.plan.ID, task.operation.RestoreReport.RequestID, task.plan.ID, task.operation.RestoreReport.RequestID).Scan(&count); err != nil {
			return err
		}
		if count == 0 {
			s.restoreTask = nil
			return &Error{Code: "RESTORE_NOT_ACCEPTED", Message: "已核实原恢复未落盘且未执行目录切换；请重新预检。", Retryable: true}
		}
		return errors.New("partial restore acceptance protected")
	}
	if err != nil {
		return err
	}
	wantPlan, _ := json.Marshal(task.plan)
	var accepted Result
	if json.Unmarshal([]byte(requestJSON), &accepted) != nil || !accepted.OK || accepted.OperationID != task.plan.ID || signature != requestSignature || signature != savedSignature || plan != string(wantPlan) || op != task.acceptanceOperation {
		return errors.New("restore acceptance unconfirmed")
	}
	if err = tx.Rollback(); err != nil {
		return err
	}
	if err = json.Unmarshal([]byte(op), &task.operation); err != nil {
		return err
	}
	task.acceptancePending = false
	s.startRestore(task)
	return nil
}
func (s *Service) restoreUnconfirmed(task *restoreTask, err error) Result {
	var known *Error
	if errors.As(err, &known) && known.Code == "RESTORE_NOT_ACCEPTED" {
		return Result{Mode: "native", Error: known}
	}
	if task.acceptancePending {
		task.operation = copyRestoreOperation(task.operation)
		task.operation.Stage = "acceptance-pending"
		task.operation.PersistencePending = true
	}
	r := storageFailure(err)
	r.Error.Code = "RESTORE_INCOMPLETE"
	r.Error.Message = "恢复受理/日志写入结果尚未核实；保护保持，只核实原请求，不重新提交另一恢复。"
	r.OperationID = task.operation.ID
	return r
}
func (s *Service) startRestore(task *restoreTask) {
	if task.running {
		return
	}
	task.running = true
	ctx, cancel := context.WithCancel(context.Background())
	task.cancel = cancel
	s.workers.Add(1)
	go s.runRestore(ctx, task)
}
func (s *Service) cancelRestore(task *restoreTask) Result {
	if task.acceptancePending || !task.running {
		return success(copyRestoreOperation(task.operation), task.operation.ID)
	}
	if task.cancel == nil {
		return failure("RESTORE_FINALIZING", "正在核对或回滚原恢复；收尾不能再次取消，请等待读取结果。", true)
	}
	committed, err := s.restoreCommitted(task.operation.ID)
	if err != nil {
		return s.restoreUnconfirmed(task, err)
	}
	if committed {
		return failure("RESTORE_COMMITTED", "数据库已提交完整新状态；迟到取消不能撤销，正在核实最终结果。", false)
	}
	op := copyRestoreOperation(task.operation)
	op.CancelRequested = true
	op.RestoreReport.Sequence++
	if err = s.persistRestore(task, op, task.phase); err != nil {
		return s.restoreUnconfirmed(task, err)
	}
	task.operation = op
	task.cancel()
	return success(op, op.ID)
}
