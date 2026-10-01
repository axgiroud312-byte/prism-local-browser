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
	"strings"

	"github.com/axgiroud312-byte/prism-local-browser/internal/desktopbase"
	"github.com/axgiroud312-byte/prism-local-browser/internal/kernel"
	"golang.org/x/sys/windows"
)

// Stable volume/file identity lets a journal recognize a rename which completed
// immediately before process death. It never treats mere path existence as ours.
type TreeIdentity struct {
	Volume uint32 `json:"volume"`
	High   uint32 `json:"high"`
	Low    uint32 `json:"low"`
}

func treeIdentity(info windows.ByHandleFileInformation) TreeIdentity {
	return TreeIdentity{info.VolumeSerialNumber, info.FileIndexHigh, info.FileIndexLow}
}
func managedPath(root, relative string) (string, error) {
	if !ValidName(relative) || strings.HasSuffix(relative, "/") {
		return "", errors.New("invalid internal relative path")
	}
	return filepath.Join(root, filepath.FromSlash(relative)), nil
}
func IdentifyTree(root, relative string) (TreeIdentity, bool, error) {
	path, err := managedPath(root, relative)
	if err != nil {
		return TreeIdentity{}, false, err
	}
	if err = desktopbase.ValidatePath(path); err != nil {
		return TreeIdentity{}, false, err
	}
	f, info, err := openCaptured(path, true, false)
	if os.IsNotExist(err) {
		return TreeIdentity{}, false, nil
	}
	if err != nil {
		return TreeIdentity{}, false, err
	}
	defer f.Close()
	return treeIdentity(info), true, nil
}

func MoveTree(ctx context.Context, root, source, destination string, expected TreeIdentity, files []File) error {
	from, err := managedPath(root, source)
	if err != nil {
		return err
	}
	to, err := managedPath(root, destination)
	if err != nil {
		return err
	}
	parent, release, err := pinRenameParents(filepath.Dir(from), filepath.Dir(to))
	if err != nil {
		return err
	}
	defer release()
	// Source traversal remains fully frozen until inventory verification finishes.
	// Release these extra read-only pins before the native target-directory open;
	// the verified rename-capable source and relative destination handles remain.
	releaseSource, err := pinDirectoryChain(filepath.Dir(from))
	if err != nil {
		return err
	}
	defer func() {
		if releaseSource != nil {
			releaseSource()
		}
	}()
	if err = desktopbase.ValidateTree(from); err != nil {
		return err
	}
	if _, err = os.Lstat(to); !os.IsNotExist(err) {
		return errors.New("destination not confirmed absent")
	}
	name, err := windows.UTF16PtrFromString(from)
	if err != nil {
		return err
	}
	h, err := windows.CreateFile(name, windows.GENERIC_READ|windows.DELETE, windows.FILE_SHARE_READ, nil, windows.OPEN_EXISTING, windows.FILE_FLAG_BACKUP_SEMANTICS|windows.FILE_FLAG_OPEN_REPARSE_POINT, 0)
	if err != nil {
		return err
	}
	file := os.NewFile(uintptr(h), from)
	defer file.Close()
	var info windows.ByHandleFileInformation
	if err = windows.GetFileInformationByHandle(h, &info); err != nil || info.FileAttributes&windows.FILE_ATTRIBUTE_REPARSE_POINT != 0 || info.FileAttributes&windows.FILE_ATTRIBUTE_DIRECTORY == 0 || treeIdentity(info) != expected {
		return errors.New("source directory identity changed")
	}
	// Capture with this very rename-capable root handle. A second root open
	// would conflict with its DELETE access, and path-only verification would
	// leave a replaceable gap. Descendants stay pinned through the final check.
	p := &Profile{root: from, enumerationRoot: file}
	if err = p.captureDirectory(ctx, from, "", file); err == nil {
		err = verifyInventory(ctx, p, files)
	}
	closeErr := p.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	releaseSource()
	releaseSource = nil
	return renameAt(h, parent, filepath.Base(to))
}

// Extract only a previously validated entry set into a newly owned same-volume
// stage. Every file is CREATE_NEW and Sync'ed; no archived ACL/link is applied.
func (p *Package) ExtractProfiles(ctx context.Context, root, stage string) error {
	for _, entry := range p.Manifest.Files {
		if entry.Path == p.Manifest.Configuration {
			continue
		}
		relative := stage + "/" + strings.TrimSuffix(entry.Path, "/")
		parent := filepath.ToSlash(filepath.Dir(relative))
		if entry.Directory {
			parent = relative
		}
		release, err := kernel.EnsureDirectory(root, parent)
		if err != nil {
			return err
		}
		freeze, err := pinDirectoryChain(filepath.Join(root, filepath.FromSlash(parent)))
		if err != nil {
			release()
			return err
		}
		if !entry.Directory {
			path := filepath.Join(root, filepath.FromSlash(relative))
			file, problem := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
			if problem == nil {
				problem = p.Copy(ctx, entry.Path, file)
				if problem == nil {
					problem = file.Sync()
				}
				problem = errors.Join(problem, file.Close())
			}
			err = problem
		}
		freeze()
		release()
		if err != nil {
			return err
		}
		if err = ctx.Err(); err != nil {
			return err
		}
	}
	return nil
}

// Inventory keeps no browser values; it fingerprints the captured stopped tree.
// The runtime lock itself is deliberately excluded, just as in the backup.
func (p *Profile) Inventory(ctx context.Context) ([]File, error) {
	items := []File{}
	for _, entry := range p.entries {
		if entry.relative == "" {
			continue
		}
		item := File{Path: entry.relative, Directory: entry.directory}
		if !entry.directory {
			if _, err := entry.file.Seek(0, io.SeekStart); err != nil {
				return nil, err
			}
			h := sha256.New()
			size, err := io.Copy(h, contextReader{ctx, entry.file})
			if err != nil {
				return nil, err
			}
			item.Size = size
			item.SHA256 = hex.EncodeToString(h.Sum(nil))
		}
		items = append(items, item)
	}
	return items, p.Validate(ctx)
}

// Validate an owned tree at either its live or rollback location. Only regular,
// single-link files are read, and the complete visible entry set must match.
func VerifyTree(ctx context.Context, root, relative string, expected TreeIdentity, files []File) error {
	path, err := managedPath(root, relative)
	if err != nil {
		return err
	}
	release, err := pinDirectoryChain(filepath.Dir(path))
	if err != nil {
		return err
	}
	defer release()
	file, info, err := openCaptured(path, true, false)
	if err != nil {
		return err
	}
	defer file.Close()
	if treeIdentity(info) != expected {
		return errors.New("tree identity unconfirmed")
	}
	p := &Profile{root: path}
	defer p.Close()
	if err = p.captureDirectory(ctx, path, "", file); err != nil {
		return err
	}
	return verifyInventory(ctx, p, files)
}

func verifyInventory(ctx context.Context, p *Profile, files []File) error {
	var err error
	p.lock, _, err = openCaptured(filepath.Join(p.root, ".prism-runtime.lock"), false, true)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	actual, err := p.Inventory(ctx)
	if err != nil {
		return err
	}
	if len(actual) != len(files) {
		return errors.New("tree entries missing or unexpected")
	}
	want := map[string]File{}
	for _, f := range files {
		want[f.Path] = f
	}
	for _, f := range actual {
		if expected, ok := want[f.Path]; !ok || expected != f {
			return errors.New("tree entry differs")
		}
	}
	return nil
}

func PrepareRestoreLock(root, relative string) (TreeIdentity, error) {
	path, err := managedPath(root, relative)
	if err != nil {
		return TreeIdentity{}, err
	}
	release, err := pinDirectoryChain(path)
	if err != nil {
		return TreeIdentity{}, err
	}
	defer release()
	file, err := os.OpenFile(filepath.Join(path, ".prism-runtime.lock"), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return TreeIdentity{}, err
	}
	err = errors.Join(file.Sync(), file.Close())
	if err != nil {
		return TreeIdentity{}, err
	}
	identity, exists, err := IdentifyTree(root, relative)
	if err != nil {
		return TreeIdentity{}, err
	}
	if !exists {
		return TreeIdentity{}, errors.New("incoming disappeared")
	}
	return identity, nil
}
