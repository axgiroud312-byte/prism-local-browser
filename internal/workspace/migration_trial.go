package workspace

import (
	"context"
	"errors"
	"time"

	"github.com/axgiroud312-byte/prism-local-browser/internal/backup"
	"github.com/axgiroud312-byte/prism-local-browser/internal/kernel"
)

func (s *Service) runMigrationTrial(ctx context.Context, task *migrationTask) {
	err := s.prepareMigrationBackup(ctx, task)
	if err == nil {
		err = s.prepareMigrationCopy(ctx, task)
	}
	var probe *kernel.MigrationProbe
	if err == nil {
		probe, err = kernel.NewMigrationProbe()
	}
	if probe != nil {
		defer probe.Close()
	}
	for _, old := range []bool{true, false} {
		if err != nil {
			break
		}
		profile := task.plan.Preview.After
		if old {
			profile = task.plan.Preview.Before
		}
		var record kernel.Record
		s.mu.Lock()
		record, err = savedKernelFrom(s.db, profile.KernelID, true)
		s.mu.Unlock()
		if err != nil {
			break
		}
		// A missing copy must be detected BEFORE the normal launcher can create a
		// directory. No raw staging root is substituted for the installation root.
		identity, present, e := backup.IdentifyTree(s.root, task.plan.Move.Incoming)
		if e != nil || !present || identity != task.plan.WorkIdentity {
			err = errors.New("migration work profile missing")
			break
		}
		err = s.migrationUpdate(task, "trial-starting", func(p *migrationPlan, r *MigrationReport) {
			p.SessionID = id()
			p.PID = 0
			p.ProcessCreatedAt = ""
			p.LaunchPermitted = true
			p.LaunchNoProcess = false
			p.TrialExited = false
			r.TrialExited = false
		})
		if err != nil {
			break
		}
		launchCtx, cancel := context.WithTimeout(ctx, 45*time.Second)
		input := kernel.ManagedProfile{EnvironmentID: task.plan.WorkID, SessionID: task.plan.SessionID, UserDataRef: task.plan.Move.Incoming, Fingerprint: profileInput(profile), Width: profile.Width, Height: profile.Height, RestoreTabs: false, URLs: []string{}, OnCreated: func(pid int, created string) error {
			return s.migrationUpdate(task, "trial-starting", func(p *migrationPlan, r *MigrationReport) { p.PID = pid; p.ProcessCreatedAt = created })
		}}
		var channel *kernel.ProtectedProxy
		var launchErr error
		if task.plan.Environment.ProxyID != "" {
			channel, launchErr = s.prepareMigrationNetwork(launchCtx, task, record, input)
		}
		var process *kernel.ManagedProcess
		if launchErr == nil {
			if channel != nil {
				input.Network = channel
			}
			process, launchErr = kernel.LaunchManagedProfile(launchCtx, s.root, record, input)
		}
		cancel()
		// Ownership transfers even when the launcher returns both a process and
		// an error. Late shutdown still reaches this exact Job through cleanup.
		s.mu.Lock()
		if process != nil {
			task.process = process
		} else {
			task.process = nil
			if channel != nil {
				task.process = ownRuntimeNetwork(nil, channel)
			}
		}
		s.mu.Unlock()
		if launchErr != nil {
			if process == nil && channel == nil {
				_ = s.migrationUpdate(task, "copy-ready", func(p *migrationPlan, r *MigrationReport) {
					p.LaunchNoProcess = true
					p.TrialExited = true
					r.TrialExited = true
				})
			}
			err = launchErr
			break
		}
		if process == nil {
			err = errors.New("migration launch returned no process")
			break
		}
		var observation kernel.MigrationObservation
		observation, err = probe.Sample(ctx, process, record, profileInput(profile), old)
		if err != nil {
			break
		}
		if old {
			stopCtx, stop := context.WithTimeout(ctx, 12*time.Second)
			err = process.Stop(stopCtx)
			stop()
			if err != nil || !normalMigrationExit(process.Snapshot()) {
				if err == nil {
					err = errors.New("old trial resources still alive")
				}
				break
			}
			observation.Fingerprint.NormalExit = true
			err = s.migrationUpdate(task, "copy-ready", func(p *migrationPlan, r *MigrationReport) {
				p.TrialExited = true
				r.TrialExited = true
				r.Before = &observation
			})
			s.mu.Lock()
			task.process = nil
			s.mu.Unlock()
		} else {
			err = s.migrationUpdate(task, "trial-running", func(p *migrationPlan, r *MigrationReport) { r.After = &observation })
		}
	}
	if err != nil {
		s.abortMigrationTrial(task, err)
		return
	}
	s.mu.Lock()
	process := task.process
	s.mu.Unlock()
	select {
	case <-ctx.Done():
		s.abortMigrationTrial(task, ctx.Err())
	case <-task.stop:
		s.stopMigrationTrial(ctx, task)
	case <-process.Done():
		s.stopMigrationTrial(ctx, task)
	}
}

func (s *Service) abortMigrationTrial(task *migrationTask, cause error) {
	s.mu.Lock()
	process := task.process
	s.mu.Unlock()
	if process != nil {
		_ = process.Close()
		if !process.Snapshot().ResourcesExited {
			s.protectMigration(task, errors.New("migration process cleanup unconfirmed"))
			return
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	s.finishMigrationRecovery(ctx, task, cause)
}

func (s *Service) stopMigrationTrial(ctx context.Context, task *migrationTask) {
	s.mu.Lock()
	process := task.process
	s.mu.Unlock()
	if process == nil {
		s.protectMigration(task, errors.New("migration process missing"))
		return
	}
	stopCtx, cancel := context.WithTimeout(ctx, 12*time.Second)
	err := process.Stop(stopCtx)
	cancel()
	if err != nil || !process.Snapshot().ResourcesExited {
		// Decide under the same lock as cancelMigration: cancellation either
		// reaches this worker or sees the protected idle task and starts cleanup.
		s.mu.Lock()
		cancelled := ctx.Err() != nil || task.operation.CancelRequested
		if !cancelled {
			s.protectMigrationLocked(task, errors.New("trial normal exit unconfirmed"))
		}
		s.mu.Unlock()
		if cancelled {
			s.abortMigrationTrial(task, context.Canceled)
		}
		return
	}
	if !normalMigrationExit(process.Snapshot()) {
		s.finishMigrationRecovery(ctx, task, errors.New("trial crashed instead of normal exit"))
		return
	}
	// Publish ready and hand off the worker under one lock. A caller that reads
	// ready must be able to commit immediately, never hit a still-running no-op.
	s.mu.Lock()
	err = s.migrationUpdateLocked(task, "ready", func(p *migrationPlan, r *MigrationReport) {
		p.TrialExited = true
		r.TrialExited = true
		if r.After != nil {
			v := *r.After
			v.Fingerprint.NormalExit = true
			r.After = &v
		}
	})
	if ctx.Err() != nil || task.operation.CancelRequested {
		s.mu.Unlock()
		s.abortMigrationTrial(task, context.Canceled)
		return
	}
	if err != nil {
		s.mu.Unlock()
		s.finishMigrationRecovery(context.Background(), task, err)
		return
	}
	task.process = nil
	task.running = false
	task.cancel = nil
	s.mu.Unlock()
}

func normalMigrationExit(snapshot kernel.RuntimeSnapshot) bool {
	return snapshot.ResourcesExited && snapshot.ExitKnown && snapshot.ExitCode == 0
}

func (s *Service) protectMigration(task *migrationTask, cause error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.protectMigrationLocked(task, cause)
}

func (s *Service) protectMigrationLocked(task *migrationTask, cause error) {
	if s.migrationTask != task {
		return
	}
	op := copyMigrationOperation(task.operation)
	op.PersistencePending = true
	op.State = "failed"
	op.Stage = "protected"
	op.Error = migrationProblem(cause).Error
	op.MigrationReport.Protected = true
	op.MigrationReport.Sequence++
	task.operation = op
	task.running = false
	task.cancel = nil
	task.finalPending = nil
	// This overlay is intentionally NOT persisted over a possibly newer decision.
}
