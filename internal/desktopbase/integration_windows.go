//go:build windows

package desktopbase

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

const productKey = `Software\Microsoft\Windows\CurrentVersion\Uninstall\PrismBrowserPreview`
const shortcutName = "棱镜浏览器 · 开发预览.lnk"

var stringValues = []string{"DisplayName", "DisplayVersion", "NumericVersion", "InstallLocation", "DisplayIcon", "UninstallString", "QuietUninstallString"}
var integerValues = []string{"NoModify", "NoRepair"}

type registryValue struct {
	exists  bool
	text    string
	number  uint32
	integer bool
}
type fileSnapshot struct {
	exists bool
	bytes  []byte
}

func snapshotRegistry(key registry.Key) (map[string]registryValue, error) {
	snapshot := map[string]registryValue{}
	for _, name := range stringValues {
		value, kind, err := key.GetStringValue(name)
		if err != nil && !errors.Is(err, registry.ErrNotExist) {
			return nil, err
		}
		if err == nil && kind != registry.SZ {
			return nil, errors.New("安装注册记录类型不匹配")
		}
		snapshot[name] = registryValue{exists: err == nil, text: value}
	}
	for _, name := range integerValues {
		value, kind, err := key.GetIntegerValue(name)
		if err != nil && !errors.Is(err, registry.ErrNotExist) {
			return nil, err
		}
		if err == nil && kind != registry.DWORD {
			return nil, errors.New("安装注册记录类型不匹配")
		}
		snapshot[name] = registryValue{exists: err == nil, number: uint32(value), integer: true}
	}
	return snapshot, nil
}
func restoreRegistry(key registry.Key, snapshot map[string]registryValue) error {
	var failures []error
	for name, value := range snapshot {
		var err error
		if !value.exists {
			err = key.DeleteValue(name)
			if errors.Is(err, registry.ErrNotExist) {
				err = nil
			}
		} else if value.integer {
			err = key.SetDWordValue(name, value.number)
		} else {
			err = key.SetStringValue(name, value.text)
		}
		failures = append(failures, err)
	}
	return errors.Join(failures...)
}
func readSnapshot(path string) (fileSnapshot, error) {
	if err := ValidatePath(path); err != nil {
		return fileSnapshot{}, err
	}
	bytes, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return fileSnapshot{}, nil
	}
	return fileSnapshot{exists: err == nil, bytes: bytes}, err
}
func replaceFile(path string, bytes []byte) error {
	pin, err := PinDirectories(filepath.Dir(path))
	if err != nil {
		return err
	}
	defer pin()
	if err := ValidatePath(path); err != nil {
		return err
	}
	file, err := os.CreateTemp(filepath.Dir(path), ".prism-entry-")
	if err != nil {
		return err
	}
	name := file.Name()
	defer os.Remove(name)
	if _, err = file.Write(bytes); err != nil {
		file.Close()
		return err
	}
	if err = file.Sync(); err != nil {
		file.Close()
		return err
	}
	if err = file.Close(); err != nil {
		return err
	}
	return os.Rename(name, path)
}
func restoreFile(path string, snapshot fileSnapshot) error {
	if snapshot.exists {
		return replaceFile(path, snapshot.bytes)
	}
	err := os.Remove(path)
	if os.IsNotExist(err) {
		return nil
	}
	return err
}

// A small reversible integration transaction. Registry rights and every entry's
// existing bytes are checked before publishing code. Unknown registry values/files
// are not overwritten. A failure is never translated to installer success.
func publishIntegration(source, root, version string, shortcuts []string, keyPath string, publish func() error) (result error) {
	key, existed, err := registry.CreateKey(registry.CURRENT_USER, keyPath, registry.QUERY_VALUE|registry.SET_VALUE|registry.WOW64_64KEY)
	if err != nil {
		return err
	}
	defer key.Close()
	defer func() {
		if result != nil && !existed {
			key.Close()
			result = errors.Join(result, registry.DeleteKey(registry.CURRENT_USER, keyPath))
		}
	}()
	oldValues, err := snapshotRegistry(key)
	if err != nil {
		return err
	}
	if oldValues["NumericVersion"].exists {
		oldParts := strings.Split(oldValues["NumericVersion"].text, ".")
		if len(oldParts) != 4 {
			return errors.New("安装版本记录无效")
		}
		oldRevision, err := strconv.Atoi(oldParts[3])
		if err != nil {
			return err
		}
		newRevision, err := strconv.Atoi(strings.TrimPrefix(version, "0.3.0-preview."))
		if err != nil {
			return err
		}
		if oldRevision > newRevision {
			return errors.New("不允许自动降级")
		}
	}
	paths := append(append([]string{}, shortcuts...), filepath.Join(root, "uninstall.exe"))
	oldFiles := map[string]fileSnapshot{}
	for _, path := range paths {
		oldFiles[path], err = readSnapshot(path)
		if err != nil {
			return err
		}
		if path != filepath.Join(root, "uninstall.exe") {
			pin, err := PinDirectories(filepath.Dir(path))
			if err != nil {
				return err
			}
			defer pin()
			probe, err := os.CreateTemp(filepath.Dir(path), ".prism-probe-")
			if err != nil {
				return err
			}
			name := probe.Name()
			probe.Close()
			if err = os.Remove(name); err != nil {
				return err
			}
		}
	}
	started := false
	defer func() {
		if result == nil {
			return
		}
		if started {
			failures := []error{result, restoreRegistry(key, oldValues)}
			for _, path := range paths {
				failures = append(failures, restoreFile(path, oldFiles[path]))
			}
			result = errors.Join(failures...)
		}
	}()
	newFiles := [][]byte{}
	for _, name := range []string{"desktop.lnk", "startmenu.lnk"} {
		bytes, err := os.ReadFile(filepath.Join(source, name))
		if err != nil {
			return err
		}
		newFiles = append(newFiles, bytes)
	}
	if len(shortcuts) != len(newFiles) {
		return errors.New("快捷方式清单无效")
	}
	started = true
	if err = publish(); err != nil {
		return err
	}
	for i, path := range shortcuts {
		if err = replaceFile(path, newFiles[i]); err != nil {
			return err
		}
	}
	executable := filepath.Join(root, "versions", version, "prism-browser.exe")
	values := map[string]string{"DisplayName": "棱镜浏览器 · 开发预览", "DisplayVersion": version, "NumericVersion": "0.3.0." + strings.TrimPrefix(version, "0.3.0-preview."), "InstallLocation": root, "DisplayIcon": executable, "UninstallString": fmt.Sprintf(`"%s"`, filepath.Join(root, "uninstall.exe")), "QuietUninstallString": fmt.Sprintf(`"%s" /S`, filepath.Join(root, "uninstall.exe"))}
	for _, name := range stringValues {
		if err = key.SetStringValue(name, values[name]); err != nil {
			return err
		}
	}
	for _, name := range integerValues {
		if err = key.SetDWordValue(name, 1); err != nil {
			return err
		}
	}
	return nil
}
func cleanIntegration(root string, shortcuts []string, keyPath string, clean func() error) error {
	key, err := registry.OpenKey(registry.CURRENT_USER, keyPath, registry.QUERY_VALUE|registry.SET_VALUE|registry.WOW64_64KEY)
	if err != nil {
		return err
	}
	defer key.Close()
	location, _, err := key.GetStringValue("InstallLocation")
	if err != nil || !strings.EqualFold(location, root) {
		return errors.New("安装位置不属于当前产品")
	}
	for _, path := range shortcuts {
		pin, err := PinDirectories(filepath.Dir(path))
		if err != nil {
			return err
		}
		defer pin()
		if err = ValidatePath(path); err != nil {
			return err
		}
	}
	if err = clean(); err != nil {
		return err
	}
	for _, path := range shortcuts {
		if err = os.Remove(path); err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	// Preserve registration and uninstaller for a retry until the selected data
	// policy has also completed. FinishUninstall is a separate final commit.
	return nil
}

func finishIntegration(root, keyPath string) error {
	key, err := registry.OpenKey(registry.CURRENT_USER, keyPath, registry.QUERY_VALUE|windows.DELETE|registry.WOW64_64KEY)
	if err != nil {
		return err
	}
	location, _, readErr := key.GetStringValue("InstallLocation")
	key.Close()
	if readErr != nil || !strings.EqualFold(location, root) {
		return errors.New("安装位置不属于当前产品")
	}
	pin, err := PinDirectories(root)
	if err != nil {
		return err
	}
	defer pin()
	path := filepath.Join(root, "uninstall.exe")
	snapshot, err := readSnapshot(path)
	if err != nil || !snapshot.exists {
		return errors.New("卸载重试入口缺失")
	}
	if err = os.Remove(path); err != nil {
		return err
	}
	if err = registry.DeleteKey(registry.CURRENT_USER, keyPath); err != nil {
		return errors.Join(err, replaceFile(path, snapshot.bytes))
	}
	pin()
	// Empty-container removal is optional. Unknown files/ACLs may retain the
	// directory, but all distributed code and registration are already removed.
	_ = os.Remove(root)
	return nil
}
