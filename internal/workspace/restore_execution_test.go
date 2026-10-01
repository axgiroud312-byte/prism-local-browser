package workspace

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func acceptRestoreFixture(t *testing.T, s *Service, p RestorePreview) (RestoreRequest, Operation) {
	t.Helper()
	r := RestoreRequest{PreviewID: p.PreviewID, ArchiveSHA256: p.ArchiveSHA256, ConfirmOverwrite: true, AcknowledgeCredentials: true, StopRunning: true, RequestID: id()}
	o := value[struct {
		Operation Operation `json:"operation"`
	}](t, call(s, "Backup.ApplyRestore", r)).Operation
	return r, o
}
func restoreDataFixture(t *testing.T, s *Service, root string) (Environment, string, string) {
	t.Helper()
	e, _ := create(t, s, "合成恢复目录")
	ref, _ := dataReference(e.ID)
	dir := filepath.Join(root, filepath.FromSlash(ref))
	if err := os.MkdirAll(filepath.Join(dir, "empty"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "Cookies"), []byte("SYNTHETIC_ARCHIVED_BYTES"), 0600); err != nil {
		t.Fatal(err)
	}
	path := restorePackageFixture(t, s, []string{e.ID})
	if err := os.WriteFile(filepath.Join(dir, "Cookies"), []byte("SYNTHETIC_CURRENT_BYTES"), 0600); err != nil {
		t.Fatal(err)
	}
	return e, dir, path
}
func TestRestoreReplacesDataPreservesIdentityAndKeepsPreviousCopy(t *testing.T) {
	s, root := fixture(t, Options{})
	e, dir, path := restoreDataFixture(t, s, root)
	p := value[RestorePreview](t, previewPackageFixture(t, s, path))
	r, o := acceptRestoreFixture(t, s, p)
	final := waitBackupFixture(t, s, o.ID)
	if final.State != "completed" || !final.RestoreReport.Committed || final.RestoreReport.Protected || final.RestoreReport.RolledBack {
		t.Fatal("restore not complete", final)
	}
	data, err := os.ReadFile(filepath.Join(dir, "Cookies"))
	if err != nil || string(data) != "SYNTHETIC_ARCHIVED_BYTES" {
		t.Fatal("archive bytes not installed", err)
	}
	old, err := os.ReadFile(filepath.Join(root, "backups", "restore", o.ID, "previous", e.ID, "user-data", "Cookies"))
	if err != nil || string(old) != "SYNTHETIC_CURRENT_BYTES" {
		t.Fatal("previous copy not retained", err)
	}
	current, revision, _, err := s.readEnvironment(e.ID)
	if err != nil || current.Seed != e.Seed || current.CoreID != e.CoreID || revision < 2 {
		t.Fatal("restored identity or revision differs", err)
	}
	var fact string
	if err = s.db.QueryRow("SELECT state FROM environment_data_state WHERE environment_id=?", e.ID).Scan(&fact); err != nil || fact != dataDirectoryPrepared {
		t.Fatal("real directory restored as never initialized", fact, err)
	}
	again := value[struct {
		Operation Operation `json:"operation"`
	}](t, call(s, "Backup.ApplyRestore", r))
	if again.Operation.ID != o.ID {
		t.Fatal("idempotent request executed a second restore")
	}
}
func TestRestoreFailuresBeforeCommitRollbackEveryDirectory(t *testing.T) {
	for _, phase := range []string{"prepared", "old-retained", "new-switched", "before-db-commit"} {
		t.Run(phase, func(t *testing.T) {
			s, root := fixture(t, Options{RestoreCheckpoint: func(p string) error {
				if p == phase {
					return errors.New("SYNTHETIC_SWITCH_FAILURE")
				}
				return nil
			}})
			e, dir, path := restoreDataFixture(t, s, root)
			before, _ := restoreBaseline(s.db)
			_, o := acceptRestoreFixture(t, s, value[RestorePreview](t, previewPackageFixture(t, s, path)))
			final := waitBackupFixture(t, s, o.ID)
			if final.State != "failed" || final.Error.Code != "RESTORE_INCOMPLETE" || !final.RestoreReport.RolledBack || final.RestoreReport.Committed {
				t.Fatal("partial restore reported success", final)
			}
			data, err := os.ReadFile(filepath.Join(dir, "Cookies"))
			if err != nil || string(data) != "SYNTHETIC_CURRENT_BYTES" {
				t.Fatal("old bytes lost", err)
			}
			after, _ := restoreBaseline(s.db)
			if after != before {
				t.Fatal("old configuration changed on rollback")
			}
			if release, err := s.AcquireProfileUse(e.ID); err != nil {
				t.Fatal("consistent rollback remained locked", err)
			} else {
				release()
			}
		})
	}
}
func TestRestorePostCommitFailureConfirmsWholeNewState(t *testing.T) {
	s, root := fixture(t, Options{RestoreCheckpoint: func(phase string) error {
		if phase == "db-committed" {
			return errors.New("SYNTHETIC_POST_COMMIT_FAILURE")
		}
		return nil
	}})
	_, dir, path := restoreDataFixture(t, s, root)
	_, o := acceptRestoreFixture(t, s, value[RestorePreview](t, previewPackageFixture(t, s, path)))
	final := waitBackupFixture(t, s, o.ID)
	if final.State != "completed" || !final.RestoreReport.Committed {
		t.Fatal("durable marker incorrectly rolled back", final)
	}
	data, _ := os.ReadFile(filepath.Join(dir, "Cookies"))
	if string(data) != "SYNTHETIC_ARCHIVED_BYTES" {
		t.Fatal("committed data reverted")
	}
}
func TestRestoreRechecksArchiveAndRevisionBeforeReplacingData(t *testing.T) {
	for _, changed := range []string{"archive", "revision"} {
		t.Run(changed, func(t *testing.T) {
			s, root := fixture(t, Options{})
			e, dir, path := restoreDataFixture(t, s, root)
			p := value[RestorePreview](t, previewPackageFixture(t, s, path))
			if changed == "revision" {
				if _, err := s.db.Exec("UPDATE environments SET revision=revision+1 WHERE id=?", e.ID); err != nil {
					t.Fatal(err)
				}
				wantError(t, call(s, "Backup.ApplyRestore", RestoreRequest{PreviewID: p.PreviewID, ArchiveSHA256: p.ArchiveSHA256, ConfirmOverwrite: true, StopRunning: true, RequestID: id()}), "REVISION_CONFLICT")
			} else {
				if err := os.WriteFile(path, []byte("SYNTHETIC_REPLACED_ARCHIVE"), 0600); err != nil {
					t.Fatal(err)
				}
				_, o := acceptRestoreFixture(t, s, p)
				final := waitBackupFixture(t, s, o.ID)
				if final.State != "failed" || !final.RestoreReport.RolledBack {
					t.Fatal("changed package accepted", final)
				}
			}
			data, _ := os.ReadFile(filepath.Join(dir, "Cookies"))
			if string(data) != "SYNTHETIC_CURRENT_BYTES" {
				t.Fatal("precommit refusal changed old data")
			}
		})
	}
}
func TestRestoreCancellationRollsBackAndBarrierRejectsMutations(t *testing.T) {
	for _, boundary := range []string{"new-switched", "before-db-commit"} {
		t.Run(boundary, func(t *testing.T) {
			reached, proceed := make(chan struct{}), make(chan struct{})
			defer func() {
				select {
				case <-proceed:
				default:
					close(proceed)
				}
			}()
			s, root := fixture(t, Options{RestoreCheckpoint: func(phase string) error {
				if phase == boundary {
					close(reached)
					<-proceed
				}
				return nil
			}})
			e, dir, path := restoreDataFixture(t, s, root)
			_, o := acceptRestoreFixture(t, s, value[RestorePreview](t, previewPackageFixture(t, s, path)))
			<-reached
			wantError(t, call(s, "Environment.Preview", map[string]string{"kind": "create"}), "RESTORE_INCOMPLETE")
			if release, err := s.AcquireProfileUse(e.ID); err == nil {
				release()
				t.Fatal("host profile lease bypassed restore barrier")
			}
			value[Operation](t, call(s, "Operation.Cancel", map[string]string{"operationId": o.ID}))
			close(proceed)
			final := waitBackupFixture(t, s, o.ID)
			if final.State != "cancelled" || !final.RestoreReport.RolledBack {
				t.Fatal("cancel did not rollback", final)
			}
			data, _ := os.ReadFile(filepath.Join(dir, "Cookies"))
			if string(data) != "SYNTHETIC_CURRENT_BYTES" {
				t.Fatal("cancel left mixed data")
			}
		})
	}
}
func TestRestoreRestoresAbsentIdentityAndStrengthensInitializationFact(t *testing.T) {
	source, _ := fixture(t, Options{})
	e, _ := create(t, source, "合成新身份恢复")
	path := restorePackageFixture(t, source, []string{e.ID})
	target, root := fixture(t, Options{})
	p := value[RestorePreview](t, previewPackageFixture(t, target, path))
	if p.AddCount != 1 {
		t.Fatal("not an add")
	}
	_, o := acceptRestoreFixture(t, target, p)
	final := waitBackupFixture(t, target, o.ID)
	if final.State != "completed" {
		t.Fatal(final)
	}
	ref, _ := dataReference(e.ID)
	if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(ref))); err != nil {
		t.Fatal(err)
	}
	var state string
	target.db.QueryRow("SELECT state FROM environment_data_state WHERE environment_id=?", e.ID).Scan(&state)
	if state != dataDirectoryPrepared {
		t.Fatal("installed directory fact lost", state)
	}
	current, _, _, err := target.readEnvironment(e.ID)
	if err != nil || current.Seed != e.Seed {
		t.Fatal("restore cloned a new identity", err)
	}
}
func TestRestoreKeepsStrongerOldInitializationFact(t *testing.T) {
	for _, fact := range []string{dataRuntimeClaimed, dataLegacyUnconfirmed} {
		t.Run(fact, func(t *testing.T) {
			s, root := fixture(t, Options{})
			e, dir, path := restoreDataFixture(t, s, root)
			if _, err := s.db.Exec("UPDATE environment_data_state SET state=? WHERE environment_id=?", fact, e.ID); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(dir, ".prism-runtime.lock"), nil, 0600); err != nil {
				t.Fatal(err)
			}
			_, o := acceptRestoreFixture(t, s, value[RestorePreview](t, previewPackageFixture(t, s, path)))
			final := waitBackupFixture(t, s, o.ID)
			if final.State != "completed" {
				t.Fatal(final)
			}
			var got string
			s.db.QueryRow("SELECT state FROM environment_data_state WHERE environment_id=?", e.ID).Scan(&got)
			if got != fact {
				t.Fatal("restoration downgraded fact", got)
			}
		})
	}
}
func TestRestoreUnconfirmedAcceptanceAbsentRowsReleasesBarrierWithoutScheduling(t *testing.T) {
	s, _ := fixture(t, Options{})
	task := &restoreTask{acceptancePending: true, signature: "synthetic-signature", plan: restorePlan{ID: id()}, operation: Operation{ID: id(), Kind: "backup-restore", RestoreReport: &RestoreReport{RequestID: id()}}}
	task.operation.ID = task.plan.ID
	s.restoreTask = task
	err := s.confirmRestoreAcceptance(task, task.signature)
	var problem *Error
	if !errors.As(err, &problem) || problem.Code != "RESTORE_NOT_ACCEPTED" || s.restoreTask != nil || task.running {
		t.Fatal("known nonacceptance remained locked or replayed", err)
	}
}
func TestRestoreRPCDoesNotLeakPrivatePlan(t *testing.T) {
	s, root := fixture(t, Options{})
	_, _, path := restoreDataFixture(t, s, root)
	_, o := acceptRestoreFixture(t, s, value[RestorePreview](t, previewPackageFixture(t, s, path)))
	waitBackupFixture(t, s, o.ID)
	result := call(s, "Operation.Read", map[string]string{"operationId": o.ID})
	encoded, _ := json.Marshal(result.Data)
	data := string(encoded)
	for _, secret := range []string{root, path, "SYNTHETIC_ARCHIVED_BYTES", "plan_json", "protected" + "Bytes"} {
		if strings.Contains(data, secret) {
			t.Fatal("RPC leaked private journal", secret)
		}
	}
}

func TestRestoreEarlyFailureDoesNotReleaseRuntimeOwner(t *testing.T) {
	s, root := fixture(t, Options{})
	e, _, path := restoreDataFixture(t, s, root)
	p := value[RestorePreview](t, previewPackageFixture(t, s, path))
	s.mu.Lock()
	s.runtimeSlots[e.ID] = &runtimeSlot{session: RuntimeSession{EnvironmentID: e.ID, State: "starting"}, cancel: func() {}}
	s.profileUses[e.ID] = true
	s.mu.Unlock()
	defer func() { s.mu.Lock(); delete(s.runtimeSlots, e.ID); delete(s.profileUses, e.ID); s.mu.Unlock() }()
	if err := os.WriteFile(path, []byte("SYNTHETIC_BAD_PACKAGE"), 0600); err != nil {
		t.Fatal(err)
	}
	_, o := acceptRestoreFixture(t, s, p)
	final := waitBackupFixture(t, s, o.ID)
	if final.State != "failed" {
		t.Fatal(final)
	}
	s.mu.Lock()
	busy := s.profileUses[e.ID] && s.runtimeOwnsProfileUse(e.ID)
	s.mu.Unlock()
	if !busy {
		t.Fatal("failed restore released live runtime ownership")
	}
}
func TestRestoreFinalWriteRetriesWithoutReplayingDirectorySwitch(t *testing.T) {
	var blocked atomic.Bool
	var switches atomic.Int32
	s, root := fixture(t, Options{BeforeCommit: func() error {
		if blocked.Load() {
			return errors.New("SYNTHETIC_STORAGE_FAILURE")
		}
		return nil
	}, RestoreCheckpoint: func(phase string) error {
		if phase == "new-switched" {
			switches.Add(1)
		}
		if phase == "db-committed" {
			blocked.Store(true)
		}
		return nil
	}})
	_, dir, path := restoreDataFixture(t, s, root)
	_, o := acceptRestoreFixture(t, s, value[RestorePreview](t, previewPackageFixture(t, s, path)))
	deadline := time.Now().Add(10 * time.Second)
	pending := false
	for time.Now().Before(deadline) {
		s.mu.Lock()
		pending = s.restoreTask != nil && s.restoreTask.finalPending != nil
		s.mu.Unlock()
		if pending {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	if !pending {
		t.Fatal("final write failure not preserved")
	}
	blocked.Store(false)
	final := waitBackupFixture(t, s, o.ID)
	if final.State != "completed" || switches.Load() != 1 {
		t.Fatal("save retry replayed file effects", final, switches.Load())
	}
	data, _ := os.ReadFile(filepath.Join(dir, "Cookies"))
	if string(data) != "SYNTHETIC_ARCHIVED_BYTES" {
		t.Fatal("save retry lost data")
	}
}
func TestRestoreAllowsSelectedNameSwapWithoutChangingOutsideNumber(t *testing.T) {
	s, _ := fixture(t, Options{})
	a, _ := create(t, s, "合成名称A")
	b, _ := create(t, s, "合成名称B")
	path := restorePackageFixture(t, s, []string{a.ID, b.ID})
	for _, change := range []struct{ id, name string }{{a.ID, "合成临时名"}, {b.ID, a.Name}, {a.ID, b.Name}} {
		p := preview(t, s, "edit", change.id)
		p.Environment.Name = change.name
		value[any](t, call(s, "Environment.Update", Mutation{PreviewID: p.PreviewID, Configuration: p.Environment.Configuration, ExpectedRevision: p.ExpectedRevision, RequestID: id()}))
	}
	p := value[RestorePreview](t, previewPackageFixture(t, s, path))
	if !p.CanRestore {
		t.Fatal("valid final names rejected")
	}
	_, o := acceptRestoreFixture(t, s, p)
	if final := waitBackupFixture(t, s, o.ID); final.State != "completed" {
		t.Fatal(final)
	}
	for _, e := range []Environment{a, b} {
		current, _, _, err := s.readEnvironment(e.ID)
		if err != nil || current.Name != e.Name || current.Seed != e.Seed {
			t.Fatal("name swap changed identity", err)
		}
	}
	other, _ := fixture(t, Options{})
	outside, _ := create(t, other, "合成包外编号")
	_, o = acceptRestoreFixture(t, other, value[RestorePreview](t, previewPackageFixture(t, other, path)))
	if final := waitBackupFixture(t, other, o.ID); final.State != "completed" {
		t.Fatal(final)
	}
	current, _, _, err := other.readEnvironment(outside.ID)
	if err != nil || current.Code != outside.Code || current.Seed != outside.Seed {
		t.Fatal("new imports overwrote outside display number", err)
	}
}

func TestRestoreProtectedRollbackCanRetryWithoutReopeningArchive(t *testing.T) {
	var s *Service
	var root, environmentID string
	var failed atomic.Bool
	options := Options{RestoreCheckpoint: func(phase string) error {
		if phase != "new-switched" || failed.Swap(true) {
			return nil
		}
		s.mu.Lock()
		operationID := s.restoreTask.plan.ID
		s.mu.Unlock()
		path := filepath.Join(root, "backups", "restore", operationID, "previous", environmentID, "user-data", "Cookies")
		if err := os.WriteFile(path, []byte("SYNTHETIC_DAMAGED_ROLLBACK_COPY"), 0600); err != nil {
			return err
		}
		return errors.New("SYNTHETIC_SWITCH_FAILURE")
	}}
	s, root = fixture(t, options)
	e, dir, path := restoreDataFixture(t, s, root)
	environmentID = e.ID
	_, o := acceptRestoreFixture(t, s, value[RestorePreview](t, previewPackageFixture(t, s, path)))
	deadline := time.Now().Add(10 * time.Second)
	protected := false
	for time.Now().Before(deadline) {
		s.mu.Lock()
		protected = s.restoreTask != nil && !s.restoreTask.running && s.restoreTask.recoveryAvailable
		s.mu.Unlock()
		if protected {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	if !protected {
		t.Fatal("unknown rollback not protected")
	}
	op := value[Operation](t, call(s, "Operation.Read", map[string]string{"operationId": o.ID}))
	if !op.PersistencePending || !op.RestoreReport.Protected {
		t.Fatal("mixed state published", op)
	}
	if err := os.WriteFile(filepath.Join(root, "backups", "restore", o.ID, "previous", e.ID, "user-data", "Cookies"), []byte("SYNTHETIC_CURRENT_BYTES"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	} // Recovery needs only the journal and retained trees.
	value[Operation](t, call(s, "Backup.RecoverRestore", map[string]string{"operationId": o.ID}))
	final := waitBackupFixture(t, s, o.ID)
	if final.State != "failed" || !final.RestoreReport.RolledBack || final.RestoreReport.Protected {
		t.Fatal("retry did not confirm old state", final)
	}
	data, _ := os.ReadFile(filepath.Join(dir, "Cookies"))
	if string(data) != "SYNTHETIC_CURRENT_BYTES" {
		t.Fatal("recovery replayed source instead of rollback")
	}
}

func TestRestoreFinalizationCannotBeCancelledOrReplayPendingTerminal(t *testing.T) {
	s, _ := fixture(t, Options{})
	op := Operation{ID: id(), Kind: "backup-restore", State: "running", RestoreReport: &RestoreReport{Mode: "native"}}
	task := &restoreTask{operation: op, running: true, recoveryAvailable: true}
	s.restoreTask = task
	wantError(t, s.cancelRestore(task), "RESTORE_FINALIZING")
	if task.operation.CancelRequested {
		t.Fatal("uninterruptible finalization falsely accepted cancel")
	}
	task.running = false
	final := copyRestoreOperation(op)
	task.finalPending = &final
	result := s.retryRestoreFinalization(op.ID)
	if !result.OK || task.running || task.finalPending == nil {
		t.Fatal("terminal-pending retry spawned a new directory observer")
	}
	s.restoreTask = nil
}
