//go:build windows

// Package desktopbase owns the desktop/installer boundary, not browser processes.
package desktopbase

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/wailsapp/go-webview2/webviewloader"
	"golang.org/x/sys/windows"
)

const MinimumRuntimeVersion = "94.0.992.31" // Wails v2.16.0 SDK minimum, not an installed-version claim.
const RuntimeHelp = "需要 Microsoft Edge WebView2 Runtime（94.0.992.31 或更新版）。请从 https://developer.microsoft.com/microsoft-edge/webview2/ 安装 Evergreen Runtime 后重试；不会自动下载，原工作区数据未修改。"

func DefaultRoot() (string, error) {
	local, err := windows.KnownFolderPath(windows.FOLDERID_LocalAppData, 0)
	if err != nil {
		return "", err
	}
	return filepath.Join(local, "PrismBrowser"), nil
}
func ValidateRuntimeVersion(version string) error {
	if version == "" {
		return errors.New(RuntimeHelp)
	}
	comparison, err := webviewloader.CompareBrowserVersions(version, MinimumRuntimeVersion)
	if err != nil || comparison < 0 {
		return errors.New(RuntimeHelp)
	}
	return nil
}
func CheckRuntime() error {
	version, err := webviewloader.GetAvailableCoreWebView2BrowserVersionString("")
	if err != nil {
		return errors.New(RuntimeHelp)
	}
	return ValidateRuntimeVersion(version)
}

// ValidatePath checks the complete ancestor chain; junctions are reparse points too.
func ValidatePath(path string) error {
	path, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	for {
		p, err := windows.UTF16PtrFromString(path)
		if err != nil {
			return err
		}
		attributes, err := windows.GetFileAttributes(p)
		if err != nil && !errors.Is(err, windows.ERROR_FILE_NOT_FOUND) && !errors.Is(err, windows.ERROR_PATH_NOT_FOUND) {
			return err
		}
		if err == nil && attributes&windows.FILE_ATTRIBUTE_REPARSE_POINT != 0 {
			return errors.New("目录包含链接或重解析点，未继续操作。")
		}
		parent := filepath.Dir(path)
		if parent == path {
			return nil
		}
		path = parent
	}
}
func ValidateTree(root string) error {
	if err := ValidatePath(root); err != nil {
		return err
	}
	if _, err := os.Lstat(root); os.IsNotExist(err) {
		return nil
	}
	return filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		return ValidatePath(path)
	})
}

type Lock struct {
	handle  windows.Handle
	release func()
}

func Acquire(root string) (*Lock, error) {
	root, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	lockPath := root + ".maintenance.lock"
	if err := ValidatePath(root); err != nil {
		return nil, err
	}
	if err := ValidateTree(root); err != nil {
		return nil, err
	}
	if err := ValidatePath(lockPath); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Dir(root), 0700); err != nil {
		return nil, err
	}
	release, err := PinDirectories(filepath.Dir(root))
	if err != nil {
		return nil, err
	}
	p, err := windows.UTF16PtrFromString(lockPath)
	if err != nil {
		release()
		return nil, err
	}
	handle, err := windows.CreateFile(p, windows.GENERIC_READ|windows.GENERIC_WRITE, 0, nil, windows.OPEN_ALWAYS, windows.FILE_ATTRIBUTE_NORMAL, 0)
	if err != nil {
		release()
		return nil, errors.New("本机工作区正在使用或维护中。请正常关闭棱镜浏览器，再重试；没有结束其他程序。")
	}
	return &Lock{handle: handle, release: release}, nil
}

// PinDirectories denies directory rename/delete while resolving and operating on
// descendants. OPEN_REPARSE_POINT inspects the directory itself, not its target.
func PinDirectories(path string) (func(), error) {
	path, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	chain := []string{}
	for {
		chain = append(chain, path)
		parent := filepath.Dir(path)
		if parent == path {
			break
		}
		path = parent
	}
	handles := []windows.Handle{}
	release := func() {
		for i := len(handles) - 1; i >= 0; i-- {
			windows.CloseHandle(handles[i])
		}
		handles = nil
	}
	for i := len(chain) - 1; i >= 0; i-- {
		p, err := windows.UTF16PtrFromString(chain[i])
		if err != nil {
			release()
			return nil, err
		}
		// READ_ATTRIBUTES alone does not participate in Windows delete-sharing checks.
		// GENERIC_READ keeps directory rename/replacement denied until this handle closes.
		h, err := windows.CreateFile(p, windows.GENERIC_READ, windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE, nil, windows.OPEN_EXISTING, windows.FILE_FLAG_BACKUP_SEMANTICS|windows.FILE_FLAG_OPEN_REPARSE_POINT, 0)
		if err != nil {
			release()
			return nil, err
		}
		var info windows.ByHandleFileInformation
		if err = windows.GetFileInformationByHandle(h, &info); err != nil || info.FileAttributes&windows.FILE_ATTRIBUTE_REPARSE_POINT != 0 || info.FileAttributes&windows.FILE_ATTRIBUTE_DIRECTORY == 0 {
			windows.CloseHandle(h)
			release()
			return nil, errors.New("目录含链接或不能安全锁定，未继续操作。")
		}
		handles = append(handles, h)
	}
	return release, nil
}
func (lock *Lock) Close() error {
	if lock.handle == 0 {
		return nil
	}
	err := windows.CloseHandle(lock.handle)
	lock.handle = 0
	if lock.release != nil {
		lock.release()
		lock.release = nil
	}
	return err
}

// RemoveDefaultWorkspace has no path parameter or environment override. It is called
// only after the installer has obtained an explicit opt-in to remove this user's data.
func RemoveDefaultWorkspace() error {
	root, err := DefaultRoot()
	if err != nil {
		return err
	}
	return removeWorkspace(root)
}
func removeWorkspace(root string) error {
	lock, err := Acquire(root)
	if err != nil {
		return err
	}
	defer lock.Close()
	if err := ValidateTree(root); err != nil {
		return err
	}
	if _, err := os.Lstat(root); os.IsNotExist(err) {
		return nil
	}
	// Never RemoveAll. Pin each parent at the actual removal step; replacing a
	// directory during a wizard cannot redirect traversal into unrelated files.
	paths := []string{}
	err = filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if err := ValidatePath(path); err != nil {
			return err
		}
		paths = append(paths, path)
		return nil
	})
	if err != nil {
		return err
	}
	for i := len(paths) - 1; i >= 0; i-- {
		pin, err := PinDirectories(filepath.Dir(paths[i]))
		if err != nil {
			return err
		}
		if err := ValidatePath(paths[i]); err != nil {
			pin()
			return err
		}
		if err := os.Remove(paths[i]); err != nil {
			pin()
			return err
		}
		pin()
	}
	return nil
}

func InstallationRoot() (string, error) {
	root, err := DefaultRoot()
	return filepath.Join(filepath.Dir(root), "Programs", "PrismBrowserPreview"), err
}
func ShowError(message string) {
	text, _ := windows.UTF16PtrFromString(strings.TrimSpace(message))
	title, _ := windows.UTF16PtrFromString("棱镜浏览器 · 开发预览")
	windows.MessageBox(0, text, title, windows.MB_OK|windows.MB_ICONERROR)
}
