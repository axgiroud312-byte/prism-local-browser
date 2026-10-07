//go:build windows

package workspace

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/axgiroud312-byte/prism-local-browser/internal/desktopbase"
	"golang.org/x/sys/windows"
)

type restoreCrashSignal struct {
	Phase       string `json:"phase"`
	OperationID string `json:"operationId"`
}

// Invoked only as a separate, test-owned process. The parent kills this exact
// Process handle at the signalled checkpoint; no panic/os.Exit simulates death.
func TestRestoreCrashHelper(t *testing.T) {
	if os.Getenv("PRISM_RESTORE_CRASH_CHILD") != "1" {
		t.Skip("private crash helper")
	}
	root, token := os.Getenv("PRISM_RESTORE_CRASH_ROOT"), os.Getenv("PRISM_RESTORE_CRASH_TOKEN")
	marker, err := os.ReadFile(filepath.Join(root, "synthetic-restore-test.token"))
	if err != nil || token == "" || string(marker) != token {
		t.Fatal("not a parent-owned synthetic workspace")
	}
	lock, err := desktopbase.Acquire(root)
	if err != nil {
		t.Fatal("synthetic workspace already owned")
	}
	defer lock.Close()
	phase, source := os.Getenv("PRISM_RESTORE_CRASH_PHASE"), os.Getenv("PRISM_RESTORE_CRASH_SOURCE")
	resumeID := os.Getenv("PRISM_RESTORE_CRASH_RESUME")
	block := make(chan struct{})
	var service *Service
	service, err = Open(root, Options{RestoreCheckpoint: func(current string) error {
		if current == phase {
			operationID := resumeID // Open may already be recovering before it returns.
			if operationID == "" {
				service.mu.Lock()
				operationID = service.restoreTask.plan.ID
				service.mu.Unlock()
			}
			signal := restoreCrashSignal{Phase: phase, OperationID: operationID}
			encoded, _ := json.Marshal(signal)
			fmt.Println("PRISM_RESTORE_CHECKPOINT " + string(encoded))
			<-block
		}
		return nil
	}})
	if err != nil {
		t.Fatal("synthetic workspace did not open")
	}
	defer service.Close()
	if resumeID == "" {
		p := value[RestorePreview](t, previewPackageFixture(t, service, source))
		acceptRestoreFixture(t, service, p)
	}
	<-block // The parent's TerminateProcess interrupts all goroutines and handles.
}

func crashRestoreFixture(t *testing.T, root, source, phase string, resume ...string) restoreCrashSignal {
	t.Helper()
	token := id()
	marker := filepath.Join(root, "synthetic-restore-test.token")
	file, err := os.OpenFile(marker, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = file.WriteString(token); err != nil {
		file.Close()
		t.Fatal(err)
	}
	if err = file.Close(); err != nil {
		t.Fatal(err)
	}
	defer os.Remove(marker)
	cmd := exec.Command(os.Args[0], "-test.run=^TestRestoreCrashHelper$", "-test.timeout=3m")
	cmd.Env = append(os.Environ(), "PRISM_RESTORE_CRASH_CHILD=1", "PRISM_RESTORE_CRASH_ROOT="+root, "PRISM_RESTORE_CRASH_SOURCE="+source, "PRISM_RESTORE_CRASH_TOKEN="+token, "PRISM_RESTORE_CRASH_PHASE="+phase)
	resumeID := ""
	if len(resume) != 0 {
		resumeID = resume[0]
	}
	cmd.Env = append(cmd.Env, "PRISM_RESTORE_CRASH_RESUME="+resumeID)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err = cmd.Start(); err != nil {
		t.Fatal("could not start owned crash helper")
	}
	waited := false
	defer func() {
		if !waited {
			cmd.Process.Kill()
			cmd.Wait()
		}
	}()
	signals := make(chan restoreCrashSignal, 1)
	go func() {
		scanner := bufio.NewScanner(stdout)
		for scanner.Scan() {
			line := scanner.Text()
			if strings.HasPrefix(line, "PRISM_RESTORE_CHECKPOINT ") {
				var signal restoreCrashSignal
				if json.Unmarshal([]byte(strings.TrimPrefix(line, "PRISM_RESTORE_CHECKPOINT ")), &signal) == nil {
					signals <- signal
					return
				}
			}
		}
		close(signals)
	}()
	var signal restoreCrashSignal
	select {
	case value, ok := <-signals:
		if !ok {
			t.Fatal("helper exited before its checkpoint; no hard-crash evidence")
		}
		signal = value
	case <-time.After(2 * time.Minute):
		t.Fatal("crash helper did not reach requested phase")
	}
	if signal.Phase != phase || signal.OperationID == "" {
		t.Fatal("checkpoint identity differs")
	}
	if resumeID != "" && signal.OperationID != resumeID {
		t.Fatal("recovery checkpoint changed the original operation")
	}
	if competing, err := desktopbase.Acquire(root); err == nil {
		competing.Close()
		t.Fatal("a live recovery host allowed a second workspace writer")
	}
	if err = cmd.Process.Kill(); err != nil {
		t.Fatal("owned helper could not be terminated")
	}
	err = cmd.Wait()
	waited = true
	if err == nil || cmd.ProcessState == nil || cmd.ProcessState.Success() {
		t.Fatal("expected an actual abnormal process exit")
	}
	lock, err := desktopbase.Acquire(root)
	if err != nil {
		t.Fatal("host maintenance lock remained owned after process death")
	}
	lock.Close()
	return signal
}

func restoreCrashWorkspace(t *testing.T) (string, string, []Environment) {
	t.Helper()
	s, root := fixture(t, Options{})
	environments := []Environment{}
	for _, name := range []string{"合成中断A", "合成中断B"} {
		e, _ := create(t, s, name)
		environments = append(environments, e)
		ref, _ := dataReference(e.ID)
		dir := filepath.Join(root, filepath.FromSlash(ref))
		if err := os.MkdirAll(filepath.Join(dir, "empty"), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "Cookies"), []byte("SYNTHETIC_ARCHIVED_"+e.ID), 0600); err != nil {
			t.Fatal(err)
		}
	}
	ids := []string{environments[0].ID, environments[1].ID}
	path := restorePackageFixture(t, s, ids)
	for _, e := range environments {
		ref, _ := dataReference(e.ID)
		if err := os.WriteFile(filepath.Join(root, filepath.FromSlash(ref), "Cookies"), []byte("SYNTHETIC_CURRENT_"+e.ID), 0600); err != nil {
			t.Fatal(err)
		}
		p := preview(t, s, "edit", e.ID)
		p.Environment.Note = "SYNTHETIC_CURRENT_" + e.ID
		value[any](t, call(s, "Environment.Update", Mutation{PreviewID: p.PreviewID, Configuration: p.Environment.Configuration, ExpectedRevision: p.ExpectedRevision, RequestID: id()}))
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	return root, path, environments
}

func assertRestoreCrashState(t *testing.T, s *Service, root string, environments []Environment, newState bool) {
	t.Helper()
	for _, e := range environments {
		ref, _ := dataReference(e.ID)
		want, note := "SYNTHETIC_CURRENT_"+e.ID, "SYNTHETIC_CURRENT_"+e.ID
		if newState {
			want, note = "SYNTHETIC_ARCHIVED_"+e.ID, e.Note
		}
		dir := filepath.Join(root, filepath.FromSlash(ref))
		data, err := os.ReadFile(filepath.Join(dir, "Cookies"))
		if err != nil || string(data) != want {
			t.Fatal("mixed or cross-environment browser bytes", err)
		}
		entries, err := os.ReadDir(dir)
		if err != nil {
			t.Fatal(err)
		}
		names := []string{}
		for _, entry := range entries {
			if entry.Name() != ".prism-runtime.lock" {
				names = append(names, entry.Name())
			}
		}
		if !reflect.DeepEqual(names, []string{"Cookies", "empty"}) {
			t.Fatal("restored file set differs", names)
		}
		empty, err := os.ReadDir(filepath.Join(dir, "empty"))
		if err != nil || len(empty) != 0 {
			t.Fatal("empty directory not preserved", err)
		}
		got, _, _, err := s.readEnvironment(e.ID)
		if err != nil || got.Seed != e.Seed || got.ID != e.ID || got.CoreID != e.CoreID || got.Note != note {
			t.Fatal("configuration and directory did not select the same side", err)
		}
		var savedRef string
		if err := s.db.QueryRow("SELECT user_data_ref FROM environments WHERE id=?", e.ID).Scan(&savedRef); err != nil || savedRef != ref {
			t.Fatal("reference changed", err)
		}
	}
}

// Keep the same desktop ownership boundary for the complete parent-side reopen.
func ownRestoreCrashRoot(t *testing.T, root string) {
	t.Helper()
	lock, err := desktopbase.Acquire(root)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { lock.Close() })
}

func waitRestoreProtectedFixture(t *testing.T, s *Service) Operation {
	t.Helper()
	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		s.mu.Lock()
		task := s.restoreTask
		if task != nil && !task.running && task.operation.RestoreReport.Protected {
			op := copyRestoreOperation(task.operation)
			s.mu.Unlock()
			return op
		}
		s.mu.Unlock()
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("restore did not remain protected")
	return Operation{}
}

func TestRestoreHardInterruptAutomaticallyFindsWholeState(t *testing.T) {
	if os.Getenv("PRISM_RESTORE_CRASH_VERIFY") != "1" {
		t.Skip("hard process interruption not explicitly selected")
	}
	evidence := []map[string]any{}
	for _, phase := range []string{"prepared", "old-retained", "new-switched", "before-db-commit", "db-committed"} {
		t.Run(phase, func(t *testing.T) {
			root, source, environments := restoreCrashWorkspace(t)
			signal := crashRestoreFixture(t, root, source, phase)
			if err := os.Remove(source); err != nil {
				t.Fatal(err)
			} // Automatic recovery never reimports the source.
			ownRestoreCrashRoot(t, root)
			s, err := Open(root, Options{})
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { s.Close() })
			final := waitBackupFixture(t, s, signal.OperationID)
			newState := phase == "db-committed"
			if !final.RestoreReport.RecoveredAfterRestart || final.RestoreReport.Protected || final.RestoreReport.Committed != newState || final.RestoreReport.RolledBack == newState {
				t.Fatal("wrong journal recovery decision", final)
			}
			if newState && final.State != "completed" || !newState && final.State != "failed" {
				t.Fatal("partial state reported complete", final)
			}
			assertRestoreCrashState(t, s, root, environments, newState)
			s.mu.Lock()
			unlocked := s.restoreTask == nil
			s.mu.Unlock()
			if !unlocked {
				t.Fatal("recovery terminal published before startup ready")
			}
			if err = s.Close(); err != nil {
				t.Fatal(err)
			}
			again, err := Open(root, Options{})
			if err != nil {
				t.Fatal(err)
			}
			defer again.Close()
			saved := value[Operation](t, call(again, "Operation.Read", map[string]string{"operationId": signal.OperationID}))
			if saved.RestoreReport.Sequence != final.RestoreReport.Sequence {
				t.Fatal("final journal replayed on second open")
			}
			assertRestoreCrashState(t, again, root, environments, newState)
			evidence = append(evidence, map[string]any{"checkpoint": phase, "operationId": signal.OperationID, "actualProcessKilled": true, "hostLockReacquired": true, "decisionNewState": newState, "allSyntheticDirectoryBytesMatched": true, "identityPreserved": true, "secondOpenIdempotent": true})
		})
	}
	if path := os.Getenv("PRISM_RESTORE_CRASH_EVIDENCE"); path != "" {
		encoded, _ := json.MarshalIndent(map[string]any{"verifiedAt": timestamp(), "scope": "test-owned native service helper; synthetic file bytes, not real browser storage", "uiClicks": "not-run", "checkpoints": evidence}, "", "  ")
		file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
		if err != nil {
			t.Fatal(err)
		}
		defer file.Close()
		if _, err := file.Write(append(encoded, '\n')); err != nil {
			t.Fatal(err)
		}
	}
}

func TestRestoreHardInterruptOccupiedRollbackStaysProtectedUntilExplicitRetry(t *testing.T) {
	if os.Getenv("PRISM_RESTORE_CRASH_VERIFY") != "1" {
		t.Skip("hard process interruption not explicitly selected")
	}
	root, source, environments := restoreCrashWorkspace(t)
	signal := crashRestoreFixture(t, root, source, "new-switched")
	previous := filepath.Join(root, "backups", "restore", signal.OperationID, "previous")
	entries, err := os.ReadDir(previous)
	if err != nil {
		t.Fatal(err)
	}
	path := ""
	for _, entry := range entries {
		candidate := filepath.Join(previous, entry.Name(), "user-data", "Cookies")
		if _, err := os.Stat(candidate); err == nil {
			if path != "" {
				t.Fatal("expected exactly one moved old tree")
			}
			path = candidate
		}
	}
	if path == "" {
		t.Fatal("retained old tree missing")
	}
	wide, _ := windows.UTF16PtrFromString(path)
	handle, err := windows.CreateFile(wide, windows.GENERIC_READ, 0, nil, windows.OPEN_EXISTING, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if handle != windows.InvalidHandle {
			windows.CloseHandle(handle)
		}
	}()
	ownRestoreCrashRoot(t, root)
	s, err := Open(root, Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	protected := waitRestoreProtectedFixture(t, s)
	if protected.Error.Code != "RESTORE_INCOMPLETE" {
		t.Fatal(protected)
	}
	wantError(t, call(s, "Environment.Preview", map[string]string{"kind": "create"}), "RESTORE_INCOMPLETE")
	if err = windows.CloseHandle(handle); err != nil {
		t.Fatal(err)
	}
	handle = windows.InvalidHandle
	value[Operation](t, call(s, "Backup.RecoverRestore", map[string]string{"operationId": signal.OperationID}))
	final := waitBackupFixture(t, s, signal.OperationID)
	if !final.RestoreReport.RolledBack || final.RestoreReport.Protected {
		t.Fatal("unoccupied retry did not recover", final)
	}
	assertRestoreCrashState(t, s, root, environments, false)
}

func TestRestoreHardInterruptAgainDuringRollback(t *testing.T) {
	if os.Getenv("PRISM_RESTORE_CRASH_VERIFY") != "1" {
		t.Skip("hard process interruption not explicitly selected")
	}
	for _, phase := range []string{"rollback-new-retained", "rollback-old-restored"} {
		t.Run(phase, func(t *testing.T) {
			root, source, environments := restoreCrashWorkspace(t)
			signal := crashRestoreFixture(t, root, source, "before-db-commit")
			if err := os.Remove(source); err != nil {
				t.Fatal(err)
			}
			crashRestoreFixture(t, root, "", phase, signal.OperationID)
			ownRestoreCrashRoot(t, root)
			s, err := Open(root, Options{})
			if err != nil {
				t.Fatal(err)
			}
			defer s.Close()
			final := waitBackupFixture(t, s, signal.OperationID)
			if !final.RestoreReport.RecoveredAfterRestart || !final.RestoreReport.RolledBack || final.RestoreReport.Protected {
				t.Fatal(final)
			}
			assertRestoreCrashState(t, s, root, environments, false)
		})
	}
}
