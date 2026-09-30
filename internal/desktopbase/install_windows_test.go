//go:build windows

package desktopbase

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/google/uuid"
	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

func payloadFixture(t *testing.T, version string) string {
	t.Helper()
	directory := t.TempDir()
	receipt := releaseReceipt{Version: version}
	for _, name := range distributedNames {
		bytes := []byte(version + " synthetic " + name)
		if err := os.WriteFile(filepath.Join(directory, name), bytes, 0600); err != nil {
			t.Fatal(err)
		}
		digest := sha256.Sum256(bytes)
		receipt.Files = append(receipt.Files, struct {
			Name   string `json:"name"`
			SHA256 string `json:"sha256"`
		}{name, hex.EncodeToString(digest[:])})
	}
	bytes, _ := json.Marshal(receipt)
	if err := os.WriteFile(filepath.Join(directory, "release.json"), bytes, 0600); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"uninstall.exe", "desktop.lnk", "startmenu.lnk"} {
		os.WriteFile(filepath.Join(directory, name), []byte(version+" synthetic "+name), 0600)
	}
	return directory
}
func assertFile(t *testing.T, path, expected string) {
	t.Helper()
	bytes, err := os.ReadFile(path)
	if err != nil || string(bytes) != expected {
		t.Fatalf("file not preserved %s: %v", filepath.Base(path), err)
	}
}
func TestProgramPublishUpgradeRepairAndCorruptPayloadFailure(t *testing.T) {
	base := t.TempDir()
	root := filepath.Join(base, "Programs", "Prism")
	data := filepath.Join(base, "app.db")
	os.WriteFile(data, []byte("same synthetic ID and seed"), 0600)
	first := payloadFixture(t, "0.3.0-preview.1")
	second := payloadFixture(t, "0.3.0-preview.2")
	if err := publishProgram(first, root); err != nil {
		t.Fatal(err)
	}
	exe := filepath.Join(root, "versions", "0.3.0-preview.1", "prism-browser.exe")
	if err := os.Remove(exe); err != nil {
		t.Fatal(err)
	}
	if err := publishProgram(first, root); err != nil {
		t.Fatal(err)
	}
	assertFile(t, exe, "0.3.0-preview.1 synthetic prism-browser.exe")
	if err := publishProgram(second, root); err != nil {
		t.Fatal(err)
	}
	assertFile(t, exe, "0.3.0-preview.1 synthetic prism-browser.exe")
	assertFile(t, data, "same synthetic ID and seed")
	os.WriteFile(filepath.Join(second, "prism-browser.exe"), []byte("damaged"), 0600)
	if err := publishProgram(second, root); err == nil {
		t.Fatal("corrupt payload accepted")
	}
	assertFile(t, filepath.Join(root, "versions", "0.3.0-preview.2", "prism-browser.exe"), "0.3.0-preview.2 synthetic prism-browser.exe")
}
func TestDirectoryPinAndStartupRejectJunction(t *testing.T) {
	base := t.TempDir()
	root := filepath.Join(base, "workspace")
	outside := filepath.Join(base, "outside")
	os.Mkdir(root, 0700)
	os.Mkdir(outside, 0700)
	pin, err := PinDirectories(root)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.Rename(root, root+".renamed"); err == nil {
		pin()
		t.Fatal("pinned directory was renamed")
	}
	pin()
	pin()
	link := filepath.Join(root, "workbench-webview")
	if output, err := exec.Command("cmd.exe", "/c", "mklink", "/J", link, outside).CombinedOutput(); err != nil {
		t.Fatalf("fixture: %v %s", err, output)
	}
	t.Cleanup(func() { os.Remove(link) })
	if lock, err := Acquire(root); err == nil {
		lock.Close()
		t.Fatal("startup accepted nested junction")
	}
}
func TestProgramCleanupRejectsJunctionAndPreservesUnknownFiles(t *testing.T) {
	root := filepath.Join(t.TempDir(), "program")
	source := payloadFixture(t, "0.3.0-preview.1")
	if err := publishProgram(source, root); err != nil {
		t.Fatal(err)
	}
	unknown := filepath.Join(root, "versions", "0.3.0-preview.1", "user-note.txt")
	os.WriteFile(unknown, []byte("unrelated synthetic note"), 0600)
	outside := t.TempDir()
	sentinel := filepath.Join(outside, "LICENSE")
	os.WriteFile(sentinel, []byte("outside synthetic sentinel"), 0600)
	link := filepath.Join(root, "versions", "0.3.0-preview.2")
	if output, err := exec.Command("cmd.exe", "/c", "mklink", "/J", link, outside).CombinedOutput(); err != nil {
		t.Fatalf("fixture: %v %s", err, output)
	}
	if err := cleanProgram(root); err == nil {
		t.Fatal("cleanup accepted junction")
	}
	assertFile(t, sentinel, "outside synthetic sentinel")
	if err := os.Remove(link); err != nil {
		t.Fatal(err)
	}
	if err := cleanProgram(root); err != nil {
		t.Fatal(err)
	}
	assertFile(t, unknown, "unrelated synthetic note")
}
func TestProgramCleanupLockedFileReturnsFailure(t *testing.T) {
	root := filepath.Join(t.TempDir(), "program")
	if err := publishProgram(payloadFixture(t, "0.3.0-preview.1"), root); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "versions", "0.3.0-preview.1", "LICENSE")
	p, _ := windows.UTF16PtrFromString(path)
	h, err := windows.CreateFile(p, windows.GENERIC_READ, windows.FILE_SHARE_READ, nil, windows.OPEN_EXISTING, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	if err := cleanProgram(root); err == nil {
		windows.CloseHandle(h)
		t.Fatal("locked-file cleanup reported success")
	}
	windows.CloseHandle(h)
	if err := cleanProgram(root); err != nil {
		t.Fatal(err)
	}
}
func TestIntegrationRollbackKeepsRealRegistryAndShortcutBytes(t *testing.T) {
	keyPath := `Software\PrismBrowserTests\` + uuid.NewString()
	key, _, err := registry.CreateKey(registry.CURRENT_USER, keyPath, registry.ALL_ACCESS)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { key.Close(); registry.DeleteKey(registry.CURRENT_USER, keyPath) })
	root := filepath.Join(t.TempDir(), "program")
	os.Mkdir(root, 0700)
	shortcuts := []string{filepath.Join(t.TempDir(), "desktop.lnk"), filepath.Join(t.TempDir(), "startmenu.lnk")}
	key.SetStringValue("NumericVersion", "0.3.0.1")
	key.SetStringValue("DisplayVersion", "0.3.0-preview.1")
	key.SetStringValue("InstallLocation", root)
	for _, path := range shortcuts {
		os.WriteFile(path, []byte("old shortcut bytes"), 0600)
	}
	uninstaller := filepath.Join(root, "uninstall.exe")
	os.WriteFile(uninstaller, []byte("old uninstaller"), 0600)
	err = publishIntegration(payloadFixture(t, "0.3.0-preview.2"), root, "0.3.0-preview.2", shortcuts, keyPath, func() error {
		os.WriteFile(uninstaller, []byte("partial new uninstaller"), 0600)
		return errors.New("injected publish failure")
	})
	if err == nil {
		t.Fatal("publish failure reported success")
	}
	version, _, err := key.GetStringValue("DisplayVersion")
	if err != nil || version != "0.3.0-preview.1" {
		t.Fatal("registry changed on failure")
	}
	for _, path := range shortcuts {
		assertFile(t, path, "old shortcut bytes")
	}
	assertFile(t, uninstaller, "old uninstaller")
}
