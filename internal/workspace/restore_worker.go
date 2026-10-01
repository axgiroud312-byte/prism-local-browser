package workspace

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/axgiroud312-byte/prism-local-browser/internal/backup"
	"github.com/axgiroud312-byte/prism-local-browser/internal/kernel"
)

func (s *Service) restoreCheckpoint(phase string) error {
	if s.options.RestoreCheckpoint != nil {
		return s.options.RestoreCheckpoint(phase)
	}
	return nil
}
func (s *Service) observeRestore(task *restoreTask, plan restorePlan, phase string, switched int) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.restoreTask != task {
		return errors.New("restore owner changed")
	}
	op := copyRestoreOperation(task.operation)
	op.State = "running"
	op.Stage = phase
	op.RestoreReport.Sequence++
	op.RestoreReport.SwitchedCount = switched
	prior := task.plan
	task.plan = plan
	if err := s.persistRestore(task, op, phase); err != nil {
		task.plan = prior
		return err
	}
	task.operation = op
	task.phase = phase
	return nil
}
func (s *Service) stopRestoreTarget(ctx context.Context, task *restoreTask, environmentID string) error {
	s.mu.Lock()
	if s.restoreTask != task || s.closed || ctx.Err() != nil {
		s.mu.Unlock()
		return context.Canceled
	}
	if !s.runtimeOwnsProfileUse(environmentID) {
		s.mu.Unlock()
		return nil
	}
	result := s.stopRuntime(runtimeRequest{EnvironmentID: environmentID, RequestID: id()})
	s.mu.Unlock()
	if !result.OK {
		return result.Error
	}
	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
		s.mu.Lock()
		s.flushRuntimePersistence()
		var text string
		err := s.db.QueryRow("SELECT result_json FROM operations WHERE id=?", result.OperationID).Scan(&text)
		var op Operation
		if err == nil {
			err = json.Unmarshal([]byte(text), &op)
		}
		busy := s.runtimeOwnsProfileUse(environmentID)
		pending := s.runtimePending[environmentID] != nil
		s.mu.Unlock()
		if err != nil {
			return err
		}
		if pending {
			return errors.New("stop persistence unconfirmed")
		}
		if op.State == "failed" || op.State == "cancelled" {
			if op.Error != nil {
				return op.Error
			}
			return errors.New("normal stop failed")
		}
		if op.State == "completed" {
			if busy {
				return errors.New("process tree still owns profile")
			}
			return nil
		}
	}
}

func (s *Service) runRestore(ctx context.Context, task *restoreTask) {
	defer s.workers.Done()
	s.mu.Lock()
	encoded, _ := json.Marshal(task.plan)
	var plan restorePlan
	decode(encoded, &plan)
	s.mu.Unlock()
	err := s.executeRestore(ctx, task, &plan)
	s.finishRestoreExecution(task, plan, err)
}
func (s *Service) executeRestore(ctx context.Context, task *restoreTask, plan *restorePlan) error {
	if err := s.observeRestore(task, *plan, "revalidating", 0); err != nil {
		return err
	}
	file, release, err := backup.FreezeFile(plan.Source)
	if err != nil {
		return err
	}
	defer release()
	defer file.Close()
	pkg, err := backup.Read(ctx, file)
	if err != nil {
		return err
	}
	if pkg.ArchiveSHA256 != plan.ArchiveSHA256 {
		return restoreInvalid("package-changed-after-preview")
	}
	if err = s.observeRestore(task, *plan, "stopping-environments", 0); err != nil {
		return err
	}
	for _, item := range plan.Environments {
		if err = s.stopRestoreTarget(ctx, task, item.Manifest.ID); err != nil {
			return err
		}
	}
	s.mu.Lock()
	baseline, err := restoreBaseline(s.db)
	s.mu.Unlock()
	if err != nil {
		return err
	}
	if baseline != plan.Baseline {
		return &Error{Code: "REVISION_CONFLICT", Message: "预检后配置或数据初始化事实已改变，原目录未替换；请重新预检。", Retryable: true}
	}
	kernelReleases := []func(){}
	defer func() {
		for i := len(kernelReleases) - 1; i >= 0; i-- {
			kernelReleases[i]()
		}
	}()
	checked := map[string]bool{}
	for _, e := range plan.Environments {
		for _, history := range e.History {
			localID := plan.KernelMapping[history.Profile.KernelID]
			if localID == PendingKernelID {
				continue
			}
			if localID == "" {
				return errors.New("required exact kernel missing")
			}
			if checked[localID] {
				continue
			}
			s.mu.Lock()
			record, err := savedKernelFrom(s.db, localID, true)
			s.mu.Unlock()
			if err != nil {
				return err
			}
			directory, err := kernel.RecordDirectory(s.root, record)
			if err != nil {
				return err
			}
			pin, err := kernel.PinFiles(directory, record.Files)
			if err != nil {
				return err
			}
			kernelReleases = append(kernelReleases, pin)
			if err = kernel.VerifyFiles(directory, record.Files); err != nil {
				return err
			}
			version, err := kernel.FileVersion(filepath.Join(directory, filepath.FromSlash(record.ExecutableRelativePath)))
			if err != nil || version != record.Version {
				return errors.New("kernel version changed")
			}
			checked[localID] = true
		}
	}
	parent, err := kernel.EnsureDirectory(s.root, "backups/restore")
	if err != nil {
		return err
	}
	defer parent()
	base := "backups/restore/" + plan.ID
	directory := filepath.Join(s.root, filepath.FromSlash(base))
	if err = os.Mkdir(directory, 0700); err != nil {
		return err
	}
	stagePin, err := backup.PinStagingDirectory(directory)
	if err != nil {
		return err
	}
	defer stagePin()
	if err = pkg.ExtractProfiles(ctx, s.root, base+"/incoming"); err != nil {
		return err
	}
	oldTargets := []backupTarget{}
	for _, item := range plan.Environments {
		m := restoreMove{ID: item.Manifest.ID, Live: item.Manifest.DataReference, Incoming: base + "/incoming/" + item.Manifest.DataReference, Previous: base + "/previous/" + item.Manifest.ID + "/user-data", OldFiles: []backup.File{}, NewFiles: []backup.File{}}
		for _, entry := range pkg.Manifest.Files {
			if strings.HasPrefix(entry.Path, m.Live+"/") {
				relative := strings.TrimSuffix(strings.TrimPrefix(entry.Path, m.Live+"/"), "/")
				if relative != "" {
					entry.Path = relative
					m.NewFiles = append(m.NewFiles, entry)
				}
			}
		}
		// Each incoming profile has a fresh local lock, never the archived PID or
		// session. Runtime-claimed facts remain monotonic without importing control.
		m.NewIdentity, err = backup.PrepareRestoreLock(s.root, m.Incoming)
		if err != nil {
			return err
		}
		var target backupTarget
		if item.ExistingRevision > 0 {
			s.mu.Lock()
			target, err = s.backupDataTarget(item.Manifest.ID, item.Manifest.DataReference)
			s.mu.Unlock()
			if err != nil {
				return err
			}
			oldTargets = append(oldTargets, target)
		} else {
			target = backupTarget{ID: item.Manifest.ID, Reference: item.Manifest.DataReference, NeverUsed: true}
		}
		captured, err := backup.CaptureProfile(ctx, s.root, target.ID, target.Reference, !target.DirectoryRequired, target.NeverUsed)
		if err != nil {
			return err
		}
		if item.ExistingRevision == 0 && !captured.Missing {
			captured.Close()
			return errors.New("new identity has an unowned existing directory")
		}
		m.OldPresent = !captured.Missing
		if m.OldPresent {
			m.OldFiles, err = captured.Inventory(ctx)
			if err == nil {
				m.OldIdentity, _, err = backup.IdentifyTree(s.root, m.Live)
			}
		}
		closeErr := captured.Close()
		if err != nil {
			return err
		}
		if closeErr != nil {
			return closeErr
		}
		for _, relative := range []string{filepath.ToSlash(filepath.Dir(m.Live)), filepath.ToSlash(filepath.Dir(m.Previous))} {
			pin, err := kernel.EnsureDirectory(s.root, relative)
			if err != nil {
				return err
			}
			pin()
		}
		if err = backup.VerifyTree(ctx, s.root, m.Incoming, m.NewIdentity, m.NewFiles); err != nil {
			return err
		}
		plan.Moves = append(plan.Moves, m)
	}
	// Preserve a consistent old configuration snapshot beside the rollback trees.
	if _, err = s.createBackupSnapshot(ctx, backupExecution{OperationID: plan.ID, Scope: "selected", CreatedAt: timestamp(), Targets: oldTargets}, filepath.Join(directory, "previous-configuration.sqlite")); err != nil {
		return err
	}
	plan.Prepared = true
	if err = s.observeRestore(task, *plan, "prepared", 0); err != nil {
		return err
	}
	if err = s.restoreCheckpoint("prepared"); err != nil {
		return err
	}
	for index, m := range plan.Moves {
		if err = ctx.Err(); err != nil {
			return err
		}
		if err = s.observeRestore(task, *plan, "swapping", index); err != nil {
			return err
		}
		if m.OldPresent {
			if err = backup.VerifyTree(ctx, s.root, m.Live, m.OldIdentity, m.OldFiles); err != nil {
				return err
			}
			if err = backup.MoveTree(ctx, s.root, m.Live, m.Previous, m.OldIdentity, m.OldFiles); err != nil {
				return err
			}
		}
		if err = s.restoreCheckpoint("old-retained"); err != nil {
			return err
		}
		if err = backup.MoveTree(ctx, s.root, m.Incoming, m.Live, m.NewIdentity, m.NewFiles); err != nil {
			return err
		}
		if err = s.restoreCheckpoint("new-switched"); err != nil {
			return err
		}
		if err = s.observeRestore(task, *plan, "swapping", index+1); err != nil {
			return err
		}
	}
	for _, m := range plan.Moves {
		if err = backup.VerifyTree(ctx, s.root, m.Live, m.NewIdentity, m.NewFiles); err != nil {
			return err
		}
	}
	if err = ctx.Err(); err != nil {
		return err
	}
	if err = s.observeRestore(task, *plan, "db-committing", len(plan.Moves)); err != nil {
		return err
	}
	if err = s.restoreCheckpoint("before-db-commit"); err != nil {
		return err
	}
	s.mu.Lock()
	err = s.commitRestoredConfiguration(task, *plan)
	s.mu.Unlock()
	if err != nil {
		return err
	}
	return s.restoreCheckpoint("db-committed")
}

func rollbackRestoreDirectories(ctx context.Context, root string, plan restorePlan) error {
	if !plan.Prepared {
		return nil
	}
	for index := len(plan.Moves) - 1; index >= 0; index-- {
		m := plan.Moves[index]
		live, present, err := backup.IdentifyTree(root, m.Live)
		if err != nil {
			return err
		}
		incoming, incomingExists, err := backup.IdentifyTree(root, m.Incoming)
		if err != nil {
			return err
		}
		old, oldExists, err := backup.IdentifyTree(root, m.Previous)
		if err != nil {
			return err
		}
		if incomingExists && incoming != m.NewIdentity || oldExists && (!m.OldPresent || old != m.OldIdentity) {
			return errors.New("rollback directory identity conflict")
		}
		if present && live == m.NewIdentity {
			if incomingExists {
				return errors.New("duplicate new tree")
			}
			if err = backup.VerifyTree(ctx, root, m.Live, m.NewIdentity, m.NewFiles); err != nil {
				return err
			}
			if err = backup.MoveTree(ctx, root, m.Live, m.Incoming, m.NewIdentity, m.NewFiles); err != nil {
				return err
			}
			present = false
		} else if present && (!m.OldPresent || live != m.OldIdentity) {
			return errors.New("unknown live directory protected")
		}
		if m.OldPresent {
			if present && oldExists {
				return errors.New("duplicate old tree")
			}
			if !present {
				if !oldExists {
					return errors.New("rollback copy missing")
				}
				if err = backup.VerifyTree(ctx, root, m.Previous, m.OldIdentity, m.OldFiles); err != nil {
					return err
				}
				if err = backup.MoveTree(ctx, root, m.Previous, m.Live, m.OldIdentity, m.OldFiles); err != nil {
					return err
				}
			}
			if err = backup.VerifyTree(ctx, root, m.Live, m.OldIdentity, m.OldFiles); err != nil {
				return err
			}
		} else if present {
			return errors.New("new identity rollback not absent")
		}
	}
	return nil
}

func (s *Service) finishRestoreExecution(task *restoreTask, plan restorePlan, cause error) {
	s.mu.Lock()
	committed, readErr := s.restoreCommitted(plan.ID)
	s.mu.Unlock()
	protected := readErr != nil
	rolledBack := false
	if !protected && !committed {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
		rollbackErr := rollbackRestoreDirectories(ctx, s.root, plan)
		cancel()
		if rollbackErr != nil {
			protected = true
			cause = errors.Join(cause, rollbackErr)
		} else {
			rolledBack = true
		}
	}
	if committed {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
		for _, m := range plan.Moves {
			if err := backup.VerifyTree(ctx, s.root, m.Live, m.NewIdentity, m.NewFiles); err != nil {
				protected = true
				cause = errors.Join(cause, err)
				break
			}
		}
		cancel()
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.restoreTask != task {
		return
	}
	task.running = false
	task.cancel = nil
	task.recoveryAvailable = true
	op := copyRestoreOperation(task.operation)
	op.RestoreReport.Sequence++
	op.RestoreReport.Committed = committed
	op.RestoreReport.RolledBack = rolledBack
	op.RestoreReport.Protected = protected
	op.PersistencePending = protected
	phase := "finalized"
	op.State = "completed"
	op.Stage = phase
	op.Error = nil
	if !committed {
		phase = "rolled-back"
		op.State = "failed"
		op.Stage = phase
		op.Error = &Error{Code: "RESTORE_INCOMPLETE", Message: "恢复未完成，已回滚并核对原目录/配置；未重新生成身份。修正原因后重新预检。", Retryable: true}
		if errors.Is(cause, context.Canceled) {
			op.State = "cancelled"
		}
		if cause != nil {
			var safe *Error
			if errors.As(cause, &safe) {
				op.Error.Details = map[string]any{"cause": safe.Code}
			}
		}
	}
	if protected {
		phase = "protected"
		op.State = "failed"
		op.Stage = phase
		op.Error = &Error{Code: "RESTORE_INCOMPLETE", Message: "恢复状态尚未一致确认；原副本与日志已保留，环境保持维护保护。请核对占用、空间和权限后重试日志恢复。", Retryable: true}
	}
	task.plan = plan
	if err := s.persistRestore(task, op, phase); err != nil {
		if !protected {
			final := copyRestoreOperation(op)
			task.finalPending = &final
			task.finalPhase = phase
		}
		op.PersistencePending = true
		op.RestoreReport.Protected = true
		op.State = "failed"
		op.Stage = "storage-pending"
		op.Error = &Error{Code: "RESTORE_INCOMPLETE", Message: "目录与数据库结果待日志保存，未重新执行切换；保护保持。", Retryable: true}
		protected = true
	}
	task.operation = op
	task.phase = phase
	if !protected {
		s.releaseRestore(task, committed)
	}
}

func (s *Service) releaseRestore(task *restoreTask, committed bool) {
	for _, e := range task.plan.Environments {
		if committed {
			delete(s.runtimeSlots, e.Manifest.ID)
			delete(s.runtimePending, e.Manifest.ID)
		}
		if !s.runtimeOwnsProfileUse(e.Manifest.ID) {
			s.releaseProfileUse(e.Manifest.ID)
		}
	}
	s.restoreTask = nil
	s.drafts = map[string]draft{}
	s.restorePreview = nil
}
