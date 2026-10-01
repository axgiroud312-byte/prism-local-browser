package workspace

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/axgiroud312-byte/prism-local-browser/internal/backup"
	"golang.org/x/sys/windows"
)

func backupDestinationFixture(t *testing.T, s *Service, path string) string {
	t.Helper()
	s.mu.Lock()
	s.options.ChooseBackupDestination = func() (string, error) { return path, nil }
	s.mu.Unlock()
	return value[struct {
		Token string `json:"destinationToken"`
	}](t, call(s, "Backup.SelectDestination", struct{}{})).Token
}
func waitBackupFixture(t *testing.T, s *Service, operationID string) Operation {
	t.Helper()
	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		operation := value[Operation](t, call(s, "Operation.Read", map[string]string{"operationId": operationID}))
		if !operation.PersistencePending && (operation.State == "completed" || operation.State == "failed" || operation.State == "cancelled") {
			return operation
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("backup did not reach durable terminal state")
	return Operation{}
}
func acceptBackupFixture(t *testing.T, s *Service, input BackupExportRequest) Operation {
	t.Helper()
	return value[struct {
		Operation Operation `json:"operation"`
	}](t, call(s, "Backup.Export", input)).Operation
}
func readPackageFixture(t *testing.T, path string) (backup.Manifest, map[string][]byte) {
	t.Helper()
	file, err := zip.OpenReader(path)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	entries := map[string][]byte{}
	for _, entry := range file.File {
		input, err := entry.Open()
		if err != nil {
			t.Fatal(err)
		}
		data, err := io.ReadAll(input)
		input.Close()
		if err != nil {
			t.Fatal(err)
		}
		entries[entry.Name] = data
	}
	var manifest backup.Manifest
	if err = json.Unmarshal(entries["manifest.json"], &manifest); err != nil {
		t.Fatal(err)
	}
	return manifest, entries
}

func TestBackupSelectedReadsRealDataAndScopedSQLiteWithoutChangingSourceIdentity(t *testing.T) {
	s, root := fixture(t, Options{})
	selected, _ := create(t, s, "合成备份选择A")
	unrelated, _ := create(t, s, "合成不选B")
	ref, _ := dataReference(selected.ID)
	directory := filepath.Join(root, filepath.FromSlash(ref))
	os.MkdirAll(filepath.Join(directory, "empty"), 0700)
	const secret = "SYNTHETIC_COOKIE_FILE_BYTES"
	os.WriteFile(filepath.Join(directory, "Cookies"), []byte(secret), 0600)
	destination := filepath.Join(t.TempDir(), "synthetic.prismbackup")
	token := backupDestinationFixture(t, s, destination)
	input := BackupExportRequest{Scope: "selected", EnvironmentIDs: []string{selected.ID}, DestinationToken: token, StopRunning: true, RequestID: id()}
	first := acceptBackupFixture(t, s, input)
	final := waitBackupFixture(t, s, first.ID)
	if final.State != "completed" || !final.BackupReport.Published || len(final.CompletedIDs) != 0 {
		t.Fatal("complete publication not confirmed")
	}
	manifest, entries := readPackageFixture(t, destination)
	if manifest.Format != backup.Format || len(manifest.Environments) != 1 || manifest.Environments[0].ID != selected.ID || manifest.Environments[0].Seed != selected.Seed || manifest.KernelBinariesIncluded || string(entries[ref+"/Cookies"]) != secret {
		t.Fatal("manifest/data/identity mismatch")
	}
	if _, ok := entries[ref+"/empty/"]; !ok {
		t.Fatal("empty directory missing")
	}
	path := filepath.Join(t.TempDir(), "configuration.sqlite")
	os.WriteFile(path, entries["configuration.sqlite"], 0600)
	copied, err := sql.Open("sqlite", sqliteFileURI(path, "ro"))
	if err != nil {
		t.Fatal(err)
	}
	defer copied.Close()
	var count int
	copied.QueryRow("SELECT COUNT(*) FROM environments WHERE id=?", unrelated.ID).Scan(&count)
	if count != 0 {
		t.Fatal("selected backup carried unrelated records")
	}
	for _, table := range []string{"operations", "requests", "activities", "runtime_sessions", "batch_items", "backup_exports"} {
		if err = copied.QueryRow("SELECT COUNT(*) FROM " + table).Scan(&count); err != nil || count != 0 {
			t.Fatal("unsafe operational history survived snapshot pruning", table, err)
		}
	}
	data, _ := os.ReadFile(filepath.Join(directory, "Cookies"))
	if string(data) != secret {
		t.Fatal("source browser data changed")
	}
	projection, _ := json.Marshal(final)
	if strings.Contains(string(projection), secret) || strings.Contains(string(projection), root) || strings.Contains(string(projection), destination) {
		t.Fatal("private bytes/paths leaked through ordinary operation")
	}
	duplicate := acceptBackupFixture(t, s, input)
	if duplicate.ID != first.ID {
		t.Fatal("same request exported another backup")
	}
}

func TestBackupAllScopeDoesNotUseCurrentPagedViewAndKeepsUninitializedExplicit(t *testing.T) {
	s, root := fixture(t, Options{})
	for index := 0; index < 11; index++ {
		create(t, s, "合成全量"+string(rune('A'+index)))
	}
	destination := filepath.Join(t.TempDir(), "synthetic-all.prismbackup")
	token := backupDestinationFixture(t, s, destination)
	operation := waitBackupFixture(t, s, acceptBackupFixture(t, s, BackupExportRequest{Scope: "all", EnvironmentIDs: []string{}, DestinationToken: token, StopRunning: true, RequestID: id()}).ID)
	manifest, _ := readPackageFixture(t, destination)
	if operation.State != "completed" || len(manifest.Environments) != 11 || len(view(t, s).State.Environments) != 8 {
		t.Fatal("full backup silently used only the current UI page")
	}
	for _, entry := range manifest.Environments {
		if entry.DataState != "never-initialized" {
			t.Fatal("uninitialized source incorrectly represented")
		}
	}
	if _, err := os.Lstat(filepath.Join(root, "environments")); !os.IsNotExist(err) {
		t.Fatal("backup initialized source browsing directories")
	}
}

func TestBackupCanPublishToWorkspaceAncestorAfterSourceValidation(t *testing.T) {
	parent := t.TempDir()
	root := filepath.Join(parent, "workspace")
	s, err := Open(root, Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	e, dir := recycleFixture(t, s, root, "合成父目录备份")
	path := filepath.Join(parent, "synthetic-ancestor.prismbackup")
	token := backupDestinationFixture(t, s, path)
	op := waitBackupFixture(t, s, acceptBackupFixture(t, s, BackupExportRequest{Scope: "selected", EnvironmentIDs: []string{e.ID}, DestinationToken: token, StopRunning: true, RequestID: id()}).ID)
	if op.State != "completed" || !op.BackupReport.Published {
		t.Fatal("valid ancestor destination could not publish", op)
	}
	manifest, entries := readPackageFixture(t, path)
	ref, _ := dataReference(e.ID)
	if len(manifest.Environments) != 1 || string(entries[ref+"/Cookies"]) != "SYNTHETIC_RECYCLE_"+e.ID {
		t.Fatal("ancestor export lost source bytes")
	}
	if data, err := os.ReadFile(filepath.Join(dir, "Cookies")); err != nil || string(data) != string(entries[ref+"/Cookies"]) {
		t.Fatal("source changed during ancestor publication", err)
	}
}

func TestBackupMaintenanceReservationSurvivesRuntimeReleaseAndRejectsReconcile(t *testing.T) {
	s, _ := fixture(t, Options{})
	environment, _ := create(t, s, "合成备份维护")
	s.mu.Lock()
	task := &backupTask{targets: []backupTarget{{ID: environment.ID}}}
	s.backupUses[environment.ID] = task
	s.profileUses[environment.ID] = true
	s.releaseProfileUse(environment.ID)
	kept := s.profileUses[environment.ID]
	s.mu.Unlock()
	if !kept {
		t.Fatal("runtime release discarded independent backup owner")
	}
	wantError(t, call(s, "Runtime.Reconcile", runtimeRequest{EnvironmentID: environment.ID, SessionID: id(), RequestID: id()}), "PROFILE_BUSY")
	s.mu.Lock()
	s.releaseBackupUses(task)
	s.mu.Unlock()
}

func TestBackupCancellationOnlyCancelsOwnQueuedTaskAndNeverPublishes(t *testing.T) {
	s, _ := fixture(t, Options{})
	environment, _ := create(t, s, "合成备份取消")
	s.backupGate <- struct{}{}
	destination := filepath.Join(t.TempDir(), "synthetic-cancel.prismbackup")
	token := backupDestinationFixture(t, s, destination)
	operation := acceptBackupFixture(t, s, BackupExportRequest{Scope: "selected", EnvironmentIDs: []string{environment.ID}, DestinationToken: token, StopRunning: true, RequestID: id()})
	value[Operation](t, call(s, "Operation.Cancel", map[string]string{"operationId": operation.ID}))
	<-s.backupGate
	final := waitBackupFixture(t, s, operation.ID)
	if final.State != "cancelled" || final.BackupReport.Published {
		t.Fatal("cancellation created complete backup")
	}
	if _, err := os.Lstat(destination); !os.IsNotExist(err) {
		t.Fatal("cancelled queued task published an output")
	}
}

func TestBackupUnknownSelectedOrPendingSessionBlocksBeforeCopying(t *testing.T) {
	s, _ := fixture(t, Options{})
	environment, _ := create(t, s, "合成待核对备份")
	destination := filepath.Join(t.TempDir(), "synthetic-pending.prismbackup")
	token := backupDestinationFixture(t, s, destination)
	input := BackupExportRequest{Scope: "selected", EnvironmentIDs: []string{"missing-selected"}, DestinationToken: token, StopRunning: true, RequestID: id()}
	wantError(t, call(s, "Backup.Export", input), "NOT_FOUND")
	s.mu.Lock()
	s.runtimeSlots[environment.ID] = &runtimeSlot{session: RuntimeSession{Mode: "native", EnvironmentID: environment.ID, NeedsReconcile: true}, cancel: func() {}}
	s.mu.Unlock()
	input.EnvironmentIDs = []string{environment.ID}
	input.RequestID = id()
	wantError(t, call(s, "Backup.Export", input), "SESSION_IDENTITY_UNCONFIRMED")
	if _, err := os.Lstat(destination); !os.IsNotExist(err) {
		t.Fatal("pending session was copied/published")
	}
}

func TestBackupReopenInterruptsAcceptedJournalWithoutCopyingOrCreatingProfile(t *testing.T) {
	s, root := fixture(t, Options{})
	environment, _ := create(t, s, "合成备份重开")
	operationID := id()
	destination, temporary, _ := backup.OutputPaths(root, filepath.Join(t.TempDir(), "synthetic-reopen.prismbackup"), operationID)
	operation := Operation{ID: operationID, Kind: "backup-export", State: "accepted", Stage: "accepted", Total: 1, CompletedIDs: []string{}, BackupReport: backupReport("selected", filepath.Base(destination), 1)}
	operation.BackupReport.RequestID = id()
	encoded, _ := json.Marshal(operation)
	targets, _ := json.Marshal([]backupTarget{{ID: environment.ID, Reference: "environments/" + environment.ID + "/user-data", NeverUsed: true}})
	s.db.Exec("INSERT INTO operations(id,result_json) VALUES(?,?)", operationID, string(encoded))
	s.db.Exec("INSERT INTO backup_exports(operation_id,destination,temporary,targets_json,created_at,phase) VALUES(?,?,?,?,?,?)", operationID, destination, temporary, string(targets), timestamp(), "accepted")
	s.Close()
	reopened, err := Open(root, Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	final := value[Operation](t, call(reopened, "Operation.Read", map[string]string{"operationId": operationID}))
	if final.State != "failed" || final.Error.Code != "APPLICATION_INTERRUPTED" {
		t.Fatal("reopened task auto replayed")
	}
	if _, err := os.Lstat(destination); !os.IsNotExist(err) {
		t.Fatal("reopen published old task")
	}
	if _, err := os.Lstat(filepath.Join(root, "environments")); !os.IsNotExist(err) {
		t.Fatal("reopen created missing browser data")
	}
}

func TestBackupSavedScopeCannotBeReusedWithOtherSelectedTargets(t *testing.T) {
	s, _ := fixture(t, Options{})
	environment, _ := create(t, s, "合成请求备份")
	destination := filepath.Join(t.TempDir(), "synthetic-request.prismbackup")
	token := backupDestinationFixture(t, s, destination)
	input := BackupExportRequest{Scope: "selected", EnvironmentIDs: []string{environment.ID}, DestinationToken: token, StopRunning: true, RequestID: id()}
	operation := acceptBackupFixture(t, s, input)
	waitBackupFixture(t, s, operation.ID)
	input.Scope, input.EnvironmentIDs = "all", []string{}
	wantError(t, call(s, "Backup.Export", input), "REQUEST_ID_REUSED")
}

func TestBackupOutputRPCRejectsClientPathsAndLeavesSourceConfiguration(t *testing.T) {
	s, _ := fixture(t, Options{})
	before := view(t, s)
	wantError(t, call(s, "Backup.SelectDestination", map[string]string{"path": "C:/SYNTHETIC_NOT_AUTHORIZED.prismbackup"}), "VALIDATION_FAILED")
	wantError(t, call(s, "Backup.Export", map[string]any{"scope": "all", "environmentIds": []string{}, "destinationToken": "missing", "stopRunning": true, "requestId": id(), "path": "C:/SYNTHETIC_NOT_AUTHORIZED"}), "VALIDATION_FAILED")
	if len(view(t, s).State.Environments) != len(before.State.Environments) {
		t.Fatal("invalid backup changed workspace")
	}
}

func TestBackupCopiesOriginalProtectedCredentialReferenceWithoutDecrypting(t *testing.T) {
	var decryptions atomic.Int32
	s, _ := fixture(t, Options{ProtectProxySecret: func(ref string, plain []byte) ([]byte, error) {
		digest := sha256.Sum256(append([]byte("SYNTHETIC_HOST_ONLY:"+ref), plain...))
		return digest[:], nil
	}, UnprotectProxySecret: func(string, []byte) ([]byte, error) {
		decryptions.Add(1)
		return nil, errors.New("synthetic export must not decrypt")
	}})
	record := importProxyFixture(t, s, "http://SYNTHETIC_USER:SYNTHETIC_PASSWORD@localhost:8080")
	draft := preview(t, s, "create", "")
	draft.Environment.Name, draft.Environment.ProxyID = "合成密文备份", record.ID
	value[map[string]any](t, call(s, "Environment.Create", Mutation{PreviewID: draft.PreviewID, Configuration: draft.Environment.Configuration, Count: 1, RequestID: id()}))
	environment := view(t, s).State.Environments[0]
	var ref string
	var original []byte
	if err := s.db.QueryRow("SELECT p.credential_ref,c.protected FROM proxies p JOIN proxy_credentials c ON c.ref=p.credential_ref WHERE p.id=?", record.ID).Scan(&ref, &original); err != nil {
		t.Fatal(err)
	}
	destination := filepath.Join(t.TempDir(), "synthetic-protected.prismbackup")
	token := backupDestinationFixture(t, s, destination)
	operation := waitBackupFixture(t, s, acceptBackupFixture(t, s, BackupExportRequest{Scope: "selected", EnvironmentIDs: []string{environment.ID}, DestinationToken: token, StopRunning: true, RequestID: id()}).ID)
	if operation.State != "completed" || decryptions.Load() != 0 {
		t.Fatal("export decrypted/rebuilt protected credentials")
	}
	_, entries := readPackageFixture(t, destination)
	if bytes.Contains(entries["configuration.sqlite"], []byte("SYNTHETIC_PASSWORD")) {
		t.Fatal("plain proxy password entered package")
	}
	path := filepath.Join(t.TempDir(), "config.sqlite")
	os.WriteFile(path, entries["configuration.sqlite"], 0600)
	copied, err := sql.Open("sqlite", sqliteFileURI(path, "ro"))
	if err != nil {
		t.Fatal(err)
	}
	defer copied.Close()
	var copiedRef string
	var protected []byte
	if err = copied.QueryRow("SELECT p.credential_ref,c.protected FROM proxies p JOIN proxy_credentials c ON c.ref=p.credential_ref WHERE p.id=?", record.ID).Scan(&copiedRef, &protected); err != nil || copiedRef != ref || !bytes.Equal(protected, original) {
		t.Fatal("DPAPI entropy reference/ciphertext changed", err)
	}
}

func TestBackupPublishedObservationRetryDoesNotRepublishOrRewritePackage(t *testing.T) {
	var blocked atomic.Bool
	s, root := fixture(t, Options{BeforeCommit: func() error {
		if blocked.Load() {
			return errors.New("synthetic storage unavailable")
		}
		return nil
	}})
	operationID := id()
	destination, temporary, _ := backup.OutputPaths(root, filepath.Join(t.TempDir(), "synthetic-pending.prismbackup"), operationID)
	const original = "SYNTHETIC_PUBLISHED_PACKAGE"
	os.WriteFile(destination, []byte(original), 0600)
	operation := Operation{ID: operationID, Kind: "backup-export", State: "completed", Stage: "completed", CompletedIDs: []string{}, BackupReport: backupReport("all", filepath.Base(destination), 0)}
	operation.BackupReport.RequestID = id()
	operation.BackupReport.Published, operation.BackupReport.ArchiveSHA256, operation.BackupReport.ManifestSHA256 = true, strings.Repeat("a", 64), strings.Repeat("b", 64)
	encoded, _ := json.Marshal(operation)
	s.db.Exec("INSERT INTO operations(id,result_json) VALUES(?,?)", operationID, string(encoded))
	s.db.Exec("INSERT INTO backup_exports(operation_id,destination,temporary,targets_json,created_at,phase) VALUES(?,?,?,?,?,?)", operationID, destination, temporary, "[]", timestamp(), "publishing")
	task := &backupTask{operation: operation, final: operation, finalPending: true, destination: destination, temporary: temporary}
	s.mu.Lock()
	s.backupTasks[operationID] = task
	blocked.Store(true)
	_ = s.flushOneBackupFinal(task)
	s.mu.Unlock()
	pending := value[Operation](t, call(s, "Operation.Read", map[string]string{"operationId": operationID}))
	if !pending.PersistencePending || !pending.BackupReport.Published {
		t.Fatal("storage error erased published fact")
	}
	blocked.Store(false)
	saved := value[Operation](t, call(s, "Operation.Read", map[string]string{"operationId": operationID}))
	if saved.State != "completed" || saved.PersistencePending {
		t.Fatal("saving observation did not converge")
	}
	data, _ := os.ReadFile(destination)
	if string(data) != original {
		t.Fatal("result persistence recopied or republished the package")
	}
}

func TestBackupUnknownAcceptanceDoesNotRepairCorruptOperationOrStartWorker(t *testing.T) {
	s, root := fixture(t, Options{})
	environment, _ := create(t, s, "合成未知备份受理")
	operationID, requestID := id(), id()
	destination, temporary, _ := backup.OutputPaths(root, filepath.Join(t.TempDir(), "synthetic-unconfirmed.prismbackup"), operationID)
	operation := Operation{ID: operationID, Kind: "backup-export", State: "accepted", Stage: "accepted", Total: 1, CompletedIDs: []string{}, BackupReport: backupReport("selected", filepath.Base(destination), 1)}
	operation.BackupReport.RequestID = requestID
	targets := []backupTarget{{ID: environment.ID, Reference: "environments/" + environment.ID + "/user-data", NeverUsed: true}}
	task := &backupTask{operation: operation, targets: targets, destination: destination, temporary: temporary, createdAt: timestamp(), requestID: requestID, signature: "synthetic-signature", acceptancePending: true}
	result := success(map[string]any{"status": "accepted", "operation": operation}, operation.ID)
	resultJSON, _ := json.Marshal(result)
	targetsJSON, _ := json.Marshal(targets)
	corrupt := operation
	corrupt.Kind = "create"
	opJSON, _ := json.Marshal(corrupt)
	s.db.Exec("INSERT INTO operations(id,result_json) VALUES(?,?)", operationID, string(opJSON))
	s.db.Exec("INSERT INTO backup_exports(operation_id,destination,temporary,targets_json,created_at,phase) VALUES(?,?,?,?,?,?)", operationID, destination, temporary, string(targetsJSON), task.createdAt, "accepted")
	s.db.Exec("INSERT INTO requests(id,signature,result_json) VALUES(?,?,?)", requestID, task.signature, string(resultJSON))
	s.mu.Lock()
	s.backupTasks[operationID] = task
	s.backupUses[environment.ID] = task
	s.profileUses[environment.ID] = true
	err := s.confirmBackupAcceptance(task)
	kept := s.backupUses[environment.ID] == task && !task.started && task.acceptancePending
	s.mu.Unlock()
	if err == nil || !kept {
		t.Fatal("unknown acceptance masked damaged operation and started side effects")
	}
	var persisted string
	s.db.QueryRow("SELECT result_json FROM operations WHERE id=?", operationID).Scan(&persisted)
	if persisted != string(opJSON) {
		t.Fatal("confirmation silently repaired a conflicting journal")
	}
	if _, err = os.Lstat(destination); !os.IsNotExist(err) {
		t.Fatal("unconfirmed operation published output")
	}
}

func TestBackupRefusesRelatedQueuedAndUnknownAcceptedBatchTargets(t *testing.T) {
	s, _ := fixture(t, Options{PrepareBatchDirectory: syntheticBatchPrepare})
	environment, _ := create(t, s, "合成排队批次源")
	s.batchGate <- struct{}{}
	page := value[BatchPage](t, call(s, "Batch.Preview", BatchPreviewRequest{Kind: "clone", SourceIDs: []string{environment.ID}}))
	operation := acceptBatchFixture(t, s, page, id())
	destination := filepath.Join(t.TempDir(), "synthetic-batch-busy.prismbackup")
	token := backupDestinationFixture(t, s, destination)
	input := BackupExportRequest{Scope: "selected", EnvironmentIDs: []string{environment.ID}, DestinationToken: token, StopRunning: true, RequestID: id()}
	wantError(t, call(s, "Backup.Export", input), "PROFILE_BUSY")
	value[Operation](t, call(s, "Operation.Cancel", map[string]string{"operationId": operation.ID}))
	<-s.batchGate
	waitBatchFixture(t, s, operation.ID)
	previewPage := value[BatchPage](t, call(s, "Batch.Preview", BatchPreviewRequest{Kind: "assign", Mappings: []BatchMapping{{EnvironmentID: environment.ID, ProxyID: ""}}}))
	s.mu.Lock()
	s.batchAcceptances["synthetic-unknown"] = &batchAcceptance{requestID: "synthetic-unknown", planID: previewPage.PlanID}
	busy, err := s.backupBatchBusy(environment.ID)
	delete(s.batchAcceptances, "synthetic-unknown")
	s.mu.Unlock()
	if err != nil || !busy {
		t.Fatal("unknown accepted batch was omitted by backup busy check", err)
	}
}

func TestBackupReservationRejectsLaterCookieWritesBeforeDraftOrSessionLookup(t *testing.T) {
	s, _ := fixture(t, Options{})
	environment, _ := create(t, s, "合成Cookie备份屏障")
	task := &backupTask{targets: []backupTarget{{ID: environment.ID}}}
	s.mu.Lock()
	s.backupUses[environment.ID] = task
	s.profileUses[environment.ID] = true
	s.mu.Unlock()
	wantError(t, call(s, "Cookie.CommitImport", CookieCommit{EnvironmentID: environment.ID, SessionID: id(), ExpectedRevision: 1, PreviewID: id(), SelectedRows: []int{1}, Policy: "merge", RequestID: id()}), "PROFILE_BUSY")
	s.mu.Lock()
	s.releaseBackupUses(task)
	s.mu.Unlock()
}

func TestSchemaSixToSevenPreservesOriginalIdentityAndBrowserBytes(t *testing.T) {
	s, root := fixture(t, Options{})
	environment, _ := create(t, s, "合成备份迁移")
	before := view(t, s).Fingerprints[environment.ID]
	ref, _ := dataReference(environment.ID)
	directory := filepath.Join(root, filepath.FromSlash(ref))
	os.MkdirAll(directory, 0700)
	path := filepath.Join(directory, "Cookies")
	os.WriteFile(path, []byte("SYNTHETIC_MIGRATION_DATA"), 0600)
	if _, err := s.db.Exec("DROP TABLE restore_jobs; DROP TABLE backup_exports"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.Exec("DROP TABLE environment_data_state"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.Exec("PRAGMA user_version=6"); err != nil {
		t.Fatal(err)
	}
	s.Close()
	reopened, err := Open(root, Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	var version int
	if err = reopened.db.QueryRow("PRAGMA user_version").Scan(&version); err != nil || version != 8 {
		t.Fatal("backup journal migration failed", err)
	}
	after := view(t, reopened).Fingerprints[environment.ID]
	if before.Profile.ConfigHash != after.Profile.ConfigHash || before.Profile.Seed != after.Profile.Seed {
		t.Fatal("backup migration changed fixed identity")
	}
	data, _ := os.ReadFile(path)
	if string(data) != "SYNTHETIC_MIGRATION_DATA" {
		t.Fatal("migration changed browser data")
	}
}

func TestAcceptedStartWithoutDataInitializationDoesNotTurnMissingDataIntoLoss(t *testing.T) {
	s, _ := fixture(t, Options{})
	environment, _ := create(t, s, "合成未初始化启动")
	// A start rejected before the launcher (such as the proxy guard) still keeps
	// its accepted session. That history is not a directory-initialization fact.
	session := RuntimeSession{Mode: "native", EnvironmentID: environment.ID, SessionID: id(), State: "error", NeedsReconcile: false}
	encoded, _ := json.Marshal(session)
	if _, err := s.db.Exec("INSERT INTO runtime_sessions(environment_id,record_json) VALUES(?,?)", environment.ID, string(encoded)); err != nil {
		t.Fatal(err)
	}
	destination := filepath.Join(t.TempDir(), "synthetic-never-initialized.prismbackup")
	token := backupDestinationFixture(t, s, destination)
	final := waitBackupFixture(t, s, acceptBackupFixture(t, s, BackupExportRequest{Scope: "selected", EnvironmentIDs: []string{environment.ID}, DestinationToken: token, StopRunning: true, RequestID: id()}).ID)
	if final.State != "completed" {
		t.Fatal("accepted-but-never-initialized history prevented backup", final.Error)
	}
	manifest, _ := readPackageFixture(t, destination)
	if manifest.Environments[0].DataState != "never-initialized" {
		t.Fatal("absent source was not identified explicitly")
	}
}

func TestDataInitializationFactIsMonotonicAndMissingClaimedDataCannotBecomeEmpty(t *testing.T) {
	s, _ := fixture(t, Options{})
	environment, _ := create(t, s, "合成已预约目录缺失")
	if _, err := s.db.Exec("UPDATE environment_data_state SET state=? WHERE environment_id=?", dataRuntimeClaimed, environment.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.Exec("UPDATE environment_data_state SET state=? WHERE environment_id=?", dataNeverInitialized, environment.ID); err == nil {
		t.Fatal("initialized identity reset to never used")
	}
	destination := filepath.Join(t.TempDir(), "synthetic-missing-data.prismbackup")
	token := backupDestinationFixture(t, s, destination)
	final := waitBackupFixture(t, s, acceptBackupFixture(t, s, BackupExportRequest{Scope: "selected", EnvironmentIDs: []string{environment.ID}, DestinationToken: token, StopRunning: true, RequestID: id()}).ID)
	if final.State != "failed" || final.BackupReport.Published {
		t.Fatal("missing previously claimed source was silently made empty")
	}
	if _, err := os.Lstat(destination); !os.IsNotExist(err) {
		t.Fatal("missing data created a complete output")
	}
}

func TestLegacyInitializationRemainsUnconfirmedAndCannotGrantMissingDirectory(t *testing.T) {
	s, root := fixture(t, Options{})
	environment, _ := create(t, s, "合成旧版缺失数据")
	for _, statement := range []string{"DROP TABLE restore_jobs", "DROP TABLE backup_exports", "DROP TABLE environment_data_state", "PRAGMA user_version=6"} {
		if _, err := s.db.Exec(statement); err != nil {
			t.Fatal(err)
		}
	}
	s.Close()
	reopened, err := Open(root, Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	reopened.mu.Lock()
	target, err := reopened.backupDataTarget(environment.ID, "environments/"+environment.ID+"/user-data")
	reopened.mu.Unlock()
	if err != nil || !target.DirectoryRequired || target.NeverUsed {
		t.Fatal("legacy lack of startup history inferred never initialized", err)
	}
}

func TestRuntimeDataPermissionRequiresDurableCommitBeforeAnyLauncher(t *testing.T) {
	var blocked atomic.Bool
	s, _ := fixture(t, Options{BeforeCommit: func() error {
		if blocked.Load() {
			return errors.New("synthetic disk unavailable")
		}
		return nil
	}})
	environment, _ := create(t, s, "合成初始化事务")
	slot := &runtimeSlot{session: RuntimeSession{EnvironmentID: environment.ID, SessionID: id()}, cancel: func() {}}
	s.mu.Lock()
	s.runtimeSlots[environment.ID] = slot
	s.mu.Unlock()
	blocked.Store(true)
	if err := s.claimRuntimeData(context.Background(), slot); err == nil {
		t.Fatal("uncommitted data permission could enter a launcher")
	}
	var state string
	s.db.QueryRow("SELECT state FROM environment_data_state WHERE environment_id=?", environment.ID).Scan(&state)
	if state != dataNeverInitialized {
		t.Fatal("failed transaction changed initialization proof")
	}
	blocked.Store(false)
	if err := s.claimRuntimeData(context.Background(), slot); err != nil {
		t.Fatal(err)
	}
	s.db.QueryRow("SELECT state FROM environment_data_state WHERE environment_id=?", environment.ID).Scan(&state)
	if state != dataRuntimeClaimed {
		t.Fatal("successful permission not recorded before launch")
	}
	s.mu.Lock()
	delete(s.runtimeSlots, environment.ID)
	s.mu.Unlock()
}

func TestBackupDiskFullErrorIsActionableAndDoesNotLeakSourceOrDestination(t *testing.T) {
	err := backupFailure(&os.PathError{Op: "write", Path: "C:/SYNTHETIC_PRIVATE_OUTPUT", Err: windows.ERROR_DISK_FULL})
	if err.Code != "DISK_FULL" || strings.Contains(err.Message, "SYNTHETIC_PRIVATE_OUTPUT") {
		t.Fatal("output failure leaked private paths or was not classified")
	}
}
