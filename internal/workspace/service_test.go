package workspace

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"
)

func fixture(t *testing.T, options Options) (*Service, string) {
	t.Helper()
	root := t.TempDir()
	s, err := Open(root, options)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s, root
}
func call(s *Service, method string, payload any) Result {
	encoded, _ := json.Marshal(payload)
	return s.Call(Request{Mode: "native", Method: method, Payload: encoded})
}
func value[T any](t *testing.T, result Result) T {
	t.Helper()
	if !result.OK || result.Mode != "native" {
		t.Fatalf("request failed: %+v", result)
	}
	encoded, err := json.Marshal(result.Data)
	if err != nil {
		t.Fatal(err)
	}
	var output T
	if err = json.Unmarshal(encoded, &output); err != nil {
		t.Fatal(err)
	}
	return output
}
func view(t *testing.T, s *Service) View {
	t.Helper()
	return value[View](t, call(s, "Workspace.Read", struct{}{}))
}
func preview(t *testing.T, s *Service, kind, sourceID string) Preview {
	t.Helper()
	return value[Preview](t, call(s, "Environment.Preview", map[string]string{"kind": kind, "sourceId": sourceID}))
}
func create(t *testing.T, s *Service, name string) (Environment, Mutation) {
	t.Helper()
	p := preview(t, s, "create", "")
	p.Environment.Name = name
	input := Mutation{PreviewID: p.PreviewID, Configuration: p.Environment.Configuration, Count: 1, RequestID: id()}
	value[struct {
		Operation Operation `json:"operation"`
	}](t, call(s, "Environment.Create", input))
	return view(t, s).State.Environments[0], input
}
func wantError(t *testing.T, result Result, code string) {
	t.Helper()
	if result.OK || result.Mode != "native" || result.Error == nil || result.Error.Code != code {
		t.Fatalf("wanted %s, got %+v", code, result)
	}
}

func TestInitializeAndReopenEmptyNativeWorkspace(t *testing.T) {
	s, root := fixture(t, Options{})
	first := view(t, s)
	if len(first.State.Environments) != 0 || len(first.State.Proxies) != 0 || len(first.State.Kernels) != 1 || first.State.Kernels[0].Available {
		t.Fatal("native initialized with demo or launchable records")
	}
	s.Close()
	reopened, err := Open(root, Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	if !reflect.DeepEqual(first, view(t, reopened)) {
		t.Fatal("repeated initialization changed native data")
	}
}

func TestCreateEditCloseReopenKeepsIdentityAndPreferences(t *testing.T) {
	s, root := fixture(t, Options{})
	saved, _ := create(t, s, "合成桌面环境")
	p := preview(t, s, "edit", saved.ID)
	config := p.Environment.Configuration
	config.Name = "合成改名"
	config.Group = "验证分组"
	config.Note = "合成备注"
	config.Width = 1440
	config.Height = 900
	config.URLs = "https://example.test/"
	config.RestoreTabs = false
	updated := value[struct {
		Environment struct {
			Record   Environment `json:"record"`
			Revision int64       `json:"revision"`
		} `json:"environment"`
	}](t, call(s, "Environment.Update", Mutation{PreviewID: p.PreviewID, Configuration: config, ExpectedRevision: p.ExpectedRevision, RequestID: id()}))
	if updated.Environment.Record.ID != saved.ID || updated.Environment.Record.Seed != saved.Seed || updated.Environment.Revision != 2 {
		t.Fatal("ordinary edit changed identity or revision")
	}
	expected := updated.Environment.Record
	s.Close()
	reopened, err := Open(root, Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	actual := view(t, reopened).State.Environments[0]
	if !reflect.DeepEqual(expected, actual) {
		t.Fatalf("reopen changed record: %+v", actual)
	}
	if actual.CoreID != PendingKernelID || view(t, reopened).State.Kernels[0].Available {
		t.Fatal("missing kernel became ready")
	}
}

func TestPreviewRegenerateAndDiscardNeverCommit(t *testing.T) {
	s, _ := fixture(t, Options{})
	saved, _ := create(t, s, "固定身份")
	before := view(t, s)
	p := preview(t, s, "edit", saved.ID)
	regenerated := value[Preview](t, call(s, "Preview.Regenerate", map[string]string{"previewId": p.PreviewID}))
	if regenerated.Environment.Seed == saved.Seed {
		t.Fatal("explicit regeneration did not change preview")
	}
	value[map[string]string](t, call(s, "Preview.Discard", map[string]string{"previewId": p.PreviewID}))
	if !reflect.DeepEqual(before, view(t, s)) {
		t.Fatal("discard wrote the preview")
	}
	wantError(t, call(s, "Environment.Update", Mutation{PreviewID: p.PreviewID, Configuration: regenerated.Environment.Configuration, ExpectedRevision: 1, RequestID: id()}), "PREVIEW_EXPIRED")
}

func TestPersistedIdempotenceSurvivesServiceReopen(t *testing.T) {
	s, root := fixture(t, Options{})
	saved, input := create(t, s, "持久幂等")
	first := call(s, "Environment.Create", input)
	s.Close()
	reopened, err := Open(root, Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	repeated := call(reopened, "Environment.Create", input)
	left, _ := json.Marshal(first)
	right, _ := json.Marshal(repeated)
	if !bytes.Equal(left, right) {
		t.Fatalf("idempotent result changed: %s != %s", left, right)
	}
	if records := view(t, reopened).State.Environments; len(records) != 1 || records[0].ID != saved.ID {
		t.Fatal("duplicate creation after reopening")
	}
	input.Configuration.Name = "不相同的请求"
	wantError(t, call(reopened, "Environment.Create", input), "REQUEST_ID_REUSED")
}

func TestStaleRevisionCannotOverwrite(t *testing.T) {
	s, _ := fixture(t, Options{})
	saved, _ := create(t, s, "修订测试")
	first := preview(t, s, "edit", saved.ID)
	stale := preview(t, s, "edit", saved.ID)
	first.Environment.Name = "较新的名称"
	value[map[string]any](t, call(s, "Environment.Update", Mutation{PreviewID: first.PreviewID, Configuration: first.Environment.Configuration, ExpectedRevision: first.ExpectedRevision, RequestID: id()}))
	stale.Environment.Name = "旧草稿覆盖"
	wantError(t, call(s, "Environment.Update", Mutation{PreviewID: stale.PreviewID, Configuration: stale.Environment.Configuration, ExpectedRevision: stale.ExpectedRevision, RequestID: id()}), "REVISION_CONFLICT")
	if view(t, s).State.Environments[0].Name != "较新的名称" {
		t.Fatal("stale draft replaced newer data")
	}
}

func TestTransactionFailureRetainsOldStateAndCanRetrySameRequest(t *testing.T) {
	fail := false
	s, _ := fixture(t, Options{BeforeCommit: func() error {
		if fail {
			return errors.New("SYNTHETIC_SECRET / private-path")
		}
		return nil
	}})
	saved, _ := create(t, s, "事务故障")
	before := view(t, s)
	p := preview(t, s, "edit", saved.ID)
	p.Environment.Name = "重试后保存"
	input := Mutation{PreviewID: p.PreviewID, Configuration: p.Environment.Configuration, ExpectedRevision: p.ExpectedRevision, RequestID: id()}
	fail = true
	result := call(s, "Environment.Update", input)
	wantError(t, result, "STORAGE_WRITE_FAILED")
	text, _ := json.Marshal(result)
	if strings.Contains(string(text), "SYNTHETIC_SECRET") || strings.Contains(string(text), "private-path") {
		t.Fatal("exception leaked")
	}
	if !reflect.DeepEqual(before, view(t, s)) {
		t.Fatal("failed transaction published records or success activity")
	}
	fail = false
	value[map[string]any](t, call(s, "Environment.Update", input))
	if view(t, s).State.Environments[0].Name != "重试后保存" {
		t.Fatal("failed request was incorrectly cached")
	}
}

func TestActualSQLiteFullRollsBackAllRecords(t *testing.T) {
	s, _ := fixture(t, Options{})
	before := view(t, s)
	var pages int
	if err := s.db.QueryRow("PRAGMA page_count").Scan(&pages); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.Exec("PRAGMA max_page_count=" + strconv.Itoa(pages)); err != nil {
		t.Fatal(err)
	}
	p := preview(t, s, "create", "")
	p.Environment.Name = "磁盘不足夹具"
	p.Environment.Note = strings.Repeat("synthetic", 300000)
	result := call(s, "Environment.Create", Mutation{PreviewID: p.PreviewID, Configuration: p.Environment.Configuration, Count: 1, RequestID: id()})
	wantError(t, result, "DISK_FULL")
	if !reflect.DeepEqual(before, view(t, s)) {
		t.Fatal("SQLITE_FULL left partial state")
	}
	for _, table := range []string{"fingerprints", "environments", "requests", "operations", "activities"} {
		var count int
		if err := s.db.QueryRow("SELECT COUNT(*) FROM " + table).Scan(&count); err != nil || count != 0 {
			t.Fatalf("partial %s rows after full: %d %v", table, count, err)
		}
	}
}

func TestReferencesAndForeignKeysAreEnforced(t *testing.T) {
	s, _ := fixture(t, Options{})
	for _, change := range []func(*Configuration){func(c *Configuration) { c.CoreID = "missing-kernel" }, func(c *Configuration) { c.ProxyID = "missing-proxy" }} {
		p := preview(t, s, "create", "")
		p.Environment.Name = "引用验证"
		change(&p.Environment.Configuration)
		wantError(t, call(s, "Environment.Create", Mutation{PreviewID: p.PreviewID, Configuration: p.Environment.Configuration, Count: 1, RequestID: id()}), "VALIDATION_FAILED")
	}
	if _, err := s.db.Exec(`INSERT INTO fingerprints(id,seed,kernel_id,template_version,generator_version,config_json) VALUES('broken',1,'not-present','v1','v1','{}')`); err == nil {
		t.Fatal("SQLite foreign key check is not enabled")
	}
	s.db.SetMaxIdleConns(0)
	if _, err := s.db.Exec(`INSERT INTO fingerprints(id,seed,kernel_id,template_version,generator_version,config_json) VALUES('broken-connection',2,'not-present','v1','v1','{}')`); err == nil {
		t.Fatal("a replacement connection lost foreign key enforcement")
	}
	if len(view(t, s).State.Environments) != 0 {
		t.Fatal("invalid reference created an environment")
	}
}

func TestRejectDemoAndPrototypePayloadsWithoutChangingNativeState(t *testing.T) {
	s, _ := fixture(t, Options{})
	before := view(t, s)
	wantError(t, s.Call(Request{Mode: "demo", Method: "Workspace.Read", Payload: json.RawMessage(`{}`)}), "CAPABILITY_UNSUPPORTED")
	wantError(t, s.Call(Request{Mode: "native", Method: "Environment.Create", Payload: json.RawMessage(`{"format":"prism-prototype","schemaVersion":1,"environments":[]}`)}), "VALIDATION_FAILED")
	wantError(t, call(s, "Backup.ApplyRestore", map[string]string{"format": "prism-prototype"}), "CAPABILITY_UNSUPPORTED")
	if !reflect.DeepEqual(before, view(t, s)) {
		t.Fatal("demo input modified native records")
	}
}

func TestSchemaVersionAndCorruptFilesAreNotOverwritten(t *testing.T) {
	s, root := fixture(t, Options{})
	saved, _ := create(t, s, "未来格式")
	if _, err := s.db.Exec("PRAGMA user_version=999"); err != nil {
		t.Fatal(err)
	}
	s.Close()
	if opened, err := Open(root, Options{}); err == nil {
		opened.Close()
		t.Fatal("unsupported future schema opened")
	}
	// Only the fixture repairs its version; a failed application open must preserve every row.
	db, err := sql.Open("sqlite", filepath.Join(root, "app.db"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec("PRAGMA user_version=5"); err != nil {
		t.Fatal(err)
	}
	db.Close()
	reopened, err := Open(root, Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	if records := view(t, reopened).State.Environments; len(records) != 1 || !reflect.DeepEqual(records[0], saved) {
		t.Fatal("unsupported database was reset")
	}
	corruptRoot := t.TempDir()
	original := []byte("synthetic non-SQLite data")
	if err := os.WriteFile(filepath.Join(corruptRoot, "app.db"), original, 0600); err != nil {
		t.Fatal(err)
	}
	if opened, err := Open(corruptRoot, Options{}); err == nil {
		opened.Close()
		t.Fatal("corrupt file accepted")
	}
	after, err := os.ReadFile(filepath.Join(corruptRoot, "app.db"))
	if err != nil || !bytes.Equal(original, after) {
		t.Fatal("corrupt original overwritten")
	}
}

func TestQueryOnlyDiskWriteFailureCanRetryWithoutPartialCreation(t *testing.T) {
	s, _ := fixture(t, Options{})
	before := view(t, s)
	p := preview(t, s, "create", "")
	p.Environment.Name = "只读故障"
	input := Mutation{PreviewID: p.PreviewID, Configuration: p.Environment.Configuration, Count: 1, RequestID: id()}
	if _, err := s.db.Exec("PRAGMA query_only=1"); err != nil {
		t.Fatal(err)
	}
	wantError(t, call(s, "Environment.Create", input), "STORAGE_WRITE_FAILED")
	if !reflect.DeepEqual(before, view(t, s)) {
		t.Fatal("read-only failure published a creation")
	}
	if _, err := s.db.Exec("PRAGMA query_only=0"); err != nil {
		t.Fatal(err)
	}
	value[map[string]any](t, call(s, "Environment.Create", input))
	if len(view(t, s).State.Environments) != 1 {
		t.Fatal("same request could not retry after write failure")
	}
}

func TestOrdinaryEditingCannotForgeANewSeed(t *testing.T) {
	s, _ := fixture(t, Options{})
	saved, _ := create(t, s, "固定种子保护")
	p := preview(t, s, "edit", saved.ID)
	p.Environment.Seed = "123"
	wantError(t, call(s, "Environment.Update", Mutation{PreviewID: p.PreviewID, Configuration: p.Environment.Configuration, ExpectedRevision: p.ExpectedRevision, RequestID: id()}), "VALIDATION_FAILED")
	if view(t, s).State.Environments[0].Seed != saved.Seed {
		t.Fatal("ordinary input changed the frozen seed")
	}
}

func TestMissingSchemaAndCorruptConfigurationAreNotAutomaticallyRepaired(t *testing.T) {
	s, root := fixture(t, Options{})
	saved, _ := create(t, s, "保留坏配置")
	if _, err := s.db.Exec("UPDATE fingerprints SET config_json='{}'"); err != nil {
		t.Fatal(err)
	}
	wantError(t, call(s, "Workspace.Read", struct{}{}), "STORAGE_READ_FAILED")
	if _, err := s.db.Exec("DROP TABLE activities"); err != nil {
		t.Fatal(err)
	}
	s.Close()
	if reopened, err := Open(root, Options{}); err == nil {
		reopened.Close()
		t.Fatal("missing schema was silently recreated")
	}
	db, err := sql.Open("sqlite", filepath.Join(root, "app.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var recordID, config string
	if err = db.QueryRow("SELECT e.id,f.config_json FROM environments e JOIN fingerprints f ON f.id=e.fingerprint_id").Scan(&recordID, &config); err != nil || recordID != saved.ID || config != "{}" {
		t.Fatal("failed open altered original data")
	}
}
