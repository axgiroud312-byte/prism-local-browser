//go:build windows

package kernel

import (
	"errors"
	"fmt"
	"testing"
	"unsafe"

	"golang.org/x/sys/windows"
)

type networkACLPermissionSnapshot struct {
	owner     string
	group     string
	protected bool
	entries   []string
}

// GetNamedSecurityInfo may include different unrequested fields on different
// Windows hosts. Query owner/group/DACL explicitly and compare actual access,
// not the complete SDDL display. Windows may set AUTO_INHERITED while applying
// inherited ACLs; the ACE inheritance flags and DACL protection must not change.
func snapshotNetworkACLPermissions(sd *windows.SECURITY_DESCRIPTOR) (networkACLPermissionSnapshot, error) {
	var snapshot networkACLPermissionSnapshot
	if sd == nil || !sd.IsValid() {
		return snapshot, errors.New("security descriptor unavailable")
	}
	owner, _, err := sd.Owner()
	if err != nil || owner == nil || !owner.IsValid() {
		return snapshot, errors.New("security owner not explicitly available")
	}
	group, _, err := sd.Group()
	if err != nil || group == nil || !group.IsValid() {
		return snapshot, errors.New("security group not explicitly available")
	}
	acl, _, err := sd.DACL()
	if err != nil || acl == nil {
		return snapshot, errors.New("missing or unrestricted DACL refused")
	}
	control, _, err := sd.Control()
	if err != nil {
		return snapshot, err
	}
	snapshot.owner, snapshot.group = owner.String(), group.String()
	snapshot.protected = control&windows.SE_DACL_PROTECTED != 0
	for index := uint32(0); index < uint32(acl.AceCount); index++ {
		var ace *windows.ACCESS_ALLOWED_ACE
		if err := windows.GetAce(acl, index, &ace); err != nil {
			return snapshot, err
		}
		if ace == nil || ace.Header.AceSize < 4 {
			return snapshot, errors.New("invalid DACL entry")
		}
		// Own every entry byte, including type, mask, SID, object fields and
		// inheritance flags. Keep ACE order; never sort or discard deny entries.
		snapshot.entries = append(snapshot.entries, string(unsafe.Slice((*byte)(unsafe.Pointer(ace)), int(ace.Header.AceSize))))
	}
	return snapshot, nil
}

func compareNetworkACLPermissions(before, after networkACLPermissionSnapshot) error {
	if before.owner != after.owner {
		return errors.New("owner changed")
	}
	if before.group != after.group {
		return errors.New("group changed")
	}
	if before.protected != after.protected {
		return errors.New("DACL inheritance protection changed")
	}
	if len(before.entries) != len(after.entries) {
		return errors.New("DACL entry count changed")
	}
	for index := range before.entries {
		if before.entries[index] != after.entries[index] {
			return fmt.Errorf("DACL entry %d changed", index)
		}
	}
	return nil
}

func TestNetworkACLPermissionSnapshotDistinguishesAccessChanges(t *testing.T) {
	entries := "(D;;FW;;;WD)(A;;FR;;;SY)(A;ID;FR;;;BA)"
	baseline := "O:SYG:BAD:P" + entries
	cases := []struct {
		name  string
		sddl  string
		equal bool
	}{
		{"same", baseline, true},
		{"automatic-inheritance-marker", "O:SYG:BAD:PAI" + entries, true},
		{"owner", "O:BAG:BAD:P" + entries, false},
		{"group", "O:SYG:SYD:P" + entries, false},
		{"inheritance-protection", "O:SYG:BAD:" + entries, false},
		{"widened-mask", "O:SYG:BAD:P(D;;FW;;;WD)(A;;FA;;;SY)(A;ID;FR;;;BA)", false},
		{"deny-to-allow", "O:SYG:BAD:P(A;;FW;;;WD)(A;;FR;;;SY)(A;ID;FR;;;BA)", false},
		{"inherited-to-explicit", "O:SYG:BAD:P(D;;FW;;;WD)(A;;FR;;;SY)(A;;FR;;;BA)", false},
		{"principal", "O:SYG:BAD:P(D;;FW;;;WD)(A;;FR;;;SY)(A;ID;FR;;;BU)", false},
		{"order", "O:SYG:BAD:P(A;;FR;;;SY)(D;;FW;;;WD)(A;ID;FR;;;BA)", false},
		{"added-entry", baseline + "(A;;FR;;;BU)", false},
		{"removed-entry", "O:SYG:BAD:P(D;;FW;;;WD)(A;;FR;;;SY)", false},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			original, err := windows.SecurityDescriptorFromString(baseline)
			if err != nil {
				t.Fatal(err)
			}
			changed, err := windows.SecurityDescriptorFromString(test.sddl)
			if err != nil {
				t.Fatal(err)
			}
			before, err := snapshotNetworkACLPermissions(original)
			if err != nil {
				t.Fatal(err)
			}
			after, err := snapshotNetworkACLPermissions(changed)
			if err != nil {
				t.Fatal(err)
			}
			if result := compareNetworkACLPermissions(before, after); (result == nil) != test.equal {
				t.Fatal("access comparison did not detect the intended security difference:", result)
			}
		})
	}
}

func TestNetworkACLPermissionSnapshotRefusesIncompleteOrUnrestrictedSecurity(t *testing.T) {
	for _, sddl := range []string{"O:SYG:BA", "O:SYG:BAD:NO_ACCESS_CONTROL", "D:(A;;FR;;;SY)"} {
		sd, err := windows.SecurityDescriptorFromString(sddl)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := snapshotNetworkACLPermissions(sd); err == nil {
			t.Fatal("incomplete or unrestricted security snapshot accepted")
		}
	}
}
