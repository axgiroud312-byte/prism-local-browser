package proxy

import (
	"context"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func waitBridgeFailure(t *testing.T, bridge *Bridge, code string) {
	t.Helper()
	select {
	case <-bridge.Failed():
	case <-time.After(2 * time.Second):
		t.Fatal("owned channel did not signal its latched fault")
	}
	if failure := bridge.Fault(); failure == nil || failure.Code != code {
		t.Fatal("latched channel fault lost its actual cause")
	}
	select {
	case <-bridge.Done():
	case <-time.After(2 * time.Second):
		t.Fatal("failed channel retained listeners/owned workers")
	}
}

func TestCancelledBrowserRequestDoesNotLatchUpstreamOrTLSFault(t *testing.T) {
	for _, code := range []string{"PROXY_UNREACHABLE", "PROXY_TLS_FAILED", "PROXY_SOCKS_NEGOTIATION_FAILED", "PROXY_AUTH_FAILED"} {
		config := Configuration{Name: "合成取消隔离", Type: "http", Host: "127.0.0.1", Port: 1080}
		b, err := OpenBridge(config, nil, BridgeOptions{ChannelID: "synthetic-cancelled-request", AuthorizeProbe: func(net.Conn) bool { return true }})
		if err != nil {
			t.Fatal(err)
		}
		if err := b.BindBrowser(func(net.Conn) bool { return true }); err != nil {
			t.Fatal(err)
		}
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		b.reject(httptest.NewRecorder(), ctx, nil, &CheckError{Code: code, Message: "合成被取消的连接。", Retryable: true})
		if b.Fault() != nil {
			t.Fatal("canceling one request latched a healthy session fault")
		}
		select {
		case <-b.Failed():
			t.Fatal("request cancellation asked the owner to kill the session")
		default:
		}
		_ = b.Close()
	}
}

type failedSyntheticUpload struct{}

func (failedSyntheticUpload) Read([]byte) (int, error) { return 0, io.ErrUnexpectedEOF }
func (failedSyntheticUpload) Close() error             { return nil }

func TestFailedOrShortBrowserUploadDoesNotCloseHealthyUpstreamBridge(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = io.Copy(io.Discard, r.Body); w.WriteHeader(200) }))
	defer upstream.Close()
	for _, body := range []io.ReadCloser{failedSyntheticUpload{}, io.NopCloser(strings.NewReader("x"))} {
		b, err := OpenBridge(checkFixtureConfig(t, upstream), nil, BridgeOptions{ChannelID: "synthetic-upload-failure", AuthorizeProbe: func(net.Conn) bool { return true }})
		if err != nil {
			t.Fatal(err)
		}
		if b.BindBrowser(func(net.Conn) bool { return true }) != nil {
			t.Fatal("bind failed")
		}
		request := httptest.NewRequest("POST", "http://synthetic-target.invalid/upload", nil)
		request.Body, request.ContentLength = body, 10
		result := httptest.NewRecorder()
		b.forwardHTTP(result, request, nil)
		if b.Fault() != nil || !strings.Contains(result.Body.String(), "客户端上传正文中断") {
			t.Fatal("client upload failure was blamed on the bound proxy")
		}
		select {
		case <-b.Failed():
			t.Fatal("one broken upload terminated the session")
		default:
		}
		_ = b.Close()
	}
}

func TestCancelledSOCKSHandshakeAndDialKeepRequestScopedError(t *testing.T) {
	config := Configuration{Name: "合成取消协商", Type: "socks5", Host: "127.0.0.1", Port: 1080}
	b, err := OpenBridge(config, nil, BridgeOptions{ChannelID: "synthetic-cancelled-socks", AuthorizeProbe: func(net.Conn) bool { return true }})
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close()
	ctx, cancel := context.WithCancel(context.Background())
	client, peer := net.Pipe()
	defer client.Close()
	defer peer.Close()
	stop := context.AfterFunc(ctx, func() { _ = client.Close() })
	defer stop()
	observed := make(chan struct{})
	go func() { packet := make([]byte, 3); _, _ = io.ReadFull(peer, packet); cancel(); close(observed) }()
	failure := b.socksConnect(ctx, client, "synthetic-target.invalid:443", nil)
	<-observed
	if failure == nil || failure.Code != "OPERATION_CANCELLED" {
		t.Fatal("canceled SOCKS read became an upstream negotiation fault")
	}
	if conn, failure := b.dial(ctx, nil); conn != nil || failure == nil || failure.Code != "OPERATION_CANCELLED" {
		t.Fatal("canceled dial was misclassified as a broken proxy")
	}
}

func TestBoundSOCKS5AuthenticationFailureLatchesAndClosesOnlyThatBridge(t *testing.T) {
	credentials := &Credentials{Username: "synthetic-user", Password: "synthetic-pass"}
	config, observations := controlledSOCKS5(t, socksFixtureOptions{credentials: credentials, authReject: true})
	opts := BridgeOptions{ChannelID: "synthetic-fault-A", AuthorizeProbe: func(net.Conn) bool { return true }}
	a, err := OpenBridge(config, credentials, opts)
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	opts.ChannelID = "synthetic-fault-B"
	b, err := OpenBridge(config, credentials, opts)
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close()
	if err := a.BindBrowser(func(net.Conn) bool { return true }); err != nil {
		t.Fatal(err)
	}
	response, _ := bridgeBrowserClient(t, a).Get("https://synthetic-target.invalid/")
	if response != nil {
		response.Body.Close()
	}
	waitBridgeFailure(t, a, "PROXY_AUTH_FAILED")
	_ = waitSOCKSRequest(t, observations)
	select {
	case <-b.Done():
		t.Fatal("A fault closed independent B")
	default:
	}
	if b.Fault() != nil {
		t.Fatal("A fault polluted B fault state")
	}
}

func TestUnexpectedListenerExitSignalsOwnerButIntentionalCloseDoesNot(t *testing.T) {
	config := Configuration{Name: "合成监听故障", Type: "http", Host: "127.0.0.1", Port: 1080}
	for _, unexpected := range []bool{true, false} {
		b, err := OpenBridge(config, nil, BridgeOptions{ChannelID: "synthetic-listener-owner", AuthorizeProbe: func(net.Conn) bool { return true }})
		if err != nil {
			t.Fatal(err)
		}
		if unexpected {
			_ = b.listener.Close()
			waitBridgeFailure(t, b, "PROXY_BRIDGE_UNAVAILABLE")
		} else {
			_ = b.Close()
			select {
			case <-b.Failed():
				t.Fatal("normal shutdown generated a network fault")
			default:
			}
			if b.Fault() != nil {
				t.Fatal("normal close was reported as failed")
			}
		}
		_ = b.Close()
	}
}

func TestUnboundPreflightFailureDoesNotPretendAnAlreadyRunningSessionFault(t *testing.T) {
	config, observations := controlledSOCKS5(t, socksFixtureOptions{selectOther: true, method: 255})
	b, err := OpenBridge(config, nil, BridgeOptions{ChannelID: "synthetic-startup-check", AuthorizeProbe: func(net.Conn) bool { return true }})
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close()
	report := b.Preflight(context.Background(), nil)
	if report.Error == nil || report.Error.Code != "PROXY_SOCKS_NEGOTIATION_FAILED" {
		t.Fatal("startup check did not preserve protocol failure")
	}
	_ = waitSOCKSRequest(t, observations)
	select {
	case <-b.Failed():
		t.Fatal("unbound check invented a live browser fault")
	default:
	}
	if b.Fault() != nil {
		t.Fatal("startup check was promoted to runtime fault")
	}
}

func TestNetworkFaultLatchKeepsFirstCauseAndNeverReopensAClosedEndpoint(t *testing.T) {
	origin := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	defer origin.Close()
	b, err := OpenBridge(checkFixtureConfig(t, origin), nil, BridgeOptions{ChannelID: "synthetic-latched-fault", AuthorizeProbe: func(net.Conn) bool { return true }})
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close()
	if err := b.BindBrowser(func(net.Conn) bool { return true }); err != nil {
		t.Fatal(err)
	}
	endpoint := b.Endpoint()
	b.failNetwork(&CheckError{Code: "PROXY_AUTH_FAILED", Message: "合成认证过期。", Retryable: true})
	b.failNetwork(&CheckError{Code: "PROXY_UNREACHABLE", Message: "合成迟到连接失败。", Retryable: true})
	waitBridgeFailure(t, b, "PROXY_AUTH_FAILED")
	if b.Endpoint() != endpoint || b.BindBrowser(func(net.Conn) bool { return true }) == nil {
		t.Fatal("fault silently revived a listener or rebound a stale session")
	}
}
