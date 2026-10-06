//go:build windows

package workspace

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"modernc.org/sqlite"
)

func TestV1SafeGapRestoreActualSQLiteFullTransactionAndDirectoryRollback(t *testing.T) {
	requireSafeGapAcceptance(t)
	source, sourceRoot := fixture(t, Options{})
	e, _ := create(t, source, "Owned capacity import")
	p := preview(t, source, "edit", e.ID)
	p.Environment.Note = strings.Repeat("SYNTHETIC_CAPACITY_ONLY_", 100000)
	value[any](t, call(source, "Environment.Update", Mutation{PreviewID: p.PreviewID, Configuration: p.Environment.Configuration, ExpectedRevision: p.ExpectedRevision, RequestID: id()}))
	sourceDir := filepath.Join(sourceRoot, "environments", e.ID, "user-data")
	if err := os.MkdirAll(sourceDir, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sourceDir, "Cookies"), []byte("SYNTHETIC_CAPACITY_IMPORT_BYTES"), 0600); err != nil {
		t.Fatal(err)
	}
	archive := restorePackageFixture(t, source, []string{e.ID})
	entered, release := make(chan struct{}), make(chan struct{})
	var once sync.Once
	options := Options{RestoreCheckpoint: func(phase string) error {
		if phase == "before-db-commit" {
			close(entered)
			<-release
		}
		return nil
	}}
	s, root := fixture(t, options)
	unrelated, dir := recycleFixture(t, s, root, "Owned capacity untouched")
	beforeData, err := os.ReadFile(filepath.Join(dir, "Cookies"))
	if err != nil {
		t.Fatal(err)
	}
	_, accepted := acceptRestoreFixture(t, s, value[RestorePreview](t, previewPackageFixture(t, s, archive)))
	t.Cleanup(func() { once.Do(func() { close(release) }) })
	select {
	case <-entered:
	case <-time.After(15 * time.Second):
		t.Fatal("actual restore did not reach its configuration transaction boundary")
	}
	s.mu.Lock()
	var pages int64
	err = s.db.QueryRow("PRAGMA page_count").Scan(&pages)
	if err == nil {
		_, err = s.db.Exec("PRAGMA max_page_count=" + strconv.FormatInt(pages, 10))
	}
	if err != nil {
		s.mu.Unlock()
		t.Fatal(err)
	}
	// Inspect the real transaction's returned SQLite error while the worker is
	// held before the same transaction. No fake writer or error-return seam is
	// used. A failed transaction has no side effects and rolls back on return.
	encoded, err := json.Marshal(s.restoreTask.plan)
	var plan restorePlan
	if err == nil {
		err = json.Unmarshal(encoded, &plan)
	}
	if err != nil {
		s.mu.Unlock()
		t.Fatal(err)
	}
	actualFailure := s.commitRestoredConfiguration(s.restoreTask, &plan)
	var sqliteFailure *sqlite.Error
	if !errors.As(actualFailure, &sqliteFailure) || sqliteFailure.Code()&255 != 13 /* SQLITE_FULL */ {
		s.mu.Unlock()
		t.Fatal("the actual configuration transaction did not return SQLITE_FULL")
	}
	var committed int
	err = s.db.QueryRow("SELECT committed FROM restore_jobs WHERE operation_id=?", accepted.ID).Scan(&committed)
	s.mu.Unlock()
	if err != nil || committed != 0 {
		t.Fatal("actual full transaction left a committed restore marker")
	}
	once.Do(func() { close(release) })
	deadline := time.Now().Add(15 * time.Second)
	stopped := false
	for time.Now().Before(deadline) {
		s.mu.Lock()
		stopped = s.restoreTask == nil || !s.restoreTask.running
		s.mu.Unlock()
		if stopped {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if !stopped {
		t.Fatal("capacity-limited actual restore worker did not stop")
	}
	s.mu.Lock()
	err = s.db.QueryRow("SELECT committed FROM restore_jobs WHERE operation_id=?", accepted.ID).Scan(&committed)
	if err == nil {
		_, err = s.db.Exec("PRAGMA max_page_count=1073741823")
	}
	s.mu.Unlock()
	if err != nil || committed != 0 {
		t.Fatal("capacity-limited actual worker committed partial configuration")
	}
	value[Operation](t, call(s, "Backup.RecoverRestore", map[string]string{"operationId": accepted.ID}))
	final := waitLocalAcceptanceDurable(t, s, accepted.ID)
	if final.State != "failed" || !final.RestoreReport.RolledBack || final.RestoreReport.Protected || final.RestoreReport.Committed {
		t.Fatal("actual SQLite capacity failure did not converge to the complete old side after capacity repair")
	}
	if _, _, _, err := s.readEnvironment(e.ID); err == nil {
		t.Fatal("actual full transaction left a partial imported environment")
	}
	if _, err := os.Stat(filepath.Join(root, "environments", e.ID, "user-data")); !os.IsNotExist(err) {
		t.Fatal("uncommitted imported directory remained live instead of rolled back")
	}
	afterData, err := os.ReadFile(filepath.Join(dir, "Cookies"))
	current, _, _, readErr := s.readEnvironment(unrelated.ID)
	if err != nil || readErr != nil || string(afterData) != string(beforeData) || current.Seed != unrelated.Seed {
		t.Fatal("capacity failure changed the unrelated original side")
	}
	writeLocalAcceptanceEvidence(t, "restore-sqlite-capacity", map[string]any{"scope": safeGapScope, "capacityMechanism": "PRAGMA max_page_count in this fixture's real SQLite only; not NTFS disk full", "actualConfigurationTransactionErrorCode": sqliteFailure.Code(), "committedMarkerStayedZero": true, "actualWorkerRolledBackDirectoriesAndConfiguration": true, "capacityRestoredBeforeOriginalTaskFinalization": true, "noPartialImportedEnvironmentOrLiveDirectory": true, "unrelatedIdentityAndBrowserBytesUnchanged": true, "realSystemDiskFilled": false})
}
