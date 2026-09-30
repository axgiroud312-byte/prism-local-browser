//go:build windows

package desktopbase

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	ole "github.com/go-ole/go-ole"
	"github.com/go-ole/go-ole/oleutil"
)

// Build only after the installed target exists. A link staged before a fresh
// target can lose its path on a user whose shell namespace is not initialized.
// Read back the saved link rather than treating COM's Save as proof of success.
func shortcutBytes(source, executable string) ([]byte, error) {
	info, err := os.Stat(executable)
	if err != nil || info.IsDir() || !filepath.IsAbs(executable) {
		return nil, errors.New("快捷方式目标程序不存在")
	}
	directory, err := os.MkdirTemp(source, ".prism-shortcut-")
	if err != nil {
		return nil, err
	}
	defer os.Remove(directory)
	path := filepath.Join(directory, "entry.lnk")
	defer os.Remove(path)
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	if err = ole.CoInitializeEx(0, ole.COINIT_APARTMENTTHREADED); err != nil {
		var comError *ole.OleError
		// S_FALSE is a successful, already initialized apartment.
		if !errors.As(err, &comError) || comError.Code() != 1 {
			return nil, err
		}
	}
	defer ole.CoUninitialize()
	unknown, err := oleutil.CreateObject("WScript.Shell")
	if err != nil {
		return nil, err
	}
	defer unknown.Release()
	shell, err := unknown.QueryInterface(ole.IID_IDispatch)
	if err != nil {
		return nil, err
	}
	defer shell.Release()
	link, err := oleutil.CallMethod(shell, "CreateShortcut", path)
	if err != nil {
		return nil, err
	}
	defer link.Clear()
	for name, value := range map[string]string{"TargetPath": executable, "WorkingDirectory": filepath.Dir(executable), "IconLocation": executable} {
		result, err := oleutil.PutProperty(link.ToIDispatch(), name, value)
		if result != nil {
			result.Clear()
		}
		if err != nil {
			return nil, err
		}
	}
	result, err := oleutil.CallMethod(link.ToIDispatch(), "Save")
	if result != nil {
		result.Clear()
	}
	if err != nil {
		return nil, err
	}
	saved, err := oleutil.CallMethod(shell, "CreateShortcut", path)
	if err != nil {
		return nil, err
	}
	defer saved.Clear()
	for name, expected := range map[string]string{"TargetPath": executable, "WorkingDirectory": filepath.Dir(executable)} {
		value, err := oleutil.GetProperty(saved.ToIDispatch(), name)
		if err != nil {
			return nil, err
		}
		actual := value.ToString()
		value.Clear()
		if actual == "" || !strings.EqualFold(filepath.Clean(actual), filepath.Clean(expected)) {
			return nil, errors.New("快捷方式回读目标或工作目录不匹配，安装未完成")
		}
	}
	return os.ReadFile(path)
}
