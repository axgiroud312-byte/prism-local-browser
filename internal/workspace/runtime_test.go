package workspace

import (
	"context"
	"errors"
	"reflect"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/axgiroud312-byte/prism-local-browser/internal/kernel"
)

// This is a host-only lifecycle seam, never fake browser/kernel evidence.
type syntheticRuntimeProcess struct {
	done        chan struct{}
	once        sync.Once
	mu          sync.Mutex
	stopError   error
	closeError  error
	rootExited  bool
	controlLost bool
	exitCode    uint32
	closeCalls  atomic.Int32
	stopPause   <-chan struct{}
}

func newSyntheticRuntimeProcess() *syntheticRuntimeProcess {
	return &syntheticRuntimeProcess{done: make(chan struct{})}
}
func (p *syntheticRuntimeProcess) PID() int              { return 4242 }
func (p *syntheticRuntimeProcess) CreatedAt() string     { return "2026-10-01T00:00:00Z" }
func (p *syntheticRuntimeProcess) Done() <-chan struct{} { return p.done }
func (p *syntheticRuntimeProcess) Alive() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.rootExited {
		return false
	}
	select {
	case <-p.done:
		return false
	default:
		return true
	}
}
func (p *syntheticRuntimeProcess) Snapshot() kernel.RuntimeSnapshot {
	p.mu.Lock()
	defer p.mu.Unlock()
	result := kernel.RuntimeSnapshot{RootAlive: !p.rootExited, ControlReady: !p.controlLost}
	select {
	case <-p.done:
		result.RootAlive, result.ControlReady, result.ResourcesExited = false, false, true
		result.ExitKnown, result.ExitCode = true, p.exitCode
	default:
		if p.rootExited {
			result.ExitKnown, result.ExitCode = true, p.exitCode
		}
	}
	return result
}
func (p *syntheticRuntimeProcess) Stop(ctx context.Context) error {
	p.mu.Lock()
	err := p.stopError
	p.mu.Unlock()
	if err == nil {
		p.once.Do(func() { close(p.done) })
		if p.stopPause != nil {
			select {
			case <-p.stopPause:
			case <-ctx.Done():
				return ctx.Err()
			}
		}
	}
	return err
}
func (p *syntheticRuntimeProcess) Close() error {
	p.closeCalls.Add(1)
	p.mu.Lock()
	err := p.closeError
	p.mu.Unlock()
	if err == nil {
		p.once.Do(func() { close(p.done) })
	}
	return err
}

func createRuntimeEnvironment(t *testing.T, s *Service, kernelID, name string) Environment {
	t.Helper()
	p := generateFingerprint(t, s, preview(t, s, "create", ""), kernelID, false)
	p.Environment.Name = name
	value[map[string]any](t, call(s, "Environment.Create", Mutation{PreviewID: p.PreviewID, Configuration: p.Environment.Configuration, Count: 1, ProfileHash: p.Fingerprint.PreviewProfile.ConfigHash, RequestID: id()}))
	for _, environment := range view(t, s).State.Environments {
		if environment.Name == name {
			return environment
		}
	}
	t.Fatal("created environment not found")
	return Environment{}
}

func acceptRuntimeTest(t *testing.T, s *Service, method string, input runtimeRequest) Operation {
	t.Helper()
	return value[struct {
		Operation Operation `json:"operation"`
	}](t, call(s, method, input)).Operation
}

func TestRuntimeAcceptanceIsNotReadinessAndDuplicateRequestsDoNotLaunchTwice(t *testing.T) {
	entered := make(chan RuntimeLaunch, 1)
	ready := make(chan struct{})
	process := newSyntheticRuntimeProcess()
	var launches atomic.Int32
	s, _, kernelID := fingerprintFixture(t, Options{LaunchRuntime: func(ctx context.Context, input RuntimeLaunch) (RuntimeProcess, error) {
		launches.Add(1)
		entered <- input
		select {
		case <-ready:
			return process, nil
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}})
	environment := createRuntimeEnvironment(t, s, kernelID, "合成运行A")
	before := view(t, s).Fingerprints[environment.ID].Profile
	input := runtimeRequest{EnvironmentID: environment.ID, RequestID: id(), NetworkPolicy: "direct"}
	accepted := acceptRuntimeTest(t, s, "Runtime.Start", input)
	select {
	case launch := <-entered:
		if launch.DataReference != "environments/"+environment.ID+"/user-data" || launch.Profile.Seed != environment.Seed {
			t.Fatal("launcher received wrong saved identity/data reference")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("launcher not reached")
	}
	if view(t, s).State.Environments[0].Status != "starting" {
		t.Fatal("accepted was reported as running")
	}
	if again := acceptRuntimeTest(t, s, "Runtime.Start", input); again.ID != accepted.ID {
		t.Fatal("request replay allocated another task")
	}
	input.RequestID = id()
	if again := acceptRuntimeTest(t, s, "Runtime.Start", input); again.ID != accepted.ID {
		t.Fatal("same environment duplicated a pending session")
	}
	if launches.Load() != 1 {
		t.Fatal("duplicate native launch")
	}
	close(ready)
	if waitKernel(t, s, accepted.ID).State != "completed" {
		t.Fatal("ready process did not complete start")
	}
	if view(t, s).State.Environments[0].Status != "running" {
		t.Fatal("controlled readiness was not published")
	}
	p := preview(t, s, "edit", environment.ID)
	p.Environment.Name = "合成运行A改名"
	saved := value[struct {
		Environment struct {
			Record Environment `json:"record"`
		} `json:"environment"`
	}](t, call(s, "Environment.Update", Mutation{PreviewID: p.PreviewID, Configuration: p.Environment.Configuration, ExpectedRevision: p.ExpectedRevision, RequestID: id()}))
	if saved.Environment.Record.Status != "running" {
		t.Fatal("running metadata commit returned a contradictory stopped record")
	}
	if !reflect.DeepEqual(before, view(t, s).Fingerprints[environment.ID].Profile) {
		t.Fatal("safe running metadata edit changed fingerprint")
	}
	p = preview(t, s, "edit", environment.ID)
	p.Environment.Timezone = "Europe/Berlin"
	wantError(t, call(s, "Environment.Update", Mutation{PreviewID: p.PreviewID, Configuration: p.Environment.Configuration, ExpectedRevision: p.ExpectedRevision, RequestID: id()}), "PROFILE_BUSY")
	stopped := acceptRuntimeTest(t, s, "Runtime.Stop", runtimeRequest{EnvironmentID: environment.ID, RequestID: id()})
	if waitKernel(t, s, stopped.ID).State != "completed" {
		t.Fatal("normal stop did not complete")
	}
	if !reflect.DeepEqual(before, view(t, s).Fingerprints[environment.ID].Profile) {
		t.Fatal("stop changed fingerprint")
	}
}

func TestRuntimeRequiresExplicitDirectAndNeverBypassesBoundProxy(t *testing.T) {
	var launches atomic.Int32
	s, _, kernelID := fingerprintFixture(t, Options{LaunchRuntime: func(context.Context, RuntimeLaunch) (RuntimeProcess, error) {
		launches.Add(1)
		return newSyntheticRuntimeProcess(), nil
	}})
	environment := createRuntimeEnvironment(t, s, kernelID, "合成代理阻断")
	wantError(t, call(s, "Runtime.Start", runtimeRequest{EnvironmentID: environment.ID, RequestID: id()}), "VALIDATION_FAILED")
	wantError(t, call(s, "Runtime.Start", map[string]any{"environmentId": environment.ID, "requestId": id(), "networkPolicy": "direct", "arguments": []string{"--no-sandbox"}}), "VALIDATION_FAILED")
	if _, err := s.db.Exec("INSERT INTO proxies(id) VALUES('synthetic-proxy')"); err != nil {
		t.Fatal(err)
	}
	p := preview(t, s, "edit", environment.ID)
	p.Environment.ProxyID = "synthetic-proxy"
	value[map[string]any](t, call(s, "Environment.Update", Mutation{PreviewID: p.PreviewID, Configuration: p.Environment.Configuration, ExpectedRevision: p.ExpectedRevision, RequestID: id()}))
	wantError(t, call(s, "Runtime.Start", runtimeRequest{EnvironmentID: environment.ID, RequestID: id(), NetworkPolicy: "direct"}), "PROXY_UNSUPPORTED")
	if launches.Load() != 0 {
		t.Fatal("unsupported policy executed a process")
	}
}

func TestRuntimePendingStopCancelsOnlyItsQueuedStart(t *testing.T) {
	entered := make(chan struct{}, 1)
	s, _, kernelID := fingerprintFixture(t, Options{LaunchRuntime: func(ctx context.Context, _ RuntimeLaunch) (RuntimeProcess, error) {
		entered <- struct{}{}
		<-ctx.Done()
		return nil, ctx.Err()
	}})
	environment := createRuntimeEnvironment(t, s, kernelID, "合成取消启动")
	started := acceptRuntimeTest(t, s, "Runtime.Start", runtimeRequest{EnvironmentID: environment.ID, RequestID: id(), NetworkPolicy: "direct"})
	select {
	case <-entered:
	case <-time.After(2 * time.Second):
		t.Fatal("launcher not reached")
	}
	stopped := acceptRuntimeTest(t, s, "Runtime.Stop", runtimeRequest{EnvironmentID: environment.ID, RequestID: id()})
	if waitKernel(t, s, started.ID).State != "cancelled" || waitKernel(t, s, stopped.ID).State != "completed" {
		t.Fatal("cancelled start/stop did not terminate accurately")
	}
	if view(t, s).State.Environments[0].Status != "ready" {
		t.Fatal("cancel left a fake running state")
	}
}

func TestRuntimeStopFailureRetainsBusyLeaseUntilOwnedTreeActuallyExits(t *testing.T) {
	process := newSyntheticRuntimeProcess()
	process.stopError = errors.New("synthetic stop failure")
	s, _, kernelID := fingerprintFixture(t, Options{LaunchRuntime: func(context.Context, RuntimeLaunch) (RuntimeProcess, error) { return process, nil }})
	environment := createRuntimeEnvironment(t, s, kernelID, "合成停止失败")
	started := acceptRuntimeTest(t, s, "Runtime.Start", runtimeRequest{EnvironmentID: environment.ID, RequestID: id(), NetworkPolicy: "direct"})
	waitKernel(t, s, started.ID)
	stopped := acceptRuntimeTest(t, s, "Runtime.Stop", runtimeRequest{EnvironmentID: environment.ID, RequestID: id()})
	if waitKernel(t, s, stopped.ID).State != "failed" {
		t.Fatal("failed stop reported complete")
	}
	p := preview(t, s, "edit", environment.ID)
	wantError(t, call(s, "Preview.Regenerate", map[string]string{"previewId": p.PreviewID}), "PROFILE_BUSY")
	wantError(t, call(s, "Runtime.Start", runtimeRequest{EnvironmentID: environment.ID, RequestID: id(), NetworkPolicy: "direct"}), "PROFILE_BUSY")
	process.mu.Lock()
	process.stopError = nil
	process.mu.Unlock()
	retry := acceptRuntimeTest(t, s, "Runtime.Stop", runtimeRequest{EnvironmentID: environment.ID, RequestID: id()})
	if waitKernel(t, s, retry.ID).State != "completed" {
		t.Fatal("failed stop could not be retried")
	}
}

func TestRuntimeFailedStartupCleanupCannotReleaseAnUnconfirmedOwnedProcess(t *testing.T) {
	process := newSyntheticRuntimeProcess()
	process.closeError = errors.New("synthetic job still occupied")
	s, _, kernelID := fingerprintFixture(t, Options{LaunchRuntime: func(context.Context, RuntimeLaunch) (RuntimeProcess, error) {
		return process, errors.New("synthetic failed readiness")
	}})
	environment := createRuntimeEnvironment(t, s, kernelID, "合成清理失败")
	started := acceptRuntimeTest(t, s, "Runtime.Start", runtimeRequest{EnvironmentID: environment.ID, RequestID: id(), NetworkPolicy: "direct"})
	if waitKernel(t, s, started.ID).State != "failed" {
		t.Fatal("failed readiness reported running")
	}
	session := view(t, s).RuntimeSessions[environment.ID]
	if session.State != "error" || session.PID != process.PID() {
		t.Fatal("failed cleanup lost the owned process identity")
	}
	wantError(t, call(s, "Runtime.Start", runtimeRequest{EnvironmentID: environment.ID, RequestID: id(), NetworkPolicy: "direct"}), "PROFILE_BUSY")
	p := preview(t, s, "edit", environment.ID)
	wantError(t, call(s, "Preview.Regenerate", map[string]string{"previewId": p.PreviewID}), "PROFILE_BUSY")
	stopped := acceptRuntimeTest(t, s, "Runtime.Stop", runtimeRequest{EnvironmentID: environment.ID, RequestID: id()})
	if waitKernel(t, s, stopped.ID).State != "completed" {
		t.Fatal("owned failed-start process could not be stopped")
	}
}

func TestRuntimeDefaultLaunchReturnsARealNilInterfaceBeforeProcessCreation(t *testing.T) {
	process, err := launchManagedRuntime(context.Background(), RuntimeLaunch{})
	if err == nil || process != nil {
		t.Fatal("failed native launch returned a typed-nil runtime interface")
	}
}

func TestRuntimeBusyMetadataCannotCommitAnOldRestorePreview(t *testing.T) {
	process := newSyntheticRuntimeProcess()
	s, _, kernelID := fingerprintFixture(t, Options{LaunchRuntime: func(context.Context, RuntimeLaunch) (RuntimeProcess, error) { return process, nil }})
	environment := createRuntimeEnvironment(t, s, kernelID, "合成旧回滚预览")
	before := view(t, s).Fingerprints[environment.ID]
	p := preview(t, s, "edit", environment.ID)
	p = value[Preview](t, call(s, "Fingerprint.PreviewRestore", map[string]any{"previewId": p.PreviewID, "revision": before.Profile.ConfigRevision}))
	started := acceptRuntimeTest(t, s, "Runtime.Start", runtimeRequest{EnvironmentID: environment.ID, RequestID: id(), NetworkPolicy: "direct"})
	if waitKernel(t, s, started.ID).State != "completed" {
		t.Fatal("fixture start failed")
	}
	p.Environment.Name = "合成运行中伪装元数据"
	wantError(t, call(s, "Fingerprint.CommitRevision", Mutation{EnvironmentID: environment.ID, PreviewID: p.PreviewID, Configuration: p.Environment.Configuration, ExpectedRevision: p.ExpectedRevision, ProfileHash: p.Fingerprint.PreviewProfile.ConfigHash, RequestID: id()}), "PROFILE_BUSY")
	if !reflect.DeepEqual(before, view(t, s).Fingerprints[environment.ID]) {
		t.Fatal("busy restore preview changed the current profile")
	}
}

func TestRuntimeAnExitedRootWithLiveChildrenIsNeverPublishedAsRunning(t *testing.T) {
	process := newSyntheticRuntimeProcess()
	process.rootExited = true
	s, _, kernelID := fingerprintFixture(t, Options{LaunchRuntime: func(context.Context, RuntimeLaunch) (RuntimeProcess, error) { return process, nil }})
	environment := createRuntimeEnvironment(t, s, kernelID, "合成主进程退出")
	started := acceptRuntimeTest(t, s, "Runtime.Start", runtimeRequest{EnvironmentID: environment.ID, RequestID: id(), NetworkPolicy: "direct"})
	if waitKernel(t, s, started.ID).State != "failed" || view(t, s).RuntimeSessions[environment.ID].State == "running" {
		t.Fatal("root death was hidden by still-owned child resources")
	}
}

func TestRuntimeControlWriteLossDoesNotPromiseANormalCloseRetry(t *testing.T) {
	process := newSyntheticRuntimeProcess()
	process.stopError = &kernel.Problem{Code: "CONTROL_CHANNEL_LOST", Reason: "control-write-unavailable", Message: "synthetic lost write channel", Retryable: false}
	s, _, kernelID := fingerprintFixture(t, Options{LaunchRuntime: func(context.Context, RuntimeLaunch) (RuntimeProcess, error) { return process, nil }})
	environment := createRuntimeEnvironment(t, s, kernelID, "合成控制通道丢失")
	started := acceptRuntimeTest(t, s, "Runtime.Start", runtimeRequest{EnvironmentID: environment.ID, RequestID: id(), NetworkPolicy: "direct"})
	waitKernel(t, s, started.ID)
	stopped := acceptRuntimeTest(t, s, "Runtime.Stop", runtimeRequest{EnvironmentID: environment.ID, RequestID: id()})
	result := waitKernel(t, s, stopped.ID)
	if result.State != "failed" || result.Error == nil || result.Error.Code != "CONTROL_CHANNEL_LOST" || result.Error.Retryable {
		t.Fatal("lost control was advertised as a retryable normal close")
	}
	if view(t, s).RuntimeSessions[environment.ID].PID == 0 {
		t.Fatal("control loss discarded a still-owned process")
	}
}

func TestRuntimeCloseWaitersShareCompletionAndDoNotAbandonDatabaseCleanup(t *testing.T) {
	process := newSyntheticRuntimeProcess()
	process.stopError, process.closeError = errors.New("synthetic held stop"), errors.New("synthetic held job")
	s, _, kernelID := fingerprintFixture(t, Options{LaunchRuntime: func(context.Context, RuntimeLaunch) (RuntimeProcess, error) { return process, nil }})
	t.Cleanup(func() { process.once.Do(func() { close(process.done) }) })
	environment := createRuntimeEnvironment(t, s, kernelID, "合成延迟清理")
	started := acceptRuntimeTest(t, s, "Runtime.Start", runtimeRequest{EnvironmentID: environment.ID, RequestID: id(), NetworkPolicy: "direct"})
	waitKernel(t, s, started.ID)
	for attempt := 0; attempt < 2; attempt++ {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
		err := s.CloseContext(ctx)
		cancel()
		if err == nil {
			t.Fatal("unconfirmed close reported success to another waiter")
		}
	}
	process.once.Do(func() { close(process.done) })
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := s.CloseContext(ctx); err != nil {
		t.Fatal(err)
	}
	if err := s.db.Ping(); err == nil {
		t.Fatal("completed cleanup left the database open")
	}
}
