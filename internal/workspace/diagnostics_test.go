package workspace

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func diagnosticCall(h *DiagnosticHost, method string, input any) Result {
	payload, _ := json.Marshal(input)
	return h.Call(Request{Mode: "native", Method: method, Payload: payload})
}

func TestDiagnosticsReadsWhitelistWithoutFlushingPendingWrites(t *testing.T) {
	s, _ := fixture(t, Options{})
	env, _ := create(t, s, "SYNTHETIC_PRIVATE_NAME")
	secret := "SYNTHETIC_SECRET_MUST_NOT_EXPORT"
	op := Operation{ID: id(), Kind: "cookie-import", State: "failed", Stage: "finished", Total: 10, CompletedIDs: []string{env.ID}, EnvironmentID: env.ID, Error: &Error{Code: "COOKIE_WRITE_UNCONFIRMED", Message: secret, Details: map[string]any{"path": secret}}, CookieReport: &CookieImportReport{VerifiedCount: 9}}
	raw, _ := json.Marshal(op)
	if _, err := s.db.Exec("INSERT INTO operations(id,result_json) VALUES(?,?)", op.ID, string(raw)); err != nil {
		t.Fatal(err)
	}
	session := map[string]any{"mode": "native", "environmentId": env.ID, "sessionId": secret, "state": "error", "networkPolicy": "proxy", "userDataRef": secret, "nextAction": secret, "proxyReport": map[string]any{"exitIp": secret}, "error": map[string]any{"code": "PROCESS_CRASHED", "message": secret}, "networkFault": map[string]any{"containment": "stopped", "error": map[string]any{"code": "PROXY_UNREACHABLE", "message": secret}}}
	raw, _ = json.Marshal(session)
	if _, err := s.db.Exec("INSERT INTO runtime_sessions(environment_id,record_json) VALUES(?,?)", env.ID, string(raw)); err != nil {
		t.Fatal(err)
	}
	s.mu.Lock()
	s.runtimePending[env.ID] = &runtimePendingWrite{}
	s.mu.Unlock()
	t.Cleanup(func() { s.mu.Lock(); delete(s.runtimePending, env.ID); s.mu.Unlock() })
	var before int
	if err := s.db.QueryRow("SELECT COUNT(*) FROM operations").Scan(&before); err != nil {
		t.Fatal(err)
	}
	r := buildDiagnosticReport(context.Background(), "0.3.0-preview.1", s, nil)
	if r.Workspace.Status != "available" || r.Workspace.Counts["runtimePendingWrites"] != 1 {
		t.Fatalf("incomplete diagnostic: %+v", r.Workspace)
	}
	if r.Workspace.Operations[0].Completed != 9 || r.Workspace.Operations[0].ProgressMetric != "verified-cookies" || r.Workspace.Operations[0].ErrorCode != "COOKIE_WRITE_UNCONFIRMED" {
		t.Fatal("cookie accounting or reason lost")
	}
	if r.Workspace.Sessions[0].ErrorCode != "PROCESS_CRASHED" || r.Workspace.Sessions[0].NetworkErrorCode != "PROXY_UNREACHABLE" {
		t.Fatal("fault identity lost")
	}
	raw, _ = json.Marshal(r)
	for _, private := range []string{secret, env.Name, env.ID, op.ID} {
		if strings.Contains(string(raw), private) {
			t.Fatalf("private field leaked: %q", private)
		}
	}
	var after int
	if err := s.db.QueryRow("SELECT COUNT(*) FROM operations").Scan(&after); err != nil {
		t.Fatal(err)
	}
	if before != after || len(s.runtimePending) != 1 {
		t.Fatal("diagnostic modified business observations")
	}
	batch := diagnosticOperation("batch", Operation{Kind: "batch-create", Total: 20, BatchReport: &BatchReport{Total: 20, CompletedCount: 19}})
	if batch.Completed != 19 || batch.ProgressMetric != "completed-batch-items" {
		t.Fatal("batch count used truncated IDs")
	}
}

func TestDiagnosticsReturnsMinimalOrPartialWithoutBlocking(t *testing.T) {
	s, _ := fixture(t, Options{})
	s.mu.Lock()
	r := buildDiagnosticReport(context.Background(), "invalid-private-version", s, nil)
	s.mu.Unlock()
	if r.Workspace.Status != "unavailable" || r.Workspace.StartupCode != "PROFILE_BUSY" || r.Application.Version != "unknown" {
		t.Fatal("busy service was not bounded")
	}
	if _, err := s.db.Exec("INSERT INTO operations(id,result_json) VALUES(?,?)", id(), "{"); err != nil {
		t.Fatal(err)
	}
	r = buildDiagnosticReport(context.Background(), "0.3.0", s, nil)
	if r.Workspace.Status != "partial" || r.Workspace.OmittedRecords != 1 {
		t.Fatal("corruption reported as complete")
	}
	r = buildDiagnosticReport(context.Background(), "0.3.0", nil, &Error{Code: "SYNTHETIC_SECRET", Message: "SYNTHETIC_SECRET"})
	if r.Workspace.Status != "unavailable" || r.Workspace.StartupCode != "other" || r.Application.Signature != "not-checked" {
		t.Fatal("startup diagnostic inaccurate")
	}
}

func TestDiagnosticExportFreezesBytesAndRetainsAllRequestReceipts(t *testing.T) {
	root, out := t.TempDir(), t.TempDir()
	calls := 0
	h := NewDiagnosticHost(root, "0.3.0", nil, &Error{Code: "STORAGE_READ_FAILED"}, func() (string, error) { calls++; return filepath.Join(out, string(rune('a'+calls))+".json"), nil })
	p := value[DiagnosticPreview](t, diagnosticCall(h, "Diagnostics.Preview", struct{}{}))
	one := DiagnosticExportRequest{ReportID: p.ReportID, RequestID: id()}
	result := diagnosticCall(h, "Diagnostics.Export", one)
	if !result.OK {
		t.Fatalf("save failed: %+v", result.Error)
	}
	content, err := os.ReadFile(filepath.Join(out, "b.json"))
	if err != nil {
		t.Fatal(err)
	}
	expected, _ := json.MarshalIndent(p.Report, "", "  ")
	expected = append(expected, '\n')
	if string(content) != string(expected) || len(content) != p.Bytes {
		t.Fatal("export differs from preview")
	}
	digest := sha256.Sum256(content)
	if hex.EncodeToString(digest[:]) != p.SHA256 {
		t.Fatal("receipt digest differs")
	}
	p2 := value[DiagnosticPreview](t, diagnosticCall(h, "Diagnostics.Preview", struct{}{}))
	if !diagnosticCall(h, "Diagnostics.Export", DiagnosticExportRequest{ReportID: p2.ReportID, RequestID: id()}).OK {
		t.Fatal("second report failed")
	}
	h.expires = time.Now().Add(-time.Minute)
	if !diagnosticCall(h, "Diagnostics.Export", one).OK || calls != 2 {
		t.Fatal("old receipt re-exported after another report")
	}
	reused := diagnosticCall(h, "Diagnostics.Export", DiagnosticExportRequest{ReportID: p2.ReportID, RequestID: one.RequestID})
	if reused.OK || reused.Error.Code != "REQUEST_ID_REUSED" || calls != 2 {
		t.Fatal("old request rebound")
	}
	unknown := diagnosticCall(h, "Diagnostics.Export", DiagnosticExportRequest{ReportID: p2.ReportID, RequestID: id()})
	if unknown.OK || unknown.Error.Code != "PREVIEW_EXPIRED" || calls != 2 {
		t.Fatal("expired report invoked chooser")
	}
}

func TestDiagnosticCancelledAndConcurrentExportsDoNotOpenAnotherChooser(t *testing.T) {
	entered, release := make(chan struct{}), make(chan struct{})
	h := NewDiagnosticHost(t.TempDir(), "0.3.0", nil, nil, func() (string, error) { close(entered); <-release; return "", nil })
	p := value[DiagnosticPreview](t, diagnosticCall(h, "Diagnostics.Preview", struct{}{}))
	input := DiagnosticExportRequest{ReportID: p.ReportID, RequestID: id()}
	done := make(chan Result, 1)
	go func() { done <- diagnosticCall(h, "Diagnostics.Export", input) }()
	released := false
	defer func() {
		if !released {
			close(release)
		}
	}()
	select {
	case <-entered:
	case <-time.After(5 * time.Second):
		t.Fatal("chooser not entered")
	}
	for _, method := range []string{"Diagnostics.Export", "Diagnostics.EndVerification"} {
		r := diagnosticCall(h, method, input)
		if r.OK || r.Error.Code != "DIAGNOSTICS_RESULT_UNCONFIRMED" {
			t.Fatal("concurrent operation escaped busy guard")
		}
	}
	close(release)
	released = true
	select {
	case r := <-done:
		if !r.OK {
			t.Fatal("cancel failed")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("cancel stalled")
	}
	value[map[string]any](t, diagnosticCall(h, "Diagnostics.Export", input)) // must use cached cancellation
}

func TestDiagnosticUnknownPublishedFileCanEndVerificationWithoutReplaying(t *testing.T) {
	h := NewDiagnosticHost(t.TempDir(), "0.3.0", nil, nil, func() (string, error) { t.Fatal("unknown result opened another chooser"); return "", nil })
	p := value[DiagnosticPreview](t, diagnosticCall(h, "Diagnostics.Preview", struct{}{}))
	input := DiagnosticExportRequest{ReportID: p.ReportID, RequestID: id()}
	// Host-only synthetic lost-close outcome; not a claim of real disk failure.
	h.exports[input.RequestID] = &diagnosticExport{input: input, published: true, digest: p.SHA256, destination: filepath.Join(t.TempDir(), "missing.json"), result: failure("DIAGNOSTICS_RESULT_UNCONFIRMED", "unconfirmed", true)}
	if r := diagnosticCall(h, "Diagnostics.Export", input); r.OK {
		t.Fatal("missing file confirmed")
	}
	r := value[map[string]any](t, diagnosticCall(h, "Diagnostics.EndVerification", input))
	if r["status"] != "unconfirmed" {
		t.Fatal("unknown outcome erased")
	}
	value[DiagnosticPreview](t, diagnosticCall(h, "Diagnostics.Preview", struct{}{}))
	if r := diagnosticCall(h, "Diagnostics.Export", input); r.OK {
		t.Fatal("old missing publication claimed saved")
	}
}

func TestDiagnosticRPCRejectsClientPathsAndDemo(t *testing.T) {
	h := NewDiagnosticHost(t.TempDir(), "0.3.0", nil, nil, nil)
	if diagnosticCall(h, "Diagnostics.Preview", map[string]string{"path": "SYNTHETIC_PRIVATE"}).OK {
		t.Fatal("path accepted")
	}
	if h.Call(Request{Mode: "demo", Method: "Diagnostics.Preview", Payload: json.RawMessage(`{}`)}).OK {
		t.Fatal("demo accepted")
	}
	if diagnosticCall(h, "Diagnostics.Export", map[string]string{"reportId": id(), "requestId": id(), "path": "SYNTHETIC_PRIVATE"}).OK {
		t.Fatal("export path accepted")
	}
}

func TestDiagnosticEndVerificationSealsUnseenRequestAgainstLateExport(t *testing.T) {
	calls := 0
	h := NewDiagnosticHost(t.TempDir(), "0.3.0", nil, nil, func() (string, error) { calls++; return "", nil })
	p := value[DiagnosticPreview](t, diagnosticCall(h, "Diagnostics.Preview", struct{}{}))
	input := DiagnosticExportRequest{ReportID: p.ReportID, RequestID: id()}
	result := value[map[string]any](t, diagnosticCall(h, "Diagnostics.EndVerification", input))
	if result["status"] != "unconfirmed" {
		t.Fatal("unseen request asserted not published")
	}
	if r := diagnosticCall(h, "Diagnostics.Export", input); r.OK || r.Error.Code != "DIAGNOSTICS_RESULT_UNCONFIRMED" {
		t.Fatal("late export not sealed")
	}
	input.ReportID = id()
	if r := diagnosticCall(h, "Diagnostics.Export", input); r.OK || r.Error.Code != "REQUEST_ID_REUSED" {
		t.Fatal("sealed request rebound")
	}
	if calls != 0 {
		t.Fatal("ended request invoked chooser")
	}
}
