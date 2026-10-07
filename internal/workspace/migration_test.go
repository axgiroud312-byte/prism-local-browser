//go:build windows

package workspace

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/axgiroud312-byte/prism-local-browser/internal/backup"
	"github.com/axgiroud312-byte/prism-local-browser/internal/kernel"
)

func migrationFixture(t *testing.T) (*Service, string, Environment, string) {
	t.Helper()
	archive := syntheticArchive(t)
	s, root := fixture(t, Options{ChooseArchive: func() (string, error) { return archive, nil }, PrepareKernel: migrationKernelPrepare})
	first, _ := installKernel(t, s)
	old := waitKernel(t, s, first.ID)
	if old.State != "completed" {
		t.Fatal(old)
	}
	oldID := old.KernelID
	e := createFingerprint(t, s, oldID)
	op, _ := installKernel(t, s)
	installed := waitKernel(t, s, op.ID)
	if installed.State != "completed" {
		t.Fatal(installed)
	}
	return s, root, e, installed.KernelID
}

// Restore the actual old schema shape, rather than relabelling new local
// journals as an old version. Shared by earlier schema-upgrade regressions.
func stripMaintenanceSchema(t *testing.T, s *Service) {
	t.Helper()
	for _, statement := range []string{"DROP TABLE migration_kernel_refs", "DROP TABLE kernel_migrations", "DROP TABLE kernel_default", "DROP TABLE recycle_jobs", "DROP TABLE environment_trash"} {
		if _, err := s.db.Exec(statement); err != nil {
			t.Fatal(err)
		}
	}
}

// A stopped synthetic journal fixture exercises persistence and directory
// recovery without executing fake binaries or calling that a real trial.
func stagedMigrationFixture(t *testing.T, s *Service, e Environment, newID string) *migrationTask {
	t.Helper()
	p := value[MigrationPreview](t, call(s, "Migration.Preview", map[string]string{"environmentId": e.ID, "kernelId": newID}))
	plan := s.migrationDraft.plan
	request := MigrationRequest{PreviewID: p.PreviewID, Confirm: true, RequestID: id()}
	op := Operation{ID: plan.ID, Kind: "migration", State: "running", Stage: "accepted", EnvironmentID: e.ID, Total: 1, CompletedIDs: []string{}, MigrationReport: &MigrationReport{Mode: "native", RequestID: request.RequestID, PreviewID: p.PreviewID, EnvironmentID: e.ID, OldKernelID: e.CoreID, NewKernelID: newID, Seed: e.Seed, Sequence: 1, Protected: true}}
	b, _ := json.Marshal(op)
	receipt, _ := json.Marshal(success(map[string]any{"status": "accepted", "operation": op}, op.ID))
	tx, err := s.db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	for _, q := range []struct {
		sql  string
		args []any
	}{
		{"INSERT INTO operations(id,result_json) VALUES(?,?)", []any{op.ID, string(b)}},
		{"INSERT INTO requests(id,signature,result_json) VALUES(?,?,?)", []any{request.RequestID, migrationSignature(request), string(receipt)}},
		{"INSERT INTO kernel_migrations(operation_id,plan_json,plan_sha256,phase,request_id,signature) VALUES(?,'','','accepted',?,?)", []any{op.ID, request.RequestID, migrationSignature(request)}},
		{"INSERT INTO migration_kernel_refs(operation_id,kernel_id) VALUES(?,?),(?,?)", []any{op.ID, e.CoreID, op.ID, newID}},
	} {
		if _, err = tx.Exec(q.sql, q.args...); err != nil {
			t.Fatal(err)
		}
	}
	if err = saveMigration(tx, plan, op, "accepted"); err != nil {
		t.Fatal(err)
	}
	if err = tx.Commit(); err != nil {
		t.Fatal(err)
	}
	task := &migrationTask{plan: plan, operation: op, phase: "accepted", stop: make(chan struct{}, 1)}
	s.migrationTask = task
	s.migrationDraft = nil
	return task
}

func TestKernelDefaultOnlyChangesFutureDraftsAndProtectsDeletion(t *testing.T) {
	s, _, e, newID := migrationFixture(t)
	oldDraft := preview(t, s, "create", "")
	d := view(t, s).DefaultKernel
	request := KernelDefaultRequest{KernelID: newID, ExpectedRevision: d.Revision, RequestID: id()}
	selected := value[KernelDefault](t, call(s, "Kernel.SetDefault", request))
	if selected.KernelID != newID || selected.Revision != d.Revision+1 {
		t.Fatal(selected)
	}
	if replay := value[KernelDefault](t, call(s, "Kernel.SetDefault", request)); replay != selected {
		t.Fatal("default retry duplicated revision")
	}
	p := preview(t, s, "create", "")
	if p.Environment.CoreID != newID || p.Fingerprint == nil || p.Fingerprint.PreviewProfile.KernelID != newID {
		t.Fatal("future draft did not compile explicit default")
	}
	if s.drafts[oldDraft.PreviewID].Preview.Environment.CoreID != PendingKernelID {
		t.Fatal("existing draft changed")
	}
	actual, _, _, err := s.readEnvironment(e.ID)
	if err != nil || actual.CoreID != e.CoreID || actual.Seed != e.Seed {
		t.Fatal("existing environment upgraded")
	}
	if result := call(s, "Kernel.Delete", map[string]string{"kernelId": newID, "requestId": id()}); result.OK || result.Error.Code != "PROFILE_BUSY" {
		t.Fatal(result)
	}
	if result := call(s, "Kernel.SetDefault", KernelDefaultRequest{KernelID: e.CoreID, ExpectedRevision: d.Revision, RequestID: id()}); result.OK || result.Error.Code != "REVISION_CONFLICT" {
		t.Fatal(result)
	}
}

func TestMigrationPreviewKeepsIdentityAndRejectsBusyOrMissingBuild(t *testing.T) {
	s, _, e, newID := migrationFixture(t)
	before := view(t, s)
	p := value[MigrationPreview](t, call(s, "Migration.Preview", map[string]string{"environmentId": e.ID, "kernelId": newID}))
	if p.Before.Seed != e.Seed || p.After.Seed != e.Seed || p.After.ConfigRevision != p.Before.ConfigRevision+1 || p.After.KernelID != newID {
		t.Fatal(p)
	}
	if !reflect.DeepEqual(before, view(t, s)) {
		t.Fatal("read-only migration preview mutated workspace")
	}
	release, err := s.AcquireProfileUse(e.ID)
	if err != nil {
		t.Fatal(err)
	}
	r := call(s, "Migration.Prepare", MigrationRequest{PreviewID: p.PreviewID, Confirm: true, RequestID: id()})
	release()
	if r.OK || r.Error.Code != "PROFILE_BUSY" {
		t.Fatal("busy source accepted", r)
	}
	if _, err = s.db.Exec("UPDATE kernels SET status='missing' WHERE id=?", newID); err != nil {
		t.Fatal(err)
	}
	r = call(s, "Migration.Prepare", MigrationRequest{PreviewID: p.PreviewID, Confirm: true, RequestID: id()})
	if r.OK {
		t.Fatal("missing exact build accepted")
	}
}

func TestMigrationBackupAndCopyPreserveOriginalAndStrictRollbackPackage(t *testing.T) {
	s, root, e, newID := migrationFixture(t)
	ref, _ := dataReference(e.ID)
	source := filepath.Join(root, filepath.FromSlash(ref))
	if err := os.MkdirAll(source, 0700); err != nil {
		t.Fatal(err)
	}
	old := []byte("SYNTHETIC_BEFORE_MIGRATION")
	if err := os.WriteFile(filepath.Join(source, "Cookies"), old, 0600); err != nil {
		t.Fatal(err)
	}
	task := stagedMigrationFixture(t, s, e, newID)
	if err := s.prepareMigrationBackup(context.Background(), task); err != nil {
		t.Fatal(err)
	}
	if !task.plan.BackupVerified || !backup.Hash(task.plan.BackupSHA256) {
		t.Fatal("unverified backup")
	}
	if err := s.prepareMigrationCopy(context.Background(), task); err != nil {
		t.Fatal(err)
	}
	work := filepath.Join(root, filepath.FromSlash(task.plan.Move.Incoming))
	if err := os.WriteFile(filepath.Join(work, "Cookies"), []byte("SYNTHETIC_UPGRADED_COPY"), 0600); err != nil {
		t.Fatal(err)
	}
	if actual, err := os.ReadFile(filepath.Join(source, "Cookies")); err != nil || string(actual) != string(old) {
		t.Fatal("trial copy shared original bytes")
	}
	if result := call(s, "Environment.Preview", map[string]string{"kind": "create"}); result.OK || result.Error.Code != "MIGRATION_INCOMPLETE" {
		t.Fatal("migration writer barrier absent")
	}
	s.recoverMigration(context.Background(), task)
	if s.migrationTask != nil {
		t.Fatal("prelaunch cancellation failed to retain old state")
	}
	selected := value[struct {
		SourceToken string `json:"sourceToken"`
	}](t, call(s, "Migration.SelectRollback", map[string]string{"operationId": task.plan.ID}))
	p := value[RestorePreview](t, call(s, "Backup.PreviewRestore", map[string]string{"sourceToken": selected.SourceToken}))
	if p.EnvironmentCount != 1 || p.OverwriteCount != 1 || p.ArchiveSHA256 != task.plan.BackupSHA256 {
		t.Fatal("rollback package/identity differs", p)
	}
}

func TestMigrationInterruptedLaunchWithoutMetadataCanRecover(t *testing.T) {
	for _, lock := range []string{"missing", "empty", "previous-session"} {
		t.Run(lock, func(t *testing.T) {
			s, root, e, newID := migrationFixture(t)
			task := stagedMigrationFixture(t, s, e, newID)
			if err := s.prepareMigrationBackup(context.Background(), task); err != nil {
				t.Fatal(err)
			}
			if err := s.prepareMigrationCopy(context.Background(), task); err != nil {
				t.Fatal(err)
			}
			if lock != "missing" {
				data := []byte{}
				if lock == "previous-session" {
					data = []byte(`{"sessionId":"synthetic-previous"}`)
				}
				if err := os.WriteFile(filepath.Join(root, filepath.FromSlash(task.plan.Move.Incoming), ".prism-runtime.lock"), data, 0600); err != nil {
					t.Fatal(err)
				}
			}
			if err := s.migrationUpdate(task, "trial-starting", func(p *migrationPlan, r *MigrationReport) { p.LaunchPermitted = true; p.SessionID = id() }); err != nil {
				t.Fatal(err)
			}
			s.recoverMigration(context.Background(), task)
			if s.migrationTask != nil {
				t.Fatal("accurate missing Job and free owned copy did not recover")
			}
			actual, _, _, err := s.readEnvironment(e.ID)
			if err != nil || actual.CoreID != e.CoreID || actual.Seed != e.Seed {
				t.Fatal("interrupted launch changed original")
			}
		})
	}
}

func TestMigrationJournalRejectsAlteredDecisionWithoutUnlocking(t *testing.T) {
	s, _, e, newID := migrationFixture(t)
	task := stagedMigrationFixture(t, s, e, newID)
	if _, err := s.db.Exec("UPDATE kernel_migrations SET committed=1 WHERE operation_id=?", task.plan.ID); err != nil {
		t.Fatal(err)
	}
	s.recoverMigration(context.Background(), task)
	if s.migrationTask == nil || !s.migrationTask.operation.PersistencePending {
		t.Fatal("unknown DB decision unlocked")
	}
	var committed int
	if err := s.db.QueryRow("SELECT committed FROM kernel_migrations WHERE operation_id=?", task.plan.ID).Scan(&committed); err != nil || committed != 1 {
		t.Fatal("stale recovery overwrote disk decision")
	}
}

func TestMigrationFinalSaveFailureOnlyRetriesKnownObservation(t *testing.T) {
	s, _, e, newID := migrationFixture(t)
	task := stagedMigrationFixture(t, s, e, newID)
	s.options.BeforeCommit = func() error { return errors.New("synthetic final storage failure") }
	s.recoverMigration(context.Background(), task)
	if s.migrationTask == nil || s.migrationTask.finalPending == nil || !s.migrationTask.operation.PersistencePending {
		t.Fatal("unsaved terminal result released protection")
	}
	s.options.BeforeCommit = nil
	value[Operation](t, call(s, "Migration.Action", MigrationAction{OperationID: task.plan.ID, Action: "recover", Confirm: true}))
	if s.migrationTask != nil {
		t.Fatal("known terminal observation did not save")
	}
}

func TestMigrationNormalExitRequiresObservedZeroExitCode(t *testing.T) {
	for _, snapshot := range []kernel.RuntimeSnapshot{{ResourcesExited: true, ExitKnown: true, ExitCode: 1}, {ResourcesExited: true}, {ExitKnown: true, ExitCode: 0}} {
		if normalMigrationExit(snapshot) {
			t.Fatal("unknown/crashed trial considered normal")
		}
	}
	if !normalMigrationExit(kernel.RuntimeSnapshot{ResourcesExited: true, ExitKnown: true, ExitCode: 0}) {
		t.Fatal("normal full exit rejected")
	}
}

func preparedMigrationFixture(t *testing.T, s *Service, e Environment, newID string) *migrationTask {
	t.Helper()
	task := stagedMigrationFixture(t, s, e, newID)
	if err := s.prepareMigrationBackup(context.Background(), task); err != nil {
		t.Fatal(err)
	}
	if err := s.prepareMigrationCopy(context.Background(), task); err != nil {
		t.Fatal(err)
	}
	work := filepath.Join(s.root, filepath.FromSlash(task.plan.Move.Incoming))
	if err := os.WriteFile(filepath.Join(work, "Cookies"), []byte("SYNTHETIC_NEW_SIDE"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(work, ".prism-runtime.lock"), []byte{}, 0600); err != nil {
		t.Fatal(err)
	}
	profile, err := backup.CaptureProfile(context.Background(), s.root, task.plan.WorkID, task.plan.Move.Incoming, false, false)
	if err != nil {
		t.Fatal(err)
	}
	files, err := profile.Inventory(context.Background())
	profile.Close()
	if err != nil {
		t.Fatal(err)
	}
	if err = s.migrationUpdate(task, "prepared", func(p *migrationPlan, r *MigrationReport) {
		p.Prepared = true
		p.TrialExited = true
		r.TrialExited = true
		p.Move.NewFiles = files
		p.Move.NewIdentity = p.WorkIdentity
	}); err != nil {
		t.Fatal(err)
	}
	return task
}

func TestMigrationEveryUncommittedDirectoryCutRecoversOriginalCompleteSide(t *testing.T) {
	for _, present := range []bool{true, false} {
		for _, cut := range []string{"prepared", "old-retained", "new-installed"} {
			t.Run(fmt.Sprintf("%t/%s", present, cut), func(t *testing.T) {
				s, root, e, newID := migrationFixture(t)
				ref, _ := dataReference(e.ID)
				source := filepath.Join(root, filepath.FromSlash(ref))
				if present {
					if err := os.MkdirAll(source, 0700); err != nil {
						t.Fatal(err)
					}
					if err := os.WriteFile(filepath.Join(source, "Cookies"), []byte("SYNTHETIC_OLD_SIDE"), 0600); err != nil {
						t.Fatal(err)
					}
				}
				task := preparedMigrationFixture(t, s, e, newID)
				m := task.plan.Move
				before := view(t, s).Fingerprints[e.ID]
				if err := os.MkdirAll(filepath.Dir(source), 0700); err != nil {
					t.Fatal(err)
				}
				if cut != "prepared" && present {
					if err := backup.MoveTree(context.Background(), root, m.Live, m.Previous, m.OldIdentity, m.OldFiles); err != nil {
						t.Fatal(err)
					}
				}
				if cut == "new-installed" {
					if err := backup.MoveTree(context.Background(), root, m.Incoming, m.Live, m.NewIdentity, m.NewFiles); err != nil {
						t.Fatal(err)
					}
				}
				s.recoverMigration(context.Background(), task)
				if s.migrationTask != nil {
					t.Fatal("uncommitted directory side remained protected", task.operation)
				}
				if !reflect.DeepEqual(before, view(t, s).Fingerprints[e.ID]) {
					t.Fatal("directory rollback changed profile")
				}
				if present {
					data, err := os.ReadFile(filepath.Join(source, "Cookies"))
					if err != nil || string(data) != "SYNTHETIC_OLD_SIDE" {
						t.Fatal("old complete side not recovered")
					}
				} else {
					if _, err := os.Stat(source); !os.IsNotExist(err) {
						t.Fatal("absent old profile was invented")
					}
				}
				if err := backup.VerifyTree(context.Background(), root, m.Incoming, m.NewIdentity, m.NewFiles); err != nil {
					t.Fatal("new work side lost instead of retained", err)
				}
			})
		}
	}
}

func TestMigrationConfigurationAndDecisionCommitTogether(t *testing.T) {
	for _, reject := range []bool{true, false} {
		t.Run(fmt.Sprint(reject), func(t *testing.T) {
			s, _, e, newID := migrationFixture(t)
			task := preparedMigrationFixture(t, s, e, newID)
			if reject {
				s.options.BeforeCommit = func() error { return errors.New("synthetic configuration transaction failure") }
			}
			s.mu.Lock()
			err := s.commitMigrationConfiguration(task)
			s.mu.Unlock()
			s.options.BeforeCommit = nil
			if (err != nil) != reject {
				t.Fatal("wrong injected transaction outcome", err)
			}
			plan, op, _, readErr := s.readMigrationJournal(task.plan.ID)
			if readErr != nil {
				t.Fatal(readErr)
			}
			current, revision, _, readErr := s.readEnvironment(e.ID)
			if readErr != nil {
				t.Fatal(readErr)
			}
			if plan.Committed != !reject || op.MigrationReport.Committed != !reject {
				t.Fatal("marker separated from configuration")
			}
			want := e.CoreID
			wantRevision := task.plan.Preview.ExpectedRevision
			if !reject {
				want = newID
				wantRevision++
			}
			if current.CoreID != want || current.Seed != e.Seed || revision != wantRevision {
				t.Fatal("configuration chose wrong decision side")
			}
		})
	}
}

type waitingMigrationProcess struct {
	*syntheticRuntimeProcess
	entered chan struct{}
}

func (p *waitingMigrationProcess) Stop(ctx context.Context) error {
	close(p.entered)
	<-ctx.Done()
	return ctx.Err()
}

func TestMigrationCancelDuringStopCleansOwnedProcessWithoutSecondRequest(t *testing.T) {
	s, _, e, newID := migrationFixture(t)
	task := stagedMigrationFixture(t, s, e, newID)
	process := &waitingMigrationProcess{syntheticRuntimeProcess: newSyntheticRuntimeProcess(), entered: make(chan struct{})}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	task.process, task.running, task.cancel = process, true, cancel
	done := make(chan struct{})
	go func() { defer close(done); s.stopMigrationTrial(ctx, task) }()
	select {
	case <-process.entered:
	case <-time.After(5 * time.Second):
		t.Fatal("Stop never entered")
	}
	value[Operation](t, call(s, "Operation.Cancel", map[string]string{"operationId": task.plan.ID}))
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("one cancellation did not finish cleanup")
	}
	if process.closeCalls.Load() != 1 || !process.Snapshot().ResourcesExited || s.migrationTask != nil {
		t.Fatal("cancel stranded owned Job or maintenance", task.operation)
	}
	final := value[Operation](t, call(s, "Operation.Read", map[string]string{"operationId": task.plan.ID}))
	if final.State != "cancelled" || final.PersistencePending || final.MigrationReport.Committed {
		t.Fatal(final)
	}
}

func TestMigrationOrdinaryStopTimeoutRetainsProcessUntilExplicitCancel(t *testing.T) {
	s, _, e, newID := migrationFixture(t)
	task := stagedMigrationFixture(t, s, e, newID)
	process := newSyntheticRuntimeProcess()
	process.stopError = context.DeadlineExceeded
	task.process, task.running = process, true
	s.stopMigrationTrial(context.Background(), task)
	if process.closeCalls.Load() != 0 || s.migrationTask != task || !task.operation.PersistencePending || task.running {
		t.Fatal("ordinary timeout forced or unlocked trial")
	}
	value[Operation](t, call(s, "Operation.Cancel", map[string]string{"operationId": task.plan.ID}))
	s.workers.Wait()
	if process.closeCalls.Load() != 1 || s.migrationTask != nil {
		t.Fatal("explicit cancellation did not converge")
	}
}

func TestMigrationCancellationAtReadyHandoffDoesNotStrandMaintenance(t *testing.T) {
	s, _, e, newID := migrationFixture(t)
	task := stagedMigrationFixture(t, s, e, newID)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	process := newSyntheticRuntimeProcess()
	task.process, task.running, task.cancel = process, true, cancel
	// The durable ready update succeeds, but cancellation arrives before the
	// worker can publish its idle handoff under the service lock.
	s.options.BeforeCommit = func() error { cancel(); return nil }
	s.stopMigrationTrial(ctx, task)
	s.options.BeforeCommit = nil
	if s.migrationTask != nil || process.closeCalls.Load() != 1 {
		t.Fatal("cancelled ready handoff stayed blocked")
	}
	final := value[Operation](t, call(s, "Operation.Read", map[string]string{"operationId": task.plan.ID}))
	if final.State != "cancelled" || final.PersistencePending {
		t.Fatal(final)
	}
}

func TestMigrationBootstrapFailureKeepsJournalUnfinishedAndRetryLoadsRemainingRecords(t *testing.T) {
	s, _, e, newID := migrationFixture(t)
	task := stagedMigrationFixture(t, s, e, newID)
	proxyOp := Operation{ID: id(), Kind: "proxy-check", State: "accepted"}
	encoded, _ := json.Marshal(proxyOp)
	if _, err := s.db.Exec("INSERT INTO operations(id,result_json) VALUES(?,?)", proxyOp.ID, string(encoded)); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.Exec("CREATE TRIGGER synthetic_migration_bootstrap_failure BEFORE UPDATE ON operations WHEN OLD.id='" + proxyOp.ID + "' BEGIN SELECT RAISE(ABORT,'SYNTHETIC_STORAGE_FAILURE'); END"); err != nil {
		t.Fatal(err)
	}
	task.startup = true
	s.recoverMigration(context.Background(), task)
	if s.migrationTask != task || !task.operation.PersistencePending || task.finalPending != nil || task.bootstrapReady {
		t.Fatal("failed bootstrap retained a stale final observation")
	}
	_, _, phase, err := s.readMigrationJournal(task.plan.ID)
	if err != nil || phase == "finished" {
		t.Fatal("bootstrap prematurely finalized journal", err)
	}
	if _, err = s.db.Exec("DROP TRIGGER synthetic_migration_bootstrap_failure"); err != nil {
		t.Fatal(err)
	}
	value[Operation](t, call(s, "Migration.Action", MigrationAction{OperationID: task.plan.ID, Action: "recover", Confirm: true}))
	s.workers.Wait()
	if s.migrationTask != nil || !task.bootstrapReady {
		t.Fatal("bootstrap recovery did not retry")
	}
	recovered := value[Operation](t, call(s, "Operation.Read", map[string]string{"operationId": proxyOp.ID}))
	if recovered.State == "accepted" || recovered.State == "running" {
		t.Fatal("remaining startup record never loaded")
	}
}

func TestMigrationUnknownCommitAndUnavailableJournalCannotRollBackCommittedDirectory(t *testing.T) {
	s, root, e, newID := migrationFixture(t)
	ref, _ := dataReference(e.ID)
	source := filepath.Join(root, filepath.FromSlash(ref))
	if err := os.MkdirAll(source, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(source, "Cookies"), []byte("SYNTHETIC_OLD_SIDE"), 0600); err != nil {
		t.Fatal(err)
	}
	task := preparedMigrationFixture(t, s, e, newID)
	s.options.CommitMigration = func(tx *sql.Tx) error {
		var committed bool
		if err := tx.QueryRow("SELECT committed FROM kernel_migrations WHERE operation_id=?", task.plan.ID).Scan(&committed); err != nil {
			return err
		}
		if err := tx.Commit(); err != nil {
			return err
		}
		if committed {
			// Actual configuration COMMIT succeeded, but its acknowledgement and
			// immediate journal read both fail. Preserve the real decision bytes.
			if _, err := s.db.Exec("ALTER TABLE kernel_migrations RENAME TO synthetic_unavailable_migrations"); err != nil {
				return err
			}
			return errors.New("SYNTHETIC_LOST_COMMIT_ACK")
		}
		return nil
	}
	s.commitMigration(context.Background(), task)
	s.options.CommitMigration = nil
	if s.migrationTask != task || !task.operation.PersistencePending || task.plan.Committed || task.finalPending != nil {
		t.Fatal("unknown commit released/rewrote cached old decision")
	}
	var committed bool
	if err := s.db.QueryRow("SELECT committed FROM synthetic_unavailable_migrations WHERE operation_id=?", task.plan.ID).Scan(&committed); err != nil || !committed {
		t.Fatal("real committed marker lost", err)
	}
	if data, err := os.ReadFile(filepath.Join(source, "Cookies")); err != nil || string(data) != "SYNTHETIC_NEW_SIDE" {
		t.Fatal("unknown result rolled back already committed directory")
	}
	if _, err := s.db.Exec("ALTER TABLE synthetic_unavailable_migrations RENAME TO kernel_migrations"); err != nil {
		t.Fatal(err)
	}
	value[Operation](t, call(s, "Migration.Action", MigrationAction{OperationID: task.plan.ID, Action: "recover", Confirm: true}))
	s.workers.Wait()
	if s.migrationTask != nil {
		t.Fatal("readable committed journal did not finish")
	}
	actual, _, _, err := s.readEnvironment(e.ID)
	if err != nil || actual.CoreID != newID || actual.Seed != e.Seed {
		t.Fatal("configuration disagrees with recovered decision")
	}
	final := value[Operation](t, call(s, "Operation.Read", map[string]string{"operationId": task.plan.ID}))
	if final.State != "completed" || !final.MigrationReport.Committed || final.PersistencePending {
		t.Fatal(final)
	}
}
