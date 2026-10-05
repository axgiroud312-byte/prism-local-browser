//go:build windows

package kernel

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/google/uuid"
	"golang.org/x/sys/windows"
)

func TestManagedProfileLocksAreExclusiveAndNeverDeleteSavedData(t *testing.T) {
	root, environmentID := t.TempDir(), uuid.NewString()
	ref := "environments/" + environmentID + "/user-data"
	first, err := lockManagedProfile(root, environmentID, ref)
	if err != nil {
		t.Fatal(err)
	}
	defer first.release()
	sentinel := filepath.Join(first.path, "synthetic-data")
	if err := os.WriteFile(sentinel, []byte("synthetic-retained"), 0600); err != nil {
		t.Fatal(err)
	}
	if second, err := lockManagedProfile(root, environmentID, ref); err == nil {
		second.release()
		t.Fatal("same actual directory opened twice")
	}
	otherID := uuid.NewString()
	other, err := lockManagedProfile(root, otherID, "environments/"+otherID+"/user-data")
	if err != nil {
		t.Fatal(err)
	}
	other.release()
	if first.path == other.path {
		t.Fatal("distinct environments share a directory")
	}
	first.release()
	reopened, err := lockManagedProfile(root, environmentID, ref)
	if err != nil {
		t.Fatal(err)
	}
	reopened.release()
	content, err := os.ReadFile(sentinel)
	if err != nil || string(content) != "synthetic-retained" {
		t.Fatal("closing directory lock modified saved data")
	}
	if escaped, err := lockManagedProfile(root, environmentID, "../other/user-data"); err == nil {
		escaped.release()
		t.Fatal("unmanaged reference accepted")
	}
}

func TestManagedProfileLockRejectsHardlinkWithoutTruncatingExternalFile(t *testing.T) {
	root, environmentID := t.TempDir(), uuid.NewString()
	ref := "environments/" + environmentID + "/user-data"
	lock, err := lockManagedProfile(root, environmentID, ref)
	if err != nil {
		t.Fatal(err)
	}
	lockPath := filepath.Join(lock.path, ".prism-runtime.lock")
	lock.release()
	if err := os.Remove(lockPath); err != nil {
		t.Fatal(err)
	}
	external := filepath.Join(t.TempDir(), "synthetic-external")
	if err := os.WriteFile(external, []byte("do-not-truncate"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Link(external, lockPath); err != nil {
		t.Skipf("hardlink fixture unavailable: %v", err)
	}
	if unsafe, err := lockManagedProfile(root, environmentID, ref); err == nil {
		unsafe.release()
		t.Fatal("hardlinked lock accepted")
	}
	content, err := os.ReadFile(external)
	if err != nil || string(content) != "do-not-truncate" {
		t.Fatal("unsafe lock changed external synthetic data")
	}
}

func TestManagedProfileRejectsExistingSharedBrowserDataFiles(t *testing.T) {
	root, aID, bID := t.TempDir(), uuid.NewString(), uuid.NewString()
	lockA, err := lockManagedProfile(root, aID, "environments/"+aID+"/user-data")
	if err != nil {
		t.Fatal(err)
	}
	directoryA := lockA.path
	lockA.release()
	lockB, err := lockManagedProfile(root, bID, "environments/"+bID+"/user-data")
	if err != nil {
		t.Fatal(err)
	}
	directoryB := lockB.path
	lockB.release()
	fileA, fileB := filepath.Join(directoryA, "synthetic-browser-data"), filepath.Join(directoryB, "synthetic-browser-data")
	if err := os.WriteFile(fileA, []byte("synthetic-shared-data"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Link(fileA, fileB); err != nil {
		t.Skipf("hardlink fixture unavailable: %v", err)
	}
	for _, environmentID := range []string{aID, bID} {
		if unsafe, err := lockManagedProfile(root, environmentID, "environments/"+environmentID+"/user-data"); err == nil {
			unsafe.release()
			t.Fatal("shared physical browser data accepted as independent")
		}
	}
	content, err := os.ReadFile(fileA)
	if err != nil || string(content) != "synthetic-shared-data" {
		t.Fatal("rejected shared file was changed")
	}
}

func TestManagedDirectoryGuardPreservesAncestorBoundaryAndAtomicBrowserWrites(t *testing.T) {
	root, environmentID := t.TempDir(), uuid.NewString()
	lock, err := lockManagedProfile(root, environmentID, "environments/"+environmentID+"/user-data")
	if err != nil {
		t.Fatal(err)
	}
	defer lock.release()
	wide, err := windows.UTF16PtrFromString(filepath.Dir(lock.path))
	if err != nil {
		t.Fatal(err)
	}
	handle, err := windows.CreateFile(wide, windows.GENERIC_WRITE, windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE, nil, windows.OPEN_EXISTING, windows.FILE_FLAG_BACKUP_SEMANTICS|windows.FILE_FLAG_OPEN_REPARSE_POINT, 0)
	if err == nil {
		windows.CloseHandle(handle)
		t.Fatal("profile ancestor can still be opened for in-place reparse mutation")
	}
	if !errors.Is(err, windows.ERROR_SHARING_VIOLATION) {
		t.Fatalf("directory guard fixture failed for a different reason: %v", err)
	}
	if err := os.WriteFile(filepath.Join(lock.path, "synthetic-writable-child"), []byte("ordinary file writes still work"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(filepath.Join(lock.path, "synthetic-writable-child"), filepath.Join(lock.path, "Local State")); err != nil {
		t.Fatal("Chromium atomic root-file rename blocked", err)
	}
	if err := os.Rename(lock.path, lock.path+"-replacement"); err == nil {
		t.Fatal("live data root replacement permitted")
	}
}

func TestManagedDirectoryGuardMaintenancePinsStillDenyRootWrites(t *testing.T) {
	root := t.TempDir()
	release, err := pinManagedDirectories(root)
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	wide, _ := windows.UTF16PtrFromString(root)
	handle, err := windows.CreateFile(wide, windows.GENERIC_WRITE, windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE, nil, windows.OPEN_EXISTING, windows.FILE_FLAG_BACKUP_SEMANTICS|windows.FILE_FLAG_OPEN_REPARSE_POINT, 0)
	if err == nil {
		windows.CloseHandle(handle)
		t.Fatal("maintenance root opened for in-place reparse mutation")
	}
	if !errors.Is(err, windows.ERROR_SHARING_VIOLATION) {
		t.Fatal("unexpected maintenance pin failure", err)
	}
}
