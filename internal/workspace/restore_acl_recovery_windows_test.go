//go:build windows

package workspace

import (
	"context"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"

	"github.com/axgiroud312-byte/prism-local-browser/internal/backup"
)

func TestV1SafeGapRestoreActualACLBlockedSwitchAndRollbackRecovery(t *testing.T) {
	requireSafeGapAcceptance(t)
	var root, environmentID string
	var denial *ownedACLDenial
	var inserted atomic.Bool
	options := Options{RestoreCheckpoint: func(phase string) error {
		if phase == "old-retained" && !inserted.Swap(true) {
			parent := filepath.Join(root, "environments", environmentID)
			denial = denyOwnedACL(t, parent, ownedDirectoryAddSubdirectory)
			assertOwnedAccessDenied(t, parent, ownedDirectoryAddSubdirectory)
		}
		return nil // Both forward and rollback fail in the actual file operations.
	}}
	s, root := fixture(t, options)
	e, dir, archive := restoreDataFixture(t, s, root)
	environmentID = e.ID
	oldIdentity, exists, err := backup.IdentifyTree(root, "environments/"+e.ID+"/user-data")
	if err != nil || !exists {
		t.Fatal("owned old directory identity unavailable")
	}
	_, accepted := acceptRestoreFixture(t, s, value[RestorePreview](t, previewPackageFixture(t, s, archive)))
	protected := waitRestoreProtectedFixture(t, s)
	if denial == nil || protected.ID != accepted.ID || !protected.PersistencePending || !protected.RestoreReport.Protected || protected.RestoreReport.Committed {
		t.Fatal("actual ACL-blocked switch/rollback did not retain maintenance protection")
	}
	s.mu.Lock()
	plan := s.restoreTask.plan
	s.mu.Unlock()
	if len(plan.Moves) != 1 || plan.Moves[0].OldIdentity != oldIdentity {
		t.Fatal("actual restore journal lost the original directory identity")
	}
	move := plan.Moves[0]
	for _, tree := range []struct {
		ref      string
		identity backup.TreeIdentity
		files    []backup.File
	}{{move.Previous, move.OldIdentity, move.OldFiles}, {move.Incoming, move.NewIdentity, move.NewFiles}} {
		if err := backup.VerifyTree(context.Background(), root, tree.ref, tree.identity, tree.files); err != nil {
			t.Fatal("actual permission failure changed a retained complete side:", err)
		}
	}
	closeProtectedSafeGapService(t, s, "restore outcome remains protected or could not be persisted")
	reopened, err := Open(root, options)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { reopened.Close() })
	protected = waitRestoreProtectedFixture(t, reopened)
	if protected.ID != accepted.ID || !protected.RestoreReport.Protected {
		t.Fatal("service reopen cleared real ACL-blocked protection")
	}
	wantError(t, call(reopened, "Runtime.Start", runtimeRequest{EnvironmentID: e.ID, RequestID: id()}), "RESTORE_INCOMPLETE")
	wantError(t, call(reopened, "Environment.Preview", map[string]string{"action": "create"}), "RESTORE_INCOMPLETE")
	if err := os.Remove(archive); err != nil {
		t.Fatal(err)
	}
	if err := denial.restore(); err != nil {
		t.Fatal(err)
	}
	value[Operation](t, call(reopened, "Backup.RecoverRestore", map[string]string{"operationId": accepted.ID}))
	final := waitLocalAcceptanceDurable(t, reopened, accepted.ID)
	if final.State != "failed" || !final.RestoreReport.RolledBack || final.RestoreReport.Protected || final.PersistencePending {
		t.Fatal("original journal could not roll back after actual DACL repair:", final.Error)
	}
	if err := backup.VerifyTree(context.Background(), root, move.Live, oldIdentity, move.OldFiles); err != nil {
		t.Fatal("repaired recovery did not restore the exact original directory object and bytes")
	}
	data, err := os.ReadFile(filepath.Join(dir, "Cookies"))
	if err != nil || string(data) != "SYNTHETIC_CURRENT_BYTES" {
		t.Fatal("actual ACL recovery replayed the archived side instead of original current bytes")
	}
	current, _, _, err := reopened.readEnvironment(e.ID)
	if err != nil || current.ID != e.ID || current.Seed != e.Seed {
		t.Fatal("actual ACL recovery regenerated identity")
	}
	if err := reopened.Close(); err != nil {
		t.Fatal(err)
	}
	stable, err := Open(root, Options{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { stable.Close() })
	if stable.restoreTask != nil {
		t.Fatal("completed actual ACL recovery replayed after a second reopen")
	}
	writeLocalAcceptanceEvidence(t, "restore-acl-recovery", map[string]any{"scope": safeGapScope, "actualWin32ParentWriteAccessDenied": true, "checkpointReturnedNoSyntheticError": true, "forwardSwitchAndRollbackBlockedByActualACL": true, "bothCompleteSidesAndOriginalDirectoryIdentityRetained": true, "serviceReopenRetainedProtection": true, "startAndMutationRejectedWhileProtected": true, "sourceArchiveRemovedBeforeRecovery": true, "originalDACLRestoredAndCompared": true, "originalTaskRecoveredExactOldObjectAndBytes": true, "identityUnchanged": true, "secondReopenIdempotent": true})
}
