package workspace

import (
	"context"
	"errors"
	"path/filepath"
	"time"

	"github.com/axgiroud312-byte/prism-local-browser/internal/backup"
	"github.com/axgiroud312-byte/prism-local-browser/internal/kernel"
)

func (s *Service) loadInterruptedMigration() error {
	rows, err := s.db.Query("SELECT operation_id FROM kernel_migrations WHERE phase<>'finished'")
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
	if len(ids) > 1 || len(ids) > 0 && (s.restoreTask != nil || s.recycleTask != nil) {
		return errors.New("multiple directory writers")
	}
	if len(ids) == 0 {
		return nil
	}
	p, op, phase, err := s.readMigrationJournal(ids[0])
	if err != nil {
		return err
	}
	s.migrationTask = &migrationTask{plan: p, operation: op, phase: phase, startup: true, stop: make(chan struct{}, 1)}
	return nil
}

func (s *Service) recoverMigration(ctx context.Context, task *migrationTask) {
	// Recovery never relaunches a trial and never turns an interrupted user trial
	// into approval. An uncommitted attempt returns to its original complete state.
	s.finishMigrationRecovery(ctx, task, context.Canceled)
}

func (s *Service) finishMigrationRecovery(_ context.Context, task *migrationTask, cause error) {
	// Cancellation stops forward work; compensation gets its own bounded context.
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	s.mu.Lock()
	plan, op, phase, err := s.readMigrationJournal(task.plan.ID)
	cancelled := task.operation.CancelRequested
	process := task.process
	s.mu.Unlock()
	if err != nil {
		s.protectMigration(task, err)
		return
	}
	if process != nil && !process.Snapshot().ResourcesExited {
		s.protectMigration(task, errors.New("trial job still owns data"))
		return
	}
	if plan.LaunchPermitted && !plan.TrialExited && !plan.Prepared {
		identity, present, e := backup.IdentifyTree(s.root, plan.Move.Incoming)
		if e != nil || !present || identity != plan.WorkIdentity {
			s.protectMigration(task, errors.New("migration work object changed"))
			return
		}
		exited, e := kernel.InspectMigrationWorkProfile(s.root, plan.WorkID, plan.SessionID, plan.Move.Incoming)
		if e != nil || !exited {
			s.protectMigration(task, errors.New("trial session exit not confirmed"))
			return
		}
		plan.TrialExited = true
		op.MigrationReport.TrialExited = true
	}
	s.mu.Lock()
	baseline, e := migrationBaseline(s.db, plan)
	s.mu.Unlock()
	want := plan.Baseline
	if plan.Committed {
		want = plan.CommittedBaseline
	}
	if e != nil || baseline != want {
		s.protectMigration(task, errors.New("migration configuration outcome differs"))
		return
	}
	if plan.BackupVerified {
		hash, e := backup.PublishedDigest(ctx, filepath.Join(s.root, filepath.FromSlash(migrationBackupRef(plan.ID))))
		if e != nil || hash != plan.BackupSHA256 {
			s.protectMigration(task, errors.New("retained migration backup differs"))
			return
		}
	}
	if plan.Prepared {
		if plan.Committed {
			release, e := s.pinMigrationKernels(ctx, plan)
			if e != nil {
				s.protectMigration(task, e)
				return
			}
			defer release()
			err = backup.VerifyTree(ctx, s.root, plan.Move.Live, plan.Move.NewIdentity, plan.Move.NewFiles)
			if err == nil {
				if _, exists, e := backup.IdentifyTree(s.root, plan.Move.Incoming); e != nil || exists {
					err = errors.New("duplicate trial directory")
				}
			}
			if err == nil && plan.Move.OldPresent {
				err = backup.VerifyTree(ctx, s.root, plan.Move.Previous, plan.Move.OldIdentity, plan.Move.OldFiles)
			}
		} else {
			err = s.rollbackRestoreDirectories(ctx, restorePlan{Prepared: true, Moves: []restoreMove{plan.Move}})
		}
		if err != nil {
			s.protectMigration(task, err)
			return
		}
	} else if plan.BackupVerified {
		if plan.Move.OldPresent {
			err = backup.VerifyTree(ctx, s.root, plan.Move.Live, plan.Move.OldIdentity, plan.Move.OldFiles)
		} else {
			if _, present, e := backup.IdentifyTree(s.root, plan.Move.Live); e != nil || present {
				err = errors.New("original empty state changed")
			}
		}
		if err != nil {
			s.protectMigration(task, err)
			return
		}
	}
	s.mu.Lock()
	if s.migrationTask != task {
		s.mu.Unlock()
		return
	}
	task.plan, task.phase, task.process = plan, phase, nil
	op.CancelRequested = op.CancelRequested || cancelled
	op.PersistencePending = false
	op.MigrationReport.Protected = false
	op.MigrationReport.Sequence = max(op.MigrationReport.Sequence, task.operation.MigrationReport.Sequence) + 1
	op.State, op.Stage, op.Error = "completed", "completed", nil
	if !plan.Committed {
		op.State = "failed"
		op.Stage = "original-retained"
		op.Error = &Error{Code: "MIGRATION_INCOMPLETE", Message: "迁移未提交，原完整数据与原构建已核对保留；试用副本及已有备份保留。", Retryable: true}
		if errors.Is(cause, context.Canceled) || cancelled {
			op.State = "cancelled"
		}
	} else {
		op.CompletedIDs = []string{plan.Environment.ID}
	}
	task.finalPending = &op
	startup := task.startup && !task.bootstrapReady
	s.mu.Unlock()
	if startup {
		if err = s.bootstrapMigration(ctx, task); err != nil {
			s.protectMigration(task, err)
			return
		}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	task.running = false
	task.cancel = nil
	s.flushMigrationPersistence()
}

// No terminal journal is written before all other startup records load. Use a
// separate container for runtime maps to avoid races with Workspace.Read.
func (s *Service) bootstrapMigration(ctx context.Context, task *migrationTask) error {
	s.mu.Lock()
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
		if s.closeRequested.Load() {
			err = context.Canceled
			break
		}
		if err = steps[step](); err != nil {
			break
		}
		step++
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	task.bootstrapStep = step
	if err != nil {
		return err
	}
	if s.closed || s.closeRequested.Load() {
		return context.Canceled
	}
	if err = s.recoverBackupExportsLocked(); err != nil {
		return err
	}
	s.runtimeSlots, s.runtimePending, s.runtimeResults, s.profileUses = loader.runtimeSlots, loader.runtimePending, loader.runtimeResults, loader.profileUses
	for id := range s.networkPending {
		s.profileUses[id] = true
	}
	task.bootstrapReady = true
	return nil
}

func (s *Service) flushMigrationPersistence() {
	task := s.migrationTask
	if task == nil || task.running || task.finalPending == nil || task.startup && !task.bootstrapReady {
		return
	}
	op := copyMigrationOperation(*task.finalPending)
	// A failed final COMMIT may already have succeeded; read before retrying so
	// no stale cached observation can overwrite a later durable decision.
	plan, stored, phase, err := s.readMigrationJournal(task.plan.ID)
	if err != nil {
		return
	}
	if phase == "finished" {
		task.plan, task.operation = plan, stored
		s.releaseMigration(task)
		return
	}
	if plan.Committed != task.plan.Committed {
		return
	}
	if err = s.persistMigration(task.plan, op, "finished"); err != nil {
		overlay := copyMigrationOperation(op)
		overlay.PersistencePending = true
		overlay.MigrationReport.Protected = true
		overlay.Stage = "storage-pending"
		task.operation = overlay
		return
	}
	task.operation = op
	task.phase = "finished"
	s.releaseMigration(task)
}
func (s *Service) releaseMigration(task *migrationTask) {
	if s.migrationTask != task {
		return
	}
	if task.plan.Committed {
		delete(s.runtimeSlots, task.plan.Environment.ID)
		delete(s.runtimePending, task.plan.Environment.ID)
		s.releaseProfileUse(task.plan.Environment.ID)
	}
	s.migrationTask = nil
	s.migrationDraft = nil
	s.restorePreview = nil
	s.recycleDraft = nil
}
