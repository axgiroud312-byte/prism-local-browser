package workspace

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"time"

	"github.com/axgiroud312-byte/prism-local-browser/internal/desktopbase"
	"github.com/axgiroud312-byte/prism-local-browser/internal/kernel"
)

const BundledKernelVersion = "148.0.7778.215"
const BundledKernelArchive = "ungoogled-chromium_148.0.7778.215-1.1_windows_x64.zip"
const BundledKernelChecksum = "9ef3f471b7a6641b4224532522b29141ce3746e27d55788d88e2fd951f362579"

// PrepareBundledKernel is a desktop-host entry point, never an RPC accepting
// client paths. It uses the ordinary installer and its persistent operation.
// An existing exact build is reused; saved environment references never change.
func (s *Service) PrepareBundledKernel(ctx context.Context, archive string) error {
	if ctx.Err() != nil {
		return &Error{Code: "OPERATION_CANCELLED", Message: "本次148内核准备已取消，未接受新任务。", Retryable: true}
	}
	s.mu.Lock()
	if s.closed || s.closeRequested.Load() {
		s.mu.Unlock()
		return &Error{Code: "NATIVE_UNAVAILABLE", Message: "工作区正在关闭，未准备内置内核。", Retryable: true}
	}
	// Interrupted directory maintenance must finish before installing anything.
	if s.restoreTask != nil || s.recycleTask != nil || s.migrationTask != nil {
		s.mu.Unlock()
		return nil
	}
	records, err := s.listKernels()
	s.mu.Unlock()
	if err != nil {
		return storageFailure(err).Error
	}
	for _, record := range records {
		if record.Version != BundledKernelVersion || record.ArchiveSHA256 != BundledKernelChecksum {
			continue
		}
		if record.Status != "verified" {
			return &Error{Code: "KERNEL_INTEGRITY_FAILED", Message: "已登记的148内核不可用，请在内核管理中核对原构建；没有替换原环境的内核。", Retryable: true}
		}
		directory, err := kernel.RecordDirectory(s.root, record.Record)
		if err == nil {
			err = kernel.VerifyFiles(directory, record.Files)
		}
		if err != nil {
			var integrity *kernel.Problem
			if errors.As(err, &integrity) && (integrity.Code == "KERNEL_INTEGRITY_FAILED" || integrity.Code == "PATH_OUTSIDE_ROOT") {
				s.mu.Lock()
				_, persistErr := s.db.Exec("UPDATE kernels SET status='missing' WHERE id=?", record.ID)
				s.mu.Unlock()
				if persistErr != nil {
					return storageFailure(persistErr).Error
				}
			}
			return kernelFailure(err).Error
		}
		return s.useBundledDefault(record.ID)
	}
	if _, err := os.Stat(archive); os.IsNotExist(err) {
		// Source-only and historical unbundled builds remain usable.
		return nil
	} else if err != nil {
		return &Error{Code: "KERNEL_MISSING", Message: "程序随附的148内核归档无法读取，请检查程序文件是否完整。", Retryable: true}
	}
	if err := desktopbase.ValidatePath(archive); err != nil {
		return &Error{Code: "PATH_OUTSIDE_ROOT", Message: "程序随附的148内核路径包含目录链接，未执行或安装。", Retryable: true}
	}
	if filepath.Base(archive) != BundledKernelArchive {
		return &Error{Code: "VALIDATION_FAILED", Message: "程序随附的148内核归档名称不匹配，未安装。"}
	}
	token := id()
	s.mu.Lock()
	s.archives[token] = archive
	s.mu.Unlock()
	defer func() {
		s.mu.Lock()
		delete(s.archives, token)
		s.mu.Unlock()
	}()
	accepted := s.bundledCall("Kernel.Install", kernel.InstallInput{
		Source: "local", Version: BundledKernelVersion, ExpectedChecksum: BundledKernelChecksum,
		ArchiveToken: token, Trusted: true, RequestID: id(),
	})
	if !accepted.OK {
		return accepted.Error
	}
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for {
		result := s.bundledCall("Operation.Read", map[string]string{"operationId": accepted.OperationID})
		if !result.OK {
			return result.Error
		}
		operation, ok := result.Data.(Operation)
		if !ok {
			return &Error{Code: "STORAGE_READ_FAILED", Message: "内置内核准备结果尚未核实，请在内核管理中读取原任务。", Retryable: true}
		}
		switch operation.State {
		case "completed":
			return s.useBundledDefault(operation.KernelID)
		case "failed", "cancelled":
			if operation.Error != nil {
				return operation.Error
			}
			return &Error{Code: "OPERATION_CANCELLED", Message: "本次148内核准备未完成，请在内核管理中核对。", Retryable: true}
		}
		select {
		case <-ctx.Done():
			s.bundledCall("Operation.Cancel", map[string]string{"operationId": accepted.OperationID})
			return &Error{Code: "OPERATION_CANCELLED", Message: "本次148内核准备已取消或超时；原数据保留，请重新打开程序重试。", Retryable: true}
		case <-ticker.C:
		}
	}
}

func (s *Service) bundledCall(method string, input any) Result {
	payload, err := json.Marshal(input)
	if err != nil {
		return storageFailure(err)
	}
	return s.Call(Request{Mode: "native", Method: method, Payload: payload})
}

func (s *Service) useBundledDefault(kernelID string) error {
	s.mu.Lock()
	current, err := s.readKernelDefault()
	s.mu.Unlock()
	if err != nil {
		return storageFailure(err).Error
	}
	if current.KernelID != "kernel-pending" {
		return nil
	}
	result := s.bundledCall("Kernel.SetDefault", KernelDefaultRequest{
		KernelID: kernelID, ExpectedRevision: current.Revision, RequestID: id(),
	})
	if !result.OK {
		return result.Error
	}
	return nil
}
