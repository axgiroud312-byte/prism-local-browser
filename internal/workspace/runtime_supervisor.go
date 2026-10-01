package workspace

import (
	"github.com/axgiroud312-byte/prism-local-browser/internal/kernel"
)

func (s *Service) applyRuntimeFault(slot *runtimeSlot, snapshot kernel.RuntimeSnapshot) {
	// The channel/Job owner already initiated safety cleanup without waiting
	// for SQLite. A failed state write must not suppress fault observation.
	if snapshot.ProxyError != nil && (slot.session.NetworkFault == nil || snapshot.NetworkCleanupFailed && slot.session.NetworkFault.Containment == "stopping") && s.observeRuntimeNetworkFault(slot, snapshot) {
		_ = s.persistRuntime(slot, nil, "代理通道故障，停止本次会话")
		return
	}
	if slot.session.NetworkFault != nil {
		return
	}
	if slot.cleanupIntent || slot.session.PersistencePending || slot.session.State == "stopping" {
		return
	}
	var observed *Error
	if !snapshot.RootAlive {
		if snapshot.ExitKnown && snapshot.ExitCode == 0 {
			slot.session.State, slot.session.CanControl = "stopping", false
			slot.session.NextAction = "主窗口已正常退出，仍在等待本次Job子进程释放目录。"
			_ = s.persistRuntime(slot, nil, "等待浏览器子进程退出")
			return
		}
		observed = &Error{Code: "PROCESS_CRASHED", Message: "浏览器主进程意外退出；本次子进程资源尚未全部退出，仍保护目录和档案。", Retryable: true}
		if !snapshot.ExitKnown {
			observed.Code, observed.Message = "PROCESS_EXIT_UNCONFIRMED", "浏览器主进程存活无法确认；尚未释放本次目录和档案锁。"
		}
	} else if !snapshot.ControlReady {
		observed = &Error{Code: "CONTROL_CHANNEL_LOST", Message: "浏览器仍在，但私有控制通道已断开；不假报停止或重新生成身份。", Retryable: false}
	}
	if observed == nil {
		return
	}
	canControl := snapshot.RootAlive && snapshot.ControlReady
	if slot.session.Error != nil && slot.session.Error.Code == observed.Code && slot.session.CanControl == canControl {
		return
	}
	slot.session.State, slot.session.Error, slot.session.CanControl = "error", observed, snapshot.RootAlive && snapshot.ControlReady
	if snapshot.ExitKnown {
		code := snapshot.ExitCode
		slot.session.LastExitCode = &code
	}
	slot.session.NextAction = "请先尝试正常关闭本次会话；失败后才提供指定会话的强制结束，未确认退出仍保护数据。"
	if err := s.persistRuntime(slot, nil, "浏览器会话异常"); err != nil {
		slot.session.Error = storageFailure(err).Error
	}
}

func (s *Service) completeObservedExit(slot *runtimeSlot, snapshot kernel.RuntimeSnapshot, forced bool) {
	if s.runtimeSlots[slot.session.EnvironmentID] != slot {
		return
	}
	if !snapshot.ExitKnown && slot.session.LastExitCode != nil {
		snapshot.ExitKnown, snapshot.ExitCode = true, *slot.session.LastExitCode
	}
	slot.process = nil
	slot.session.PID, slot.session.CanControl, slot.session.CanForce, slot.session.NeedsReconcile = 0, false, false, false
	if snapshot.ExitKnown {
		code := snapshot.ExitCode
		slot.session.LastExitCode = &code
	}
	_ = s.observeRuntimeNetworkFault(slot, snapshot)
	if slot.session.NetworkFault != nil {
		setRuntimeNetworkContainment(slot, "stopped")
		slot.session.State, slot.session.Error = "error", slot.session.NetworkFault.Error
		slot.session.NextAction = "网络故障后的本次进程树/桥接资源已确认退出；原代理策略与档案不变，修复后重新检查并启动。"
	} else if slot.cleanupIntent && !forced && slot.startupError != nil && slot.startupError.Code != "OPERATION_CANCELLED" {
		slot.session.State, slot.session.Error = "error", slot.startupError
		slot.session.NextAction = "启动失败后的本次清理已确认退出；保留原始失败原因，修复后可重试原档案。"
	} else if snapshot.ExitKnown && snapshot.ExitCode != 0 && !slot.cleanupIntent && !forced && !s.closed {
		slot.session.State = "error"
		slot.session.Error = &Error{Code: "PROCESS_CRASHED", Message: "浏览器异常退出，已确认本次进程树和目录不再占用；原浏览数据及固定档案保持。", Retryable: true}
		slot.session.NextAction = "查看退出码和原因，修复后使用原档案重新启动。"
	} else {
		slot.session.State, slot.session.Error = "ready", nil
		slot.session.NextAction = "已确认本次进程树退出，原浏览数据保持，可使用原档案重开。"
	}
	if !runtimeStopPending(slot) && s.cookieTasks[slot.session.EnvironmentID] == nil {
		delete(s.profileUses, slot.session.EnvironmentID)
	}
}

func (s *Service) forceStopRuntime(input runtimeRequest) Result {
	slot := s.runtimeSlots[input.EnvironmentID]
	if slot == nil || slot.session.SessionID != input.SessionID {
		return failure("REVISION_CONFLICT", "所选旧会话已变更，未结束现在的会话或其他进程。", true)
	}
	if slot.stop != nil && slot.stop.Kind == "runtime-force-stop" && (slot.stop.State == "accepted" || slot.stop.State == "running") {
		return s.acceptRuntime("Runtime.ForceStop", input, *slot.stop)
	}
	if !slot.session.CanForce || slot.session.NeedsReconcile || slot.process == nil || slot.stop == nil || slot.stop.State != "failed" {
		return failure("FORCE_STOP_NOT_ALLOWED", "必须先正常关闭失败，并且仍持有这份会话的受控Job；不能按PID或名称结束浏览器。", false)
	}
	if slot.process.PID() != slot.session.RootPID || slot.process.CreatedAt() != slot.session.ProcessCreatedAt {
		return failure("SESSION_IDENTITY_UNCONFIRMED", "受控会话身份不匹配，未结束任何进程。", false)
	}
	operation := Operation{ID: id(), Kind: "runtime-force-stop", State: "accepted", Stage: "terminating-owned-job", Total: 1, CompletedIDs: []string{}, EnvironmentID: input.EnvironmentID, SessionID: input.SessionID, KernelID: slot.session.KernelID}
	next := slot.session
	next.State, next.CanForce, next.NextAction = "stopping", false, "仅结束已确认的本次Job，仍等待全部子进程退出；不按PID搜索或清空浏览数据。"
	result := s.acceptRuntimeRecord("Runtime.ForceStop", input, operation, &next)
	if !result.OK {
		return result
	}
	slot.session, slot.stop = next, &operation
	s.cancelRuntimeCookies(input.EnvironmentID)
	process := slot.process
	s.workers.Add(1)
	go func() {
		defer s.workers.Done()
		err := process.Close()
		select {
		case <-process.Done():
			err = nil
		default:
			if err == nil {
				err = kernelProblemExitUnconfirmed()
			}
		}
		s.mu.Lock()
		defer s.mu.Unlock()
		if s.runtimeSlots[input.EnvironmentID] != slot {
			return
		}
		if err != nil {
			slot.stop.State, slot.stop.Stage = "failed", "force-exit-unconfirmed"
			slot.stop.Error = &Error{Code: "PROCESS_STOP_TIMEOUT", Message: "指定Job强制结束后仍未确认全部退出；目录与档案锁保留，未操作其他进程。", Retryable: true}
			slot.session.State, slot.session.Error, slot.session.CanForce = "error", slot.stop.Error, true
			slot.session.NextAction = "可以重试结束同一已确认会话；实际目录未释放前不能启动或替换数据。"
		} else {
			slot.stop.State, slot.stop.Stage, slot.stop.CompletedIDs = "completed", "owned-job-exited", []string{input.EnvironmentID}
			s.completeObservedExit(slot, process.Snapshot(), true)
		}
		if err := s.persistRuntime(slot, slot.stop, "指定会话强制结束"); err != nil {
			slot.session.State, slot.session.Error = "error", storageFailure(err).Error
		}
	}()
	return result
}

func kernelProblemExitUnconfirmed() error {
	return &kernel.Problem{Code: "PROCESS_EXIT_UNCONFIRMED", Message: "owned tree exit not confirmed", Retryable: true}
}

func (s *Service) reconcileRuntime(input runtimeRequest) Result {
	if s.cookieTasks[input.EnvironmentID] != nil {
		return failure("PROFILE_BUSY", "本次Cookie任务或观测待保存，核对不能提前释放它的数据预留。", true)
	}
	slot := s.runtimeSlots[input.EnvironmentID]
	if slot == nil || slot.session.SessionID != input.SessionID {
		return failure("REVISION_CONFLICT", "会话已变更，未重新接管或修改其他会话。", true)
	}
	if slot.process != nil || runtimeStopPending(slot) || slot.session.State == "starting" || slot.session.State == "stopping" {
		return failure("PROFILE_BUSY", "这份会话仍由当前监督器控制，请查看当前状态或先正常关闭。", true)
	}
	if slot.reconcile != nil && (slot.reconcile.State == "accepted" || slot.reconcile.State == "running") {
		return s.acceptRuntime("Runtime.Reconcile", input, *slot.reconcile)
	}
	operation := Operation{ID: id(), Kind: "runtime-reconcile", State: "accepted", Stage: "checking-process-and-directory", Total: 1, CompletedIDs: []string{}, EnvironmentID: input.EnvironmentID, SessionID: input.SessionID, KernelID: slot.session.KernelID}
	next := slot.session
	next.NeedsReconcile, next.NextAction = true, "正在核对原进程创建时间、会话身份和实际目录锁；尚不允许启动或替换数据。"
	result := s.acceptRuntimeRecord("Runtime.Reconcile", input, operation, &next)
	if !result.OK {
		return result
	}
	slot.reconcile = &operation
	slot.session = next
	s.profileUses[input.EnvironmentID] = true
	s.workers.Add(1)
	go func() {
		defer s.workers.Done()
		s.mu.Lock()
		if s.runtimeSlots[input.EnvironmentID] != slot {
			s.mu.Unlock()
			return
		}
		snapshot := slot.session
		s.mu.Unlock()
		// Filesystem scans and process inspection never hold the global service
		// mutex. Busy remains reserved; late results must match this generation.
		recovery, inspectionErr := s.inspectSavedRuntime(snapshot)
		s.mu.Lock()
		defer s.mu.Unlock()
		if s.runtimeSlots[input.EnvironmentID] != slot {
			return
		}
		_ = s.applyReconciledRuntime(slot, recovery, inspectionErr, slot.reconcile)
	}()
	return result
}
