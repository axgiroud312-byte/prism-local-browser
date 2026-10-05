//go:build windows

package kernel

import (
	"context"
	"errors"
	"fmt"
	"net"
	"runtime"
	"strings"
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

var networkUserenv = windows.NewLazySystemDLL("userenv.dll")
var networkUser32 = windows.NewLazySystemDLL("user32.dll")

type networkSecurityCapabilities struct {
	SID             *windows.SID
	Capabilities    uintptr
	Count, Reserved uint32
}

func deriveNetworkSID(name string) (*windows.SID, error) {
	n, err := windows.UTF16PtrFromString(name)
	if err != nil {
		return nil, err
	}
	var sid *windows.SID
	hr, _, _ := networkUserenv.NewProc("DeriveAppContainerSidFromAppContainerName").Call(uintptr(unsafe.Pointer(n)), uintptr(unsafe.Pointer(&sid)))
	if int32(hr) < 0 {
		return nil, fmt.Errorf("derive package HRESULT %x", uint32(hr))
	}
	defer windows.FreeSid(sid)
	return windows.StringToSid(sid.String())
}

func createNetworkContainer(name string, expected *windows.SID) error {
	n, err := windows.UTF16PtrFromString(name)
	if err != nil {
		return err
	}
	var sid *windows.SID
	hr, _, _ := networkUserenv.NewProc("CreateAppContainerProfile").Call(uintptr(unsafe.Pointer(n)), uintptr(unsafe.Pointer(n)), uintptr(unsafe.Pointer(n)), 0, 0, uintptr(unsafe.Pointer(&sid)))
	if int32(hr) < 0 {
		return fmt.Errorf("create package HRESULT %x", uint32(hr))
	}
	defer windows.FreeSid(sid)
	if !sid.Equals(expected) {
		return errors.New("created package identity mismatch")
	}
	return nil
}

func deleteNetworkContainer(intent NetworkSessionIntent) error {
	sid, err := deriveNetworkSID(intent.ContainerName)
	if err != nil || sid.String() != intent.PackageSID {
		return errors.New("saved package identity mismatch")
	}
	n, _ := windows.UTF16PtrFromString(intent.ContainerName)
	hr, _, _ := networkUserenv.NewProc("DeleteAppContainerProfile").Call(uintptr(unsafe.Pointer(n)))
	if int32(hr) >= 0 || uint32(hr) == 0x80070002 {
		exists, err := networkContainerExists(intent.PackageSID)
		if err != nil {
			return err
		}
		if exists {
			return errors.New("container registration still present")
		}
		return nil
	}
	return fmt.Errorf("delete package HRESULT %x", uint32(hr))
}

func networkContainerExists(sid string) (bool, error) {
	key, err := registry.OpenKey(registry.CURRENT_USER, `Software\Classes\Local Settings\Software\Microsoft\Windows\CurrentVersion\AppContainer\Mappings\`+sid, registry.QUERY_VALUE)
	if errors.Is(err, registry.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	key.Close()
	return true, nil
}

func networkLogonIdentity() (string, string, error) {
	// USER names the default station from AuthenticationId, not a logon group
	// inherited from a linked/restricted token. Encode that LUID in SID form for
	// the journal and LSA recovery query; it is never used as an ACL trustee.
	var size uint32
	token := windows.GetCurrentProcessToken()
	_ = windows.GetTokenInformation(token, windows.TokenStatistics, nil, 0, &size)
	if size < 16 || size > 65536 {
		return "", "", errors.New("logon statistics unavailable")
	}
	buffer := make([]byte, size)
	if err := windows.GetTokenInformation(token, windows.TokenStatistics, &buffer[0], size, &size); err != nil {
		return "", "", err
	}
	auth := (*[2]windows.LUID)(unsafe.Pointer(&buffer[0]))[1]
	return fmt.Sprintf("S-1-5-5-%d-%d", uint32(auth.HighPart), auth.LowPart), fmt.Sprintf("Service-0x%x-%x$", uint32(auth.HighPart), auth.LowPart), nil
}

func networkLogonEnded(logon string) (bool, error) {
	sid, err := windows.StringToSid(logon)
	if err != nil || !strings.HasPrefix(logon, "S-1-5-5-") || sid.SubAuthorityCount() != 3 {
		return false, errors.New("saved logon SID invalid")
	}
	luid := windows.LUID{HighPart: int32(sid.SubAuthority(1)), LowPart: sid.SubAuthority(2)}
	var data uintptr
	dll := windows.NewLazySystemDLL("secur32.dll")
	status, _, _ := dll.NewProc("LsaGetLogonSessionData").Call(uintptr(unsafe.Pointer(&luid)), uintptr(unsafe.Pointer(&data)))
	if data != 0 {
		dll.NewProc("LsaFreeReturnBuffer").Call(data)
	}
	if uint32(status) == 0xc000005f {
		return true, nil
	} // STATUS_NO_SUCH_LOGON_SESSION
	if status != 0 {
		return false, fmt.Errorf("saved logon query NTSTATUS %x", uint32(status))
	}
	return false, nil
}

// A logon LUID is unique only within one Windows boot. The kernel boot GUID,
// unlike a wall-clock-minus-uptime estimate, distinguishes reboot from a clock
// adjustment or a reused authentication LUID without modifying system state.
func networkBootIdentity() (string, error) {
	var info struct {
		Identifier [16]byte
		Firmware   uint32
		Flags      uint64
	}
	var size uint32
	status, _, _ := windows.NewLazySystemDLL("ntdll.dll").NewProc("NtQuerySystemInformation").Call(90, uintptr(unsafe.Pointer(&info)), unsafe.Sizeof(info), uintptr(unsafe.Pointer(&size)))
	if int32(status) < 0 || size < 20 || info.Identifier == [16]byte{} {
		return "", errors.New("Windows boot identity unavailable")
	}
	return fmt.Sprintf("%x", info.Identifier), nil
}

func openNetworkStation(name string, create bool) (windows.Handle, error) {
	n, err := windows.UTF16PtrFromString(name)
	if err != nil {
		return 0, err
	}
	var h uintptr
	if create {
		// NULL requests Windows' standard per-logon service station. Supplying
		// its reserved Service-* name explicitly is denied to ordinary users.
		// Its derived identity is journaled first and verified before any ACL edit.
		h, _, err = networkUser32.NewProc("CreateWindowStationW").Call(0, 0, windows.READ_CONTROL|windows.WRITE_DAC|2, 0)
	} else {
		h, _, err = networkUser32.NewProc("OpenWindowStationW").Call(uintptr(unsafe.Pointer(n)), 0, windows.READ_CONTROL|windows.WRITE_DAC|2)
	}
	if h == 0 {
		return 0, err
	}
	actualName := make([]uint16, 256)
	var nameSize uint32
	nameOK, _, _ := networkUser32.NewProc("GetUserObjectInformationW").Call(h, 2, uintptr(unsafe.Pointer(&actualName[0])), uintptr(len(actualName)*2), uintptr(unsafe.Pointer(&nameSize)))
	if nameOK == 0 || !strings.EqualFold(name, windows.UTF16ToString(actualName)) {
		networkUser32.NewProc("CloseWindowStation").Call(h)
		return 0, errors.New("derived window station identity mismatch")
	}
	var flags struct{ Inherit, Reserved, Flags uint32 }
	var size uint32
	ok, _, err := networkUser32.NewProc("GetUserObjectInformationW").Call(h, 1, uintptr(unsafe.Pointer(&flags)), unsafe.Sizeof(flags), uintptr(unsafe.Pointer(&size)))
	if ok == 0 || flags.Flags&1 != 0 || !strings.HasPrefix(name, "Service-") {
		networkUser32.NewProc("CloseWindowStation").Call(h)
		return 0, errors.New("noninteractive station not confirmed")
	}
	return windows.Handle(h), nil
}

func closeNetworkStation(handle windows.Handle) error {
	ok, _, err := networkUser32.NewProc("CloseWindowStation").Call(uintptr(handle))
	if ok == 0 {
		return fmt.Errorf("window station handle close not confirmed: %w", err)
	}
	return nil
}

func networkLowBox(sid *windows.SID) (windows.Token, error) {
	var original windows.Token
	if err := windows.OpenProcessToken(windows.CurrentProcess(), windows.TOKEN_QUERY|windows.TOKEN_DUPLICATE|windows.TOKEN_ASSIGN_PRIMARY, &original); err != nil {
		return 0, err
	}
	defer original.Close()
	attributes := windows.OBJECT_ATTRIBUTES{Length: uint32(unsafe.Sizeof(windows.OBJECT_ATTRIBUTES{}))}
	var primary windows.Token
	status, _, _ := windows.NewLazySystemDLL("ntdll.dll").NewProc("NtCreateLowBoxToken").Call(uintptr(unsafe.Pointer(&primary)), uintptr(original), windows.MAXIMUM_ALLOWED, uintptr(unsafe.Pointer(&attributes)), uintptr(unsafe.Pointer(sid)), 0, 0, 0, 0)
	if int32(status) < 0 {
		return 0, fmt.Errorf("lowbox NTSTATUS %x", uint32(status))
	}
	defer primary.Close()
	var token windows.Token
	err := windows.DuplicateTokenEx(primary, windows.TOKEN_QUERY|windows.TOKEN_IMPERSONATE, nil, windows.SecurityImpersonation, windows.TokenImpersonation, &token)
	return token, err
}

// Always await the synchronous thread callback, even after cancellation. A
// failed RevertToSelf retires the locked thread, never returns it to Go's pool.
func withNetworkIdentity(token windows.Token, action func() error) error {
	done := make(chan error, 1)
	go func() {
		runtime.LockOSThread()
		if err := windows.SetThreadToken(nil, token); err != nil {
			runtime.UnlockOSThread()
			done <- err
			return
		}
		err := action()
		if revoke := windows.RevertToSelf(); revoke != nil {
			done <- errors.Join(err, revoke)
			return
		}
		runtime.UnlockOSThread()
		done <- err
	}()
	return <-done
}

func networkListener(token windows.Token) (net.Listener, error) {
	var listener net.Listener
	err := withNetworkIdentity(token, func() (err error) { listener, err = net.Listen("tcp4", "127.0.0.1:0"); return err })
	if err != nil && listener != nil {
		listener.Close()
		listener = nil
	}
	return listener, err
}

func networkDial(token windows.Token, address string, ctx context.Context) (net.Conn, error) {
	var conn net.Conn
	err := withNetworkIdentity(token, func() (err error) {
		// An IP literal avoids handing asynchronous DNS work to another thread.
		conn, err = (&net.Dialer{}).DialContext(ctx, "tcp4", address)
		return err
	})
	if err != nil && conn != nil {
		conn.Close()
		conn = nil
	}
	return conn, err
}

func verifyNetworkProcess(process windows.Handle, sid *windows.SID) error {
	var token windows.Token
	if err := windows.OpenProcessToken(process, windows.TOKEN_QUERY, &token); err != nil {
		return err
	}
	defer token.Close()
	read := func(kind uint32) ([]byte, error) {
		var size uint32
		_ = windows.GetTokenInformation(token, kind, nil, 0, &size)
		if size < 4 || size > 65536 {
			return nil, errors.New("invalid token information size")
		}
		buffer := make([]byte, size)
		err := windows.GetTokenInformation(token, kind, &buffer[0], size, &size)
		return buffer, err
	}
	ac, err := read(29)
	if err != nil || *(*uint32)(unsafe.Pointer(&ac[0])) != 1 {
		return errors.New("process is not AppContainer")
	}
	caps, err := read(30)
	if err != nil || *(*uint32)(unsafe.Pointer(&caps[0])) != 0 {
		return errors.New("process has unexpected capabilities")
	}
	box, err := read(31)
	if err != nil || len(box) < int(unsafe.Sizeof(uintptr(0))) {
		return errors.New("process package unavailable")
	}
	actual := *(**windows.SID)(unsafe.Pointer(&box[0]))
	if actual == nil || !actual.Equals(sid) {
		return errors.New("process package mismatch")
	}
	runtime.KeepAlive(box)
	return nil
}

func verifyNetworkTree(p *pipeProcess, sid *windows.SID) error {
	for size := 64; size <= 65536; size *= 2 {
		list := make([]uintptr, size+1) // header is two DWORDs on Windows x64
		err := windows.QueryInformationJobObject(p.job, windows.JobObjectBasicProcessIdList, uintptr(unsafe.Pointer(&list[0])), uint32(len(list))*uint32(unsafe.Sizeof(uintptr(0))), nil)
		if errors.Is(err, windows.ERROR_MORE_DATA) {
			continue
		}
		if err != nil {
			return err
		}
		header := (*[2]uint32)(unsafe.Pointer(&list[0]))
		if header[1] == 0 || header[1] > uint32(size) || header[0] != header[1] {
			return errors.New("incomplete browser tree")
		}
		for _, pid := range list[1 : 1+int(header[1])] {
			h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION|windows.SYNCHRONIZE, false, uint32(pid))
			if err != nil {
				return err
			}
			var member uint32
			ok, _, _ := processInJob.Call(uintptr(h), uintptr(p.job), uintptr(unsafe.Pointer(&member)))
			if ok == 0 || member == 0 {
				windows.CloseHandle(h)
				return errors.New("browser job membership changed")
			}
			err = verifyNetworkProcess(h, sid)
			windows.CloseHandle(h)
			if err != nil {
				return err
			}
		}
		return nil
	}
	return errors.New("browser tree query exceeds bounded observation")
}
