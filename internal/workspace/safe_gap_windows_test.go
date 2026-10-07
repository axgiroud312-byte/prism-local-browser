//go:build windows

package workspace

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/axgiroud312-byte/prism-local-browser/internal/kernel"
	"golang.org/x/sys/windows"
)

const safeGapScope = "actual Windows files/DACL and application services in newly owned temporary roots; synthetic kernel metadata, no Chromium/browser/UI/external-egress claim"

func closeProtectedSafeGapService(t *testing.T, s *Service, warning string) {
	t.Helper()
	// A protected maintenance outcome is intentionally reported as a shutdown
	// warning even though the service closes its DB and can be reopened safely.
	if err := s.Close(); err == nil || err.Error() != warning {
		t.Fatal("protected shutdown did not report exactly its retained outcome:", err)
	}
	select {
	case <-s.closeDone:
	default:
		t.Fatal("protected service shutdown did not finish")
	}
	if s.db.Ping() == nil {
		t.Fatal("protected shutdown left the original database open")
	}
}

func TestV1SafeGapBatchProductionACLFailureNoTypedNil(t *testing.T) {
	requireSafeGapAcceptance(t)
	s, root := fixture(t, Options{})
	parent := filepath.Join(root, "environments")
	if err := os.MkdirAll(parent, 0700); err != nil {
		t.Fatal(err)
	}
	denial := denyOwnedACL(t, parent, ownedDirectoryAddSubdirectory)
	assertOwnedAccessDenied(t, parent, ownedDirectoryAddSubdirectory)
	page := createBatchPreviewFixture(t, s, "Owned production permission retry", 1)
	accepted := acceptBatchFixture(t, s, page, id())
	failed := waitRuntimeReal(t, s, accepted.ID)
	if failed.State != "failed" || failed.Error == nil || failed.Error.Code != "DIRECTORY_WRITE_FAILED" || failed.BatchReport.CompletedCount != 0 {
		t.Fatal("production actual permission failure did not pause safely without a typed-nil cleanup panic")
	}
	if err := denial.restore(); err != nil {
		t.Fatal(err)
	}
	// Independently exercise the exact failed prepare return as an interface.
	lease, err := s.prepareBatchDirectory(BatchDirectoryInput{Root: root, EnvironmentID: "invalid", PlanID: page.PlanID})
	if lease != nil || err == nil {
		t.Fatal("failed production preparation exposed a typed-nil lease")
	}
	if _, ok := err.(*kernel.Problem); !ok {
		t.Fatal("production ownership validation no longer classifies its failure")
	}
	retry := value[struct {
		Operation Operation `json:"operation"`
	}](t, call(s, "Batch.Retry", map[string]string{"operationId": accepted.ID, "requestId": id()})).Operation
	if final := waitRuntimeReal(t, s, retry.ID); final.State != "completed" || final.BatchReport.CompletedCount != 1 {
		t.Fatal("production actual directory retry failed after restoring access")
	}
	writeLocalAcceptanceEvidence(t, "batch-production-acl", map[string]any{"scope": safeGapScope, "noPrepareBatchDirectorySeam": true, "actualWin32AccessDenied": true, "failedLeaseIsNilInterface": true, "safeFailureCode": failed.Error.Code, "noCleanupPanic": true, "retryAfterOriginalDACLRestorationCompleted": true})
}

func TestV1SafeGapBatch257ActualACLRetry(t *testing.T) {
	requireSafeGapAcceptance(t)
	var count atomic.Int32
	var denial *ownedACLDenial
	s, root := fixture(t, Options{PrepareBatchDirectory: func(input BatchDirectoryInput) (BatchDirectoryLease, error) {
		if count.Add(1) == 129 {
			parent := filepath.Join(input.Root, "environments")
			denial = denyOwnedACL(t, parent, ownedDirectoryAddSubdirectory)
			assertOwnedAccessDenied(t, parent, ownedDirectoryAddSubdirectory)
		}
		// Change only real filesystem permissions, then use the same production
		// wrapper (including its nil-lease normalization) without recursion.
		production := &Service{}
		return production.prepareBatchDirectory(input)
	}})
	page := createBatchPreviewFixture(t, s, "Owned 257 actual directories", 257)
	request := id()
	first := acceptBatchFixture(t, s, page, request)
	failed := waitRuntimeReal(t, s, first.ID)
	if failed.State != "failed" || failed.BatchReport.CompletedCount != 128 || denial == nil {
		t.Fatal("actual ACL failure did not preserve accurate partial completion")
	}
	var preparedID, preparedSeed string
	if err := s.db.QueryRow("SELECT json_extract(identity_json,'$.environmentId'),json_extract(identity_json,'$.profile.seed') FROM batch_items WHERE plan_id=? AND item_index=128", page.PlanID).Scan(&preparedID, &preparedSeed); err != nil {
		t.Fatal(err)
	}
	before := map[string]string{}
	rows, err := s.db.Query("SELECT e.id,f.seed FROM environments e JOIN fingerprints f ON f.id=e.fingerprint_id")
	if err != nil {
		t.Fatal(err)
	}
	for rows.Next() {
		var id, seed string
		if err := rows.Scan(&id, &seed); err != nil {
			t.Fatal(err)
		}
		before[id] = seed
	}
	if err := rows.Close(); err != nil {
		t.Fatal(err)
	}
	if err := denial.restore(); err != nil {
		t.Fatal(err)
	}
	retried := value[struct {
		Operation Operation `json:"operation"`
	}](t, call(s, "Batch.Retry", map[string]string{"operationId": first.ID, "requestId": id()})).Operation
	final := waitRuntimeReal(t, s, retried.ID)
	if final.State != "completed" || final.BatchReport.CompletedCount != 257 {
		t.Fatal("large real directory batch did not resume after DACL restoration:", final.Error)
	}
	seenIDs, seenSeeds := map[string]bool{}, map[string]bool{}
	for offset := int64(0); offset < 257; offset += 25 {
		for _, item := range readBatchPageFixture(t, s, page.PlanID, offset).Items {
			e, _, _, err := s.readEnvironment(item.EnvironmentID)
			if err != nil || seenIDs[e.ID] || seenSeeds[e.Seed] {
				t.Fatal("large batch duplicated or lost a prepared identity")
			}
			if seed, exists := before[e.ID]; exists && seed != e.Seed {
				t.Fatal("retry regenerated a completed identity")
			}
			if e.ID == preparedID && e.Seed != preparedSeed {
				t.Fatal("retry regenerated the failed prepared identity")
			}
			seenIDs[e.ID], seenSeeds[e.Seed] = true, true
			entries, err := os.ReadDir(filepath.Join(root, "environments", e.ID, "user-data"))
			if err != nil || len(entries) != 0 {
				t.Fatal("actual created directory was not empty")
			}
		}
	}
	if len(seenIDs) != 257 || !seenIDs[preparedID] || len(view(t, s).State.Environments) != 8 {
		t.Fatal("large batch pagination or prepared identity preservation failed")
	}
	if repeated := acceptBatchFixture(t, s, page, request); repeated.ID != first.ID {
		t.Fatal("original large batch request lost idempotency")
	}
	writeLocalAcceptanceEvidence(t, "batch-257-acl", map[string]any{"scope": safeGapScope, "actualEmptyDirectories": 257, "completedBeforeActualWin32AccessDenied": 128, "failedPreparedIdentityPreserved": true, "completedIdentitiesPreserved": true, "distinctIDsAndSeeds": 257, "resultPagesOf25AndEnvironmentPageOf8Checked": true, "originalDACLRestoredAndCompared": true, "requestDeduplicated": true})
}

func TestV1SafeGapBackupActualACLFailures(t *testing.T) {
	requireSafeGapAcceptance(t)
	for _, stage := range []string{"source-read", "output-create"} {
		t.Run(stage, func(t *testing.T) {
			s, root := fixture(t, Options{})
			e, dir, _ := restoreDataFixture(t, s, root)
			file := filepath.Join(dir, "Cookies")
			baseline, err := os.ReadFile(file)
			if err != nil {
				t.Fatal(err)
			}
			outputDir := t.TempDir()
			destination := filepath.Join(outputDir, "owned-permission.prismbackup")
			token := backupDestinationFixture(t, s, destination)
			var denial *ownedACLDenial
			if stage == "source-read" {
				denial = denyOwnedACL(t, file, windows.FILE_READ_DATA)
				assertOwnedAccessDenied(t, file, windows.GENERIC_READ)
			} else {
				denial = denyOwnedACL(t, outputDir, ownedDirectoryAddFile)
				assertOwnedAccessDenied(t, outputDir, ownedDirectoryAddFile)
			}
			accepted := acceptBackupFixture(t, s, BackupExportRequest{Scope: "selected", EnvironmentIDs: []string{e.ID}, DestinationToken: token, StopRunning: true, RequestID: id()})
			failed := waitRuntimeReal(t, s, accepted.ID)
			if failed.State != "failed" || failed.BackupReport != nil && failed.BackupReport.Published {
				t.Fatal("actual permission failure published a successful full backup")
			}
			if _, err := os.Stat(destination); !os.IsNotExist(err) {
				t.Fatal("actual permission failure left a formally published package")
			}
			projection, _ := json.Marshal(failed)
			if strings.Contains(string(projection), root) || strings.Contains(string(projection), destination) || strings.Contains(string(projection), string(baseline)) {
				t.Fatal("actual failure response leaked private paths or source data")
			}
			if err := denial.restore(); err != nil {
				t.Fatal(err)
			}
			data, err := os.ReadFile(file)
			if err != nil || !reflect.DeepEqual(data, baseline) {
				t.Fatal("permission failure changed source bytes")
			}
			token = backupDestinationFixture(t, s, destination)
			done := waitRuntimeReal(t, s, acceptBackupFixture(t, s, BackupExportRequest{Scope: "selected", EnvironmentIDs: []string{e.ID}, DestinationToken: token, StopRunning: true, RequestID: id()}).ID)
			if done.State != "completed" || !done.BackupReport.Published {
				t.Fatal("repaired actual permissions did not allow a complete export")
			}
			writeLocalAcceptanceEvidence(t, "backup-acl-"+stage, map[string]any{"scope": safeGapScope, "actualWin32AccessDenied": true, "noPackagePublishedOnFailure": true, "sourceBytesUnchanged": true, "ordinaryResponseRedacted": true, "originalDACLRestoredAndCompared": true, "newRequestExportAfterRepairCompleted": true})
		})
	}
}

func TestV1SafeGapRestorePreviewActualACLReadFailure(t *testing.T) {
	requireSafeGapAcceptance(t)
	s, root := fixture(t, Options{})
	e, dir, path := restoreDataFixture(t, s, root)
	beforeDB := workspaceBytesFixture(t, root)
	beforeData, err := os.ReadFile(filepath.Join(dir, "Cookies"))
	if err != nil {
		t.Fatal(err)
	}
	denial := denyOwnedACL(t, path, windows.FILE_READ_DATA)
	assertOwnedAccessDenied(t, path, windows.GENERIC_READ)
	result := previewPackageFixture(t, s, path)
	if result.OK || result.Error == nil {
		t.Fatal("unreadable actual backup unexpectedly passed preflight")
	}
	if !reflect.DeepEqual(beforeDB, workspaceBytesFixture(t, root)) {
		t.Fatal("failed read-only preflight changed SQLite/sidecars")
	}
	if err := denial.restore(); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(dir, "Cookies"))
	if err != nil || !reflect.DeepEqual(beforeData, data) {
		t.Fatal("failed actual preflight changed browser files")
	}
	p := value[RestorePreview](t, previewPackageFixture(t, s, path))
	if !p.CanRestore || p.OverwriteCount != 1 {
		t.Fatal("preflight did not recover after restoring actual source access")
	}
	value[map[string]string](t, call(s, "Backup.DiscardRestore", map[string]string{"previewId": p.PreviewID}))
	if !reflect.DeepEqual(beforeDB, workspaceBytesFixture(t, root)) {
		t.Fatal("repaired read-only preflight changed SQLite/sidecars")
	}
	writeLocalAcceptanceEvidence(t, "restore-preview-acl", map[string]any{"scope": safeGapScope, "actualWin32ReadAccessDenied": true, "databaseWALSHMAndBrowserBytesUnchanged": true, "originalDACLRestoredAndCompared": true, "preflightAfterRepairCanRestore": true, "sameIdentityInPreview": e.ID != ""})
}
