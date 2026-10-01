package workspace

import "github.com/axgiroud312-byte/prism-local-browser/internal/kernel"

func (s *Service) observeRuntimeNetworkFault(slot *runtimeSlot, snapshot kernel.RuntimeSnapshot) bool {
	if slot.session.NetworkPolicy != "proxy" || snapshot.ProxyError == nil {
		return false
	}
	if slot.session.NetworkFault == nil {
		slot.session.NetworkFault = &RuntimeNetworkFault{State: "network_error", Error: &Error{Code: snapshot.ProxyError.Code, Message: snapshot.ProxyError.Message, Retryable: snapshot.ProxyError.Retryable}, ObservedAt: timestamp(), Containment: "stopping"}
	}
	// Preserve prior pending/event snapshots instead of mutating shared JSON
	// pointers captured at an earlier containment stage.
	next := *slot.session.NetworkFault
	network := &next
	slot.session.NetworkFault = network
	if snapshot.NetworkCleanupFailed {
		network.Containment = "exit-unconfirmed"
	}
	if snapshot.ResourcesExited {
		network.Containment = "stopped"
	}
	slot.session.State, slot.session.Error = "error", network.Error
	slot.session.CanControl = snapshot.RootAlive && snapshot.ControlReady
	slot.session.NextAction = "代理通道出现故障，已关闭本次监听/连接并请求结束准确Job；仍待确认全树退出，不改直连或重建旧端口。"
	if network.Containment == "stopped" {
		slot.session.NextAction = "本次故障资源已确认退出；修复后重新检查并启动原环境，保留seed、内核与浏览数据。"
	}
	if network.Containment == "exit-unconfirmed" {
		slot.session.NextAction = "安全停止后全树退出仍未确认，数据/档案锁保持；请先正常关闭，失败后可明确结束这份准确会话。"
	}
	return true
}

func setRuntimeNetworkContainment(slot *runtimeSlot, state string) {
	if slot.session.NetworkFault == nil {
		return
	}
	next := *slot.session.NetworkFault
	next.Containment = state
	slot.session.NetworkFault = &next
}
