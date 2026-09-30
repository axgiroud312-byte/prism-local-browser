//go:build windows

package kernel

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"golang.org/x/sys/windows"
)

func TestArchiveJunctionCannotWriteOutsideAndPinsDenyReplacement(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	junction := filepath.Join(root, "package")
	command := exec.Command("cmd.exe", "/d", "/c", "mklink", "/J", junction, outside)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("junction fixture: %v %s", err, output)
	}
	if _, _, err := extractArchive(context.Background(), zipFixture(t, "package/chrome.exe"), root); err == nil {
		t.Fatal("junction accepted")
	}
	if entries, _ := os.ReadDir(outside); len(entries) != 0 {
		t.Fatal("extraction wrote outside its root")
	}
	os.Remove(junction)
	release, err := EnsureDirectory(root, "package")
	if err != nil {
		t.Fatal(err)
	}
	if err = os.Rename(filepath.Join(root, "package"), filepath.Join(root, "renamed")); err == nil {
		t.Fatal("pinned directory replaced")
	}
	release()
}

func TestPinnedFilesDenyWritesAndDoNotFollowReparsePoints(t *testing.T) {
	root := t.TempDir()
	bytes := []byte("synthetic")
	os.WriteFile(filepath.Join(root, "chrome.exe"), bytes, 0600)
	hash, _ := fileHash(filepath.Join(root, "chrome.exe"))
	release, err := PinFiles(root, map[string]string{"chrome.exe": hash})
	if err != nil {
		t.Fatal(err)
	}
	wide, _ := windows.UTF16PtrFromString(filepath.Join(root, "chrome.exe"))
	handle, err := windows.CreateFile(wide, windows.GENERIC_WRITE, windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE, nil, windows.OPEN_EXISTING, 0, 0)
	if err == nil {
		windows.CloseHandle(handle)
		t.Fatal("selected binary writable during verification/probe")
	}
	if err = os.Rename(filepath.Join(root, "chrome.exe"), filepath.Join(root, "replacement.exe")); err == nil {
		t.Fatal("selected binary replaceable")
	}
	release()
}
