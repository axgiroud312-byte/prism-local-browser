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
	"github.com/axgiroud312-byte/prism-local-browser/internal/proxy"
)

type runtimeSlot struct {
	session       RuntimeSession
	start         Operation
	stop          *Operation
	process       RuntimeProcess
	cancel        context.CancelFunc
	launchDone    chan struct{}
	reconcile     *Operation
	cleanupIntent bool
	startupError  *Error
}

func (s *Service) runtimeOwnsProfileUse(environmentID string) bool {
	if s.hasPendingNetwork(environmentID) {
		return true
	}
	if s.cookieTasks[environmentID] != nil {
		return true
	}
	slot := s.runtimeSlots[environmentID]
	return slot != nil && (slot.process != nil || runtimeStopPending(slot) || slot.session.NeedsReconcile || slot.session.PersistencePending || slot.session.State == "starting" || slot.session.State == "stopping")
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
	return s.acceptRuntimeRecord(method, input, operation, nil)
}

func (s *Service) acceptRuntimeRecord(method string, input runtimeRequest, operation Operation, session *RuntimeSession) Result {
	result := success(map[string]any{"status": "accepted", "operation": operation}, operation.ID)
	opJSON, _ := json.Marshal(operation)
	resultJSON, _ := json.Marshal(result)
	tx, err := s.db.Begin()
	if err != nil {
		return storageFailure(err)
	}
	defer tx.Rollback()
	if session != nil {
		if err = saveRuntimeSession(tx, *session, method == "Runtime.Start"); err != nil {
			return storageFailure(err)
		}
	}
	if _, err = tx.Exec("INSERT INTO operations(id,result_json) VALUES(?,?) ON CONFLICT(id) DO NOTHING", operation.ID, string(opJSON)); err != nil {
		return storageFailure(err)
	}
	if _, err = tx.Exec("INSERT INTO requests(id,signature,result_json) VALUES(?,?,?)", input.RequestID, runtimeSignature(method, input), string(resultJSON)); err != nil {
		return storageFailure(err)
	}
	if s.options.BeforeCommit != nil {
		if err = s.options.BeforeCommit(); err != nil {
			return storageFailure(err)
		}
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
			sessions = append(sessions, runtimeSessionView(s.runtimeSlots[environmentID]))
		}
		return success(sessions, "")
	}
	var input runtimeRequest
	if decode(request.Payload, &input) != nil || input.EnvironmentID == "" || strings.TrimSpace(input.RequestID) == "" || len(input.RequestID) > 128 {
		return failure("VALIDATION_FAILED", "运行请求仅接受环境标识和请求标识；不能覆盖路径、参数或状态。", false)
	}
	if request.Method == "Runtime.Start" && input.NetworkPolicy != "direct" && input.NetworkPolicy != "proxy" {
		return failure("VALIDATION_FAILED", "启动需明确选择直连或已绑定代理策略；不能自动回退。", false)
	}
	if request.Method == "Runtime.Stop" && input.NetworkPolicy != "" {
		return failure("VALIDATION_FAILED", "停止请求不接受网络或启动覆盖项。", false)
	}
	if (request.Method == "Runtime.Start" || request.Method == "Runtime.Stop") && input.SessionID != "" {
		return failure("VALIDATION_FAILED", "普通启停不接受客户端会话身份覆盖。", false)
	}
	if (request.Method == "Runtime.ForceStop" || request.Method == "Runtime.Reconcile") && (input.NetworkPolicy != "" || input.SessionID == "") {
		return failure("VALIDATION_FAILED", "会话恢复操作必须指定当前会话标识，不能覆盖网络、路径或PID。", false)
	}
	if result, exists := s.priorRuntime(request.Method, input); exists {
		return result
	}
	if pending := s.runtimePending[input.EnvironmentID]; pending != nil {
		return failure("STORAGE_WRITE_FAILED", "前一个实际运行结果尚未保存，未受理新操作；修复存储后重新读取并重试，不会重复外部启停。", true)
	}
	if request.Method == "Runtime.Stop" {
		return s.stopRuntime(input)
	}
	if request.Method == "Runtime.ForceStop" {
		return s.forceStopRuntime(input)
	}
	if request.Method == "Runtime.Reconcile" {
		return s.reconcileRuntime(input)
	}

	if input.Purpose != "" && input.Purpose != "cookie-import" || input.Purpose == "cookie-import" && input.ExpectedRevision < 1 {
		return failure("VALIDATION_FAILED", "启动用途或Cookie目标修订无效，未创建会话。", false)
	}
	environment, revision, profileID, err := s.readEnvironment(input.EnvironmentID)
	if err != nil {
		return failure("NOT_FOUND", "环境无法读取，未启动。", true)
	}
	if input.ExpectedRevision < 0 || input.ExpectedRevision > maxSafeInteger {
		return failure("VALIDATION_FAILED", "启动修订应为精确正整数或省略，不接受状态覆盖。", false)
	}
	if input.ExpectedRevision > 0 && revision != input.ExpectedRevision {
		return failure("REVISION_CONFLICT", "启动预览后目标环境修订已变，未按旧配置或网络策略启动；请重新读取。", true)
	}
	if s.cookieTasks[input.EnvironmentID] != nil {
		return failure("PROFILE_BUSY", "前一次Cookie操作/观测结果尚未保存，不能替换它的会话。", true)
	}
	var proxyRecord *ProxyView
	var proxyRef string
	if environment.ProxyID != "" {
		if input.NetworkPolicy != "proxy" {
			return failure("PROXY_POLICY_MISMATCH", "环境已绑定代理，拒绝直连启动；请明确使用已绑定代理。", false)
		}
		record, ref, err := s.savedProxy(environment.ProxyID)
		if err != nil {
			return failure("PROXY_UNAVAILABLE", "已绑定代理配置不存在或无法读取，未启动或改为直连。", true)
		}
		if s.proxyChecks[record.ID] != nil && s.proxyPending[s.proxyChecks[record.ID].operation.ID] != nil {
			return failure("STORAGE_WRITE_FAILED", "该代理前一个观测结果尚未保存，未启动；请先修复存储。", true)
		}
		proxyRecord, proxyRef = &record, ref
	} else if input.NetworkPolicy != "direct" {
		return failure("PROXY_POLICY_MISMATCH", "此环境没有绑定代理，不能将代理启动自动降为直连。", false)
	}
	if slot := s.runtimeSlots[input.EnvironmentID]; runtimeStopPending(slot) {
		return failure("PROFILE_BUSY", "本次停止任务尚未确认并保存终态，请等待后重新启动。", true)
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
	operation := Operation{ID: id(), Kind: "runtime-start", State: "accepted", Stage: "queued", Total: 1, CompletedIDs: []string{}, EnvironmentID: environment.ID, SessionID: sessionID, KernelID: record.ID, ProxyID: environment.ProxyID}
	session := RuntimeSession{Mode: "native", EnvironmentID: environment.ID, SessionID: sessionID, OperationID: operation.ID, State: "starting", Revision: revision, FingerprintRevision: profile.Profile.ConfigRevision, KernelID: record.ID, UserDataRef: ref, NetworkPolicy: input.NetworkPolicy, ResourceVersion: kernel.ManagedRuntimeVersion, LaunchStage: "queued", NextAction: "正在核验固定档案、网络通道与实际进程，受理不代表已经运行。"}
	if proxyRecord != nil {
		session.ProxyID, session.ProxyRevision, session.ProxyChannelID = proxyRecord.ID, proxyRecord.Revision, id()
	}
	result := s.acceptRuntimeRecord(request.Method, input, operation, &session)
	if !result.OK {
		return result
	}
	ctx, cancel := context.WithCancel(context.Background())
	slot := &runtimeSlot{session: session, start: operation, cancel: cancel, launchDone: make(chan struct{})}
	s.runtimeSlots[environment.ID] = slot
	s.profileUses[environment.ID] = true
	s.workers.Add(1)
	launchConfig := environment.Configuration
	// Launch-only: leave saved tabs/URLs, identity and network policy unchanged.
	if input.Purpose == "cookie-import" {
		launchConfig.RestoreTabs, launchConfig.URLs = false, ""
	}
	go s.launchRuntime(ctx, slot, RuntimeLaunch{Root: s.root, EnvironmentID: environment.ID, SessionID: sessionID, DataReference: ref, Kernel: record, Profile: profile.Profile, Configuration: launchConfig, Proxy: proxyRecord, ProxyCredentialRef: proxyRef, OnCreated: func(pid int, createdAt string) error {
		s.mu.Lock()
		defer s.mu.Unlock()
		if s.runtimeSlots[environment.ID] != slot || s.closed || s.closeRequested.Load() || slot.session.State == "stopping" {
			return context.Canceled
		}
		slot.session.PID, slot.session.RootPID, slot.session.ProcessCreatedAt, slot.session.LaunchStage = pid, pid, createdAt, "process-created"
		if createdAt == "" {
			slot.session.LaunchStage = "identity-unconfirmed"
		}
		return s.persistRuntime(slot, &slot.start, "")
	}})
	return result
}

func launchManagedRuntime(ctx context.Context, input RuntimeLaunch) (RuntimeProcess, error) {
	if input.Configuration.ProxyID != "" && input.Network == nil {
		return nil, &proxy.CheckError{Code: "PROXY_BRIDGE_UNAVAILABLE", Message: "代理通道没有就绪，未用直连参数启动。", Retryable: true}
	}
	process, err := kernel.LaunchManagedProfile(ctx, input.Root, input.Kernel, kernel.ManagedProfile{EnvironmentID: input.EnvironmentID, SessionID: input.SessionID, UserDataRef: input.DataReference, Fingerprint: profileInput(input.Profile), Width: input.Configuration.Width, Height: input.Configuration.Height, RestoreTabs: input.Configuration.RestoreTabs, URLs: strings.Fields(input.Configuration.URLs), OnCreated: input.OnCreated, Network: input.Network})
	// A typed nil pointer becomes a non-nil interface. Normalize it before
	// failure cleanup; missing kernels/locked directories must never panic.
	if process == nil {
		return nil, err
	}
	return process, err
}

func (s *Service) runtimeStage(slot *runtimeSlot, stage string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	slot.start.State, slot.start.Stage = "running", stage
	slot.session.LaunchStage = stage
	if err := s.persistRuntime(slot, &slot.start, ""); err != nil {
		slot.cancel()
		return err
	}
	return nil
}

func (s *Service) launchRuntime(ctx context.Context, slot *runtimeSlot, input RuntimeLaunch) {
	defer s.workers.Done()
	defer close(slot.launchDone)
	select {
	case s.startGate <- struct{}{}:
		defer func() { <-s.startGate }()
	case <-ctx.Done():
		s.finishRuntimeStart(slot, nil, nil, ctx.Err())
		return
	}
	var network RuntimeProxyChannel
	if input.Proxy != nil {
		var err error
		network, err = s.prepareRuntimeNetwork(ctx, slot, input)
		if err != nil {
			s.finishRuntimeStart(slot, nil, network, err)
			return
		}
		input.Network = network
	}
	if err := s.runtimeStage(slot, "verifying-and-starting"); err != nil {
		s.finishRuntimeStart(slot, nil, network, err)
		return
	}
	launcher := s.options.LaunchRuntime
	if launcher == nil {
		launcher = launchManagedRuntime
	}
	if err := s.claimRuntimeData(ctx, slot); err != nil {
		s.finishRuntimeStart(slot, nil, network, err)
		return
	}
	startup, cancel := context.WithTimeout(ctx, 45*time.Second)
	process, err := launcher(startup, input)
	if network != nil {
		if process != nil {
			process = ownRuntimeNetwork(process, network)
		}
		if fault := network.Fault(); fault != nil {
			err = errors.Join(fault, err)
		}
	}
	deadlineErr := startup.Err()
	cancel()
	if errors.Is(deadlineErr, context.DeadlineExceeded) {
		err = errors.Join(err, deadlineErr)
	}
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
		err = errors.Join(err, ctx.Err())
	}
	s.finishRuntimeStart(slot, process, network, err)
}

func runtimeError(err error) *Error {
	if integrity := runtimeIntegrityProblem(err); integrity != nil {
		return kernelFailure(integrity).Error
	}
	var persistence *runtimePersistenceFailure
	if errors.As(err, &persistence) {
		return storageFailure(err).Error
	}
	var networkError *proxy.CheckError
	if errors.As(err, &networkError) {
		return &Error{Code: networkError.Code, Message: networkError.Message, Retryable: networkError.Retryable}
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return &Error{Code: "PROCESS_READY_TIMEOUT", Message: "进程或控制通道未在时限内就绪，未报告运行中。", Retryable: true}
	}
	if errors.Is(err, context.Canceled) {
		return &Error{Code: "OPERATION_CANCELLED", Message: "本次启动已取消，原浏览数据保持。", Retryable: true}
	}
	var problem *kernel.Problem
	_ = errors.As(err, &problem)
	if problem != nil {
		return kernelFailure(err).Error
	}
	return &Error{Code: "PROCESS_START_FAILED", Message: "真实进程或安全控制通道无法启动，原数据保持。", Retryable: true}
}

// errors.As returns only the first matching Problem in errors.Join. Integrity
// facts anywhere in the tree must survive cancellation and cleanup failures.
func runtimeIntegrityProblem(err error) *kernel.Problem {
	if problem, ok := err.(*kernel.Problem); ok && (problem.Code == "KERNEL_INTEGRITY_FAILED" || problem.Code == "PATH_OUTSIDE_ROOT") {
		return problem
	}
	if joined, ok := err.(interface{ Unwrap() []error }); ok {
		for _, cause := range joined.Unwrap() {
			if found := runtimeIntegrityProblem(cause); found != nil {
				return found
			}
		}
	} else if wrapped, ok := err.(interface{ Unwrap() error }); ok {
		return runtimeIntegrityProblem(wrapped.Unwrap())
	}
	return nil
}

func startupNetworkSnapshot(process RuntimeProcess, channel RuntimeProxyChannel) kernel.RuntimeSnapshot {
	snapshot := kernel.RuntimeSnapshot{ResourcesExited: process == nil && channel == nil}
	if process != nil {
		snapshot = process.Snapshot()
	}
	if channel != nil && snapshot.ProxyError == nil {
		snapshot.ProxyError = channel.Fault()
	}
	return snapshot
}

func runtimeStopPending(slot *runtimeSlot) bool {
	return slot != nil && slot.stop != nil && (slot.stop.State == "accepted" || slot.stop.State == "running")
}

func (s *Service) finishRuntimeStart(slot *runtimeSlot, process RuntimeProcess, channel RuntimeProxyChannel, taskErr error) {
	// A bridge can exist even if CreateProcess never ran. Preserve its cleanup
	// owner just like an already-created Job; nil PID is not resource exit.
	if process == nil && channel != nil {
		process = ownRuntimeNetwork(nil, channel)
	}
	s.mu.Lock()
	if s.runtimeSlots[slot.session.EnvironmentID] != slot {
		s.mu.Unlock()
		if process != nil {
			_ = process.Close()
		}
		return
	}
	// Re-read the latched channel error at readiness publication, not only
	// immediately after launch. A bound channel may fail during startup.
	if snapshot := startupNetworkSnapshot(process, channel); s.observeRuntimeNetworkFault(slot, snapshot) {
		taskErr = errors.Join(snapshot.ProxyError, taskErr)
	}
	if s.closed || s.closeRequested.Load() || slot.session.State == "stopping" {
		taskErr = errors.Join(taskErr, context.Canceled)
	}
	if taskErr == nil && (process == nil || !process.Alive()) {
		taskErr = errors.New("browser root exited before readiness publication")
	}
	if taskErr == nil {
		slot.start.State, slot.start.Stage, slot.start.CompletedIDs = "completed", "ready", []string{slot.session.EnvironmentID}
		slot.session.State, slot.session.PID, slot.session.RootPID, slot.session.ProcessCreatedAt, slot.session.StartedAt = "running", process.PID(), process.PID(), process.CreatedAt(), timestamp()
		slot.session.CanControl, slot.session.NextAction = true, "可正常关闭；设备、代理和数据替换需确认本次进程树退出后执行。"
		slot.session.LaunchStage = "ready"
		if err := s.persistRuntime(slot, &slot.start, "浏览器就绪"); err != nil {
			taskErr = err
			// A storage retry must not publish running after this launch has
			// already been selected for failure cleanup.
			slot.session.State, slot.session.CanControl = "starting", false
			slot.start.State, slot.start.Stage, slot.start.Error = "failed", "storage-pending", runtimeError(err)
			if pending := s.runtimePending[slot.session.EnvironmentID]; pending != nil {
				pending.Session = slot.session
				pending.Session.PersistencePending = false
				wanted := slot.start
				wanted.PersistencePending = false
				pending.Operations[slot.start.ID] = wanted
				if last := len(pending.Events) - 1; last >= 0 && pending.Events[last].Action == "浏览器就绪" {
					pending.Events = pending.Events[:last]
				}
			}
		}
	}
	if taskErr == nil {
		slot.process = process
		s.workers.Add(1)
		s.mu.Unlock()
		go s.watchRuntime(slot, process)
		return
	}
	slot.cleanupIntent = true
	slot.startupError = runtimeError(taskErr)
	// Retain the owner before releasing the service lock for cleanup. A network
	// fault may already display "error"; pending-write flush must not treat that
	// display state plus a nil slot.process as permission to reuse the profile.
	slot.process = process
	if slot.session.NetworkPolicy == "proxy" && slot.startupError.Code == "NETWORK_PROTECTION_UNAVAILABLE" {
		slot.session.NetworkFault = &RuntimeNetworkFault{State: "network_error", Error: slot.startupError, ObservedAt: timestamp(), Containment: "stopped"}
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
	if s.runtimeSlots[slot.session.EnvironmentID] != slot {
		return
	}
	if s.observeRuntimeNetworkFault(slot, startupNetworkSnapshot(process, channel)) && runtimeIntegrityProblem(taskErr) == nil {
		// The channel can latch while cleanup is in progress. Preserve that
		// concrete cause instead of a generic launch/cleanup error.
		slot.startupError = slot.session.NetworkFault.Error
	}
	slot.start.State, slot.start.Stage, slot.start.Error = "failed", "failed", slot.startupError
	if slot.startupError.Code == "OPERATION_CANCELLED" {
		slot.start.State, slot.start.Stage, slot.start.CancelRequested = "cancelled", "cancelled", true
	}
	if cleanupErr != nil {
		// Failed cleanup must keep its process identity and busy lease. Do not
		// permit an overlapping writer just because startup never became ready.
		slot.process = process
		slot.session.State, slot.session.PID, slot.session.RootPID, slot.session.ProcessCreatedAt = "error", process.PID(), process.PID(), process.CreatedAt()
		if !runtimeHasBrowser(process) {
			slot.session.LaunchStage = "no-process-created"
		} else if process.CreatedAt() == "" {
			slot.session.LaunchStage = "identity-unconfirmed"
		}
		slot.session.Error = &Error{Code: "PROCESS_STOP_TIMEOUT", Message: "启动失败后本次进程树或代理通道资源清理尚未确认，环境占用仍保留；可重试停止。", Retryable: true}
		slot.session.CanControl, slot.session.NextAction = process.Snapshot().ControlReady, "请先尝试正常关闭本次会话；未确认退出前不能启动或替换数据。"
		_ = s.observeRuntimeNetworkFault(slot, startupNetworkSnapshot(process, channel))
		if slot.session.NetworkFault != nil {
			setRuntimeNetworkContainment(slot, "exit-unconfirmed")
		}
		_ = s.persistRuntime(slot, &slot.start, "启动失败，清理未确认")
		s.workers.Add(1)
		go s.watchRuntime(slot, process)
		return
	}
	slot.process = nil
	if slot.session.State != "stopping" {
		slot.session.State, slot.session.Error = "error", slot.startupError
	}
	if !runtimeHasBrowser(process) {
		slot.session.LaunchStage = "no-process-created"
	} else {
		slot.session.PID, slot.session.CanControl, slot.session.CanForce = 0, false, false
		slot.session.RootPID, slot.session.ProcessCreatedAt = process.PID(), process.CreatedAt()
		if process.CreatedAt() == "" {
			slot.session.LaunchStage = "identity-unconfirmed"
		}
		if snapshot := process.Snapshot(); snapshot.ExitKnown {
			code := snapshot.ExitCode
			slot.session.LastExitCode = &code
		}
	}
	slot.session.NextAction = "本次启动未完成且资源已退出，可修复所示原因后使用原档案重试。"
	if slot.startupError.Code == "NETWORK_PROTECTION_UNAVAILABLE" {
		slot.session.NextAction = "当前代理启动被安全门禁阻止，没有创建浏览器；需隔离组件实现并验证，普通重试不能解除此门禁。"
	}
	_ = s.observeRuntimeNetworkFault(slot, startupNetworkSnapshot(process, channel))
	if slot.session.NetworkFault != nil {
		setRuntimeNetworkContainment(slot, "stopped")
		slot.session.State, slot.session.Error = "error", slot.session.NetworkFault.Error
		if slot.session.NetworkFault.Error.Code != "NETWORK_PROTECTION_UNAVAILABLE" {
			slot.session.NextAction = "启动期间代理通道失败，本次资源已确认退出；修复后沿原档案新建通道并重新前检。"
		}
	}
	_ = s.persistRuntime(slot, &slot.start, "浏览器启动失败")
	if !slot.session.PersistencePending && !runtimeStopPending(slot) && s.cookieTasks[slot.session.EnvironmentID] == nil {
		s.releaseProfileUse(slot.session.EnvironmentID)
	}
}

func (s *Service) watchRuntime(slot *runtimeSlot, process RuntimeProcess) {
	defer s.workers.Done()
	ticker := time.NewTicker(250 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-process.Done():
			s.mu.Lock()
			if s.runtimeSlots[slot.session.EnvironmentID] == slot && slot.process == process {
				if slot.stop != nil && (slot.stop.State == "accepted" || slot.stop.State == "running") {
					// Keep the generation/busy reservation until the stop worker's
					// terminal result is applied; a later start cannot overtake it.
					slot.session.PID, slot.session.CanControl = 0, false
				} else {
					forced := slot.stop != nil && slot.stop.Kind == "runtime-force-stop"
					s.completeObservedExit(slot, process.Snapshot(), forced)
					_ = s.persistRuntime(slot, nil, "浏览器会话退出")
				}
			}
			s.mu.Unlock()
			return
		case <-ticker.C:
			snapshot := process.Snapshot()
			s.mu.Lock()
			if s.runtimeSlots[slot.session.EnvironmentID] == slot && slot.process == process {
				s.applyRuntimeFault(slot, snapshot)
			}
			s.mu.Unlock()
		}
	}
}

func (s *Service) stopRuntime(input runtimeRequest) Result {
	if _, _, _, err := s.readEnvironment(input.EnvironmentID); err != nil {
		return failure("NOT_FOUND", "所选环境不存在，没有停止其他进程。", false)
	}
	slot := s.runtimeSlots[input.EnvironmentID]
	if slot != nil && slot.session.NeedsReconcile {
		return failure("SESSION_IDENTITY_UNCONFIRMED", "这份会话没有当前受控Job和私有通道，不能按PID结束；请正常关闭原浏览器后核对会话。", false)
	}
	if slot != nil && slot.stop != nil && (slot.stop.State == "accepted" || slot.stop.State == "running") {
		return s.acceptRuntime("Runtime.Stop", input, *slot.stop)
	}
	operation := Operation{ID: id(), Kind: "runtime-stop", State: "accepted", Stage: "closing", Total: 1, CompletedIDs: []string{}, EnvironmentID: input.EnvironmentID}
	if slot == nil || (slot.process == nil && slot.session.State != "starting" && slot.session.State != "stopping") {
		operation.State, operation.Stage, operation.CompletedIDs = "completed", "no-controlled-session", []string{input.EnvironmentID}
		result := s.acceptRuntime("Runtime.Stop", input, operation)
		return result
	}
	operation.SessionID, operation.KernelID = slot.session.SessionID, slot.session.KernelID
	next := slot.session
	next.State, next.CanForce, next.NextAction = "stopping", false, "正在正常关闭，确认本次进程树与代理通道均已清理后释放环境占用。"
	result := s.acceptRuntimeRecord("Runtime.Stop", input, operation, &next)
	if !result.OK {
		return result
	}
	slot.stop = &operation
	s.cancelRuntimeCookies(input.EnvironmentID)
	slot.session = next
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
	if s.runtimeSlots[slot.session.EnvironmentID] != slot {
		return
	}
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
		slot.session.CanForce = runtimeHasBrowser(process)
		if process != nil {
			snapshot := process.Snapshot()
			slot.session.CanControl = snapshot.RootAlive && snapshot.ControlReady
		}
		slot.session.NextAction = "正常关闭未成功，可重试允许的正常关闭，或明确确认仅强制结束这份会话；未确认退出仍保护目录。"
		if !runtimeHasBrowser(process) {
			slot.session.NextAction = "浏览器没有创建，代理通道清理尚未确认；请重试关闭，本次环境占用继续保留。"
		}
	} else {
		slot.stop.State, slot.stop.Stage, slot.stop.CompletedIDs = "completed", "exited", []string{slot.session.EnvironmentID}
		if process != nil {
			s.completeObservedExit(slot, process.Snapshot(), false)
		} else {
			s.completeObservedExit(slot, kernel.RuntimeSnapshot{ResourcesExited: true}, false)
		}
	}
	if err := s.persistRuntime(slot, slot.stop, "正常关闭会话"); err != nil {
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
