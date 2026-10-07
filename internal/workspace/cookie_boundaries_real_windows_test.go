//go:build windows

package workspace

import (
	"context"
	"encoding/json"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/axgiroud312-byte/prism-local-browser/internal/cookies"
)

// A scheduling barrier AFTER a real write/readback, not a fake transport.
// It deterministically lets cancellation happen before the next row is sent.
type localCookieBarrier struct {
	RuntimeProcess
	transport cookieTransport
	after     func(context.Context)
	writes    atomic.Int32
}

func (p *localCookieBarrier) ReadCookies(ctx context.Context) ([]cookies.Stored, error) {
	return p.transport.ReadCookies(ctx)
}
func (p *localCookieBarrier) ClearCookies(ctx context.Context) error {
	return p.transport.ClearCookies(ctx)
}
func (p *localCookieBarrier) ApplyCookie(ctx context.Context, value cookies.Cookie) (cookies.ApplyResult, error) {
	result, err := p.transport.ApplyCookie(ctx, value)
	if err == nil && p.writes.Add(1) == 1 {
		p.after(ctx)
	}
	return result, err
}

func localCookieImport(t *testing.T, f *localAcceptanceFixture, environment Environment, text string, rows []int, policy string) Operation {
	t.Helper()
	p := cookiePreviewFixture(t, f.s, environment.ID, text)
	accepted := acceptCookieFixture(t, f.s, cookieCommitFixture(t, f.s, p, rows, policy))
	return waitRuntimeReal(t, f.s, accepted.ID)
}

func TestRealLocalAcceptanceCookiePartitionsExpiryClearAndCancellation(t *testing.T) {
	f := openLocalAcceptanceFixture(t)
	a, b := f.environment(t, "cookie-A", false), f.environment(t, "cookie-B", false)
	original := view(t, f.s).Fingerprints[a.ID].Profile
	reference := view(t, f.s).DataReferences[a.ID]
	f.start(t, a, true)
	f.start(t, b, true)
	if done := localCookieImport(t, f, b, `[{"name":"untouched","value":"SYNTHETIC_OTHER_ENV","domain":"example.test"}]`, []int{1}, "merge"); done.State != "completed" {
		t.Fatal("other environment baseline missing")
	}
	bBaseline := readLocalCookies(t, f.transport(t, b))
	partitioned := `[{"name":"partitioned","value":"SYNTHETIC_NORMAL","domain":"example.test","secure":true,"sameSite":"None"},{"name":"partitioned","value":"SYNTHETIC_PARTITION_FALSE","domain":"example.test","secure":true,"sameSite":"None","partitionKey":{"topLevelSite":"https://store.test","hasCrossSiteAncestor":false}},{"name":"partitioned","value":"SYNTHETIC_PARTITION_TRUE","domain":"example.test","secure":true,"sameSite":"None","partitionKey":{"topLevelSite":"https://store.test","hasCrossSiteAncestor":true}}]`
	imported := localCookieImport(t, f, a, partitioned, []int{1, 2, 3}, "merge")
	if imported.State != "completed" || imported.CookieReport == nil || imported.CookieReport.VerifiedCount != 3 {
		t.Fatal("actual partitioned/default key write-readback mismatch:", imported.Error)
	}
	stored := readLocalCookies(t, f.transport(t, a))
	if len(stored) != 3 {
		t.Fatal("partitioned/default identities did not remain three separate Cookies")
	}
	matched := localCookieImport(t, f, a, partitioned, []int{1, 2, 3}, "merge")
	if matched.State != "completed" || matched.CookieReport.AlreadyMatchedCount != 3 || matched.CookieReport.WrittenCount != 0 {
		t.Fatal("real already-matched import rewrote existing partition identities")
	}
	unsupported := cookiePreviewFixture(t, f.s, a.ID, `[{"name":"opaque","value":"SYNTHETIC_REJECTED","domain":"example.test","secure":true,"partitionKey":{"topLevelSite":"https://store.test"}}]`)
	if unsupported.ErrorCount != 1 || unsupported.Rows[0].ErrorCode != "COOKIE_PARTITION_UNSUPPORTED" {
		t.Fatal("incomplete partition was not rejected without dropping its partition")
	}
	if !sameLocalCookieCollection(stored, readLocalCookies(t, f.transport(t, a))) {
		t.Fatal("unsupported partition preview changed the real browser")
	}
	expired := cookiePreviewFixture(t, f.s, a.ID, `[{"name":"partitioned","value":"SYNTHETIC_EXPIRED","domain":"example.test","secure":true,"sameSite":"None","expires":1}]`)
	if expired.ExpiredCount != 1 {
		t.Fatal("expired Cookie preview was not marked")
	}
	wantError(t, call(f.s, "Cookie.CommitImport", cookieCommitFixture(t, f.s, expired, []int{1}, "merge")), "COOKIE_SELECTION_INVALID")
	if !sameLocalCookieCollection(stored, readLocalCookies(t, f.transport(t, a))) {
		t.Fatal("expired input deleted or renewed the real same-key Cookie")
	}
	conflict := cookiePreviewFixture(t, f.s, a.ID, `[{"name":"partitioned","value":"SYNTHETIC_REPLACEMENT","domain":"example.test","secure":true,"sameSite":"None"}]`)
	if conflict.ExistingConflictCount == nil || *conflict.ExistingConflictCount != 1 {
		t.Fatal("actual existing default-key conflict was not reported")
	}
	changed := acceptCookieFixture(t, f.s, cookieCommitFixture(t, f.s, conflict, []int{1}, "merge"))
	if done := waitRuntimeReal(t, f.s, changed.ID); done.State != "completed" {
		t.Fatal("actual same-key replacement failed")
	}
	if len(readLocalCookies(t, f.transport(t, a))) != 3 {
		t.Fatal("merge of default-key conflict erased partitioned keys")
	}
	replacement := cookiePreviewFixture(t, f.s, a.ID, `[{"name":"replacement","value":"SYNTHETIC_CLEAR_RESULT","domain":"example.test","session":true}]`)
	request := cookieCommitFixture(t, f.s, replacement, []int{1}, "replace-all")
	clear := acceptCookieFixture(t, f.s, request)
	cleared := waitRuntimeReal(t, f.s, clear.ID)
	if cleared.State != "completed" || cleared.CookieReport.ClearState != "verified-empty" || len(readLocalCookies(t, f.transport(t, a))) != 1 {
		t.Fatal("explicit clear did not actually clear default and partitioned Cookies before import")
	}
	if replay := acceptCookieFixture(t, f.s, request); replay.ID != clear.ID {
		t.Fatal("clear request replay created a new import")
	}
	if !sameLocalCookieCollection(bBaseline, readLocalCookies(t, f.transport(t, b))) {
		t.Fatal("clear/merge affected the other actual browser context")
	}

	// Actual first write completes, then host-only scheduling pauses this worker
	// until the real Operation.Cancel request cancels its context.
	f.s.mu.Lock()
	originalProcess := f.s.runtimeSlots[a.ID].process
	firstWritten := make(chan struct{})
	barrier := &localCookieBarrier{RuntimeProcess: originalProcess, transport: originalProcess.(cookieTransport), after: func(ctx context.Context) { close(firstWritten); <-ctx.Done() }}
	f.s.runtimeSlots[a.ID].process = barrier
	f.s.mu.Unlock()
	t.Cleanup(func() {
		f.s.mu.Lock()
		if slot := f.s.runtimeSlots[a.ID]; slot != nil && slot.process == barrier {
			slot.process = originalProcess
		}
		f.s.mu.Unlock()
	})
	cancelPreview := cookiePreviewFixture(t, f.s, a.ID, `[{"name":"first","value":"SYNTHETIC_CANCEL_CONFIRMED","domain":"example.test"},{"name":"never-sent","value":"SYNTHETIC_CANCEL_UNSENT","domain":"example.test"}]`)
	cancelled := acceptCookieFixture(t, f.s, cookieCommitFixture(t, f.s, cancelPreview, []int{1, 2}, "merge"))
	select {
	case <-firstWritten:
	case <-time.After(15 * time.Second):
		t.Fatal("first actual Cookie write did not reach cancellation barrier")
	}
	value[Operation](t, call(f.s, "Operation.Cancel", map[string]string{"operationId": cancelled.ID}))
	cancelled = waitRuntimeReal(t, f.s, cancelled.ID)
	if cancelled.State != "cancelled" || cancelled.CookieReport.VerifiedCount != 1 || cancelled.CookieReport.SkippedCount != 1 || barrier.writes.Load() != 1 {
		t.Fatal("cancellation did not preserve real first write and skip the next row")
	}
	f.s.mu.Lock()
	f.s.runtimeSlots[a.ID].process = originalProcess
	f.s.mu.Unlock()
	for _, value := range readLocalCookies(t, f.transport(t, a)) {
		if value.Name == "never-sent" {
			t.Fatal("cancelled subsequent Cookie was sent to the real browser")
		}
	}
	serialized, err := json.Marshal(view(t, f.s))
	if err != nil || strings.Contains(string(serialized), "SYNTHETIC_CANCEL_CONFIRMED") || strings.Contains(string(serialized), "SYNTHETIC_PARTITION") {
		t.Fatal("Cookie values leaked into an ordinary application response")
	}
	if !sameLocalCookieCollection(bBaseline, readLocalCookies(t, f.transport(t, b))) {
		t.Fatal("cancellation affected the other actual environment")
	}
	assertLocalIdentityUnchanged(t, f.s, a, original, reference)
	f.stop(t, a)
	f.stop(t, b)
	writeLocalAcceptanceEvidence(t, "cookie-boundaries", map[string]any{
		"defaultAndBothPartitionAncestorKeysReadBack": 3,
		"alreadyMatchedWithoutRewrite":                3, "incompletePartitionRejected": true,
		"expiredPreviewRejectedWithoutChangingSameKey":               true,
		"existingConflictReportedAndMergedWithoutRemovingPartitions": true,
		"explicitClearActuallyRemovedAllPartitions":                  true, "clearRequestDeduplicated": true,
		"cancellationVerifiedFirstAndSkippedSecond": true,
		"cancellationBoundary":                      "host-only scheduling barrier after first real write/readback; not an uncontrolled pipe interruption",
		"otherEnvironmentCookiesUnchanged":          true, "ordinaryResponsesExcludeCookieValues": true,
		"savedIdentityAndDataReferenceUnchanged": true,
		"kernelVersion":                          f.record.Version,
	})
}
