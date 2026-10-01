package workspace

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"reflect"
	"sort"
	"strings"
	"time"

	"github.com/axgiroud312-byte/prism-local-browser/internal/kernel"
)

type runtimeSlot struct {
	session    RuntimeSession
	start      Operation
	stop       *Operation
	process    RuntimeProcess
	cancel     context.CancelFunc
	launchDone chan struct{}
}

func (s *Service) runtimeOwnsProfileUse(environmentID string) bool {
	slot := s.runtimeSlots[environmentID]
	return slot != nil && (slot.process != nil || slot.session.State == "starting" || slot.session.State == "stopping")
}

func runtimeSignature(method string, input runtimeRequest) string {
	encoded, _ := json.Marshal(input)
	digest := sha256.Sum256(append([]byte(method), encoded...))
	return hex.EncodeToString(digest[:])
}

func (s *Service) priorRuntime(method string, input runtimeRequest) (Result, bool) {
	var signature, encoded string
	err := s.db.QueryRow("SELECT signature,result_json FROM requests WHERE id=?", input.RequestID).Scan(&signature, &encoded)
	if err == sql.ErrNoRows {
		return Result{}, false
	}
	if err != nil {
		return storageFailure(err), true
	}
	if signature != runtimeSignature(method, input) {
		return failure("REQUEST_ID_REUSED", "同一请求标识不能用于不同操作。", false), true
	}
	var result Result
	if json.Unmarshal([]byte(encoded), &result) != nil {
		return failure("STORAGE_READ_FAILED", "运行请求记录无法读取，未重新启动。", true), true
	}
	return result, true
}

// Acceptance and request deduplication are persisted together before a worker
// can create a process. An accepted operation is never a running assertion.
func (s *Service) acceptRuntime(method string, input runtimeRequest, operation Operation) Result {
	result := success(map[string]any{"status": "accepted", "operation": operation}, operation.ID)
	opJSON, _ := json.Marshal(operation)
	resultJSON, _ := json.Marshal(result)
	tx, err := s.db.Begin()
	if err != nil {
		return storageFailure(err)
	}
	defer tx.Rollback()
	if _, err = tx.Exec("INSERT INTO operations(id,result_json) VALUES(?,?) ON CONFLICT(id) DO NOTHING", operation.ID, string(opJSON)); err != nil {
		return storageFailure(err)
	}
	if _, err = tx.Exec("INSERT INTO requests(id,signature,result_json) VALUES(?,?,?)", input.RequestID, runtimeSignature(method, input), string(resultJSON)); err != nil {
		return storageFailure(err)
	}
	if err = tx.Commit(); err != nil {
		return storageFailure(err)
	}
	return result
}

func (s *Service) runtimeCall(request Request) Result {
	if request.Method == "Runtime.Inspect" {
		var input struct {
			IDs []string `json:"ids"`
		}
		if decode(request.Payload, &input) != nil {
			return failure("VALIDATION_FAILED", "只接受环境标识查询，不接受客户端会话状态。", false)
		}
		wanted := map[string]bool{}
		for _, environmentID := range input.IDs {
			wanted[environmentID] = true
		}
		ids := []string{}
		for environmentID := range s.runtimeSlots {
			if len(wanted) == 0 || wanted[environmentID] {
				ids = append(ids, environmentID)
			}
		}
		sort.Strings(ids)
		sessions := []RuntimeSession{}
		for _, environmentID := range ids {
			sessions = append(sessions, s.runtimeSlots[environmentID].session)
		}
		return success(sessions, "")
	}
	var input runtimeRequest
	if decode(request.Payload, &input) != nil || input.EnvironmentID == "" || strings.TrimSpace(input.RequestID) == "" || len(input.RequestID) > 128 {
		return failure("VALIDATION_FAILED", "运行请求仅接受环境标识和请求标识；不能覆盖路径、参数或状态。", false)
	}
	if request.Method == "Runtime.Start" && input.NetworkPolicy != "direct" {
		return failure("VALIDATION_FAILED", "当前仅支持用户明确确认的本机直连；未启动。", false)
	}
	if request.Method == "Runtime.Stop" && input.NetworkPolicy != "" {
		return failure("VALIDATION_FAILED", "停止请求不接受网络或启动覆盖项。", false)
	}
	if result, exists := s.priorRuntime(request.Method, input); exists {
		return result
	}
	if request.Method == "Runtime.Stop" {
		return s.stopRuntime(input)
	}

	environment, revision, profileID, err := s.readEnvironment(input.EnvironmentID)
	if err != nil {
		return failure("NOT_FOUND", "环境无法读取，未启动。", true)
	}
	if environment.ProxyID != "" {
		return failure("PROXY_UNSUPPORTED", "此阶段尚未接入已绑定代理，环境未启动；不会绕过代理改为直连。", false)
	}
	if slot := s.runtimeSlots[input.EnvironmentID]; slot != nil && (slot.process != nil || slot.session.State == "starting" || slot.session.State == "stopping") {
		if slot.session.State == "error" {
			return failure("PROFILE_BUSY", "上次进程树退出尚未确认，请重试停止后再启动。", true)
		}
		if slot.session.State == "stopping" {
			return failure("PROFILE_BUSY", "本次会话仍在停止，请等待退出后重新启动。", true)
		}
		return s.acceptRuntime(request.Method, input, slot.start)
	}
	if s.profileUses[input.EnvironmentID] {
		return failure("PROFILE_BUSY", "环境正在维护，不能同时启动。", true)
	}
	if environment.CoreID == PendingKernelID {
		return failure("KERNEL_MISSING", "环境尚未绑定已核验的精确内核，未启动。", true)
	}
	profile, err := readProfileFrom(s.db, profileID)
	if err != nil {
		return failure("KERNEL_INTEGRITY_FAILED", "固定档案与内核证据无法核对，未启动或换版本。", true)
	}
	record, err := savedKernelFrom(s.db, environment.CoreID, true)
	if err != nil {
		return kernelFailure(err)
	}
	parameters, err := kernel.CompileFingerprint(record, profileInput(profile.Profile))
	if err != nil {
		return kernelFailure(err)
	}
	if !profileMatchesConfiguration(profile.Profile, environment.Configuration) || !reflect.DeepEqual(parameters, profile.Profile.Parameters) {
		return failure("KERNEL_INTEGRITY_FAILED", "保存档案不是对应构建的固定输入，未启动。", true)
	}
	var ref string
	if err = s.db.QueryRow("SELECT user_data_ref FROM environments WHERE id=?", environment.ID).Scan(&ref); err != nil {
		return failure("STORAGE_READ_FAILED", "环境数据引用无法读取，未启动。", true)
	}
	expected, err := dataReference(environment.ID)
	if err != nil || expected != ref {
		return failure("PATH_OUTSIDE_ROOT", "环境数据引用不在受管理的独立目录，未启动。", false)
	}
	sessionID := id()
	operation := Operation{ID: id(), Kind: "runtime-start", State: "accepted", Stage: "queued", Total: 1, CompletedIDs: []string{}, EnvironmentID: environment.ID, SessionID: sessionID, KernelID: record.ID}
	result := s.acceptRuntime(request.Method, input, operation)
	if !result.OK {
		return result
	}
	ctx, cancel := context.WithCancel(context.Background())
	slot := &runtimeSlot{session: RuntimeSession{Mode: "native", EnvironmentID: environment.ID, SessionID: sessionID, OperationID: operation.ID, State: "starting", Revision: revision, FingerprintRevision: profile.Profile.ConfigRevision, KernelID: record.ID, UserDataRef: ref, NetworkPolicy: "direct"}, start: operation, cancel: cancel, launchDone: make(chan struct{})}
	s.runtimeSlots[environment.ID] = slot
	s.profileUses[environment.ID] = true
	s.workers.Add(1)
	go s.launchRuntime(ctx, slot, RuntimeLaunch{Root: s.root, EnvironmentID: environment.ID, SessionID: sessionID, DataReference: ref, Kernel: record, Profile: profile.Profile, Configuration: environment.Configuration})
	return result
}

func launchManagedRuntime(ctx context.Context, input RuntimeLaunch) (RuntimeProcess, error) {
	process, err := kernel.LaunchManagedProfile(ctx, input.Root, input.Kernel, kernel.ManagedProfile{EnvironmentID: input.EnvironmentID, SessionID: input.SessionID, UserDataRef: input.DataReference, Fingerprint: profileInput(input.Profile), Width: input.Configuration.Width, Height: input.Configuration.Height, RestoreTabs: input.Configuration.RestoreTabs, URLs: strings.Fields(input.Configuration.URLs)})
	// A typed nil pointer becomes a non-nil interface. Normalize it before
	// failure cleanup; missing kernels/locked directories must never panic.
	if process == nil {
		return nil, err
	}
	return process, err
}

func (s *Service) runtimeStage(slot *runtimeSlot, stage string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	slot.start.State, slot.start.Stage = "running", stage
	if s.storeOperation(slot.start) != nil {
		slot.cancel()
	}
}

func (s *Service) launchRuntime(ctx context.Context, slot *runtimeSlot, input RuntimeLaunch) {
	defer s.workers.Done()
	defer close(slot.launchDone)
	select {
	case s.startGate <- struct{}{}:
		defer func() { <-s.startGate }()
	case <-ctx.Done():
		s.finishRuntimeStart(slot, nil, ctx.Err())
		return
	}
	s.runtimeStage(slot, "verifying-and-starting")
	launcher := s.options.LaunchRuntime
	if launcher == nil {
		launcher = launchManagedRuntime
	}
	startup, cancel := context.WithTimeout(ctx, 45*time.Second)
	process, err := launcher(startup, input)
	cancel()
	if err == nil && process == nil {
		err = errors.New("launcher did not return a controlled process")
	}
	if err == nil && !process.Alive() {
		err = errors.New("browser root exited before readiness")
	}
	if err == nil {
		if _, parseErr := time.Parse(time.RFC3339Nano, process.CreatedAt()); process.PID() < 1 || parseErr != nil {
			err = errors.New("controlled process identity unavailable")
		}
	}
	if err == nil {
		select {
		case <-process.Done():
			err = errors.New("process exited before readiness")
		default:
		}
	}
	if ctx.Err() != nil {
		err = ctx.Err()
	}
	s.finishRuntimeStart(slot, process, err)
}

func runtimeError(err error) *Error {
	var problem *kernel.Problem
	if errors.As(err, &problem) {
		return kernelFailure(err).Error
	}
	if errors.Is(err, context.Canceled) {
		return &Error{Code: "OPERATION_CANCELLED", Message: "本次启动已取消，原浏览数据保持。", Retryable: true}
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return &Error{Code: "PROCESS_READY_TIMEOUT", Message: "进程或控制通道未在时限内就绪，未报告运行中。", Retryable: true}
	}
	return &Error{Code: "PROCESS_START_FAILED", Message: "真实进程或安全控制通道无法启动，原数据保持。", Retryable: true}
}

func (s *Service) finishRuntimeStart(slot *runtimeSlot, process RuntimeProcess, taskErr error) {
	s.mu.Lock()
	if s.closed || slot.session.State == "stopping" {
		taskErr = context.Canceled
	}
	if taskErr == nil && !process.Alive() {
		taskErr = errors.New("browser root exited before readiness publication")
	}
	if taskErr == nil {
		slot.start.State, slot.start.Stage, slot.start.CompletedIDs = "completed", "ready", []string{slot.session.EnvironmentID}
		if err := s.storeOperation(slot.start); err != nil {
			taskErr = err
		}
	}
	if taskErr == nil {
		slot.process = process
		slot.session.State, slot.session.PID, slot.session.ProcessCreatedAt, slot.session.StartedAt = "running", process.PID(), process.CreatedAt(), timestamp()
		s.workers.Add(1)
		s.mu.Unlock()
		go s.watchRuntime(slot, process)
		return
	}
	s.mu.Unlock()
	var cleanupErr error
	if process != nil {
		cleanupErr = process.Close()
		select {
		case <-process.Done():
			cleanupErr = nil
		default:
			if cleanupErr == nil {
				cleanupErr = errors.New("owned process exit not confirmed")
			}
		}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	slot.start.State, slot.start.Stage, slot.start.Error = "failed", "failed", runtimeError(taskErr)
	if errors.Is(taskErr, context.Canceled) {
		slot.start.State, slot.start.Stage, slot.start.CancelRequested = "cancelled", "cancelled", true
	}
	_ = s.storeOperation(slot.start)
	if cleanupErr != nil {
		// Failed cleanup must keep its process identity and busy lease. Do not
		// permit an overlapping writer just because startup never became ready.
		slot.process = process
		slot.session.State, slot.session.PID, slot.session.ProcessCreatedAt = "error", process.PID(), process.CreatedAt()
		slot.session.Error = &Error{Code: "PROCESS_STOP_TIMEOUT", Message: "启动失败后本次进程树退出尚未确认，目录与档案锁仍保留；可重试停止。", Retryable: true}
		s.workers.Add(1)
		go s.watchRuntime(slot, process)
		return
	}
	if slot.session.State != "stopping" {
		slot.session.State, slot.session.Error = "error", slot.start.Error
	}
	delete(s.profileUses, slot.session.EnvironmentID)
}

func (s *Service) watchRuntime(slot *runtimeSlot, process RuntimeProcess) {
	defer s.workers.Done()
	<-process.Done()
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.runtimeSlots[slot.session.EnvironmentID] != slot {
		return
	}
	slot.process = nil
	slot.session.PID = 0
	if slot.session.State != "stopping" {
		slot.session.State, slot.session.Error = "ready", nil
	}
	delete(s.profileUses, slot.session.EnvironmentID)
}

func (s *Service) stopRuntime(input runtimeRequest) Result {
	if _, _, _, err := s.readEnvironment(input.EnvironmentID); err != nil {
		return failure("NOT_FOUND", "所选环境不存在，没有停止其他进程。", false)
	}
	slot := s.runtimeSlots[input.EnvironmentID]
	if slot != nil && slot.stop != nil && (slot.stop.State == "accepted" || slot.stop.State == "running") {
		return s.acceptRuntime("Runtime.Stop", input, *slot.stop)
	}
	operation := Operation{ID: id(), Kind: "runtime-stop", State: "accepted", Stage: "closing", Total: 1, CompletedIDs: []string{}, EnvironmentID: input.EnvironmentID}
	if slot == nil || (slot.process == nil && slot.session.State != "starting" && slot.session.State != "stopping") {
		operation.State, operation.Stage, operation.CompletedIDs = "completed", "no-controlled-session", []string{input.EnvironmentID}
		result := s.acceptRuntime("Runtime.Stop", input, operation)
		if result.OK && slot != nil {
			slot.session.State, slot.session.Error = "ready", nil
		}
		return result
	}
	operation.SessionID, operation.KernelID = slot.session.SessionID, slot.session.KernelID
	result := s.acceptRuntime("Runtime.Stop", input, operation)
	if !result.OK {
		return result
	}
	slot.stop = &operation
	slot.session.State = "stopping"
	slot.cancel()
	s.workers.Add(1)
	go s.closeRuntime(slot)
	return result
}

func (s *Service) closeRuntime(slot *runtimeSlot) {
	defer s.workers.Done()
	<-slot.launchDone
	s.mu.Lock()
	process := slot.process
	s.mu.Unlock()
	var taskErr error
	if process != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		taskErr = process.Stop(ctx)
		cancel()
		select {
		case <-process.Done():
			taskErr = nil
		default:
			if taskErr == nil {
				taskErr = errors.New("owned process exit not confirmed")
			}
		}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if taskErr != nil {
		slot.stop.State, slot.stop.Stage = "failed", "close-timeout"
		slot.stop.Error = &Error{Code: "PROCESS_STOP_TIMEOUT", Message: "本次会话尚未确认退出，仍保留目录和档案锁；没有强制结束其他进程。可重试停止。", Retryable: true}
		var problem *kernel.Problem
		if errors.As(taskErr, &problem) {
			slot.stop.Error = kernelFailure(taskErr).Error
			if problem.Code == "CONTROL_CHANNEL_LOST" {
				slot.stop.Stage = "control-lost"
			}
		}
		slot.session.State, slot.session.Error = "error", slot.stop.Error
	} else {
		slot.stop.State, slot.stop.Stage, slot.stop.CompletedIDs = "completed", "exited", []string{slot.session.EnvironmentID}
		slot.session.State, slot.session.Error, slot.session.PID = "ready", nil, 0
		slot.process = nil
		delete(s.profileUses, slot.session.EnvironmentID)
	}
	if err := s.storeOperation(*slot.stop); err != nil {
		slot.stop.State, slot.stop.Error = "failed", storageFailure(err).Error
		slot.session.State, slot.session.Error = "error", slot.stop.Error
	}
}

func (s *Service) cancelRuntimeOperation(operation Operation) Result {
	if operation.Kind != "runtime-start" || (operation.State != "accepted" && operation.State != "running") {
		return success(operation, operation.ID)
	}
	slot := s.runtimeSlots[operation.EnvironmentID]
	if slot != nil && slot.start.ID == operation.ID {
		slot.cancel()
		slot.start.CancelRequested = true
		if err := s.storeOperation(slot.start); err != nil {
			return storageFailure(err)
		}
		return success(slot.start, operation.ID)
	}
	return success(operation, operation.ID)
}
