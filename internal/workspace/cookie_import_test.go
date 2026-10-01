package workspace

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/axgiroud312-byte/prism-local-browser/internal/cookies"
	"github.com/axgiroud312-byte/prism-local-browser/internal/kernel"
)

// Host-only synthetic cookie storage, not Chromium or network evidence.
type syntheticCookieProcess struct {
	*syntheticRuntimeProcess
	cookieMu    sync.Mutex
	stored      []cookies.Stored
	fail        map[string]bool
	writes      atomic.Int32
	clears      atomic.Int32
	beforeApply func(context.Context, cookies.Cookie) error
	afterApply  func()
}

func newSyntheticCookieProcess() *syntheticCookieProcess {
	return &syntheticCookieProcess{syntheticRuntimeProcess: newSyntheticRuntimeProcess(), stored: []cookies.Stored{}, fail: map[string]bool{}}
}
func (p *syntheticCookieProcess) ReadCookies(context.Context) ([]cookies.Stored, error) {
	p.cookieMu.Lock()
	defer p.cookieMu.Unlock()
	return append([]cookies.Stored{}, p.stored...), nil
}
func (p *syntheticCookieProcess) ApplyCookie(ctx context.Context, value cookies.Cookie) (cookies.ApplyResult, error) {
	if p.beforeApply != nil {
		if err := p.beforeApply(ctx, value); err != nil {
			return cookies.ApplyResult{}, err
		}
	}
	p.cookieMu.Lock()
	if p.fail[value.Name] {
		p.cookieMu.Unlock()
		return cookies.ApplyResult{}, &cookies.Error{Code: "COOKIE_VERIFY_MISMATCH", Message: "合成读回差异，不是内核证据。", Retryable: true}
	}
	if cookies.Matches(value, p.stored) {
		p.cookieMu.Unlock()
		return cookies.ApplyResult{Status: "already-matched"}, nil
	}
	for index, existing := range p.stored {
		if cookies.Key(existing.Cookie) == cookies.Key(value) {
			p.stored = append(p.stored[:index], p.stored[index+1:]...)
			break
		}
	}
	stored := cookies.Stored{Cookie: value, Session: value.Expires == nil}
	if stored.Session {
		expiry := float64(-1)
		stored.Expires = &expiry
	}
	p.stored = append(p.stored, stored)
	p.writes.Add(1)
	p.cookieMu.Unlock()
	if p.afterApply != nil {
		p.afterApply()
	}
	return cookies.ApplyResult{Status: "verified"}, nil
}
func (p *syntheticCookieProcess) ClearCookies(context.Context) error {
	p.cookieMu.Lock()
	defer p.cookieMu.Unlock()
	p.stored = []cookies.Stored{}
	p.clears.Add(1)
	return nil
}

func cookiePreviewFixture(t *testing.T, s *Service, environmentID, text string) CookiePreview {
	t.Helper()
	return value[CookiePreview](t, call(s, "Cookie.ParseImport", map[string]string{"environmentId": environmentID, "text": text}))
}
func cookieCommitFixture(t *testing.T, s *Service, preview CookiePreview, rows []int, policy string) CookieCommit {
	t.Helper()
	session := view(t, s).RuntimeSessions[preview.EnvironmentID]
	return CookieCommit{PreviewID: preview.PreviewID, EnvironmentID: preview.EnvironmentID, ExpectedRevision: preview.ExpectedRevision, SessionID: session.SessionID, SelectedRows: rows, Policy: policy, RequestID: id()}
}
func acceptCookieFixture(t *testing.T, s *Service, request CookieCommit) Operation {
	t.Helper()
	return value[struct {
		Operation Operation `json:"operation"`
	}](t, call(s, "Cookie.CommitImport", request)).Operation
}
func startCookieFixture(t *testing.T, s *Service, environment Environment) {
	t.Helper()
	start := acceptRuntimeTest(t, s, "Runtime.Start", runtimeRequest{EnvironmentID: environment.ID, RequestID: id(), NetworkPolicy: "direct"})
	if waitKernel(t, s, start.ID).State != "completed" {
		t.Fatal("synthetic cookie session not ready")
	}
}

func TestCookiePreviewIsSafeBoundAndDoesNotAutoStartStoppedEnvironment(t *testing.T) {
	var launches atomic.Int32
	s, _, kernelID := fingerprintFixture(t, Options{LaunchRuntime: func(context.Context, RuntimeLaunch) (RuntimeProcess, error) {
		launches.Add(1)
		return newSyntheticCookieProcess(), nil
	}})
	environment := createRuntimeEnvironment(t, s, kernelID, "合成Cookie预览")
	p := cookiePreviewFixture(t, s, environment.ID, `[{"name":"synthetic-empty","value":"","domain":"example.test","session":true},{"name":"synthetic-secret","value":"SYNTHETIC_PRIVATE_COOKIE_VALUE","domain":"example.test"},{"name":"synthetic-expired","value":"x","domain":"example.test","expires":1},{"name":"synthetic-invalid","value":"SYNTHETIC_PRIVATE_COOKIE_VALUE","domain":"bad/domain"}]`)
	encoded, _ := json.Marshal(p)
	if p.EnvironmentID != environment.ID || p.ExpectedRevision < 1 || !p.RequiresStart || p.ExistingConflictCount != nil || p.ValidCount != 3 || p.ErrorCount != 1 || p.ExpiredCount != 1 || launches.Load() != 0 || strings.Contains(string(encoded), "SYNTHETIC_PRIVATE_COOKIE_VALUE") {
		t.Fatal("preview leaked values, invented conflict count or implicitly launched")
	}
	value[map[string]string](t, call(s, "Cookie.DiscardImport", map[string]string{"previewId": p.PreviewID}))
	request := CookieCommit{PreviewID: p.PreviewID, EnvironmentID: environment.ID, ExpectedRevision: p.ExpectedRevision, SessionID: "synthetic-not-started", SelectedRows: []int{1}, Policy: "merge", RequestID: id()}
	wantError(t, call(s, "Cookie.CommitImport", request), "PREVIEW_EXPIRED")
}

func TestCookieMergePreservesOtherEnvironmentAndRetriesOnlyUnverifiedKeys(t *testing.T) {
	a, b := newSyntheticCookieProcess(), newSyntheticCookieProcess()
	a.fail["synthetic-fail"] = true
	s, _, kernelID := fingerprintFixture(t, Options{LaunchRuntime: func(_ context.Context, launch RuntimeLaunch) (RuntimeProcess, error) {
		if launch.Configuration.Name == "合成CookieA" {
			return a, nil
		}
		return b, nil
	}})
	envA, envB := createRuntimeEnvironment(t, s, kernelID, "合成CookieA"), createRuntimeEnvironment(t, s, kernelID, "合成CookieB")
	startCookieFixture(t, s, envA)
	startCookieFixture(t, s, envB)
	p := cookiePreviewFixture(t, s, envA.ID, `[{"name":"synthetic-empty","value":"","domain":"example.test","httpOnly":true},{"name":"synthetic-fail","value":"SYNTHETIC_PRIVATE_COOKIE_VALUE","domain":"example.test"}]`)
	request := cookieCommitFixture(t, s, p, []int{1, 2}, "merge")
	operation := acceptCookieFixture(t, s, request)
	final := waitKernel(t, s, operation.ID)
	if final.State != "failed" || final.CookieReport.VerifiedCount != 1 || final.CookieReport.FailedCount != 1 || b.writes.Load() != 0 || a.writes.Load() != 1 {
		t.Fatal("partial result or cross-environment side effects were misstated")
	}
	linked := false
	for _, activity := range view(t, s).State.Activities {
		if activity.ID == final.ID {
			linked = activity.Result == "error" && activity.ErrorCode == "COOKIE_VERIFY_MISMATCH" && activity.EnvironmentID == envA.ID && activity.SessionID == final.SessionID
		}
	}
	if !linked {
		t.Fatal("partial Cookie observation was not committed with a correctly linked failure activity")
	}
	if repeat := acceptCookieFixture(t, s, request); repeat.ID != operation.ID || a.writes.Load() != 1 {
		t.Fatal("request retry replayed external Cookie writes")
	}
	a.cookieMu.Lock()
	a.fail["synthetic-fail"] = false
	a.cookieMu.Unlock()
	request.RequestID = id()
	retry := waitKernel(t, s, acceptCookieFixture(t, s, request).ID)
	if retry.State != "completed" || retry.CookieReport.VerifiedCount != 2 || retry.CookieReport.AlreadyMatchedCount != 1 || a.writes.Load() != 2 || a.clears.Load() != 0 {
		t.Fatal("retry rewrote matching/unrelated keys or invented verified count")
	}
	for _, query := range []string{"SELECT result_json FROM operations", "SELECT result_json FROM requests", "SELECT detail FROM activities"} {
		rows, err := s.db.Query(query)
		if err != nil {
			t.Fatal(err)
		}
		for rows.Next() {
			var record string
			if err := rows.Scan(&record); err != nil {
				t.Fatal(err)
			}
			if strings.Contains(record, "SYNTHETIC_PRIVATE_COOKIE_VALUE") {
				t.Fatal("Cookie values entered durable ordinary records")
			}
		}
		rows.Close()
	}
}

func TestCookieCommitRejectsForeignSessionRevisionDuplicateAndRPCOverrides(t *testing.T) {
	process := newSyntheticCookieProcess()
	s, _, kernelID := fingerprintFixture(t, Options{LaunchRuntime: func(context.Context, RuntimeLaunch) (RuntimeProcess, error) { return process, nil }})
	environment := createRuntimeEnvironment(t, s, kernelID, "合成Cookie边界")
	startCookieFixture(t, s, environment)
	p := cookiePreviewFixture(t, s, environment.ID, `[{"name":"synthetic","value":"a","domain":"example.test"},{"name":"synthetic","value":"b","domain":"example.test"}]`)
	request := cookieCommitFixture(t, s, p, []int{1, 2}, "merge")
	wantError(t, call(s, "Cookie.CommitImport", request), "COOKIE_DUPLICATE_SELECTED")
	request.SelectedRows, request.SessionID = []int{1}, "synthetic-foreign-session"
	wantError(t, call(s, "Cookie.CommitImport", request), "COOKIE_SESSION_REQUIRED")
	request.SessionID = view(t, s).RuntimeSessions[environment.ID].SessionID
	request.ExpectedRevision++
	wantError(t, call(s, "Cookie.CommitImport", request), "REVISION_CONFLICT")
	request.ExpectedRevision--
	encoded, _ := json.Marshal(request)
	var attack map[string]any
	_ = json.Unmarshal(encoded, &attack)
	attack["endpoint"] = "http://127.0.0.1:9999"
	attack["cookies"] = []any{}
	wantError(t, call(s, "Cookie.CommitImport", attack), "VALIDATION_FAILED")
	if process.writes.Load() != 0 || process.clears.Load() != 0 {
		t.Fatal("rejected Cookie request still performed side effects")
	}
}

func TestCookieExplicitReplaceClearsOnceAndRetryCannotReclear(t *testing.T) {
	process := newSyntheticCookieProcess()
	process.fail["synthetic-fail"] = true
	s, _, kernelID := fingerprintFixture(t, Options{LaunchRuntime: func(context.Context, RuntimeLaunch) (RuntimeProcess, error) { return process, nil }})
	environment := createRuntimeEnvironment(t, s, kernelID, "合成Cookie明确清空")
	startCookieFixture(t, s, environment)
	p := cookiePreviewFixture(t, s, environment.ID, `[{"name":"synthetic-ok","value":"x","domain":"example.test"},{"name":"synthetic-fail","value":"y","domain":"example.test"}]`)
	request := cookieCommitFixture(t, s, p, []int{1, 2}, "replace-all")
	final := waitKernel(t, s, acceptCookieFixture(t, s, request).ID)
	if final.CookieReport.ClearState != "verified-empty" || final.CookieReport.VerifiedCount != 1 || process.clears.Load() != 1 {
		t.Fatal("explicit clear/partial import result was misstated")
	}
	request.RequestID = id()
	wantError(t, call(s, "Cookie.CommitImport", request), "COOKIE_CLEAR_ALREADY_ATTEMPTED")
	process.cookieMu.Lock()
	process.fail["synthetic-fail"] = false
	process.cookieMu.Unlock()
	request.Policy, request.SelectedRows, request.RequestID = "merge", []int{2}, id()
	final = waitKernel(t, s, acceptCookieFixture(t, s, request).ID)
	if final.State != "completed" || process.clears.Load() != 1 || process.writes.Load() != 2 {
		t.Fatal("failure retry implicitly cleared again or rewrote unrelated row")
	}
}

func TestCookieCancellationPreservesUnknownWriteAndDoesNotSendFollowingRows(t *testing.T) {
	process := newSyntheticCookieProcess()
	entered := make(chan struct{})
	process.beforeApply = func(ctx context.Context, _ cookies.Cookie) error {
		close(entered)
		<-ctx.Done()
		return &cookies.Error{Code: "COOKIE_WRITE_UNCONFIRMED", Message: "合成已发送但读回中断。", Retryable: true}
	}
	s, _, kernelID := fingerprintFixture(t, Options{LaunchRuntime: func(context.Context, RuntimeLaunch) (RuntimeProcess, error) { return process, nil }})
	environment := createRuntimeEnvironment(t, s, kernelID, "合成Cookie取消")
	startCookieFixture(t, s, environment)
	p := cookiePreviewFixture(t, s, environment.ID, `[{"name":"synthetic-one","value":"x","domain":"example.test"},{"name":"synthetic-two","value":"x","domain":"example.test"}]`)
	operation := acceptCookieFixture(t, s, cookieCommitFixture(t, s, p, []int{1, 2}, "merge"))
	<-entered
	value[Operation](t, call(s, "Operation.Cancel", map[string]string{"operationId": operation.ID}))
	final := waitKernel(t, s, operation.ID)
	if final.State != "cancelled" || final.CookieReport.VerifiedCount != 0 || final.CookieReport.UnconfirmedCount != 1 || final.CookieReport.SkippedCount != 1 || final.CookieReport.Items[0].Status != "unknown" || final.CookieReport.Items[1].Status != "cancelled" {
		t.Fatal("cancellation pretended sent write had no effect or sent remaining rows")
	}
}

func TestCookieStoragePendingRetainsReservationAndOnlyRetriesPersistence(t *testing.T) {
	var broken atomic.Bool
	process := newSyntheticCookieProcess()
	process.afterApply = func() { broken.Store(true) }
	s, _, kernelID := fingerprintFixture(t, Options{LaunchRuntime: func(context.Context, RuntimeLaunch) (RuntimeProcess, error) { return process, nil }, BeforeCommit: func() error {
		if broken.Load() {
			return errors.New("synthetic storage outage")
		}
		return nil
	}})
	environment := createRuntimeEnvironment(t, s, kernelID, "合成Cookie待保存")
	startCookieFixture(t, s, environment)
	p := cookiePreviewFixture(t, s, environment.ID, `[{"name":"synthetic","value":"x","domain":"example.test"}]`)
	operation := acceptCookieFixture(t, s, cookieCommitFixture(t, s, p, []int{1}, "merge"))
	pending := waitKernel(t, s, operation.ID)
	if !pending.PersistencePending || pending.CookieReport.VerifiedCount != 1 || process.writes.Load() != 1 {
		t.Fatal("observed external result was lost or replayed on storage failure")
	}
	wantError(t, call(s, "Runtime.Start", runtimeRequest{EnvironmentID: environment.ID, NetworkPolicy: "direct", RequestID: id()}), "PROFILE_BUSY")
	wantError(t, call(s, "Runtime.Reconcile", runtimeRequest{EnvironmentID: environment.ID, SessionID: view(t, s).RuntimeSessions[environment.ID].SessionID, RequestID: id()}), "PROFILE_BUSY")
	s.mu.Lock()
	retained, wiped := s.profileUses[environment.ID], s.cookieTasks[environment.ID].values == nil
	s.mu.Unlock()
	if !retained || !wiped {
		t.Fatal("pending observation lost reservation or retained task secrets")
	}
	broken.Store(false)
	final := value[Operation](t, call(s, "Operation.Read", map[string]string{"operationId": operation.ID}))
	if final.PersistencePending || final.State != "completed" || process.writes.Load() != 1 {
		t.Fatal("query recovery repeated Cookie side effects rather than saving original result")
	}
}

func TestCookieInterruptedRecoveryDoesNotReplayValuesOrClear(t *testing.T) {
	process := newSyntheticCookieProcess()
	s, root, kernelID := fingerprintFixture(t, Options{LaunchRuntime: func(context.Context, RuntimeLaunch) (RuntimeProcess, error) { return process, nil }})
	environment := createRuntimeEnvironment(t, s, kernelID, "合成Cookie中断")
	startCookieFixture(t, s, environment)
	p := cookiePreviewFixture(t, s, environment.ID, `[{"name":"synthetic","value":"SYNTHETIC_PRIVATE_COOKIE_VALUE","domain":"example.test"}]`)
	operation := acceptCookieFixture(t, s, cookieCommitFixture(t, s, p, []int{1}, "merge"))
	waitKernel(t, s, operation.ID)
	if _, err := s.db.Exec("UPDATE operations SET result_json=json_set(result_json,'$.state','accepted','$.cookieReport.items[0].status','pending','$.cookieReport.verifiedCount',0,'$.cookieReport.writtenCount',0) WHERE id=?", operation.ID); err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := Open(root, Options{InspectRuntime: func(RuntimeSession) (kernel.ManagedRecovery, error) {
		return kernel.ManagedRecovery{ProcessState: "exited", DirectoryFree: true, ResourcesExited: true, SessionMatches: true}, nil
	}})
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	saved := value[Operation](t, call(reopened, "Operation.Read", map[string]string{"operationId": operation.ID}))
	if saved.Error.Code != "APPLICATION_INTERRUPTED" || saved.CookieReport.UnconfirmedCount != 1 || saved.CookieReport.VerifiedCount != 0 || process.writes.Load() != 1 || process.clears.Load() != 0 {
		t.Fatal("reopen invented confirmed result or replayed private data")
	}
}

func TestCookieImportPurposeSuppressesSavedTabsWithoutChangingIdentityOrBypassingProxyGate(t *testing.T) {
	launches := make(chan RuntimeLaunch, 1)
	s, _, kernelID := fingerprintFixture(t, Options{LaunchRuntime: func(_ context.Context, input RuntimeLaunch) (RuntimeProcess, error) {
		launches <- input
		return newSyntheticCookieProcess(), nil
	}})
	environment := createRuntimeEnvironment(t, s, kernelID, "合成Cookie空白启动")
	edit := preview(t, s, "edit", environment.ID)
	edit.Environment.URLs = "https://synthetic.example.test"
	edit.Environment.RestoreTabs = true
	value[map[string]any](t, call(s, "Environment.Update", Mutation{PreviewID: edit.PreviewID, Configuration: edit.Environment.Configuration, ExpectedRevision: edit.ExpectedRevision, RequestID: id()}))
	p := cookiePreviewFixture(t, s, environment.ID, `[{"name":"synthetic","value":"","domain":"example.test"}]`)
	before := view(t, s).Fingerprints[environment.ID].Profile.Seed
	start := acceptRuntimeTest(t, s, "Runtime.Start", runtimeRequest{EnvironmentID: environment.ID, RequestID: id(), NetworkPolicy: "direct", Purpose: "cookie-import", ExpectedRevision: p.ExpectedRevision})
	waitKernel(t, s, start.ID)
	launched := <-launches
	if launched.Configuration.URLs != "" || launched.Configuration.RestoreTabs || launched.Profile.Seed != before || view(t, s).State.Environments[0].URLs != "https://synthetic.example.test" {
		t.Fatal("Cookie purpose changed saved identity/preferences or opened old tabs")
	}
	guarded, _, guardedKernel := fingerprintFixture(t, Options{})
	record := importProxyFixture(t, guarded, "localhost:8080")
	bound := createRuntimeEnvironment(t, guarded, guardedKernel, "合成Cookie代理门禁")
	bindRuntimeProxyFixture(t, guarded, bound, record)
	boundPreview := cookiePreviewFixture(t, guarded, bound.ID, `[{"name":"synthetic","value":"","domain":"example.test"}]`)
	guardStart := acceptRuntimeTest(t, guarded, "Runtime.Start", runtimeRequest{EnvironmentID: bound.ID, RequestID: id(), NetworkPolicy: "proxy", Purpose: "cookie-import", ExpectedRevision: boundPreview.ExpectedRevision})
	if final := waitKernel(t, guarded, guardStart.ID); final.Error == nil || final.Error.Code != "NETWORK_PROTECTION_UNAVAILABLE" {
		t.Fatal("Cookie purpose bypassed production network protection gate")
	}
}
