package workspace

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/axgiroud312-byte/prism-local-browser/internal/proxy"
)

type retryableCloseChannel struct {
	*syntheticRuntimeChannel
	fail    atomic.Bool
	entered chan struct{}
	release <-chan struct{}
	once    sync.Once
}

type latchedCleanupChannel struct{ *retryableCloseChannel }

func (*latchedCleanupChannel) Fault() *proxy.CheckError {
	return &proxy.CheckError{Code: "PROXY_AUTH_FAILED", Message: "合成已闭锁网络故障。", Retryable: true}
}

func (c *retryableCloseChannel) Close() error {
	c.closes.Add(1)
	if c.entered != nil {
		c.once.Do(func() { close(c.entered) })
	}
	if c.release != nil {
		<-c.release
	}
	if c.fail.Load() {
		return errors.New("SYNTHETIC_PRIVATE_CLOSE_DETAIL")
	}
	return nil
}

func TestNetworkOwnerDoesNotPublishJobExitAsCompleteWhenChannelCloseFails(t *testing.T) {
	channel := &retryableCloseChannel{syntheticRuntimeChannel: &syntheticRuntimeChannel{}}
	channel.fail.Store(true)
	process := newSyntheticRuntimeProcess()
	owned := ownRuntimeNetwork(process, channel)
	defer func() { channel.fail.Store(false); _ = owned.Close() }()
	if err := owned.Close(); err == nil {
		t.Fatal("channel cleanup failure reported complete")
	}
	select {
	case <-process.Done():
	default:
		t.Fatal("channel failure prevented the exact process owner from closing")
	}
	select {
	case <-owned.Done():
		t.Fatal("Job exit hid the retained channel")
	default:
	}
	if snapshot := owned.Snapshot(); snapshot.RootAlive || snapshot.ResourcesExited || !snapshot.NetworkCleanupFailed {
		t.Fatal("resource observation released an unconfirmed channel")
	}
	channel.fail.Store(false)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := owned.Stop(ctx); err != nil {
		t.Fatal(err)
	}
	select {
	case <-owned.Done():
	default:
		t.Fatal("successful retry did not finish the existing resource owner")
	}
	if !owned.Snapshot().ResourcesExited {
		t.Fatal("confirmed cleanup remained incomplete")
	}
}

func TestConcurrentStopAndExitObservationShareOneChannelCloseAttempt(t *testing.T) {
	release := make(chan struct{})
	channel := &retryableCloseChannel{syntheticRuntimeChannel: &syntheticRuntimeChannel{}, entered: make(chan struct{}), release: release}
	owned := ownRuntimeNetwork(newSyntheticRuntimeProcess(), channel)
	var releaseOnce sync.Once
	defer func() { releaseOnce.Do(func() { close(release) }); _ = owned.Close() }()
	closed := make(chan error, 1)
	go func() { closed <- owned.Close() }()
	select {
	case <-channel.entered:
	case <-time.After(2 * time.Second):
		t.Fatal("channel cleanup did not begin")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	if err := owned.Stop(ctx); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatal("Stop reported completion before the shared channel close finished")
	}
	if owned.Snapshot().ResourcesExited {
		t.Fatal("pending channel close released the environment")
	}
	select {
	case <-owned.Done():
		t.Fatal("timed-out Stop closed the resource owner")
	default:
	}
	if channel.closes.Load() != 1 {
		t.Fatal("Stop started an overlapping close attempt")
	}
	releaseOnce.Do(func() { close(release) })
	select {
	case err := <-closed:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("shared cleanup did not finish")
	}
	if channel.closes.Load() != 1 {
		t.Fatal("concurrent cleanup replayed an already successful channel close")
	}
}

func TestStartupWithoutBrowserRetainsFailedChannelAndAllowsCleanupRetry(t *testing.T) {
	channel := &retryableCloseChannel{syntheticRuntimeChannel: &syntheticRuntimeChannel{}}
	channel.fail.Store(true)
	var launches atomic.Int32
	s, _, kernelID := fingerprintFixture(t, Options{
		OpenProxyChannel: func(_ proxy.Configuration, _ *proxy.Credentials, options proxy.BridgeOptions) (RuntimeProxyChannel, error) {
			channel.channelID = options.ChannelID
			return channel, &proxy.CheckError{Code: "PROXY_AUTH_FAILED", Message: "合成前检失败。", Retryable: true}
		},
		LaunchRuntime: func(context.Context, RuntimeLaunch) (RuntimeProcess, error) {
			launches.Add(1)
			return newSyntheticRuntimeProcess(), nil
		},
	})
	t.Cleanup(func() { channel.fail.Store(false) })
	proxyRecord := importProxyFixture(t, s, "localhost:8080")
	environment := createRuntimeEnvironment(t, s, kernelID, "合成无浏览器但通道待清理")
	bindRuntimeProxyFixture(t, s, environment, proxyRecord)
	start := acceptRuntimeTest(t, s, "Runtime.Start", runtimeRequest{EnvironmentID: environment.ID, RequestID: id(), NetworkPolicy: "proxy"})
	final := waitKernel(t, s, start.ID)
	session := view(t, s).RuntimeSessions[environment.ID]
	if final.State != "failed" || final.Error == nil || final.Error.Code != "PROXY_AUTH_FAILED" || launches.Load() != 0 {
		t.Fatal("cleanup symptom replaced the original startup failure or invoked the browser")
	}
	if session.PID != 0 || session.RootPID != 0 || session.LaunchStage != "no-process-created" || session.CanForce || !session.ResourcesPending {
		t.Fatal("channel-only owner invented a process identity or force-stop permission")
	}
	if err := validateSavedRuntime(session, environment.ID); err != nil {
		t.Fatal(err)
	}
	wantError(t, call(s, "Runtime.Start", runtimeRequest{EnvironmentID: environment.ID, RequestID: id(), NetworkPolicy: "proxy"}), "PROFILE_BUSY")
	stop := acceptRuntimeTest(t, s, "Runtime.Stop", runtimeRequest{EnvironmentID: environment.ID, RequestID: id()})
	if waitKernel(t, s, stop.ID).State != "failed" || view(t, s).RuntimeSessions[environment.ID].CanForce {
		t.Fatal("failed channel-only stop released the environment or allowed killing a nonexistent Job")
	}
	wantError(t, call(s, "Runtime.ForceStop", runtimeRequest{EnvironmentID: environment.ID, SessionID: session.SessionID, RequestID: id()}), "FORCE_STOP_NOT_ALLOWED")
	channel.fail.Store(false)
	retry := acceptRuntimeTest(t, s, "Runtime.Stop", runtimeRequest{EnvironmentID: environment.ID, RequestID: id()})
	if waitKernel(t, s, retry.ID).State != "completed" {
		t.Fatal("channel-only cleanup could not be retried")
	}
	session = view(t, s).RuntimeSessions[environment.ID]
	if session.Error == nil || session.Error.Code != "PROXY_AUTH_FAILED" || session.RootPID != 0 || session.ResourcesPending {
		t.Fatal("successful cleanup lost the original failure or created a fake process")
	}
	s.mu.Lock()
	retained := s.profileUses[environment.ID]
	s.mu.Unlock()
	if retained {
		t.Fatal("confirmed channel-only cleanup never released the original reservation")
	}
}

func TestApplicationCloseRetriesChannelOwnerCreatedAfterInitialShutdownScan(t *testing.T) {
	channel := &retryableCloseChannel{syntheticRuntimeChannel: &syntheticRuntimeChannel{}}
	channel.fail.Store(true)
	entered, release := make(chan struct{}), make(chan struct{})
	var released sync.Once
	var factories, launches atomic.Int32
	s, _, kernelID := fingerprintFixture(t, Options{
		OpenProxyChannel: func(_ proxy.Configuration, _ *proxy.Credentials, options proxy.BridgeOptions) (RuntimeProxyChannel, error) {
			factories.Add(1)
			channel.channelID = options.ChannelID
			close(entered)
			<-release
			return channel, &proxy.CheckError{Code: "PROXY_AUTH_FAILED", Message: "合成前检失败。", Retryable: true}
		},
		LaunchRuntime: func(context.Context, RuntimeLaunch) (RuntimeProcess, error) {
			launches.Add(1)
			return newSyntheticRuntimeProcess(), nil
		},
	})
	t.Cleanup(func() { channel.fail.Store(false); released.Do(func() { close(release) }) })
	proxyRecord := importProxyFixture(t, s, "localhost:8080")
	environment := createRuntimeEnvironment(t, s, kernelID, "合成应用退出期间迟到通道")
	bindRuntimeProxyFixture(t, s, environment, proxyRecord)
	acceptRuntimeTest(t, s, "Runtime.Start", runtimeRequest{EnvironmentID: environment.ID, RequestID: id(), NetworkPolicy: "proxy"})
	select {
	case <-entered:
	case <-time.After(2 * time.Second):
		t.Fatal("factory was not entered")
	}
	s.mu.Lock()
	slot := s.runtimeSlots[environment.ID]
	s.mu.Unlock()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	if err := s.CloseContext(ctx); err == nil {
		cancel()
		t.Fatal("shutdown ignored pending startup")
	}
	cancel()
	released.Do(func() { close(release) })
	select {
	case <-slot.launchDone:
	case <-time.After(2 * time.Second):
		t.Fatal("late channel did not finish failed startup")
	}
	s.mu.Lock()
	retained := slot.process != nil && s.profileUses[environment.ID]
	s.mu.Unlock()
	if !retained {
		t.Fatal("late failed channel was abandoned during shutdown")
	}
	channel.fail.Store(false)
	ctx, cancel = context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := s.CloseContext(ctx); err != nil {
		t.Fatal("explicit application-close retry did not finish the retained channel:", err)
	}
	if factories.Load() != 1 || launches.Load() != 0 {
		t.Fatal("shutdown retry repeated startup side effects")
	}
}

func TestProcessCrashRemainsRootCauseWhenItsChannelCleanupAlsoFails(t *testing.T) {
	channel := &retryableCloseChannel{syntheticRuntimeChannel: &syntheticRuntimeChannel{}}
	channel.fail.Store(true)
	process := newSyntheticRuntimeProcess()
	s, _, kernelID := fingerprintFixture(t, Options{
		OpenProxyChannel: func(_ proxy.Configuration, _ *proxy.Credentials, options proxy.BridgeOptions) (RuntimeProxyChannel, error) {
			channel.channelID = options.ChannelID
			return channel, nil
		},
		LaunchRuntime: func(context.Context, RuntimeLaunch) (RuntimeProcess, error) { return process, nil },
	})
	t.Cleanup(func() { channel.fail.Store(false) })
	proxyRecord := importProxyFixture(t, s, "localhost:8080")
	environment := createRuntimeEnvironment(t, s, kernelID, "合成崩溃并伴随清理失败")
	bindRuntimeProxyFixture(t, s, environment, proxyRecord)
	start := acceptRuntimeTest(t, s, "Runtime.Start", runtimeRequest{EnvironmentID: environment.ID, RequestID: id(), NetworkPolicy: "proxy"})
	if waitKernel(t, s, start.ID).State != "completed" {
		t.Fatal("synthetic startup failed")
	}
	process.mu.Lock()
	process.exitCode = 77
	process.mu.Unlock()
	if err := process.Close(); err != nil {
		t.Fatal(err)
	}
	session := waitRuntimeObservation(t, s, environment.ID, func(session RuntimeSession) bool {
		s.mu.Lock()
		owner := s.runtimeSlots[environment.ID].process
		s.mu.Unlock()
		return session.Error != nil && session.Error.Code == "PROCESS_CRASHED" && owner != nil && owner.Snapshot().NetworkCleanupFailed
	})
	if session.NetworkFault != nil || !session.ResourcesPending {
		t.Fatal("cleanup symptom replaced a crash or released its resources")
	}
	channel.fail.Store(false)
	stop := acceptRuntimeTest(t, s, "Runtime.Stop", runtimeRequest{EnvironmentID: environment.ID, RequestID: id()})
	if waitKernel(t, s, stop.ID).State != "completed" {
		t.Fatal("cleanup retry failed")
	}
	session = view(t, s).RuntimeSessions[environment.ID]
	if session.Error == nil || session.Error.Code != "PROCESS_CRASHED" || session.NetworkFault != nil || session.LastExitCode == nil || *session.LastExitCode != 77 || session.ResourcesPending {
		t.Fatal("successful cleanup erased the process crash or misclassified it as proxy failure")
	}
}

func TestPendingObservationFlushCannotReleaseChannelOwnerWhileCleanupIsBlocked(t *testing.T) {
	release := make(chan struct{})
	var released sync.Once
	channel := &latchedCleanupChannel{&retryableCloseChannel{syntheticRuntimeChannel: &syntheticRuntimeChannel{}, release: release}}
	var failWrites atomic.Bool
	s, _, kernelID := fingerprintFixture(t, Options{
		BeforeCommit: func() error {
			if failWrites.Load() {
				return errors.New("synthetic storage unavailable")
			}
			return nil
		},
		OpenProxyChannel: func(_ proxy.Configuration, _ *proxy.Credentials, options proxy.BridgeOptions) (RuntimeProxyChannel, error) {
			channel.channelID = options.ChannelID
			return channel, channel.Fault()
		},
		LaunchRuntime: func(context.Context, RuntimeLaunch) (RuntimeProcess, error) {
			t.Error("channel failure invoked browser launch")
			return newSyntheticRuntimeProcess(), nil
		},
	})
	t.Cleanup(func() { failWrites.Store(false); released.Do(func() { close(release) }) })
	proxyRecord := importProxyFixture(t, s, "localhost:8080")
	environment := createRuntimeEnvironment(t, s, kernelID, "合成清理中观测写失败")
	bindRuntimeProxyFixture(t, s, environment, proxyRecord)
	start := acceptRuntimeTest(t, s, "Runtime.Start", runtimeRequest{EnvironmentID: environment.ID, RequestID: id(), NetworkPolicy: "proxy"})
	waitRuntimeObservation(t, s, environment.ID, func(session RuntimeSession) bool { return session.State == "error" && session.ResourcesPending })
	// A host-side observation writer races the ongoing cleanup. Use the actual
	// transaction failure/pending retry path, then public reads and Start to
	// check that saving that observation cannot release the live owner.
	failWrites.Store(true)
	s.mu.Lock()
	slot := s.runtimeSlots[environment.ID]
	writeErr := s.persistRuntime(slot, nil, "合成清理仍在进行")
	s.mu.Unlock()
	if writeErr == nil {
		t.Fatal("fixture did not enter pending persistence")
	}
	failWrites.Store(false)
	session := view(t, s).RuntimeSessions[environment.ID]
	if session.PersistencePending || !session.ResourcesPending || session.PID != 0 {
		t.Fatal("pending flush lost the channel-only owner")
	}
	wantError(t, call(s, "Runtime.Start", runtimeRequest{EnvironmentID: environment.ID, RequestID: id(), NetworkPolicy: "proxy"}), "PROFILE_BUSY")
	s.mu.Lock()
	retained := s.profileUses[environment.ID]
	s.mu.Unlock()
	if !retained {
		t.Fatal("error/PID0 observation released a channel still draining")
	}
	released.Do(func() { close(release) })
	if result := waitKernel(t, s, start.ID); result.State != "failed" || result.Error == nil || result.Error.Code != "PROXY_AUTH_FAILED" {
		t.Fatal("drain completion replaced the original start failure")
	}
	if view(t, s).RuntimeSessions[environment.ID].ResourcesPending {
		t.Fatal("successful drain never released the owner")
	}
}

func TestFailedNormalStopRemainsActionableWhileChannelCleanupIsUnconfirmed(t *testing.T) {
	channel := &retryableCloseChannel{syntheticRuntimeChannel: &syntheticRuntimeChannel{}}
	channel.fail.Store(true)
	process := newSyntheticRuntimeProcess()
	s, _, kernelID := fingerprintFixture(t, Options{
		OpenProxyChannel: func(_ proxy.Configuration, _ *proxy.Credentials, options proxy.BridgeOptions) (RuntimeProxyChannel, error) {
			channel.channelID = options.ChannelID
			return channel, nil
		},
		LaunchRuntime: func(context.Context, RuntimeLaunch) (RuntimeProcess, error) { return process, nil },
	})
	t.Cleanup(func() { channel.fail.Store(false) })
	proxyRecord := importProxyFixture(t, s, "localhost:8080")
	environment := createRuntimeEnvironment(t, s, kernelID, "合成停止失败仍可重试")
	bindRuntimeProxyFixture(t, s, environment, proxyRecord)
	start := acceptRuntimeTest(t, s, "Runtime.Start", runtimeRequest{EnvironmentID: environment.ID, RequestID: id(), NetworkPolicy: "proxy"})
	if waitKernel(t, s, start.ID).State != "completed" {
		t.Fatal("synthetic startup failed")
	}
	stop := acceptRuntimeTest(t, s, "Runtime.Stop", runtimeRequest{EnvironmentID: environment.ID, RequestID: id()})
	if waitKernel(t, s, stop.ID).State != "failed" {
		t.Fatal("channel cleanup failure was hidden")
	}
	// Apply the next supervisor observation deterministically after Stop's
	// durable failure. A known normal root exit is not a new stopping task.
	s.mu.Lock()
	slot := s.runtimeSlots[environment.ID]
	s.applyRuntimeFault(slot, slot.process.Snapshot())
	s.mu.Unlock()
	session := view(t, s).RuntimeSessions[environment.ID]
	if session.State != "error" || !session.ResourcesPending {
		t.Fatal("normal root exit disabled the failed close's retry action")
	}
	channel.fail.Store(false)
	retry := acceptRuntimeTest(t, s, "Runtime.Stop", runtimeRequest{EnvironmentID: environment.ID, RequestID: id()})
	if waitKernel(t, s, retry.ID).State != "completed" {
		t.Fatal("retry failed to release the retained channel")
	}
}

func TestShutdownKeepsMigrationOwnerRetryableAfterItsWorkerHasEnded(t *testing.T) {
	s, _ := fixture(t, Options{})
	process := newSyntheticRuntimeProcess()
	process.stopError, process.closeError = errors.New("synthetic normal stop failure"), errors.New("synthetic resource cleanup failure")
	t.Cleanup(func() { process.mu.Lock(); process.closeError = nil; process.mu.Unlock(); _ = process.Close() })
	s.mu.Lock()
	s.migrationTask = &migrationTask{process: process, phase: "exit-unconfirmed"}
	s.mu.Unlock()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	if err := s.CloseContext(ctx); err == nil {
		cancel()
		t.Fatal("shutdown reported complete with retained migration resources")
	}
	cancel()
	select {
	case <-s.closeDone:
		t.Fatal("worker completion closed shutdown before migration resources exited")
	default:
	}
	process.mu.Lock()
	process.closeError = nil
	process.mu.Unlock()
	ctx, cancel = context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	// The journal is still unresolved; shutdown may report that separately.
	_ = s.CloseContext(ctx)
	select {
	case <-process.Done():
	default:
		t.Fatal("explicit retry never reached the retained migration owner")
	}
	select {
	case <-s.closeDone:
	default:
		t.Fatal("resource exit never allowed shutdown to finish")
	}
}
