//go:build windows

package workspace

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/axgiroud312-byte/prism-local-browser/internal/kernel"
	"golang.org/x/sys/windows"
)

type syntheticBrowserStorage struct {
	Cookie  string  `json:"cookie"`
	Local   *string `json:"local"`
	Indexed *string `json:"indexed"`
}
type syntheticBrowserReport struct {
	Name          string                  `json:"name"`
	Phase         string                  `json:"phase"`
	Before        syntheticBrowserStorage `json:"before"`
	After         syntheticBrowserStorage `json:"after"`
	RequestCookie string                  `json:"requestCookie"`
}

func waitRuntimeReal(t *testing.T, s *Service, operationID string) Operation {
	t.Helper()
	deadline := time.Now().Add(2 * time.Minute)
	for time.Now().Before(deadline) {
		operation := value[Operation](t, call(s, "Operation.Read", map[string]string{"operationId": operationID}))
		if operation.State == "completed" || operation.State == "failed" || operation.State == "cancelled" {
			return operation
		}
		time.Sleep(25 * time.Millisecond)
	}
	t.Fatal("real operation did not terminate")
	return Operation{}
}

func assertRealRootAlive(t *testing.T, session RuntimeSession) {
	t.Helper()
	handle, err := windows.OpenProcess(windows.SYNCHRONIZE|windows.PROCESS_QUERY_LIMITED_INFORMATION, false, uint32(session.PID))
	if err != nil {
		t.Fatal("independent browser root could not be inspected")
	}
	defer windows.CloseHandle(handle)
	var created, exited, kernelTime, userTime windows.Filetime
	if err = windows.GetProcessTimes(handle, &created, &exited, &kernelTime, &userTime); err != nil || time.Unix(0, created.Nanoseconds()).UTC().Format(time.RFC3339Nano) != session.ProcessCreatedAt {
		t.Fatal("independent browser process identity changed")
	}
	state, err := windows.WaitForSingleObject(handle, 0)
	if err != nil || state != uint32(windows.WAIT_TIMEOUT) {
		t.Fatal("stopping A ended B's independently owned process")
	}
}

// Deliberate fault injection only for the exact root created by this fixture.
// Production recovery/force commands never accept or terminate a client PID.
func terminateRealRuntimeFixtureRoot(t *testing.T, session RuntimeSession) {
	t.Helper()
	handle, err := windows.OpenProcess(windows.SYNCHRONIZE|windows.PROCESS_QUERY_LIMITED_INFORMATION|windows.PROCESS_TERMINATE, false, uint32(session.PID))
	if err != nil {
		t.Fatal("test-owned browser root could not be inspected for fault injection")
	}
	defer windows.CloseHandle(handle)
	var created, exited, kernelTime, userTime windows.Filetime
	if err = windows.GetProcessTimes(handle, &created, &exited, &kernelTime, &userTime); err != nil || time.Unix(0, created.Nanoseconds()).UTC().Format(time.RFC3339Nano) != session.ProcessCreatedAt {
		t.Fatal("fault injection identity does not match the fixture's root")
	}
	if err = windows.TerminateProcess(handle, 79); err != nil {
		t.Fatal("test-owned root fault injection failed")
	}
}

// Explicit opt-in is necessary: unlike T04/T05 diagnostics this opens real
// normal browser windows (still no UI clicking). Never runs in default tests.
func TestRealIndependentBrowserSessions(t *testing.T) {
	archive := os.Getenv("PRISM_KERNEL_ARCHIVE")
	if archive == "" || os.Getenv("PRISM_RUNTIME_VERIFY") != "1" {
		t.Skip("real visible runtime verification not explicitly selected")
	}
	reports := make(chan syntheticBrowserReport, 64)
	var readPhase atomic.Bool
	token := id()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		if r.URL.Path == "/"+token+"/report" && r.Method == "POST" {
			var report syntheticBrowserReport
			if json.NewDecoder(io.LimitReader(r.Body, 16384)).Decode(&report) != nil || (report.Name != "A" && report.Name != "B") {
				http.Error(w, "invalid synthetic report", 400)
				return
			}
			report.RequestCookie = r.Header.Get("Cookie")
			select {
			case reports <- report:
			default:
			}
			w.WriteHeader(204)
			return
		}
		name := strings.TrimPrefix(r.URL.Path, "/"+token+"/")
		if name != "A" && name != "B" {
			http.NotFound(w, r)
			return
		}
		phase := "write"
		if readPhase.Load() {
			phase = "read"
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprintf(w, `<!doctype html><title>Prism synthetic isolation %s</title><script>
(async () => {
 const name=%q, phase=%q, marker="SYNTHETIC-"+name;
 const database = () => new Promise((resolve,reject) => {
   const request=indexedDB.open("prism-synthetic-isolation",1);
   request.onupgradeneeded=()=>request.result.createObjectStore("state");
   request.onsuccess=()=>resolve(request.result); request.onerror=()=>reject(request.error);
 });
 const indexed = async (value) => {
   const db=await database();
   return await new Promise((resolve,reject) => {
     const tx=db.transaction("state",value===undefined?"readonly":"readwrite");
     const request=value===undefined?tx.objectStore("state").get("marker"):tx.objectStore("state").put(value,"marker");
     tx.oncomplete=()=>{const result=value===undefined?(request.result??null):value;db.close();resolve(result);};
     tx.onerror=()=>{db.close();reject(tx.error);}; tx.onabort=tx.onerror;
   });
 };
 const state=async()=>({cookie:document.cookie,local:localStorage.getItem("marker"),indexed:await indexed()});
 const before=await state();
 if(phase==="write"){document.cookie="prism_synthetic="+marker+";Path=/;Max-Age=3600;SameSite=Lax";localStorage.setItem("marker",marker);await indexed(marker);}
 const after=await state();
 await fetch(%q,{method:"POST",headers:{"Content-Type":"application/json"},body:JSON.stringify({name,phase,before,after})});
})();</script>`, name, name, phase, "/"+token+"/report")
	}))
	defer server.Close()

	s, root := fixture(t, Options{ChooseArchive: func() (string, error) { return archive, nil }})
	selected := value[struct {
		ArchiveToken string `json:"archiveToken"`
	}](t, call(s, "Kernel.SelectArchive", struct{}{}))
	installed := value[struct {
		Operation Operation `json:"operation"`
	}](t, call(s, "Kernel.Install", kernel.InstallInput{Source: "local", Version: "148.0.7778.215", ExpectedChecksum: "9ef3f471b7a6641b4224532522b29141ce3746e27d55788d88e2fd951f362579", ArchiveToken: selected.ArchiveToken, Trusted: true, RequestID: id()})).Operation
	completed := waitRuntimeReal(t, s, installed.ID)
	if completed.State != "completed" {
		t.Fatalf("real install failed: %+v", completed.Error)
	}
	record := view(t, s).KernelRecords[0].Record
	environments := map[string]Environment{}
	profiles := map[string]DeviceProfile{}
	refs := map[string]string{}
	for _, name := range []string{"A", "B"} {
		p := generateFingerprint(t, s, preview(t, s, "create", ""), record.ID, false)
		p.Environment.Name, p.Environment.URLs = "合成真实隔离"+name, server.URL+"/"+token+"/"+name
		value[map[string]any](t, call(s, "Environment.Create", Mutation{PreviewID: p.PreviewID, Configuration: p.Environment.Configuration, Count: 1, ProfileHash: p.Fingerprint.PreviewProfile.ConfigHash, RequestID: id()}))
		for _, environment := range view(t, s).State.Environments {
			if environment.Name == p.Environment.Name {
				environments[name] = environment
			}
		}
		environment := environments[name]
		profiles[name], refs[name] = view(t, s).Fingerprints[environment.ID].Profile, view(t, s).DataReferences[environment.ID]
	}
	if environments["A"].ID == environments["B"].ID || profiles["A"].Seed == profiles["B"].Seed || refs["A"] == refs["B"] {
		t.Fatal("environments are not independent identities/references")
	}
	observations := []syntheticBrowserReport{}
	startedSessions := []RuntimeSession{}
	stoppedSessions := []RuntimeSession{}
	inspect := func(service *Service, name, phase string) {
		deadline := time.After(45 * time.Second)
		for {
			select {
			case report := <-reports:
				if report.Name != name || report.Phase != phase {
					continue
				}
				marker := "SYNTHETIC-" + name
				if report.After.Local == nil || *report.After.Local != marker || report.After.Indexed == nil || *report.After.Indexed != marker || report.After.Cookie != "prism_synthetic="+marker || report.RequestCookie != "prism_synthetic="+marker {
					t.Fatalf("wrong independent synthetic storage: %+v", report)
				}
				if phase == "write" && (report.Before.Cookie != "" || report.Before.Local != nil || report.Before.Indexed != nil) {
					t.Fatal("new profile inherited another profile's browser storage")
				}
				if phase == "read" && !reflect.DeepEqual(report.Before, report.After) {
					t.Fatal("reopen changed saved browser storage")
				}
				observations = append(observations, report)
				return
			case <-deadline:
				t.Fatal("real browser storage report not received")
			}
		}
	}
	start := func(service *Service, name string) {
		operation := acceptRuntimeTest(t, service, "Runtime.Start", runtimeRequest{EnvironmentID: environments[name].ID, RequestID: id(), NetworkPolicy: "direct"})
		if completed := waitRuntimeReal(t, service, operation.ID); completed.State != "completed" {
			t.Fatalf("real start failed: %+v", completed.Error)
		}
		session := view(t, service).RuntimeSessions[environments[name].ID]
		if session.State != "running" || session.PID < 1 || session.ProcessCreatedAt == "" {
			t.Fatal("running lacks actual process identity/readiness")
		}
		startedSessions = append(startedSessions, session)
	}
	stop := func(service *Service, name string) {
		operation := acceptRuntimeTest(t, service, "Runtime.Stop", runtimeRequest{EnvironmentID: environments[name].ID, RequestID: id()})
		if completed := waitRuntimeReal(t, service, operation.ID); completed.State != "completed" {
			t.Fatalf("real stop failed: %+v", completed.Error)
		}
		session := view(t, service).RuntimeSessions[environments[name].ID]
		if session.PID != 0 {
			t.Fatal("stop retained an active process")
		}
		stoppedSessions = append(stoppedSessions, session)
	}
	for _, name := range []string{"A", "B"} {
		start(s, name)
		inspect(s, name, "write")
	}
	if view(t, s).RuntimeSessions[environments["A"].ID].PID == view(t, s).RuntimeSessions[environments["B"].ID].PID {
		t.Fatal("two environments share a browser root")
	}
	bRunning := view(t, s).RuntimeSessions[environments["B"].ID]
	stop(s, "A")
	assertRealRootAlive(t, bRunning)
	stop(s, "B")
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := Open(root, Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	readPhase.Store(true)
	for _, name := range []string{"A", "B"} {
		if !reflect.DeepEqual(view(t, reopened).Fingerprints[environments[name].ID].Profile, profiles[name]) || view(t, reopened).DataReferences[environments[name].ID] != refs[name] {
			t.Fatal("service reopen changed frozen inputs/data references")
		}
		start(reopened, name)
		inspect(reopened, name, "read")
		stop(reopened, name)
	}
	recoveryEvidence := map[string]any{"status": "not-run", "applicationCrashRestart": "not-run"}
	if os.Getenv("PRISM_RECOVERY_VERIFY") == "1" {
		// This opens additional visible browsers and deliberately kills a root;
		// the extra switch is required even after T06's explicit opt-in.
		start(reopened, "A")
		inspect(reopened, "A", "read")
		start(reopened, "B")
		inspect(reopened, "B", "read")
		beforeCrash := view(t, reopened).RuntimeSessions[environments["A"].ID]
		independent := view(t, reopened).RuntimeSessions[environments["B"].ID]
		terminateRealRuntimeFixtureRoot(t, beforeCrash)
		observed := waitRuntimeObservation(t, reopened, environments["A"].ID, func(session RuntimeSession) bool {
			return session.Error != nil && session.Error.Code == "PROCESS_CRASHED"
		})
		forceUsed := false
		if observed.PID > 0 {
			closing := acceptRuntimeTest(t, reopened, "Runtime.Stop", runtimeRequest{EnvironmentID: environments["A"].ID, RequestID: id()})
			if waitRuntimeReal(t, reopened, closing.ID).State == "failed" {
				current := view(t, reopened).RuntimeSessions[environments["A"].ID]
				if !current.CanForce {
					t.Fatal("real failed normal close did not offer the exact still-owned job")
				}
				forced := acceptRuntimeTest(t, reopened, "Runtime.ForceStop", runtimeRequest{EnvironmentID: environments["A"].ID, SessionID: current.SessionID, RequestID: id()})
				if waitRuntimeReal(t, reopened, forced.ID).State != "completed" {
					t.Fatal("real force did not confirm the selected owned job's exit")
				}
				forceUsed = true
			}
		}
		assertRealRootAlive(t, independent)
		start(reopened, "A")
		inspect(reopened, "A", "read")
		wantError(t, call(reopened, "Runtime.ForceStop", runtimeRequest{EnvironmentID: environments["A"].ID, SessionID: beforeCrash.SessionID, RequestID: id()}), "REVISION_CONFLICT")
		for _, name := range []string{"A", "B"} {
			stop(reopened, name)
			if !reflect.DeepEqual(view(t, reopened).Fingerprints[environments[name].ID].Profile, profiles[name]) {
				t.Fatal("real fault recovery changed fixed device inputs")
			}
		}
		recoveryEvidence = map[string]any{"status": "observed", "rootFaultInjected": true, "faultExitCode": 79, "crashObservation": observed, "otherRootAlive": true, "sameBrowserStorageAfterRetry": true, "forceStopActuallyUsed": forceUsed, "applicationCrashRestart": "not-run"}
	}
	if destination := os.Getenv("PRISM_RUNTIME_EVIDENCE"); destination != "" {
		evidence := map[string]any{"verifiedAt": timestamp(), "mode": "native", "kernelVersion": record.Version, "archiveSha256": record.ArchiveSHA256, "executableSha256": record.ExecutableSHA256, "profiles": profiles, "dataReferences": refs, "observations": observations, "startedSessions": startedSessions, "stoppedSessions": stoppedSessions, "transport": "inherited-private-pipe", "sandbox": true, "networkPolicy": "explicit-direct-test", "uiClicks": "not-run", "visibleBrowserWindows": true, "independentRootProcesses": true, "controlledJobsExited": true, "sameInputsAfterReopen": true}
		evidence["recovery"] = recoveryEvidence
		encoded, err := json.MarshalIndent(evidence, "", "  ")
		if err != nil {
			t.Fatal(err)
		}
		if err = os.WriteFile(filepath.Clean(destination), append(encoded, '\n'), 0600); err != nil {
			t.Fatal(err)
		}
	}
}
