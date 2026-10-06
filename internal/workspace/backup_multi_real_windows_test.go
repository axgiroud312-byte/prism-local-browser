//go:build windows

package workspace

import (
	"path/filepath"
	"testing"
	"time"
)

func (f *localAcceptanceFixture) readReport(t *testing.T, name, phase, generation string) {
	t.Helper()
	deadline := time.NewTimer(30 * time.Second)
	defer deadline.Stop()
	for {
		select {
		case report := <-f.reports:
			if report.Name != name {
				continue
			}
			storage := report.After
			if phase == "read" {
				storage = report.Before
			}
			if report.Phase != phase || storage.Cookie != "prism="+generation || storage.Local == nil || *storage.Local != generation || storage.Indexed == nil || *storage.Indexed != generation {
				t.Fatal("actual selected browser three-storage report mismatched")
			}
			return
		case <-deadline.C:
			t.Fatal("actual selected browser page did not report through controlled upstream")
		}
	}
}

func TestRealLocalAcceptanceTwoRunningEnvironmentBackupAndRestore(t *testing.T) {
	f := openLocalAcceptanceFixture(t)
	environments := map[string]Environment{}
	profiles := map[string]DeviceProfile{}
	references := map[string]string{}
	for _, name := range []string{"A", "B", "C"} {
		environment := f.environment(t, name, true)
		environments[name] = environment
		profiles[name] = view(t, f.s).Fingerprints[environment.ID].Profile
		references[name] = view(t, f.s).DataReferences[environment.ID]
		f.start(t, environment, false)
		f.readReport(t, name, "write", name+"-OLD")
	}
	other := view(t, f.s).RuntimeSessions[environments["C"].ID]
	path := filepath.Join(t.TempDir(), "owned-two-running.prismbackup")
	token := backupDestinationFixture(t, f.s, path)
	request := BackupExportRequest{Scope: "selected", EnvironmentIDs: []string{environments["A"].ID, environments["B"].ID}, DestinationToken: token, StopRunning: true, RequestID: id()}
	exported := acceptBackupFixture(t, f.s, request)
	final := waitRuntimeReal(t, f.s, exported.ID)
	if final.State != "completed" || final.BackupReport == nil || !final.BackupReport.Published {
		t.Fatal("complete backup of two running real environments failed:", final.Error)
	}
	for _, name := range []string{"A", "B"} {
		stopped := view(t, f.s).RuntimeSessions[environments[name].ID]
		if stopped.PID != 0 || stopped.ResourcesPending || stopped.LastExitCode == nil || *stopped.LastExitCode != 0 {
			t.Fatal("backup did not normally stop the whole selected real process tree")
		}
	}
	assertRealRootAlive(t, other)
	manifest, _ := readPackageFixture(t, path)
	if len(manifest.Environments) != 2 || manifest.KernelBinariesIncluded {
		t.Fatal("selected full backup scope or kernel distribution boundary changed")
	}
	for _, item := range manifest.Environments {
		if item.ID == environments["C"].ID || item.ID != environments["A"].ID && item.ID != environments["B"].ID {
			t.Fatal("unselected real environment entered the selected package")
		}
	}
	if repeated := acceptBackupFixture(t, f.s, request); repeated.ID != exported.ID {
		t.Fatal("same real backup request produced another package")
	}
	f.changed.Store(true)
	for _, name := range []string{"A", "B"} {
		environment := environments[name]
		f.start(t, environment, false)
		f.readReport(t, name, "write", name+"-NEW")
		f.stop(t, environment)
		p := preview(t, f.s, "edit", environment.ID)
		p.Environment.Note = "SYNTHETIC_CHANGED_AFTER_TWO_ENV_BACKUP"
		value[any](t, call(f.s, "Environment.Update", Mutation{PreviewID: p.PreviewID, Configuration: p.Environment.Configuration, ExpectedRevision: p.ExpectedRevision, RequestID: id()}))
	}
	assertRealRootAlive(t, other)
	f.stop(t, environments["C"])
	p := value[RestorePreview](t, previewPackageFixture(t, f.s, path))
	if p.OverwriteCount != 2 || !p.CanRestore {
		t.Fatal("two real environment restore preflight scope mismatch")
	}
	_, restoring := acceptRestoreFixture(t, f.s, p)
	finished := waitRuntimeReal(t, f.s, restoring.ID)
	if finished.State != "completed" || !finished.RestoreReport.Committed || finished.RestoreReport.Protected {
		t.Fatal("two real directories/configurations were not restored completely:", finished.Error)
	}
	options, root := f.s.options, f.s.root
	if err := f.s.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := Open(root, options)
	if err != nil {
		t.Fatal(err)
	}
	f.s = reopened
	f.changed.Store(false)
	f.readOnly.Store(true)
	for _, name := range []string{"A", "B", "C"} {
		environment := environments[name]
		assertLocalIdentityUnchanged(t, f.s, environment, profiles[name], references[name])
		current, _, _, err := f.s.readEnvironment(environment.ID)
		if err != nil || current.Note != environment.Note {
			t.Fatal("restored or unselected real environment configuration mismatched")
		}
		f.start(t, environment, false)
		f.readReport(t, name, "read", name+"-OLD")
		f.stop(t, environment)
	}
	writeLocalAcceptanceEvidence(t, "two-environment-backup-restore", map[string]any{
		"realRunningSelectedEnvironments": 2, "normallyStoppedByBackupWithExit0": 2,
		"fullPackageEnvironmentCount": len(manifest.Environments), "kernelBinariesIncluded": false,
		"requestDeduplicated": true, "bothRealThreeStorageMutationsObservedAfterBackup": true,
		"bothRealDirectoriesAndConfigurationsRestored": true, "serviceReopened": true,
		"bothCookieLocalStorageIndexedDBReadBackAfterRestore":                 true,
		"unselectedThirdEnvironmentAliveDuringBackupAndUnchangedAfterRestore": true,
		"allOriginalIDsProfilesAndDataReferencesPreserved":                    true,
		"windowsUserContext": "same-user", "kernelVersion": f.record.Version,
	})
}
