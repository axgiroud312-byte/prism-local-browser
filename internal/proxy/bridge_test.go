package proxy

import (
	"context"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func bridgeBrowserClient(t *testing.T, b *Bridge) *http.Client {
	t.Helper()
	endpoint, err := url.Parse(b.Endpoint())
	if err != nil {
		t.Fatal(err)
	}
	transport := &http.Transport{Proxy: http.ProxyURL(endpoint), DisableKeepAlives: true}
	t.Cleanup(transport.CloseIdleConnections)
	return &http.Client{Transport: transport, Timeout: 3 * time.Second}
}

func TestBridgePreflightAndHTTPSRequestsUseTheSameAuthenticatedListener(t *testing.T) {
	for _, tlsProxy := range []bool{false, true} {
		var targets, tunnels atomic.Int32
		target := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			targets.Add(1)
			if r.Header.Get("Proxy-Authorization") != "" {
				t.Error("proxy secret reached HTTPS target")
			}
			_, _ = io.WriteString(w, `{"ip":"203.0.113.81"}`)
		}))
		t.Cleanup(target.Close)
		credentials := &Credentials{Username: "SYNTHETIC_CHANNEL_USER", Password: "SYNTHETIC_CHANNEL_PASSWORD"}
		auth := "Basic " + base64.StdEncoding.EncodeToString([]byte(credentials.Username+":"+credentials.Password))
		upstream := controlledProxy(t, target, tlsProxy, auth, &tunnels)
		trust := x509.NewCertPool()
		trust.AddCert(target.Certificate())
		if tlsProxy {
			trust.AddCert(upstream.Certificate())
		}
		b, err := OpenBridge(checkFixtureConfig(t, upstream), credentials, BridgeOptions{ChannelID: "synthetic-same-channel", AuthorizeProbe: func(net.Conn) bool { return true }, TargetURL: target.URL, RootCAs: trust})
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = b.Close() })
		initial := b.Endpoint()
		report := b.Preflight(context.Background(), nil)
		if report.Error != nil || report.ChannelID != b.ID() || report.ExitIP != "203.0.113.81" || b.Endpoint() != initial || targets.Load() != 1 {
			t.Fatal("preflight did not use the real channel instance")
		}
		encoded, _ := json.Marshal(report)
		if strings.Contains(string(encoded), "SYNTHETIC_CHANNEL") || strings.Contains(string(encoded), initial) || strings.Contains(string(encoded), "prism-probe") {
			t.Fatal("ordinary report leaked private channel material")
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
		if string(body) != `{"ip":"203.0.113.81"}` || targets.Load() != 2 || tunnels.Load() != 2 || b.Endpoint() != initial {
			t.Fatal("subsequent HTTPS traffic switched away from the checked listener")
		}
	}
}

func TestBridgeHTTPForwardingUsesUpstreamAuthenticationNotClientCredentials(t *testing.T) {
	for _, tlsProxy := range []bool{false, true} {
		var targets, forwarded atomic.Int32
		target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			targets.Add(1)
			if r.Header.Get("Proxy-Authorization") != "" || r.Header.Get("Proxy-Connection") != "" {
				t.Error("hop credentials entered HTTP origin")
			}
			_, _ = io.WriteString(w, "SYNTHETIC_HTTP_ORIGIN")
		}))
		t.Cleanup(target.Close)
		expectedAuth := "Basic " + base64.StdEncoding.EncodeToString([]byte("synthetic-user:SYNTHETIC_STORED_PASSWORD"))
		transport := &http.Transport{Proxy: nil, DisableKeepAlives: true}
		t.Cleanup(transport.CloseIdleConnections)
		upstream := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Header.Get("Proxy-Authorization") != expectedAuth || !r.URL.IsAbs() || r.URL.Host != target.Listener.Addr().String() {
				w.WriteHeader(407)
				return
			}
			forwarded.Add(1)
			request := r.Clone(r.Context())
			request.RequestURI = ""
			request.Header.Del("Proxy-Authorization")
			response, err := transport.RoundTrip(request)
			if err != nil {
				w.WriteHeader(502)
				return
			}
			defer response.Body.Close()
			w.WriteHeader(response.StatusCode)
			_, _ = io.Copy(w, response.Body)
		}))
		if tlsProxy {
			upstream.StartTLS()
		} else {
			upstream.Start()
		}
		t.Cleanup(upstream.Close)
		trust := x509.NewCertPool()
		if tlsProxy {
			trust.AddCert(upstream.Certificate())
		}
		b, err := OpenBridge(checkFixtureConfig(t, upstream), &Credentials{Username: "synthetic-user", Password: "SYNTHETIC_STORED_PASSWORD"}, BridgeOptions{ChannelID: "synthetic-http-channel", AuthorizeProbe: func(net.Conn) bool { return false }, RootCAs: trust})
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = b.Close() })
		if b.BindBrowser(func(net.Conn) bool { return true }) != nil {
			t.Fatal("fixture guard bind failed")
		}
		request, _ := http.NewRequest("GET", target.URL, nil)
		request.Header.Set("Proxy-Authorization", "SYNTHETIC_CLIENT_PASSWORD_MUST_NOT_WIN")
		response, err := bridgeBrowserClient(t, b).Do(request)
		if err != nil {
			t.Fatal(err)
		}
		body, _ := io.ReadAll(response.Body)
		response.Body.Close()
		if string(body) != "SYNTHETIC_HTTP_ORIGIN" || targets.Load() != 1 || forwarded.Load() != 1 {
			t.Fatal("HTTP forward bypassed the configured upstream")
		}
	}
}

func TestBridgeCallerDenialAuthAndTLSFailureNeverReachTargetOrFallback(t *testing.T) {
	var targets, tunnels atomic.Int32
	target := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		targets.Add(1)
		_, _ = io.WriteString(w, `{"ip":"203.0.113.82"}`)
	}))
	defer target.Close()
	upstream := controlledProxy(t, target, true, "Basic expected-synthetic-auth", &tunnels)
	trust := x509.NewCertPool()
	trust.AddCert(target.Certificate())
	trust.AddCert(upstream.Certificate())
	for _, test := range []struct {
		trust *x509.CertPool
		probe bool
		code  string
	}{{trust, false, "PROXY_TARGET_FAILED"}, {nil, true, "PROXY_TLS_FAILED"}, {trust, true, "PROXY_AUTH_FAILED"}} {
		b, err := OpenBridge(checkFixtureConfig(t, upstream), &Credentials{Username: "wrong", Password: "synthetic-wrong"}, BridgeOptions{ChannelID: "synthetic-denied-channel", AuthorizeProbe: func(net.Conn) bool { return test.probe }, TargetURL: target.URL, RootCAs: test.trust})
		if err != nil {
			t.Fatal(err)
		}
		report := b.Preflight(context.Background(), nil)
		if report.Error == nil || report.Error.Code != test.code || targets.Load() != 0 || tunnels.Load() != 0 {
			t.Fatal("guard/auth/TLS failure bypassed the upstream or lost its actual reason")
		}
		if err := b.BindBrowser(func(net.Conn) bool { return false }); err != nil {
			t.Fatal(err)
		}
		response, err := bridgeBrowserClient(t, b).Get("http://synthetic-target.invalid/")
		if err != nil {
			t.Fatal(err)
		}
		response.Body.Close()
		if response.StatusCode != 403 {
			t.Fatal("an unrelated client used a loopback bridge")
		}
		_ = b.Close()
	}
}

func TestClosingOneBridgeClosesItsInFlightCONNECTButNotAnotherListener(t *testing.T) {
	entered, release := make(chan struct{}), make(chan struct{})
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { close(entered); <-release; w.WriteHeader(407) }))
	defer upstream.Close()
	defer close(release)
	options := BridgeOptions{ChannelID: "synthetic-A", AuthorizeProbe: func(net.Conn) bool { return true }, TargetURL: "https://synthetic-target.invalid/"}
	a, err := OpenBridge(checkFixtureConfig(t, upstream), nil, options)
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	options.ChannelID = "synthetic-B"
	b, err := OpenBridge(checkFixtureConfig(t, upstream), nil, options)
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close()
	if a.Endpoint() == b.Endpoint() || a.ID() == b.ID() {
		t.Fatal("two environments shared a bridge endpoint or identity")
	}
	probeDone := make(chan Report, 1)
	go func() { probeDone <- a.Preflight(context.Background(), nil) }()
	select {
	case <-entered:
	case <-time.After(2 * time.Second):
		t.Fatal("fixture CONNECT not reached")
	}
	closed := make(chan struct{})
	go func() { _ = a.Close(); close(closed) }()
	select {
	case <-closed:
	case <-time.After(2 * time.Second):
		t.Fatal("closing the channel retained a CONNECT socket")
	}
	select {
	case <-probeDone:
	case <-time.After(time.Second):
		t.Fatal("preflight remained stuck after the channel closed")
	}
	select {
	case <-b.Done():
		t.Fatal("closing A ended independent B")
	default:
	}
}

func TestBridgeForwardsFinalHTTPResponseAfterInformationalHeaders(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(103)
		w.WriteHeader(200)
		_, _ = io.WriteString(w, "SYNTHETIC_FINAL_RESPONSE")
	}))
	defer upstream.Close()
	b, err := OpenBridge(checkFixtureConfig(t, upstream), nil, BridgeOptions{ChannelID: "synthetic-informational", AuthorizeProbe: func(net.Conn) bool { return false }})
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close()
	if b.BindBrowser(func(net.Conn) bool { return true }) != nil {
		t.Fatal("fixture bind failed")
	}
	response, err := bridgeBrowserClient(t, b).Get("http://synthetic-origin.invalid/")
	if err != nil {
		t.Fatal(err)
	}
	body, err := io.ReadAll(response.Body)
	response.Body.Close()
	if err != nil || response.StatusCode != 200 || string(body) != "SYNTHETIC_FINAL_RESPONSE" {
		t.Fatal("103 ended the forward before its final status/body")
	}
}
func TestBridgeFlushesHTTPEventsBeforeUpstreamConnectionEnds(t *testing.T) {
	release := make(chan struct{})
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, "data: synthetic\n\n")
		w.(http.Flusher).Flush()
		<-release
	}))
	defer upstream.Close()
	defer close(release)
	b, err := OpenBridge(checkFixtureConfig(t, upstream), nil, BridgeOptions{ChannelID: "synthetic-stream", AuthorizeProbe: func(net.Conn) bool { return false }})
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close()
	if b.BindBrowser(func(net.Conn) bool { return true }) != nil {
		t.Fatal("fixture bind failed")
	}
	observed := make(chan string, 1)
	go func() {
		response, err := bridgeBrowserClient(t, b).Get("http://synthetic-origin.invalid/events")
		if err != nil {
			observed <- "request failed"
			return
		}
		defer response.Body.Close()
		value, _ := io.ReadAll(io.LimitReader(response.Body, int64(len("data: synthetic\n\n"))))
		observed <- string(value)
	}()
	select {
	case value := <-observed:
		if value != "data: synthetic\n\n" {
			t.Fatal("stream event changed or was not received")
		}
	case <-time.After(time.Second):
		t.Fatal("small event remained buffered until connection EOF")
	}
}
