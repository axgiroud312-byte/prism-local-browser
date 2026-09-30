//go:build windows

package desktopbase

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"unsafe"

	ole "github.com/go-ole/go-ole"
	"golang.org/x/sys/windows"
)

// IShellLinkW and IPersistFile have the documented COM method order. Never use
// WScript.Shell here: its ANSI path handling depends on the Windows code page.
type shellLinkVtbl struct {
	ole.IUnknownVtbl
	GetPath, GetIDList, SetIDList, GetDescription, SetDescription        uintptr
	GetWorkingDirectory, SetWorkingDirectory, GetArguments, SetArguments uintptr
	GetHotkey, SetHotkey, GetShowCmd, SetShowCmd                         uintptr
	GetIconLocation, SetIconLocation, SetRelativePath, Resolve, SetPath  uintptr
}
type persistFileVtbl struct {
	ole.IUnknownVtbl
	GetClassID, IsDirty, Load, Save, SaveCompleted, GetCurFile uintptr
}

func shellCall(action string, method uintptr, arguments ...uintptr) error {
	hr, _, _ := syscall.SyscallN(method, arguments...)
	if int32(hr) < 0 {
		return fmt.Errorf("Windows %s: HRESULT 0x%08x", action, uint32(hr))
	}
	return nil
}
func newShellLink() (*ole.IUnknown, error) {
	return ole.CreateInstance(ole.NewGUID("00021401-0000-0000-C000-000000000046"), ole.NewGUID("000214F9-0000-0000-C000-000000000046"))
}
func persistLink(link *ole.IUnknown, path string, load bool) error {
	var persist *ole.IUnknown
	if err := link.PutQueryInterface(ole.NewGUID("0000010B-0000-0000-C000-000000000046"), &persist); err != nil {
		return err
	}
	defer persist.Release()
	wide, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return err
	}
	table := (*persistFileVtbl)(unsafe.Pointer(persist.RawVTable))
	method, action, flag := table.Save, "IPersistFile.Save", uintptr(1)
	if load {
		method, action, flag = table.Load, "IPersistFile.Load", 0 // STGM_READ
	}
	err = shellCall(action, method, uintptr(unsafe.Pointer(persist)), uintptr(unsafe.Pointer(wide)), flag)
	runtime.KeepAlive(wide)
	return err
}
func linkPath(link *ole.IUnknown, workingDirectory bool) (string, error) {
	buffer := make([]uint16, 32768)
	table := (*shellLinkVtbl)(unsafe.Pointer(link.RawVTable))
	var err error
	if workingDirectory {
		err = shellCall("IShellLinkW.GetWorkingDirectory", table.GetWorkingDirectory, uintptr(unsafe.Pointer(link)), uintptr(unsafe.Pointer(&buffer[0])), uintptr(len(buffer)))
	} else {
		err = shellCall("IShellLinkW.GetPath", table.GetPath, uintptr(unsafe.Pointer(link)), uintptr(unsafe.Pointer(&buffer[0])), uintptr(len(buffer)), 0, 4) // SLGP_RAWPATH, no target search
	}
	return windows.UTF16ToString(buffer), err
}

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
	link, err := newShellLink()
	if err != nil {
		return nil, err
	}
	defer link.Release()
	table := (*shellLinkVtbl)(unsafe.Pointer(link.RawVTable))
	for _, entry := range []struct {
		action, value string
		method        uintptr
	}{{"IShellLinkW.SetPath", executable, table.SetPath}, {"IShellLinkW.SetWorkingDirectory", filepath.Dir(executable), table.SetWorkingDirectory}, {"IShellLinkW.SetIconLocation", executable, table.SetIconLocation}} {
		wide, err := windows.UTF16PtrFromString(entry.value)
		if err != nil {
			return nil, err
		}
		err = shellCall(entry.action, entry.method, uintptr(unsafe.Pointer(link)), uintptr(unsafe.Pointer(wide)), 0)
		runtime.KeepAlive(wide)
		if err != nil {
			return nil, err
		}
	}
	if err = persistLink(link, path, false); err != nil {
		return nil, err
	}
	saved, err := newShellLink()
	if err != nil {
		return nil, err
	}
	defer saved.Release()
	if err = persistLink(saved, path, true); err != nil {
		return nil, err
	}
	for _, workingDirectory := range []bool{false, true} {
		actual, err := linkPath(saved, workingDirectory)
		if err != nil {
			return nil, err
		}
		expected := executable
		if workingDirectory {
			expected = filepath.Dir(executable)
		}
		if actual == "" || !strings.EqualFold(filepath.Clean(actual), filepath.Clean(expected)) {
			return nil, errors.New("快捷方式回读目标或工作目录不匹配，安装未完成")
		}
	}
	return os.ReadFile(path)
}
