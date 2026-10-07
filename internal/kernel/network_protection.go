package kernel

// A generic capability check cannot authorize a browser. The launcher requires
// concrete ProtectedProxy material bound to this exact session/root/build and
// a successful same-channel preflight, then verifies the actual process tree.
// This owner/Job gate does not claim system-wide browser egress isolation.
func RequireProxyNetworkBoundary() error {
	return &Problem{Code: "NETWORK_PROTECTION_UNAVAILABLE", Reason: "session-egress-boundary-missing", Message: "本次启动未提供与环境、内核和会话匹配的代理通道及前检结果，已拒绝启动且未改为直连；请使用正式环境启动入口，原档案和数据保持。", Retryable: false}
}
