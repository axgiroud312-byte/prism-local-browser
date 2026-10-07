package workspace

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/axgiroud312-byte/prism-local-browser/internal/backup"
)

func TestRestoreSourceRecoversOnlyOriginalSelectionAndPreview(t *testing.T) {
	s, _ := fixture(t, Options{})
	e, _ := create(t, s, "SYNTHETIC original restore owner")
	path := restorePackageFixture(t, s, []string{e.ID})
	selections := 0
	s.options.ChooseBackupSource = func() (string, error) { selections++; return path, nil }
	request := map[string]string{"requestId": id()}
	selected := value[RestoreSourceState](t, call(s, "Backup.SelectRestoreSource", request))
	// Model the bridge losing the selection response. Query the original
	// request, not an invented token or a newly selected file.
	recovered := value[RestoreSourceState](t, call(s, "Backup.ReadRestoreSource", request))
	if recovered.RequestID != request["requestId"] || recovered.SourceToken != selected.SourceToken || recovered.Status != "selected" {
		t.Fatal("original source identity not recovered", recovered)
	}
	replayed := value[RestoreSourceState](t, call(s, "Backup.SelectRestoreSource", request))
	if replayed.SourceToken != selected.SourceToken || selections != 1 {
		t.Fatal("original request reopened chooser or created another token")
	}
	wantError(t, call(s, "Backup.ReadRestoreSource", map[string]string{"requestId": id()}), "RESTORE_SOURCE_UNCONFIRMED")
	wantError(t, call(s, "Backup.SelectRestoreSource", map[string]string{"requestId": id()}), "PROFILE_BUSY")
	p := value[RestorePreview](t, call(s, "Backup.PreviewRestore", map[string]string{"sourceToken": selected.SourceToken}))
	recovered = value[RestoreSourceState](t, call(s, "Backup.ReadRestoreSource", request))
	if recovered.Preview == nil || recovered.Preview.PreviewID != p.PreviewID || recovered.Preview.EnvironmentCount != 1 {
		t.Fatal("original preview not recovered")
	}
	encoded, _ := json.Marshal(recovered)
	if strings.Contains(string(encoded), path) || strings.Contains(string(encoded), "configuration.sqlite") {
		t.Fatal("ownership query leaked private paths")
	}
	s.mu.Lock()
	source := s.restoreSources[selected.SourceToken]
	source.expires = time.Now().Add(-time.Minute)
	s.restoreSources[selected.SourceToken] = source
	s.mu.Unlock()
	if value[RestoreSourceState](t, call(s, "Backup.ReadRestoreSource", request)).SourceToken != selected.SourceToken {
		t.Fatal("expired source lost cleanup ownership")
	}
	value[map[string]string](t, call(s, "Backup.DiscardRestore", request))
	if value[RestoreSourceState](t, call(s, "Backup.ReadRestoreSource", request)).Status != "discarded" || len(s.restoreSources) != 0 || s.restorePreview != nil {
		t.Fatal("discard acknowledged without consuming exact source")
	}
	value[RestoreSourceState](t, call(s, "Backup.SelectRestoreSource", map[string]string{"requestId": id()}))
}

func TestRestoreSourceSelectionInFlightRetainsOriginalRequest(t *testing.T) {
	s, _ := fixture(t, Options{})
	entered, release, done := make(chan struct{}), make(chan struct{}), make(chan Result, 1)
	s.options.ChooseBackupSource = func() (string, error) {
		close(entered)
		<-release
		return filepath.Join(t.TempDir(), "SYNTHETIC.prismbackup"), nil
	}
	request := map[string]string{"requestId": id()}
	go func() { done <- call(s, "Backup.SelectRestoreSource", request) }()
	<-entered
	state := value[RestoreSourceState](t, call(s, "Backup.ReadRestoreSource", request))
	if state.Status != "selecting" || state.SourceToken != "" {
		t.Fatal("in-flight selector manufactured a source")
	}
	wantError(t, call(s, "Backup.DiscardRestore", request), "PROFILE_BUSY")
	wantError(t, call(s, "Backup.SelectRestoreSource", request), "PROFILE_BUSY")
	close(release)
	selected := value[RestoreSourceState](t, <-done)
	if selected.SourceToken == "" || value[RestoreSourceState](t, call(s, "Backup.ReadRestoreSource", request)).SourceToken != selected.SourceToken {
		t.Fatal("late selector lost original ownership")
	}
	value[map[string]string](t, call(s, "Backup.DiscardRestore", request))
}

func TestRestoreSourceRecoveredCleanupRequiresExactOwnerAndActualRemoval(t *testing.T) {
	s, root := fixture(t, Options{})
	s.options.ChooseBackupSource = func() (string, error) { return filepath.Join(t.TempDir(), "SYNTHETIC.prismbackup"), nil }
	request := map[string]string{"requestId": id()}
	selected := value[RestoreSourceState](t, call(s, "Backup.SelectRestoreSource", request))
	directory := filepath.Join(root, "backups", "preflight", id())
	if err := os.MkdirAll(directory, 0700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(directory, "configuration.sqlite")
	if err := os.WriteFile(path, []byte("SYNTHETIC_OWNED_PREFLIGHT"), 0600); err != nil {
		t.Fatal(err)
	}
	source := s.restoreSources[selected.SourceToken]
	source.scratch = directory
	s.restoreSources[selected.SourceToken] = source
	s.restoreScratch = directory
	file, unpin, err := backup.FreezeFile(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { file.Close(); unpin() })
	wantError(t, call(s, "Backup.DiscardRestore", map[string]string{"requestId": request["requestId"], "sourceToken": id()}), "VALIDATION_FAILED")
	wantError(t, call(s, "Backup.DiscardRestore", request), "BACKUP_PREFLIGHT_FAILED")
	state := value[RestoreSourceState](t, call(s, "Backup.ReadRestoreSource", request))
	if !state.CleanupPending || state.SourceToken != selected.SourceToken || s.restoreScratch != directory {
		t.Fatal("failed cleanup lost accurate ownership")
	}
	file.Close()
	unpin()
	value[map[string]string](t, call(s, "Backup.DiscardRestore", request))
	if _, err = os.Stat(directory); !os.IsNotExist(err) || s.restoreScratch != "" {
		t.Fatal("reported cleanup before exact scratch disappeared", err)
	}
}
