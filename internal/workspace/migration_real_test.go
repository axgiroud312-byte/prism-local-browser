//go:build windows

package workspace

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"sync/atomic"
	"testing"
	"time"

	"github.com/axgiroud312-byte/prism-local-browser/internal/kernel"
)

func waitMigrationStage(t *testing.T, s *Service, operationID, stage string) Operation {
	t.Helper()
	deadline := time.Now().Add(10 * time.Minute)
	for time.Now().Before(deadline) {
		op := value[Operation](t, call(s, "Operation.Read", map[string]string{"operationId": operationID}))
		if op.Stage == stage {
			return op
		}
		if op.State == "failed" || op.State == "cancelled" {
			t.Fatalf("migration failed before %s: %+v", stage, op)
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatal("migration stage timeout")
	return Operation{}
}

// Opens visible owned browsers without clicking. Two separately verified REAL
// versions are mandatory; two IDs pointing to the same 148 package do not count.
func TestRealMigrationTwoBuildsTrialSwitchFailureAndCompleteRollback(t *testing.T) {
	if os.Getenv("PRISM_MIGRATION_VERIFY") != "1" {
		t.Skip("real two-build migration verification not selected")
	}
	oldArchive, newArchive := os.Getenv("PRISM_KERNEL_ARCHIVE"), os.Getenv("PRISM_MIGRATION_KERNEL_ARCHIVE")
	newVersion, newSHA := os.Getenv("PRISM_MIGRATION_KERNEL_VERSION"), os.Getenv("PRISM_MIGRATION_KERNEL_SHA256")
	if oldArchive == "" || newArchive == "" || newVersion == "" || newVersion == "148.0.7778.215" || newSHA == "" {
		t.Fatal("provide two distinct verified versions and the target archive digest")
	}
	selectedArchive := oldArchive
	s, root := fixture(t, Options{ChooseArchive: func() (string, error) { return selectedArchive, nil }})
	install := func(version, sha string) string {
		token := value[struct {
			ArchiveToken string `json:"archiveToken"`
		}](t, call(s, "Kernel.SelectArchive", struct{}{}))
		op := value[struct {
			Operation Operation `json:"operation"`
		}](t, call(s, "Kernel.Install", kernel.InstallInput{Source: "local", Version: version, ExpectedChecksum: sha, ArchiveToken: token.ArchiveToken, Trusted: true, RequestID: id()})).Operation
		final := waitRuntimeReal(t, s, op.ID)
		if final.State != "completed" {
			t.Fatal("real build verification failed", final.Error)
		}
		return final.KernelID
	}
	oldID := install("148.0.7778.215", "9ef3f471b7a6641b4224532522b29141ce3746e27d55788d88e2fd951f362579")
	selectedArchive = newArchive
	newID := install(newVersion, newSHA)
	oldBuild, _ := s.savedKernel(oldID)
	newBuild, _ := s.savedKernel(newID)
	if oldBuild.Version == newBuild.Version || oldBuild.ExecutableSHA256 == newBuild.ExecutableSHA256 {
		t.Fatal("not two distinct real builds")
	}

	token := id()
	var write atomic.Bool
	write.Store(true)
	var changed atomic.Bool
	type report struct{ Generation, Cookie, Local, Indexed string }
	reports := make(chan report, 32)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		if r.URL.Path == "/"+token+"/report" && r.Method == "POST" {
			var v report
			if json.NewDecoder(io.LimitReader(r.Body, 4096)).Decode(&v) != nil {
				http.Error(w, "invalid", 400)
				return
			}
			select {
			case reports <- v:
			default:
			}
			w.WriteHeader(204)
			return
		}
		if r.URL.Path != "/"+token+"/" {
			http.NotFound(w, r)
			return
		}
		marker := "SYNTHETIC_BEFORE"
		if changed.Load() {
			marker = "SYNTHETIC_AFTER"
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprintf(w, `<!doctype html><title>Prism synthetic migration storage</title><script>(async()=>{
const generation=%q,write=%t,marker=%q;
const db=await new Promise((resolve,reject)=>{const r=indexedDB.open('prism-migration-real-test',1);r.onupgradeneeded=()=>r.result.createObjectStore('state');r.onsuccess=()=>resolve(r.result);r.onerror=reject;});
const indexed=async(value)=>await new Promise((resolve,reject)=>{const tx=db.transaction('state',value===undefined?'readonly':'readwrite');const r=value===undefined?tx.objectStore('state').get('marker'):tx.objectStore('state').put(value,'marker');tx.oncomplete=()=>resolve(value===undefined?r.result:value);tx.onerror=tx.onabort=reject;});
if(write){document.cookie='migration_real='+marker+';Path=/;Max-Age=3600;SameSite=Strict';localStorage.setItem('marker',marker);await indexed(marker);}
const result={generation,cookie:document.cookie,local:localStorage.getItem('marker'),indexed:await indexed()};db.close();await fetch('report',{method:'POST',body:JSON.stringify(result)});
})();</script>`, r.URL.Query().Get("generation"), write.Load(), marker)
	}))
	defer server.Close()
	policy := "direct"
	proxyID := ""
	if os.Getenv("PRISM_MIGRATION_PROXY_VERIFY") == "1" {
		upstream, check := protectedSiteProxy(t, server.URL)
		s.options.ProtectedProxyCheck = check
		proxyID = importProxyFixture(t, s, upstream).ID
		policy = "proxy"
	}
	p := generateFingerprint(t, s, preview(t, s, "create", ""), oldID, false)
	p.Environment.Name = "合成真实迁移目标"
	p.Environment.ProxyID = proxyID
	value[any](t, call(s, "Environment.Create", Mutation{PreviewID: p.PreviewID, Configuration: p.Environment.Configuration, ProfileHash: p.Fingerprint.PreviewProfile.ConfigHash, Count: 1, RequestID: id()}))
	e := view(t, s).State.Environments[0]
	other := createRuntimeEnvironment(t, s, oldID, "合成未选迁移环境")
	otherProfile := view(t, s).Fingerprints[other.ID]
	readStorage := func(service *Service, want string) {
		generation := id()
		draft := preview(t, service, "edit", e.ID)
		draft.Environment.URLs = server.URL + "/" + token + "/?generation=" + generation
		value[any](t, call(service, "Environment.Update", Mutation{PreviewID: draft.PreviewID, Configuration: draft.Environment.Configuration, ExpectedRevision: draft.ExpectedRevision, RequestID: id()}))
		op := acceptRuntimeTest(t, service, "Runtime.Start", runtimeRequest{EnvironmentID: e.ID, NetworkPolicy: policy, RequestID: id()})
		if final := waitRuntimeReal(t, service, op.ID); final.State != "completed" {
			t.Fatal(final)
		}
		deadline := time.After(45 * time.Second)
		for {
			select {
			case v := <-reports:
				if v.Generation != generation {
					continue
				}
				if v.Cookie != "migration_real="+want || v.Local != want || v.Indexed != want {
					t.Fatal("real browser storage mismatch", v)
				}
				goto observed
			case <-deadline:
				t.Fatal("no fresh real storage observation")
			}
		}
	observed:
		op = acceptRuntimeTest(t, service, "Runtime.Stop", runtimeRequest{EnvironmentID: e.ID, RequestID: id()})
		if final := waitRuntimeReal(t, service, op.ID); final.State != "completed" {
			t.Fatal(final)
		}
	}
	readStorage(s, "SYNTHETIC_BEFORE")
	write.Store(false)
	original := view(t, s).Fingerprints[e.ID]
	trial := func() Operation {
		p := value[MigrationPreview](t, call(s, "Migration.Preview", map[string]string{"environmentId": e.ID, "kernelId": newID}))
		if p.Before.Seed != original.Profile.Seed || p.After.Seed != original.Profile.Seed {
			t.Fatal("migration rerandomized seed")
		}
		op := value[struct {
			Operation Operation `json:"operation"`
		}](t, call(s, "Migration.Prepare", MigrationRequest{PreviewID: p.PreviewID, Confirm: true, RequestID: id()})).Operation
		observed := waitMigrationStage(t, s, op.ID, "trial-running")
		if observed.MigrationReport.Before == nil || observed.MigrationReport.After == nil || observed.MigrationReport.Before.Fingerprint.BrowserVersion != oldBuild.Version || observed.MigrationReport.After.Fingerprint.BrowserVersion != newBuild.Version {
			t.Fatal("trial lacks actual per-build observations")
		}
		if current := view(t, s).Fingerprints[e.ID]; !reflect.DeepEqual(current, original) {
			t.Fatal("trial changed original before explicit commit")
		}
		value[Operation](t, call(s, "Migration.Action", MigrationAction{OperationID: op.ID, Action: "stop", Confirm: true}))
		return waitMigrationStage(t, s, op.ID, "ready")
	}
	// Real directory switch failure must restore original data AND old build.
	failed := trial()
	var failureCheckpointHit atomic.Bool
	s.options.MigrationCheckpoint = func(stage string) error {
		if stage == "new-installed" {
			failureCheckpointHit.Store(true)
			return errors.New("synthetic switch interruption")
		}
		return nil
	}
	value[Operation](t, call(s, "Migration.Action", MigrationAction{OperationID: failed.ID, Action: "commit", Confirm: true}))
	failure := waitRuntimeReal(t, s, failed.ID)
	if !failureCheckpointHit.Load() || failure.State != "failed" || failure.PersistencePending || failure.MigrationReport.Committed {
		t.Fatal("switch failure not rolled back", failure)
	}
	s.options.MigrationCheckpoint = nil
	readStorage(s, "SYNTHETIC_BEFORE")
	ready := trial()
	value[Operation](t, call(s, "Migration.Action", MigrationAction{OperationID: ready.ID, Action: "commit", Confirm: true}))
	final := waitMigrationStage(t, s, ready.ID, "completed")
	if !final.MigrationReport.Committed || final.MigrationReport.Protected {
		t.Fatal(final)
	}
	if !reflect.DeepEqual(view(t, s).Fingerprints[other.ID], otherProfile) {
		t.Fatal("unselected environment upgraded")
	}
	if view(t, s).Fingerprints[e.ID].Profile.KernelID != newID {
		t.Fatal("selected configuration not switched")
	}
	// Read the original site state under the NEW build before any new writes;
	// a freshly written canary cannot prove pre-existing storage survived.
	readStorage(s, "SYNTHETIC_BEFORE")
	write.Store(true)
	changed.Store(true)
	readStorage(s, "SYNTHETIC_AFTER")
	write.Store(false)
	selected := value[struct {
		SourceToken string `json:"sourceToken"`
	}](t, call(s, "Migration.SelectRollback", map[string]string{"operationId": final.ID}))
	restore := value[RestorePreview](t, call(s, "Backup.PreviewRestore", map[string]string{"sourceToken": selected.SourceToken}))
	if !restore.CanRestore {
		t.Fatal(restore)
	}
	_, op := acceptRestoreFixture(t, s, restore)
	restored := waitRuntimeReal(t, s, op.ID)
	if restored.State != "completed" {
		t.Fatal(restored)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := Open(root, Options{ProtectedProxyCheck: s.options.ProtectedProxyCheck})
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	if !reflect.DeepEqual(view(t, reopened).Fingerprints[e.ID], original) || !reflect.DeepEqual(view(t, reopened).Fingerprints[other.ID], otherProfile) {
		t.Fatal("full rollback changed saved original identity/history")
	}
	readStorage(reopened, "SYNTHETIC_BEFORE")
	if destination := os.Getenv("PRISM_MIGRATION_EVIDENCE"); destination != "" {
		encoded, err := json.MarshalIndent(map[string]any{"verifiedAt": timestamp(), "oldBuild": oldBuild, "newBuild": newBuild, "migration": final, "switchFailure": failure, "rollback": restored, "networkPolicy": policy, "networkScope": "controlled local upstream; not external egress evidence", "cookieLocalStorageIndexedDBRestored": true, "selectedSeedPreserved": true, "unselectedProfileUnchanged": true, "applicationReopened": true, "uiClicks": "not-run"}, "", "  ")
		if err != nil {
			t.Fatal(err)
		}
		if err = os.WriteFile(filepath.Clean(destination), append(encoded, '\n'), 0600); err != nil {
			t.Fatal(err)
		}
	}
}
