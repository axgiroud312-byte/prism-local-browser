//go:build windows

package workspace

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/axgiroud312-byte/prism-local-browser/internal/backup"
	"github.com/axgiroud312-byte/prism-local-browser/internal/desktopbase"
)

// This helper operates only in the parent-created synthetic workspace. It
// stages synthetic trial bytes, then uses the production switch/commit path.
// It never executes synthetic kernel binaries or claims real browser evidence.
func TestMigrationCrashHelper(t *testing.T) {
	if os.Getenv("PRISM_MIGRATION_CRASH_CHILD") != "1" {
		t.Skip("private migration crash helper")
	}
	root, token := os.Getenv("PRISM_MIGRATION_CRASH_ROOT"), os.Getenv("PRISM_MIGRATION_CRASH_TOKEN")
	marker, err := os.ReadFile(filepath.Join(root, "synthetic-migration-test.token"))
	if err != nil || token == "" || string(marker) != token {
		t.Fatal("not parent-owned synthetic workspace")
	}
	lock, err := desktopbase.Acquire(root)
	if err != nil {
		t.Fatal(err)
	}
	defer lock.Close()
	s, err := Open(root, Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	e, _, _, err := s.readEnvironment(os.Getenv("PRISM_MIGRATION_CRASH_ENVIRONMENT"))
	if err != nil {
		t.Fatal(err)
	}
	task := preparedMigrationFixture(t, s, e, os.Getenv("PRISM_MIGRATION_CRASH_KERNEL"))
	phase := os.Getenv("PRISM_MIGRATION_CRASH_PHASE")
	block := make(chan struct{})
	s.options.MigrationCheckpoint = func(current string) error {
		if current == phase {
			encoded, _ := json.Marshal(restoreCrashSignal{Phase: phase, OperationID: task.plan.ID})
			fmt.Println("PRISM_MIGRATION_CHECKPOINT " + string(encoded))
			<-block
		}
		return nil
	}
	s.commitMigration(context.Background(), task)
	t.Fatal("helper completed without requested hard-interrupt checkpoint")
}

func crashMigrationFixture(t *testing.T, root, environmentID, kernelID, phase string) restoreCrashSignal {
	t.Helper()
	token := id()
	marker := filepath.Join(root, "synthetic-migration-test.token")
	f, err := os.OpenFile(marker, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.WriteString(token); err != nil {
		f.Close()
		t.Fatal(err)
	}
	if err = f.Close(); err != nil {
		t.Fatal(err)
	}
	defer os.Remove(marker)
	cmd := exec.Command(os.Args[0], "-test.run=^TestMigrationCrashHelper$", "-test.timeout=3m")
	cmd.Env = append(os.Environ(), "PRISM_MIGRATION_CRASH_CHILD=1", "PRISM_MIGRATION_CRASH_ROOT="+root, "PRISM_MIGRATION_CRASH_TOKEN="+token, "PRISM_MIGRATION_CRASH_ENVIRONMENT="+environmentID, "PRISM_MIGRATION_CRASH_KERNEL="+kernelID, "PRISM_MIGRATION_CRASH_PHASE="+phase)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err = cmd.Start(); err != nil {
		t.Fatal("could not start owned migration helper")
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
		defer close(signals)
		scanner := bufio.NewScanner(stdout)
		for scanner.Scan() {
			line := scanner.Text()
			if strings.HasPrefix(line, "PRISM_MIGRATION_CHECKPOINT ") {
				var signal restoreCrashSignal
				if json.Unmarshal([]byte(strings.TrimPrefix(line, "PRISM_MIGRATION_CHECKPOINT ")), &signal) == nil {
					signals <- signal
					return
				}
			}
		}
	}()
	var signal restoreCrashSignal
	select {
	case v, ok := <-signals:
		if !ok {
			t.Fatal("helper exited before checkpoint; no hard-crash evidence")
		}
		signal = v
	case <-time.After(2 * time.Minute):
		t.Fatal("migration helper did not reach checkpoint")
	}
	if signal.Phase != phase || !backup.CanonicalID(signal.OperationID) {
		t.Fatal("checkpoint identity differs")
	}
	if competing, err := desktopbase.Acquire(root); err == nil {
		competing.Close()
		t.Fatal("live helper permitted competing writer")
	}
	if err = cmd.Process.Kill(); err != nil {
		t.Fatal("could not terminate exact owned process")
	}
	err = cmd.Wait()
	waited = true
	if err == nil || cmd.ProcessState == nil || cmd.ProcessState.Success() {
		t.Fatal("expected actual abnormal helper exit")
	}
	lock, err := desktopbase.Acquire(root)
	if err != nil {
		t.Fatal("host ownership not released on process death")
	}
	lock.Close()
	return signal
}

func TestMigrationHardInterruptFindsMatchingCompleteConfigurationAndDirectory(t *testing.T) {
	if os.Getenv("PRISM_MIGRATION_CRASH_VERIFY") != "1" {
		t.Skip("hard process interruption not explicitly selected")
	}
	evidence := []map[string]any{}
	for _, phase := range []string{"prepared", "old-retained", "new-installed", "configuration-committed"} {
		t.Run(phase, func(t *testing.T) {
			s, root, e, newID := migrationFixture(t)
			other := createRuntimeEnvironment(t, s, e.CoreID, "合成未选硬中断环境")
			oldProfile, otherProfile := view(t, s).Fingerprints[e.ID], view(t, s).Fingerprints[other.ID]
			for _, target := range []Environment{e, other} {
				ref, _ := dataReference(target.ID)
				dir := filepath.Join(root, filepath.FromSlash(ref))
				if err := os.MkdirAll(filepath.Join(dir, "empty"), 0700); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(dir, "Cookies"), []byte("SYNTHETIC_OLD_"+target.ID), 0600); err != nil {
					t.Fatal(err)
				}
			}
			if err := s.Close(); err != nil {
				t.Fatal(err)
			}
			signal := crashMigrationFixture(t, root, e.ID, newID, phase)
			ownRestoreCrashRoot(t, root)
			reopened, err := Open(root, Options{})
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { reopened.Close() })
			final := waitBackupFixture(t, reopened, signal.OperationID)
			committed := phase == "configuration-committed"
			if final.MigrationReport == nil || final.MigrationReport.Committed != committed || final.MigrationReport.Protected || final.PersistencePending || (final.State == "completed") != committed {
				t.Fatal("wrong recovered decision", final)
			}
			assertState := func(service *Service) {
				for _, target := range []Environment{e, other} {
					ref, _ := dataReference(target.ID)
					dir := filepath.Join(root, filepath.FromSlash(ref))
					want := "SYNTHETIC_OLD_" + target.ID
					if committed && target.ID == e.ID {
						want = "SYNTHETIC_NEW_SIDE"
					}
					data, err := os.ReadFile(filepath.Join(dir, "Cookies"))
					if err != nil || string(data) != want {
						t.Fatal("configuration selected mixed/cross-environment bytes", err)
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
						t.Fatal("file set differs", names)
					}
					empty, err := os.ReadDir(filepath.Join(dir, "empty"))
					if err != nil || len(empty) != 0 {
						t.Fatal("empty directory lost")
					}
					actual, _, _, err := service.readEnvironment(target.ID)
					kernelID := target.CoreID
					if committed && target.ID == e.ID {
						kernelID = newID
					}
					if err != nil || actual.CoreID != kernelID || actual.Seed != target.Seed || actual.Note != target.Note {
						t.Fatal("wrong configuration/identity side")
					}
					var savedRef string
					if err = service.db.QueryRow("SELECT user_data_ref FROM environments WHERE id=?", target.ID).Scan(&savedRef); err != nil || savedRef != ref {
						t.Fatal("reference changed")
					}
				}
				v := view(t, service)
				if !reflect.DeepEqual(v.Fingerprints[other.ID], otherProfile) {
					t.Fatal("unselected profile changed")
				}
				current := v.Fingerprints[e.ID]
				if !committed && !reflect.DeepEqual(current, oldProfile) || committed && (current.Profile.KernelID != newID || current.Profile.Seed != oldProfile.Profile.Seed || current.Profile.ConfigRevision != oldProfile.Profile.ConfigRevision+1) {
					t.Fatal("wrong profile/history side")
				}
				plan, _, savedPhase, err := service.readMigrationJournal(signal.OperationID)
				if err != nil || savedPhase != "finished" {
					t.Fatal("journal not finalized", err)
				}
				if digest, err := backup.PublishedDigest(context.Background(), filepath.Join(root, filepath.FromSlash(migrationBackupRef(plan.ID)))); err != nil || digest != plan.BackupSHA256 {
					t.Fatal("pre-upgrade full package lost")
				}
				retained, identity, files := plan.Move.Incoming, plan.Move.NewIdentity, plan.Move.NewFiles
				if committed {
					retained, identity, files = plan.Move.Previous, plan.Move.OldIdentity, plan.Move.OldFiles
				}
				if err := backup.VerifyTree(context.Background(), root, retained, identity, files); err != nil {
					t.Fatal("other complete side lost", err)
				}
			}
			assertState(reopened)
			if err = reopened.Close(); err != nil {
				t.Fatal(err)
			}
			again, err := Open(root, Options{})
			if err != nil {
				t.Fatal(err)
			}
			defer again.Close()
			assertState(again)
			stored := value[Operation](t, call(again, "Operation.Read", map[string]string{"operationId": signal.OperationID}))
			if stored.MigrationReport.Sequence != final.MigrationReport.Sequence {
				t.Fatal("second open replayed finalized migration")
			}
			evidence = append(evidence, map[string]any{"checkpoint": phase, "actualProcessKilled": true, "hostLockReacquired": true, "newDecision": committed, "selectedAndUnselectedDataMatched": true, "backupAndOppositeSideRetained": true, "secondOpenIdempotent": true})
		})
	}
	if destination := os.Getenv("PRISM_MIGRATION_CRASH_EVIDENCE"); destination != "" {
		encoded, _ := json.MarshalIndent(map[string]any{"verifiedAt": timestamp(), "scope": "test-owned native helper with synthetic trial bytes; not real browser migration", "checkpoints": evidence}, "", "  ")
		f, err := os.OpenFile(destination, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
		if err != nil {
			t.Fatal(err)
		}
		defer f.Close()
		if _, err = f.Write(append(encoded, '\n')); err != nil {
			t.Fatal(err)
		}
	}
}
