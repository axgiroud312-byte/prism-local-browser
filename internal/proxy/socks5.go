package proxy

import (
	"context"
	"encoding/binary"
	"io"
	"net"
	"net/netip"
	"strconv"
)

const SOCKS5ResolutionPolicy = "remote-target-dns"

func ValidateProtocolCredentials(kind string, credentials Credentials) error {
	if kind != "socks5" {
		if ValidateCredentials(credentials) != nil {
			return &CheckError{Code: "PROXY_AUTH_INVALID", Message: "保存的HTTP/HTTPS认证不兼容：用户名不能含冒号或控制字符；请明确替换或清除认证，未无认证重试。", Retryable: false}
		}
		return nil
	}
	if ValidateStoredCredentials(credentials) != nil || len(credentials.Username) < 1 || len(credentials.Username) > 255 || len(credentials.Password) < 1 || len(credentials.Password) > 255 {
		return &CheckError{Code: "PROXY_AUTH_INVALID", Message: "SOCKS5认证的用户名和密码各需1–255个UTF-8字节，不能含控制字符；空认证请明确选择清除，不降级无认证。", Retryable: false}
	}
	return nil
}

func writeSOCKSPacket(conn net.Conn, value []byte) error {
	for len(value) > 0 {
		written, err := conn.Write(value)
		if err != nil {
			return err
		}
		if written == 0 {
			return io.ErrShortWrite
		}
		value = value[written:]
	}
	return nil
}
func socksDestination(target string) ([]byte, string, *CheckError) {
	canonical, err := proxyDestination(target)
	if err != nil {
		return nil, "", &CheckError{Code: "PROXY_TARGET_FAILED", Message: "SOCKS5目标地址无效，未尝试本机目标解析。", Retryable: false}
	}
	host, portText, _ := net.SplitHostPort(canonical)
	port, _ := strconv.Atoi(portText)
	packet := []byte{5, 1 /* CONNECT */, 0}
	stage := "domain-target"
	if ip, err := netip.ParseAddr(host); err == nil {
		if ip.Is4() {
			packet = append(packet, 1)
			bytes := ip.As4()
			packet = append(packet, bytes[:]...)
			stage = "ipv4-target"
		} else {
			packet = append(packet, 4)
			bytes := ip.As16()
			packet = append(packet, bytes[:]...)
			stage = "ipv6-target"
		}
	} else {
		// Host is already normalized ASCII/IDNA. Pass DOMAINNAME bytes to
		// SOCKS5; never call a local resolver for the destination hostname.
		if len(host) == 0 || len(host) > 255 {
			return nil, "", &CheckError{Code: "PROXY_TARGET_UNSUPPORTED", Message: "目标域名超过SOCKS5地址范围，未改为本机解析或直连。", Retryable: false}
		}
		packet = append(packet, 3, byte(len(host)))
		packet = append(packet, []byte(host)...)
	}
	packet = binary.BigEndian.AppendUint16(packet, uint16(port))
	return packet, stage, nil
}

func (b *Bridge) socksConnect(ctx context.Context, conn net.Conn, target string, probe *bridgeProbe) (failure *CheckError) {
	defer func() { failure = requestContextFailure(ctx, failure) }()
	if ctx.Err() != nil {
		return &CheckError{Code: "OPERATION_CANCELLED", Message: "本次SOCKS5连接已取消，没有发起目标请求。", Retryable: true}
	}
	method := byte(0)
	if b.authentication != nil {
		method = 2
		if err := ValidateProtocolCredentials("socks5", *b.authentication); err != nil {
			return &CheckError{Code: "PROXY_AUTH_INVALID", Message: "保存的SOCKS5认证超出1–255字节范围，未无认证重试。", Retryable: false}
		}
	}
	negotiationFailure := func() *CheckError {
		return &CheckError{Code: "PROXY_SOCKS_NEGOTIATION_FAILED", Message: "SOCKS5协议或认证方法协商失败，未改用其他方法或直连。", Retryable: true}
	}
	b.event(probe, "socks-negotiation", "running", "仅提供保存配置要求的SOCKS5认证方法，不静默降级。")
	if writeSOCKSPacket(conn, []byte{5, 1, method}) != nil {
		return negotiationFailure()
	}
	reply := make([]byte, 2)
	if _, err := io.ReadFull(conn, reply); err != nil || reply[0] != 5 || reply[1] != method {
		return negotiationFailure()
	}
	b.event(probe, "socks-negotiation", "passed", "上游接受本次配置要求的SOCKS5认证方法。")
	if method == 2 {
		b.event(probe, "upstream-authentication", "running", "正在验证已保存的SOCKS5用户名密码认证，认证内容仅发给绑定上游。")
		credentials := b.authentication
		packet := []byte{1, byte(len(credentials.Username))}
		packet = append(packet, []byte(credentials.Username)...)
		packet = append(packet, byte(len(credentials.Password)))
		packet = append(packet, []byte(credentials.Password)...)
		err := writeSOCKSPacket(conn, packet)
		Wipe(packet)
		if err != nil {
			return negotiationFailure()
		}
		if _, err := io.ReadFull(conn, reply); err != nil || reply[0] != 1 {
			return negotiationFailure()
		}
		if reply[1] != 0 {
			return &CheckError{Code: "PROXY_AUTH_FAILED", Message: "SOCKS5上游拒绝本次用户名密码认证；未降级无认证或切直连。", Retryable: false}
		}
		b.event(probe, "upstream-authentication", "passed", "SOCKS5上游已接受保存的用户名密码认证，不回显内容。")
	} else {
		b.event(probe, "upstream-authentication", "passed", "SOCKS5上游已接受明确保存的无认证策略。")
	}
	packet, stage, failure := socksDestination(target)
	if failure != nil {
		return failure
	}
	message := "目标域名按DOMAINNAME传给SOCKS5上游解析，未调用本机目标DNS。"
	if stage == "ipv4-target" {
		message = "IPv4目标以地址字节传给SOCKS5上游，不转换为本机直连。"
	}
	if stage == "ipv6-target" {
		message = "IPv6目标以地址字节传给SOCKS5上游，是否支持以实际回复为准。"
	}
	b.event(probe, "target-resolution", "passed", message)
	if writeSOCKSPacket(conn, packet) != nil {
		return &CheckError{Code: "PROXY_TARGET_FAILED", Message: "SOCKS5目标请求未能发送，没有直连请求。", Retryable: true}
	}
	header := make([]byte, 4)
	if _, err := io.ReadFull(conn, header); err != nil || header[0] != 5 || header[2] != 0 {
		return negotiationFailure()
	}
	if header[1] != 0 {
		code, message := "PROXY_TARGET_FAILED", "SOCKS5上游拒绝本次目标连接，未切换直连。"
		if header[1] == 8 {
			code, message = "PROXY_TARGET_UNSUPPORTED", "SOCKS5上游不支持本次目标地址类型；IPv4/IPv6或域名路径没有被偷偷替换。"
		}
		if header[1] == 3 || header[1] == 4 || header[1] == 5 {
			code, message = "PROXY_TARGET_UNREACHABLE", "SOCKS5上游无法到达或被目标拒绝；代理TCP连接不等于目标成功。"
		}
		return &CheckError{Code: code, Message: message, Retryable: true}
	}
	length := 0
	switch header[3] {
	case 1:
		length = 4
	case 4:
		length = 16
	case 3:
		if _, err := io.ReadFull(conn, reply[:1]); err != nil || reply[0] == 0 {
			return negotiationFailure()
		}
		length = int(reply[0])
	default:
		return negotiationFailure()
	}
	// Consume the complete bound-address reply before passing target bytes.
	// Its value is never exposed as the public exit IP or private endpoint.
	if _, err := io.ReadFull(conn, make([]byte, length+2)); err != nil {
		return negotiationFailure()
	}
	b.event(probe, stage, "passed", "SOCKS5上游已确认本次目标地址连接；网络不经本机目标拨号。")
	return nil
}
