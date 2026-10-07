package workspace

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"

	"github.com/axgiroud312-byte/prism-local-browser/internal/backup"
)

func validateRecycleJournal(plan recyclePlan, op Operation, phase string, raw string) error {
	invalid := func() error { return errors.New("invalid recycle journal decision") }
	r := op.RecycleReport
	canonical, err := json.Marshal(plan)
	// This private journal is always encoded by this host. Requiring its complete
	// canonical shape rejects missing false decisions and truncated empty arrays.
	if err != nil || string(canonical) != raw || plan.Version != 1 || !backup.CanonicalID(plan.ID) || op.ID != plan.ID || op.Kind != "recycle" || r == nil || r.Mode != "native" || r.Action != plan.Action || r.RequestID == "" || !backup.CanonicalID(r.PreviewID) || r.Sequence < 1 || len(plan.Items) == 0 || op.Total != len(plan.Items) {
		return invalid()
	}
	if plan.Action != "remove" && plan.Action != "restore" && plan.Action != "purge" {
		return invalid()
	}
	known := false
	for _, p := range []string{"accepted", "prepared", "purging", "data-deleted", "item-committed", "workspace-recovery", "finished"} {
		if phase == p {
			known = true
		}
	}
	if !known {
		return invalid()
	}
	ids, trash := map[string]bool{}, map[string]bool{}
	completed, failed, notExecuted := 0, 0, 0
	for _, item := range plan.Items {
		e := item.Entry
		ref, err := dataReference(e.Item.EnvironmentID)
		if err != nil || ids[e.Item.EnvironmentID] || !backup.CanonicalID(e.TrashID) || trash[e.TrashID] || ref != e.Reference || !backup.Hash(e.Baseline) || e.Item.Revision < 1 || e.Item.Seed == "" || e.Item.Name == "" || e.Item.KernelID != PendingKernelID && !backup.CanonicalID(e.Item.KernelID) || !validRestoreInventory(e.Files) {
			return invalid()
		}
		ids[e.Item.EnvironmentID] = true
		if e.Lock != nil && (!e.Item.DataPresent || e.Lock.Identity == (backup.TreeIdentity{}) || e.Lock.Size < 0 || !backup.Hash(e.Lock.SHA256)) {
			return invalid()
		}
		trash[e.TrashID] = true
		if plan.Action == "remove" && e.Item.ID != e.Item.EnvironmentID || plan.Action != "remove" && e.Item.ID != e.TrashID {
			return invalid()
		}
		if e.Item.DataPresent && e.Identity == (backup.TreeIdentity{}) || !e.Item.DataPresent && (e.Identity != (backup.TreeIdentity{}) || len(e.Files) != 0) {
			return invalid()
		}
		if item.Committed && (!item.Prepared || !backup.Hash(item.AfterBaseline)) || !item.Committed && item.AfterBaseline != "" || item.PurgeStarted && (!item.Prepared || plan.Action != "purge") || item.DataDeleted && !item.PurgeStarted || plan.Action == "purge" && item.Committed && !item.DataDeleted {
			return invalid()
		}
		if item.Result != "pending" && item.Result != "failed" && item.Result != "protected" && item.Result != map[string]string{"remove": "recycled", "restore": "restored", "purge": "purged"}[plan.Action] {
			return invalid()
		}
		if item.Committed {
			completed++
		} else if item.Result == "failed" || item.Result == "protected" {
			failed++
		} else {
			notExecuted++
		}
	}
	if r.Completed != completed || r.Failed != failed || r.NotExecuted != notExecuted {
		return invalid()
	}
	return nil
}

func recycleReceipt(plan recyclePlan, op Operation, requestID, signature, requestSignature, receipt string) error {
	var result Result
	input := RecycleRequest{PreviewID: op.RecycleReport.PreviewID, Confirm: true, RequestID: requestID}
	encoded, _ := json.Marshal(input)
	sum := sha256.Sum256(append([]byte("Recycle.Commit:"), encoded...))
	if requestID != op.RecycleReport.RequestID || signature != hex.EncodeToString(sum[:]) || signature != requestSignature || json.Unmarshal([]byte(receipt), &result) != nil || !result.OK || result.Mode != "native" || result.OperationID != plan.ID {
		return errors.New("recycle receipt differs")
	}
	return nil
}

func (s *Service) loadInterruptedRecycle() error {
	rows, err := s.db.Query("SELECT operation_id FROM recycle_jobs WHERE phase<>'finished'")
	if err != nil {
		return err
	}
	ids := []string{}
	for rows.Next() {
		var id string
		if err = rows.Scan(&id); err != nil {
			rows.Close()
			return err
		}
		ids = append(ids, id)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	if len(ids) > 1 || len(ids) > 0 && s.restoreTask != nil {
		return errors.New("multiple directory maintenance journals")
	}
	if len(ids) == 0 {
		return nil
	}
	plan, op, phase, err := s.readRecycleJournal(ids[0])
	if err != nil {
		return err
	}
	s.recycleTask = &recycleTask{plan: plan, operation: op, phase: phase, startup: true}
	return nil
}

func (s *Service) recoverRecycle(operationID string) Result {
	task := s.recycleTask
	if task == nil {
		_, op, _, err := s.readRecycleJournal(operationID)
		if err != nil {
			return recycleProblem(err)
		}
		return success(op, op.ID)
	}
	if task.operation.ID != operationID {
		return failure("PROFILE_BUSY", "请核对当前原回收任务。", true)
	}
	if task.acceptancePending {
		result := s.confirmRecycleAcceptance(task)
		if !result.OK {
			return result
		}
		return success(copyRecycleOperation(task.operation), operationID)
	}
	if task.running || task.finalPending != nil {
		return success(copyRecycleOperation(task.operation), operationID)
	}
	if task.bootstrapOutcome != nil {
		s.scheduleRecycleBootstrap(task)
	} else {
		s.startRecycle(task, true)
	}
	return success(copyRecycleOperation(task.operation), operationID)
}

func (s *Service) cancelRecycle(task *recycleTask) Result {
	if task.acceptancePending {
		return failure("RECYCLE_INCOMPLETE", "先核实原请求是否受理。", true)
	}
	if !task.running || task.bootstrapOutcome != nil {
		return success(copyRecycleOperation(task.operation), task.operation.ID)
	}
	// Stop forward execution even if the cancellation COMMIT becomes unknown.
	// Every later worker observation inherits this monotonic in-memory intent.
	task.operation = copyRecycleOperation(task.operation)
	task.operation.CancelRequested = true
	if task.cancel != nil {
		task.cancel()
	}
	plan, op, phase, err := s.readRecycleJournal(task.plan.ID)
	if err != nil {
		return recycleProblem(err)
	}
	op.CancelRequested = true
	if err := s.persistRecycle(plan, op, phase); err != nil {
		return recycleProblem(err)
	}
	task.operation.CancelRequested = true
	return success(op, op.ID)
}

func (s *Service) flushRecyclePersistence() {
	task := s.recycleTask
	if task == nil || task.running || task.acceptancePending {
		return
	}
	if task.bootstrapReady && task.bootstrapOutcome != nil {
		s.finishRecycleBootstrap(task)
		return
	}
	if task.finalPending == nil {
		return
	}
	op := copyRecycleOperation(*task.finalPending)
	if err := s.persistRecycle(task.plan, op, "finished"); err != nil {
		return
	}
	task.operation = op
	task.phase = "finished"
	task.finalPending = nil
	s.releaseRecycle(task)
}

func (s *Service) scheduleRecycleBootstrap(task *recycleTask) {
	if task.bootstrapOutcome == nil {
		op := copyRecycleOperation(task.operation)
		task.bootstrapOutcome = &op
	}
	if s.closed || s.closeRequested.Load() {
		return
	}
	op := copyRecycleOperation(task.operation)
	op.State = "running"
	op.Stage = "workspace-recovery"
	op.PersistencePending = true
	op.RecycleReport.Protected = true
	op.RecycleReport.Sequence++
	task.operation = op
	task.phase = "workspace-recovery"
	task.running = true
	ctx, cancel := context.WithCancel(context.Background())
	task.cancel = cancel
	s.workers.Add(1)
	go func() { defer s.workers.Done(); defer cancel(); s.resumeWorkspaceAfterRecycle(ctx, task) }()
}

func (s *Service) resumeWorkspaceAfterRecycle(ctx context.Context, task *recycleTask) {
	s.mu.Lock()
	if s.recycleTask != task || s.closed || s.closeRequested.Load() {
		task.running = false
		s.mu.Unlock()
		return
	}
	if err := s.persistRecycle(task.plan, task.operation, "workspace-recovery"); err != nil {
		task.running = false
		s.recycleBootstrapFailure(task)
		s.mu.Unlock()
		return
	}
	if task.bootstrapLoader == nil {
		task.bootstrapLoader = &Service{db: s.db, root: s.root, options: s.options, runtimeSlots: map[string]*runtimeSlot{}, runtimePending: map[string]*runtimePendingWrite{}, runtimeResults: map[string]Operation{}, profileUses: map[string]bool{}}
		task.bootstrapLoader.inheritNetworkResources(s)
	}
	loader, step := task.bootstrapLoader, task.bootstrapStep
	s.mu.Unlock()
	steps := []func() error{func() error { return loader.recoverKernelOperationsContext(ctx) }, func() error { return loader.recoverRuntimeSessionsContext(ctx) }, loader.recoverProxyChecks, loader.recoverCookieImports, loader.recoverBatchTasks}
	var err error
	for step < len(steps) {
		if err = ctx.Err(); err != nil {
			break
		}
		if err = steps[step](); err != nil {
			break
		}
		step++
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	task.running = false
	task.cancel = nil
	task.bootstrapStep = step
	if s.recycleTask != task || s.closed || s.closeRequested.Load() {
		return
	}
	if err != nil {
		s.recycleBootstrapFailure(task)
		return
	}
	if !task.bootstrapReady {
		if err := s.recoverBackupExportsLocked(); err != nil {
			s.recycleBootstrapFailure(task)
			return
		}
		s.runtimeSlots, s.runtimePending, s.runtimeResults, s.profileUses = loader.runtimeSlots, loader.runtimePending, loader.runtimeResults, loader.profileUses
		for id := range s.networkPending {
			s.profileUses[id] = true
		}
		task.bootstrapReady = true
	}
	s.finishRecycleBootstrap(task)
}

func (s *Service) recycleBootstrapFailure(task *recycleTask) {
	op := copyRecycleOperation(task.operation)
	op.State = "failed"
	op.Stage = "workspace-recovery"
	op.Error = &Error{Code: "RECYCLE_INCOMPLETE", Message: "回收目录已核对，其余启动记录尚未就绪；维护保护保持，请修复存储或占用后重试原任务。", Retryable: true}
	task.operation = op
}

func (s *Service) finishRecycleBootstrap(task *recycleTask) {
	op := copyRecycleOperation(*task.bootstrapOutcome)
	op.RecycleReport.Sequence = task.operation.RecycleReport.Sequence + 1
	if err := s.persistRecycle(task.plan, op, "finished"); err != nil {
		s.recycleBootstrapFailure(task)
		return
	}
	task.operation = op
	task.phase = "finished"
	task.startup = false
	task.bootstrapOutcome = nil
	s.releaseRecycle(task)
}
