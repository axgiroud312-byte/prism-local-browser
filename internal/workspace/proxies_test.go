package workspace

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/axgiroud312-byte/prism-local-browser/internal/proxy"
)

func importProxyFixture(t *testing.T, s *Service, text string) ProxyView {
	t.Helper()
	p := value[ProxyImportPreview](t, call(s, "Proxy.ParseImport", map[string]string{"text": text}))
	lines := []int{}
	for _, row := range p.Rows {
		if row.Error == "" {
			lines = append(lines, row.Line)
		}
	}
	value[struct {
		ImportedIDs []string `json:"importedIds"`
	}](t, call(s, "Proxy.CommitImport", ProxyCommitImport{PreviewID: p.PreviewID, SelectedRows: lines, RequestID: id()}))
	return view(t, s).NativeProxyRecords[0]
}
func stripProxySchema(t *testing.T, s *Service) {
	t.Helper()
	for _, statement := range []string{"DROP TABLE proxy_config", "DROP TABLE proxy_credentials", "DROP TABLE proxy_request_key"} {
		if _, err := s.db.Exec(statement); err != nil {
			t.Fatal(err)
		}
	}
}
func syntheticProxyCheck(_ context.Context, _ proxy.Configuration, _ *proxy.Credentials, progress func(proxy.Step)) proxy.Report {
	progress(proxy.Step{Stage: "exit", Status: "passed", Time: timestamp(), Message: "仅合成监督器seam，不是网络证据。"})
	return proxy.Report{Mode: "native", AdapterVersion: "synthetic-host-only", StartedAt: timestamp(), FinishedAt: timestamp(), Steps: []proxy.Step{}, ExitIP: "203.0.113.45"}
}
func acceptProxyCheck(t *testing.T, s *Service, record ProxyView) Operation {
	t.Helper()
	return value[struct {
		Operation Operation `json:"operation"`
	}](t, call(s, "Proxy.Check", ProxyTarget{ProxyID: record.ID, ExpectedRevision: record.Revision, RequestID: id()})).Operation
}
func TestNativeProxyImportPreviewRedactsAndSelectsOnlyExplicitValidRows(t *testing.T) {
	s, _ := fixture(t, Options{})
	marker := "SYNTHETIC_SAVED_PROXY_PASSWORD"
	p := value[ProxyImportPreview](t, call(s, "Proxy.ParseImport", map[string]string{"text": "# synthetic\nhttp://synthetic-user:" + marker + "@localhost:8080\nlocalhost:8080\nhttp://synthetic-user:" + marker + "@localhost:0\nsocks5://[::1]:1080"}))
	encoded, _ := json.Marshal(p)
	if strings.Contains(string(encoded), marker) || strings.Contains(string(encoded), "synthetic-user") || len(p.Rows) != 4 || p.Rows[0].DuplicateCount != 1 || p.Rows[2].Error == "" {
		t.Fatal("preview leaked authentication or lost error/duplicate lines")
	}
	wantError(t, call(s, "Proxy.CommitImport", ProxyCommitImport{PreviewID: p.PreviewID, SelectedRows: []int{2, 4}, RequestID: id()}), "VALIDATION_FAILED")
	request := ProxyCommitImport{PreviewID: p.PreviewID, SelectedRows: []int{2, 5}, RequestID: id()}
	first := call(s, "Proxy.CommitImport", request)
	firstJSON, _ := json.Marshal(first)
	repeatedJSON, _ := json.Marshal(call(s, "Proxy.CommitImport", request))
	if !first.OK || string(firstJSON) != string(repeatedJSON) {
		t.Fatal("import retry duplicated nodes or changed response")
	}
	request.SelectedRows = []int{3}
	wantError(t, call(s, "Proxy.CommitImport", request), "REQUEST_ID_REUSED")
	records := view(t, s).NativeProxyRecords
	if len(records) != 2 {
		t.Fatal("unselected or error row was silently imported")
	}
	for _, statement := range []string{"SELECT config_json FROM proxy_config", "SELECT result_json FROM requests", "SELECT detail FROM activities"} {
		rows, err := s.db.Query(statement)
		if err != nil {
			t.Fatal(err)
		}
		for rows.Next() {
			var value string
			rows.Scan(&value)
			if strings.Contains(value, marker) || strings.Contains(value, "synthetic-user") {
				t.Fatal("raw authentication entered ordinary durable records")
			}
		}
		rows.Close()
	}
}
func TestNativeProxyDuplicateGroupsRemainLinearForLargeRepeatedEndpoints(t *testing.T) {
	s, _ := fixture(t, Options{})
	p := value[ProxyImportPreview](t, call(s, "Proxy.ParseImport", map[string]string{"text": strings.Repeat("http://localhost:8080\n", 10000)}))
	encoded, _ := json.Marshal(p)
	if len(p.Rows) != 10000 || len(p.DuplicateGroups) != 1 || p.Rows[0].DuplicateCount != 9999 || len(encoded) > 5<<20 {
		t.Fatal("duplicate relations expanded quadratically or rows were capped")
	}
	for _, group := range p.DuplicateGroups {
		if len(group.Lines) != 10000 {
			t.Fatal("shared duplicate group lost candidates")
		}
	}
}
func TestNativeProxyUpdateKeepsProtectedCredentialsInvalidatesCheckAndDoesNotChangeSeed(t *testing.T) {
	s, root := fixture(t, Options{CheckProxy: syntheticProxyCheck})
	record := importProxyFixture(t, s, "http://synthetic-user:SYNTHETIC_KEEP_PASSWORD@localhost:8080")
	e, _ := create(t, s, "合成代理绑定环境")
	edit := preview(t, s, "edit", e.ID)
	config := edit.Environment.Configuration
	config.ProxyID = record.ID
	value[struct{}](t, call(s, "Environment.Update", Mutation{PreviewID: edit.PreviewID, Configuration: config, ExpectedRevision: edit.ExpectedRevision, RequestID: id()}))
	if waitKernel(t, s, acceptProxyCheck(t, s, record).ID).State != "completed" {
		t.Fatal("synthetic check did not finish")
	}
	configProxy := record.Configuration
	configProxy.Name = "合成改名不回填空密码"
	result := value[struct {
		Record ProxyView `json:"record"`
	}](t, call(s, "Proxy.Update", ProxyUpdate{ProxyID: record.ID, ExpectedRevision: record.Revision, Configuration: configProxy, Credentials: ProxyCredentialChange{Action: "keep"}, RequestID: id()})).Record
	if !result.HasAuthentication || result.CheckReport != nil || result.Status != "unchecked" || result.Revision != 2 {
		t.Fatal("keep erased credentials or reused a stale check")
	}
	wantError(t, call(s, "Proxy.Delete", ProxyTarget{ProxyID: record.ID, ExpectedRevision: result.Revision, RequestID: id()}), "PROXY_IN_USE")
	wantError(t, call(s, "Proxy.Update", ProxyUpdate{ProxyID: record.ID, ExpectedRevision: 1, Configuration: configProxy, Credentials: ProxyCredentialChange{Action: "clear"}, RequestID: id()}), "REVISION_CONFLICT")
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := Open(root, Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	reopenedView := view(t, reopened)
	if reopenedView.State.Environments[0].Seed != e.Seed || !reopenedView.NativeProxyRecords[0].HasAuthentication {
		t.Fatal("reopen changed identity or protected auth existence")
	}
	_, ref, err := reopened.savedProxy(record.ID)
	if err != nil {
		t.Fatal(err)
	}
	credentials, err := reopened.storedProxyCredentials(ref)
	if err != nil || credentials.Username != "synthetic-user" || credentials.Password != "SYNTHETIC_KEEP_PASSWORD" {
		t.Fatal("same-user stored authentication did not survive reopen")
	}
}
func TestNativeProxyCredentialFailureAndCommitFailureLeaveOldRecordsUntouched(t *testing.T) {
	var fail atomic.Bool
	s, _ := fixture(t, Options{BeforeCommit: func() error {
		if fail.Load() {
			return errors.New("synthetic storage loss")
		}
		return nil
	}})
	record := importProxyFixture(t, s, "http://synthetic-user:SYNTHETIC_OLD_SECRET@localhost:8080")
	update := ProxyUpdate{ProxyID: record.ID, ExpectedRevision: 1, Configuration: record.Configuration, Credentials: ProxyCredentialChange{Action: "replace", Username: "synthetic-new", Password: "SYNTHETIC_NEW_SECRET"}, RequestID: id()}
	fail.Store(true)
	wantError(t, call(s, "Proxy.Update", update), "STORAGE_WRITE_FAILED")
	fail.Store(false)
	current, ref, err := s.savedProxy(record.ID)
	if err != nil || current.Revision != 1 {
		t.Fatal("failed update altered the configuration")
	}
	credentials, err := s.storedProxyCredentials(ref)
	if err != nil || credentials.Password != "SYNTHETIC_OLD_SECRET" {
		t.Fatal("failed credential replacement destroyed old auth")
	}
	s.options.ProtectProxySecret = func(string, []byte) ([]byte, error) { return nil, errors.New("synthetic DPAPI denied") }
	wantError(t, call(s, "Proxy.Update", update), "CREDENTIALS_UNAVAILABLE")
	if current := view(t, s).NativeProxyRecords[0]; current.Revision != 1 || !current.HasAuthentication {
		t.Fatal("protection failure used a plaintext/no-auth fallback")
	}
}
func TestNativeProxyPendingResultRetriesStorageNotNetworkAndFailureActivityIsError(t *testing.T) {
	var fail atomic.Bool
	var networks atomic.Int32
	s, _ := fixture(t, Options{BeforeCommit: func() error {
		if fail.Load() {
			return errors.New("synthetic final write failed")
		}
		return nil
	}, CheckProxy: func(context.Context, proxy.Configuration, *proxy.Credentials, func(proxy.Step)) proxy.Report {
		networks.Add(1)
		fail.Store(true)
		return proxy.Report{Mode: "native", AdapterVersion: "synthetic-host-only", StartedAt: timestamp(), FinishedAt: timestamp(), Steps: []proxy.Step{}, Error: &proxy.CheckError{Code: "PROXY_AUTH_FAILED", Message: "合成认证拒绝，不是实际网络证据。", Retryable: false}}
	}})
	defer fail.Store(false)
	record := importProxyFixture(t, s, "localhost:8080")
	operation := acceptProxyCheck(t, s, record)
	pending := waitKernel(t, s, operation.ID)
	if !pending.PersistencePending || pending.Error == nil || pending.Error.Code != "STORAGE_WRITE_FAILED" {
		t.Fatal("lost final write was hidden as completed/accepted")
	}
	wantError(t, call(s, "Proxy.Update", ProxyUpdate{ProxyID: record.ID, ExpectedRevision: 1, Configuration: record.Configuration, Credentials: ProxyCredentialChange{Action: "keep"}, RequestID: id()}), "PROFILE_BUSY")
	fail.Store(false)
	final := waitKernel(t, s, operation.ID)
	if final.PersistencePending || final.Error.Code != "PROXY_AUTH_FAILED" || networks.Load() != 1 {
		t.Fatal("storage retry repeated network or lost its true result")
	}
	activities := view(t, s).State.Activities
	found := false
	for _, activity := range activities {
		if activity.ID == operation.ID {
			found = true
			if activity.Result != "error" || activity.ErrorCode != "PROXY_AUTH_FAILED" {
				t.Fatal("failed check activity projected success")
			}
		}
	}
	if !found {
		t.Fatal("check activity missing")
	}
}
func TestNativeProxyCancelledChecksDoNotBlockReadOrLeaveBusyAfterShutdown(t *testing.T) {
	entered := make(chan struct{})
	s, root := fixture(t, Options{CheckProxy: func(ctx context.Context, _ proxy.Configuration, _ *proxy.Credentials, _ func(proxy.Step)) proxy.Report {
		close(entered)
		<-ctx.Done()
		return proxy.Report{Mode: "native", AdapterVersion: "synthetic-host-only", StartedAt: timestamp(), FinishedAt: timestamp(), Steps: []proxy.Step{}, Error: &proxy.CheckError{Code: "OPERATION_CANCELLED", Message: "合成网络等待已取消。", Retryable: true}}
	}})
	record := importProxyFixture(t, s, "localhost:8080")
	operation := acceptProxyCheck(t, s, record)
	select {
	case <-entered:
	case <-time.After(time.Second):
		t.Fatal("worker did not enter host seam")
	}
	query := make(chan Result, 1)
	go func() { query <- call(s, "Workspace.Read", struct{}{}) }()
	select {
	case result := <-query:
		if !result.OK {
			t.Fatal("query failed during check")
		}
	case <-time.After(time.Second):
		t.Fatal("network held service mutex")
	}
	value[Operation](t, call(s, "Operation.Cancel", map[string]string{"operationId": operation.ID}))
	if waitKernel(t, s, operation.ID).State != "cancelled" {
		t.Fatal("cancel did not preserve true check result")
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := Open(root, Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	if current := value[Operation](t, call(reopened, "Operation.Read", map[string]string{"operationId": operation.ID})); current.State != "cancelled" {
		t.Fatal("cancel result did not survive reopen")
	}
}
func TestNativeProxyV4MigrationAndInterruptedCheckRecoveryPreserveEnvironmentIdentity(t *testing.T) {
	s, root := fixture(t, Options{})
	e, _ := create(t, s, "合成schema4原环境")
	stripProxySchema(t, s)
	if _, err := s.db.Exec("PRAGMA user_version=4"); err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := Open(root, Options{})
	if err != nil {
		t.Fatal(err)
	}
	var version int
	if err := reopened.db.QueryRow("PRAGMA user_version").Scan(&version); err != nil || version != 5 || view(t, reopened).State.Environments[0].Seed != e.Seed {
		t.Fatal("schema4 migration lost fixed identity")
	}
	record := importProxyFixture(t, reopened, "localhost:8080")
	if err := reopened.Close(); err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("sqlite", filepath.Join(root, "app.db"))
	if err != nil {
		t.Fatal(err)
	}
	interrupted := Operation{ID: id(), Kind: "proxy-check", State: "accepted", Stage: "queued", ProxyID: record.ID, Total: 1, CompletedIDs: []string{}}
	encoded, _ := json.Marshal(interrupted)
	_, err = db.Exec("INSERT INTO operations(id,result_json) VALUES(?,?)", interrupted.ID, string(encoded))
	db.Close()
	if err != nil {
		t.Fatal(err)
	}
	var networks atomic.Int32
	again, err := Open(root, Options{CheckProxy: func(context.Context, proxy.Configuration, *proxy.Credentials, func(proxy.Step)) proxy.Report {
		networks.Add(1)
		return proxy.Report{}
	}})
	if err != nil {
		t.Fatal(err)
	}
	defer again.Close()
	actual := value[Operation](t, call(again, "Operation.Read", map[string]string{"operationId": interrupted.ID}))
	if actual.State != "failed" || actual.Error.Code != "APPLICATION_INTERRUPTED" || networks.Load() != 0 {
		t.Fatal("recovery resumed a stale check or reported success")
	}
}
