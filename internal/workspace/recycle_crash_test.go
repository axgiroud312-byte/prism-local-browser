//go:build windows

package workspace

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/axgiroud312-byte/prism-local-browser/internal/desktopbase"
)

func TestRecycleCrashHelper(t *testing.T) {
	if os.Getenv("PRISM_RECYCLE_CRASH_CHILD") != "1" {
		t.Skip("private owned crash helper")
	}
	root, token := os.Getenv("PRISM_RECYCLE_CRASH_ROOT"), os.Getenv("PRISM_RECYCLE_CRASH_TOKEN")
	marker, err := os.ReadFile(filepath.Join(root, "synthetic-recycle.token"))
	if err != nil || token == "" || string(marker) != token {
		t.Fatal("not a synthetic parent-owned workspace")
	}
	lease, err := desktopbase.Acquire(root)
	if err != nil {
		t.Fatal(err)
	}
	defer lease.Close()
	phase := os.Getenv("PRISM_RECYCLE_CRASH_PHASE")
	hold := make(chan struct{})
	var s *Service
	s, err = Open(root, Options{RecycleCheckpoint: func(current string) error {
		if current == phase {
			s.mu.Lock()
			operationID := s.recycleTask.plan.ID
			s.mu.Unlock()
			fmt.Println("PRISM_RECYCLE_CHECKPOINT " + operationID)
			<-hold
		}
		return nil
	}})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	acceptRecycleFixture(t, s, previewRecycleFixture(t, s, os.Getenv("PRISM_RECYCLE_CRASH_ACTION"), os.Getenv("PRISM_RECYCLE_CRASH_SELECTION")))
	<-hold
}

func crashRecycleFixture(t *testing.T, root, action, selected, phase string) string {
	t.Helper()
	token := id()
	marker := filepath.Join(root, "synthetic-recycle.token")
	f, err := os.OpenFile(marker, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		t.Fatal(err)
	}
	_, err = f.WriteString(token)
	closeErr := f.Close()
	if err != nil || closeErr != nil {
		t.Fatal(err, closeErr)
	}
	defer os.Remove(marker)
	cmd := exec.Command(os.Args[0], "-test.run=^TestRecycleCrashHelper$", "-test.timeout=3m")
	cmd.Env = append(os.Environ(), "PRISM_RECYCLE_CRASH_CHILD=1", "PRISM_RECYCLE_CRASH_ROOT="+root, "PRISM_RECYCLE_CRASH_TOKEN="+token, "PRISM_RECYCLE_CRASH_ACTION="+action, "PRISM_RECYCLE_CRASH_SELECTION="+selected, "PRISM_RECYCLE_CRASH_PHASE="+phase)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err = cmd.Start(); err != nil {
		t.Fatal(err)
	}
	waited := false
	defer func() {
		if !waited {
			cmd.Process.Kill()
			cmd.Wait()
		}
	}()
	signal := make(chan string, 1)
	go func() {
		scanner := bufio.NewScanner(stdout)
		for scanner.Scan() {
			if id, ok := strings.CutPrefix(scanner.Text(), "PRISM_RECYCLE_CHECKPOINT "); ok {
				signal <- id
				return
			}
		}
		close(signal)
	}()
	var operationID string
	select {
	case operationID = <-signal:
		if operationID == "" {
			t.Fatal("helper exited before checkpoint")
		}
	case <-time.After(2 * time.Minute):
		t.Fatal("checkpoint timeout")
	}
	if competing, err := desktopbase.Acquire(root); err == nil {
		competing.Close()
		t.Fatal("second workspace host acquired active root")
	}
	if err := cmd.Process.Kill(); err != nil {
		t.Fatal(err)
	}
	err = cmd.Wait()
	waited = true
	if err == nil {
		t.Fatal("helper exited normally, no hard interruption")
	}
	return operationID
}

func TestRecycleHardInterruptionPreservesConfirmedItemDecision(t *testing.T) {
	if os.Getenv("PRISM_RECYCLE_CRASH_VERIFY") != "1" {
		t.Skip("owned Windows hard-interruption verification not selected")
	}
	for _, action := range []string{"remove", "restore", "purge"} {
		phases := []string{"prepared", "directory-changed", "item-committed"}
		if action == "purge" {
			phases = []string{"prepared", "purging", "data-deleted", "item-committed"}
		}
		for _, phase := range phases {
			t.Run(action+"/"+phase, func(t *testing.T) {
				s, root := fixture(t, Options{})
				e, dir := recycleFixture(t, s, root, "合成硬中断目标")
				other, otherDir := recycleFixture(t, s, root, "合成硬中断包外")
				selected := e.ID
				if action != "remove" {
					if op := runRecycleFixture(t, s, "remove", e.ID); op.State != "completed" {
						t.Fatal(op)
					}
					selected = recycleListFixture(t, s).Items[0].ID
				}
				if err := s.Close(); err != nil {
					t.Fatal(err)
				}
				opID := crashRecycleFixture(t, root, action, selected, phase)
				lease, err := desktopbase.Acquire(root)
				if err != nil {
					t.Fatal(err)
				}
				defer lease.Close()
				reopened, err := Open(root, Options{})
				if err != nil {
					t.Fatal(err)
				}
				defer reopened.Close()
				op := waitBackupFixture(t, reopened, opID)
				if op.RecycleReport.Protected || op.PersistencePending {
					t.Fatal("recovery did not resolve", op)
				}
				purged := action == "purge" && phase != "prepared"
				trashed := action == "remove" && phase == "item-committed" || action == "restore" && phase != "item-committed" || action == "purge" && !purged
				stored, _, _, readErr := reopened.readStoredEnvironment(e.ID)
				if purged {
					if readErr == nil {
						t.Fatal("purged config remained")
					}
				} else if readErr != nil || stored.Seed != e.Seed || stored.CoreID != e.CoreID || stored.Name != e.Name {
					t.Fatal("original identity lost", readErr)
				}
				list := recycleListFixture(t, reopened)
				if trashed {
					if list.Total != 1 || list.Items[0].EnvironmentID != e.ID {
						t.Fatal(list)
					}
					dir = filepath.Join(root, filepath.FromSlash(recycleReference(list.Items[0].ID)))
				} else if list.Total != 0 {
					t.Fatal("incorrect recycle membership", list)
				}
				if !purged {
					if data, err := os.ReadFile(filepath.Join(dir, "Cookies")); err != nil || string(data) != "SYNTHETIC_RECYCLE_"+e.ID {
						t.Fatal("original browser bytes lost", err)
					}
				}
				if data, err := os.ReadFile(filepath.Join(otherDir, "Cookies")); err != nil || string(data) != "SYNTHETIC_RECYCLE_"+other.ID {
					t.Fatal("unselected tree changed", err)
				}
			})
		}
	}
}
