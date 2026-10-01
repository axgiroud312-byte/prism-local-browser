package workspace

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"path/filepath"
	"reflect"
	"sync/atomic"
	"testing"
	"time"

	"github.com/axgiroud312-byte/prism-local-browser/internal/kernel"
)

func waitRuntimeObservation(t *testing.T, s *Service, environmentID string, predicate func(RuntimeSession) bool) RuntimeSession {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		session := view(t, s).RuntimeSessions[environmentID]
		if predicate(session) {
			return session
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("runtime observation did not arrive")
	return RuntimeSession{}
}

// Restores a stale durable record after clean synthetic shutdown. This is
// fault injection, NOT actual application crash or Windows process evidence.
func restoreSyntheticRuntimeRecord(t *testing.T, root string, session RuntimeSession) {
	t.Helper()
	db, err := sql.Open("sqlite", filepath.Join(root, "app.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	encoded, err := json.Marshal(session)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec("UPDATE runtime_sessions SET record_json=? WHERE environment_id=?", string(encoded), session.EnvironmentID); err != nil {
		t.Fatal(err)
	}
}

func TestRuntimeForceStopRequiresFailedNormalStopAndExactCurrentSession(t *testing.T) {
	first, second := newSyntheticRuntimeProcess(), newSyntheticRuntimeProcess()
	first.stopError = errors.New("synthetic normal stop timeout")
	var launches atomic.Int32
	s, _, kernelID := fingerprintFixture(t, Options{LaunchRuntime: func(context.Context, RuntimeLaunch) (RuntimeProcess, error) {
		if launches.Add(1) == 1 {
			return first, nil
		}
		return second, nil
	}})
	environment := createRuntimeEnvironment(t, s, kernelID, "合成指定会话结束")
	start := acceptRuntimeTest(t, s, "Runtime.Start", runtimeRequest{EnvironmentID: environment.ID, RequestID: id(), NetworkPolicy: "direct"})
	waitKernel(t, s, start.ID)
	session := view(t, s).RuntimeSessions[environment.ID]
	force := runtimeRequest{EnvironmentID: environment.ID, SessionID: session.SessionID, RequestID: id()}
	wantError(t, call(s, "Runtime.ForceStop", force), "FORCE_STOP_NOT_ALLOWED")
	if first.closeCalls.Load() != 0 {
		t.Fatal("force used before normal close failed")
	}
	stop := acceptRuntimeTest(t, s, "Runtime.Stop", runtimeRequest{EnvironmentID: environment.ID, RequestID: id()})
	if waitKernel(t, s, stop.ID).State != "failed" || !view(t, s).RuntimeSessions[environment.ID].CanForce {
		t.Fatal("failed stop did not make the owned force choice available")
	}
	forced := acceptRuntimeTest(t, s, "Runtime.ForceStop", force)
	if waitKernel(t, s, forced.ID).State != "completed" || first.closeCalls.Load() != 1 {
		t.Fatal("specified owned job was not stopped once")
	}
	newStart := acceptRuntimeTest(t, s, "Runtime.Start", runtimeRequest{EnvironmentID: environment.ID, RequestID: id(), NetworkPolicy: "direct"})
	waitKernel(t, s, newStart.ID)
	if replay := acceptRuntimeTest(t, s, "Runtime.ForceStop", force); replay.ID != forced.ID {
		t.Fatal("force request replay created a new task")
	}
	force.RequestID = id()
	wantError(t, call(s, "Runtime.ForceStop", force), "REVISION_CONFLICT")
	if second.closeCalls.Load() != 0 {
		t.Fatal("old session action stopped the newly reused PID/session")
	}
	wantError(t, call(s, "Runtime.ForceStop", map[string]any{"environmentId": environment.ID, "sessionId": session.SessionID, "requestId": id(), "pid": 4242}), "VALIDATION_FAILED")
}

func TestRuntimeCrashKeepsBusyUntilResourcesExitAndPersistsActualExitCode(t *testing.T) {
	process := newSyntheticRuntimeProcess()
	s, _, kernelID := fingerprintFixture(t, Options{LaunchRuntime: func(context.Context, RuntimeLaunch) (RuntimeProcess, error) { return process, nil }})
	environment := createRuntimeEnvironment(t, s, kernelID, "合成内核崩溃")
	before := view(t, s).Fingerprints[environment.ID].Profile
	start := acceptRuntimeTest(t, s, "Runtime.Start", runtimeRequest{EnvironmentID: environment.ID, RequestID: id(), NetworkPolicy: "direct"})
	waitKernel(t, s, start.ID)
	process.mu.Lock()
	process.rootExited, process.exitCode = true, 77
	process.mu.Unlock()
	waitRuntimeObservation(t, s, environment.ID, func(session RuntimeSession) bool {
		return session.Error != nil && session.Error.Code == "PROCESS_CRASHED"
	})
	p := preview(t, s, "edit", environment.ID)
	wantError(t, call(s, "Preview.Regenerate", map[string]string{"previewId": p.PreviewID}), "PROFILE_BUSY")
	process.once.Do(func() { close(process.done) })
	session := waitRuntimeObservation(t, s, environment.ID, func(session RuntimeSession) bool { return session.PID == 0 && session.LastExitCode != nil })
	if session.LastExitCode == nil || *session.LastExitCode != 77 || session.Error == nil || session.Error.Code != "PROCESS_CRASHED" {
		t.Fatal("crash was replaced with fake successful stop")
	}
	if !reflect.DeepEqual(before, view(t, s).Fingerprints[environment.ID].Profile) {
		t.Fatal("crash changed fixed identity")
	}
	var events int
	if err := s.db.QueryRow("SELECT COUNT(*) FROM runtime_events WHERE error_code='PROCESS_CRASHED' AND session_id=?", session.SessionID).Scan(&events); err != nil || events < 1 {
		t.Fatal("redacted crash activity was not persisted")
	}
}

func TestRuntimeReadinessTimeoutIsDistinctFromControlChannelLoss(t *testing.T) {
	s, _, kernelID := fingerprintFixture(t, Options{LaunchRuntime: func(context.Context, RuntimeLaunch) (RuntimeProcess, error) { return nil, context.DeadlineExceeded }})
	environment := createRuntimeEnvironment(t, s, kernelID, "合成就绪超时")
	start := acceptRuntimeTest(t, s, "Runtime.Start", runtimeRequest{EnvironmentID: environment.ID, RequestID: id(), NetworkPolicy: "direct"})
	result := waitKernel(t, s, start.ID)
	if result.Error == nil || result.Error.Code != "PROCESS_READY_TIMEOUT" || view(t, s).RuntimeSessions[environment.ID].CanForce {
		t.Fatal("readiness timeout got another failure/force identity")
	}
}

func TestRuntimeControlLossKeepsLiveIdentityAndRequiresNormalStopBeforeForce(t *testing.T) {
	process := newSyntheticRuntimeProcess()
	s, _, kernelID := fingerprintFixture(t, Options{LaunchRuntime: func(context.Context, RuntimeLaunch) (RuntimeProcess, error) { return process, nil }})
	environment := createRuntimeEnvironment(t, s, kernelID, "合成管道断开")
	start := acceptRuntimeTest(t, s, "Runtime.Start", runtimeRequest{EnvironmentID: environment.ID, RequestID: id(), NetworkPolicy: "direct"})
	waitKernel(t, s, start.ID)
	process.mu.Lock()
	process.controlLost = true
	process.stopError = &kernel.Problem{Code: "CONTROL_CHANNEL_LOST", Message: "synthetic read/write loss", Retryable: false}
	process.mu.Unlock()
	session := waitRuntimeObservation(t, s, environment.ID, func(session RuntimeSession) bool {
		return session.Error != nil && session.Error.Code == "CONTROL_CHANNEL_LOST"
	})
	if session.CanControl || session.CanForce || session.PID == 0 {
		t.Fatal("control loss was treated as stopped or prematurely forceable")
	}
	wantError(t, call(s, "Runtime.ForceStop", runtimeRequest{EnvironmentID: environment.ID, SessionID: session.SessionID, RequestID: id()}), "FORCE_STOP_NOT_ALLOWED")
	stop := acceptRuntimeTest(t, s, "Runtime.Stop", runtimeRequest{EnvironmentID: environment.ID, RequestID: id()})
	waitKernel(t, s, stop.ID)
	if !view(t, s).RuntimeSessions[environment.ID].CanForce {
		t.Fatal("actual normal stop failure did not permit owned force choice")
	}
}

func TestRuntimeReopenDoesNotTrustSavedRunningAndRetainsFixedInputs(t *testing.T) {
	process := newSyntheticRuntimeProcess()
	s, root, kernelID := fingerprintFixture(t, Options{LaunchRuntime: func(context.Context, RuntimeLaunch) (RuntimeProcess, error) { return process, nil }})
	environment := createRuntimeEnvironment(t, s, kernelID, "合成重开核对")
	start := acceptRuntimeTest(t, s, "Runtime.Start", runtimeRequest{EnvironmentID: environment.ID, RequestID: id(), NetworkPolicy: "direct"})
	waitKernel(t, s, start.ID)
	before := view(t, s)
	saved := before.RuntimeSessions[environment.ID]
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	restoreSyntheticRuntimeRecord(t, root, saved)
	var inspected bool
	reopened, err := Open(root, Options{InspectRuntime: func(session RuntimeSession) (kernel.ManagedRecovery, error) {
		inspected = true
		if session.RootPID != saved.RootPID || session.ProcessCreatedAt != saved.ProcessCreatedAt || session.SessionID != saved.SessionID {
			t.Fatal("reopen inspector lost the saved composite identity")
		}
		return kernel.ManagedRecovery{ProcessState: "exited", DirectoryFree: true, SessionMatches: true, ResourcesExited: true}, nil
	}})
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	after := view(t, reopened)
	actual := after.RuntimeSessions[environment.ID]
	if !inspected || actual.State == "running" || actual.PID != 0 || actual.CanControl || actual.CanForce || actual.NeedsReconcile {
		t.Fatal("saved running was trusted without real identity/lock reconciliation")
	}
	if !reflect.DeepEqual(before.Fingerprints[environment.ID], after.Fingerprints[environment.ID]) || before.DataReferences[environment.ID] != after.DataReferences[environment.ID] {
		t.Fatal("reconciliation altered fixed inputs/data reference")
	}
}

func TestRuntimeReusedPIDOrLockedDirectoryNeverGrantsProcessControl(t *testing.T) {
	process := newSyntheticRuntimeProcess()
	s, root, kernelID := fingerprintFixture(t, Options{LaunchRuntime: func(context.Context, RuntimeLaunch) (RuntimeProcess, error) { return process, nil }})
	environment := createRuntimeEnvironment(t, s, kernelID, "合成PID复用")
	start := acceptRuntimeTest(t, s, "Runtime.Start", runtimeRequest{EnvironmentID: environment.ID, RequestID: id(), NetworkPolicy: "direct"})
	waitKernel(t, s, start.ID)
	saved := view(t, s).RuntimeSessions[environment.ID]
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	restoreSyntheticRuntimeRecord(t, root, saved)
	var free atomic.Bool
	reopened, err := Open(root, Options{InspectRuntime: func(RuntimeSession) (kernel.ManagedRecovery, error) {
		return kernel.ManagedRecovery{ProcessState: "reused", DirectoryFree: free.Load(), SessionMatches: true, ResourcesExited: true}, nil
	}})
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	actual := view(t, reopened).RuntimeSessions[environment.ID]
	if !actual.NeedsReconcile || actual.CanForce || actual.CanControl {
		t.Fatal("directory still occupied but stale PID got control")
	}
	wantError(t, call(reopened, "Runtime.Start", runtimeRequest{EnvironmentID: environment.ID, RequestID: id(), NetworkPolicy: "direct"}), "PROFILE_BUSY")
	wantError(t, call(reopened, "Runtime.ForceStop", runtimeRequest{EnvironmentID: environment.ID, SessionID: actual.SessionID, RequestID: id()}), "FORCE_STOP_NOT_ALLOWED")
	p := preview(t, reopened, "edit", environment.ID)
	p.Environment.Note = "仍允许安全备注"
	value[map[string]any](t, call(reopened, "Environment.Update", Mutation{PreviewID: p.PreviewID, Configuration: p.Environment.Configuration, ExpectedRevision: p.ExpectedRevision, RequestID: id()}))
	p = preview(t, reopened, "edit", environment.ID)
	wantError(t, call(reopened, "Preview.Regenerate", map[string]string{"previewId": p.PreviewID}), "PROFILE_BUSY")
	free.Store(true)
	reconcile := acceptRuntimeTest(t, reopened, "Runtime.Reconcile", runtimeRequest{EnvironmentID: environment.ID, SessionID: actual.SessionID, RequestID: id()})
	if waitKernel(t, reopened, reconcile.ID).State != "completed" {
		t.Fatal("explicit reconciliation did not complete")
	}
	final := view(t, reopened).RuntimeSessions[environment.ID]
	if final.NeedsReconcile || final.Error == nil || final.Error.Code != "PROCESS_ID_REUSED" || final.CanForce || final.CanControl {
		t.Fatal("PID reuse was not kept separate from process ownership")
	}
}

func TestRuntimeV3MigrationPreservesAllFixedProfiles(t *testing.T) {
	s, root, kernelID := fingerprintFixture(t, Options{})
	environment := createRuntimeEnvironment(t, s, kernelID, "合成schema3升级")
	before := view(t, s)
	for _, statement := range []string{"DROP TABLE runtime_events", "DROP TABLE runtime_sessions", "PRAGMA user_version=3"} {
		if _, err := s.db.Exec(statement); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := Open(root, Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	var version int
	if err := reopened.db.QueryRow("PRAGMA user_version").Scan(&version); err != nil || version != 4 {
		t.Fatal("runtime schema migration did not complete")
	}
	if !reflect.DeepEqual(before.Fingerprints[environment.ID], view(t, reopened).Fingerprints[environment.ID]) {
		t.Fatal("runtime schema migration regenerated fixed profile")
	}
}

func TestRuntimeLateNormalStopCannotReleaseTheNextSessionLease(t *testing.T) {
	first, second := newSyntheticRuntimeProcess(), newSyntheticRuntimeProcess()
	pause := make(chan struct{})
	first.stopPause = pause
	var released atomic.Bool
	defer func() {
		if released.CompareAndSwap(false, true) {
			close(pause)
		}
	}()
	var launches atomic.Int32
	s, _, kernelID := fingerprintFixture(t, Options{LaunchRuntime: func(context.Context, RuntimeLaunch) (RuntimeProcess, error) {
		if launches.Add(1) == 1 {
			return first, nil
		}
		return second, nil
	}})
	environment := createRuntimeEnvironment(t, s, kernelID, "合成迟到正常关闭")
	started := acceptRuntimeTest(t, s, "Runtime.Start", runtimeRequest{EnvironmentID: environment.ID, RequestID: id(), NetworkPolicy: "direct"})
	waitKernel(t, s, started.ID)
	oldSession := view(t, s).RuntimeSessions[environment.ID]
	stop := acceptRuntimeTest(t, s, "Runtime.Stop", runtimeRequest{EnvironmentID: environment.ID, RequestID: id()})
	waitRuntimeObservation(t, s, environment.ID, func(session RuntimeSession) bool { return session.PID == 0 && session.State == "stopping" })
	wantError(t, call(s, "Runtime.Start", runtimeRequest{EnvironmentID: environment.ID, RequestID: id(), NetworkPolicy: "direct"}), "PROFILE_BUSY")
	if launches.Load() != 1 {
		t.Fatal("a new start overtook the old stop's terminal result")
	}
	p := preview(t, s, "edit", environment.ID)
	wantError(t, call(s, "Preview.Regenerate", map[string]string{"previewId": p.PreviewID}), "PROFILE_BUSY")
	if released.CompareAndSwap(false, true) {
		close(pause)
	}
	if waitKernel(t, s, stop.ID).State != "completed" {
		t.Fatal("old stop did not finish")
	}
	newStart := acceptRuntimeTest(t, s, "Runtime.Start", runtimeRequest{EnvironmentID: environment.ID, RequestID: id(), NetworkPolicy: "direct"})
	waitKernel(t, s, newStart.ID)
	current := view(t, s).RuntimeSessions[environment.ID]
	if current.SessionID == oldSession.SessionID || current.State != "running" {
		t.Fatal("late old stop overwrote the new session")
	}
	p = preview(t, s, "edit", environment.ID)
	wantError(t, call(s, "Preview.Regenerate", map[string]string{"previewId": p.PreviewID}), "PROFILE_BUSY")
}

func TestRuntimeStopStorageFailureIsVisibleAndRetriesPersistenceNotExternalStop(t *testing.T) {
	process := newSyntheticRuntimeProcess()
	pause := make(chan struct{})
	process.stopPause = pause
	var released, fail atomic.Bool
	defer func() {
		fail.Store(false)
		if released.CompareAndSwap(false, true) {
			close(pause)
		}
	}()
	s, _, kernelID := fingerprintFixture(t, Options{LaunchRuntime: func(context.Context, RuntimeLaunch) (RuntimeProcess, error) { return process, nil }, BeforeCommit: func() error {
		if fail.Load() {
			return errors.New("synthetic full storage")
		}
		return nil
	}})
	environment := createRuntimeEnvironment(t, s, kernelID, "合成终态落盘失败")
	started := acceptRuntimeTest(t, s, "Runtime.Start", runtimeRequest{EnvironmentID: environment.ID, RequestID: id(), NetworkPolicy: "direct"})
	waitKernel(t, s, started.ID)
	stop := acceptRuntimeTest(t, s, "Runtime.Stop", runtimeRequest{EnvironmentID: environment.ID, RequestID: id()})
	waitRuntimeObservation(t, s, environment.ID, func(session RuntimeSession) bool { return session.PID == 0 && session.State == "stopping" })
	fail.Store(true)
	if released.CompareAndSwap(false, true) {
		close(pause)
	}
	failed := waitKernel(t, s, stop.ID)
	if failed.State != "failed" || failed.Error == nil || failed.Error.Code != "STORAGE_WRITE_FAILED" || !view(t, s).RuntimeSessions[environment.ID].PersistencePending {
		t.Fatal("lost final commit was still shown as accepted/successful")
	}
	p := preview(t, s, "edit", environment.ID)
	wantError(t, call(s, "Preview.Regenerate", map[string]string{"previewId": p.PreviewID}), "PROFILE_BUSY")
	fail.Store(false)
	completed := waitKernel(t, s, stop.ID)
	if completed.State != "completed" || view(t, s).RuntimeSessions[environment.ID].PersistencePending || process.closeCalls.Load() != 0 {
		t.Fatal("storage retry repeated an external stop or lost its real final result")
	}
}

func TestRuntimeReconciliationIOIsOutsideTheServiceMutexAndCloseWaitHasADeadline(t *testing.T) {
	process := newSyntheticRuntimeProcess()
	s, root, kernelID := fingerprintFixture(t, Options{LaunchRuntime: func(context.Context, RuntimeLaunch) (RuntimeProcess, error) { return process, nil }})
	environment := createRuntimeEnvironment(t, s, kernelID, "合成缓慢核对")
	start := acceptRuntimeTest(t, s, "Runtime.Start", runtimeRequest{EnvironmentID: environment.ID, RequestID: id(), NetworkPolicy: "direct"})
	waitKernel(t, s, start.ID)
	saved := view(t, s).RuntimeSessions[environment.ID]
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	restoreSyntheticRuntimeRecord(t, root, saved)
	entered, release := make(chan struct{}), make(chan struct{})
	var calls atomic.Int32
	reopened, err := Open(root, Options{InspectRuntime: func(RuntimeSession) (kernel.ManagedRecovery, error) {
		if calls.Add(1) > 1 {
			close(entered)
			<-release
		}
		return kernel.ManagedRecovery{ProcessState: "exited", DirectoryFree: false}, nil
	}})
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	defer close(release)
	accepted := acceptRuntimeTest(t, reopened, "Runtime.Reconcile", runtimeRequest{EnvironmentID: environment.ID, SessionID: saved.SessionID, RequestID: id()})
	select {
	case <-entered:
	case <-time.After(time.Second):
		t.Fatal("reconcile did not reach the I/O seam")
	}
	query := make(chan Result, 1)
	go func() { query <- call(reopened, "Workspace.Read", map[string]any{}) }()
	select {
	case result := <-query:
		if !result.OK {
			t.Fatal("query blocked/failed during reconciliation")
		}
	case <-time.After(time.Second):
		t.Fatal("reconciliation held the service mutex during filesystem I/O")
	}
	if value[Operation](t, call(reopened, "Operation.Read", map[string]string{"operationId": accepted.ID})).State != "accepted" {
		t.Fatal("slow reconciliation was declared complete prematurely")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	if err := reopened.CloseContext(ctx); err == nil {
		t.Fatal("close claimed completion with an in-flight reconciliation worker")
	}
}

func TestRuntimeTimeoutPrecedenceAndIntentionalCleanupPreserveTheOriginalCause(t *testing.T) {
	combined := errors.Join(context.DeadlineExceeded, &kernel.Problem{Code: "CONTROL_CHANNEL_LOST", Message: "synthetic deadline broke control", Retryable: false})
	if runtimeError(combined).Code != "PROCESS_READY_TIMEOUT" {
		t.Fatal("startup deadline was attributed to cleanup control loss")
	}
	if runtimeError(errors.Join(combined, &kernel.Problem{Code: "KERNEL_INTEGRITY_FAILED", Message: "synthetic integrity loss", Retryable: false})).Code != "KERNEL_INTEGRITY_FAILED" {
		t.Fatal("deadline masked a real integrity failure")
	}
	process := newSyntheticRuntimeProcess()
	process.mu.Lock()
	process.exitCode, process.rootExited, process.closeError = 1, true, errors.New("synthetic remaining children")
	process.mu.Unlock()
	s, _, kernelID := fingerprintFixture(t, Options{LaunchRuntime: func(context.Context, RuntimeLaunch) (RuntimeProcess, error) {
		return process, errors.New("synthetic initial readiness fault")
	}})
	environment := createRuntimeEnvironment(t, s, kernelID, "合成主动清理非崩溃")
	start := acceptRuntimeTest(t, s, "Runtime.Start", runtimeRequest{EnvironmentID: environment.ID, RequestID: id(), NetworkPolicy: "direct"})
	if waitKernel(t, s, start.ID).Error.Code != "PROCESS_START_FAILED" {
		t.Fatal("original launch fault was overwritten")
	}
	process.once.Do(func() { close(process.done) })
	final := waitRuntimeObservation(t, s, environment.ID, func(session RuntimeSession) bool { return session.PID == 0 })
	if final.Error == nil || final.Error.Code != "PROCESS_START_FAILED" {
		t.Fatal("intentional failed-launch cleanup was logged as a browser crash")
	}
}

func TestRuntimeAnOldPendingTerminalMustBeSavedBeforeAnyNewReconciliationIsAccepted(t *testing.T) {
	process := newSyntheticRuntimeProcess()
	var fail, restoreOnFailure atomic.Bool
	entered, release := make(chan struct{}), make(chan struct{})
	released := false
	defer func() {
		if !released {
			close(release)
		}
	}()
	s, _, kernelID := fingerprintFixture(t, Options{LaunchRuntime: func(context.Context, RuntimeLaunch) (RuntimeProcess, error) { return process, nil }, BeforeCommit: func() error {
		if fail.Load() {
			if restoreOnFailure.Swap(false) {
				fail.Store(false)
			}
			return errors.New("synthetic pending write")
		}
		return nil
	}, InspectRuntime: func(RuntimeSession) (kernel.ManagedRecovery, error) {
		close(entered)
		<-release
		return kernel.ManagedRecovery{ProcessState: "exited", DirectoryFree: true, SessionMatches: true, ResourcesExited: true}, nil
	}})
	defer fail.Store(false)
	environment := createRuntimeEnvironment(t, s, kernelID, "合成前序保存与新核对顺序")
	start := acceptRuntimeTest(t, s, "Runtime.Start", runtimeRequest{EnvironmentID: environment.ID, RequestID: id(), NetworkPolicy: "direct"})
	waitKernel(t, s, start.ID)
	saved := view(t, s).RuntimeSessions[environment.ID]
	fail.Store(true)
	process.once.Do(func() { close(process.done) })
	waitRuntimeObservation(t, s, environment.ID, func(session RuntimeSession) bool { return session.PersistencePending })
	restoreOnFailure.Store(true)
	wantError(t, call(s, "Runtime.Reconcile", runtimeRequest{EnvironmentID: environment.ID, SessionID: saved.SessionID, RequestID: id()}), "STORAGE_WRITE_FAILED")
	accepted := acceptRuntimeTest(t, s, "Runtime.Reconcile", runtimeRequest{EnvironmentID: environment.ID, SessionID: saved.SessionID, RequestID: id()})
	select {
	case <-entered:
	case <-time.After(time.Second):
		t.Fatal("reconcile I/O was not entered")
	}
	current := view(t, s).RuntimeSessions[environment.ID]
	if !current.NeedsReconcile || current.PersistencePending {
		t.Fatal("older ready result erased the later reconciliation reservation")
	}
	wantError(t, call(s, "Runtime.Start", runtimeRequest{EnvironmentID: environment.ID, RequestID: id(), NetworkPolicy: "direct"}), "PROFILE_BUSY")
	if value[Operation](t, call(s, "Operation.Read", map[string]string{"operationId": accepted.ID})).State != "accepted" {
		t.Fatal("in-flight reconciliation was changed by an older result")
	}
	close(release)
	released = true
	if waitKernel(t, s, accepted.ID).State != "completed" {
		t.Fatal("accepted reconciliation was abandoned after its older write recovered")
	}
}

func TestRuntimePersistenceRecoveryCannotOverwriteTheImmutableStartupFailure(t *testing.T) {
	process := newSyntheticRuntimeProcess()
	process.closeError, process.exitCode, process.rootExited = errors.New("synthetic still-owned children"), 1, true
	var fail atomic.Bool
	s, _, kernelID := fingerprintFixture(t, Options{LaunchRuntime: func(context.Context, RuntimeLaunch) (RuntimeProcess, error) {
		fail.Store(true)
		return process, errors.New("synthetic original startup fault")
	}, BeforeCommit: func() error {
		if fail.Load() {
			return errors.New("synthetic storage loss")
		}
		return nil
	}})
	defer fail.Store(false)
	defer process.once.Do(func() { close(process.done) })
	environment := createRuntimeEnvironment(t, s, kernelID, "合成不可变启动原因")
	start := acceptRuntimeTest(t, s, "Runtime.Start", runtimeRequest{EnvironmentID: environment.ID, RequestID: id(), NetworkPolicy: "direct"})
	if pending := waitKernel(t, s, start.ID); pending.Error == nil || pending.Error.Code != "STORAGE_WRITE_FAILED" || !pending.PersistencePending {
		t.Fatal("write failure was not exposed as transient")
	}
	process.once.Do(func() { close(process.done) })
	waitRuntimeObservation(t, s, environment.ID, func(session RuntimeSession) bool { return session.PID == 0 })
	fail.Store(false)
	final := view(t, s).RuntimeSessions[environment.ID]
	if final.Error == nil || final.Error.Code != "PROCESS_START_FAILED" || final.PersistencePending {
		t.Fatal("temporary storage error replaced the immutable launch cause after recovery")
	}
	if result := waitKernel(t, s, start.ID); result.Error == nil || result.Error.Code != "PROCESS_START_FAILED" || result.PersistencePending {
		t.Fatal("persisted startup operation kept a transient overlay")
	}
}

func TestRuntimeReopenNeverTreatsAnExitedRootAndFreeDirectoryAsACompleteTreeExit(t *testing.T) {
	process := newSyntheticRuntimeProcess()
	s, root, kernelID := fingerprintFixture(t, Options{LaunchRuntime: func(context.Context, RuntimeLaunch) (RuntimeProcess, error) { return process, nil }})
	environment := createRuntimeEnvironment(t, s, kernelID, "合成根退出而子树未退")
	start := acceptRuntimeTest(t, s, "Runtime.Start", runtimeRequest{EnvironmentID: environment.ID, RequestID: id(), NetworkPolicy: "direct"})
	waitKernel(t, s, start.ID)
	saved := view(t, s).RuntimeSessions[environment.ID]
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	restoreSyntheticRuntimeRecord(t, root, saved)
	reopened, err := Open(root, Options{InspectRuntime: func(RuntimeSession) (kernel.ManagedRecovery, error) {
		return kernel.ManagedRecovery{ProcessState: "exited", DirectoryFree: true, SessionMatches: true, ResourcesExited: false}, nil
	}})
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	actual := view(t, reopened).RuntimeSessions[environment.ID]
	if !actual.NeedsReconcile || actual.CanControl || actual.CanForce {
		t.Fatal("root exit was trusted while the exact owned tree still has resources")
	}
	wantError(t, call(reopened, "Runtime.Start", runtimeRequest{EnvironmentID: environment.ID, RequestID: id(), NetworkPolicy: "direct"}), "PROFILE_BUSY")
}

func TestRuntimeReopenRepairsAStoredTransientOperationOverlayInsteadOfFreezingProgress(t *testing.T) {
	process := newSyntheticRuntimeProcess()
	s, root, kernelID := fingerprintFixture(t, Options{LaunchRuntime: func(context.Context, RuntimeLaunch) (RuntimeProcess, error) { return process, nil }})
	environment := createRuntimeEnvironment(t, s, kernelID, "合成遗留临时操作覆盖")
	start := acceptRuntimeTest(t, s, "Runtime.Start", runtimeRequest{EnvironmentID: environment.ID, RequestID: id(), NetworkPolicy: "direct"})
	waitKernel(t, s, start.ID)
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("sqlite", filepath.Join(root, "app.db"))
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec("UPDATE operations SET result_json=json_set(result_json,'$.state','failed','$.persistencePending',json('true')) WHERE id=?", start.ID)
	_ = db.Close()
	if err != nil {
		t.Fatal(err)
	}
	reopened, err := Open(root, Options{InspectRuntime: func(RuntimeSession) (kernel.ManagedRecovery, error) {
		return kernel.ManagedRecovery{ProcessState: "exited", DirectoryFree: true, SessionMatches: true, ResourcesExited: true}, nil
	}})
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	actual := value[Operation](t, call(reopened, "Operation.Read", map[string]string{"operationId": start.ID}))
	if actual.PersistencePending || actual.Error == nil || actual.Error.Code != "APPLICATION_INTERRUPTED" {
		t.Fatal("stored transient failure stayed progress forever after reopen")
	}
}
