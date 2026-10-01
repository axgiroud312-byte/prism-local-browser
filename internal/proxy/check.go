package proxy

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptrace"
	"net/netip"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

const DefaultCheckTarget = "https://api.ipify.org?format=json"
const CheckVersion = "go-proxy-preflight-v1"

// Non-default target/CA are host-only seams for controlled proxy fixtures.
// Desktop bridge requests cannot override them or disable TLS verification.
type CheckOptions struct {
	TargetURL string
	RootCAs   *x509.CertPool
}

func Check(ctx context.Context, config Configuration, credentials *Credentials, options CheckOptions, progress func(Step)) Report {
	ctx, cancel := context.WithTimeout(ctx, 25*time.Second)
	defer cancel()
	started := time.Now()
	report := Report{Mode: "native", AdapterVersion: CheckVersion, StartedAt: started.UTC().Format(time.RFC3339Nano), Steps: []Step{}}
	var mu sync.Mutex
	finished := false
	step := func(stage, status, message string) {
		value := Step{Stage: stage, Status: status, Time: time.Now().UTC().Format(time.RFC3339Nano), Message: message}
		mu.Lock()
		if finished {
			mu.Unlock()
			return
		}
		report.Steps = append(report.Steps, value)
		if progress != nil {
			progress(value)
		}
		mu.Unlock()
	}
	finish := func(code, message string, retryable bool) Report {
		if code != "" {
			step("result", "failed", message)
		}
		mu.Lock()
		defer mu.Unlock()
		finished = true
		report.FinishedAt, report.DurationMS = time.Now().UTC().Format(time.RFC3339Nano), time.Since(started).Milliseconds()
		if code != "" {
			report.Error = &CheckError{Code: code, Message: message, Retryable: retryable}
		}
		report.Steps = append([]Step(nil), report.Steps...)
		return report
	}
	normalized, err := Normalize(config)
	if err != nil {
		return finish("PROXY_INVALID", "代理格式无效；未发起网络请求。", false)
	}
	config = normalized
	step("validation", "passed", "已校验协议、地址和端口。")
	if config.Type == "socks5" {
		step("support", "unsupported", "SOCKS5配置可保存，但当前不支持真实检查。")
		return finish("PROXY_UNSUPPORTED", "SOCKS5检查尚未接入，没有发起请求或切换直连。", false)
	}
	if credentials != nil && ValidateCredentials(*credentials) != nil {
		return finish("CREDENTIALS_UNAVAILABLE", "保存的认证字段不可用，请明确替换凭据。", false)
	}
	target := options.TargetURL
	if target == "" {
		target = DefaultCheckTarget
	}
	u, err := url.Parse(target)
	if err != nil || u.Scheme != "https" || u.Host == "" || u.User != nil || u.Fragment != "" {
		return finish("PROXY_TARGET_FAILED", "受控出口检查目标无效；未发起请求。", false)
	}
	report.TargetOrigin = u.Scheme + "://" + u.Host
	proxyURL := &url.URL{Scheme: config.Type, Host: net.JoinHostPort(config.Host, strconv.Itoa(config.Port))}
	headers := make(http.Header)
	if credentials != nil {
		headers.Set("Proxy-Authorization", "Basic "+base64.StdEncoding.EncodeToString([]byte(credentials.Username+":"+credentials.Password)))
	}
	var tunnelErr *CheckError
	var connMu sync.Mutex
	connections := []net.Conn{}
	connectionsClosed := false
	closeConnections := func() {
		connMu.Lock()
		connectionsClosed = true
		owned := append([]net.Conn(nil), connections...)
		connMu.Unlock()
		for _, conn := range owned {
			_ = conn.Close()
		}
	}
	stopConnectionCleanup := context.AfterFunc(ctx, closeConnections)
	defer func() { stopConnectionCleanup(); closeConnections() }()
	dialer := &net.Dialer{Timeout: 6 * time.Second, KeepAlive: -1}
	transport := &http.Transport{
		Proxy: http.ProxyURL(proxyURL), ProxyConnectHeader: headers,
		DialContext: func(_ context.Context, network, address string) (net.Conn, error) {
			// Transport may detach its dial context from a cancelled request.
			// Pin this private transport to the original check and close every
			// owned socket on cancellation/finish, including an in-flight CONNECT.
			step("connection", "running", "正在连接所选代理，不使用直连回退。")
			conn, err := dialer.DialContext(ctx, network, address)
			if err != nil {
				step("connection", "failed", "到所选代理的连接未完成，没有回退直连。")
				return nil, err
			}
			connMu.Lock()
			if connectionsClosed {
				connMu.Unlock()
				conn.Close()
				return nil, errors.New("check-connections-closed")
			}
			connections = append(connections, conn)
			connMu.Unlock()
			step("connection", "passed", "已建立到所选代理的TCP连接。")
			return conn, nil
		},
		TLSClientConfig:     &tls.Config{MinVersion: tls.VersionTLS12, RootCAs: options.RootCAs},
		TLSHandshakeTimeout: 8 * time.Second, ResponseHeaderTimeout: 8 * time.Second,
		DisableKeepAlives: true, DisableCompression: true, MaxConnsPerHost: 1,
		OnProxyConnectResponse: func(_ context.Context, _ *url.URL, _ *http.Request, response *http.Response) error {
			if response.StatusCode == http.StatusProxyAuthRequired {
				observed := &CheckError{Code: "PROXY_AUTH_FAILED", Message: "代理拒绝本次认证；请修正凭据后重检。", Retryable: false}
				mu.Lock()
				if !finished {
					tunnelErr = observed
				}
				mu.Unlock()
				step("authentication", "failed", observed.Message)
				return errors.New("proxy-auth-rejected")
			}
			if response.StatusCode != http.StatusOK {
				observed := &CheckError{Code: "PROXY_TARGET_FAILED", Message: "代理未允许到检查目标的隧道；未回退直连。", Retryable: true}
				mu.Lock()
				if !finished {
					tunnelErr = observed
				}
				mu.Unlock()
				step("tunnel", "failed", observed.Message)
				return errors.New("proxy-tunnel-rejected")
			}
			message := "代理接受本次无认证隧道请求。"
			if credentials != nil {
				message = "代理接受本次携带认证的隧道请求；不回显凭据。"
			}
			step("authentication", "passed", message)
			return nil
		},
	}
	defer transport.CloseIdleConnections()
	client := &http.Client{Transport: transport, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return finish("PROXY_TARGET_FAILED", "出口检查请求无效。", false)
	}
	request.Header.Set("Accept", "application/json")
	request.Header.Set("User-Agent", "Prism-Local-Proxy-Check/1")
	tlsCount := 0
	trace := &httptrace.ClientTrace{
		TLSHandshakeDone: func(_ tls.ConnectionState, err error) {
			mu.Lock()
			tlsCount++
			stage := "target-tls"
			if config.Type == "https" && tlsCount == 1 {
				stage = "proxy-tls"
			}
			mu.Unlock()
			status, message := "passed", "TLS证书和主机名已通过验证。"
			if err != nil {
				status, message = "failed", "TLS证书、主机名或握手验证失败；未关闭验证。"
			}
			step(stage, status, message)
		},
	}
	request = request.WithContext(httptrace.WithClientTrace(request.Context(), trace))
	response, err := client.Do(request)
	if err != nil {
		mu.Lock()
		observed := tunnelErr
		mu.Unlock()
		if observed != nil {
			return finish(observed.Code, observed.Message, observed.Retryable)
		}
		var verify *tls.CertificateVerificationError
		if errors.As(err, &verify) {
			return finish("PROXY_TLS_FAILED", "TLS证书或主机名未通过验证；请修复证书，不能跳过验证。", false)
		}
		if errors.Is(ctx.Err(), context.Canceled) {
			return finish("OPERATION_CANCELLED", "本次检查已取消，没有修改环境身份。", true)
		}
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return finish("PROXY_CHECK_TIMEOUT", "检查超过时限；结果不是连接成功，请稍后重试。", true)
		}
		return finish("PROXY_UNREACHABLE", "所选代理或目标连接失败；没有静默直连，原配置保持。", true)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return finish("PROXY_TARGET_FAILED", "出口检查目标未返回成功；不跟随重定向，不推断真实出口。", true)
	}
	step("target", "passed", "已通过所选代理访问受控HTTPS出口检查目标。")
	data, err := io.ReadAll(io.LimitReader(response.Body, (16<<10)+1))
	if err != nil {
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return finish("PROXY_CHECK_TIMEOUT", "目标响应读取超过时限；未报告完整出口结果，请稍后重检。", true)
		}
		if errors.Is(ctx.Err(), context.Canceled) {
			return finish("OPERATION_CANCELLED", "本次检查在读取目标响应时取消，没有修改环境身份。", true)
		}
	}
	if err != nil || len(data) > 16<<10 {
		return finish("PROXY_EXIT_INVALID", "出口响应读取失败或过大，未报告出口成功。", true)
	}
	var payload struct {
		IP string `json:"ip"`
	}
	if json.Unmarshal(data, &payload) != nil {
		return finish("PROXY_EXIT_INVALID", "出口响应格式无效，未报告出口成功。", true)
	}
	ip, err := netip.ParseAddr(strings.TrimSpace(payload.IP))
	if err != nil || ip.Zone() != "" || !ip.IsGlobalUnicast() {
		return finish("PROXY_EXIT_INVALID", "响应没有有效出口IP，未报告出口成功。", true)
	}
	report.ExitIP = ip.String()
	step("exit", "passed", "已读取本次请求的实际出口IP；历史成功不保证下次可用。")
	return finish("", "", false)
}
