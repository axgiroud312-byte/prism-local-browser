//go:build windows

package backup

import (
	"errors"
	"path/filepath"
	"sort"
	"strings"
	"unsafe"

	"golang.org/x/sys/windows"
)

// Native rename still opens its target directory with write access. Permit that
// sharing only on the fixed destination-parent object, deny its deletion, and
// perform destination operations relative to its handle (never its mutable path).
func pinRenameParents(source, destination string) (windows.Handle, func(), error) {
	paths := map[string]string{}
	for _, directory := range []string{source, destination} {
		absolute, err := filepath.Abs(directory)
		if err != nil || strings.HasPrefix(absolute, `\\`) {
			return 0, nil, errors.New("invalid rename parent")
		}
		for current := absolute; ; current = filepath.Dir(current) {
			paths[strings.ToLower(current)] = current
			if current == filepath.Dir(current) {
				break
			}
		}
	}
	destination, err := filepath.Abs(destination)
	if err != nil {
		return 0, nil, err
	}
	ordered := []string{}
	for _, p := range paths {
		ordered = append(ordered, p)
	}
	sort.Slice(ordered, func(i, j int) bool { return len(ordered[i]) < len(ordered[j]) })
	handles := []windows.Handle{}
	release := func() {
		for i := len(handles) - 1; i >= 0; i-- {
			windows.CloseHandle(handles[i])
		}
	}
	var target windows.Handle
	for _, path := range ordered {
		name, err := windows.UTF16PtrFromString(path)
		if err != nil {
			release()
			return 0, nil, err
		}
		share := uint32(windows.FILE_SHARE_READ)
		if strings.EqualFold(path, destination) {
			share |= windows.FILE_SHARE_WRITE
		}
		h, err := windows.CreateFile(name, windows.GENERIC_READ, share, nil, windows.OPEN_EXISTING, windows.FILE_FLAG_BACKUP_SEMANTICS|windows.FILE_FLAG_OPEN_REPARSE_POINT, 0)
		if err != nil {
			release()
			return 0, nil, err
		}
		handles = append(handles, h)
		var info windows.ByHandleFileInformation
		if windows.GetFileInformationByHandle(h, &info) != nil || info.FileAttributes&windows.FILE_ATTRIBUTE_REPARSE_POINT != 0 || info.FileAttributes&windows.FILE_ATTRIBUTE_DIRECTORY == 0 {
			release()
			return 0, nil, errors.New("unsafe rename parent")
		}
		if strings.EqualFold(path, destination) {
			target = h
		}
	}
	return target, release, nil
}

func createFileAt(parent windows.Handle, basename string) (windows.Handle, error) {
	if !ValidName(basename) || strings.ContainsAny(basename, `/\`) {
		return 0, errors.New("creation requires a single leaf")
	}
	name, err := windows.NewNTUnicodeString(basename)
	if err != nil {
		return 0, err
	}
	oa := windows.OBJECT_ATTRIBUTES{RootDirectory: parent, ObjectName: name, Attributes: windows.OBJ_CASE_INSENSITIVE | windows.OBJ_DONT_REPARSE}
	oa.Length = uint32(unsafe.Sizeof(oa))
	var h windows.Handle
	var iosb windows.IO_STATUS_BLOCK
	err = windows.NtCreateFile(&h, windows.FILE_GENERIC_READ|windows.FILE_GENERIC_WRITE|windows.DELETE, &oa, &iosb, nil, windows.FILE_ATTRIBUTE_NORMAL, 0, windows.FILE_CREATE, windows.FILE_NON_DIRECTORY_FILE|windows.FILE_SYNCHRONOUS_IO_NONALERT|windows.FILE_OPEN_REPARSE_POINT, 0, 0)
	return h, err
}

func renameAt(file, parent windows.Handle, basename string) error {
	if !ValidName(basename) || strings.ContainsAny(basename, `/\`) {
		return errors.New("rename requires a single leaf")
	}
	name, err := windows.UTF16FromString(basename)
	if err != nil {
		return err
	}
	type renameInfo struct {
		Replace uint32
		Root    windows.Handle
		Length  uint32
		Name    uint16
	}
	var layout renameInfo
	offset := int(unsafe.Offsetof(layout.Name))
	buffer := make([]byte, offset+len(name)*2)
	info := (*renameInfo)(unsafe.Pointer(&buffer[0]))
	info.Root = parent
	info.Length = uint32((len(name) - 1) * 2)
	copy(unsafe.Slice((*uint16)(unsafe.Pointer(&buffer[offset])), len(name)), name)
	var iosb windows.IO_STATUS_BLOCK
	return windows.NtSetInformationFile(file, &iosb, &buffer[0], uint32(len(buffer)), windows.FileRenameInformation)
}
