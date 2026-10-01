//go:build windows

package kernel

import (
	"os"
	"path/filepath"
	"testing"
	"time"
	"unsafe"

	"github.com/google/uuid"
	"golang.org/x/sys/windows"
)

func TestRecoveryInspectionNeverCreatesMissingBrowserDirectoriesOrLocks(t *testing.T) {
	root, environmentID, sessionID := t.TempDir(), uuid.NewString(), uuid.NewString()
	ref := "environments/" + environmentID + "/user-data"
	dir := filepath.Join(root, filepath.FromSlash(ref))
	for attempt := 0; attempt < 2; attempt++ {
		observed, err := InspectExistingManagedProfile(root, environmentID, sessionID, ref, 0, "", true, true, true)
		if err != nil || !observed.DirectoryFree || !observed.SessionMatches || !observed.ResourcesExited {
			t.Fatal("known uninitialized state could not be checked", err)
		}
		if _, err := os.Stat(dir); !os.IsNotExist(err) {
			t.Fatal("inspection created missing directory", err)
		}
	}
	if _, err := InspectExistingManagedProfile(root, environmentID, sessionID, ref, 0, "", true, false, false); err == nil {
		t.Fatal("required missing data was accepted")
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	observed, err := InspectExistingManagedProfile(root, environmentID, sessionID, ref, 0, "", true, false, true)
	if err != nil || !observed.DirectoryFree || !observed.SessionMatches {
		t.Fatal("prepared directory without runtime claim was rejected", err)
	}
	if _, err := os.Stat(filepath.Join(dir, ".prism-runtime.lock")); !os.IsNotExist(err) {
		t.Fatal("inspection created a lock file", err)
	}
	if _, err := InspectExistingManagedProfile(root, environmentID, sessionID, ref, 0, "", false, true, true); err == nil {
		t.Fatal("unknown creation stage bypassed missing record")
	}
}

func TestManagedRecoveryChecksTheActualLockAndNeverAdoptsAReusedPID(t *testing.T) {
	root, environmentID, sessionID := t.TempDir(), uuid.NewString(), uuid.NewString()
	ref := "environments/" + environmentID + "/user-data"
	lock, err := lockManagedProfile(root, environmentID, ref)
	if err != nil {
		t.Fatal(err)
	}
	wrongTime := "2000-01-01T00:00:00Z"
	if err = lock.record(environmentID, sessionID, uint32(os.Getpid()), wrongTime); err != nil {
		t.Fatal(err)
	}
	busy, err := InspectManagedProfile(root, environmentID, sessionID, ref, os.Getpid(), wrongTime, false)
	if err != nil || busy.DirectoryFree {
		t.Fatal("held actual lock was treated as stale/free")
	}
	lock.release()
	observed, err := InspectManagedProfile(root, environmentID, sessionID, ref, os.Getpid(), wrongTime, false)
	if err != nil || observed.ProcessState != "reused" || !observed.DirectoryFree || !observed.SessionMatches {
		t.Fatal("PID reuse/actual directory identity was not distinguished")
	}
	state, err := windows.WaitForSingleObject(windows.CurrentProcess(), 0)
	if err != nil || state != uint32(windows.WAIT_TIMEOUT) {
		t.Fatal("inspection affected the unrelated current process")
	}
}

func TestManagedRecoveryPIDZeroInStartingStageReadsTheActualCreatedProcessRecord(t *testing.T) {
	root, environmentID, sessionID := t.TempDir(), uuid.NewString(), uuid.NewString()
	ref := "environments/" + environmentID + "/user-data"
	var created, exited, kernelTime, userTime windows.Filetime
	if err := windows.GetProcessTimes(windows.CurrentProcess(), &created, &exited, &kernelTime, &userTime); err != nil {
		t.Fatal(err)
	}
	createdAt := time.Unix(0, created.Nanoseconds()).UTC().Format(time.RFC3339Nano)
	lock, err := lockManagedProfile(root, environmentID, ref)
	if err != nil {
		t.Fatal(err)
	}
	if err = lock.record(environmentID, sessionID, uint32(os.Getpid()), createdAt); err != nil {
		t.Fatal(err)
	}
	lock.release()
	observed, err := InspectManagedProfile(root, environmentID, sessionID, ref, 0, "", false)
	if err != nil || observed.ProcessState != "alive" || observed.RootPID != os.Getpid() || !observed.SessionMatches {
		t.Fatal("zero journal PID bypassed actual live process identity in metadata")
	}
	queued, err := InspectManagedProfile(root, environmentID, uuid.NewString(), ref, 0, "", true)
	if err != nil || queued.ProcessState != "not-created" || !queued.DirectoryFree {
		t.Fatal("definitely queued session cannot be safely distinguished")
	}
}

func TestManagedJobIdentityCannotAdoptAnExistingNamedJobAndInspectionHasNoControlRights(t *testing.T) {
	sessionID := uuid.NewString()
	job, err := createManagedJob(sessionID)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if job != 0 {
			_ = windows.CloseHandle(job)
		}
	}()
	if duplicate, err := createManagedJob(sessionID); err == nil {
		windows.CloseHandle(duplicate)
		t.Fatal("new session adopted an already existing job object")
	}
	name, err := managedJobName(sessionID)
	if err != nil {
		t.Fatal(err)
	}
	value, _, err := inspectManagedJobProc.Call(4, 0, uintptr(unsafe.Pointer(name)))
	if value == 0 {
		t.Fatal(err)
	}
	query := windows.Handle(value)
	if err := windows.TerminateJobObject(query, 1); err == nil {
		windows.CloseHandle(query)
		t.Fatal("query-only recovery handle has termination rights")
	}
	_ = windows.CloseHandle(query)
	if exited, err := managedJobResourcesExited(sessionID); err != nil || !exited {
		t.Fatal("empty exact job accounting could not be queried")
	}
	_ = windows.CloseHandle(job)
	job = 0
	if exited, err := managedJobResourcesExited(sessionID); err != nil || !exited {
		t.Fatal("destroyed empty job was not distinguished from unavailable accounting")
	}
}
