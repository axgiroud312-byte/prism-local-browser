//go:build windows

package workspace

import (
	"errors"
	"os"
	"reflect"
	"runtime"
	"testing"
	"unsafe"

	"golang.org/x/sys/windows"
)

type ownedACLAccess struct {
	Owner, Group string
	Protected    bool
	Entries      []string
}

func readOwnedACLAccess(sd *windows.SECURITY_DESCRIPTOR) (ownedACLAccess, error) {
	var access ownedACLAccess
	owner, _, err := sd.Owner()
	if err != nil || owner == nil {
		return access, errors.New("owner unavailable")
	}
	group, _, err := sd.Group()
	if err != nil || group == nil {
		return access, errors.New("group unavailable")
	}
	acl, _, err := sd.DACL()
	if err != nil || acl == nil {
		return access, errors.New("DACL unavailable")
	}
	control, _, err := sd.Control()
	if err != nil {
		return access, err
	}
	access.Owner, access.Group, access.Protected = owner.String(), group.String(), control&windows.SE_DACL_PROTECTED != 0
	for index := uint32(0); index < uint32(acl.AceCount); index++ {
		var ace *windows.ACCESS_ALLOWED_ACE
		if err := windows.GetAce(acl, index, &ace); err != nil {
			return access, err
		}
		if ace == nil || ace.Header.AceSize < 4 {
			return access, errors.New("invalid DACL entry")
		}
		access.Entries = append(access.Entries, string(unsafe.Slice((*byte)(unsafe.Pointer(ace)), int(ace.Header.AceSize))))
	}
	return access, nil
}

type ownedACLDenial struct {
	handle   windows.Handle
	original *windows.SECURITY_DESCRIPTOR
	access   ownedACLAccess
	restored bool
	closed   bool
}

const ownedSecurityInformation = windows.OWNER_SECURITY_INFORMATION | windows.GROUP_SECURITY_INFORMATION | windows.DACL_SECURITY_INFORMATION

// The Win32 directory aliases share the file bits; x/sys exports only the
// file names. FILE_ADD_FILE=FILE_WRITE_DATA, FILE_ADD_SUBDIRECTORY=FILE_APPEND_DATA.
const ownedDirectoryAddFile = windows.FILE_WRITE_DATA
const ownedDirectoryAddSubdirectory = windows.FILE_APPEND_DATA

// Only callers' newly created synthetic test objects are accepted here. The
// WRITE_DAC handle is retained before changing permissions, and never denied.
// Restoration uses that exact object handle, not a possibly moved path.
func denyOwnedACL(t *testing.T, path string, mask uint32) *ownedACLDenial {
	t.Helper()
	p, err := windows.UTF16PtrFromString(path)
	if err != nil {
		t.Fatal(err)
	}
	handle, err := windows.CreateFile(p, windows.READ_CONTROL|windows.WRITE_DAC, windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE, nil, windows.OPEN_EXISTING, windows.FILE_FLAG_BACKUP_SEMANTICS|windows.FILE_FLAG_OPEN_REPARSE_POINT, 0)
	if err != nil {
		t.Fatal("cannot retain permission restoration handle")
	}
	d := &ownedACLDenial{handle: handle}
	t.Cleanup(func() {
		if err := d.restore(); err != nil {
			t.Error("owned synthetic DACL restoration failed:", err)
		}
		if !d.closed {
			windows.CloseHandle(handle)
			d.closed = true
		}
	})
	d.original, err = windows.GetSecurityInfo(handle, windows.SE_FILE_OBJECT, ownedSecurityInformation)
	if err != nil {
		t.Fatal(err)
	}
	d.access, err = readOwnedACLAccess(d.original)
	if err != nil {
		t.Fatal(err)
	}
	acl, _, err := d.original.DACL()
	if err != nil {
		t.Fatal(err)
	}
	user, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		t.Fatal(err)
	}
	updated, err := windows.ACLFromEntries([]windows.EXPLICIT_ACCESS{{AccessPermissions: windows.ACCESS_MASK(mask), AccessMode: windows.DENY_ACCESS, Inheritance: windows.NO_INHERITANCE, Trustee: windows.TRUSTEE{TrusteeForm: windows.TRUSTEE_IS_SID, TrusteeType: windows.TRUSTEE_IS_USER, TrusteeValue: windows.TrusteeValueFromSID(user.User.Sid)}}}, acl)
	if err != nil {
		t.Fatal(err)
	}
	err = windows.SetSecurityInfo(handle, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION, nil, nil, updated, nil)
	runtime.KeepAlive(user)
	if err != nil {
		t.Fatal("owned DACL denial could not be applied; no privilege escalation")
	}
	return d
}

func (d *ownedACLDenial) restore() error {
	if d.restored || d.original == nil {
		return nil
	}
	acl, _, err := d.original.DACL()
	if err != nil {
		return err
	}
	flags := uint32(windows.DACL_SECURITY_INFORMATION | windows.UNPROTECTED_DACL_SECURITY_INFORMATION)
	if d.access.Protected {
		flags = windows.DACL_SECURITY_INFORMATION | windows.PROTECTED_DACL_SECURITY_INFORMATION
	}
	if err = windows.SetSecurityInfo(d.handle, windows.SE_FILE_OBJECT, windows.SECURITY_INFORMATION(flags), nil, nil, acl, nil); err != nil {
		return err
	}
	after, err := windows.GetSecurityInfo(d.handle, windows.SE_FILE_OBJECT, ownedSecurityInformation)
	if err != nil {
		return err
	}
	actual, err := readOwnedACLAccess(after)
	if err != nil || !reflect.DeepEqual(d.access, actual) {
		return errors.New("actual owner/group/ordered ACEs/protection not restored")
	}
	d.restored = true
	return nil
}

func (d *ownedACLDenial) restoreAndRelease() error {
	if err := d.restore(); err != nil {
		return err
	}
	if !d.closed {
		if err := windows.CloseHandle(d.handle); err != nil {
			return err
		}
		d.closed = true
	}
	return nil
}

func assertOwnedAccessDenied(t *testing.T, path string, access uint32) {
	t.Helper()
	p, err := windows.UTF16PtrFromString(path)
	if err != nil {
		t.Fatal(err)
	}
	handle, err := windows.CreateFile(p, access, windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE, nil, windows.OPEN_EXISTING, windows.FILE_FLAG_BACKUP_SEMANTICS|windows.FILE_FLAG_OPEN_REPARSE_POINT, 0)
	if err == nil {
		windows.CloseHandle(handle)
		t.Fatal("actual access was not denied; cannot claim an ACL failure")
	}
	if !errors.Is(err, windows.ERROR_ACCESS_DENIED) {
		t.Fatal("access failed for a reason other than actual Windows access denial")
	}
}

func requireSafeGapAcceptance(t *testing.T) {
	t.Helper()
	if os.Getenv("PRISM_SAFE_GAP_VERIFY") != "1" {
		t.Skip("owned real DACL/capacity acceptance not explicitly selected")
	}
}
