package workspace

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"errors"
	"strings"
	"sync/atomic"
	"testing"
)

func TestShutdownRetriesOriginalObservationBeforeClosingDatabase(t *testing.T) {
	s, root := fixture(t, Options{})
	environment, _ := create(t, s, "Synthetic exit persistence retry")
	op := Operation{ID: id(), Kind: "cookie-import", EnvironmentID: environment.ID, State: "completed", Stage: "finished"}
	encoded, _ := json.Marshal(op)
	if _, err := s.db.Exec("INSERT INTO operations(id,result_json) VALUES(?,?)", op.ID, string(encoded)); err != nil {
		t.Fatal(err)
	}
	s.cookiePending[op.ID] = op
	if _, err := s.db.Exec(`CREATE TRIGGER synthetic_exit_write_failure BEFORE UPDATE ON operations BEGIN SELECT RAISE(ABORT, 'synthetic disk failure'); END`); err != nil {
		t.Fatal(err)
	}
	err := s.Close()
	var failure *ShutdownError
	if !errors.As(err, &failure) || failure.Code != "EXIT_STORAGE_PENDING" || failure.CanExit {
		t.Fatal("unpersisted observation was allowed to exit", err)
	}
	if s.db.Ping() != nil || len(s.cookiePending) != 1 || !s.profileUses[environment.ID] {
		t.Fatal("shutdown discarded the database or original observation protection")
	}
	if result := call(s, "Workspace.Read", struct{}{}); result.OK || result.Error.Code != "NATIVE_UNAVAILABLE" {
		t.Fatal("retained database resumed ordinary business")
	}
	if _, err := s.db.Exec("DROP TRIGGER synthetic_exit_write_failure"); err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal("cleanup retry did not save the original observation", err)
	}
	if s.db.Ping() == nil || len(s.cookiePending) != 0 {
		t.Fatal("confirmed cleanup did not close storage after the retained write")
	}
	reopened, err := Open(root, Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	var stored string
	if err := reopened.db.QueryRow("SELECT result_json FROM operations WHERE id=?", op.ID).Scan(&stored); err != nil {
		t.Fatal(err)
	}
	var observed Operation
	if json.Unmarshal([]byte(stored), &observed) != nil || observed.State != "completed" || observed.PersistencePending || observed.ID != op.ID {
		t.Fatal("retry rewrote or lost the original outcome")
	}
}

func shutdownPendingAcceptanceFixture(t *testing.T, kind string, persisted bool) (*Service, func() (bool, bool)) {
	t.Helper()
	if kind == "migration" && persisted {
		s, _, environment, kernelID := migrationFixture(t)
		task := stagedMigrationFixture(t, s, environment, kernelID)
		task.acceptancePending = true
		return s, func() (bool, bool) { return task.acceptancePending, task.running }
	}
	s, _ := fixture(t, Options{})
	op := Operation{ID: id(), Kind: kind, State: "accepted", Stage: "accepted"}
	requestID, signature := id(), strings.Repeat("1", 64)
	if kind == "restore" {
		op.Kind = "backup-restore"
		op.RestoreReport = &RestoreReport{Mode: "native", RequestID: requestID}
	} else if kind == "recycle" {
		op.RecycleReport = &RecycleReport{Mode: "native", RequestID: requestID}
	} else {
		op.MigrationReport = &MigrationReport{Mode: "native", RequestID: requestID}
	}
	encoded, _ := json.Marshal(op)
	receipt, _ := json.Marshal(success(map[string]any{"status": "accepted", "operation": op}, op.ID))
	if persisted {
		if _, err := s.db.Exec("INSERT INTO operations(id,result_json) VALUES(?,?)", op.ID, string(encoded)); err != nil {
			t.Fatal(err)
		}
		if _, err := s.db.Exec("INSERT INTO requests(id,signature,result_json) VALUES(?,?,?)", requestID, signature, string(receipt)); err != nil {
			t.Fatal(err)
		}
	}
	if kind == "restore" {
		plan := restorePlan{ID: op.ID}
		planJSON, _ := json.Marshal(plan)
		task := &restoreTask{plan: plan, operation: op, phase: "accepted", acceptancePending: true, signature: signature, acceptanceOperation: string(encoded)}
		s.restoreTask = task
		if persisted {
			if _, err := s.db.Exec("INSERT INTO restore_jobs(operation_id,plan_json,phase,request_id,signature) VALUES(?,?,?,?,?)", op.ID, string(planJSON), "accepted", requestID, signature); err != nil {
				t.Fatal(err)
			}
		}
		return s, func() (bool, bool) { return task.acceptancePending, task.running }
	}
	if kind == "recycle" {
		plan := recyclePlan{ID: op.ID}
		planJSON, _ := json.Marshal(plan)
		task := &recycleTask{plan: plan, operation: op, phase: "accepted", acceptancePending: true, signature: signature, acceptedJSON: string(encoded)}
		s.recycleTask = task
		if persisted {
			if _, err := s.db.Exec("INSERT INTO recycle_jobs(operation_id,plan_json,plan_sha256,phase,request_id,signature) VALUES(?,?,?,?,?,?)", op.ID, string(planJSON), recyclePlanHash(planJSON), "accepted", requestID, signature); err != nil {
				t.Fatal(err)
			}
		}
		return s, func() (bool, bool) { return task.acceptancePending, task.running }
	}
	task := &migrationTask{plan: migrationPlan{ID: op.ID}, operation: op, phase: "accepted", acceptancePending: true}
	s.migrationTask = task
	return s, func() (bool, bool) { return task.acceptancePending, task.running }
}

func TestShutdownConfirmsOriginalPersistedAcceptanceWithoutScheduling(t *testing.T) {
	for _, kind := range []string{"restore", "recycle", "migration"} {
		t.Run(kind, func(t *testing.T) {
			s, state := shutdownPendingAcceptanceFixture(t, kind, true)
			var failure *ShutdownError
			if err := s.Close(); !errors.As(err, &failure) || failure.Code != "EXIT_RECOVERY_REQUIRED" || !failure.CanExit {
				t.Fatal("confirmed durable acceptance did not allow an explicit incomplete exit", err)
			}
			if pending, running := state(); pending || running {
				t.Fatal("shutdown either kept acceptance unknown or launched a new worker")
			}
			if s.db.Ping() == nil {
				t.Fatal("confirmed durable protection left ordinary storage open")
			}
		})
	}
}

func TestShutdownConfirmsOriginalNonAcceptanceWithoutScheduling(t *testing.T) {
	for _, kind := range []string{"restore", "recycle", "migration"} {
		t.Run(kind, func(t *testing.T) {
			s, state := shutdownPendingAcceptanceFixture(t, kind, false)
			if err := s.Close(); err != nil {
				t.Fatal("known absent original acceptance prevented clean shutdown", err)
			}
			if _, running := state(); running || s.restoreTask != nil || s.recycleTask != nil || s.migrationTask != nil {
				t.Fatal("nonaccepted request was retained or scheduled")
			}
		})
	}
}

func TestShutdownUnconfirmedReceiptRetainsOriginalAndCanRetry(t *testing.T) {
	s, state := shutdownPendingAcceptanceFixture(t, "restore", false)
	opID := s.restoreTask.plan.ID
	if _, err := s.db.Exec("INSERT INTO operations(id,result_json) VALUES(?,'{}')", opID); err != nil {
		t.Fatal(err)
	}
	var failure *ShutdownError
	if err := s.Close(); !errors.As(err, &failure) || failure.Code != "EXIT_STORAGE_PENDING" || failure.CanExit {
		t.Fatal("partial original receipt was treated as confirmed or disposable", err)
	}
	if pending, running := state(); !pending || running || s.db.Ping() != nil {
		t.Fatal("unknown acceptance lost original identity or its retry store")
	}
	if _, err := s.db.Exec("DELETE FROM operations WHERE id=?", opID); err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil || s.restoreTask != nil {
		t.Fatal("later original nonacceptance confirmation could not finish shutdown", err)
	}
}

type shutdownFailingDriver struct{ closed *atomic.Int32 }
type shutdownFailingConnection struct{ closed *atomic.Int32 }

func (d shutdownFailingDriver) Open(string) (driver.Conn, error) {
	return shutdownFailingConnection{closed: d.closed}, nil
}
func (c shutdownFailingConnection) Prepare(string) (driver.Stmt, error) {
	return nil, errors.New("synthetic close-only connection")
}
func (c shutdownFailingConnection) Begin() (driver.Tx, error) {
	return nil, errors.New("synthetic close-only connection")
}
func (c shutdownFailingConnection) Ping(context.Context) error { return nil }
func (c shutdownFailingConnection) Close() error {
	c.closed.Add(1)
	return errors.New("synthetic driver close unconfirmed")
}

func TestShutdownRetainsDriverCloseFailureInsteadOfFalseRetry(t *testing.T) {
	var closed atomic.Int32
	name := "synthetic-shutdown-close-" + id()
	sql.Register(name, shutdownFailingDriver{closed: &closed})
	db, err := sql.Open(name, "")
	if err != nil || db.Ping() != nil {
		t.Fatal("could not prepare synthetic driver close")
	}
	s := &Service{db: db, closeDone: make(chan struct{})}
	err = s.Close()
	var failure *ShutdownError
	if !errors.As(err, &failure) || failure.Code != "EXIT_CLOSE_FAILED" || !failure.CanExit || closed.Load() != 1 {
		t.Fatal("driver close error was not an explicitly incomplete exit", err)
	}
	if again := s.Close(); again != err || closed.Load() != 1 {
		t.Fatal("idempotent DB.Close washed out or pretended to retry the original error", again)
	}
	if s.db.Ping() == nil {
		t.Fatal("logically closed service resumed database business")
	}
}
