package proxy

import (
	"bufio"
	"context"
	"crypto/subtle"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"errors"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

const BridgeVersion = "authenticated-local-channel-v2-socks5-remote-dns"

// Only the host supplies caller guards and fixture trust/target settings. No
// bridge endpoint, upstream credential, probe token or bypass option is RPC.
type BridgeOptions struct {
	ChannelID      string
	AuthorizeProbe func(net.Conn) bool
	TargetURL      string
	RootCAs        *x509.CertPool
}
type bridgeProbe struct {
	progress func(Step)
	err      *CheckError
}
type Bridge struct {
	config         Configuration
	opts           BridgeOptions
	upstreamAuth   string
	authentication *Credentials
	listener       net.Listener
	server         *http.Server
	ctx            context.Context
	cancel         context.CancelFunc
	mu             sync.Mutex
	closing        bool
	fatal          *CheckError
	browserGuard   func(net.Conn) bool
	probes         map[string]*bridgeProbe
	connections    map[net.Conn]bool
	workers        sync.WaitGroup
	forwardGate    chan struct{}
	serveDone      chan struct{}
	done           chan struct{}
	closeOnce      sync.Once
	faultOnce      sync.Once
	failed         chan struct{}
	monitors       sync.WaitGroup
}
type bridgeClientKey struct{}

func OpenBridge(config Configuration, credentials *Credentials, options BridgeOptions) (*Bridge, error) {
	config, err := Normalize(config)
	if err != nil || options.ChannelID == "" || options.AuthorizeProbe == nil {
		return nil, errors.New("invalid host channel configuration")
	}
	if credentials != nil {
		if err := ValidateProtocolCredentials(config.Type, *credentials); err != nil {
			return nil, err
		}
	}
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		return nil, &CheckError{Code: "PROXY_BRIDGE_UNAVAILABLE", Message: "本次环境的独立本机通道无法建立，未启动。", Retryable: true}
	}
	ctx, cancel := context.WithCancel(context.Background())
	b := &Bridge{config: config, opts: options, listener: listener, ctx: ctx, cancel: cancel, probes: map[string]*bridgeProbe{}, connections: map[net.Conn]bool{}, forwardGate: make(chan struct{}, 64), serveDone: make(chan struct{}), done: make(chan struct{}), failed: make(chan struct{})}
	if credentials != nil {
		copy := *credentials
		b.authentication = &copy
		if config.Type != "socks5" {
			b.upstreamAuth = "Basic " + base64.StdEncoding.EncodeToString([]byte(credentials.Username+":"+credentials.Password))
		}
	}
	b.server = &http.Server{
		Handler: b, ReadHeaderTimeout: 6 * time.Second, IdleTimeout: 30 * time.Second, MaxHeaderBytes: 64 << 10,
		ErrorLog: log.New(io.Discard, "", 0), BaseContext: func(net.Listener) context.Context { return ctx },
		ConnContext: func(ctx context.Context, conn net.Conn) context.Context {
			b.track(conn)
			return context.WithValue(ctx, bridgeClientKey{}, conn)
		},
		ConnState: func(conn net.Conn, state http.ConnState) {
			if state == http.StateClosed {
				b.untrack(conn)
			}
		},
	}
	go func() {
		_ = b.server.Serve(listener)
		b.mu.Lock()
		unexpected := !b.closing
		if unexpected && b.fatal == nil {
			b.fatal = &CheckError{Code: "PROXY_BRIDGE_UNAVAILABLE", Message: "本次本机代理监听已中断，没有切换直连；请先关闭本次环境。", Retryable: true}
			b.faultOnce.Do(func() { close(b.failed) })
		}
		b.mu.Unlock()
		close(b.serveDone)
		if unexpected {
			go b.Close()
		}
	}()
	return b, nil
}

// Endpoint is private launch material. The UI only receives ChannelID.
func (b *Bridge) Endpoint() string        { return "http://" + b.listener.Addr().String() }
func (b *Bridge) ID() string              { return b.opts.ChannelID }
func (b *Bridge) Done() <-chan struct{}   { return b.done }
func (b *Bridge) Failed() <-chan struct{} { return b.failed }
func (b *Bridge) Fault() *CheckError {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.fatal == nil {
		return nil
	}
	copy := *b.fatal
	return &copy
}
func (b *Bridge) BindBrowser(guard func(net.Conn) bool) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closing || b.browserGuard != nil || guard == nil {
		return &CheckError{Code: "PROXY_BRIDGE_UNAVAILABLE", Message: "本次代理通道已关闭或会话绑定不匹配，未报告浏览器就绪。", Retryable: true}
	}
	b.browserGuard = guard
	b.monitors.Add(1)
	go b.watchNetwork()
	return nil
}
func (b *Bridge) track(conn net.Conn) bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closing {
		_ = conn.Close()
		return false
	}
	b.connections[conn] = true
	return true
}
func (b *Bridge) untrack(conn net.Conn) { b.mu.Lock(); delete(b.connections, conn); b.mu.Unlock() }
func (b *Bridge) Close() error {
	b.closeOnce.Do(func() {
		b.mu.Lock()
		b.closing = true
		owned := make([]net.Conn, 0, len(b.connections))
		for conn := range b.connections {
			owned = append(owned, conn)
		}
		b.mu.Unlock()
		b.cancel()
		_ = b.server.Close()
		_ = b.listener.Close()
		for _, conn := range owned {
			_ = conn.Close()
		}
		<-b.serveDone
		b.workers.Wait()
		b.monitors.Wait()
		b.mu.Lock()
		b.upstreamAuth = ""
		if b.authentication != nil {
			b.authentication.Username, b.authentication.Password = "", ""
		}
		b.authentication = nil
		b.browserGuard = nil
		clear(b.probes)
		clear(b.connections)
		b.mu.Unlock()
		close(b.done)
	})
	return nil
}

func (b *Bridge) authorize(r *http.Request) (*bridgeProbe, bool) {
	conn, _ := r.Context().Value(bridgeClientKey{}).(net.Conn)
	if conn == nil {
		return nil, false
	}
	b.mu.Lock()
	if b.closing {
		b.mu.Unlock()
		return nil, false
	}
	var probe *bridgeProbe
	for token, active := range b.probes {
		if subtle.ConstantTimeCompare([]byte(r.Header.Get("Proxy-Authorization")), []byte(token)) == 1 {
			probe = active
			break
		}
	}
	guard := b.browserGuard
	b.mu.Unlock()
	if probe != nil {
		return probe, b.opts.AuthorizeProbe(conn)
	}
	return nil, guard != nil && guard(conn)
}
func (b *Bridge) event(probe *bridgeProbe, stage, status, message string) {
	if probe == nil {
		return
	}
	b.mu.Lock()
	active := false
	for _, p := range b.probes {
		if p == probe {
			active = true
			break
		}
	}
	callback := probe.progress
	b.mu.Unlock()
	if active && callback != nil {
		callback(Step{Stage: stage, Status: status, Time: time.Now().UTC().Format(time.RFC3339Nano), Message: message})
	}
}
func (b *Bridge) reject(w http.ResponseWriter, ctx context.Context, probe *bridgeProbe, err *CheckError) {
	err = requestContextFailure(ctx, err)
	if probe == nil {
		switch err.Code {
		case "PROXY_UNREACHABLE", "PROXY_AUTH_FAILED", "PROXY_TLS_FAILED", "PROXY_SOCKS_NEGOTIATION_FAILED":
			b.failNetwork(err)
		}
	}
	if probe != nil {
		b.mu.Lock()
		probe.err = err
		b.mu.Unlock()
		b.event(probe, "upstream-result", "failed", err.Message)
	}
	w.Header().Set("Connection", "close")
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusBadGateway)
	_, _ = io.WriteString(w, err.Message)
}

func (b *Bridge) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	b.mu.Lock()
	if b.closing {
		b.mu.Unlock()
		http.Error(w, "本次代理通道已关闭。", 503)
		return
	}
	b.workers.Add(1)
	b.mu.Unlock()
	defer b.workers.Done()
	probe, allowed := b.authorize(r)
	if !allowed {
		w.Header().Set("Connection", "close")
		http.Error(w, "调用进程不属于本次受控会话。", 403)
		return
	}
	select {
	case b.forwardGate <- struct{}{}:
		defer func() { <-b.forwardGate }()
	default:
		b.reject(w, r.Context(), probe, &CheckError{Code: "PROXY_BRIDGE_BUSY", Message: "本次通道并发连接繁忙，未回退直连。", Retryable: true})
		return
	}
	if r.Method == http.MethodConnect {
		b.connect(w, r, probe)
		return
	}
	b.forwardHTTP(w, r, probe)
}

func (b *Bridge) dial(ctx context.Context, probe *bridgeProbe) (_ net.Conn, failure *CheckError) {
	defer func() { failure = requestContextFailure(ctx, failure) }()
	startup, cancel := context.WithTimeout(ctx, 12*time.Second)
	defer cancel()
	b.event(probe, "upstream-connection", "running", "本机通道正在连接该环境绑定的上游代理。")
	conn, err := (&net.Dialer{Timeout: 6 * time.Second, KeepAlive: 30 * time.Second}).DialContext(startup, "tcp", net.JoinHostPort(b.config.Host, strconv.Itoa(b.config.Port)))
	if err != nil {
		return nil, &CheckError{Code: "PROXY_UNREACHABLE", Message: "该环境的上游代理无法连接，没有直连目标或切换节点。", Retryable: true}
	}
	if !b.track(conn) {
		return nil, &CheckError{Code: "PROXY_BRIDGE_UNAVAILABLE", Message: "本次通道已关闭，未发起目标请求。", Retryable: true}
	}
	b.event(probe, "upstream-connection", "passed", "实际TCP连接已到达绑定的上游代理。")
	if b.config.Type == "https" {
		tlsConn := tls.Client(conn, &tls.Config{ServerName: b.config.Host, MinVersion: tls.VersionTLS12, RootCAs: b.opts.RootCAs})
		if err := tlsConn.HandshakeContext(startup); err != nil {
			_ = conn.Close()
			b.untrack(conn)
			b.event(probe, "proxy-tls", "failed", "到上游代理的TLS验证失败，未跳过证书验证。")
			return nil, &CheckError{Code: "PROXY_TLS_FAILED", Message: "到HTTPS代理的证书、主机名或握手验证失败；没有关闭验证。", Retryable: false}
		}
		b.untrack(conn)
		if !b.track(tlsConn) {
			return nil, &CheckError{Code: "PROXY_BRIDGE_UNAVAILABLE", Message: "TLS建立期间本次通道关闭。", Retryable: true}
		}
		conn = tlsConn
		b.event(probe, "proxy-tls", "passed", "到HTTPS上游代理的TLS证书和主机名验证通过。")
	}
	return conn, nil
}

// Bound upstream response headers without discarding bytes already buffered
// after a CONNECT header. No arbitrary peer diagnostics enter host reports.
type headerBoundReader struct {
	reader    io.Reader
	remaining int64
	bounded   bool
}

func (r *headerBoundReader) Read(p []byte) (int, error) {
	if r.bounded {
		if r.remaining <= 0 {
			return 0, errors.New("proxy-response-header-too-large")
		}
		if int64(len(p)) > r.remaining {
			p = p[:r.remaining]
		}
	}
	n, err := r.reader.Read(p)
	if r.bounded {
		r.remaining -= int64(n)
	}
	return n, err
}
func readProxyResponse(conn net.Conn, request *http.Request) (*http.Response, *bufio.Reader, error) {
	limited := &headerBoundReader{reader: conn, remaining: 64 << 10, bounded: true}
	reader := bufio.NewReader(limited)
	for interim := 0; interim < 16; interim++ {
		response, err := http.ReadResponse(reader, request)
		if err != nil {
			return nil, reader, err
		}
		if response.StatusCode == http.StatusSwitchingProtocols {
			return nil, reader, errors.New("unsupported-proxy-upgrade")
		}
		if response.StatusCode >= 200 {
			limited.bounded = false
			return response, reader, nil
		}
		// 100/103 are not the final response. Keep the combined header bound
		// and connection deadline across every informational block.
		_ = response.Body.Close()
	}
	return nil, reader, errors.New("too-many-informational-responses")
}
func upstreamStatus(status int) *CheckError {
	if status == http.StatusProxyAuthRequired {
		return &CheckError{Code: "PROXY_AUTH_FAILED", Message: "绑定代理拒绝本次认证，未无认证重试或直连。", Retryable: false}
	}
	return &CheckError{Code: "PROXY_TARGET_FAILED", Message: "上游代理未允许本次目标连接，未切换直连。", Retryable: true}
}
func proxyDestination(value string) (string, error) {
	host, portText, err := net.SplitHostPort(value)
	if err != nil {
		return "", errors.New("invalid proxy target authority")
	}
	port, err := strconv.Atoi(portText)
	if err != nil {
		return "", err
	}
	config, err := Normalize(Configuration{Name: "目标", Type: "http", Host: host, Port: port})
	if err != nil {
		return "", err
	}
	return net.JoinHostPort(config.Host, strconv.Itoa(config.Port)), nil
}
func cleanProxyHeaders(header http.Header) {
	for _, field := range strings.Split(header.Get("Connection"), ",") {
		if field = strings.TrimSpace(field); field != "" {
			header.Del(field)
		}
	}
	for _, field := range []string{"Connection", "Proxy-Connection", "Keep-Alive", "Proxy-Authenticate", "Proxy-Authorization", "TE", "Trailer", "Transfer-Encoding", "Upgrade"} {
		header.Del(field)
	}
}

func (b *Bridge) connect(w http.ResponseWriter, r *http.Request, probe *bridgeProbe) {
	target, err := proxyDestination(r.Host)
	if err != nil || r.ContentLength > 0 || len(r.TransferEncoding) != 0 {
		b.reject(w, r.Context(), probe, &CheckError{Code: "PROXY_TARGET_FAILED", Message: "本次CONNECT目标或请求格式无效，未直连解析目标。", Retryable: false})
		return
	}
	upstream, failure := b.dial(r.Context(), probe)
	if failure != nil {
		b.reject(w, r.Context(), probe, failure)
		return
	}
	defer func() { _ = upstream.Close(); b.untrack(upstream) }()
	stop := context.AfterFunc(r.Context(), func() { _ = upstream.Close() })
	defer stop()
	_ = upstream.SetDeadline(time.Now().Add(15 * time.Second))
	var buffered io.Reader = upstream
	if b.config.Type == "socks5" {
		if failure := b.socksConnect(r.Context(), upstream, target, probe); failure != nil {
			b.reject(w, r.Context(), probe, failure)
			return
		}
	} else {
		request := &http.Request{Method: http.MethodConnect, URL: &url.URL{Opaque: target}, Host: target, Header: make(http.Header)}
		if b.upstreamAuth != "" {
			request.Header.Set("Proxy-Authorization", b.upstreamAuth)
		}
		if err := request.Write(upstream); err != nil {
			b.reject(w, r.Context(), probe, &CheckError{Code: "PROXY_UNREACHABLE", Message: "发送上游隧道请求失败，未使用直接目标连接。", Retryable: true})
			return
		}
		response, reader, err := readProxyResponse(upstream, request)
		if err != nil {
			b.reject(w, r.Context(), probe, &CheckError{Code: "PROXY_TARGET_FAILED", Message: "上游隧道响应无效或超过时限，不回显响应正文。", Retryable: true})
			return
		}
		if response.StatusCode != http.StatusOK {
			b.reject(w, r.Context(), probe, upstreamStatus(response.StatusCode))
			return
		}
		b.event(probe, "upstream-authentication", "passed", "绑定上游接受本次隧道及保存的认证策略；凭据不传给目标。")
		buffered = reader
	}
	_ = upstream.SetDeadline(time.Time{})
	client, buffer, err := w.(http.Hijacker).Hijack()
	if err != nil {
		return
	}
	defer func() { _ = client.Close(); b.untrack(client) }()
	_, err = buffer.WriteString("HTTP/1.1 200 Connection Established\r\n\r\n")
	if err != nil || buffer.Flush() != nil {
		return
	}
	_ = client.SetDeadline(time.Time{})
	finished := make(chan struct{}, 2)
	copyHalf := func(destination net.Conn, source io.Reader) {
		_, _ = io.Copy(destination, source)
		if half, ok := destination.(interface{ CloseWrite() error }); ok {
			_ = half.CloseWrite()
		}
		finished <- struct{}{}
	}
	go copyHalf(upstream, buffer)
	go copyHalf(client, buffered)
	<-finished
	<-finished
}

func (b *Bridge) forwardHTTP(w http.ResponseWriter, r *http.Request, probe *bridgeProbe) {
	if r.URL == nil || r.URL.Scheme != "http" || r.URL.Hostname() == "" || r.URL.User != nil || r.URL.Fragment != "" || r.Header.Get("Upgrade") != "" {
		b.reject(w, r.Context(), probe, &CheckError{Code: "PROXY_TARGET_FAILED", Message: "本机通道仅转发标准HTTP和CONNECT；非法目标或升级请求被阻断。", Retryable: false})
		return
	}
	port := r.URL.Port()
	if port == "" {
		port = "80"
	}
	target, err := proxyDestination(net.JoinHostPort(r.URL.Hostname(), port))
	if err != nil {
		b.reject(w, r.Context(), probe, &CheckError{Code: "PROXY_TARGET_FAILED", Message: "HTTP目标格式无效，未建立目标直连。", Retryable: false})
		return
	}
	upstream, failure := b.dial(r.Context(), probe)
	if failure != nil {
		b.reject(w, r.Context(), probe, failure)
		return
	}
	defer func() { _ = upstream.Close(); b.untrack(upstream) }()
	stop := context.AfterFunc(r.Context(), func() { _ = upstream.Close() })
	defer stop()
	_ = upstream.SetDeadline(time.Now().Add(25 * time.Second))
	if b.config.Type == "socks5" {
		if failure := b.socksConnect(r.Context(), upstream, target, probe); failure != nil {
			b.reject(w, r.Context(), probe, failure)
			return
		}
	}
	request := r.Clone(r.Context())
	request.RequestURI = ""
	request.Header = r.Header.Clone()
	request.Trailer = r.Trailer.Clone()
	cleanProxyHeaders(request.Header)
	cleanProxyHeaders(request.Trailer)
	request.Close = true
	var body *proxyRequestBody
	if request.Body != nil && request.Body != http.NoBody {
		body = &proxyRequestBody{ReadCloser: request.Body, expected: request.ContentLength}
		request.Body = body
	}
	if b.upstreamAuth != "" {
		request.Header.Set("Proxy-Authorization", b.upstreamAuth)
	}
	if b.config.Type == "socks5" {
		err = request.Write(upstream)
	} else {
		err = request.WriteProxy(upstream)
	}
	if err != nil {
		failure := &CheckError{Code: "PROXY_UNREACHABLE", Message: "发送绑定上游的HTTP请求失败，未直接连接目标。", Retryable: true}
		if body != nil && body.sourceFailed() {
			failure.Code, failure.Message = "PROXY_REQUEST_FAILED", "本次客户端上传正文中断，仅关闭本次连接，未判定整个代理失效。"
		} else if b.config.Type == "socks5" {
			// After SOCKS CONNECT, writes go to the target, not the proxy
			// protocol. A rejected upload is not an upstream auth failure.
			failure.Code, failure.Message = "PROXY_TARGET_FAILED", "本次目标HTTP请求未能发送，仅关闭本次连接，没有直连或替换代理。"
		}
		b.reject(w, r.Context(), probe, failure)
		return
	}
	response, _, err := readProxyResponse(upstream, request)
	if err != nil {
		b.reject(w, r.Context(), probe, &CheckError{Code: "PROXY_TARGET_FAILED", Message: "上游HTTP响应无效或超过时限，未回退直连。", Retryable: true})
		return
	}
	defer response.Body.Close()
	if b.config.Type != "socks5" && response.StatusCode == http.StatusProxyAuthRequired {
		b.reject(w, r.Context(), probe, upstreamStatus(response.StatusCode))
		return
	}
	_ = upstream.SetDeadline(time.Time{})
	cleanProxyHeaders(response.Header)
	for field, values := range response.Header {
		w.Header()[field] = append([]string(nil), values...)
	}
	w.Header().Set("Connection", "close")
	w.WriteHeader(response.StatusCode)
	controller := http.NewResponseController(w)
	if controller.Flush() != nil {
		return
	}
	_, _ = io.Copy(flushingProxyWriter{writer: w, controller: controller}, response.Body)
}

type flushingProxyWriter struct {
	writer     io.Writer
	controller *http.ResponseController
}

func (w flushingProxyWriter) Write(value []byte) (int, error) {
	count, err := w.writer.Write(value)
	if err == nil {
		err = w.controller.Flush()
	}
	return count, err
}
