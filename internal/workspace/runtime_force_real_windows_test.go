//go:build windows

package workspace

import (
	"errors"
	"fmt"
	"testing"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

// Suspend only threads whose retained handle still belongs to the exact root
// created by this fixture. No debugger, bare-PID termination, process-name scan,
// privilege escalation or system service change is used. Cleanup resumes every
// retained live thread even when a test fails.
func suspendLocalAcceptanceRoot(t *testing.T, session RuntimeSession) int {
	t.Helper()
	root, err := windows.OpenProcess(windows.SYNCHRONIZE|windows.PROCESS_QUERY_LIMITED_INFORMATION, false, uint32(session.PID))
	if err != nil {
		t.Fatal("exact owned root cannot be retained; no threads suspended")
	}
	t.Cleanup(func() { windows.CloseHandle(root) })
	var created, exited, kernelTime, userTime windows.Filetime
	if err := windows.GetProcessTimes(root, &created, &exited, &kernelTime, &userTime); err != nil || time.Unix(0, created.Nanoseconds()).UTC().Format(time.RFC3339Nano) != session.ProcessCreatedAt {
		t.Fatal("retained root creation identity does not match; no threads suspended")
	}
	assertRetainedRootAlive := func() {
		state, err := windows.WaitForSingleObject(root, 0)
		if err != nil || state != uint32(windows.WAIT_TIMEOUT) {
			t.Fatal("retained owned root exited; refusing further suspension")
		}
	}
	// This retained process handle prevents PID reuse throughout enumeration,
	// suspension and thread cleanup, even if the original browser exits.
	assertRetainedRootAlive()
	dll := windows.NewLazySystemDLL("kernel32.dll")
	owner := dll.NewProc("GetProcessIdOfThread")
	suspend := dll.NewProc("SuspendThread")
	retained := map[uint32]windows.Handle{}
	t.Cleanup(func() {
		for _, handle := range retained {
			state, err := windows.WaitForSingleObject(handle, 0)
			if err != nil {
				t.Error("retained owned thread exit state unavailable during cleanup")
			} else if state == uint32(windows.WAIT_TIMEOUT) {
				if _, err := windows.ResumeThread(handle); err != nil {
					t.Error("still-live owned thread could not be resumed:", err)
				}
			} else if state != windows.WAIT_OBJECT_0 {
				t.Error("unexpected retained owned thread state during cleanup")
			}
			windows.CloseHandle(handle)
		}
	})
	for pass := 0; pass < 4; pass++ {
		assertRetainedRootAlive()
		snapshot, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPTHREAD, 0)
		if err != nil {
			t.Fatal("owned thread snapshot unavailable")
		}
		entry := windows.ThreadEntry32{Size: uint32(unsafe.Sizeof(windows.ThreadEntry32{}))}
		added := 0
		err = windows.Thread32First(snapshot, &entry)
		for err == nil {
			if entry.OwnerProcessID == uint32(session.PID) {
				if _, already := retained[entry.ThreadID]; !already {
					handle, openErr := windows.OpenThread(windows.SYNCHRONIZE|windows.THREAD_SUSPEND_RESUME|windows.THREAD_QUERY_LIMITED_INFORMATION, false, entry.ThreadID)
					if openErr != nil {
						windows.CloseHandle(snapshot)
						t.Fatal("owned thread could not be pinned for controlled timeout")
					}
					pid, _, _ := owner.Call(uintptr(handle))
					if pid != uintptr(session.PID) {
						windows.CloseHandle(handle)
						windows.CloseHandle(snapshot)
						t.Fatal("thread identity changed; refusing suspension")
					}
					previous, _, _ := suspend.Call(uintptr(handle))
					if uint32(previous) == ^uint32(0) {
						windows.CloseHandle(handle)
						windows.CloseHandle(snapshot)
						t.Fatal("owned thread suspension failed")
					}
					retained[entry.ThreadID] = handle
					added++
				}
			}
			err = windows.Thread32Next(snapshot, &entry)
		}
		windows.CloseHandle(snapshot)
		if !errors.Is(err, windows.ERROR_NO_MORE_FILES) {
			t.Fatal("incomplete owned thread observation")
		}
		if pass > 0 && added == 0 && len(retained) > 0 {
			assertRetainedRootAlive()
			return len(retained)
		}
	}
	t.Fatal("owned thread set did not stabilize; no stop/force acceptance claim")
	return 0
}

func TestRealLocalAcceptanceForceStopAfterOrdinaryTimeout(t *testing.T) {
	f := openLocalAcceptanceFixture(t)
	a, b := f.environment(t, "force-A", false), f.environment(t, "force-B", false)
	original := view(t, f.s).Fingerprints[a.ID].Profile
	reference := view(t, f.s).DataReferences[a.ID]
	f.start(t, a, true)
	seed := cookiePreviewFixture(t, f.s, a.ID, fmt.Sprintf(`[{"name":"retained","value":"SYNTHETIC_FORCE_REOPEN","domain":"example.test","expires":%d}]`, time.Now().Unix()+3600))
	imported := acceptCookieFixture(t, f.s, cookieCommitFixture(t, f.s, seed, []int{1}, "merge"))
	if done := waitRuntimeReal(t, f.s, imported.ID); done.State != "completed" {
		t.Fatal("force fixture durable Cookie seed failed")
	}
	f.stop(t, a) // Persist a known baseline before the deliberately hung session.
	current := f.start(t, a, true)
	other := f.start(t, b, true)
	before := readLocalCookies(t, f.transport(t, a))
	force := runtimeRequest{EnvironmentID: a.ID, SessionID: current.SessionID, RequestID: id()}
	wantError(t, call(f.s, "Runtime.ForceStop", force), "FORCE_STOP_NOT_ALLOWED")
	threads := suspendLocalAcceptanceRoot(t, current)
	stop := acceptRuntimeTest(t, f.s, "Runtime.Stop", runtimeRequest{EnvironmentID: a.ID, RequestID: id()})
	failed := waitRuntimeReal(t, f.s, stop.ID)
	if failed.State != "failed" || failed.Error == nil || failed.Error.Code != "PROCESS_STOP_TIMEOUT" {
		t.Fatal("ordinary close did not actually time out against the suspended owned root")
	}
	state := view(t, f.s).RuntimeSessions[a.ID]
	if !state.CanForce || state.PID != current.PID || !state.ResourcesPending {
		t.Fatal("failed ordinary stop released ownership before real force")
	}
	assertRealRootAlive(t, other)
	forced := acceptRuntimeTest(t, f.s, "Runtime.ForceStop", force)
	if done := waitRuntimeReal(t, f.s, forced.ID); done.State != "completed" {
		t.Fatal("actual owned Job force-stop did not complete:", done.Error)
	}
	ended := view(t, f.s).RuntimeSessions[a.ID]
	if ended.PID != 0 || ended.ResourcesPending || ended.LastExitCode == nil || *ended.LastExitCode != 1 {
		t.Fatal("force did not confirm actual termination exit1 and complete owned cleanup")
	}
	assertRealRootAlive(t, other)
	f.start(t, a, true)
	if !sameLocalCookieCollection(before, readLocalCookies(t, f.transport(t, a))) {
		t.Fatal("force/reopen changed the durably saved Cookie baseline")
	}
	force.RequestID = id()
	wantError(t, call(f.s, "Runtime.ForceStop", force), "REVISION_CONFLICT")
	assertLocalIdentityUnchanged(t, f.s, a, original, reference)
	f.stop(t, a)
	f.stop(t, b)
	writeLocalAcceptanceEvidence(t, "force-stop", map[string]any{
		"ordinaryStopActuallyTimedOut": true, "ordinaryStopError": failed.Error.Code,
		"exactRootHandleRetainedThroughoutThreadSuspensionAndCleanup": true,
		"suspendedOnlyExactOwnedRootThreads":                          threads, "forceStopActuallyUsed": true,
		"ownedJobExitCode": *ended.LastExitCode, "completeResourcesReleased": true,
		"otherRootAliveBeforeAndAfterForce": true, "durableCookieBaselinePreservedAfterReopen": true,
		"oldSessionForceRejected": true, "savedIdentityAndDataReferenceUnchanged": true,
	})
}
