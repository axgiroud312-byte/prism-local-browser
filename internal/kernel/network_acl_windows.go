//go:build windows

package kernel

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"runtime"
	"strings"
	"unsafe"

	"golang.org/x/sys/windows"
)

type networkACLDelta struct {
	SID  string `json:"sid"`
	Mask uint32 `json:"mask"`
	Tree bool   `json:"tree"`
}

// A tree grant is a single durable intent over an anchored root and one unique
// package SID. No old DACL snapshot is restored. Recovery removes this SID from
// the current tree, including files created/renamed by Chromium after launch.
// Every explicit mutation uses a non-reparse handle. The tree intent also owns
// Windows' inheritance propagation within the validated, anchored subtree.
func openNetworkACLObject(path string) (windows.Handle, string, bool, error) {
	if err := CheckBoundary(path, isNetworkDirectory(path)); err != nil {
		return 0, "", false, err
	}
	p, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return 0, "", false, err
	}
	h, err := windows.CreateFile(p, windows.READ_CONTROL|windows.WRITE_DAC|windows.FILE_READ_ATTRIBUTES, windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE, nil, windows.OPEN_EXISTING, windows.FILE_FLAG_OPEN_REPARSE_POINT|windows.FILE_FLAG_BACKUP_SEMANTICS, 0)
	if err != nil {
		return 0, "", false, err
	}
	var info windows.ByHandleFileInformation
	err = windows.GetFileInformationByHandle(h, &info)
	dir := info.FileAttributes&windows.FILE_ATTRIBUTE_DIRECTORY != 0
	if err != nil || info.FileAttributes&windows.FILE_ATTRIBUTE_REPARSE_POINT != 0 || (!dir && info.NumberOfLinks != 1) {
		windows.CloseHandle(h)
		return 0, "", false, errors.New("network ACL object identity unavailable")
	}
	identity := fmt.Sprintf("%08x:%08x%08x:%08x%08x", info.VolumeSerialNumber, info.FileIndexHigh, info.FileIndexLow, info.CreationTime.HighDateTime, info.CreationTime.LowDateTime)
	return h, identity, dir, nil
}

func isNetworkDirectory(path string) bool {
	p, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return false
	}
	a, err := windows.GetFileAttributes(p)
	return err == nil && a&windows.FILE_ATTRIBUTE_DIRECTORY != 0
}

func lockNetworkACL(identity string) (func(), error) {
	runtime.LockOSThread()
	hash := sha256.Sum256([]byte(strings.ToLower(identity)))
	name, _ := windows.UTF16PtrFromString(fmt.Sprintf(`Local\Prism-Network-ACL-%x`, hash))
	h, err := windows.CreateMutex(nil, false, name)
	if err != nil && !errors.Is(err, windows.ERROR_ALREADY_EXISTS) {
		runtime.UnlockOSThread()
		return nil, err
	}
	state, err := windows.WaitForSingleObject(h, 10000)
	if err != nil || (state != windows.WAIT_OBJECT_0 && state != windows.WAIT_ABANDONED) {
		windows.CloseHandle(h)
		runtime.UnlockOSThread()
		return nil, errors.New("network ACL lock unavailable")
	}
	return func() { windows.ReleaseMutex(h); windows.CloseHandle(h); runtime.UnlockOSThread() }, nil
}

func changeNetworkACL(h windows.Handle, objectType windows.SE_OBJECT_TYPE, sid *windows.SID, mask uint32, inheritance byte, grant bool) error {
	sd, err := windows.GetSecurityInfo(h, objectType, windows.DACL_SECURITY_INFORMATION)
	if err != nil {
		return err
	}
	acl, _, err := sd.DACL()
	if err != nil || acl == nil {
		return errors.New("null network object DACL refused")
	}
	header := append([]byte(nil), unsafe.Slice((*byte)(unsafe.Pointer(acl)), 8)...)
	data := header
	count := uint16(0)
	found := false
	for index := uint32(0); index < uint32(acl.AceCount); index++ {
		var ace *windows.ACCESS_ALLOWED_ACE
		if err = windows.GetAce(acl, index, &ace); err != nil {
			return err
		}
		size := int(ace.Header.AceSize)
		if size < 4 {
			return errors.New("invalid network object ACE")
		}
		bytes := unsafe.Slice((*byte)(unsafe.Pointer(ace)), size)
		matches := false
		if (ace.Header.AceType == 0 || ace.Header.AceType == 1) && size >= 16 {
			candidate := (*windows.SID)(unsafe.Pointer(&bytes[8]))
			if candidate.IsValid() && candidate.Len() <= size-8 {
				matches = candidate.Equals(sid)
			}
		}
		if matches {
			if ace.Header.AceType != 0 {
				return errors.New("unexpected deny for owned package")
			}
			found = true
			if grant && (uint32(ace.Mask) != mask || ace.Header.AceFlags&^windows.INHERITED_ACE != inheritance) {
				return errors.New("package permission differs from durable intent")
			}
			if !grant {
				continue
			}
		}
		data = append(data, bytes...)
		count++
	}
	if grant && !found {
		entry := make([]byte, 8+sid.Len())
		entry[1] = inheritance
		binary.LittleEndian.PutUint16(entry[2:], uint16(len(entry)))
		binary.LittleEndian.PutUint32(entry[4:], mask)
		copy(entry[8:], unsafe.Slice((*byte)(unsafe.Pointer(sid)), sid.Len()))
		// An explicit allow precedes inherited ACEs. Existing ACE order remains.
		offset := 8
		for offset < len(data) && data[offset+1]&windows.INHERITED_ACE == 0 {
			offset += int(binary.LittleEndian.Uint16(data[offset+2:]))
		}
		data = append(append(append([]byte(nil), data[:offset]...), entry...), data[offset:]...)
		count++
	}
	if len(data) > 65535 {
		return errors.New("network ACL too large")
	}
	binary.LittleEndian.PutUint16(data[2:], uint16(len(data)))
	binary.LittleEndian.PutUint16(data[4:], count)
	changed, err := sd.ToAbsolute()
	if err != nil {
		return err
	}
	if err = changed.SetDACL((*windows.ACL)(unsafe.Pointer(&data[0])), true, false); err != nil {
		return err
	}
	err = windows.SetSecurityInfo(h, objectType, windows.DACL_SECURITY_INFORMATION, nil, nil, (*windows.ACL)(unsafe.Pointer(&data[0])), nil)
	runtime.KeepAlive(data)
	runtime.KeepAlive(sd)
	if err != nil {
		return err
	}
	check, err := windows.GetSecurityInfo(h, objectType, windows.DACL_SECURITY_INFORMATION)
	if err != nil {
		return err
	}
	checkACL, _, err := check.DACL()
	if err != nil || checkACL == nil {
		return errors.New("network ACL verification unavailable")
	}
	present := false
	for index := uint32(0); index < uint32(checkACL.AceCount); index++ {
		var ace *windows.ACCESS_ALLOWED_ACE
		if err = windows.GetAce(checkACL, index, &ace); err != nil {
			return err
		}
		if ace.Header.AceType == 0 && ace.Header.AceSize >= 16 {
			candidate := (*windows.SID)(unsafe.Pointer(&ace.SidStart))
			if candidate.IsValid() && candidate.Equals(sid) {
				present = true
			}
		}
	}
	if present != grant {
		return errors.New("network ACL change not confirmed")
	}
	return nil
}

func applyNetworkTree(ctx context.Context, path, identity string, delta networkACLDelta, grant bool) error {
	sid, err := windows.StringToSid(delta.SID)
	if err != nil {
		return err
	}
	root, actual, dir, err := openNetworkACLObject(path)
	if err != nil {
		return err
	}
	defer windows.CloseHandle(root)
	if actual != identity || !dir {
		return errors.New("network ACL root replaced")
	}
	if err := checkManagedProfileFiles(path, filepath.Join(path, ".prism-runtime.lock")); err != nil {
		return err
	}
	return filepath.WalkDir(path, func(name string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if entry.Name() == ".prism-runtime.lock" && grant {
			return nil
		}
		h, id, directory, err := openNetworkACLObject(name)
		if err != nil {
			return err
		}
		defer windows.CloseHandle(h)
		unlock, err := lockNetworkACL(id)
		if err != nil {
			return err
		}
		defer unlock()
		inherit := byte(0)
		if directory {
			inherit = windows.OBJECT_INHERIT_ACE | windows.CONTAINER_INHERIT_ACE
		}
		return changeNetworkACL(h, windows.SE_FILE_OBJECT, sid, delta.Mask, inherit, grant)
	})
}

func (p *ProtectedProxy) grantTree(ctx context.Context, path string, mask uint32) error {
	h, identity, dir, err := openNetworkACLObject(path)
	if err != nil {
		return err
	}
	defer windows.CloseHandle(h)
	if !dir {
		return errors.New("network permission root is not directory")
	}
	delta := networkACLDelta{SID: p.intent.PackageSID, Mask: mask, Tree: true}
	encoded, _ := json.Marshal(delta)
	resource := NetworkResourceIntent{ResourceID: "tree:" + identity, Kind: "file-acl", ObjectIdentity: identity, Locator: path, Delta: string(encoded)}
	return p.store.journal.ApplyResource(ctx, p.intent.SessionID, resource, func(ctx context.Context) error { return applyNetworkTree(ctx, path, identity, delta, true) })
}
