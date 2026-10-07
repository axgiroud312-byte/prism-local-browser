package kernel

import (
	"net"
	"net/url"
	"strconv"
)

func managedNetworkArguments(endpoint string) ([]string, error) {
	if endpoint == "" {
		return []string{"--no-proxy-server"}, nil
	}
	u, err := url.Parse(endpoint)
	if err != nil || u.Scheme != "http" || u.User != nil || u.Hostname() != "127.0.0.1" || u.Path != "" || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" {
		return nil, problem("PROXY_BRIDGE_UNAVAILABLE", "invalid-private-endpoint", "本次受控代理入口无效，未启动或切换直连。")
	}
	port, err := strconv.Atoi(u.Port())
	if err != nil || port < 1 || port > 65535 || u.Host != net.JoinHostPort("127.0.0.1", strconv.Itoa(port)) {
		return nil, problem("PROXY_BRIDGE_UNAVAILABLE", "invalid-private-port", "本次代理入口不是实际本机监听地址，未启动。")
	}
	return []string{"--proxy-server=" + endpoint, "--proxy-bypass-list=<-loopback>", "--disable-quic", "--force-webrtc-ip-handling-policy=disable_non_proxied_udp"}, nil
}
