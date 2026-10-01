package workspace

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/axgiroud312-byte/prism-local-browser/internal/kernel"
	"github.com/axgiroud312-byte/prism-local-browser/internal/proxy"
)

type faultableRuntimeChannel struct {
	*syntheticRuntimeChannel
	mu      sync.Mutex
	failed  chan struct{}
	failure *proxy.CheckError
	once    sync.Once
}

func TestStartupNetworkFaultSurvivesFailedCleanupForceStopAndReopen(t *testing.T) {
	channel := newFaultableRuntimeChannel()
	process := newSyntheticRuntimeProcess()
	process.closeError, process.stopError = errors.New("synthetic cleanup unconfirmed"), errors.New("synthetic normal stop failure")
	s, root, kernelID := fingerprintFixture(t, Options{OpenProxyChannel: func(_ proxy.Configuration, _ *proxy.Credentials, options proxy.BridgeOptions) (RuntimeProxyChannel, error) {
		channel.channelID = options.ChannelID
		return channel, nil
	}, LaunchRuntime: func(context.Context, RuntimeLaunch) (RuntimeProcess, error) {
		channel.trip("PROXY_AUTH_FAILED")
		return process, errors.New("synthetic startup interrupted by channel failure")
	}, InspectRuntime: func(RuntimeSession) (kernel.ManagedRecovery, error) {
		return kernel.ManagedRecovery{ProcessState: "exited", DirectoryFree: true, ResourcesExited: true, SessionMatches: true}, nil
	}})
	record := importProxyFixture(t, s, "localhost:8080")
	environment := createRuntimeEnvironment(t, s, kernelID, "合成启动期间网络故障")
	bindRuntimeProxyFixture(t, s, environment, record)
	start := acceptRuntimeTest(t, s, "Runtime.Start", runtimeRequest{EnvironmentID: environment.ID, RequestID: id(), NetworkPolicy: "proxy"})
	final := waitKernel(t, s, start.ID)
	session := view(t, s).RuntimeSessions[environment.ID]
	if final.Error == nil || final.Error.Code != "PROXY_AUTH_FAILED" || session.NetworkFault == nil || session.NetworkFault.Containment != "exit-unconfirmed" || session.Error.Code != "PROXY_AUTH_FAILED" {
		t.Fatal("startup cleanup discarded the actual channel root cause")
	}
	stop := acceptRuntimeTest(t, s, "Runtime.Stop", runtimeRequest{EnvironmentID: environment.ID, RequestID: id()})
	if waitKernel(t, s, stop.ID).State != "failed" {
		t.Fatal("fixture did not preserve unconfirmed stop")
	}
	process.mu.Lock()
	process.closeError = nil
	process.mu.Unlock()
	force := acceptRuntimeTest(t, s, "Runtime.ForceStop", runtimeRequest{EnvironmentID: environment.ID, SessionID: session.SessionID, RequestID: id()})
	if waitKernel(t, s, force.ID).State != "completed" {
		t.Fatal("owned force cleanup failed")
	}
	session = view(t, s).RuntimeSessions[environment.ID]
	if session.State != "error" || session.NetworkFault.Containment != "stopped" || session.Error.Code != "PROXY_AUTH_FAILED" {
		t.Fatal("force cleanup cleared startup channel root cause")
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := Open(root, Options{InspectRuntime: s.options.InspectRuntime})
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	if saved := view(t, reopened).RuntimeSessions[environment.ID]; saved.NetworkFault == nil || saved.NetworkFault.Error.Code != "PROXY_AUTH_FAILED" || saved.NetworkFault.Containment != "stopped" || saved.State != "error" {
		t.Fatal("reopen cleared a confirmed startup network failure")
	}
}

// Trip after launchRuntime's last channel.Fault call but before the service
// publishes readiness. This is a deterministic host seam, not an OS event.
type lateNetworkFaultProcess struct {
	*syntheticRuntimeProcess
	channel *faultableRuntimeChannel
	once    sync.Once
}

func (p *lateNetworkFaultProcess) Snapshot() kernel.RuntimeSnapshot {
	p.once.Do(func() { p.channel.trip("PROXY_BRIDGE_UNAVAILABLE") })
	return p.syntheticRuntimeProcess.Snapshot()
}
func TestLateStartupChannelFailureIsNotPublishedAsReadyOrOrdinaryProcessCrash(t *testing.T) {
	channel := newFaultableRuntimeChannel()
	process := &lateNetworkFaultProcess{syntheticRuntimeProcess: newSyntheticRuntimeProcess(), channel: channel}
	s, _, kernelID := fingerprintFixture(t, Options{OpenProxyChannel: func(_ proxy.Configuration, _ *proxy.Credentials, options proxy.BridgeOptions) (RuntimeProxyChannel, error) {
		channel.channelID = options.ChannelID
		return channel, nil
	}, LaunchRuntime: func(context.Context, RuntimeLaunch) (RuntimeProcess, error) { return process, nil }})
	record := importProxyFixture(t, s, "localhost:8080")
	environment := createRuntimeEnvironment(t, s, kernelID, "合成迟到启动网络故障")
	bindRuntimeProxyFixture(t, s, environment, record)
	start := acceptRuntimeTest(t, s, "Runtime.Start", runtimeRequest{EnvironmentID: environment.ID, RequestID: id(), NetworkPolicy: "proxy"})
	final := waitKernel(t, s, start.ID)
	session := view(t, s).RuntimeSessions[environment.ID]
	if final.State != "failed" || final.Error.Code != "PROXY_BRIDGE_UNAVAILABLE" || session.NetworkFault == nil || session.NetworkFault.Error.Code != "PROXY_BRIDGE_UNAVAILABLE" || session.PID != 0 || session.NetworkFault.Containment != "stopped" {
		t.Fatal("late startup channel failure was published as ready or lost during cleanup")
	}
}

func TestLatchedChannelFaultWithoutCreatedProcessPersistsAcrossReopen(t *testing.T) {
	channel := newFaultableRuntimeChannel()
	s, root, kernelID := fingerprintFixture(t, Options{OpenProxyChannel: func(_ proxy.Configuration, _ *proxy.Credentials, options proxy.BridgeOptions) (RuntimeProxyChannel, error) {
		channel.channelID = options.ChannelID
		return channel, nil
	}, LaunchRuntime: func(context.Context, RuntimeLaunch) (RuntimeProcess, error) {
		channel.trip("PROXY_BRIDGE_UNAVAILABLE")
		return nil, errors.New("synthetic bound channel failed before CreateProcess")
	}})
	record := importProxyFixture(t, s, "localhost:8080")
	environment := createRuntimeEnvironment(t, s, kernelID, "合成未创建进程的桥故障")
	bindRuntimeProxyFixture(t, s, environment, record)
	start := acceptRuntimeTest(t, s, "Runtime.Start", runtimeRequest{EnvironmentID: environment.ID, RequestID: id(), NetworkPolicy: "proxy"})
	final := waitKernel(t, s, start.ID)
	session := view(t, s).RuntimeSessions[environment.ID]
	if final.Error.Code != "PROXY_BRIDGE_UNAVAILABLE" || session.NetworkFault == nil || session.NetworkFault.Containment != "stopped" || session.RootPID != 0 || channel.closes.Load() == 0 {
		t.Fatal("nil-process cleanup lost a latched channel fault")
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := Open(root, Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	if saved := view(t, reopened).RuntimeSessions[environment.ID]; saved.NetworkFault == nil || saved.State != "error" || saved.Error.Code != "PROXY_BRIDGE_UNAVAILABLE" {
		t.Fatal("reopen erased channel failure because no root process was created")
	}
}

func TestUnfinishedStopKeepsReservationEvenWhenNetworkErrorHasNoLiveProcess(t *testing.T) {
	s, _, kernelID := fingerprintFixture(t, Options{})
	record := importProxyFixture(t, s, "localhost:8080")
	environment := createRuntimeEnvironment(t, s, kernelID, "合成门禁与并发停止")
	bindRuntimeProxyFixture(t, s, environment, record)
	start := acceptRuntimeTest(t, s, "Runtime.Start", runtimeRequest{EnvironmentID: environment.ID, RequestID: id(), NetworkPolicy: "proxy"})
	waitKernel(t, s, start.ID)
	// Deterministic scheduling seam: reproduce an accepted Stop while startup
	// cleanup is between its two locks, without racing a background test loop.
	s.mu.Lock()
	slot := s.runtimeSlots[environment.ID]
	stop := Operation{ID: id(), Kind: "runtime-stop", State: "accepted", Stage: "closing", Total: 1, CompletedIDs: []string{}, EnvironmentID: environment.ID, SessionID: slot.session.SessionID}
	slot.stop = &stop
	s.profileUses[environment.ID] = true
	if err := s.persistRuntime(slot, slot.stop, "合成并发停止受理"); err != nil {
		s.mu.Unlock()
		t.Fatal(err)
	}
	s.mu.Unlock()
	s.finishRuntimeStart(slot, nil, nil, kernel.RequireProxyNetworkBoundary())
	wantError(t, call(s, "Runtime.Start", runtimeRequest{EnvironmentID: environment.ID, RequestID: id(), NetworkPolicy: "proxy"}), "PROFILE_BUSY")
	s.mu.Lock()
	retained := s.profileUses[environment.ID]
	s.mu.Unlock()
	if !retained {
		t.Fatal("error display released the unfinished stop's reservation")
	}
	s.workers.Add(1)
	s.closeRuntime(slot)
	if final := waitKernel(t, s, stop.ID); final.State != "completed" {
		t.Fatal("old Stop was abandoned rather than publishing its terminal result")
	}
	s.mu.Lock()
	retained = s.profileUses[environment.ID]
	s.mu.Unlock()
	if retained {
		t.Fatal("durable stop completion never released the original reservation")
	}
}

func newFaultableRuntimeChannel() *faultableRuntimeChannel {
	return &faultableRuntimeChannel{syntheticRuntimeChannel: &syntheticRuntimeChannel{}, failed: make(chan struct{})}
}
func (c *faultableRuntimeChannel) Failed() <-chan struct{} { return c.failed }
func (c *faultableRuntimeChannel) Fault() *proxy.CheckError {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.failure == nil {
		return nil
	}
	copy := *c.failure
	return &copy
}
func (c *faultableRuntimeChannel) trip(code string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.failure == nil {
		c.failure = &proxy.CheckError{Code: code, Message: "合成网络故障，不是实机流量证据。", Retryable: true}
		c.once.Do(func() { close(c.failed) })
	}
}

func TestProductionProxyStartGateCreatesNeitherBridgeNorBrowserAndPreservesIdentity(t *testing.T) {
	var factories, decodes atomic.Int32
	s, root, kernelID := fingerprintFixture(t, Options{OpenProxyChannel: func(proxy.Configuration, *proxy.Credentials, proxy.BridgeOptions) (RuntimeProxyChannel, error) {
		factories.Add(1)
		return nil, errors.New("must not reach bridge before protection")
	}})
	record := importProxyFixture(t, s, "socks5://synthetic-user:synthetic-pass@localhost:1080")
	environment := createRuntimeEnvironment(t, s, kernelID, "合成缺失隔离阻断")
	bindRuntimeProxyFixture(t, s, environment, record)
	before := view(t, s)
	s.options.UnprotectProxySecret = func(string, []byte) ([]byte, error) {
		decodes.Add(1)
		return nil, errors.New("must not decode before protection")
	}
	request := runtimeRequest{EnvironmentID: environment.ID, RequestID: id(), NetworkPolicy: "proxy"}
	operation := acceptRuntimeTest(t, s, "Runtime.Start", request)
	final := waitKernel(t, s, operation.ID)
	after := view(t, s)
	session := after.RuntimeSessions[environment.ID]
	if final.Error == nil || final.Error.Code != "NETWORK_PROTECTION_UNAVAILABLE" || final.Error.Retryable || session.PID != 0 || session.NetworkFault == nil || session.NetworkFault.State != "network_error" || session.ProxyReport != nil || factories.Load() != 0 || decodes.Load() != 0 {
		t.Fatal("missing system protection performed network/native work or reported ready")
	}
	if replay := acceptRuntimeTest(t, s, "Runtime.Start", request); replay.ID != operation.ID || factories.Load() != 0 {
		t.Fatal("retry bypassed gate or changed accepted request identity")
	}
	if after.Fingerprints[environment.ID].Profile.ConfigHash != before.Fingerprints[environment.ID].Profile.ConfigHash || after.DataReferences[environment.ID] != before.DataReferences[environment.ID] {
		t.Fatal("protection failure modified device identity/data reference")
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := Open(root, Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	if saved := view(t, reopened).RuntimeSessions[environment.ID]; saved.NetworkFault == nil || saved.NetworkFault.Containment != "stopped" || saved.Error.Code != "NETWORK_PROTECTION_UNAVAILABLE" {
		t.Fatal("reopen cleared gate failure or revived a channel")
	}
}

func TestNetworkFaultStopsOnlyItsOwnedSessionAndPreservesOriginalDeviceData(t *testing.T) {
	channels := []*faultableRuntimeChannel{newFaultableRuntimeChannel(), newFaultableRuntimeChannel()}
	processes := []*syntheticRuntimeProcess{newSyntheticRuntimeProcess(), newSyntheticRuntimeProcess()}
	var factories, launches atomic.Int32
	s, _, kernelID := fingerprintFixture(t, Options{OpenProxyChannel: func(_ proxy.Configuration, _ *proxy.Credentials, options proxy.BridgeOptions) (RuntimeProxyChannel, error) {
		c := channels[int(factories.Add(1))-1]
		c.channelID = options.ChannelID
		return c, nil
	}, LaunchRuntime: func(context.Context, RuntimeLaunch) (RuntimeProcess, error) {
		return processes[int(launches.Add(1))-1], nil
	}})
	record := importProxyFixture(t, s, "localhost:8080")
	a := createRuntimeEnvironment(t, s, kernelID, "合成故障A")
	b := createRuntimeEnvironment(t, s, kernelID, "合成保持B")
	bindRuntimeProxyFixture(t, s, a, record)
	bindRuntimeProxyFixture(t, s, b, record)
	for _, environment := range []Environment{a, b} {
		operation := acceptRuntimeTest(t, s, "Runtime.Start", runtimeRequest{EnvironmentID: environment.ID, RequestID: id(), NetworkPolicy: "proxy"})
		if waitKernel(t, s, operation.ID).State != "completed" {
			t.Fatal("synthetic session did not reach ready")
		}
	}
	before := view(t, s)
	channels[0].trip("PROXY_AUTH_FAILED")
	faulted := waitRuntimeObservation(t, s, a.ID, func(session RuntimeSession) bool {
		return session.NetworkFault != nil && session.NetworkFault.Containment == "stopped" && session.PID == 0
	})
	after := view(t, s)
	if faulted.State != "error" || faulted.Error.Code != "PROXY_AUTH_FAILED" || processes[0].closeCalls.Load() == 0 || processes[1].closeCalls.Load() != 0 || after.RuntimeSessions[b.ID].State != "running" || after.RuntimeSessions[b.ID].NetworkFault != nil {
		t.Fatal("network fault became process crash or stopped unrelated environment")
	}
	if after.Fingerprints[a.ID].Profile.ConfigHash != before.Fingerprints[a.ID].Profile.ConfigHash || after.DataReferences[a.ID] != before.DataReferences[a.ID] {
		t.Fatal("safety shutdown regenerated identity or changed data reference")
	}
}

func TestNetworkFaultCleanupFailureRetainsBusyAndCanRecoverThroughNormalStop(t *testing.T) {
	channel := newFaultableRuntimeChannel()
	process := newSyntheticRuntimeProcess()
	process.closeError = errors.New("synthetic owned-tree exit unconfirmed")
	s, _, kernelID := fingerprintFixture(t, Options{OpenProxyChannel: func(_ proxy.Configuration, _ *proxy.Credentials, options proxy.BridgeOptions) (RuntimeProxyChannel, error) {
		channel.channelID = options.ChannelID
		return channel, nil
	}, LaunchRuntime: func(context.Context, RuntimeLaunch) (RuntimeProcess, error) { return process, nil }})
	record := importProxyFixture(t, s, "localhost:8080")
	environment := createRuntimeEnvironment(t, s, kernelID, "合成网络清理未确认")
	bindRuntimeProxyFixture(t, s, environment, record)
	operation := acceptRuntimeTest(t, s, "Runtime.Start", runtimeRequest{EnvironmentID: environment.ID, RequestID: id(), NetworkPolicy: "proxy"})
	waitKernel(t, s, operation.ID)
	channel.trip("PROXY_BRIDGE_UNAVAILABLE")
	session := waitRuntimeObservation(t, s, environment.ID, func(session RuntimeSession) bool {
		return session.NetworkFault != nil && session.NetworkFault.Containment == "exit-unconfirmed"
	})
	if session.PID != process.PID() {
		t.Fatal("failed safety cleanup released a live session")
	}
	wantError(t, call(s, "Runtime.Start", runtimeRequest{EnvironmentID: environment.ID, RequestID: id(), NetworkPolicy: "proxy"}), "PROFILE_BUSY")
	process.mu.Lock()
	process.closeError = nil
	process.mu.Unlock()
	stop := acceptRuntimeTest(t, s, "Runtime.Stop", runtimeRequest{EnvironmentID: environment.ID, RequestID: id()})
	if waitKernel(t, s, stop.ID).State != "completed" {
		t.Fatal("normal stop did not confirm owned exit")
	}
	session = view(t, s).RuntimeSessions[environment.ID]
	if session.NetworkFault.Containment != "stopped" || session.Error.Code != "PROXY_BRIDGE_UNAVAILABLE" || session.PID != 0 {
		t.Fatal("confirmed stop forgot original network root cause")
	}
}

func TestNetworkSafetyDoesNotWaitForStorageAndPersistenceDoesNotReplayShutdown(t *testing.T) {
	var fail atomic.Bool
	channel := newFaultableRuntimeChannel()
	process := newSyntheticRuntimeProcess()
	s, _, kernelID := fingerprintFixture(t, Options{BeforeCommit: func() error {
		if fail.Load() {
			return errors.New("synthetic storage fault")
		}
		return nil
	}, OpenProxyChannel: func(_ proxy.Configuration, _ *proxy.Credentials, options proxy.BridgeOptions) (RuntimeProxyChannel, error) {
		channel.channelID = options.ChannelID
		return channel, nil
	}, LaunchRuntime: func(context.Context, RuntimeLaunch) (RuntimeProcess, error) { return process, nil }})
	defer fail.Store(false)
	record := importProxyFixture(t, s, "localhost:8080")
	environment := createRuntimeEnvironment(t, s, kernelID, "合成网络故障写失败")
	bindRuntimeProxyFixture(t, s, environment, record)
	operation := acceptRuntimeTest(t, s, "Runtime.Start", runtimeRequest{EnvironmentID: environment.ID, RequestID: id(), NetworkPolicy: "proxy"})
	waitKernel(t, s, operation.ID)
	fail.Store(true)
	channel.trip("PROXY_UNREACHABLE")
	session := waitRuntimeObservation(t, s, environment.ID, func(session RuntimeSession) bool {
		return session.NetworkFault != nil && session.NetworkFault.Containment == "stopped" && session.PersistencePending
	})
	if session.NetworkFault.Error.Code != "PROXY_UNREACHABLE" || process.closeCalls.Load() != 1 {
		t.Fatal("storage delayed safety cleanup or lost its cause")
	}
	fail.Store(false)
	session = waitRuntimeObservation(t, s, environment.ID, func(session RuntimeSession) bool { return !session.PersistencePending })
	if session.Error.Code != "PROXY_UNREACHABLE" || process.closeCalls.Load() != 1 || channel.checks.Load() != 1 {
		t.Fatal("persistence retry repeated network/shutdown side effects")
	}
}

func TestNetworkFaultCopyOnWriteKeepsPriorContainmentSnapshotAndRejectsRPCBypass(t *testing.T) {
	slot := &runtimeSlot{session: RuntimeSession{NetworkPolicy: "proxy", NetworkFault: &RuntimeNetworkFault{State: "network_error", Error: &Error{Code: "PROXY_AUTH_FAILED", Message: "合成失败。"}, ObservedAt: timestamp(), Containment: "stopping"}}}
	before := slot.session.NetworkFault
	setRuntimeNetworkContainment(slot, "stopped")
	if before.Containment != "stopping" || slot.session.NetworkFault.Containment != "stopped" || before == slot.session.NetworkFault {
		t.Fatal("late containment stage mutated an earlier durable/event snapshot")
	}
	s, _, kernelID := fingerprintFixture(t, Options{})
	environment := createRuntimeEnvironment(t, s, kernelID, "合成保护RPC覆盖")
	record := importProxyFixture(t, s, "localhost:8080")
	bindRuntimeProxyFixture(t, s, environment, record)
	wantError(t, call(s, "Runtime.Start", map[string]any{"environmentId": environment.ID, "requestId": id(), "networkPolicy": "proxy", "networkProtectionReady": true, "allowUnsafeProxy": true}), "VALIDATION_FAILED")
}
