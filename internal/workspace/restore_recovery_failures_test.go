//go:build windows

package workspace

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/axgiroud312-byte/prism-local-browser/internal/kernel"
)

// Produces a real, replayable unfinished journal using a damaged synthetic
// rollback snapshot. This exercises recovery failures, not hard-process death.
func protectedRestoreJournalFixture(t *testing.T, absent ...bool) (*Service, string, Environment, Operation, string, []byte) {
	t.Helper()
	var s *Service
	var root, snapshot string
	var original []byte
	options := Options{RestoreCheckpoint: func(phase string) error {
		if phase != "new-switched" {
			return nil
		}
		s.mu.Lock()
		operationID := s.restoreTask.plan.ID
		s.mu.Unlock()
		snapshot = filepath.Join(root, "backups", "restore", operationID, "previous-configuration.sqlite")
		var err error
		original, err = os.ReadFile(snapshot)
		if err != nil {
			return err
		}
		if err = os.WriteFile(snapshot, []byte("SYNTHETIC_DAMAGED_SNAPSHOT"), 0600); err != nil {
			return err
		}
		return errors.New("SYNTHETIC_RESTORE_INTERRUPTION")
	}}
	s, root = fixture(t, options)
	var e Environment
	var source string
	if len(absent) > 0 && absent[0] {
		e, _ = create(t, s, "合成未初始化恢复")
		source = restorePackageFixture(t, s, []string{e.ID})
	} else {
		e, _, source = restoreDataFixture(t, s, root)
	}
	_, op := acceptRestoreFixture(t, s, value[RestorePreview](t, previewPackageFixture(t, s, source)))
	protected := waitRestoreProtectedFixture(t, s)
	if protected.ID != op.ID || len(original) == 0 {
		t.Fatal("fixture did not retain the intended journal")
	}
	if err := os.Remove(source); err != nil {
		t.Fatal(err)
	}
	return s, root, e, op, snapshot, original
}

func saveNoProcessRestoreFixture(t *testing.T, s *Service, e Environment) {
	t.Helper()
	ref, _ := dataReference(e.ID)
	_, revision, profileID, err := s.readEnvironment(e.ID)
	if err != nil {
		t.Fatal(err)
	}
	profile, err := readProfileFrom(s.db, profileID)
	if err != nil {
		t.Fatal(err)
	}
	session := RuntimeSession{Mode: "native", EnvironmentID: e.ID, SessionID: id(), OperationID: id(), State: "error", Revision: revision, FingerprintRevision: profile.Profile.ConfigRevision, KernelID: e.CoreID, UserDataRef: ref, NetworkPolicy: "direct", LaunchStage: "no-process-created", ResourceVersion: kernel.ManagedRuntimeVersion}
	operation := Operation{ID: session.OperationID, Kind: "runtime-start", EnvironmentID: e.ID, SessionID: session.SessionID, State: "failed"}
	encoded, _ := json.Marshal(operation)
	if _, err := s.db.Exec("INSERT INTO operations(id,result_json) VALUES(?,?)", operation.ID, string(encoded)); err != nil {
		t.Fatal(err)
	}
	encoded, _ = json.Marshal(session)
	if _, err := s.db.Exec("INSERT INTO runtime_sessions(environment_id,record_json) VALUES(?,?)", e.ID, string(encoded)); err != nil {
		t.Fatal(err)
	}
}

func TestRestoreBootstrapQueriesAndShutdownDoNotWaitForInspectionMutex(t *testing.T) {
	s, root, e, op, snapshot, original := protectedRestoreJournalFixture(t)
	if err := os.WriteFile(snapshot, original, 0600); err != nil {
		t.Fatal(err)
	}
	saveNoProcessRestoreFixture(t, s, e)
	s.Close()
	entered, proceed := make(chan struct{}), make(chan struct{})
	var once sync.Once
	release := func() { once.Do(func() { close(proceed) }) }
	defer release()
	reopened, err := Open(root, Options{InspectRuntime: func(RuntimeSession) (kernel.ManagedRecovery, error) {
		close(entered)
		<-proceed
		return kernel.ManagedRecovery{ProcessState: "not-created", DirectoryFree: true, SessionMatches: true, ResourcesExited: true}, nil
	}})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { reopened.Close() })
	select {
	case <-entered:
	case <-time.After(5 * time.Second):
		t.Fatal("bootstrap did not inspect")
	}
	read := make(chan Result, 1)
	go func() { read <- call(reopened, "Operation.Read", map[string]string{"operationId": op.ID}) }()
	select {
	case result := <-read:
		current := value[Operation](t, result)
		if current.State != "running" || !current.PersistencePending || !current.RestoreReport.Protected {
			t.Fatal("startup inspection lost barrier", current)
		}
	case <-time.After(time.Second):
		t.Fatal("slow inspection held the RPC mutex")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	if err := reopened.CloseContext(ctx); err == nil {
		t.Fatal("close reported done before tracked inspection returned")
	}
	release()
	if err := reopened.Close(); err == nil {
		t.Fatal("close lost unfinished bootstrap journal")
	}
	again, err := Open(root, Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer again.Close()
	assertOldRestoreFixture(t, again, root, e, op.ID)
}

func TestRestoreBootstrapDoesNotRecreateAbsentOldProfileBeforeAnotherRestart(t *testing.T) {
	s, root, e, op, snapshot, original := protectedRestoreJournalFixture(t, true)
	if err := os.WriteFile(snapshot, original, 0600); err != nil {
		t.Fatal(err)
	}
	saveNoProcessRestoreFixture(t, s, e)
	proxyOp := Operation{ID: id(), Kind: "proxy-check", State: "accepted"}
	encoded, _ := json.Marshal(proxyOp)
	if _, err := s.db.Exec("INSERT INTO operations(id,result_json) VALUES(?,?)", proxyOp.ID, string(encoded)); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.Exec("CREATE TRIGGER synthetic_late_bootstrap_failure BEFORE UPDATE ON operations WHEN OLD.id='" + proxyOp.ID + "' BEGIN SELECT RAISE(ABORT,'SYNTHETIC_STORAGE_FAILURE'); END"); err != nil {
		t.Fatal(err)
	}
	s.Close()
	reopened, err := Open(root, Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	protected := waitRestoreProtectedFixture(t, reopened)
	if protected.Stage != "workspace-recovery" {
		t.Fatal(protected)
	}
	ref, _ := dataReference(e.ID)
	dir := filepath.Join(root, filepath.FromSlash(ref))
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Fatal("bootstrap created new directory after rolling back absence", err)
	}
	if _, err := reopened.db.Exec("DROP TRIGGER synthetic_late_bootstrap_failure"); err != nil {
		t.Fatal(err)
	}
	reopened.Close()
	again, err := Open(root, Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer again.Close()
	final := waitBackupFixture(t, again, op.ID)
	if !final.RestoreReport.RolledBack || final.RestoreReport.Protected {
		t.Fatal("repeated startup could not recover absent old tree", final)
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Fatal("second startup created browser data", err)
	}
	value[Preview](t, call(again, "Environment.Preview", map[string]string{"kind": "create"}))
}

func assertOldRestoreFixture(t *testing.T, s *Service, root string, e Environment, operationID string) {
	t.Helper()
	final := waitBackupFixture(t, s, operationID)
	if !final.RestoreReport.RecoveredAfterRestart || !final.RestoreReport.RolledBack || final.RestoreReport.Protected || final.State != "failed" {
		t.Fatal(final)
	}
	ref, _ := dataReference(e.ID)
	data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(ref), "Cookies"))
	if err != nil || string(data) != "SYNTHETIC_CURRENT_BYTES" {
		t.Fatal("rollback data differs", err)
	}
	current, _, _, err := s.readEnvironment(e.ID)
	if err != nil || current.Seed != e.Seed || current.Note != e.Note || current.CoreID != e.CoreID {
		t.Fatal("rollback identity/config differs", err)
	}
	value[Preview](t, call(s, "Environment.Preview", map[string]string{"kind": "create"}))
}

func TestRestoreRestartRejectsMissingPreparedDecisionAndConflictingCommit(t *testing.T) {
	for _, damage := range []string{"missing-prepared", "false-prepared", "committed-without-baseline", "unknown-version", "unknown-tree"} {
		t.Run(damage, func(t *testing.T) {
			s, root, e, op, snapshot, original := protectedRestoreJournalFixture(t)
			if err := os.WriteFile(snapshot, original, 0600); err != nil {
				t.Fatal(err)
			}
			var encoded string
			if err := s.db.QueryRow("SELECT plan_json FROM restore_jobs WHERE operation_id=?", op.ID).Scan(&encoded); err != nil {
				t.Fatal(err)
			}
			var plan map[string]any
			if err := json.Unmarshal([]byte(encoded), &plan); err != nil {
				t.Fatal(err)
			}
			switch damage {
			case "missing-prepared":
				delete(plan, "prepared")
			case "false-prepared":
				plan["prepared"] = false
			case "unknown-version":
				plan["journalVersion"] = 999
			case "unknown-tree":
				plan["moves"].([]any)[0].(map[string]any)["previous"] = "profiles/unknown/user-data"
			case "committed-without-baseline":
				if _, err := s.db.Exec("UPDATE restore_jobs SET committed=1 WHERE operation_id=?", op.ID); err != nil {
					t.Fatal(err)
				}
			}
			changed, _ := json.Marshal(plan)
			if _, err := s.db.Exec("UPDATE restore_jobs SET plan_json=? WHERE operation_id=?", string(changed), op.ID); err != nil {
				t.Fatal(err)
			}
			s.Close() // Expected protected outcome; keep its persistent row untouched.
			reopened, err := Open(root, Options{})
			if reopened != nil {
				reopened.Close()
			}
			var problem *Error
			if !errors.As(err, &problem) || problem.Code != "RESTORE_INCOMPLETE" {
				t.Fatal("unsafe journal was accepted", err)
			}
			ref, _ := dataReference(e.ID)
			data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(ref), "Cookies"))
			if err != nil || string(data) != "SYNTHETIC_ARCHIVED_BYTES" {
				t.Fatal("invalid journal moved live tree", err)
			}
			old, err := os.ReadFile(filepath.Join(root, "backups", "restore", op.ID, "previous", e.ID, "user-data", "Cookies"))
			if err != nil || string(old) != "SYNTHETIC_CURRENT_BYTES" {
				t.Fatal("invalid journal changed retained tree", err)
			}
		})
	}
}

func TestRestoreRestartDamagedCopiesAndConfigurationStayProtectedUntilRepaired(t *testing.T) {
	for _, damage := range []string{"snapshot-missing", "snapshot-corrupt", "old-file-missing", "old-file-corrupt", "configuration-differs"} {
		t.Run(damage, func(t *testing.T) {
			s, root, e, op, snapshot, original := protectedRestoreJournalFixture(t)
			oldPath := filepath.Join(root, "backups", "restore", op.ID, "previous", e.ID, "user-data", "Cookies")
			if damage != "snapshot-corrupt" {
				if err := os.WriteFile(snapshot, original, 0600); err != nil {
					t.Fatal(err)
				}
			}
			switch damage {
			case "snapshot-missing":
				if err := os.Remove(snapshot); err != nil {
					t.Fatal(err)
				}
			case "old-file-missing":
				if err := os.Remove(oldPath); err != nil {
					t.Fatal(err)
				}
			case "old-file-corrupt":
				if err := os.WriteFile(oldPath, []byte("SYNTHETIC_CORRUPT"), 0600); err != nil {
					t.Fatal(err)
				}
			case "configuration-differs":
				if _, err := s.db.Exec("UPDATE environments SET revision=revision+1 WHERE id=?", e.ID); err != nil {
					t.Fatal(err)
				}
			}
			s.Close()
			reopened, err := Open(root, Options{})
			if err != nil {
				t.Fatal(err)
			}
			defer reopened.Close()
			protected := waitRestoreProtectedFixture(t, reopened)
			if protected.Error == nil || protected.Error.Code != "RESTORE_INCOMPLETE" {
				t.Fatal(protected)
			}
			wantError(t, call(reopened, "Environment.Preview", map[string]string{"kind": "create"}), "RESTORE_INCOMPLETE")
			if err := os.WriteFile(snapshot, original, 0600); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(oldPath, []byte("SYNTHETIC_CURRENT_BYTES"), 0600); err != nil {
				t.Fatal(err)
			}
			if damage == "configuration-differs" {
				if _, err := reopened.db.Exec("UPDATE environments SET revision=revision-1 WHERE id=?", e.ID); err != nil {
					t.Fatal(err)
				}
			}
			value[Operation](t, call(reopened, "Backup.RecoverRestore", map[string]string{"operationId": op.ID}))
			assertOldRestoreFixture(t, reopened, root, e, op.ID)
		})
	}
}

func TestRestoreStartupStorageFailureNeverPersistsPrematureTerminal(t *testing.T) {
	s, root, e, op, snapshot, original := protectedRestoreJournalFixture(t)
	if err := os.WriteFile(snapshot, original, 0600); err != nil {
		t.Fatal(err)
	}
	s.Close()
	var deny atomic.Bool
	deny.Store(true)
	options := Options{BeforeCommit: func() error {
		if deny.Load() {
			return errors.New("SYNTHETIC_STORAGE_UNAVAILABLE")
		}
		return nil
	}}
	for attempt := 0; attempt < 2; attempt++ {
		reopened, err := Open(root, options)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { reopened.Close() })
		protected := waitRestoreProtectedFixture(t, reopened)
		if protected.Stage != "workspace-recovery" {
			t.Fatal("did not reach bootstrap write failure", protected)
		}
		var phase string
		if err := reopened.db.QueryRow("SELECT phase FROM restore_jobs WHERE operation_id=?", op.ID).Scan(&phase); err != nil {
			t.Fatal(err)
		}
		if phase == "rolled-back" || phase == "finalized" {
			t.Fatal("bootstrap persisted a premature terminal")
		}
		wantError(t, call(reopened, "Environment.Preview", map[string]string{"kind": "create"}), "RESTORE_INCOMPLETE")
		if attempt == 0 {
			if err := reopened.Close(); err == nil {
				t.Fatal("close concealed pending recovery")
			}
		} else {
			deny.Store(false)
			value[Operation](t, call(reopened, "Backup.RecoverRestore", map[string]string{"operationId": op.ID}))
			assertOldRestoreFixture(t, reopened, root, e, op.ID)
		}
	}
}

func TestRestoreBootstrapPartialLoaderFailureSurvivesReopenAndRetry(t *testing.T) {
	s, root, e, op, snapshot, original := protectedRestoreJournalFixture(t)
	if err := os.WriteFile(snapshot, original, 0600); err != nil {
		t.Fatal(err)
	}
	kernelOp := Operation{ID: id(), Kind: "kernel-install", State: "accepted", ResourceKey: id()}
	proxyOp := Operation{ID: id(), Kind: "proxy-check", State: "accepted"}
	for _, operation := range []Operation{kernelOp, proxyOp} {
		encoded, _ := json.Marshal(operation)
		if _, err := s.db.Exec("INSERT INTO operations(id,result_json) VALUES(?,?)", operation.ID, string(encoded)); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := s.db.Exec("CREATE TRIGGER synthetic_block_proxy_recovery BEFORE UPDATE ON operations WHEN OLD.id='" + proxyOp.ID + "' BEGIN SELECT RAISE(ABORT,'SYNTHETIC_STORAGE_FAILURE'); END"); err != nil {
		t.Fatal(err)
	}
	s.Close()
	for attempt := 0; attempt < 2; attempt++ {
		reopened, err := Open(root, Options{})
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { reopened.Close() })
		protected := waitRestoreProtectedFixture(t, reopened)
		if protected.Stage != "workspace-recovery" {
			t.Fatal("loader failure lost bootstrap", protected)
		}
		prior := value[Operation](t, call(reopened, "Operation.Read", map[string]string{"operationId": kernelOp.ID}))
		if prior.State != "failed" {
			t.Fatal("earlier loader did not finish")
		}
		wantError(t, call(reopened, "Environment.Preview", map[string]string{"kind": "create"}), "RESTORE_INCOMPLETE")
		if attempt == 0 {
			reopened.Close()
			continue
		}
		if _, err := reopened.db.Exec("DROP TRIGGER synthetic_block_proxy_recovery"); err != nil {
			t.Fatal(err)
		}
		value[Operation](t, call(reopened, "Backup.RecoverRestore", map[string]string{"operationId": op.ID}))
		assertOldRestoreFixture(t, reopened, root, e, op.ID)
		after := value[Operation](t, call(reopened, "Operation.Read", map[string]string{"operationId": proxyOp.ID}))
		if after.State != "failed" || after.Error == nil || after.Error.Code != "APPLICATION_INTERRUPTED" {
			t.Fatal(after)
		}
	}
}
