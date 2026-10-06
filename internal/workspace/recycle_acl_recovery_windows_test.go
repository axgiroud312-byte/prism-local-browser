//go:build windows

package workspace

import (
	"context"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"

	"github.com/axgiroud312-byte/prism-local-browser/internal/backup"
	"golang.org/x/sys/windows"
)

func TestV1SafeGapRecycleActualDeleteACLRecoveryOnlyConfirmedTarget(t *testing.T) {
	requireSafeGapAcceptance(t)
	var root, targetID string
	var fileDenial, parentDenial *ownedACLDenial
	var inserted atomic.Bool
	options := Options{RecycleCheckpoint: func(phase string) error {
		if phase == "purging" && !inserted.Swap(true) {
			dir := filepath.Join(root, filepath.FromSlash(recycleReference(targetID)))
			// Windows grants DELETE through either the object or its parent.
			// Deny both routes, and independently prove DELETE-access open fails.
			parentDenial = denyOwnedACL(t, dir, 0x40 /* FILE_DELETE_CHILD */)
			file := filepath.Join(dir, "Cookies")
			fileDenial = denyOwnedACL(t, file, windows.DELETE)
			assertOwnedAccessDenied(t, file, windows.DELETE)
		}
		return nil
	}}
	s, root := fixture(t, options)
	a, _ := recycleFixture(t, s, root, "Owned purge ACL A")
	b, bDir := recycleFixture(t, s, root, "Owned purge ACL untouched B")
	archive := restorePackageFixture(t, s, []string{a.ID})
	archiveDigest, err := backup.PublishedDigest(context.Background(), archive)
	if err != nil {
		t.Fatal(err)
	}
	bBaseline, err := os.ReadFile(filepath.Join(bDir, "Cookies"))
	if err != nil {
		t.Fatal(err)
	}
	if removed := runRecycleFixture(t, s, "remove", a.ID); removed.State != "completed" {
		t.Fatal("owned recycle prerequisite failed")
	}
	for _, item := range recycleListFixture(t, s).Items {
		if item.EnvironmentID == a.ID {
			targetID = item.ID
		}
	}
	if targetID == "" {
		t.Fatal("owned recycled target missing")
	}
	_, accepted := acceptRecycleFixture(t, s, previewRecycleFixture(t, s, "purge", targetID))
	waitRecycleProtectedFixture(t, s)
	s.mu.Lock()
	plan := s.recycleTask.plan
	protected := copyRecycleOperation(s.recycleTask.operation)
	s.mu.Unlock()
	if fileDenial == nil || parentDenial == nil || protected.ID != accepted.ID || !protected.RecycleReport.Protected || len(plan.Items) != 1 || plan.Items[0].DataDeleted {
		t.Fatal("actual delete denial was not retained before any target deletion")
	}
	entry := plan.Items[0].Entry
	if err := backup.VerifyTree(context.Background(), root, recycleReference(targetID), entry.Identity, entry.Files); err != nil {
		t.Fatal("actual ACL-blocked purge partially changed the target inventory")
	}
	closeProtectedSafeGapService(t, s, "recycle outcome remains protected or could not be persisted")
	reopened, err := Open(root, options)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { reopened.Close() })
	waitRecycleProtectedFixture(t, reopened)
	wantError(t, call(reopened, "Runtime.Start", runtimeRequest{EnvironmentID: b.ID, RequestID: id()}), "RECYCLE_INCOMPLETE")
	// The restoration handles must also be released before actual deletion,
	// otherwise this test itself keeps the repaired file delete-pending.
	if err := fileDenial.restoreAndRelease(); err != nil {
		t.Fatal(err)
	}
	if err := parentDenial.restoreAndRelease(); err != nil {
		t.Fatal(err)
	}
	value[Operation](t, call(reopened, "Recycle.Recover", map[string]string{"operationId": accepted.ID}))
	final := waitLocalAcceptanceDurable(t, reopened, accepted.ID)
	if final.State != "completed" || final.RecycleReport.Protected || final.RecycleReport.Completed != 1 {
		t.Fatal("original confirmed purge did not finish after actual DELETE access repair:", final.Error)
	}
	if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(recycleReference(targetID)))); !os.IsNotExist(err) {
		t.Fatal("confirmed repaired target still present")
	}
	after, err := os.ReadFile(filepath.Join(bDir, "Cookies"))
	if err != nil || string(after) != string(bBaseline) {
		t.Fatal("repaired purge changed unconfirmed environment data")
	}
	if digest, err := backup.PublishedDigest(context.Background(), archive); err != nil || digest != archiveDigest {
		t.Fatal("repaired purge changed the historical full backup")
	}
	current, _, _, err := reopened.readEnvironment(b.ID)
	if err != nil || current.Seed != b.Seed {
		t.Fatal("repaired purge changed unconfirmed environment identity")
	}
	writeLocalAcceptanceEvidence(t, "recycle-delete-acl-recovery", map[string]any{"scope": safeGapScope, "actualWin32DeleteAccessDenied": true, "objectDeleteAndParentDeleteChildRoutesDenied": true, "checkpointReturnedNoSyntheticError": true, "completeTargetInventoryRetainedBeforeRepair": true, "serviceReopenedAndProtectionRetained": true, "originalDACLsRestoredAndCompared": true, "sameConfirmedOperationCompleted": true, "onlyAuthorizedTargetDeleted": true, "otherEnvironmentIdentityAndBytesUnchanged": true, "historicalBackupDigestUnchanged": true})
}
