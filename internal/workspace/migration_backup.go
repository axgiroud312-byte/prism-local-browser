package workspace

import (
	"context"
	"errors"
	"os"
	"path/filepath"

	"github.com/axgiroud312-byte/prism-local-browser/internal/backup"
	"github.com/axgiroud312-byte/prism-local-browser/internal/kernel"
)

func (s *Service) migrationUpdate(task *migrationTask, phase string, change func(*migrationPlan, *MigrationReport)) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.migrationUpdateLocked(task, phase, change)
}

func (s *Service) migrationUpdateLocked(task *migrationTask, phase string, change func(*migrationPlan, *MigrationReport)) error {
	if s.migrationTask != task || s.closed || s.closeRequested.Load() || task.operation.CancelRequested {
		return context.Canceled
	}
	plan := task.plan
	op := copyMigrationOperation(task.operation)
	if change != nil {
		change(&plan, op.MigrationReport)
	}
	op.State, op.Stage, op.PersistencePending, op.Error = "running", phase, false, nil
	op.MigrationReport.Sequence++
	if err := s.persistMigration(plan, op, phase); err != nil {
		return err
	}
	task.plan, task.operation, task.phase = plan, op, phase
	return nil
}

func (s *Service) prepareMigrationBackup(ctx context.Context, task *migrationTask) error {
	plan := task.plan
	p, err := backup.CaptureProfile(ctx, s.root, plan.Target.ID, plan.Target.Reference, !plan.Target.DirectoryRequired, plan.Target.NeverUsed)
	if err != nil {
		return err
	}
	defer p.Close()
	files, err := p.Inventory(ctx)
	if err != nil {
		return err
	}
	identity, present, err := backup.IdentifyTree(s.root, plan.Target.Reference)
	if err != nil || present == p.Missing {
		return errors.New("migration source identity differs")
	}
	stage := "backups/staging/" + plan.ID
	release, err := kernel.EnsureDirectory(s.root, stage)
	if err != nil {
		return err
	}
	release()
	path := filepath.Join(s.root, filepath.FromSlash(stage), "configuration.sqlite")
	if _, err = os.Lstat(path); !os.IsNotExist(err) {
		return errors.New("migration snapshot path occupied")
	}
	freeze, err := backup.PinStagingDirectory(filepath.Dir(path))
	if err != nil {
		return err
	}
	defer freeze()
	manifest, err := s.createBackupSnapshot(ctx, backupExecution{OperationID: plan.ID, Scope: "selected", CreatedAt: plan.CreatedAt, Targets: []backupTarget{plan.Target}}, path)
	if err != nil {
		return err
	}
	// Validate the configuration against the same strict schema/identity contract
	// consumed by formal restoration, including profile history and credentials.
	if _, err = s.readRestoreConfiguration(ctx, path, manifest); err != nil {
		return err
	}
	release, err = kernel.EnsureDirectory(s.root, migrationStageRef(plan.ID))
	if err != nil {
		return err
	}
	release()
	output, err := backup.NewMigrationOutput(s.root, plan.ID)
	if err != nil {
		return err
	}
	defer output.Close()
	writer := backup.NewWriter(output.File)
	file, unpin, err := backup.FreezeFile(path)
	if err != nil {
		return err
	}
	info, err := file.Stat()
	if err == nil {
		err = writer.Add(ctx, "configuration.sqlite", file, info.Size(), false)
	}
	err = errors.Join(err, file.Close())
	unpin()
	if err != nil {
		return err
	}
	if p.Missing {
		manifest.Environments[0].DataState = "never-initialized"
	}
	if err = p.Write(ctx, writer, plan.Target.Reference); err != nil {
		return err
	}
	if _, err = writer.Finish(ctx, manifest); err != nil {
		return err
	}
	if err = backup.VerifyWritten(ctx, output.File, writer.Files); err != nil {
		return err
	}
	if err = p.Validate(ctx); err != nil {
		return err
	}
	if err = p.Close(); err != nil {
		return err
	}
	digest, err := output.Digest(ctx)
	if err != nil {
		return err
	}
	// Publication is preceded by its digest. A crash here keeps an incomplete
	// backup and untouched source; it can never authorize a trial or switch.
	if err = s.migrationUpdate(task, "accepted", func(p *migrationPlan, r *MigrationReport) {
		p.BackupSHA256 = digest
		r.ArchiveSHA256 = digest
		p.Move.OldIdentity = identity
		p.Move.OldPresent = present
		p.Move.OldFiles = files
	}); err != nil {
		return err
	}
	if err = output.Publish(); err != nil {
		return err
	}
	if err = output.Close(); err != nil {
		return err
	}
	file, unpin, err = backup.FreezeFile(filepath.Join(s.root, filepath.FromSlash(migrationBackupRef(plan.ID))))
	if err != nil {
		return err
	}
	pkg, err := backup.Read(ctx, file)
	file.Close()
	unpin()
	if err != nil {
		return err
	}
	if pkg.ArchiveSHA256 != digest {
		return errors.New("published migration backup differs")
	}
	return s.migrationUpdate(task, "backup-ready", func(p *migrationPlan, r *MigrationReport) { p.BackupVerified = true; r.BackupVerified = true })
}

func (s *Service) prepareMigrationCopy(ctx context.Context, task *migrationTask) error {
	plan := task.plan
	file, release, err := backup.FreezeFile(filepath.Join(s.root, filepath.FromSlash(migrationBackupRef(plan.ID))))
	if err != nil {
		return err
	}
	pkg, err := backup.Read(ctx, file)
	if err == nil && pkg.ArchiveSHA256 != plan.BackupSHA256 {
		err = errors.New("migration backup changed")
	}
	stage := migrationStageRef(plan.ID) + "/extract"
	if err == nil {
		if _, present, e := backup.IdentifyTree(s.root, stage); e != nil || present {
			err = errors.New("migration extraction already exists")
		}
	}
	if err == nil {
		err = pkg.ExtractProfiles(ctx, s.root, stage)
	}
	file.Close()
	release()
	if err != nil {
		return err
	}
	incoming := stage + "/" + plan.Target.Reference
	identity, exists, err := backup.IdentifyTree(s.root, incoming)
	if err != nil || !exists {
		return errors.New("extracted migration data missing")
	}
	// Extraction includes the root directory even for a never-initialized source.
	workParent := "environments/" + plan.WorkID
	release, err = kernel.EnsureDirectory(s.root, workParent)
	if err != nil {
		return err
	}
	release()
	if err = backup.MoveTree(ctx, s.root, incoming, plan.Move.Incoming, identity, plan.Move.OldFiles); err != nil {
		return err
	}
	return s.migrationUpdate(task, "copy-ready", func(p *migrationPlan, r *MigrationReport) { p.WorkIdentity = identity })
}
