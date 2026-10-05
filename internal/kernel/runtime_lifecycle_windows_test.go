//go:build windows

package kernel

import (
	"context"
	"errors"
	"net"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/axgiroud312-byte/prism-local-browser/internal/proxy"
)

type cleanupFixtureNetwork struct {
	calls   atomic.Int32
	fail    atomic.Bool
	closeFn func()
	fault   *proxy.CheckError
}

func (*cleanupFixtureNetwork) Endpoint() string                      { return "http://127.0.0.1:18443" }
func (*cleanupFixtureNetwork) BindBrowser(func(net.Conn) bool) error { return nil }
func (*cleanupFixtureNetwork) Failed() <-chan struct{}               { return nil }
func (n *cleanupFixtureNetwork) Fault() *proxy.CheckError            { return n.fault }
func (n *cleanupFixtureNetwork) Close() error {
	n.calls.Add(1)
	if n.closeFn != nil {
		n.closeFn()
	}
	if n.fail.Load() {
		return errors.New("SYNTHETIC_PRIVATE_CLEANUP_DETAIL")
	}
	return nil
}

type managedLifecycleFixture struct {
	mu           sync.Mutex
	rootExited   bool
	children     uint32
	queryErr     error
	code         uint32
	request      func() error
	order        []string
	normalCalls  atomic.Int32
	terminations atomic.Int32
	queries      atomic.Int32
	process      *ManagedProcess
}

func (f *managedLifecycleFixture) exit(code uint32) {
	f.mu.Lock()
	f.rootExited, f.children, f.code, f.queryErr = true, 0, code, nil
	f.mu.Unlock()
}
func (f *managedLifecycleFixture) record(value string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.order = append(f.order, value)
}
func (f *managedLifecycleFixture) releaseOrder() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.order...)
}

// These are host lifecycle hooks, not fake Windows isolation evidence. No
// native process, filesystem grant, socket or network gate is exercised here.
func newManagedLifecycleFixture(t *testing.T, network *cleanupFixtureNetwork) *managedLifecycleFixture {
	t.Helper()
	f := &managedLifecycleFixture{children: 1}
	var ownedNetwork ManagedNetwork
	if network != nil {
		ownedNetwork = network
	}
	f.process = newManagedProcess(&pipeProcess{pid: 4242, createdAt: "2026-10-05T00:00:00Z"}, ownedNetwork, func() { f.record("pins") }, managedProcessControl{
		snapshot: func() RuntimeSnapshot {
			f.mu.Lock()
			defer f.mu.Unlock()
			return RuntimeSnapshot{RootAlive: !f.rootExited, ControlReady: !f.rootExited, ExitKnown: f.rootExited, ExitCode: f.code}
		},
		exitReady: func() (bool, error) {
			f.queries.Add(1)
			f.mu.Lock()
			defer f.mu.Unlock()
			return f.rootExited && f.children == 0, f.queryErr
		},
		requestClose: func(context.Context) error {
			f.normalCalls.Add(1)
			f.mu.Lock()
			request := f.request
			f.mu.Unlock()
			if request != nil {
				return request()
			}
			f.exit(0)
			return nil
		},
		terminate:   func() error { f.terminations.Add(1); f.exit(1); return nil },
		closeWrite:  func() {},
		releasePipe: func() { f.record("pipe") },
	})
	t.Cleanup(func() {
		if network != nil {
			network.fail.Store(false)
		}
		f.exit(0)
		if err := f.process.Close(); err != nil {
			t.Error(err)
		}
	})
	return f
}

func TestManagedExitRetainsPinsForLiveChildrenOrUnknownAccounting(t *testing.T) {
	for _, unknown := range []bool{false, true} {
		t.Run(map[bool]string{false: "children-alive", true: "query-failed"}[unknown], func(t *testing.T) {
			f := newManagedLifecycleFixture(t, nil)
			f.mu.Lock()
			f.rootExited = true
			if unknown {
				f.children, f.queryErr = 0, errors.New("synthetic query failed")
			}
			f.mu.Unlock()
			if f.process.confirmJobExit() {
				t.Fatal("root exit or failed accounting became proof that the tree is empty")
			}
			select {
			case <-f.process.Done():
				t.Fatal("resources released without whole-Job evidence")
			default:
			}
			if len(f.releaseOrder()) != 0 || f.process.Snapshot().ResourcesExited {
				t.Fatal("pins released while child exit was unconfirmed")
			}
		})
	}
}

func TestManagedNetworkFailureRetainsPinsAndNormalStopRetriesAfterCDPExit(t *testing.T) {
	network := &cleanupFixtureNetwork{fault: &proxy.CheckError{Code: "PROXY_AUTH_FAILED", Message: "合成认证故障。"}}
	network.fail.Store(true)
	f := newManagedLifecycleFixture(t, network)
	f.exit(77)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	err := f.process.Stop(ctx)
	if err == nil || strings.Contains(err.Error(), "SYNTHETIC_PRIVATE") {
		t.Fatal("cleanup failure was hidden or leaked raw details")
	}
	select {
	case <-f.process.Done():
		t.Fatal("network failure released the profile")
	default:
	}
	if len(f.releaseOrder()) != 0 || !f.process.Snapshot().NetworkCleanupFailed || f.normalCalls.Load() != 0 {
		t.Fatal("cleanup failed to preserve pins or retried a dead CDP channel")
	}
	network.fail.Store(false)
	if err := f.process.Stop(ctx); err != nil {
		t.Fatal(err)
	}
	snapshot := f.process.Snapshot()
	if !snapshot.ResourcesExited || snapshot.NetworkCleanupFailed || snapshot.ExitCode != 77 || snapshot.ProxyError.Code != "PROXY_AUTH_FAILED" || network.calls.Load() != 2 {
		t.Fatal("cleanup retry changed the exit/fault cause or failed to finish the same owner")
	}
	if !reflect.DeepEqual(f.releaseOrder(), []string{"pipe", "pins"}) {
		t.Fatal("resource release was repeated or ordered incorrectly")
	}
}

func TestManagedNetworkDrainIsOutsideSnapshotLockAndWaitersCannotBypassIt(t *testing.T) {
	network := &cleanupFixtureNetwork{}
	f := newManagedLifecycleFixture(t, network)
	entered, release := make(chan struct{}), make(chan struct{})
	var releaseOnce sync.Once
	defer releaseOnce.Do(func() { close(release) })
	network.closeFn = func() {
		if f.process.Snapshot().ResourcesExited {
			t.Error("cleanup published completion before its own drain")
		}
		close(entered)
		<-release
		f.record("network")
	}
	f.exit(0)
	closed := make(chan error, 1)
	go func() { closed <- f.process.Close() }()
	select {
	case <-entered:
	case <-time.After(2 * time.Second):
		t.Fatal("network drain held the Snapshot mutex")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	if err := f.process.Stop(ctx); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatal("pending network drain reported a successful Stop")
	}
	select {
	case <-f.process.Done():
		t.Fatal("timed-out waiter released the profile")
	default:
	}
	if network.calls.Load() != 1 || len(f.releaseOrder()) != 0 {
		t.Fatal("waiter started overlapping cleanup or released pins early")
	}
	releaseOnce.Do(func() { close(release) })
	select {
	case err := <-closed:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("successful drain never completed")
	}
	if snapshot := f.process.Snapshot(); !snapshot.ResourcesExited || snapshot.NetworkCleanupFailed {
		t.Fatal("late completion retained a stale failure")
	}
	if !reflect.DeepEqual(f.releaseOrder(), []string{"network", "pipe", "pins"}) {
		t.Fatal("Done preceded complete network/handle/pin release")
	}
}

func TestManagedStopAcceptsConfirmedJobExitAfterNormalCommandFailure(t *testing.T) {
	f := newManagedLifecycleFixture(t, nil)
	f.mu.Lock()
	f.request = func() error { f.exit(0); return controlWriteLost() }
	f.mu.Unlock()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := f.process.Stop(ctx); err != nil {
		t.Fatal(err)
	}
	if f.normalCalls.Load() != 1 || f.terminations.Load() != 0 || !f.process.Snapshot().ResourcesExited {
		t.Fatal("confirmed normal exit was treated as a control failure or force-killed")
	}
}

func TestAutomaticManagedCleanupOwnersShareFailureUntilExplicitRetry(t *testing.T) {
	network := &cleanupFixtureNetwork{}
	network.fail.Store(true)
	f := newManagedLifecycleFixture(t, network)
	f.exit(0)
	go f.process.observeExit()
	for attempt := 0; attempt < 2; attempt++ {
		if err := f.process.closeOwned(false); err == nil {
			t.Fatal("automatic cleanup forgot its failure")
		}
	}
	if network.calls.Load() != 1 {
		t.Fatal("automatic observer/startup cleanup silently retried a completed failure")
	}
	network.fail.Store(false)
	if err := f.process.Close(); err != nil {
		t.Fatal(err)
	}
	if network.calls.Load() != 2 || !f.process.Snapshot().ResourcesExited {
		t.Fatal("explicit close did not retry the retained owner")
	}
}

func TestManagedCloseTimeoutKeepsPinsUntilLateDrainReallyFinishes(t *testing.T) {
	network := &cleanupFixtureNetwork{}
	f := newManagedLifecycleFixture(t, network)
	release := make(chan struct{})
	var released sync.Once
	defer released.Do(func() { close(release) })
	network.closeFn = func() { <-release }
	f.exit(0)
	// Exercise the real Close wait limit, rather than only a cancelled Stop.
	err := f.process.Close()
	if err == nil || !f.process.Snapshot().NetworkCleanupFailed || len(f.releaseOrder()) != 0 {
		t.Fatal("timed-out Close falsely released resources or lost its unconfirmed observation")
	}
	select {
	case <-f.process.Done():
		t.Fatal("timeout published Done")
	default:
	}
	released.Do(func() { close(release) })
	select {
	case <-f.process.Done():
	case <-time.After(2 * time.Second):
		t.Fatal("late successful cleanup never completed")
	}
	if err := f.process.closeOwned(false); err != nil {
		t.Fatal(err)
	}
	if snapshot := f.process.Snapshot(); !snapshot.ResourcesExited || snapshot.NetworkCleanupFailed || network.calls.Load() != 1 {
		t.Fatal("late completion was polluted by an old timeout or repeated network cleanup")
	}
}

func TestManagedForceCloseDoesNotWaitForNormalStopGate(t *testing.T) {
	f := newManagedLifecycleFixture(t, nil)
	entered, release := make(chan struct{}), make(chan struct{})
	var released sync.Once
	defer released.Do(func() { close(release) })
	f.mu.Lock()
	f.request = func() error { close(entered); <-release; return controlWriteLost() }
	f.mu.Unlock()
	go f.process.observeExit()
	stopped := make(chan error, 1)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	go func() { stopped <- f.process.Stop(ctx) }()
	select {
	case <-entered:
	case <-ctx.Done():
		t.Fatal("normal close never entered its gate")
	}
	closed := make(chan error, 1)
	go func() { closed <- f.process.Close() }()
	select {
	case err := <-closed:
		if err != nil {
			t.Fatal(err)
		}
	case <-ctx.Done():
		t.Fatal("force close waited behind the blocked normal command")
	}
	queries := f.queries.Load()
	released.Do(func() { close(release) })
	select {
	case err := <-stopped:
		if err != nil {
			t.Fatal(err)
		}
	case <-ctx.Done():
		t.Fatal("normal waiter failed to observe actual resource exit")
	}
	if err := f.process.Close(); err != nil {
		t.Fatal(err)
	}
	if f.queries.Load() != queries || f.terminations.Load() != 1 || !reflect.DeepEqual(f.releaseOrder(), []string{"pipe", "pins"}) {
		t.Fatal("a late waiter queried/terminated released handles or repeated release")
	}
}
