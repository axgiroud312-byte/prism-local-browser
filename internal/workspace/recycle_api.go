package workspace

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/axgiroud312-byte/prism-local-browser/internal/backup"
)

func recycleProblem(err error) Result {
	var safe *Error
	if errors.As(err, &safe) {
		return Result{Mode: "native", Error: safe}
	}
	return failure("RECYCLE_INCOMPLETE", "回收目录或配置尚未完整核对，保留原数据和日志；请检查目录占用、空间、权限及原副本后重试原任务。", true)
}
func (s *Service) recycleIdle() bool {
	return s.restoreTask == nil && s.recycleTask == nil && s.restorePreflight == nil && s.kernelTask == nil && len(s.batchTasks) == 0 && len(s.batchAcceptances) == 0 && len(s.backupTasks) == 0 && len(s.cookieTasks) == 0 && len(s.proxyChecks) == 0 && len(s.proxyPending) == 0 && len(s.cookiePending) == 0 && len(s.runtimePending) == 0
}
func (s *Service) recycleTargetFree(environmentID string) error {
	if s.profileUses[environmentID] || s.runtimeOwnsProfileUse(environmentID) || s.runtimePending[environmentID] != nil {
		return &Error{Code: "PROFILE_BUSY", Message: "所选环境仍在运行、停止未确认或维护中；请先正常停止并核对，不自动强制结束。", Retryable: true}
	}
	if slot := s.runtimeSlots[environmentID]; slot != nil && (slot.session.NeedsReconcile || slot.session.PersistencePending || runtimeStopPending(slot)) {
		return &Error{Code: "PROFILE_BUSY", Message: "原会话退出结果尚未确认，不能移除。", Retryable: true}
	}
	busy, err := s.backupBatchBusy(environmentID)
	if err != nil {
		return err
	}
	if busy {
		return &Error{Code: "PROFILE_BUSY", Message: "所选环境仍有已受理或受理未知批次，先结束原任务。", Retryable: true}
	}
	return nil
}
func (s *Service) recycleCall(request Request) Result {
	switch request.Method {
	case "Recycle.Preview":
		var input struct {
			Action string   `json:"action"`
			IDs    []string `json:"ids"`
		}
		if decode(request.Payload, &input) != nil || (input.Action != "remove" && input.Action != "restore" && input.Action != "purge") || len(input.IDs) == 0 {
			return failure("VALIDATION_FAILED", "请选择明确环境或回收项及操作，不接受路径。", false)
		}
		return s.previewRecycle(input.Action, input.IDs)
	case "Recycle.ReadPage":
		var input recyclePageRequest
		if decode(request.Payload, &input) != nil || input.Offset < 0 || input.PageSize < 1 || input.PageSize > 100 || input.OperationID != "" && input.PreviewID != "" {
			return failure("VALIDATION_FAILED", "回收分页范围无效。", false)
		}
		return s.readRecyclePage(input)
	case "Recycle.Commit":
		var input RecycleRequest
		if decode(request.Payload, &input) != nil {
			return failure("VALIDATION_FAILED", "回收确认只接受原预览和请求标识。", false)
		}
		return s.acceptRecycle(input)
	case "Recycle.Recover":
		var input struct {
			OperationID string `json:"operationId"`
		}
		if decode(request.Payload, &input) != nil || input.OperationID == "" {
			return failure("VALIDATION_FAILED", "请选择原回收任务。", false)
		}
		return s.recoverRecycle(input.OperationID)
	}
	return failure("CAPABILITY_UNSUPPORTED", "未知回收操作。", false)
}
func (s *Service) previewRecycle(action string, ids []string) Result {
	if !s.recycleIdle() {
		return failure("PROFILE_BUSY", "已有维护或持久任务；完成后再查看回收影响。", true)
	}
	d := &recycleDraft{expires: time.Now().Add(15 * time.Minute), plan: recyclePlan{Version: 1, ID: id(), Action: action, Items: []recyclePlanItem{}}}
	d.preview = RecyclePreview{Mode: "native", PreviewID: id(), Action: action, ExpiresAt: d.expires.UTC().Format(time.RFC3339Nano)}
	seen := map[string]bool{}
	for _, selected := range ids {
		if !backup.CanonicalID(selected) || seen[selected] {
			return failure("VALIDATION_FAILED", "选择中有无效或重复ID。", false)
		}
		seen[selected] = true
		var entry recycleEntry
		if action == "remove" {
			e, revision, _, err := s.readEnvironment(selected)
			if err != nil {
				return failure("NOT_FOUND", "所选环境已不存在或已在回收区；未改动其他环境。", true)
			}
			if err := s.recycleTargetFree(selected); err != nil {
				return recycleProblem(err)
			}
			ref, _ := dataReference(selected)
			identity, present, err := backup.IdentifyTree(s.root, ref)
			if err != nil {
				return recycleProblem(err)
			}
			target, err := s.backupDataTarget(selected, ref)
			if err != nil || !present && target.DirectoryRequired {
				return failure("RECYCLE_INCOMPLETE", "已有数据的环境目录缺失或初始化事实未知，不能当空环境移除。", true)
			}
			baseline, err := recycleBaseline(s.db, selected)
			if err != nil {
				return recycleProblem(err)
			}
			entry = recycleEntry{TrashID: id(), Reference: ref, Identity: identity, Files: []backup.File{}, Baseline: baseline, Item: RecycleItem{ID: selected, EnvironmentID: selected, Name: e.Name, Seed: e.Seed, KernelID: e.CoreID, Revision: revision, DataPresent: present, RemovedAt: timestamp(), State: "pending"}}
		} else {
			var err error
			entry, err = readRecycleEntry(s.db, selected)
			if err != nil {
				return failure("NOT_FOUND", "回收项不存在或记录无法核实，未找回或删除其他项。", true)
			}
			baseline, err := recycleBaseline(s.db, entry.Item.EnvironmentID)
			if err != nil || baseline != entry.Baseline {
				return failure("REVISION_CONFLICT", "回收配置与原记录不一致，保留保护。", true)
			}
			if err := s.recycleTargetFree(entry.Item.EnvironmentID); err != nil {
				return recycleProblem(err)
			}
			if action == "restore" {
				if _, exists, err := backup.IdentifyTree(s.root, entry.Reference); err != nil || exists {
					return failure("RECYCLE_CONFLICT", "原数据位置存在未知目录或无法核对，不覆盖、不克隆新身份。", true)
				}
			}
		}
		var recorded bool
		if err := s.db.QueryRow(`SELECT EXISTS(SELECT 1 FROM backup_exports b JOIN operations o ON o.id=b.operation_id,json_each(b.targets_json) t WHERE b.phase='completed' AND json_extract(o.result_json,'$.backupReport.published')=1 AND json_extract(t.value,'$.id')=?)`, entry.Item.EnvironmentID).Scan(&recorded); err != nil {
			return recycleProblem(err)
		}
		entry.Item.BackupRecorded = recorded
		if recorded {
			d.preview.BackupCount++
		}
		if entry.Item.DataPresent {
			d.preview.DataCount++
		}
		d.plan.Items = append(d.plan.Items, recyclePlanItem{Entry: entry, Result: "pending"})
	}
	d.preview.Total = len(d.plan.Items)
	s.recycleDraft = d
	return s.readRecyclePage(recyclePageRequest{PreviewID: d.preview.PreviewID, PageSize: 25})
}
func (s *Service) readRecyclePage(input recyclePageRequest) Result {
	page := RecyclePage{Mode: "native", Offset: input.Offset, PageSize: input.PageSize, Items: []RecycleItem{}}
	if input.PreviewID != "" || input.OperationID != "" {
		var plan recyclePlan
		if input.PreviewID != "" {
			d := s.recycleDraft
			if d == nil || d.preview.PreviewID != input.PreviewID || time.Now().After(d.expires) {
				return failure("PREVIEW_EXPIRED", "回收影响预览已过期。", true)
			}
			plan = d.plan
			page.Preview = &d.preview
		} else {
			var op Operation
			var err error
			plan, op, _, err = s.readRecycleJournal(input.OperationID)
			if err != nil {
				return recycleProblem(err)
			}
			if s.recycleTask != nil && s.recycleTask.operation.ID == input.OperationID {
				plan = s.recycleTask.plan
				op = copyRecycleOperation(s.recycleTask.operation)
			}
			page.Operation = &op
		}
		page.Total = len(plan.Items)
		if input.Offset > page.Total {
			return failure("VALIDATION_FAILED", "页位置超出当前结果。", true)
		}
		for _, item := range plan.Items[input.Offset:min(input.Offset+input.PageSize, page.Total)] {
			value := item.Entry.Item
			if input.OperationID != "" {
				value.State = item.Result
				value.Error = item.Error
			}
			page.Items = append(page.Items, value)
		}
	} else {
		if err := s.db.QueryRow("SELECT COUNT(*) FROM environment_trash").Scan(&page.Total); err != nil {
			return recycleProblem(err)
		}
		if input.Offset > page.Total {
			return failure("VALIDATION_FAILED", "回收列表已变化，请回到首页。", true)
		}
		rows, err := s.db.Query("SELECT entry_json FROM environment_trash ORDER BY rowid DESC LIMIT ? OFFSET ?", input.PageSize, input.Offset)
		if err != nil {
			return recycleProblem(err)
		}
		defer rows.Close()
		for rows.Next() {
			var text string
			var entry recycleEntry
			if err := rows.Scan(&text); err != nil {
				return recycleProblem(err)
			}
			if err := backup.DecodeJSON([]byte(text), &entry); err != nil {
				return recycleProblem(err)
			}
			page.Items = append(page.Items, entry.Item)
		}
		if err := rows.Err(); err != nil {
			return recycleProblem(err)
		}
	}
	return success(page, "")
}
func (s *Service) acceptRecycle(input RecycleRequest) Result {
	if !backup.CanonicalID(input.PreviewID) || !input.Confirm || strings.TrimSpace(input.RequestID) == "" || len(input.RequestID) > 128 {
		return failure("VALIDATION_FAILED", "需要明确确认原预览和独立请求标识。", false)
	}
	encoded, _ := json.Marshal(input)
	sum := sha256.Sum256(append([]byte("Recycle.Commit:"), encoded...))
	signature := hex.EncodeToString(sum[:])
	if task := s.recycleTask; task != nil && task.acceptancePending && task.operation.RecycleReport.RequestID == input.RequestID {
		if signature != task.signature {
			return failure("REQUEST_ID_REUSED", "不能改变原回收确认。", false)
		}
		return s.confirmRecycleAcceptance(task)
	}
	var oldSignature, text string
	err := s.db.QueryRow("SELECT signature,result_json FROM requests WHERE id=?", input.RequestID).Scan(&oldSignature, &text)
	if err == nil {
		if oldSignature != signature {
			return failure("REQUEST_ID_REUSED", "原请求标识不能用于另一回收操作。", false)
		}
		var result Result
		if json.Unmarshal([]byte(text), &result) != nil || !result.OK || result.OperationID == "" {
			return recycleProblem(errors.New("invalid receipt"))
		}
		_, op, _, err := s.readRecycleJournal(result.OperationID)
		if err != nil {
			return recycleProblem(err)
		}
		if s.recycleTask != nil && s.recycleTask.operation.ID == op.ID {
			op = copyRecycleOperation(s.recycleTask.operation)
		}
		return success(map[string]any{"status": "accepted", "operation": op}, op.ID)
	}
	if err != sql.ErrNoRows {
		return recycleProblem(err)
	}
	if !s.recycleIdle() {
		return failure("PROFILE_BUSY", "已有维护任务，未受理新的回收操作。", true)
	}
	d := s.recycleDraft
	if d == nil || d.preview.PreviewID != input.PreviewID || time.Now().After(d.expires) {
		return failure("PREVIEW_EXPIRED", "回收预览已失效，请重新核对影响。", true)
	}
	for _, item := range d.plan.Items {
		if err := s.recycleTargetFree(item.Entry.Item.EnvironmentID); err != nil {
			return recycleProblem(err)
		}
		got, err := recycleBaseline(s.db, item.Entry.Item.EnvironmentID)
		if err != nil || got != item.Entry.Baseline {
			return failure("REVISION_CONFLICT", "预览后配置修订已变，请重新核对。", true)
		}
	}
	op := Operation{ID: d.plan.ID, Kind: "recycle", State: "accepted", Stage: "accepted", Total: len(d.plan.Items), CompletedIDs: []string{}, PersistencePending: true, RecycleReport: &RecycleReport{Mode: "native", Action: d.plan.Action, RequestID: input.RequestID, PreviewID: input.PreviewID, Sequence: 1, NotExecuted: len(d.plan.Items), Protected: true}}
	task := &recycleTask{plan: d.plan, operation: op, phase: "accepted", signature: signature}
	result := success(map[string]any{"status": "accepted", "operation": op}, op.ID)
	p, _ := json.Marshal(task.plan)
	o, _ := json.Marshal(op)
	receipt, _ := json.Marshal(result)
	task.acceptedJSON = string(o)
	tx, err := s.db.Begin()
	if err != nil {
		return storageFailure(err)
	}
	defer tx.Rollback()
	if _, err = tx.Exec("INSERT INTO operations(id,result_json) VALUES(?,?)", op.ID, string(o)); err == nil {
		_, err = tx.Exec("INSERT INTO recycle_jobs(operation_id,plan_json,plan_sha256,phase,request_id,signature) VALUES(?,?,?,?,?,?)", op.ID, string(p), recyclePlanHash(p), "accepted", input.RequestID, signature)
	}
	if err == nil {
		_, err = tx.Exec("INSERT INTO requests(id,signature,result_json) VALUES(?,?,?)", input.RequestID, signature, string(receipt))
	}
	if err == nil && s.options.BeforeCommit != nil {
		err = s.options.BeforeCommit()
	}
	if err != nil {
		return storageFailure(err)
	}
	err = tx.Commit()
	s.recycleTask = task
	s.recycleDraft = nil
	s.drafts = map[string]draft{}
	s.restorePreview = nil
	if err != nil {
		task.acceptancePending = true
		return s.confirmRecycleAcceptance(task)
	}
	s.startRecycle(task, false)
	return result
}
func (s *Service) confirmRecycleAcceptance(task *recycleTask) Result {
	var p, o, signature, requestSignature, receipt string
	err := s.db.QueryRow("SELECT j.plan_json,o.result_json,j.signature,r.signature,r.result_json FROM recycle_jobs j JOIN operations o ON o.id=j.operation_id JOIN requests r ON r.id=j.request_id WHERE j.operation_id=? AND j.request_id=?", task.plan.ID, task.operation.RecycleReport.RequestID).Scan(&p, &o, &signature, &requestSignature, &receipt)
	if err == sql.ErrNoRows {
		var count int
		if err = s.db.QueryRow("SELECT (SELECT COUNT(*) FROM recycle_jobs WHERE operation_id=? OR request_id=?)+(SELECT COUNT(*) FROM operations WHERE id=?)+(SELECT COUNT(*) FROM requests WHERE id=?)", task.plan.ID, task.operation.RecycleReport.RequestID, task.plan.ID, task.operation.RecycleReport.RequestID).Scan(&count); err == nil && count == 0 {
			s.recycleTask = nil
			return failure("RECYCLE_NOT_ACCEPTED", "已核实原请求未保存、未移动或删除；请重新查看影响。", true)
		}
	}
	want, _ := json.Marshal(task.plan)
	var prior Result
	if err != nil || p != string(want) || o != task.acceptedJSON || signature != task.signature || requestSignature != signature || json.Unmarshal([]byte(receipt), &prior) != nil || !prior.OK || prior.OperationID != task.plan.ID {
		task.operation = copyRecycleOperation(task.operation)
		task.operation.Stage = "acceptance-pending"
		result := failure("RECYCLE_INCOMPLETE", "受理保存尚未核实，未执行目录动作；只核实原请求，不创建新的删除。", true)
		result.OperationID = task.plan.ID
		return result
	}
	json.Unmarshal([]byte(o), &task.operation)
	task.acceptancePending = false
	s.startRecycle(task, false)
	return success(map[string]any{"status": "accepted", "operation": copyRecycleOperation(task.operation)}, task.plan.ID)
}
