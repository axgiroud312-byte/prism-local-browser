package workspace

import (
	"context"

	"github.com/axgiroud312-byte/prism-local-browser/internal/kernel"
)

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
