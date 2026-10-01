//go:build windows

package backup

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"path/filepath"

	"golang.org/x/sys/windows"
)

// DeleteRetainedTree consumes a durable, explicitly confirmed purge inventory.
// With partial=true missing entries are already authorized deletions; different
// or additional entries are never adopted. All existing objects are verified and
// held by their DELETE-capable handles before the first deletion.
func DeleteRetainedTree(ctx context.Context, root, relative string, identity TreeIdentity, inventory []File, lock *RetainedLock, partial bool) error {
	path, err := managedPath(root, relative)
	if err != nil {
		return err
	}
	parent, err := pinDirectoryChain(filepath.Dir(path))
	if err != nil {
		return err
	}
	defer parent()
	files := map[string]File{}
	for _, entry := range inventory {
		if !ValidName(entry.Path) || entry.Path == ".prism-runtime.lock" {
			return errors.New("invalid retained inventory")
		}
		if _, exists := files[entry.Path]; exists {
			return errors.New("duplicate retained entry")
		}
		files[entry.Path] = entry
	}
	handles := []*os.File{}
	defer func() {
		for i := len(handles) - 1; i >= 0; i-- {
			if handles[i] != nil {
				handles[i].Close()
			}
		}
	}()
	seen := map[string]bool{}
	lockSeen := false
	var capture func(string, string, bool) error
	capture = func(current, name string, directory bool) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		wide, err := windows.UTF16PtrFromString(current)
		if err != nil {
			return err
		}
		h, err := windows.CreateFile(wide, windows.GENERIC_READ|windows.DELETE, windows.FILE_SHARE_READ, nil, windows.OPEN_EXISTING, windows.FILE_FLAG_BACKUP_SEMANTICS|windows.FILE_FLAG_OPEN_REPARSE_POINT, 0)
		if err != nil {
			return err
		}
		file := os.NewFile(uintptr(h), current)
		handles = append(handles, file)
		var info windows.ByHandleFileInformation
		if err := windows.GetFileInformationByHandle(h, &info); err != nil {
			return err
		}
		if info.FileAttributes&windows.FILE_ATTRIBUTE_REPARSE_POINT != 0 || (info.FileAttributes&windows.FILE_ATTRIBUTE_DIRECTORY != 0) != directory || !directory && info.NumberOfLinks != 1 {
			return errors.New("retained entry changed type or boundary")
		}
		if name == "" {
			if treeIdentity(info) != identity {
				return errors.New("retained tree identity differs")
			}
		} else if name != ".prism-runtime.lock" {
			expected, known := files[name]
			if !known || expected.Directory != directory {
				return errors.New("unknown retained entry protected")
			}
			seen[name] = true
			if !directory {
				hash := sha256.New()
				size, err := io.Copy(hash, contextReader{ctx, file})
				if err != nil {
					return err
				}
				if size != expected.Size || hex.EncodeToString(hash.Sum(nil)) != expected.SHA256 {
					return errors.New("retained file differs")
				}
			}
		} else {
			if directory || lock == nil || treeIdentity(info) != lock.Identity {
				return errors.New("unrecognized runtime ownership file")
			}
			hash := sha256.New()
			size, err := io.Copy(hash, contextReader{ctx, file})
			if err != nil {
				return err
			}
			if size != lock.Size || hex.EncodeToString(hash.Sum(nil)) != lock.SHA256 {
				return errors.New("runtime ownership file changed")
			}
			lockSeen = true
		}
		// Reject before deleting anything, rather than stranding a partial tree.
		if !directory && info.FileAttributes&windows.FILE_ATTRIBUTE_READONLY != 0 {
			return errors.New("retained read-only file must be unlocked before purge")
		}
		if directory {
			entries, err := file.ReadDir(-1)
			if err != nil {
				return err
			}
			for _, entry := range entries {
				relative := entry.Name()
				if name != "" {
					relative = name + "/" + entry.Name()
				}
				if err := capture(filepath.Join(current, entry.Name()), relative, entry.IsDir()); err != nil {
					return err
				}
			}
		}
		return nil
	}
	if err := capture(path, "", true); err != nil {
		if partial && len(handles) == 0 && os.IsNotExist(err) {
			return nil
		}
		return err
	}
	if !partial && len(seen) != len(files) {
		return errors.New("retained inventory is incomplete")
	}
	if !partial && lock != nil && !lockSeen {
		return errors.New("retained ownership file missing")
	}
	for index := len(handles) - 1; index >= 0; index-- {
		if err := ctx.Err(); err != nil {
			return err
		}
		remove := byte(1)
		if err := windows.SetFileInformationByHandle(windows.Handle(handles[index].Fd()), windows.FileDispositionInfo, &remove, 1); err != nil {
			return err
		}
		if err := handles[index].Close(); err != nil {
			return err
		}
		handles[index] = nil
	}
	if _, err := os.Lstat(path); !os.IsNotExist(err) {
		return errors.New("retained tree deletion not yet confirmed")
	}
	return nil
}
