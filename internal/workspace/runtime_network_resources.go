package workspace

import (
	"context"

	"github.com/axgiroud312-byte/prism-local-browser/internal/kernel"
)

type networkRecoveryTask struct {
	intent              kernel.NetworkSessionIntent
	operation           Operation
	finished, recovered bool
}

// Runs only on the primary Service, never a private maintenance loader. A
// journal-only owner needs no invented browser PID/session row in app.db.
func (s *Service) reconcileNetworkResources(input runtimeRequest) Result {
	intent := s.networkPending[input.EnvironmentID]
	if intent.SessionID != input.SessionID {
		return failure("REVISION_CONFLICT", "网络资源会话已变更，请重新读取。", true)
	}
	if task := s.networkRecoveries[input.EnvironmentID]; task != nil {
		return s.acceptRuntime("Runtime.Reconcile", input, task.operation)
	}
	slot := s.runtimeSlots[input.EnvironmentID]
	if slot != nil && (slot.process != nil || runtimeStopPending(slot) || slot.session.State == "starting" || slot.session.State == "stopping") {
		return failure("PROFILE_BUSY", "当前会话仍由监督器控制，请先关闭。", true)
	}
	op := Operation{ID: id(), Kind: "runtime-reconcile", State: "accepted", Stage: "recovering-network-resources", Total: 1, CompletedIDs: []string{}, EnvironmentID: input.EnvironmentID, SessionID: input.SessionID}
	result := s.acceptRuntime("Runtime.Reconcile", input, op)
	if !result.OK {
		return result
	}
	if s.networkRecoveries == nil {
		s.networkRecoveries = map[string]*networkRecoveryTask{}
	}
	task := &networkRecoveryTask{intent: intent, operation: op}
	s.networkRecoveries[input.EnvironmentID] = task
	s.profileUses[input.EnvironmentID] = true
	var snapshot RuntimeSession
	if slot != nil {
		snapshot = slot.session
	}
	s.workers.Add(1)
	go func() {
		defer s.workers.Done()
		err := s.networkStore.Recover(context.Background(), intent)
		var recovery kernel.ManagedRecovery
		var inspectErr error
		if err == nil && slot != nil {
			recovery, inspectErr = s.inspectRuntimeAfterNetworkRecovery(snapshot)
		}
		s.mu.Lock()
		defer s.mu.Unlock()
		if s.networkRecoveries[input.EnvironmentID] != task || s.networkPending[input.EnvironmentID] != intent {
			return
		}
		task.finished, task.recovered = true, err == nil
		task.operation.State, task.operation.Stage = "completed", "network-resources-recovered"
		task.operation.CompletedIDs = []string{input.EnvironmentID}
		if err != nil {
			task.operation.State, task.operation.Stage = "failed", "network-cleanup-pending"
			task.operation.CompletedIDs = []string{}
			task.operation.Error = &Error{Code: "NETWORK_CLEANUP_PENDING", Message: "原代理会话资源仍无法确认清理，保持占用；解除目录/进程问题后可再次核对，无需重开。", Retryable: true}
		}
		if err == nil && slot != nil && s.runtimeSlots[input.EnvironmentID] == slot {
			delete(s.networkPending, input.EnvironmentID)
			slot.reconcile = &task.operation
			_ = s.applyReconciledRuntime(slot, recovery, inspectErr, slot.reconcile)
			delete(s.networkRecoveries, input.EnvironmentID)
			return
		}
		s.flushNetworkRecoveries()
	}()
	return result
}

func (s *Service) flushNetworkRecoveries() {
	for env, task := range s.networkRecoveries {
		if !task.finished {
			continue
		}
		if err := s.storeOperation(task.operation); err != nil {
			pending := task.operation
			pending.State, pending.Stage, pending.PersistencePending = "failed", "storage-pending", true
			pending.Error = storageFailure(err).Error
			s.runtimeResults[pending.ID] = pending
			continue
		}
		delete(s.runtimeResults, task.operation.ID)
		delete(s.networkRecoveries, env)
		if task.recovered && s.networkPending[env] == task.intent {
			delete(s.networkPending, env)
			s.releaseProfileUse(env)
		}
	}
}

func (s *Service) networkResourceViews() map[string]string {
	result := map[string]string{}
	for env, intent := range s.networkPending {
		result[env] = intent.SessionID
	}
	return result
}

func (s *Service) openNetworkResources() error {
	store, err := kernel.OpenNetworkStore(s.root)
	if err != nil {
		return &Error{Code: "NETWORK_CLEANUP_PENDING", Message: "网络资源日志无法安全打开；请关闭另一管理程序，检查工作区权限后重开，保留原数据与日志。", Retryable: true}
	}
	s.networkStore = store
	pending, err := store.Pending(context.Background())
	if err != nil {
		store.Close()
		return &Error{Code: "NETWORK_CLEANUP_PENDING", Message: "网络资源日志不能完整读取，请保留日志并修复存储后重开。", Retryable: true}
	}
	s.networkPending = pending
	for environmentID, intent := range pending {
		if err := store.Recover(context.Background(), intent); err == nil {
			delete(pending, environmentID)
		} else {
			s.profileUses[environmentID] = true
		}
	}
	return nil
}

func (s *Service) hasPendingNetwork(environmentID string) bool {
	_, pending := s.networkPending[environmentID]
	return pending
}

// Called before publishing a private bootstrap loader. These maps are not shared
// across Service mutexes; resource ownership survives replacing runtime maps.
func (s *Service) inheritNetworkResources(parent *Service) {
	s.networkStore = parent.networkStore
	s.networkPending = make(map[string]kernel.NetworkSessionIntent, len(parent.networkPending))
	for id, intent := range parent.networkPending {
		s.networkPending[id] = intent
		s.profileUses[id] = true
	}
}
