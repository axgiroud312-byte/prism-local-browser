package workspace

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"time"

	"github.com/axgiroud312-byte/prism-local-browser/internal/backup"
	"github.com/axgiroud312-byte/prism-local-browser/internal/kernel"
	"golang.org/x/sys/windows"
	"modernc.org/sqlite"
)

func backupFailure(err error) *Error {
	var safe *Error
	if errors.As(err, &safe) {
		return safe
	}
	if errors.Is(err, context.Canceled) {
		return &Error{Code: "APPLICATION_INTERRUPTED", Message: "应用退出中断本次备份，原数据未替换；临时文件不是成功包，未自动重做。", Retryable: true}
	}
	var sqliteError *sqlite.Error
	if errors.Is(err, windows.ERROR_DISK_FULL) || errors.Is(err, windows.ERROR_HANDLE_DISK_FULL) || errors.As(err, &sqliteError) && sqliteError.Code()&255 == 13 {
		return &Error{Code: "DISK_FULL", Message: "备份输出磁盘已满；原浏览数据仍在，临时文件不是完成包，请释放空间后选择新文件重试。", Retryable: true}
	}
	var problem *kernel.Problem
	if errors.As(err, &problem) {
		return kernelFailure(err).Error
	}
	return &Error{Code: "BACKUP_EXPORT_FAILED", Message: "备份的停止、目录、配置或文件核对未全部成功，未确认完整发布；原浏览数据未替换，请核对占用、权限与空间。", Retryable: true}
}

func (s *Service) observeBackup(task *backupTask, stage string, update func(*BackupReport)) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.backupTasks[task.operation.ID] != task || s.closed || s.closeRequested.Load() || task.operation.CancelRequested {
		return context.Canceled
	}
	operation := copyBackupOperation(task.operation)
	operation.State, operation.Stage = "running", stage
	operation.BackupReport.Sequence++
	if update != nil {
		update(operation.BackupReport)
	}
	if err := s.persistBackup(task, operation, "running"); err != nil {
		return err
	}
	task.operation = operation
	return nil
}

func (s *Service) stopBackupTarget(ctx context.Context, task *backupTask, target backupTarget) error {
	s.mu.Lock()
	if s.closed || ctx.Err() != nil || s.backupUses[target.ID] != task {
		s.mu.Unlock()
		return context.Canceled
	}
	if !s.runtimeOwnsProfileUse(target.ID) {
		s.mu.Unlock()
		return nil
	}
	result := s.stopRuntime(runtimeRequest{EnvironmentID: target.ID, RequestID: id()})
	s.mu.Unlock()
	if !result.OK {
		return result.Error
	}
	operationID := result.OperationID
	timer := time.NewTicker(100 * time.Millisecond)
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-timer.C:
		}
		result := s.Call(Request{Mode: "native", Method: "Operation.Read", Payload: []byte(`{"operationId":"` + operationID + `"}`)})
		if !result.OK {
			return result.Error
		}
		operation, ok := result.Data.(Operation)
		if !ok {
			return errors.New("stop result type unavailable")
		}
		if operation.PersistencePending {
			return &Error{Code: "STORAGE_WRITE_FAILED", Message: "正常停止观测尚未持久化，未把变化中的数据标为完整备份。", Retryable: true}
		}
		if operation.State == "failed" || operation.State == "cancelled" {
			if operation.Error != nil {
				return operation.Error
			}
			return errors.New("normal stop failed without reason")
		}
		if operation.State == "completed" {
			s.mu.Lock()
			stillOwned := s.runtimeOwnsProfileUse(target.ID)
			valid := s.backupUses[target.ID] == task
			s.mu.Unlock()
			if stillOwned || !valid {
				return &Error{Code: "PROFILE_BUSY", Message: "正常停止返回后仍不能确认实际进程树/目录释放，未复制浏览数据。", Retryable: true}
			}
			return nil
		}
	}
}

func (s *Service) runBackup(ctx context.Context, task *backupTask, input backupExecution) {
	defer s.workers.Done()
	var cause error
	defer func() { s.finishBackup(task, cause) }()
	select {
	case s.backupGate <- struct{}{}:
		defer func() { <-s.backupGate }()
	case <-ctx.Done():
		cause = ctx.Err()
		return
	}
	if cause = s.observeBackup(task, "stopping-environments", nil); cause != nil {
		return
	}
	for _, target := range input.Targets {
		if cause = s.stopBackupTarget(ctx, task, target); cause != nil {
			return
		}
	}
	// A start already in flight at acceptance may have crossed the durable data
	// initialization boundary while stopping. Re-read only those same frozen IDs.
	s.mu.Lock()
	for index, target := range input.Targets {
		if s.backupUses[target.ID] != task || s.runtimeOwnsProfileUse(target.ID) {
			cause = errors.New("backup ownership or stop unconfirmed")
			break
		}
		input.Targets[index], cause = s.backupDataTarget(target.ID, target.Reference)
		if cause != nil {
			break
		}
	}
	s.mu.Unlock()
	if cause != nil {
		return
	}
	profiles := []*backup.Profile{}
	defer func() {
		for _, profile := range profiles {
			_ = profile.Close()
		}
	}()
	for _, target := range input.Targets {
		profile, err := backup.CaptureProfile(ctx, s.root, target.ID, target.Reference, !target.DirectoryRequired, target.NeverUsed)
		if err != nil {
			cause = err
			return
		}
		profiles = append(profiles, profile)
	}
	if cause = s.observeBackup(task, "configuration-snapshot", nil); cause != nil {
		return
	}
	stageRef := "backups/staging/" + input.OperationID
	release, err := kernel.EnsureDirectory(s.root, stageRef)
	if err != nil {
		cause = err
		return
	}
	defer release()
	freezeStage, err := backup.PinStagingDirectory(filepath.Join(s.root, filepath.FromSlash(stageRef)))
	if err != nil {
		cause = err
		return
	}
	defer freezeStage()
	snapshotPath := filepath.Join(s.root, filepath.FromSlash(stageRef), "configuration.sqlite")
	if _, err = os.Lstat(snapshotPath); !os.IsNotExist(err) {
		cause = errors.New("snapshot staging file already exists")
		return
	}
	manifest, err := s.createBackupSnapshot(ctx, input, snapshotPath)
	if err != nil {
		cause = err
		return
	}
	// Source-side staging is an internal scratch file only; no profile files are
	// created or rewritten. Retain interrupted scratch for explicit later cleanup.
	output, err := backup.NewOutput(s.root, input.Destination, input.OperationID)
	if err != nil {
		cause = err
		return
	}
	defer output.Close()
	writer := backup.NewWriter(output.File)
	file, releaseSnapshot, err := backup.FreezeFile(snapshotPath)
	if err != nil {
		cause = err
		return
	}
	info, err := file.Stat()
	if err == nil {
		err = writer.Add(ctx, "configuration.sqlite", file, info.Size(), false)
	}
	file.Close()
	releaseSnapshot()
	if err != nil {
		cause = err
		return
	}
	for index, profile := range profiles {
		if profile.Missing {
			manifest.Environments[index].DataState = "never-initialized"
		}
		if cause = profile.Write(ctx, writer, input.Targets[index].Reference); cause != nil {
			return
		}
		if cause = s.observeBackup(task, "copying-browser-data", func(report *BackupReport) {
			report.CopiedEnvironmentCount = index + 1
			report.FileCount = int64(len(writer.Files))
			report.ByteCount = 0
			for _, entry := range writer.Files {
				report.ByteCount += entry.Size
			}
		}); cause != nil {
			return
		}
	}
	manifestHash, err := writer.Finish(ctx, manifest)
	if err != nil {
		cause = err
		return
	}
	if cause = s.observeBackup(task, "verifying-package", nil); cause != nil {
		return
	}
	if cause = backup.VerifyWritten(ctx, output.File, writer.Files); cause != nil {
		return
	}
	for _, profile := range profiles {
		if cause = profile.Validate(ctx); cause != nil {
			return
		}
	}
	archiveHash, err := output.Digest(ctx)
	if err != nil {
		cause = err
		return
	}
	// Linearization: cancel/shutdown cannot interleave between this final check,
	// durable publishing journal and rename of the owned object. Late cancel only
	// observes the actual published result; no rollback or source deletion.
	s.mu.Lock()
	if ctx.Err() != nil || task.operation.CancelRequested || s.closed || s.closeRequested.Load() {
		s.mu.Unlock()
		cause = context.Canceled
		return
	}
	operation := copyBackupOperation(task.operation)
	operation.Stage = "publishing"
	operation.BackupReport.Sequence++
	operation.BackupReport.ArchiveSHA256, operation.BackupReport.ManifestSHA256 = archiveHash, manifestHash
	operation.BackupReport.FileCount = int64(len(writer.Files))
	operation.BackupReport.ByteCount = 0
	for _, entry := range writer.Files {
		operation.BackupReport.ByteCount += entry.Size
	}
	if cause = s.persistBackup(task, operation, "publishing"); cause != nil {
		s.mu.Unlock()
		return
	}
	task.operation, task.publishing = operation, true
	if cause = output.Publish(); cause == nil {
		task.operation.BackupReport.Published = true
	}
	s.mu.Unlock()
}
