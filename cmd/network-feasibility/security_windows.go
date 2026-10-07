//go:build windows

package main

import (
	"fmt"
	"io/fs"
	"path/filepath"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

var userenv = windows.NewLazySystemDLL("userenv.dll")

type containerProfile struct {
	name *uint16
	sid  *windows.SID
}
type securityCapabilities struct {
	SID             *windows.SID
	Capabilities    uintptr
	Count, Reserved uint32
}

func createContainer(name string) (*containerProfile, error) {
	n, err := windows.UTF16PtrFromString(name)
	if err != nil {
		return nil, err
	}
	var sid *windows.SID
	hr, _, _ := userenv.NewProc("CreateAppContainerProfile").Call(uintptr(unsafe.Pointer(n)), uintptr(unsafe.Pointer(n)), uintptr(unsafe.Pointer(n)), 0, 0, uintptr(unsafe.Pointer(&sid)))
	if int32(hr) < 0 {
		return nil, fmt.Errorf("CreateAppContainerProfile HRESULT 0x%x", uint32(hr))
	}
	return &containerProfile{n, sid}, nil
}
func (c *containerProfile) close() error {
	defer windows.FreeSid(c.sid)
	return deleteSyntheticContainer(c.name)
}
func deleteSyntheticContainer(name *uint16) error {
	var hr uintptr
	for attempt := 0; attempt < 20; attempt++ {
		hr, _, _ = userenv.NewProc("DeleteAppContainerProfile").Call(uintptr(unsafe.Pointer(name)))
		if int32(hr) >= 0 {
			return nil
		}
		if uint32(hr) != 0x80070020 && uint32(hr) != 0x80070005 && uint32(hr) != 0x80070091 {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if int32(hr) < 0 {
		return fmt.Errorf("DeleteAppContainerProfile HRESULT 0x%x", uint32(hr))
	}
	return nil
}

// Applied ONLY to a newly created, disposable synthetic root. Never grant
// access to the checkout, archive source, a normal workspace or user profile.
func allowSyntheticRoot(root string, sid *windows.SID) error {
	u, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		return err
	}
	sddl := "D:P(A;OICI;FA;;;" + u.User.Sid.String() + ")(A;OICI;FA;;;SY)(A;OICI;GRGX;;;" + sid.String() + ")(A;OICI;GRGX;;;S-1-15-2-1)"
	sd, err := windows.SecurityDescriptorFromString(sddl)
	if err != nil {
		return err
	}
	dacl, _, err := sd.DACL()
	if err != nil {
		return err
	}
	return windows.SetNamedSecurityInfo(root, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION|windows.PROTECTED_DACL_SECURITY_INFORMATION, nil, nil, dacl, nil)
}

func allowSyntheticProfile(path string, sid *windows.SID) error {
	u, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		return err
	}
	sd, err := windows.SecurityDescriptorFromString("D:P(A;OICI;FA;;;" + u.User.Sid.String() + ")(A;OICI;FA;;;SY)(A;OICI;FA;;;" + sid.String() + ")S:(ML;OICI;NW;;;LW)")
	if err != nil {
		return err
	}
	dacl, _, err := sd.DACL()
	if err != nil {
		return err
	}
	sacl, _, err := sd.SACL()
	if err != nil {
		return err
	}
	return windows.SetNamedSecurityInfo(path, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION|windows.PROTECTED_DACL_SECURITY_INFORMATION|windows.LABEL_SECURITY_INFORMATION, nil, nil, dacl, sacl)
}

// This later-stage compatibility probe grants access only after an ordinary
// browser has created real synthetic data. It never changes integrity labels.
func grantSyntheticDataTree(root string, sid *windows.SID) (func() error, error) {
	update := func(mode windows.ACCESS_MODE) error {
		return filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			name, e := windows.UTF16PtrFromString(path)
			if e != nil {
				return e
			}
			attributes, e := windows.GetFileAttributes(name)
			if e != nil {
				return e
			}
			if attributes&windows.FILE_ATTRIBUTE_REPARSE_POINT != 0 {
				return fmt.Errorf("synthetic-data-reparse-refused")
			}
			sd, e := windows.GetNamedSecurityInfo(path, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION)
			if e != nil {
				return e
			}
			acl, _, e := sd.DACL()
			if e != nil {
				return e
			}
			inheritance := uint32(0)
			if entry.IsDir() {
				inheritance = windows.SUB_CONTAINERS_AND_OBJECTS_INHERIT
			}
			changed, e := windows.ACLFromEntries([]windows.EXPLICIT_ACCESS{{AccessPermissions: windows.GENERIC_ALL, AccessMode: mode, Inheritance: inheritance, Trustee: windows.TRUSTEE{TrusteeForm: windows.TRUSTEE_IS_SID, TrusteeType: windows.TRUSTEE_IS_UNKNOWN, TrusteeValue: windows.TrusteeValueFromSID(sid)}}}, acl)
			if e != nil {
				return e
			}
			return windows.SetNamedSecurityInfo(path, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION, nil, nil, changed, nil)
		})
	}
	revoke := func() error { return update(windows.REVOKE_ACCESS) }
	if err := update(windows.GRANT_ACCESS); err != nil {
		return revoke, err
	}
	return revoke, nil
}

func tokenValue(token windows.Token, kind uint32) (uint32, error) {
	var size uint32
	_ = windows.GetTokenInformation(token, kind, nil, 0, &size)
	if size < 4 {
		return 0, fmt.Errorf("token information unavailable")
	}
	data := make([]byte, size)
	if err := windows.GetTokenInformation(token, kind, &data[0], size, &size); err != nil {
		return 0, err
	}
	return *(*uint32)(unsafe.Pointer(&data[0])), nil
}

func tokenIntegrity(token windows.Token) (uint32, error) {
	var size uint32
	_ = windows.GetTokenInformation(token, windows.TokenIntegrityLevel, nil, 0, &size)
	if size < uint32(unsafe.Sizeof(windows.Tokenmandatorylabel{})) {
		return 0, fmt.Errorf("token integrity unavailable")
	}
	buffer := make([]byte, size)
	if err := windows.GetTokenInformation(token, windows.TokenIntegrityLevel, &buffer[0], size, &size); err != nil {
		return 0, err
	}
	label := (*windows.Tokenmandatorylabel)(unsafe.Pointer(&buffer[0]))
	count := label.Label.Sid.SubAuthorityCount()
	if count == 0 {
		return 0, fmt.Errorf("invalid integrity SID")
	}
	return label.Label.Sid.SubAuthority(uint32(count) - 1), nil
}

// Documented native low-box creation from the host's own token, with no
// capabilities and no inherited object handles. Observe its integrity rather
// than attempting to raise an already restricted token's integrity.
func nativeLowBox(sid *windows.SID) (windows.Token, error) {
	var original windows.Token
	if err := windows.OpenProcessToken(windows.CurrentProcess(), windows.TOKEN_QUERY|windows.TOKEN_DUPLICATE|windows.TOKEN_ASSIGN_PRIMARY, &original); err != nil {
		return 0, err
	}
	defer original.Close()
	attributes := windows.OBJECT_ATTRIBUTES{}
	attributes.Length = uint32(unsafe.Sizeof(attributes))
	var token windows.Token
	status, _, _ := windows.NewLazySystemDLL("ntdll.dll").NewProc("NtCreateLowBoxToken").Call(uintptr(unsafe.Pointer(&token)), uintptr(original), windows.MAXIMUM_ALLOWED, uintptr(unsafe.Pointer(&attributes)), uintptr(unsafe.Pointer(sid)), 0, 0, 0, 0)
	if int32(status) < 0 {
		return 0, fmt.Errorf("NtCreateLowBoxToken NTSTATUS 0x%x", uint32(status))
	}
	return token, nil
}
