package proxy

import (
	"context"
	"crypto/x509"
	"encoding/binary"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type socksFixtureOptions struct {
	credentials   *Credentials
	selectOther   bool
	method        byte
	authReject    bool
	connectStatus byte
	badReply      bool
	boundType     byte
	targetAddress string // Explicit loopback mapping: the fixture, not host DNS, resolves the requested name.
}
type socksFixtureRequest struct {
	methods              []byte
	user, password, host string
	port                 int
	addressType          byte
}

// Controlled RFC1928/1929 fixture only. Never consults a resolver for a target;
// every synthetic domain/IP is mapped to an explicitly supplied loopback origin.
func controlledSOCKS5(t *testing.T, options socksFixtureOptions) (Configuration, <-chan socksFixtureRequest) {
	t.Helper()
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	observations := make(chan socksFixtureRequest, 32)
	var mu sync.Mutex
	var workers sync.WaitGroup
	owned := map[net.Conn]bool{}
	closed := false
	track := func(conn net.Conn) bool {
		mu.Lock()
		defer mu.Unlock()
		if closed {
			_ = conn.Close()
			return false
		}
		owned[conn] = true
		return true
	}
	untrack := func(conn net.Conn) { _ = conn.Close(); mu.Lock(); delete(owned, conn); mu.Unlock() }
	acceptDone := make(chan struct{})
	go func() {
		defer close(acceptDone)
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			if !track(conn) {
				return
			}
			workers.Add(1)
			go func() {
				defer workers.Done()
				defer untrack(conn)
				_ = conn.SetDeadline(time.Now().Add(4 * time.Second))
				request := socksFixtureRequest{}
				defer func() {
					select {
					case observations <- request:
					default:
					}
				}()
				header := make([]byte, 2)
				if _, err := io.ReadFull(conn, header); err != nil || header[0] != 5 {
					return
				}
				request.methods = make([]byte, int(header[1]))
				if _, err := io.ReadFull(conn, request.methods); err != nil {
					return
				}
				method := byte(0)
				if options.credentials != nil {
					method = 2
				}
				if options.selectOther {
					method = options.method
				}
				if writeSOCKSPacket(conn, []byte{5, method}) != nil || options.selectOther {
					return
				}
				if method == 2 {
					if _, err := io.ReadFull(conn, header); err != nil || header[0] != 1 {
						return
					}
					value := make([]byte, int(header[1]))
					if _, err := io.ReadFull(conn, value); err != nil {
						return
					}
					request.user = string(value)
					if _, err := io.ReadFull(conn, header[:1]); err != nil {
						return
					}
					value = make([]byte, int(header[0]))
					if _, err := io.ReadFull(conn, value); err != nil {
						return
					}
					request.password = string(value)
					status := byte(0)
					if options.authReject || request.user != options.credentials.Username || request.password != options.credentials.Password {
						status = 1
					}
					if writeSOCKSPacket(conn, []byte{1, status}) != nil || status != 0 {
						return
					}
				}
				prefix := make([]byte, 4)
				if _, err := io.ReadFull(conn, prefix); err != nil || prefix[0] != 5 || prefix[1] != 1 || prefix[2] != 0 {
					return
				}
				request.addressType = prefix[3]
				length := 0
				switch prefix[3] {
				case 1:
					length = 4
				case 4:
					length = 16
				case 3:
					if _, err := io.ReadFull(conn, header[:1]); err != nil {
						return
					}
					length = int(header[0])
				default:
					return
				}
				address := make([]byte, length)
				if _, err := io.ReadFull(conn, address); err != nil {
					return
				}
				if prefix[3] == 3 {
					request.host = string(address)
				} else {
					request.host = net.IP(address).String()
				}
				if _, err := io.ReadFull(conn, header); err != nil {
					return
				}
				request.port = int(binary.BigEndian.Uint16(header))
				if options.badReply {
					_ = writeSOCKSPacket(conn, []byte{4, 0, 0, 1})
					return
				}
				bound := []byte{5, options.connectStatus, 0, 1, 127, 0, 0, 1, 0, 80}
				if options.boundType == 4 {
					bound = []byte{5, options.connectStatus, 0, 4}
					ip := netip.MustParseAddr("2001:db8::1").As16()
					bound = append(bound, ip[:]...)
					bound = append(bound, 0, 80)
				}
				if options.boundType == 3 {
					bound = append([]byte{5, options.connectStatus, 0, 3, 7}, []byte("fixture")...)
					bound = append(bound, 0, 80)
				}
				if writeSOCKSPacket(conn, bound) != nil || options.connectStatus != 0 {
					return
				}
				if options.targetAddress == "" {
					return
				}
				origin, err := net.DialTimeout("tcp", options.targetAddress, time.Second)
				if err != nil {
					return
				}
				if !track(origin) {
					return
				}
				defer untrack(origin)
				_ = origin.SetDeadline(time.Now().Add(4 * time.Second))
				finished := make(chan struct{}, 2)
				go func() { _, _ = io.Copy(origin, conn); finished <- struct{}{} }()
				go func() { _, _ = io.Copy(conn, origin); finished <- struct{}{} }()
				<-finished
				_ = conn.Close()
				_ = origin.Close()
				<-finished
			}()
		}
	}()
	t.Cleanup(func() {
		mu.Lock()
		closed = true
		conns := make([]net.Conn, 0, len(owned))
		for conn := range owned {
			conns = append(conns, conn)
		}
		mu.Unlock()
		_ = listener.Close()
		for _, conn := range conns {
			_ = conn.Close()
		}
		<-acceptDone
		workers.Wait()
	})
	address := listener.Addr().(*net.TCPAddr)
	return Configuration{Name: "合成受控SOCKS5", Type: "socks5", Host: "127.0.0.1", Port: address.Port}, observations
}
func waitSOCKSRequest(t *testing.T, observations <-chan socksFixtureRequest) socksFixtureRequest {
	t.Helper()
	select {
	case request := <-observations:
		return request
	case <-time.After(5 * time.Second):
		t.Fatal("SOCKS5 fixture did not finish observing the owned request")
		return socksFixtureRequest{}
	}
}

func TestSOCKS5PreflightAndBrowserReuseSameChannelWithBothAuthenticationMethods(t *testing.T) {
	for _, authenticated := range []bool{false, true} {
		var targets atomic.Int32
		target := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			targets.Add(1)
			if r.Header.Get("Proxy-Authorization") != "" {
				t.Error("SOCKS5 credentials reached target")
			}
			_, _ = io.WriteString(w, `{"ip":"203.0.113.101"}`)
		}))
		t.Cleanup(target.Close)
		var credentials *Credentials
		if authenticated {
			credentials = &Credentials{Username: "synthetic:user", Password: "SYNTHETIC_SOCKS_PASSWORD"}
		}
		config, observations := controlledSOCKS5(t, socksFixtureOptions{credentials: credentials, targetAddress: target.Listener.Addr().String(), boundType: 3})
		trust := x509.NewCertPool()
		trust.AddCert(target.Certificate())
		b, err := OpenBridge(config, credentials, BridgeOptions{ChannelID: "synthetic-socks-session", AuthorizeProbe: func(net.Conn) bool { return true }, TargetURL: target.URL, RootCAs: trust})
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = b.Close() })
		endpoint := b.Endpoint()
		report := b.Preflight(context.Background(), nil)
		if report.Error != nil || report.ChannelID != b.ID() || report.ResolutionPolicy != SOCKS5ResolutionPolicy || report.ExitIP != "203.0.113.101" {
			t.Fatal("SOCKS5 preflight failed to preserve the shared bridge report")
		}
		request := waitSOCKSRequest(t, observations)
		wantMethod := byte(0)
		if authenticated {
			wantMethod = 2
		}
		if len(request.methods) != 1 || request.methods[0] != wantMethod {
			t.Fatal("bridge offered an implicit auth downgrade")
		}
		if err := b.BindBrowser(func(net.Conn) bool { return true }); err != nil {
			t.Fatal(err)
		}
		client := bridgeBrowserClient(t, b)
		client.Transport.(*http.Transport).TLSClientConfig = target.Client().Transport.(*http.Transport).TLSClientConfig.Clone()
		response, err := client.Get(target.URL)
		if err != nil {
			t.Fatal(err)
		}
		body, _ := io.ReadAll(response.Body)
		response.Body.Close()
		if string(body) != `{"ip":"203.0.113.101"}` || targets.Load() != 2 || b.Endpoint() != endpoint {
			t.Fatal("HTTPS browser traffic did not use the checked SOCKS5 listener")
		}
		_ = waitSOCKSRequest(t, observations)
		encoded, _ := json.Marshal(report)
		if strings.Contains(string(encoded), "SYNTHETIC_SOCKS_PASSWORD") || strings.Contains(string(encoded), "synthetic:user") || strings.Contains(string(encoded), endpoint) {
			t.Fatal("report exposed authentication or private listener")
		}
	}
}

func TestSOCKS5HTTPDomainTargetsResolveAtUpstreamWithoutHostDNSOrProxyHeaders(t *testing.T) {
	for _, destination := range []struct {
		host        string
		addressType byte
		canonical   string
	}{{"remote-only.synthetic.invalid", 3, "remote-only.synthetic.invalid"}, {"münich.synthetic.invalid", 3, "xn--mnich-kva.synthetic.invalid"}, {"203.0.113.102", 1, "203.0.113.102"}, {"[2001:db8::102]", 4, "2001:db8::102"}} {
		var targets atomic.Int32
		target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			targets.Add(1)
			if r.URL.IsAbs() || r.Header.Get("Proxy-Authorization") != "" || r.Header.Get("Proxy-Connection") != "" {
				t.Error("SOCKS5-origin HTTP was proxy-form or leaked hop credentials")
			}
			_, _ = io.WriteString(w, "SYNTHETIC_REMOTE_DNS_ORIGIN")
		}))
		t.Cleanup(target.Close)
		credentials := &Credentials{Username: "synthetic-user", Password: "synthetic-pass"}
		config, observations := controlledSOCKS5(t, socksFixtureOptions{credentials: credentials, targetAddress: target.Listener.Addr().String(), boundType: 4})
		b, err := OpenBridge(config, credentials, BridgeOptions{ChannelID: "synthetic-remote-dns", AuthorizeProbe: func(net.Conn) bool { return false }})
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = b.Close() })
		if b.BindBrowser(func(net.Conn) bool { return true }) != nil {
			t.Fatal("guard bind failed")
		}
		request, _ := http.NewRequest("GET", "http://"+destination.host+":18080/page", nil)
		request.Header.Set("Proxy-Authorization", "SYNTHETIC_CLIENT_AUTH_MUST_NOT_LEAK")
		request.Header.Set("Proxy-Connection", "keep-alive")
		response, err := bridgeBrowserClient(t, b).Do(request)
		if err != nil {
			t.Fatal(err)
		}
		body, _ := io.ReadAll(response.Body)
		response.Body.Close()
		observed := waitSOCKSRequest(t, observations)
		if observed.host != destination.canonical || observed.addressType != destination.addressType || observed.port != 18080 || string(body) != "SYNTHETIC_REMOTE_DNS_ORIGIN" || targets.Load() != 1 {
			t.Fatal("destination was locally resolved, converted to wrong address type, or bypassed configured SOCKS5")
		}
	}
}

func TestSOCKS5AuthNegotiationAndAddressFailuresStayDistinctAndNeverReachOrigin(t *testing.T) {
	var targets atomic.Int32
	target := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		targets.Add(1)
		_, _ = io.WriteString(w, `{"ip":"203.0.113.103"}`)
	}))
	t.Cleanup(target.Close)
	credentials := &Credentials{Username: "synthetic-user", Password: "synthetic-pass"}
	for _, test := range []struct {
		options socksFixtureOptions
		code    string
	}{{socksFixtureOptions{selectOther: true, method: 0}, "PROXY_SOCKS_NEGOTIATION_FAILED"}, {socksFixtureOptions{selectOther: true, method: 255}, "PROXY_SOCKS_NEGOTIATION_FAILED"}, {socksFixtureOptions{authReject: true}, "PROXY_AUTH_FAILED"}, {socksFixtureOptions{connectStatus: 8}, "PROXY_TARGET_UNSUPPORTED"}, {socksFixtureOptions{connectStatus: 4}, "PROXY_TARGET_UNREACHABLE"}, {socksFixtureOptions{badReply: true}, "PROXY_SOCKS_NEGOTIATION_FAILED"}} {
		test.options.credentials, test.options.targetAddress = credentials, target.Listener.Addr().String()
		config, observations := controlledSOCKS5(t, test.options)
		b, err := OpenBridge(config, credentials, BridgeOptions{ChannelID: "synthetic-failed-socks", AuthorizeProbe: func(net.Conn) bool { return true }, TargetURL: target.URL})
		if err != nil {
			t.Fatal(err)
		}
		report := b.Preflight(context.Background(), nil)
		if report.Error == nil || report.Error.Code != test.code || report.ResolutionPolicy != SOCKS5ResolutionPolicy || targets.Load() != 0 {
			t.Fatal("SOCKS5 protocol failure was misclassified or bypassed to target")
		}
		for _, step := range report.Steps {
			if step.Stage == "result" && step.Message != report.Error.Message {
				t.Fatal("final error and step describe different causes")
			}
		}
		_ = b.Close()
		_ = waitSOCKSRequest(t, observations)
	}
}

func TestSOCKS5CredentialsUseWireByteLimitsAndDoNotApplyHTTPUsernameDelimiters(t *testing.T) {
	for _, value := range []Credentials{{Username: "", Password: "password"}, {Username: "user", Password: ""}, {Username: strings.Repeat("a", 256), Password: "p"}, {Username: strings.Repeat("用", 86), Password: "p"}, {Username: "user", Password: strings.Repeat("密", 86)}, {Username: "bad\nuser", Password: "p"}} {
		if ValidateProtocolCredentials("socks5", value) == nil {
			t.Fatal("invalid/oversized SOCKS5 auth accepted")
		}
	}
	valid := Credentials{Username: "synthetic:user", Password: strings.Repeat("密", 85)}
	if ValidateProtocolCredentials("socks5", valid) != nil || ValidateProtocolCredentials("http", valid) == nil || ValidateStoredCredentials(valid) != nil {
		t.Fatal("wire-specific auth rules were conflated with protected storage or HTTP Basic")
	}
	if _, _, err := ParseLine("socks5://synthetic%3Auser:synthetic-pass@localhost:1080"); err != nil {
		t.Fatal("percent-encoded SOCKS5 username delimiter rejected")
	}
	if _, _, err := ParseLine("socks5://user:" + strings.Repeat("a", 256) + "@localhost:1080"); err == nil {
		t.Fatal("import accepted credentials that cannot be sent on the wire")
	}
	config := Configuration{Name: "合成认证范围", Type: "socks5", Host: "127.0.0.1", Port: 1080}
	if _, err := OpenBridge(config, &Credentials{Username: "u", Password: strings.Repeat("a", 256)}, BridgeOptions{ChannelID: "synthetic-invalid-secret", AuthorizeProbe: func(net.Conn) bool { return true }}); err == nil {
		t.Fatal("invalid persisted auth established a bridge or tried no auth")
	}
}

func TestSOCKS5DestinationEncodingNeverLooksUpOrSilentlyChangesAddressFamily(t *testing.T) {
	for _, test := range []struct {
		target      string
		addressType byte
		stage       string
	}{{"remote-only.synthetic.invalid:443", 3, "domain-target"}, {"198.51.100.1:443", 1, "ipv4-target"}, {"[2001:db8::1]:443", 4, "ipv6-target"}} {
		packet, stage, failure := socksDestination(test.target)
		if failure != nil || stage != test.stage || packet[3] != test.addressType || binary.BigEndian.Uint16(packet[len(packet)-2:]) != 443 {
			t.Fatal("SOCKS5 target encoding selected local DNS/wrong family")
		}
	}
	for _, invalid := range []string{"host:0", "host:65536", "[fe80::1%zone]:443", "http://host:443", "host"} {
		if _, _, failure := socksDestination(invalid); failure == nil {
			t.Fatal("invalid address changed into a valid target")
		}
	}
}

func TestSOCKS5Origin407IsNotReportedAsUpstreamAuthenticationFailure(t *testing.T) {
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(407)
		_, _ = io.WriteString(w, "SYNTHETIC_ORIGIN_STATUS")
	}))
	t.Cleanup(target.Close)
	config, observations := controlledSOCKS5(t, socksFixtureOptions{targetAddress: target.Listener.Addr().String()})
	b, err := OpenBridge(config, nil, BridgeOptions{ChannelID: "synthetic-origin-status", AuthorizeProbe: func(net.Conn) bool { return false }})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = b.Close() })
	if b.BindBrowser(func(net.Conn) bool { return true }) != nil {
		t.Fatal("guard bind failed")
	}
	response, err := bridgeBrowserClient(t, b).Get("http://remote-only.synthetic.invalid/")
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(response.Body)
	response.Body.Close()
	_ = waitSOCKSRequest(t, observations)
	if response.StatusCode != 407 || string(body) != "SYNTHETIC_ORIGIN_STATUS" {
		t.Fatal("origin 407 was replaced with proxy credential failure")
	}
}

func TestSOCKS5UnreachableUpstreamAndIndependentChannelCloseDoNotFallback(t *testing.T) {
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = listener.Close() })
	port := listener.Addr().(*net.TCPAddr).Port
	config := Configuration{Name: "合成不可达", Type: "socks5", Host: "127.0.0.1", Port: port}
	opts := BridgeOptions{ChannelID: "synthetic-socks-A", AuthorizeProbe: func(net.Conn) bool { return true }, TargetURL: "https://remote-only.synthetic.invalid/"}
	a, err := OpenBridge(config, nil, opts)
	if err != nil {
		t.Fatal(err)
	}
	opts.ChannelID = "synthetic-socks-B"
	b, err := OpenBridge(config, nil, opts)
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close()
	t.Cleanup(func() { _ = a.Close() })
	_ = listener.Close()
	report := a.Preflight(context.Background(), nil)
	if report.Error == nil || report.Error.Code != "PROXY_UNREACHABLE" {
		t.Fatal("upstream unreachable became negotiation success or direct DNS target request")
	}
	if a.Endpoint() == b.Endpoint() {
		t.Fatal("independent environments shared SOCKS5 channel")
	}
	endpoint, _ := url.Parse(a.Endpoint())
	port, _ = strconv.Atoi(endpoint.Port())
	_ = a.Close()
	select {
	case <-a.Done():
	default:
		t.Fatal("closed channel has unfinished owned resources")
	}
	select {
	case <-b.Done():
		t.Fatal("closing A ended B")
	default:
	}
	probe, err := net.DialTimeout("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(port)), time.Second)
	if err == nil {
		_ = probe.Close()
		t.Fatal("closed SOCKS5 channel retained listener")
	}
}
