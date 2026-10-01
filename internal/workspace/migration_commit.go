package workspace

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"

	"github.com/axgiroud312-byte/prism-local-browser/internal/backup"
	"github.com/axgiroud312-byte/prism-local-browser/internal/kernel"
)

func (s *Service) migrationCheckpoint(stage string) error {
	if s.options.MigrationCheckpoint != nil {
		return s.options.MigrationCheckpoint(stage)
	}
	return nil
}

func (s *Service) pinMigrationKernels(ctx context.Context, plan migrationPlan) (func(), error) {
	p := restorePlan{Environments: []restoreStoredEnvironment{{History: []ProfileRevision{{Profile: plan.Preview.Before}, {Profile: plan.Preview.After}}}}, KernelMapping: map[string]string{plan.Preview.Before.KernelID: plan.Preview.Before.KernelID, plan.Preview.After.KernelID: plan.Preview.After.KernelID}}
	return s.pinRestoredKernels(ctx, p)
}

func (s *Service) commitMigration(ctx context.Context, task *migrationTask) {
	err := s.prepareMigrationSwitch(ctx, task)
	if err == nil {
		err = s.migrationCheckpoint("prepared")
	}
	plan := task.plan
	if err == nil && plan.Move.OldPresent {
		err = backup.MoveTree(ctx, s.root, plan.Move.Live, plan.Move.Previous, plan.Move.OldIdentity, plan.Move.OldFiles)
	}
	if err == nil {
		err = s.migrationCheckpoint("old-retained")
	}
	if err == nil {
		err = backup.MoveTree(ctx, s.root, plan.Move.Incoming, plan.Move.Live, plan.Move.NewIdentity, plan.Move.NewFiles)
	}
	if err == nil {
		err = s.migrationCheckpoint("new-installed")
	}
	if err == nil {
		s.mu.Lock()
		err = s.commitMigrationConfiguration(task)
		s.mu.Unlock()
	}
	if err == nil {
		err = s.migrationCheckpoint("configuration-committed")
	}
	// Always re-read the persisted decision, including successful COMMIT. Never
	// compensate using the in-memory pre-commit plan when the result is unknown.
	s.finishMigrationRecovery(ctx, task, err)
}

func (s *Service) prepareMigrationSwitch(ctx context.Context, task *migrationTask) error {
	plan := task.plan
	if !plan.TrialExited || !plan.BackupVerified {
		return errors.New("trial or backup incomplete")
	}
	release, err := s.pinMigrationKernels(ctx, plan)
	if err != nil {
		return err
	}
	defer release()
	hash, err := backup.PublishedDigest(ctx, filepath.Join(s.root, filepath.FromSlash(migrationBackupRef(plan.ID))))
	if err != nil || hash != plan.BackupSHA256 {
		return errors.New("pre-upgrade backup invalid")
	}
	if plan.Move.OldPresent {
		if err = backup.VerifyTree(ctx, s.root, plan.Move.Live, plan.Move.OldIdentity, plan.Move.OldFiles); err != nil {
			return err
		}
	} else {
		if _, present, e := backup.IdentifyTree(s.root, plan.Move.Live); e != nil || present {
			return errors.New("original absent profile changed")
		}
	}
	p, err := backup.CaptureProfile(ctx, s.root, plan.WorkID, plan.Move.Incoming, false, false)
	if err != nil {
		return err
	}
	files, err := p.Inventory(ctx)
	if err == nil {
		err = p.Validate(ctx)
	}
	err = errors.Join(err, p.Close())
	if err != nil {
		return err
	}
	identity, present, err := backup.IdentifyTree(s.root, plan.Move.Incoming)
	if err != nil || !present || identity != plan.WorkIdentity {
		return errors.New("trial copy missing")
	}
	if _, present, err = backup.IdentifyTree(s.root, plan.Move.Previous); err != nil || present {
		return errors.New("retained original position occupied")
	}
	// Parent for a never-initialized original may not exist yet. Only create its
	// parent, never a live user-data tree that would obscure the absent baseline.
	release, err = kernel.EnsureDirectory(s.root, "environments/"+plan.Environment.ID)
	if err != nil {
		return err
	}
	release()
	s.mu.Lock()
	baseline, err := migrationBaseline(s.db, plan)
	s.mu.Unlock()
	if err != nil || baseline != plan.Baseline {
		return errors.New("migration baseline changed")
	}
	return s.migrationUpdate(task, "prepared", func(p *migrationPlan, r *MigrationReport) {
		p.Prepared = true
		p.Move.NewIdentity = identity
		p.Move.NewFiles = files
	})
}

// Caller owns s.mu. Configuration/history and the durable decision are one tx.
func (s *Service) commitMigrationConfiguration(task *migrationTask) error {
	if s.migrationTask != task || task.operation.CancelRequested || s.closed || s.closeRequested.Load() {
		return context.Canceled
	}
	plan := task.plan
	op := copyMigrationOperation(task.operation)
	if !plan.Prepared || !plan.TrialExited || !plan.BackupVerified {
		return errors.New("migration commit not prepared")
	}
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	baseline, err := migrationBaseline(tx, plan)
	if err != nil || baseline != plan.Baseline {
		return errors.New("migration original configuration differs")
	}
	if err = checkProfileEvidence(tx, plan.Preview.After); err != nil {
		return err
	}
	config := plan.Environment.Configuration
	applyProfile(&config, plan.Preview.After)
	encoded, _ := json.Marshal(config)
	r, err := tx.Exec("UPDATE environments SET kernel_id=?,revision=revision+1 WHERE id=? AND revision=? AND fingerprint_id=?", config.CoreID, plan.Environment.ID, plan.Preview.ExpectedRevision, plan.FingerprintID)
	if err != nil {
		return err
	}
	if n, e := r.RowsAffected(); e != nil || n != 1 {
		return errors.New("migration environment update conflict")
	}
	if _, err = tx.Exec("UPDATE fingerprints SET kernel_id=?,config_json=? WHERE id=?", config.CoreID, string(encoded), plan.FingerprintID); err != nil {
		return err
	}
	if err = appendProfile(tx, plan.FingerprintID, plan.Preview.After, "kernel-migration", 0); err != nil {
		return err
	}
	if _, err = tx.Exec("UPDATE environment_data_state SET state=? WHERE environment_id=?", dataRuntimeClaimed, plan.Environment.ID); err != nil {
		return err
	}
	if _, err = tx.Exec("DELETE FROM runtime_sessions WHERE environment_id=?", plan.Environment.ID); err != nil {
		return err
	}
	plan.Committed = true
	plan.CommittedBaseline, err = migrationBaseline(tx, plan)
	if err != nil {
		return err
	}
	op.Stage = "committed"
	op.MigrationReport.Committed = true
	op.MigrationReport.Sequence++
	if err = saveMigration(tx, plan, op, "committed"); err != nil {
		return err
	}
	if _, err = tx.Exec("INSERT INTO activities(id,created_at,action,target,detail) VALUES(?,?,?,?,?)", id(), timestamp(), "内核迁移配置已提交", plan.Environment.Name, "保留原seed、升级前完整备份与旧构建；正在核对目录。"); err != nil {
		return err
	}
	if s.options.BeforeCommit != nil {
		if err = s.options.BeforeCommit(); err != nil {
			return err
		}
	}
	if err = s.commitMigrationTransaction(tx); err != nil {
		return err
	}
	task.plan, task.operation, task.phase = plan, op, "committed"
	return nil
}
