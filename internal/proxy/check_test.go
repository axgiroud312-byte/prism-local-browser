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
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// Loopback-only authentication/tunnel fixture. Not an external supplier and
// not a normal browser proxy channel or runtime fail-closed verification.
func controlledProxy(t *testing.T, target *httptest.Server, useTLS bool, expectedAuth string, tunnels *atomic.Int32) *httptest.Server {
	t.Helper()
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodConnect || r.Host != target.Listener.Addr().String() {
			w.WriteHeader(403)
			return
		}
		if r.Header.Get("Proxy-Authorization") != expectedAuth {
			w.WriteHeader(http.StatusProxyAuthRequired)
			_, _ = io.WriteString(w, "SYNTHETIC_PEER_BODY_MUST_NOT_ENTER_LOGS")
			return
		}
		tunnels.Add(1)
		upstream, err := net.DialTimeout("tcp", target.Listener.Addr().String(), time.Second)
		if err != nil {
			w.WriteHeader(502)
			return
		}
		defer upstream.Close()
		_ = upstream.SetDeadline(time.Now().Add(5 * time.Second))
		client, buffer, err := w.(http.Hijacker).Hijack()
		if err != nil {
			return
		}
		defer client.Close()
		_ = client.SetDeadline(time.Now().Add(5 * time.Second))
		_, _ = buffer.WriteString("HTTP/1.1 200 Connection Established\r\n\r\n")
		_ = buffer.Flush()
		finished := make(chan struct{})
		go func() { _, _ = io.Copy(upstream, buffer); close(finished) }()
		_, _ = io.Copy(client, upstream)
		_ = client.Close()
		_ = upstream.Close()
		<-finished
	}))
	if useTLS {
		server.StartTLS()
	} else {
		server.Start()
	}
	t.Cleanup(server.Close)
	return server
}
func checkFixtureConfig(t *testing.T, server *httptest.Server) Configuration {
	t.Helper()
	u, err := url.Parse(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	port, err := strconv.Atoi(u.Port())
	if err != nil {
		t.Fatal(err)
	}
	return Configuration{Name: "合成受控代理", Type: u.Scheme, Host: u.Hostname(), Port: port}
}
func TestHTTPAndTLSProxyChecksActuallyTunnelAuthenticationAndExit(t *testing.T) {
	for _, useTLS := range []bool{false, true} {
		t.Run(strconv.FormatBool(useTLS), func(t *testing.T) {
			var targets, tunnels atomic.Int32
			target := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				targets.Add(1)
				if r.Header.Get("Proxy-Authorization") != "" {
					t.Error("upstream credentials leaked through the tunnel to the target")
				}
				_, _ = io.WriteString(w, `{"ip":"203.0.113.45"}`)
			}))
			defer target.Close()
			credentials := &Credentials{Username: "synthetic-user", Password: "SYNTHETIC_PROXY_SECRET"}
			auth := "Basic " + base64.StdEncoding.EncodeToString([]byte(credentials.Username+":"+credentials.Password))
			upstream := controlledProxy(t, target, useTLS, auth, &tunnels)
			pool := x509.NewCertPool()
			pool.AddCert(target.Certificate())
			if useTLS {
				pool.AddCert(upstream.Certificate())
			}
			report := Check(context.Background(), checkFixtureConfig(t, upstream), credentials, CheckOptions{TargetURL: target.URL, RootCAs: pool}, nil)
			if report.Error != nil || report.ExitIP != "203.0.113.45" || report.FinishedAt == "" || tunnels.Load() != 1 || targets.Load() != 1 {
				t.Fatal("controlled proxy tunnel/exit was not actually observed")
			}
			encoded, _ := json.Marshal(report)
			if strings.Contains(string(encoded), credentials.Password) || strings.Contains(string(encoded), credentials.Username) {
				t.Fatal("report contains raw authentication")
			}
			foundProxyTLS, foundConnection := false, false
			for _, step := range report.Steps {
				if step.Stage == "connection" && step.Status == "passed" {
					foundConnection = true
				}
				if step.Stage == "proxy-tls" && step.Status == "passed" {
					foundProxyTLS = true
				}
			}
			if !foundConnection {
				t.Fatal("private dial context lost the real TCP connection stage")
			}
			if useTLS && !foundProxyTLS {
				t.Fatal("TLS-to-proxy stage missing")
			}
		})
	}
}
func TestProxyAuthAndTLSFailuresNeverFallBackToDirect(t *testing.T) {
	var targets, tunnels atomic.Int32
	target := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		targets.Add(1)
		_, _ = io.WriteString(w, `{"ip":"203.0.113.45"}`)
	}))
	defer target.Close()
	upstream := controlledProxy(t, target, true, "Basic synthetic-required", &tunnels)
	config := checkFixtureConfig(t, upstream)
	untrusted := Check(context.Background(), config, nil, CheckOptions{TargetURL: target.URL}, nil)
	if untrusted.Error == nil || untrusted.Error.Code != "PROXY_TLS_FAILED" || targets.Load() != 0 || tunnels.Load() != 0 {
		t.Fatal("untrusted proxy TLS was bypassed")
	}
	pool := x509.NewCertPool()
	pool.AddCert(target.Certificate())
	pool.AddCert(upstream.Certificate())
	wrongAuth := Check(context.Background(), config, &Credentials{Username: "synthetic-wrong", Password: "synthetic-wrong"}, CheckOptions{TargetURL: target.URL, RootCAs: pool}, nil)
	if wrongAuth.Error == nil || wrongAuth.Error.Code != "PROXY_AUTH_FAILED" || targets.Load() != 0 || tunnels.Load() != 0 {
		t.Fatal("407 was accepted or retried directly")
	}
	encoded, _ := json.Marshal(wrongAuth)
	if strings.Contains(string(encoded), "SYNTHETIC_PEER_BODY") {
		t.Fatal("arbitrary peer error body entered report")
	}
	config.Type = "socks5"
	unsupported := Check(context.Background(), config, nil, CheckOptions{TargetURL: target.URL, RootCAs: pool}, nil)
	if unsupported.Error == nil || unsupported.Error.Code != "PROXY_UNSUPPORTED" || targets.Load() != 0 {
		t.Fatal("unimplemented SOCKS5 silently used another route")
	}
}
func TestProxyCancellationClosesAnInFlightCONNECTWithoutWaitingForPeer(t *testing.T) {
	entered, release := make(chan struct{}), make(chan struct{})
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		close(entered)
		<-release
		w.WriteHeader(http.StatusProxyAuthRequired)
	}))
	defer upstream.Close()
	defer close(release)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	result := make(chan Report, 1)
	go func() {
		result <- Check(ctx, checkFixtureConfig(t, upstream), nil, CheckOptions{TargetURL: "https://synthetic-target.invalid/"}, nil)
	}()
	select {
	case <-entered:
	case <-time.After(2 * time.Second):
		t.Fatal("controlled CONNECT was never received")
	}
	cancel()
	select {
	case report := <-result:
		if report.Error == nil || report.Error.Code != "OPERATION_CANCELLED" {
			t.Fatal("cancel was not distinguished from a finished network result")
		}
	case <-time.After(time.Second):
		t.Fatal("cancelled CONNECT retained an open request")
	}
}

func TestProxyResponseBodyCancellationAndDeadlineKeepTheirActualCause(t *testing.T) {
	for _, timedOut := range []bool{false, true} {
		t.Run(strconv.FormatBool(timedOut), func(t *testing.T) {
			entered, release := make(chan struct{}), make(chan struct{})
			target := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_, _ = io.WriteString(w, `{"ip":"`)
				w.(http.Flusher).Flush()
				close(entered)
				<-release
			}))
			defer target.Close()
			defer close(release)
			var tunnels atomic.Int32
			upstream := controlledProxy(t, target, false, "", &tunnels)
			pool := x509.NewCertPool()
			pool.AddCert(target.Certificate())
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			result := make(chan Report, 1)
			go func() {
				result <- Check(ctx, checkFixtureConfig(t, upstream), nil, CheckOptions{TargetURL: target.URL, RootCAs: pool}, nil)
			}()
			select {
			case <-entered:
			case <-time.After(4 * time.Second):
				t.Fatal("fixture never reached response-body read")
			}
			if !timedOut {
				cancel()
			}
			select {
			case report := <-result:
				code := "OPERATION_CANCELLED"
				if timedOut {
					code = "PROXY_CHECK_TIMEOUT"
				}
				if report.Error == nil || report.Error.Code != code {
					t.Fatal("body cancellation/deadline was misclassified as an invalid exit response")
				}
			case <-time.After(6 * time.Second):
				t.Fatal("body read did not end with the original check context")
			}
		})
	}
}
