//go:build windows

package desktopbase

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/google/uuid"
	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

func TestUninstallTailFailureKeepsRetryThenCompletes(t *testing.T) {
	keyPath := `Software\PrismBrowserTests\` + uuid.NewString()
	key, _, err := registry.CreateKey(registry.CURRENT_USER, keyPath, registry.ALL_ACCESS)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { key.Close(); registry.DeleteKey(registry.CURRENT_USER, keyPath) })
	root := filepath.Join(t.TempDir(), "program")
	if err = publishProgram(payloadFixture(t, "0.3.0-preview.1"), root); err != nil {
		t.Fatal(err)
	}
	if err = key.SetStringValue("InstallLocation", root); err != nil {
		t.Fatal(err)
	}
	shortcuts := []string{filepath.Join(t.TempDir(), "desktop.lnk"), filepath.Join(t.TempDir(), "startmenu.lnk")}
	for _, path := range shortcuts {
		os.WriteFile(path, []byte("synthetic entry"), 0600)
	}
	p, _ := windows.UTF16PtrFromString(shortcuts[0])
	h, err := windows.CreateFile(p, windows.GENERIC_READ, windows.FILE_SHARE_READ, nil, windows.OPEN_EXISTING, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	clean := func() error { return cleanProgram(root) }
	if err = cleanIntegration(root, shortcuts, keyPath, clean); err == nil {
		windows.CloseHandle(h)
		t.Fatal("locked shortcut reported success")
	}
	windows.CloseHandle(h)
	uninstaller := filepath.Join(root, "uninstall.exe")
	assertFile(t, uninstaller, "0.3.0-preview.1 synthetic uninstall.exe")
	if _, _, err = key.GetStringValue("InstallLocation"); err != nil {
		t.Fatal("retry registration lost")
	}
	if err = cleanIntegration(root, shortcuts, keyPath, clean); err != nil {
		t.Fatal(err)
	}
	data := filepath.Join(t.TempDir(), "workspace")
	os.Mkdir(data, 0700)
	database := filepath.Join(data, "app.db")
	os.WriteFile(database, []byte("synthetic ID seed"), 0600)
	p, _ = windows.UTF16PtrFromString(database)
	h, err = windows.CreateFile(p, windows.GENERIC_READ, windows.FILE_SHARE_READ, nil, windows.OPEN_EXISTING, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	if err = removeWorkspace(data); err == nil {
		windows.CloseHandle(h)
		t.Fatal("locked data reported success")
	}
	windows.CloseHandle(h)
	assertFile(t, uninstaller, "0.3.0-preview.1 synthetic uninstall.exe")
	if _, _, err = key.GetStringValue("InstallLocation"); err != nil {
		t.Fatal("data-failure retry registration lost")
	}
	if err = removeWorkspace(data); err != nil {
		t.Fatal(err)
	}
	if err = finishIntegration(root, keyPath); err != nil {
		t.Fatal(err)
	}
	if _, err = os.Stat(uninstaller); !os.IsNotExist(err) {
		t.Fatal("successful uninstall left the executable")
	}
	if k, err := registry.OpenKey(registry.CURRENT_USER, keyPath, registry.QUERY_VALUE); err == nil {
		k.Close()
		t.Fatal("successful uninstall left registration")
	}
}
