package workspace

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/axgiroud312-byte/prism-local-browser/internal/kernel"
	"github.com/axgiroud312-byte/prism-local-browser/internal/proxy"
)

type syntheticRuntimeChannel struct {
	channelID   string
	checks      atomic.Int32
	closes      atomic.Int32
	result      *proxy.CheckError
	beforeCheck func()
}

func (c *syntheticRuntimeChannel) ID() string                            { return c.channelID }
func (c *syntheticRuntimeChannel) Endpoint() string                      { return "http://127.0.0.1:18443" }
func (c *syntheticRuntimeChannel) BindBrowser(func(net.Conn) bool) error { return nil }
func (c *syntheticRuntimeChannel) Close() error                          { c.closes.Add(1); return nil }
func (c *syntheticRuntimeChannel) Fault() *proxy.CheckError              { return nil }
func (c *syntheticRuntimeChannel) Preflight(_ context.Context, progress func(proxy.Step)) proxy.Report {
	c.checks.Add(1)
	if c.beforeCheck != nil {
		c.beforeCheck()
	}
	progress(proxy.Step{Stage: "upstream-connection", Status: "passed", Time: timestamp(), Message: "合成host seam，不是网络观测。"})
	return proxy.Report{Mode: "native", AdapterVersion: "synthetic-channel-host-only", ChannelID: c.channelID, StartedAt: timestamp(), FinishedAt: timestamp(), Steps: []proxy.Step{}, ExitIP: "203.0.113.91", Error: c.result}
}
func bindRuntimeProxyFixture(t *testing.T, s *Service, environment Environment, record ProxyView) {
	t.Helper()
	edit := preview(t, s, "edit", environment.ID)
	edit.Environment.ProxyID = record.ID
	value[map[string]any](t, call(s, "Environment.Update", Mutation{PreviewID: edit.PreviewID, Configuration: edit.Environment.Configuration, ExpectedRevision: edit.ExpectedRevision, RequestID: id()}))
}
func TestRuntimeProxyUsesCheckedChannelAndPreservesOriginalIdentity(t *testing.T) {
	channel := &syntheticRuntimeChannel{}
	process := newSyntheticRuntimeProcess()
	var launches atomic.Int32
	s, _, kernelID := fingerprintFixture(t, Options{OpenProxyChannel: func(_ proxy.Configuration, credentials *proxy.Credentials, options proxy.BridgeOptions) (RuntimeProxyChannel, error) {
		if credentials == nil || credentials.Password != "SYNTHETIC_RUNTIME_SECRET" {
			t.Error("protected stored authentication did not reach private factory")
		}
		channel.channelID = options.ChannelID
		return channel, nil
	}, LaunchRuntime: func(_ context.Context, input RuntimeLaunch) (RuntimeProcess, error) {
		launches.Add(1)
		if input.Network != channel || channel.checks.Load() != 1 {
			t.Error("launcher changed bridge instance or ran before preflight")
		}
		return process, nil
	}})
	record := importProxyFixture(t, s, "http://synthetic-user:SYNTHETIC_RUNTIME_SECRET@localhost:8080")
	environment := createRuntimeEnvironment(t, s, kernelID, "合成同通道环境")
	bindRuntimeProxyFixture(t, s, environment, record)
	before := view(t, s).Fingerprints[environment.ID].Profile
	request := runtimeRequest{EnvironmentID: environment.ID, RequestID: id(), NetworkPolicy: "proxy"}
	operation := acceptRuntimeTest(t, s, "Runtime.Start", request)
	final := waitKernel(t, s, operation.ID)
	if final.State != "completed" || final.ProxyReport.ChannelID != channel.ID() || final.ProxyReport.Revision != record.Revision || launches.Load() != 1 {
		t.Fatal("start did not publish same-channel readiness")
	}
	session := view(t, s).RuntimeSessions[environment.ID]
	if session.NetworkPolicy != "proxy" || session.ProxyID != record.ID || session.ProxyReport.ChannelID != channel.ID() {
		t.Fatal("session lost exact proxy channel identity")
	}
	if replay := acceptRuntimeTest(t, s, "Runtime.Start", request); replay.ID != operation.ID || launches.Load() != 1 || channel.checks.Load() != 1 {
		t.Fatal("request retry repeated preflight or browser launch")
	}
	encoded, _ := json.Marshal(view(t, s))
	if strings.Contains(string(encoded), channel.Endpoint()) || strings.Contains(string(encoded), "SYNTHETIC_RUNTIME_SECRET") {
		t.Fatal("ordinary workspace contains private endpoint or credentials")
	}
	stopped := acceptRuntimeTest(t, s, "Runtime.Stop", runtimeRequest{EnvironmentID: environment.ID, RequestID: id()})
	if waitKernel(t, s, stopped.ID).State != "completed" || channel.closes.Load() == 0 {
		t.Fatal("browser stop retained channel resources")
	}
	if view(t, s).Fingerprints[environment.ID].Profile.ConfigHash != before.ConfigHash || view(t, s).DataReferences[environment.ID] != "environments/"+environment.ID+"/user-data" {
		t.Fatal("proxy start/stop changed device or data reference")
	}
}
func TestRuntimeProxyFailureNeverInvokesLauncherAndReleasesChannel(t *testing.T) {
	for _, code := range []string{"PROXY_AUTH_FAILED", "PROXY_TARGET_FAILED", "PROXY_TLS_FAILED", "PROXY_BRIDGE_UNAVAILABLE"} {
		channel := &syntheticRuntimeChannel{result: &proxy.CheckError{Code: code, Message: "合成前检失败。", Retryable: true}}
		var launches atomic.Int32
		s, _, kernelID := fingerprintFixture(t, Options{OpenProxyChannel: func(_ proxy.Configuration, _ *proxy.Credentials, options proxy.BridgeOptions) (RuntimeProxyChannel, error) {
			channel.channelID = options.ChannelID
			return channel, nil
		}, LaunchRuntime: func(context.Context, RuntimeLaunch) (RuntimeProcess, error) {
			launches.Add(1)
			return newSyntheticRuntimeProcess(), nil
		}})
		record := importProxyFixture(t, s, "localhost:8080")
		environment := createRuntimeEnvironment(t, s, kernelID, "合成前检阻断"+code)
		bindRuntimeProxyFixture(t, s, environment, record)
		operation := acceptRuntimeTest(t, s, "Runtime.Start", runtimeRequest{EnvironmentID: environment.ID, RequestID: id(), NetworkPolicy: "proxy"})
		final := waitKernel(t, s, operation.ID)
		if final.State != "failed" || final.Error.Code != code || launches.Load() != 0 || channel.closes.Load() == 0 || view(t, s).RuntimeSessions[environment.ID].PID != 0 {
			t.Fatal("failed channel launched a direct/unready process or retained bridge")
		}
	}
}
func TestRuntimeProxyResultStorageFailureDoesNotRepeatPreflightOrLaunch(t *testing.T) {
	var fail atomic.Bool
	var launches atomic.Int32
	channel := &syntheticRuntimeChannel{beforeCheck: func() { fail.Store(true) }}
	s, _, kernelID := fingerprintFixture(t, Options{BeforeCommit: func() error {
		if fail.Load() {
			return errors.New("synthetic storage fault")
		}
		return nil
	}, OpenProxyChannel: func(_ proxy.Configuration, _ *proxy.Credentials, options proxy.BridgeOptions) (RuntimeProxyChannel, error) {
		channel.channelID = options.ChannelID
		return channel, nil
	}, LaunchRuntime: func(context.Context, RuntimeLaunch) (RuntimeProcess, error) {
		launches.Add(1)
		return newSyntheticRuntimeProcess(), nil
	}})
	defer fail.Store(false)
	record := importProxyFixture(t, s, "localhost:8080")
	environment := createRuntimeEnvironment(t, s, kernelID, "合成前检写入失败")
	bindRuntimeProxyFixture(t, s, environment, record)
	operation := acceptRuntimeTest(t, s, "Runtime.Start", runtimeRequest{EnvironmentID: environment.ID, RequestID: id(), NetworkPolicy: "proxy"})
	pending := waitKernel(t, s, operation.ID)
	if !pending.PersistencePending || launches.Load() != 0 {
		t.Fatal("unsaved preflight was accepted as browser readiness")
	}
	s.workers.Wait()
	fail.Store(false)
	final := waitKernel(t, s, operation.ID)
	if final.PersistencePending || launches.Load() != 0 || channel.checks.Load() != 1 || channel.closes.Load() == 0 {
		t.Fatal("persistence retry repeated external network/launch side effects")
	}
}
func TestRuntimeProxyFactoryAndCredentialsFailureBlockWithoutFallback(t *testing.T) {
	var factories, launches atomic.Int32
	s, _, kernelID := fingerprintFixture(t, Options{OpenProxyChannel: func(proxy.Configuration, *proxy.Credentials, proxy.BridgeOptions) (RuntimeProxyChannel, error) {
		factories.Add(1)
		return nil, &proxy.CheckError{Code: "PROXY_BRIDGE_UNAVAILABLE", Message: "合成本机绑定失败。", Retryable: true}
	}, LaunchRuntime: func(context.Context, RuntimeLaunch) (RuntimeProcess, error) {
		launches.Add(1)
		return newSyntheticRuntimeProcess(), nil
	}})
	record := importProxyFixture(t, s, "http://synthetic-user:synthetic-pass@localhost:8080")
	environment := createRuntimeEnvironment(t, s, kernelID, "合成凭据失败")
	bindRuntimeProxyFixture(t, s, environment, record)
	operation := acceptRuntimeTest(t, s, "Runtime.Start", runtimeRequest{EnvironmentID: environment.ID, RequestID: id(), NetworkPolicy: "proxy"})
	if waitKernel(t, s, operation.ID).Error.Code != "PROXY_BRIDGE_UNAVAILABLE" {
		t.Fatal("factory failure was concealed")
	}
	s.options.UnprotectProxySecret = func(string, []byte) ([]byte, error) { return nil, errors.New("synthetic DPAPI denied") }
	operation = acceptRuntimeTest(t, s, "Runtime.Start", runtimeRequest{EnvironmentID: environment.ID, RequestID: id(), NetworkPolicy: "proxy"})
	if waitKernel(t, s, operation.ID).Error.Code != "CREDENTIALS_UNAVAILABLE" || factories.Load() != 1 || launches.Load() != 0 {
		t.Fatal("DPAPI failure tried no-auth or direct fallback")
	}
}
func TestRuntimeProxyCallerCannotOverridePolicyEndpointTLSOrAuthentication(t *testing.T) {
	s, _, kernelID := fingerprintFixture(t, Options{})
	environment := createRuntimeEnvironment(t, s, kernelID, "合成策略边界")
	wantError(t, call(s, "Runtime.Start", runtimeRequest{EnvironmentID: environment.ID, RequestID: id(), NetworkPolicy: "proxy"}), "PROXY_POLICY_MISMATCH")
	record := importProxyFixture(t, s, "socks5://localhost:1080")
	bindRuntimeProxyFixture(t, s, environment, record)
	wantError(t, call(s, "Runtime.Start", runtimeRequest{EnvironmentID: environment.ID, RequestID: id(), NetworkPolicy: "direct"}), "PROXY_POLICY_MISMATCH")
	wantError(t, call(s, "Runtime.Start", map[string]any{"environmentId": environment.ID, "requestId": id(), "networkPolicy": "proxy", "endpoint": "http://127.0.0.1:8080", "skipTlsVerify": true, "password": "SYNTHETIC_CLIENT_SECRET"}), "VALIDATION_FAILED")
}
func TestSavedProxySessionRecoversWithoutRecreatingChannelOrNetwork(t *testing.T) {
	channel := &syntheticRuntimeChannel{}
	process := newSyntheticRuntimeProcess()
	var factories atomic.Int32
	options := Options{OpenProxyChannel: func(_ proxy.Configuration, _ *proxy.Credentials, opts proxy.BridgeOptions) (RuntimeProxyChannel, error) {
		factories.Add(1)
		channel.channelID = opts.ChannelID
		return channel, nil
	}, LaunchRuntime: func(context.Context, RuntimeLaunch) (RuntimeProcess, error) { return process, nil }, InspectRuntime: func(session RuntimeSession) (kernel.ManagedRecovery, error) {
		return kernel.ManagedRecovery{RootPID: session.RootPID, ProcessCreatedAt: session.ProcessCreatedAt, ProcessState: "exited", DirectoryFree: true, ResourcesExited: true, SessionMatches: true}, nil
	}}
	s, root, kernelID := fingerprintFixture(t, options)
	record := importProxyFixture(t, s, "localhost:8080")
	environment := createRuntimeEnvironment(t, s, kernelID, "合成代理重开核对")
	bindRuntimeProxyFixture(t, s, environment, record)
	operation := acceptRuntimeTest(t, s, "Runtime.Start", runtimeRequest{EnvironmentID: environment.ID, RequestID: id(), NetworkPolicy: "proxy"})
	if waitKernel(t, s, operation.ID).State != "completed" {
		t.Fatal("host fixture did not reach ready")
	}
	before := view(t, s).RuntimeSessions[environment.ID]
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := Open(root, options)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	if err := validateSavedRuntime(before, environment.ID); err != nil {
		t.Fatal(err)
	}
	after := view(t, reopened).RuntimeSessions[environment.ID]
	if factories.Load() != 1 || channel.checks.Load() != 1 || after.ProxyChannelID != before.ProxyChannelID || after.ProxyReport.ChannelID != before.ProxyChannelID || after.PID != 0 || after.NeedsReconcile {
		t.Fatal("reopen recreated a stale bridge or lost its historical report identity")
	}
}

func TestRuntimeDecryptDoesNotBlockOtherQueriesAndLateCancellationCannotBuildBridge(t *testing.T) {
	entered, release := make(chan struct{}), make(chan struct{})
	var releaseOnce sync.Once
	resume := func() { releaseOnce.Do(func() { close(release) }) }
	defer resume()
	var factories, launches atomic.Int32
	s, _, kernelID := fingerprintFixture(t, Options{OpenProxyChannel: func(proxy.Configuration, *proxy.Credentials, proxy.BridgeOptions) (RuntimeProxyChannel, error) {
		factories.Add(1)
		return nil, errors.New("must not enter after cancel")
	}, LaunchRuntime: func(context.Context, RuntimeLaunch) (RuntimeProcess, error) {
		launches.Add(1)
		return newSyntheticRuntimeProcess(), nil
	}})
	record := importProxyFixture(t, s, "http://synthetic-user:synthetic-pass@localhost:8080")
	environment := createRuntimeEnvironment(t, s, kernelID, "合成解密锁外取消")
	bindRuntimeProxyFixture(t, s, environment, record)
	s.options.UnprotectProxySecret = func(ref string, protected []byte) ([]byte, error) {
		close(entered)
		<-release
		return proxy.Unprotect(ref, protected)
	}
	operation := acceptRuntimeTest(t, s, "Runtime.Start", runtimeRequest{EnvironmentID: environment.ID, RequestID: id(), NetworkPolicy: "proxy"})
	select {
	case <-entered:
	case <-time.After(time.Second):
		t.Fatal("decrypt seam not reached")
	}
	queryDone := make(chan Result, 1)
	go func() { queryDone <- call(s, "Workspace.Read", struct{}{}) }()
	select {
	case result := <-queryDone:
		if !result.OK {
			t.Fatal("query failed during decrypt")
		}
	case <-time.After(time.Second):
		resume()
		t.Fatal("DPAPI blocked global workspace mutex")
	}
	stop := acceptRuntimeTest(t, s, "Runtime.Stop", runtimeRequest{EnvironmentID: environment.ID, RequestID: id()})
	resume()
	if waitKernel(t, s, operation.ID).State != "cancelled" || waitKernel(t, s, stop.ID).State != "completed" || factories.Load() != 0 || launches.Load() != 0 {
		t.Fatal("late decrypt result started a cancelled bridge/browser")
	}
}

type unknownCreationProcess struct{ *syntheticRuntimeProcess }

func (*unknownCreationProcess) CreatedAt() string { return "" }
func TestFailedCreationTimeObservationRetainsLiveTreeAndBusyUntilConfirmedExit(t *testing.T) {
	process := &unknownCreationProcess{newSyntheticRuntimeProcess()}
	process.closeError = errors.New("synthetic owned-tree cleanup unconfirmed")
	s, _, kernelID := fingerprintFixture(t, Options{LaunchRuntime: func(context.Context, RuntimeLaunch) (RuntimeProcess, error) {
		return process, &kernel.Problem{Code: "PROCESS_IDENTITY_UNAVAILABLE", Message: "合成创建时间观测失败。", Retryable: true}
	}})
	environment := createRuntimeEnvironment(t, s, kernelID, "合成身份失败仍保护")
	operation := acceptRuntimeTest(t, s, "Runtime.Start", runtimeRequest{EnvironmentID: environment.ID, RequestID: id(), NetworkPolicy: "direct"})
	if waitKernel(t, s, operation.ID).Error.Code != "PROCESS_IDENTITY_UNAVAILABLE" {
		t.Fatal("identity failure was lost")
	}
	session := view(t, s).RuntimeSessions[environment.ID]
	if session.PID != process.PID() || session.LaunchStage != "identity-unconfirmed" || session.ProcessCreatedAt != "" || validateSavedRuntime(session, environment.ID) != nil {
		t.Fatal("created resources were recorded as never created or made workspace unreopenable")
	}
	wantError(t, call(s, "Runtime.Start", runtimeRequest{EnvironmentID: environment.ID, RequestID: id(), NetworkPolicy: "direct"}), "PROFILE_BUSY")
	process.mu.Lock()
	process.closeError = nil
	process.mu.Unlock()
	stop := acceptRuntimeTest(t, s, "Runtime.Stop", runtimeRequest{EnvironmentID: environment.ID, RequestID: id()})
	if waitKernel(t, s, stop.ID).State != "completed" || view(t, s).RuntimeSessions[environment.ID].PID != 0 {
		t.Fatal("confirmed owned exit did not clear active PID/busy")
	}
}
