//go:build windows

package main

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"runtime"
	"strings"
	"unsafe"

	"golang.org/x/sys/windows"
)

func lockLabStation(name string) (func(), error) {
	runtime.LockOSThread()
	key := sha256.Sum256([]byte(strings.ToLower(name)))
	mutexName, _ := windows.UTF16PtrFromString(fmt.Sprintf(`Local\Prism-NetworkLab-Station-%x`, key))
	h, err := windows.CreateMutex(nil, false, mutexName)
	if err != nil && err != windows.ERROR_ALREADY_EXISTS {
		runtime.UnlockOSThread()
		return nil, err
	}
	status, err := windows.WaitForSingleObject(h, 10000)
	if err != nil || (status != windows.WAIT_OBJECT_0 && status != windows.WAIT_ABANDONED) {
		windows.CloseHandle(h)
		runtime.UnlockOSThread()
		return nil, fmt.Errorf("station-acl-lock-unavailable")
	}
	return func() { windows.ReleaseMutex(h); windows.CloseHandle(h); runtime.UnlockOSThread() }, nil
}

// Opt-in is separate from the ordinary synthetic experiment because this
// changes an existing per-logon USER object. Only the newly generated SID is
// added. Cooperating copies serialize their read/modify/verify operations;
// this does not claim atomicity against unrelated external ACL writers.
func grantExistingSyntheticWindowStation(sid *windows.SID, recordPath string) (cleanup func() error, resultErr error) {
	if os.Getenv("PRISM_NETWORK_STATION_ACL") != "1" {
		return nil, fmt.Errorf("temporary-window-station-acl-not-authorized")
	}
	dll := windows.NewLazySystemDLL("user32.dll")
	h, _, err := dll.NewProc("CreateWindowStationW").Call(0, 0, windows.READ_CONTROL|windows.WRITE_DAC|2, 0)
	if h == 0 {
		return nil, fmt.Errorf("open-window-station: %w", err)
	}
	defer func() {
		if resultErr != nil {
			dll.NewProc("CloseWindowStation").Call(h)
		}
	}()
	name := make([]uint16, 256)
	var size uint32
	ok, _, err := dll.NewProc("GetUserObjectInformationW").Call(h, 2, uintptr(unsafe.Pointer(&name[0])), uintptr(len(name)*2), uintptr(unsafe.Pointer(&size)))
	if ok == 0 {
		return nil, fmt.Errorf("window-station-name: %w", err)
	}
	if !strings.HasPrefix(strings.ToLower(windows.UTF16ToString(name)), "service-") {
		return nil, fmt.Errorf("not-a-noninteractive-service-window-station")
	}
	var flags struct{ Inherit, Reserved, Flags uint32 }
	ok, _, err = dll.NewProc("GetUserObjectInformationW").Call(h, 1, uintptr(unsafe.Pointer(&flags)), unsafe.Sizeof(flags), uintptr(unsafe.Pointer(&size)))
	if ok == 0 {
		return nil, fmt.Errorf("window-station-flags: %w", err)
	}
	if flags.Flags&1 != 0 {
		return nil, fmt.Errorf("interactive-window-station-refused")
	}
	stationName := windows.UTF16ToString(name)
	unlock, err := lockLabStation(stationName)
	if err != nil {
		return nil, err
	}
	defer unlock()
	sd, err := windows.GetSecurityInfo(windows.Handle(h), windows.SE_WINDOW_OBJECT, windows.DACL_SECURITY_INFORMATION)
	if err != nil {
		return nil, err
	}
	if strings.Contains(sd.String(), sid.String()) {
		return nil, fmt.Errorf("test-sid-already-present")
	}
	if err = os.WriteFile(recordPath, []byte("Original DACL (local recovery only):\n"+sd.String()+"\nTest SID: "+sid.String()+"\n"), 0600); err != nil {
		return nil, err
	}
	acl, _, err := sd.DACL()
	if err != nil {
		return nil, err
	}
	trustee := windows.TRUSTEE{TrusteeForm: windows.TRUSTEE_IS_SID, TrusteeType: windows.TRUSTEE_IS_UNKNOWN, TrusteeValue: windows.TrusteeValueFromSID(sid)}
	merged, err := windows.ACLFromEntries([]windows.EXPLICIT_ACCESS{{AccessPermissions: windows.GENERIC_READ | 8, AccessMode: windows.GRANT_ACCESS, Trustee: trustee}}, acl)
	if err != nil {
		return nil, err
	}
	if err = windows.SetSecurityInfo(windows.Handle(h), windows.SE_WINDOW_OBJECT, windows.DACL_SECURITY_INFORMATION, nil, nil, merged, nil); err != nil {
		return nil, err
	}
	return func() error {
		defer dll.NewProc("CloseWindowStation").Call(h)
		unlock, e := lockLabStation(stationName)
		if e != nil {
			return e
		}
		defer unlock()
		current, e := windows.GetSecurityInfo(windows.Handle(h), windows.SE_WINDOW_OBJECT, windows.DACL_SECURITY_INFORMATION)
		if e != nil {
			return e
		}
		currentACL, _, e := current.DACL()
		if e != nil {
			return e
		}
		revoked, e := windows.ACLFromEntries([]windows.EXPLICIT_ACCESS{{AccessMode: windows.REVOKE_ACCESS, Trustee: trustee}}, currentACL)
		if e != nil {
			return e
		}
		if e = windows.SetSecurityInfo(windows.Handle(h), windows.SE_WINDOW_OBJECT, windows.DACL_SECURITY_INFORMATION, nil, nil, revoked, nil); e != nil {
			return e
		}
		check, e := windows.GetSecurityInfo(windows.Handle(h), windows.SE_WINDOW_OBJECT, windows.DACL_SECURITY_INFORMATION)
		if e != nil {
			return e
		}
		if strings.Contains(check.String(), sid.String()) {
			return fmt.Errorf("temporary-window-station-ace-still-present")
		}
		file, e := os.OpenFile(recordPath, os.O_APPEND|os.O_WRONLY, 0600)
		if e != nil {
			return e
		}
		_, e = file.WriteString("Temporary SID revoked and read back.\n")
		return errors.Join(e, file.Close())
	}, nil
}

func createSyntheticWindowStation(sid *windows.SID) (func(), error) {
	u, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		return nil, err
	}
	sd, err := windows.SecurityDescriptorFromString("D:P(A;;GA;;;" + u.User.Sid.String() + ")(A;;GA;;;SY)(A;;GA;;;" + sid.String() + ")")
	if err != nil {
		return nil, err
	}
	sa := windows.SecurityAttributes{Length: uint32(unsafe.Sizeof(windows.SecurityAttributes{})), SecurityDescriptor: sd}
	dll := windows.NewLazySystemDLL("user32.dll")
	// CREATE_ONLY prevents taking over or editing a pre-existing window station.
	h, _, err := dll.NewProc("CreateWindowStationW").Call(0, 1, windows.GENERIC_ALL, uintptr(unsafe.Pointer(&sa)))
	if h == 0 {
		return nil, fmt.Errorf("create-only-window-station: %w", err)
	}
	return func() { dll.NewProc("CloseWindowStation").Call(h) }, nil
}

type desktopObservation struct {
	Stage string `json:"stage"`
	Error uint32 `json:"error"`
}

type windowObservation struct {
	Visible   int    `json:"visible"`
	Minimized int    `json:"minimized"`
	Error     string `json:"error,omitempty"`
}

// Query only top-level Chromium widgets owned by this exact live browser PID.
// No input, focus, movement, activation or window-state change is performed.
func browserWindows(pid uint32) (r windowObservation) {
	dll := windows.NewLazySystemDLL("user32.dll")
	callback := windows.NewCallback(func(handle, parameter uintptr) uintptr {
		var owner uint32
		dll.NewProc("GetWindowThreadProcessId").Call(handle, uintptr(unsafe.Pointer(&owner)))
		if owner != pid {
			return 1
		}
		name := make([]uint16, 128)
		n, _, _ := dll.NewProc("GetClassNameW").Call(handle, uintptr(unsafe.Pointer(&name[0])), uintptr(len(name)))
		if n == 0 || !strings.HasPrefix(windows.UTF16ToString(name), "Chrome_WidgetWin_") {
			return 1
		}
		if visible, _, _ := dll.NewProc("IsWindowVisible").Call(handle); visible != 0 {
			r.Visible++
		}
		if minimized, _, _ := dll.NewProc("IsIconic").Call(handle); minimized != 0 {
			r.Minimized++
		}
		return 1
	})
	if ok, _, _ := dll.NewProc("EnumWindows").Call(callback, 0); ok == 0 {
		r.Error = "window-enumeration-failed"
	}
	return
}

// Exercise the documented object operations needed by the browser broker in
// a disposable helper. Do not edit any existing desktop/window-station ACL.
func probeDesktop() []desktopObservation {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	dll := windows.NewLazySystemDLL("user32.dll")
	result := []desktopObservation{}
	add := func(stage string, err error) bool {
		code := uint32(0)
		if e, ok := err.(windows.Errno); ok {
			code = uint32(e)
		} else if err != nil {
			code = 0xffffffff
		}
		result = append(result, desktopObservation{stage, code})
		return code == 0
	}
	winsta, _, err := dll.NewProc("GetProcessWindowStation").Call()
	if winsta == 0 {
		add("get-window-station", err)
		return result
	}
	sd, err := windows.GetSecurityInfo(windows.Handle(winsta), windows.SE_WINDOW_OBJECT, windows.DACL_SECURITY_INFORMATION)
	if !add("read-window-station-dacl", err) {
		return result
	}
	absolute, err := sd.ToAbsolute()
	if !add("window-station-absolute-sd", err) {
		return result
	}
	sa := windows.SecurityAttributes{Length: uint32(unsafe.Sizeof(windows.SecurityAttributes{})), SecurityDescriptor: absolute}
	alt, _, err := dll.NewProc("CreateWindowStationW").Call(0, 0, windows.GENERIC_READ|8, uintptr(unsafe.Pointer(&sa)))
	if alt == 0 && err == windows.ERROR_ACCESS_DENIED {
		alt, _, err = dll.NewProc("CreateWindowStationW").Call(0, 0, 2|8, uintptr(unsafe.Pointer(&sa)))
	}
	if alt == 0 {
		add("create-window-station", err)
		return result
	}
	add("create-window-station", nil)
	defer dll.NewProc("CloseWindowStation").Call(alt)
	desktop, _, err := dll.NewProc("GetThreadDesktop").Call(uintptr(windows.GetCurrentThreadId()))
	if desktop == 0 {
		add("get-desktop", err)
		return result
	}
	sd, err = windows.GetSecurityInfo(windows.Handle(desktop), windows.SE_WINDOW_OBJECT, windows.DACL_SECURITY_INFORMATION)
	if !add("read-desktop-dacl", err) {
		return result
	}
	dacl, _, err := sd.DACL()
	if !add("desktop-dacl", err) {
		return result
	}
	restricted, err := windows.CreateWellKnownSid(windows.WinRestrictedCodeSid)
	if !add("restricted-sid", err) {
		return result
	}
	mask := windows.ACCESS_MASK(windows.WRITE_DAC | windows.WRITE_OWNER | windows.DELETE | 4 | 2 | 8 | 32 | 16 | 256)
	dacl, err = windows.ACLFromEntries([]windows.EXPLICIT_ACCESS{{AccessPermissions: mask, AccessMode: windows.DENY_ACCESS, Trustee: windows.TRUSTEE{TrusteeForm: windows.TRUSTEE_IS_SID, TrusteeType: windows.TRUSTEE_IS_WELL_KNOWN_GROUP, TrusteeValue: windows.TrusteeValueFromSID(restricted)}}}, dacl)
	if !add("desktop-restricted-deny", err) {
		return result
	}
	absolute, err = sd.ToAbsolute()
	if !add("desktop-absolute-sd", err) {
		return result
	}
	if !add("desktop-set-dacl", absolute.SetDACL(dacl, true, false)) {
		return result
	}
	ok, _, err := dll.NewProc("SetProcessWindowStation").Call(alt)
	if ok == 0 {
		add("switch-window-station", err)
		return result
	}
	defer dll.NewProc("SetProcessWindowStation").Call(winsta)
	name, _ := windows.UTF16PtrFromString("prism-feasibility-desktop")
	sa.SecurityDescriptor = absolute
	created, _, err := dll.NewProc("CreateDesktopW").Call(uintptr(unsafe.Pointer(name)), 0, 0, 0, 2|1|windows.READ_CONTROL|windows.WRITE_DAC|windows.WRITE_OWNER, uintptr(unsafe.Pointer(&sa)))
	if created == 0 {
		add("create-desktop", err)
		return result
	}
	defer dll.NewProc("CloseDesktop").Call(created)
	add("create-desktop", nil)
	return result
}
