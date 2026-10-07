package workspace

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/axgiroud312-byte/prism-local-browser/internal/backup"
	"github.com/axgiroud312-byte/prism-local-browser/internal/kernel"
)

func restorePackageFixture(t *testing.T, s *Service, ids []string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "synthetic-restore.prismbackup")
	token := backupDestinationFixture(t, s, path)
	op := waitBackupFixture(t, s, acceptBackupFixture(t, s, BackupExportRequest{Scope: "selected", EnvironmentIDs: ids, DestinationToken: token, StopRunning: true, RequestID: id()}).ID)
	if op.State != "completed" {
		t.Fatal("fixture export failed", op.Error)
	}
	return path
}
func previewPackageFixture(t *testing.T, s *Service, path string) Result {
	t.Helper()
	s.options.ChooseBackupSource = func() (string, error) { return path, nil }
	selected := value[struct {
		Token string `json:"sourceToken"`
	}](t, call(s, "Backup.SelectRestoreSource", struct{}{}))
	return call(s, "Backup.PreviewRestore", map[string]string{"sourceToken": selected.Token})
}
func workspaceBytesFixture(t *testing.T, root string) map[string][32]byte {
	t.Helper()
	result := map[string][32]byte{}
	for _, name := range []string{"app.db", "app.db-wal", "app.db-shm"} {
		data, err := os.ReadFile(filepath.Join(root, name))
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			t.Fatal(err)
		}
		result[name] = sha256.Sum256(data)
	}
	return result
}
func TestRestorePreflightPreservesDatabaseAndBrowserBytesAndShowsOriginalIdentity(t *testing.T) {
	s, root := fixture(t, Options{})
	e, _ := create(t, s, "合成恢复预检")
	ref, _ := dataReference(e.ID)
	profile := filepath.Join(root, filepath.FromSlash(ref))
	os.MkdirAll(profile, 0700)
	os.WriteFile(filepath.Join(profile, "Cookies"), []byte("SYNTHETIC_PRIVATE_BYTES"), 0600)
	path := restorePackageFixture(t, s, []string{e.ID})
	before := workspaceBytesFixture(t, root)
	p := value[RestorePreview](t, previewPackageFixture(t, s, path))
	if p.AddCount != 0 || p.OverwriteCount != 1 || !p.CanRestore || p.ArchiveSHA256 == "" {
		t.Fatal("preflight impact incorrect", p)
	}
	page := value[struct {
		Items []RestoreEnvironment `json:"items"`
	}](t, call(s, "Backup.ReadRestorePage", map[string]any{"previewId": p.PreviewID, "offset": 0, "pageSize": 25}))
	if len(page.Items) != 1 || page.Items[0].Seed != e.Seed || page.Items[0].ID != e.ID || page.Items[0].DataState != "present" {
		t.Fatal("identity or existing data was replaced by clone semantics")
	}
	for name, hash := range before {
		data, err := os.ReadFile(filepath.Join(root, name))
		if err != nil || sha256.Sum256(data) != hash {
			t.Fatal("preflight modified current database/sidecar", name)
		}
	}
	data, _ := os.ReadFile(filepath.Join(profile, "Cookies"))
	if string(data) != "SYNTHETIC_PRIVATE_BYTES" {
		t.Fatal("preflight changed browser data")
	}
	encoded, _ := json.Marshal(p)
	if strings.Contains(string(encoded), root) || strings.Contains(string(encoded), "SYNTHETIC_PRIVATE_BYTES") {
		t.Fatal("preflight leaked private path/content")
	}
}
func TestRestorePreflightRejectsPrototypeWithoutInitializingData(t *testing.T) {
	s, root := fixture(t, Options{})
	create(t, s, "合成未初始化")
	path := filepath.Join(t.TempDir(), "bad.prismbackup")
	os.WriteFile(path, []byte(`{"format":"prism-prototype","schemaVersion":1}`), 0600)
	before := workspaceBytesFixture(t, root)
	wantError(t, previewPackageFixture(t, s, path), "BACKUP_INVALID")
	for name, hash := range before {
		data, _ := os.ReadFile(filepath.Join(root, name))
		if sha256.Sum256(data) != hash {
			t.Fatal("bad preflight wrote current database")
		}
	}
	if _, err := os.Stat(filepath.Join(root, "environments")); !os.IsNotExist(err) {
		t.Fatal("preflight created a browsing directory")
	}
}
func TestRestorePreflightCannotFlushPendingBackupWrites(t *testing.T) {
	s, _ := fixture(t, Options{})
	e, _ := create(t, s, "合成只读边界")
	path := restorePackageFixture(t, s, []string{e.ID})
	// This pending observation would call BeforeCommit if the
	// preview accidentally entered the generic Backup.* persistence dispatch.
	writes := 0
	s.options.BeforeCommit = func() error { writes++; return nil }
	var text string
	if err := s.db.QueryRow("SELECT result_json FROM operations WHERE json_extract(result_json,'$.kind')='backup-export'").Scan(&text); err != nil {
		t.Fatal(err)
	}
	var operation Operation
	if err := json.Unmarshal([]byte(text), &operation); err != nil {
		t.Fatal(err)
	}
	task := &backupTask{operation: operation, finalPending: true, final: operation}
	s.backupTasks[task.operation.ID] = task
	value[RestorePreview](t, previewPackageFixture(t, s, path))
	if writes != 0 || !task.finalPending {
		t.Fatal("read-only preflight flushed mutable tasks")
	}
	delete(s.backupTasks, task.operation.ID)
}
func TestRestoreConfigurationRejectsExtraSchemaWithoutOpeningCurrentWorkspace(t *testing.T) {
	s, _ := fixture(t, Options{})
	e, _ := create(t, s, "合成恶意schema")
	path := restorePackageFixture(t, s, []string{e.ID})
	m, entries := readPackageFixture(t, path)
	config := filepath.Join(t.TempDir(), "configuration.sqlite")
	os.WriteFile(config, entries["configuration.sqlite"], 0600)
	db, err := sql.Open("sqlite", sqliteFileURI(config, "rw"))
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec("CREATE VIEW injected AS SELECT * FROM environments")
	db.Close()
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.readRestoreConfiguration(context.Background(), config, m); err == nil {
		t.Fatal("untrusted schema accepted")
	}
}
func TestRestorePreviewReportsUnavailableDPAPIWithoutExposingOrReplacingCredentials(t *testing.T) {
	s, _ := fixture(t, Options{})
	e, _ := create(t, s, "SYNTHETIC credential restore preview")
	p := importProxyFixture(t, s, "http://SYNTHETIC_USER:SYNTHETIC_REENTRY_PASSWORD@localhost:8080")
	bindRuntimeProxyFixture(t, s, e, p)
	path := restorePackageFixture(t, s, []string{e.ID})
	available := value[RestorePreview](t, previewPackageFixture(t, s, path))
	if len(available.Credentials) != 1 || available.Credentials[0].State != "available-current-user" || available.CredentialReentryCount != 0 {
		t.Fatal("same-user real DPAPI backup was not available")
	}
	value[map[string]any](t, call(s, "Backup.DiscardRestore", map[string]string{"previewId": available.PreviewID}))
	var ref string
	var original []byte
	if err := s.db.QueryRow("SELECT credential_ref FROM proxies WHERE id=?", p.ID).Scan(&ref); err != nil {
		t.Fatal(err)
	}
	if err := s.db.QueryRow("SELECT protected FROM proxy_credentials WHERE ref=?", ref).Scan(&original); err != nil {
		t.Fatal(err)
	}
	s.options.UnprotectProxySecret = func(string, []byte) ([]byte, error) { return nil, errors.New("synthetic DPAPI unavailable") }
	unavailable := value[RestorePreview](t, previewPackageFixture(t, s, path))
	if len(unavailable.Credentials) != 1 || unavailable.Credentials[0].State != "reentry-required" || unavailable.CredentialReentryCount != 1 {
		t.Fatal("unreadable credentials were silently cleared or reported usable")
	}
	encoded, _ := json.Marshal(unavailable)
	if strings.Contains(string(encoded), "SYNTHETIC_USER") || strings.Contains(string(encoded), "SYNTHETIC_REENTRY_PASSWORD") {
		t.Fatal("credential preview exposed authentication material")
	}
	var after []byte
	if err := s.db.QueryRow("SELECT protected FROM proxy_credentials WHERE ref=?", ref).Scan(&after); err != nil || string(after) != string(original) {
		t.Fatal("read-only preview replaced protected credential bytes", err)
	}
}

func TestRestoreKernelCandidatesMatchExactBuildInsteadOfSourceID(t *testing.T) {
	s, _, kernelID := fingerprintFixture(t, Options{})
	r, err := s.savedKernel(kernelID)
	if err != nil {
		t.Fatal(err)
	}
	alias := "synthetic-backup-kernel-alias"
	for _, wrongDigest := range []bool{false, true} {
		source := r
		if wrongDigest {
			source.ExecutableSHA256 = strings.Repeat("f", 64)
		}
		d := &restoreDraft{manifest: backup.Manifest{Kernels: []backup.Kernel{{ID: alias, State: "exact", Version: source.Version, Architecture: source.Architecture, ArchiveSHA256: source.ArchiveSHA256, ExecutableSHA256: source.ExecutableSHA256}}}, data: restoreData{kernels: map[string]kernel.Record{alias: source}}, kernelMapping: map[string]string{}, kernelCandidates: map[string][]string{}}
		if _, err := s.planRestoreImpact(d); err != nil {
			t.Fatal(err)
		}
		candidates := d.kernelCandidates[alias]
		if wrongDigest && len(candidates) != 0 || !wrongDigest && (len(candidates) != 1 || candidates[0] != kernelID) {
			t.Fatal("restore mapped by ID/version without exact digests, or rejected an equivalent build alias")
		}
	}
}

func TestRestoreReadPageDoesNotAcceptAnotherPreviewOrPath(t *testing.T) {
	s, _ := fixture(t, Options{})
	e, _ := create(t, s, "合成预览绑定")
	p := value[RestorePreview](t, previewPackageFixture(t, s, restorePackageFixture(t, s, []string{e.ID})))
	wantError(t, call(s, "Backup.ReadRestorePage", map[string]any{"previewId": id(), "offset": 0, "pageSize": 25}), "PREVIEW_EXPIRED")
	wantError(t, call(s, "Backup.PreviewRestore", map[string]string{"sourceToken": "synthetic", "path": "C:/outside"}), "VALIDATION_FAILED")
	value[map[string]string](t, call(s, "Backup.DiscardRestore", map[string]string{"previewId": p.PreviewID, "sourceToken": ""}))
	wantError(t, call(s, "Backup.ReadRestorePage", map[string]any{"previewId": p.PreviewID, "offset": 0, "pageSize": 25}), "PREVIEW_EXPIRED")
}

func TestRestorePreviewCleansOwnedScratchOnSuccessAndFailure(t *testing.T) {
	s, root := fixture(t, Options{})
	e, _ := create(t, s, "合成暂存清理")
	value[RestorePreview](t, previewPackageFixture(t, s, restorePackageFixture(t, s, []string{e.ID})))
	entries, err := os.ReadDir(filepath.Join(root, "backups", "preflight"))
	if err != nil || len(entries) != 0 {
		t.Fatal("completed preflight retained configuration payload", err)
	}
	path := filepath.Join(t.TempDir(), "synthetic-bad-db.prismbackup")
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	w := backup.NewWriter(file)
	payload := strings.Repeat("SYNTHETIC_NOT_SQLITE", 1024)
	if err = w.Add(context.Background(), "configuration.sqlite", strings.NewReader(payload), int64(len(payload)), false); err != nil {
		t.Fatal(err)
	}
	_, err = w.Finish(context.Background(), backup.Manifest{Format: backup.Format, SchemaVersion: 1, WorkspaceSchema: 7, AppVersion: "synthetic", BackupID: id(), CreatedAt: timestamp(), Scope: "all", Configuration: "configuration.sqlite", Credentials: "windows-current-user-dpapi", BrowserData: "sensitive-same-user-not-portable", Environments: []backup.Environment{}, Kernels: []backup.Kernel{}})
	file.Close()
	if err != nil {
		t.Fatal(err)
	}
	if previewPackageFixture(t, s, path).OK {
		t.Fatal("non-SQLite payload accepted")
	}
	entries, err = os.ReadDir(filepath.Join(root, "backups", "preflight"))
	if err != nil || len(entries) != 0 {
		t.Fatal("failed preflight retained payload", err)
	}
}

func TestRestorePreviewChecksNamesAfterAllSelectedOverwrites(t *testing.T) {
	s, _ := fixture(t, Options{})
	a, _ := create(t, s, "合成原名称甲")
	b, _ := create(t, s, "合成原名称乙")
	path := restorePackageFixture(t, s, []string{a.ID, b.ID})
	for _, change := range []struct{ id, name string }{{a.ID, "合成临时名称丙"}, {b.ID, a.Name}} {
		p := preview(t, s, "edit", change.id)
		p.Environment.Name = change.name
		value[map[string]any](t, call(s, "Environment.Update", Mutation{PreviewID: p.PreviewID, Configuration: p.Environment.Configuration, ExpectedRevision: p.ExpectedRevision, RequestID: id()}))
	}
	p := value[RestorePreview](t, previewPackageFixture(t, s, path))
	if p.ConflictCount != 0 || !p.CanRestore {
		t.Fatal("restorable final name set reported as external conflict")
	}
}

func TestRestoreConfigurationRejectsCurrentPointerBehindHistory(t *testing.T) {
	s, _ := fixture(t, Options{})
	e, _ := create(t, s, "合成修订倒退")
	path := restorePackageFixture(t, s, []string{e.ID})
	manifest, entries := readPackageFixture(t, path)
	config := filepath.Join(t.TempDir(), "configuration.sqlite")
	os.WriteFile(config, entries["configuration.sqlite"], 0600)
	db, err := sql.Open("sqlite", sqliteFileURI(config, "rw"))
	if err != nil {
		t.Fatal(err)
	}
	r, err := readProfileFrom(db, manifest.Environments[0].FingerprintID)
	if err != nil {
		t.Fatal(err)
	}
	r.Profile.ConfigRevision = 2
	r.Profile.ConfigHash = profileHash(r.Profile)
	text, _ := json.Marshal(r.Profile)
	_, err = db.Exec("INSERT INTO fingerprint_revisions(fingerprint_id,revision,kernel_id,profile_json,created_at,action) VALUES(?,?,?,?,?,?)", manifest.Environments[0].FingerprintID, 2, r.Profile.KernelID, string(text), timestamp(), "regenerate")
	db.Close()
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.readRestoreConfiguration(context.Background(), config, manifest); err == nil {
		t.Fatal("current profile behind immutable history accepted")
	}
}
