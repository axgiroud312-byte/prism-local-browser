package workspace

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"

	"github.com/axgiroud312-byte/prism-local-browser/internal/desktopbase"
	"github.com/axgiroud312-byte/prism-local-browser/internal/kernel"
	"github.com/google/uuid"
)

type kernelTask struct {
	operation Operation
	cancel    context.CancelFunc
}

func (s *Service) listKernelOperations() ([]Operation, error) {
	rows, err := s.db.Query("SELECT result_json FROM operations WHERE json_extract(result_json,'$.kind') LIKE 'kernel-%' ORDER BY rowid DESC LIMIT 20")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	operations := []Operation{}
	for rows.Next() {
		var text string
		if err = rows.Scan(&text); err != nil {
			return nil, err
		}
		var operation Operation
		if err = json.Unmarshal([]byte(text), &operation); err != nil {
			return nil, err
		}
		operations = append(operations, operation)
	}
	return operations, rows.Err()
}

func kernelFailure(err error) Result {
	var p *kernel.Problem
	if errors.As(err, &p) {
		result := failure(p.Code, p.Message, p.Retryable)
		result.Error.Details = map[string]any{"reason": p.Reason}
		return result
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		result := failure("OPERATION_CANCELLED", "本次内核任务已取消或超时；旧内核与环境引用未修改。", true)
		result.Error.Details = map[string]any{"reason": "cancelled-or-timeout"}
		return result
	}
	return storageFailure(err)
}
func (s *Service) selectKernelArchive() Result {
	if s.options.ChooseArchive == nil {
		return failure("CAPABILITY_UNSUPPORTED", "本地文件选择器尚未就绪，请在Windows桌面使用。", true)
	}
	path, err := s.options.ChooseArchive()
	if err != nil {
		return failure("VALIDATION_FAILED", "本地文件选择未完成，未执行或安装任何程序。", true)
	}
	if path == "" {
		return success(map[string]string{"status": "cancelled"}, "")
	}
	if !strings.EqualFold(filepath.Ext(path), ".zip") {
		return failure("VALIDATION_FAILED", "请选择可信ZIP归档，不直接运行安装器或任意exe。", false)
	}
	info, err := os.Stat(path)
	if err != nil || !info.Mode().IsRegular() {
		return failure("VALIDATION_FAILED", "选择的归档无法读取，请重新选择。", true)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return failure("NATIVE_UNAVAILABLE", "工作区已关闭。", true)
	}
	token := id()
	s.archives[token] = path
	return success(map[string]string{"status": "selected", "archiveToken": token, "name": filepath.Base(path)}, "")
}

func (s *Service) listKernels() ([]KernelView, error) {
	rows, err := s.db.Query(`SELECT e.record_json,k.status FROM kernel_evidence e JOIN kernels k ON k.id=e.kernel_id ORDER BY k.rowid DESC`)
	if err != nil {
		return nil, err
	}
	records := []KernelView{}
	for rows.Next() {
		var text, status string
		if err = rows.Scan(&text, &status); err != nil {
			rows.Close()
			return nil, err
		}
		var record kernel.Record
		if err = json.Unmarshal([]byte(text), &record); err != nil {
			rows.Close()
			return nil, err
		}
		if err = kernel.CheckRecord(record); err != nil {
			rows.Close()
			return nil, err
		}
		records = append(records, KernelView{Record: record, Status: status, UsedBy: []string{}})
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, err
	}
	for index := range records {
		if err = s.db.QueryRow("SELECT COUNT(*) FROM environments WHERE kernel_id=?", records[index].ID).Scan(&records[index].UsedCount); err != nil {
			return nil, err
		}
		rows, err = s.db.Query("SELECT id FROM environments WHERE kernel_id=? ORDER BY code LIMIT 100", records[index].ID)
		if err != nil {
			return nil, err
		}
		for rows.Next() {
			var environmentID string
			if err = rows.Scan(&environmentID); err != nil {
				rows.Close()
				return nil, err
			}
			records[index].UsedBy = append(records[index].UsedBy, environmentID)
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return nil, err
		}
	}
	return records, nil
}
func (s *Service) savedKernel(kernelID string) (kernel.Record, error) {
	var text string
	if err := s.db.QueryRow("SELECT record_json FROM kernel_evidence WHERE kernel_id=?", kernelID).Scan(&text); err != nil {
		return kernel.Record{}, err
	}
	var record kernel.Record
	if err := json.Unmarshal([]byte(text), &record); err != nil {
		return record, err
	}
	if record.ID != kernelID {
		return record, errors.New("kernel evidence ID mismatch")
	}
	if err := kernel.CheckRecord(record); err != nil {
		return record, err
	}
	return record, nil
}
func (s *Service) storeOperation(operation Operation) error {
	bytes, err := json.Marshal(operation)
	if err != nil {
		return err
	}
	_, err = s.db.Exec("UPDATE operations SET result_json=? WHERE id=?", string(bytes), operation.ID)
	return err
}

func (s *Service) kernelCall(request Request) Result {
	if request.Method == "Kernel.List" {
		records, err := s.listKernels()
		if err != nil {
			return failure("STORAGE_READ_FAILED", "本机内核记录无法读取，原数据保留。", true)
		}
		return success(records, "")
	}
	var input kernel.InstallInput
	var target struct {
		KernelID  string `json:"kernelId"`
		RequestID string `json:"requestId"`
	}
	if request.Method == "Kernel.Install" {
		if decode(request.Payload, &input) != nil {
			return failure("VALIDATION_FAILED", "内核安装请求包含无效字段；不接受任意路径或启动参数。", false)
		}
		if err := kernel.ValidateInput(input); err != nil {
			return kernelFailure(err)
		}
	} else if decode(request.Payload, &target) != nil || target.KernelID == "" || target.RequestID == "" {
		return failure("VALIDATION_FAILED", "请选择具体内核并提供请求标识。", false)
	}
	requestID := input.RequestID
	if request.Method != "Kernel.Install" {
		requestID = target.RequestID
	}
	digest := sha256.Sum256(append([]byte(request.Method), request.Payload...))
	signature := hex.EncodeToString(digest[:])
	var priorSignature, priorJSON string
	err := s.db.QueryRow("SELECT signature,result_json FROM requests WHERE id=?", requestID).Scan(&priorSignature, &priorJSON)
	if err == nil {
		if signature != priorSignature {
			return failure("REQUEST_ID_REUSED", "同一请求标识不能用于不同内核操作。", false)
		}
		var prior Result
		if json.Unmarshal([]byte(priorJSON), &prior) != nil {
			return failure("STORAGE_READ_FAILED", "请求记录无法读取。", true)
		}
		return prior
	}
	if err != sql.ErrNoRows {
		return storageFailure(err)
	}
	if s.kernelTask != nil {
		return failure("PROFILE_BUSY", "另一个内核维护任务尚未结束，请等待或取消该任务；这不是内核数量配额。", true)
	}
	var record kernel.Record
	localPath := ""
	if request.Method == "Kernel.Install" && input.Source == "local" {
		var exists bool
		localPath, exists = s.archives[input.ArchiveToken]
		if !exists {
			return failure("VALIDATION_FAILED", "本地ZIP选择已失效，请重新选择文件。", true)
		}
	} else if request.Method != "Kernel.Install" {
		record, err = s.savedKernel(target.KernelID)
		if err != nil {
			return failure("KERNEL_MISSING", "所选精确内核记录不存在；未改用其他版本。", false)
		}
		if request.Method == "Kernel.Delete" {
			var count int
			if err = s.db.QueryRow("SELECT (SELECT COUNT(*) FROM fingerprints WHERE kernel_id=?)+(SELECT COUNT(*) FROM fingerprint_revisions WHERE kernel_id=?)", target.KernelID, target.KernelID).Scan(&count); err != nil {
				return storageFailure(err)
			}
			if count > 0 {
				result := failure("PROFILE_BUSY", "该构建被设备档案引用，不能直接移除；请先查看受影响环境。", false)
				result.Error.Details = map[string]any{"reason": "kernel-in-use", "kernelId": target.KernelID}
				return result
			}
		}
	}
	operation := Operation{ID: id(), Kind: map[string]string{"Kernel.Install": "kernel-install", "Kernel.Verify": "kernel-verify", "Kernel.Delete": "kernel-delete"}[request.Method], State: "accepted", Stage: "queued", Total: 1, CompletedIDs: []string{}, KernelID: target.KernelID, ResourceKey: id()}
	input.ResourceKey = operation.ResourceKey
	result := success(map[string]any{"status": "accepted", "operation": operation}, operation.ID)
	operationJSON, _ := json.Marshal(operation)
	resultJSON, _ := json.Marshal(result)
	tx, err := s.db.Begin()
	if err != nil {
		return storageFailure(err)
	}
	defer tx.Rollback()
	if _, err = tx.Exec("INSERT INTO operations(id,result_json) VALUES(?,?)", operation.ID, string(operationJSON)); err != nil {
		return storageFailure(err)
	}
	if _, err = tx.Exec("INSERT INTO requests(id,signature,result_json) VALUES(?,?,?)", requestID, signature, string(resultJSON)); err != nil {
		return storageFailure(err)
	}
	if err = tx.Commit(); err != nil {
		return storageFailure(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	s.kernelTask = &kernelTask{operation: operation, cancel: cancel}
	s.workers.Add(1)
	go s.runKernelTask(ctx, operation, input, localPath, record)
	return result
}

func (s *Service) kernelStage(operationID, stage string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.kernelTask == nil || s.kernelTask.operation.ID != operationID {
		return
	}
	s.kernelTask.operation.State = "running"
	s.kernelTask.operation.Stage = stage
	if s.storeOperation(s.kernelTask.operation) != nil {
		s.kernelTask.cancel()
	}
}
func (s *Service) runKernelTask(ctx context.Context, operation Operation, input kernel.InstallInput, localPath string, record kernel.Record) {
	defer s.workers.Done()
	var prepared *kernel.Prepared
	var report *kernel.Report
	var taskErr error
	integrityFailureConfirmed := false
	progress := func(stage string) { s.kernelStage(operation.ID, stage) }
	if operation.Kind == "kernel-install" {
		prepare := s.options.PrepareKernel
		if prepare == nil {
			prepare = kernel.Prepare
		}
		prepared, taskErr = prepare(ctx, s.root, input, localPath, nil, progress)
	} else if operation.Kind == "kernel-verify" {
		progress("verifying-files")
		staging := filepath.Join(s.root, "staging", "kernel-"+operation.ResourceKey)
		release, err := kernel.EnsureDirectory(s.root, "staging")
		if err == nil {
			err = os.Mkdir(staging, 0700)
			release()
		}
		if err != nil {
			taskErr = err
		} else {
			pin, err := desktopbase.PinDirectories(staging)
			if err != nil {
				taskErr = err
			} else {
				verify := s.options.VerifyKernel
				if verify == nil {
					verify = kernel.Verify
				}
				fresh, err := verify(ctx, s.root, record, staging)
				taskErr = err
				var integrity *kernel.Problem
				integrityFailureConfirmed = errors.As(err, &integrity) && (integrity.Code == "KERNEL_INTEGRITY_FAILED" || integrity.Code == "PATH_OUTSIDE_ROOT")
				report = &fresh
				pin()
			}
			if cleanupErr := kernel.RemoveOwnedTree(staging); cleanupErr != nil {
				taskErr = cleanupErr
			}
		}
	}
	s.mu.Lock()
	operation = s.kernelTask.operation
	if ctx.Err() != nil {
		taskErr = ctx.Err()
	}
	if taskErr == nil {
		switch operation.Kind {
		case "kernel-install":
			operation.KernelID = prepared.Record.ID
			operation.Stage = "publishing"
			if taskErr = s.storeOperation(operation); taskErr == nil {
				taskErr = s.publishKernel(prepared, &operation)
			}
		case "kernel-verify":
			_, taskErr = s.db.Exec("UPDATE kernels SET status='verified' WHERE id=?", record.ID)
			if taskErr == nil {
				operation.CompletedIDs = []string{record.ID}
				operation.Report = report
			}
		case "kernel-delete":
			taskErr = s.deleteKernel(record, &operation)
		}
	}
	if operation.Kind == "kernel-verify" && integrityFailureConfirmed {
		if _, err := s.db.Exec("UPDATE kernels SET status='missing' WHERE id=?", record.ID); err != nil {
			taskErr = err
		}
	}
	if prepared != nil {
		if cleanupErr := kernel.RemoveOwnedTree(prepared.Staging); cleanupErr != nil {
			taskErr = &kernel.Problem{Code: "STORAGE_WRITE_FAILED", Reason: "cleanup-failed", Message: "本次暂存清理失败，请关闭占用后重试。安装状态以重新读取记录为准。", Retryable: true}
		}
	}
	if taskErr == nil {
		operation.State = "completed"
		operation.Stage = "completed"
	} else {
		operation.State = "failed"
		operation.Stage = "failed"
		operation.Error = kernelFailure(taskErr).Error
		if integrityFailureConfirmed {
			if operation.Error.Details == nil {
				operation.Error.Details = map[string]any{}
			}
			operation.Error.Details["integrityFailureConfirmed"] = true
			operation.Error.Message += " 已确认该构建完整性失败，仍保持不可用。"
		}
		if errors.Is(taskErr, context.Canceled) {
			operation.State = "cancelled"
			operation.Stage = "cancelled"
		}
	}
	if err := s.storeOperation(operation); err != nil {
		operation.State = "failed"
		operation.Error = storageFailure(err).Error
		s.storeOperation(operation)
	}
	s.kernelTask.cancel()
	s.kernelTask = nil
	s.mu.Unlock()
}

func (s *Service) publishKernel(prepared *kernel.Prepared, operation *Operation) (resultErr error) {
	parent := filepath.Join(s.root, "kernels")
	if err := desktopbase.ValidateTree(parent); err != nil {
		return err
	}
	release, err := kernel.EnsureDirectory(s.root, "kernels")
	if err != nil {
		return err
	}
	defer release()
	destination, err := kernel.RecordDirectory(s.root, prepared.Record)
	if err != nil {
		return err
	}
	if _, err = os.Lstat(destination); !os.IsNotExist(err) {
		return errors.New("kernel ID already exists; never overwrite it")
	}
	if err = os.Rename(prepared.Directory, destination); err != nil {
		return err
	}
	defer func() {
		if resultErr != nil {
			if err := kernel.RemoveOwnedTree(destination); err != nil {
				resultErr = err
			}
		}
	}()
	pinFiles, err := kernel.PinFiles(destination, prepared.Record.Files)
	if err != nil {
		return err
	}
	defer pinFiles()
	if err = kernel.VerifyFiles(destination, prepared.Record.Files); err != nil {
		return err
	}
	bytes, _ := json.Marshal(prepared.Record)
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err = tx.Exec("INSERT INTO kernels(id,version,source,status) VALUES(?,?,?,'verified')", prepared.Record.ID, prepared.Record.Version, "fingerprint-chromium"); err != nil {
		return err
	}
	if _, err = tx.Exec("INSERT INTO kernel_evidence(kernel_id,record_json) VALUES(?,?)", prepared.Record.ID, string(bytes)); err != nil {
		return err
	}
	committed := *operation
	committed.State = "completed"
	committed.Stage = "completed"
	committed.CompletedIDs = []string{prepared.Record.ID}
	operationJSON, _ := json.Marshal(committed)
	if _, err = tx.Exec("UPDATE operations SET result_json=? WHERE id=?", string(operationJSON), operation.ID); err != nil {
		return err
	}
	if _, err = tx.Exec("INSERT INTO activities(id,created_at,action,target,detail) VALUES(?,?,?,?,?)", id(), timestamp(), "安装内核", prepared.Record.Version, "精确归档、全部文件摘要与真实隔离诊断已核对；正常环境启停尚未接入。"); err != nil {
		return err
	}
	if s.options.BeforeCommit != nil {
		if err = s.options.BeforeCommit(); err != nil {
			return err
		}
	}
	if err = tx.Commit(); err != nil {
		return err
	}
	*operation = committed
	return nil
}

func (s *Service) deleteKernel(record kernel.Record, operation *Operation) error {
	var count int
	if err := s.db.QueryRow("SELECT (SELECT COUNT(*) FROM fingerprints WHERE kernel_id=?)+(SELECT COUNT(*) FROM fingerprint_revisions WHERE kernel_id=?)", record.ID, record.ID).Scan(&count); err != nil {
		return err
	}
	if count > 0 {
		return &kernel.Problem{Code: "PROFILE_BUSY", Reason: "kernel-in-use", Message: "任务期间该构建已被引用，未删除。", Retryable: false}
	}
	directory, err := kernel.RecordDirectory(s.root, record)
	if err != nil {
		return err
	}
	if _, err = s.db.Exec("UPDATE kernels SET status='missing' WHERE id=?", record.ID); err != nil {
		return err
	}
	if err = kernel.RemoveOwnedTree(directory); err != nil {
		return err
	}
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err = tx.Exec("DELETE FROM kernel_evidence WHERE kernel_id=?", record.ID); err != nil {
		return err
	}
	if _, err = tx.Exec("DELETE FROM kernels WHERE id=?", record.ID); err != nil {
		return err
	}
	if err = tx.Commit(); err != nil {
		return err
	}
	operation.CompletedIDs = []string{record.ID}
	return nil
}

func (s *Service) recoverKernelOperations() error {
	rows, err := s.db.Query("SELECT result_json FROM operations WHERE json_extract(result_json,'$.kind') LIKE 'kernel-%'")
	if err != nil {
		return err
	}
	operations := []Operation{}
	for rows.Next() {
		var text string
		if err = rows.Scan(&text); err != nil {
			rows.Close()
			return err
		}
		var operation Operation
		if err = json.Unmarshal([]byte(text), &operation); err != nil {
			rows.Close()
			return err
		}
		if strings.HasPrefix(operation.Kind, "kernel-") {
			operations = append(operations, operation)
		}
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	for _, operation := range operations {
		if parsed, err := uuid.Parse(operation.ResourceKey); err != nil || parsed.String() != operation.ResourceKey {
			return errors.New("invalid internal staging resource key")
		}
		if err = kernel.RemoveOwnedTree(filepath.Join(s.root, "staging", "kernel-"+operation.ResourceKey)); err != nil {
			return err
		}
		if operation.Kind == "kernel-install" && operation.KernelID != "" {
			var count int
			if err = s.db.QueryRow("SELECT COUNT(*) FROM kernel_evidence WHERE kernel_id=?", operation.KernelID).Scan(&count); err != nil {
				return err
			}
			if count == 0 {
				if parsed, err := uuid.Parse(operation.KernelID); err != nil || parsed.String() != operation.KernelID {
					return errors.New("invalid recovery kernel ID")
				}
				if err = kernel.RemoveOwnedTree(filepath.Join(s.root, "kernels", operation.KernelID)); err != nil {
					return err
				}
			}
		}
		if operation.State == "completed" || operation.State == "failed" || operation.State == "cancelled" {
			continue
		}
		operation.State = "failed"
		operation.Stage = "interrupted"
		operation.Error = &Error{Code: "OPERATION_CANCELLED", Message: "上次内核任务被中断，本次临时资源已清理，未自动重装或改动既有引用。", Retryable: true, Details: map[string]any{"reason": "interrupted"}}
		if err = s.storeOperation(operation); err != nil {
			return err
		}
	}
	return nil
}
