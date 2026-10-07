package workspace

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/axgiroud312-byte/prism-local-browser/internal/backup"
)

func validRestoreInventory(files []backup.File) bool {
	names := map[string]bool{}
	for _, file := range files {
		name := strings.ToLower(file.Path)
		if !backup.ValidName(file.Path) || strings.HasSuffix(file.Path, "/") || name == ".prism-runtime.lock" || names[name] || file.Size < 0 || file.Directory && (file.Size != 0 || file.SHA256 != "") || !file.Directory && !backup.Hash(file.SHA256) {
			return false
		}
		names[name] = true
	}
	return true
}

func validateRestoreJournal(task *restoreTask, committed bool) error {
	p, op := task.plan, task.operation
	r := op.RestoreReport
	invalid := func() error {
		return &Error{Code: "RESTORE_INCOMPLETE", Message: "恢复日志的版本、身份或目录清单无法核实；原数据和日志保留，未执行目录移动。请保留完整工作区并修复原日志后重开。", Retryable: true}
	}
	if (p.JournalVersion != 2 && p.JournalVersion != 3) || !backup.CanonicalID(p.ID) || op.ID != p.ID || op.Kind != "backup-restore" || r == nil || r.Mode != "native" || r.RequestID == "" || !backup.CanonicalID(r.PreviewID) || !backup.Hash(p.Baseline) || !backup.Hash(p.ArchiveSHA256) || p.ArchiveSHA256 != r.ArchiveSHA256 || op.Total != len(p.Environments) || r.EnvironmentCount != len(p.Environments) || r.SwitchedCount < 0 || r.SwitchedCount > len(p.Environments) || r.Sequence < 1 {
		return invalid()
	}
	if p.Prepared && (!backup.Hash(p.PreviousBaseline) || !backup.Hash(p.PreviousConfigurationSHA256) || len(p.Moves) != len(p.Environments)) || committed && (!p.Prepared || !backup.Hash(p.CommittedBaseline)) {
		return invalid()
	}
	// A false/default prepared bit is not proof that no rename occurred. Later
	// phases or any switch material require the complete prepared decision.
	if !p.Prepared && (len(p.Moves) != 0 || p.PreviousConfigurationSHA256 != "" || p.CommittedBaseline != "" || task.phase == "prepared" || task.phase == "swapping" || task.phase == "db-committing" || task.phase == "db-committed") {
		return invalid()
	}
	knownPhase := false
	for _, phase := range []string{"accepted", "revalidating", "stopping-environments", "prepared", "swapping", "db-committing", "db-committed", "protected", "recovering", "workspace-recovery"} {
		if task.phase == phase {
			knownPhase = true
		}
	}
	if !knownPhase {
		return invalid()
	}
	ids, profiles := map[string]bool{}, map[string]bool{}
	for _, e := range p.Environments {
		m := e.Manifest
		ref, err := dataReference(m.ID)
		if err != nil || ids[m.ID] || profiles[m.FingerprintID] || !backup.CanonicalID(m.FingerprintID) || ref != m.DataReference || e.Environment.ID != m.ID || e.Environment.Seed != m.Seed || e.Environment.CoreID != m.KernelID || e.Environment.ProxyID != m.ProxyID || validate(e.Environment.Configuration) != "" || e.ExistingRevision < 0 || m.Revision < 1 || e.Code < 1 || len(e.History) == 0 || int64(len(e.History)) != m.ProfileRevision {
			return invalid()
		}
		ids[m.ID] = true
		profiles[m.FingerprintID] = true
		for index, h := range e.History {
			local := p.KernelMapping[h.Profile.KernelID]
			if local != PendingKernelID && !backup.CanonicalID(local) || h.Profile.ConfigRevision != int64(index+1) || checkProfile(h.Profile) != nil {
				return invalid()
			}
		}
		last := e.History[len(e.History)-1].Profile
		if last.ConfigHash != m.ProfileHash || !profileMatchesConfiguration(last, e.Environment.Configuration) {
			return invalid()
		}
	}
	base := "backups/restore/" + p.ID
	moves := map[string]bool{}
	for _, m := range p.Moves {
		ref, err := dataReference(m.ID)
		zero := backup.TreeIdentity{}
		if err != nil || !ids[m.ID] || moves[m.ID] || m.Live != ref || m.Incoming != base+"/incoming/"+ref || m.Previous != base+"/previous/"+m.ID+"/user-data" || m.NewIdentity == zero || m.OldPresent && m.OldIdentity == zero || !m.OldPresent && (m.OldIdentity != zero || len(m.OldFiles) != 0) || !validRestoreInventory(m.OldFiles) || !validRestoreInventory(m.NewFiles) {
			return invalid()
		}
		moves[m.ID] = true
	}
	return nil
}

// No archived source path, process launch, or fresh restore is used on restart.
// The durable DB decision and recognized directory objects alone select a side.
func (s *Service) startInterruptedRestore(task *restoreTask) {
	task.startup = true
	task.recoveryAvailable = true
	task.running = true
	task.operation = copyRestoreOperation(task.operation)
	task.operation.RestoreReport.RecoveredAfterRestart = true
	if task.operation.RestoreReport.InterruptedStage == "" {
		task.operation.RestoreReport.InterruptedStage = task.phase
	}
	task.operation.RestoreReport.Sequence++
	task.operation.RestoreReport.Protected = true
	task.operation.State = "running"
	task.operation.Stage = "recovering"
	task.operation.PersistencePending = true
	plan := task.plan
	var cause error = &Error{Code: "APPLICATION_INTERRUPTED", Message: "应用退出中断了恢复，按持久记录核对完整旧或新状态。", Retryable: true}
	if task.operation.CancelRequested {
		cause = context.Canceled
	}
	s.workers.Add(1)
	go func() { defer s.workers.Done(); s.finishRestoreExecution(task, plan, cause) }()
}

// Called with s.mu held, after the directory outcome is durably known. Keep the
// barrier until the remaining startup loaders have finished; their old worker
// records must never run against a half-switched workspace.
func (s *Service) scheduleRestoreBootstrap(task *restoreTask) {
	if task.bootstrapOutcome == nil {
		op := copyRestoreOperation(task.operation)
		task.bootstrapOutcome = &op
	}
	if s.closed || s.closeRequested.Load() {
		return
	}
	op := copyRestoreOperation(task.operation)
	op.State = "running"
	op.Stage = "workspace-recovery"
	op.PersistencePending = true
	op.RestoreReport.Protected = true
	op.RestoreReport.Sequence++
	task.operation = op
	task.phase = "workspace-recovery"
	task.running = true
	ctx, cancel := context.WithCancel(context.Background())
	task.bootstrapCancel = cancel
	s.workers.Add(1)
	go func() { defer s.workers.Done(); defer cancel(); s.resumeWorkspaceAfterRestore(ctx, task) }()
}

func (s *Service) resumeWorkspaceAfterRestore(ctx context.Context, task *restoreTask) {
	s.mu.Lock()
	if s.restoreTask != task || s.closed || s.closeRequested.Load() {
		task.running = false
		s.mu.Unlock()
		return
	}
	if err := s.persistRestore(task, task.operation, "workspace-recovery"); err != nil {
		task.running = false
		s.restoreBootstrapFailure(task)
		s.mu.Unlock()
		return
	}
	if task.bootstrapLoader == nil {
		task.bootstrapLoader = &Service{db: s.db, root: s.root, options: s.options,
			runtimeSlots: map[string]*runtimeSlot{}, runtimePending: map[string]*runtimePendingWrite{}, runtimeResults: map[string]Operation{}, profileUses: map[string]bool{}}
		task.bootstrapLoader.inheritNetworkResources(s)
	}
	loader, step := task.bootstrapLoader, task.bootstrapStep
	s.mu.Unlock()
	// Startup has an independent map owner; slow filesystem work neither holds
	// the RPC mutex nor exposes half-loaded slots to queries/Close. No worker is
	// started on this loader. The main service owns all cancellation and lifetime.
	steps := []func() error{func() error { return loader.recoverKernelOperationsContext(ctx) }, func() error { return loader.recoverRuntimeSessionsContext(ctx) }, loader.recoverProxyChecks, loader.recoverCookieImports, loader.recoverBatchTasks}
	var loadErr error
	for step < len(steps) {
		if loadErr = ctx.Err(); loadErr != nil {
			break
		}
		if loadErr = steps[step](); loadErr != nil {
			break
		}
		step++
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	task.running = false
	task.bootstrapCancel = nil
	task.bootstrapStep = step
	if s.restoreTask != task || s.closed || s.closeRequested.Load() {
		return
	}
	if loadErr != nil {
		s.restoreBootstrapFailure(task)
		return
	}
	if !task.bootstrapReady {
		// Only synchronous DB setup is under this mutex; backup file verification
		// is already a tracked asynchronous worker and cannot replay directory moves.
		if err := s.recoverBackupExportsLocked(); err != nil {
			s.restoreBootstrapFailure(task)
			return
		}
		s.runtimeSlots, s.runtimePending, s.runtimeResults, s.profileUses = loader.runtimeSlots, loader.runtimePending, loader.runtimeResults, loader.profileUses
		for id := range s.networkPending {
			s.profileUses[id] = true
		}
		task.bootstrapReady = true
	}
	s.finishRestoreBootstrap(task)
}

func (s *Service) restoreBootstrapFailure(task *restoreTask) {
	op := copyRestoreOperation(task.operation)
	op.State = "failed"
	op.Stage = "workspace-recovery"
	op.Error = &Error{Code: "RESTORE_INCOMPLETE", Message: "恢复目录与配置已核对，但其余启动记录尚未就绪；维护保护保持。请核对存储权限、空间或会话占用后重试原任务。", Retryable: true}
	task.operation = op
}

func (s *Service) finishRestoreBootstrap(task *restoreTask) {
	op := copyRestoreOperation(*task.bootstrapOutcome)
	op.RestoreReport.Sequence = task.operation.RestoreReport.Sequence + 1
	op.PersistencePending = false
	op.RestoreReport.Protected = false
	if err := s.persistRestore(task, op, op.Stage); err != nil {
		s.restoreBootstrapFailure(task)
		return
	}
	task.operation = op
	task.phase = op.Stage
	task.startup = false
	task.bootstrapOutcome = nil
	s.releaseRestore(task, op.RestoreReport.Committed)
}

func (s *Service) restoreRecoveryRecord(opJSON, planJSON, phase, requestID, signature, requestSignature, requestJSON string, committed bool) (*restoreTask, error) {
	task := &restoreTask{phase: phase}
	if backup.DecodeJSON([]byte(opJSON), &task.operation) != nil || backup.DecodeJSON([]byte(planJSON), &task.plan) != nil {
		return nil, &Error{Code: "RESTORE_INCOMPLETE", Message: "恢复日志无法读取；未移动原目录。保留完整数据根及日志，修复存储后重开。", Retryable: true}
	}
	var required struct {
		Prepared *bool `json:"prepared"`
	}
	if json.Unmarshal([]byte(planJSON), &required) != nil || required.Prepared == nil {
		return nil, &Error{Code: "RESTORE_INCOMPLETE", Message: "恢复日志缺少目录切换决策，无法证明原目录未改变；请保留完整工作区和日志后修复。", Retryable: true}
	}
	if err := validateRestoreJournal(task, committed); err != nil {
		return nil, err
	}
	var request Result
	if json.Unmarshal([]byte(requestJSON), &request) != nil || !request.OK || request.Mode != "native" || request.OperationID != task.plan.ID || signature != requestSignature || !backup.Hash(signature) || requestID != task.operation.RestoreReport.RequestID {
		return nil, errors.New("restore receipt does not match journal")
	}
	if committed {
		task.operation.RestoreReport.Committed = true
		task.operation.RestoreReport.RolledBack = false
	}
	return task, nil
}

func restoreRecoveryCountError(count int) error {
	return &Error{Code: "RESTORE_INCOMPLETE", Message: fmt.Sprintf("检测到 %d 份未完成恢复记录，无法确认唯一目录写入者；保留原日志与全部副本，未自动移动。", count), Retryable: true}
}
