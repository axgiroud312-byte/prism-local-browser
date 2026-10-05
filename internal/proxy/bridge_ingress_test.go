package proxy

import (
	"context"
	"crypto/x509"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

type observedIngressListener struct {
	net.Listener
	accepts atomic.Int32
	closes  atomic.Int32
}

func (l *observedIngressListener) Accept() (net.Conn, error) {
	conn, err := l.Listener.Accept()
	if err == nil {
		l.accepts.Add(1)
	}
	return conn, err
}
func (l *observedIngressListener) Close() error {
	l.closes.Add(1)
	return l.Listener.Close()
}

func newObservedIngress(t *testing.T) *observedIngressListener {
	t.Helper()
	l, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = l.Close() })
	return &observedIngressListener{Listener: l}
}

func TestSuppliedIngressChecksAndBrowserUseOneListenerWithoutReplacingHostAuthorization(t *testing.T) {
	var targets, tunnels, dials, authorizations atomic.Int32
	target := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		targets.Add(1)
		if r.Header.Get("Proxy-Authorization") != "" {
			t.Error("probe authorization escaped the local ingress")
		}
		_, _ = io.WriteString(w, `{"ip":"203.0.113.81"}`)
	}))
	defer target.Close()
	upstream := controlledProxy(t, target, false, "", &tunnels)
	trust := x509.NewCertPool()
	trust.AddCert(target.Certificate())
	listener := newObservedIngress(t)
	ingress := &BridgeIngress{Listener: listener, ProbeDial: func(ctx context.Context) (net.Conn, error) {
		dials.Add(1)
		return (&net.Dialer{}).DialContext(ctx, "tcp4", listener.Addr().String())
	}}
	b, err := OpenBridge(checkFixtureConfig(t, upstream), nil, BridgeOptions{ChannelID: "synthetic-supplied-ingress", Ingress: ingress, AuthorizeProbe: func(net.Conn) bool { authorizations.Add(1); return true }, TargetURL: target.URL, RootCAs: trust})
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close()
	// Mutating the input object after ownership transfer must not change the
	// endpoint or drop the private dialer on a subsequent periodic check.
	ingress.Listener, ingress.ProbeDial = nil, nil
	for attempt := 0; attempt < 2; attempt++ {
		report := b.Preflight(context.Background(), nil)
		if report.Error != nil || report.ExitIP != "203.0.113.81" || b.Endpoint() != "http://"+listener.Addr().String() {
			t.Fatal("supplied ingress did not carry the actual checked request")
		}
	}
	if dials.Load() != 2 || authorizations.Load() != 2 || targets.Load() != 2 || listener.accepts.Load() != 2 {
		t.Fatal("identity ingress bypassed the probe caller check or forwarding path")
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
	_, _ = io.Copy(io.Discard, response.Body)
	response.Body.Close()
	if targets.Load() != 3 || listener.accepts.Load() != 3 || dials.Load() != 2 {
		t.Fatal("browser traffic did not use the same supplied listener")
	}
	if err := b.Close(); err != nil || listener.closes.Load() == 0 {
		t.Fatal("successful bridge did not release its supplied listener")
	}
}

func TestIdentityIngressProbeFailureCannotFallBackToOrdinarySocket(t *testing.T) {
	listener := newObservedIngress(t)
	var dials atomic.Int32
	b, err := OpenBridge(Configuration{Name: "合成前检入口", Type: "http", Host: "synthetic-upstream.invalid", Port: 8080}, nil, BridgeOptions{
		ChannelID: "synthetic-denied-ingress", AuthorizeProbe: func(net.Conn) bool { return true },
		TargetURL: "https://synthetic-origin.invalid/",
		Ingress: &BridgeIngress{Listener: listener, ProbeDial: func(context.Context) (net.Conn, error) {
			dials.Add(1)
			return nil, errors.New("SYNTHETIC_PRIVATE_TOKEN_DETAIL")
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close()
	report := b.Preflight(context.Background(), nil)
	if report.Error == nil || dials.Load() != 1 || listener.accepts.Load() != 0 {
		t.Fatal("failed identity dial silently used an ordinary host connection")
	}
	encoded, _ := json.Marshal(report)
	if strings.Contains(string(encoded), "SYNTHETIC_PRIVATE_TOKEN_DETAIL") || strings.Contains(string(encoded), listener.Addr().String()) {
		t.Fatal("identity dial error leaked private launch material")
	}
}

func TestClosingBridgeCancelsPendingIdentityDial(t *testing.T) {
	listener := newObservedIngress(t)
	entered := make(chan struct{})
	cancelled := make(chan struct{})
	finishedUnwinding := make(chan struct{})
	b, err := OpenBridge(Configuration{Name: "合成待取消入口", Type: "http", Host: "synthetic-upstream.invalid", Port: 8080}, nil, BridgeOptions{
		ChannelID: "synthetic-cancelled-ingress", AuthorizeProbe: func(net.Conn) bool { return true }, TargetURL: "https://synthetic-origin.invalid/",
		Ingress: &BridgeIngress{Listener: listener, ProbeDial: func(ctx context.Context) (net.Conn, error) {
			close(entered)
			<-ctx.Done()
			close(cancelled)
			<-finishedUnwinding
			return nil, ctx.Err()
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	unwound := false
	defer func() {
		if !unwound {
			close(finishedUnwinding)
		}
		_ = b.Close()
	}()
	done := make(chan Report, 1)
	go func() { done <- b.Preflight(context.Background(), nil) }()
	select {
	case <-entered:
	case <-time.After(2 * time.Second):
		t.Fatal("identity probe never began")
	}
	closed := make(chan error, 1)
	go func() { closed <- b.Close() }()
	select {
	case <-cancelled:
	case <-time.After(2 * time.Second):
		t.Fatal("bridge closure did not cancel the identity dial")
	}
	select {
	case <-closed:
		t.Fatal("bridge released ownership before its identity dial finished")
	case <-time.After(30 * time.Millisecond):
	}
	close(finishedUnwinding)
	unwound = true
	select {
	case err := <-closed:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("bridge did not finish after the identity dial released its resources")
	}
	select {
	case report := <-done:
		if report.Error == nil || report.Error.Code != "OPERATION_CANCELLED" || listener.accepts.Load() != 0 {
			t.Fatal("closed bridge published a successful or fallback probe")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("closing the ingress retained a pending identity dial")
	}
}

func TestIncompleteSuppliedIngressIsRejectedWithoutTakingListenerOwnership(t *testing.T) {
	listener := newObservedIngress(t)
	options := BridgeOptions{ChannelID: "synthetic-incomplete-ingress", AuthorizeProbe: func(net.Conn) bool { return true }, Ingress: &BridgeIngress{Listener: listener}}
	b, err := OpenBridge(Configuration{Name: "合成无降级入口", Type: "http", Host: "localhost", Port: 8080}, nil, options)
	if err == nil || b != nil || listener.closes.Load() != 0 || listener.accepts.Load() != 0 {
		t.Fatal("invalid supplied ingress was replaced or its caller-owned listener was closed")
	}
}

type differentAddressIngress struct {
	net.Listener
	address net.Addr
}

func (l differentAddressIngress) Addr() net.Addr { return l.address }

func TestSuppliedIngressRejectsPublicWildcardAndIPv6Endpoints(t *testing.T) {
	for _, address := range []*net.TCPAddr{
		{IP: net.IPv4zero, Port: 8080},
		{IP: net.IPv4(192, 0, 2, 1), Port: 8080},
		{IP: net.IPv6loopback, Port: 8080},
		{IP: net.IPv4(127, 0, 0, 1), Port: 0},
	} {
		listener := newObservedIngress(t)
		b, err := OpenBridge(Configuration{Name: "合成入口地址", Type: "http", Host: "localhost", Port: 8080}, nil, BridgeOptions{
			ChannelID: "synthetic-invalid-address", AuthorizeProbe: func(net.Conn) bool { return true },
			Ingress: &BridgeIngress{Listener: differentAddressIngress{Listener: listener, address: address}, ProbeDial: func(context.Context) (net.Conn, error) {
				t.Error("invalid ingress dialed")
				return nil, errors.New("unexpected")
			}},
		})
		if b != nil || err == nil || listener.closes.Load() != 0 {
			t.Fatal("non-private supplied endpoint was accepted or silently replaced")
		}
	}
}

func TestIdentityIngressStillRequiresTheExactHostProbeAuthorization(t *testing.T) {
	listener := newObservedIngress(t)
	var authorized atomic.Int32
	b, err := OpenBridge(Configuration{Name: "合成仍需准入", Type: "http", Host: "synthetic-upstream.invalid", Port: 8080}, nil, BridgeOptions{
		ChannelID: "synthetic-denied-host", AuthorizeProbe: func(net.Conn) bool { authorized.Add(1); return false }, TargetURL: "https://synthetic-origin.invalid/",
		Ingress: &BridgeIngress{Listener: listener, ProbeDial: func(ctx context.Context) (net.Conn, error) {
			return (&net.Dialer{}).DialContext(ctx, "tcp4", listener.Addr().String())
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close()
	report := b.Preflight(context.Background(), nil)
	if report.Error == nil || report.Error.Code != "PROXY_TARGET_FAILED" || authorized.Load() != 1 || listener.accepts.Load() != 1 {
		t.Fatal("socket identity was incorrectly treated as host probe authorization")
	}
}
