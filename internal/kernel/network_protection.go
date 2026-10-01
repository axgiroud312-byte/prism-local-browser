package kernel

// No currently integrated provider establishes a verified per-session OS
// egress boundary across Chromium sockets AND delegated DNS, nor preserves
// that boundary through host/broker crashes. Proxy flags and a healthy Bridge
// are not substitutes. Installing an administrator-authorized broker is now
// allowed, but authority alone is not an implemented/verified boundary.
//
// Fail before creating any protected browser. This gate is intentionally not
// an RPC toggle, a build whitelist, or a promise that a broker already exists.
func RequireProxyNetworkBoundary() error {
	return &Problem{Code: "NETWORK_PROTECTION_UNAVAILABLE", Reason: "verified-egress-boundary-missing", Message: "当前构建尚无已实现并验证的系统级代理出网隔离；已阻止代理环境启动，未改为直连。原档案和数据保持，隔离组件完成并验证后才能重试。", Retryable: false}
}
