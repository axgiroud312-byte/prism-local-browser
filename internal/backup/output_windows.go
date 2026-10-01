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
	"github.com/google/uuid"
	"golang.org/x/sys/windows"
)

type Output struct {
	File        *os.File
	Temporary   string
	Destination string
	release     func()
	published   bool
	parent      windows.Handle
}

func OutputPaths(root, destination, operationID string) (string, string, error) {
	parsed, err := uuid.Parse(operationID)
	if err != nil || parsed.String() != operationID {
		return "", "", errors.New("invalid backup owner")
	}
	destination, err = filepath.Abs(destination)
	if err != nil || strings.HasPrefix(destination, `\\`) || filepath.VolumeName(destination) == "" || !strings.EqualFold(filepath.Ext(destination), Extension) || !ValidName(filepath.Base(destination)) {
		return "", "", errors.New("unsupported backup destination")
	}
	root, err = filepath.Abs(root)
	if err != nil {
		return "", "", err
	}
	relative, err := filepath.Rel(root, destination)
	if err == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return "", "", errors.New("backup destination is inside source workspace")
	}
	return destination, filepath.Join(filepath.Dir(destination), ".prism-"+operationID+".partial"), nil
}

func NewOutput(root, destination, operationID string) (_ *Output, resultErr error) {
	destination, temporary, err := OutputPaths(root, destination, operationID)
	if err != nil {
		return nil, err
	}
	return newOutput(root, destination, temporary, "")
}

// Host-only, fixed destination for a journal-owned pre-upgrade backup. The
// external OutputPaths contract continues to reject every workspace destination.
func NewMigrationOutput(root, operationID string) (*Output, error) {
	if !CanonicalID(operationID) {
		return nil, errors.New("invalid migration backup owner")
	}
	relative := "backups/migrations/" + operationID
	parent := filepath.Join(root, filepath.FromSlash(relative))
	return newOutput(root, filepath.Join(parent, "before"+Extension), filepath.Join(parent, ".prism-"+operationID+".partial"), relative)
}

func newOutput(root, destination, temporary, internalParent string) (_ *Output, resultErr error) {
	var err error
	if err = desktopbase.ValidatePath(destination); err != nil {
		return nil, err
	}
	parent := filepath.Dir(destination)
	parentHandle, release, err := pinRenameParents(parent, parent)
	if err != nil {
		return nil, err
	}
	defer func() {
		if resultErr != nil {
			release()
		}
	}()
	actualParent, err := actualDirectoryHandle(parentHandle)
	if err != nil {
		return nil, err
	}
	actualRoot, err := actualDirectory(root)
	if err != nil {
		return nil, err
	}
	if internalParent != "" {
		if actualParent != actualRoot+`\`+strings.ReplaceAll(internalParent, "/", `\`) {
			return nil, errors.New("migration destination object outside owned root")
		}
	} else if actualParent == actualRoot || strings.HasPrefix(actualParent, actualRoot+`\`) {
		return nil, errors.New("actual destination is inside source workspace")
	}
	if _, err = os.Lstat(destination); err == nil || !os.IsNotExist(err) {
		return nil, errors.New("destination already exists or cannot be confirmed absent")
	}
	handle, err := createFileAt(parentHandle, filepath.Base(temporary))
	if err != nil {
		return nil, err
	}
	return &Output{File: os.NewFile(uintptr(handle), temporary), Temporary: temporary, Destination: destination, release: release, parent: parentHandle}, nil
}

func (o *Output) Digest(ctx context.Context) (string, error) {
	if err := o.File.Sync(); err != nil {
		return "", err
	}
	if _, err := o.File.Seek(0, io.SeekStart); err != nil {
		return "", err
	}
	digest := sha256.New()
	if _, err := io.Copy(digest, contextReader{ctx, o.File}); err != nil {
		return "", err
	}
	return hex.EncodeToString(digest.Sum(nil)), nil
}

// Rename the exact open temporary object, never replace an existing destination.
// The caller persists the publishing journal/hash before this irreversible step.
func (o *Output) Publish() error {
	if o.published {
		return nil
	}
	if err := o.File.Sync(); err != nil {
		return err
	}
	if err := renameAt(windows.Handle(o.File.Fd()), o.parent, filepath.Base(o.Destination)); err != nil {
		return err
	}
	o.published = true
	return nil
}
func (o *Output) Close() error {
	err := o.File.Close()
	if o.release != nil {
		o.release()
		o.release = nil
	}
	return err
}

// Parent-to-child pins deny in-place reparse conversion as well as replacement.
// They do not claim to exclude a hostile same-SID creator of new child entries.
func pinDirectoryChain(directory string) (func(), error) {
	absolute, err := filepath.Abs(directory)
	if err != nil || strings.HasPrefix(absolute, `\\`) {
		return nil, errors.New("unsupported directory")
	}
	chain := []string{}
	for current := absolute; ; current = filepath.Dir(current) {
		chain = append(chain, current)
		if current == filepath.Dir(current) {
			break
		}
	}
	handles := []windows.Handle{}
	release := func() {
		for index := len(handles) - 1; index >= 0; index-- {
			windows.CloseHandle(handles[index])
		}
	}
	for index := len(chain) - 1; index >= 0; index-- {
		name, err := windows.UTF16PtrFromString(chain[index])
		if err != nil {
			release()
			return nil, err
		}
		handle, err := windows.CreateFile(name, windows.GENERIC_READ, windows.FILE_SHARE_READ, nil, windows.OPEN_EXISTING, windows.FILE_FLAG_BACKUP_SEMANTICS|windows.FILE_FLAG_OPEN_REPARSE_POINT, 0)
		if err != nil {
			release()
			return nil, err
		}
		var info windows.ByHandleFileInformation
		if windows.GetFileInformationByHandle(handle, &info) != nil || info.FileAttributes&windows.FILE_ATTRIBUTE_REPARSE_POINT != 0 || info.FileAttributes&windows.FILE_ATTRIBUTE_DIRECTORY == 0 {
			windows.CloseHandle(handle)
			release()
			return nil, errors.New("unconfirmed directory object")
		}
		handles = append(handles, handle)
	}
	return release, nil
}

func actualDirectory(path string) (string, error) {
	file, _, err := openCaptured(path, true, false)
	if err != nil {
		return "", err
	}
	defer file.Close()
	return actualDirectoryHandle(windows.Handle(file.Fd()))
}

func actualDirectoryHandle(handle windows.Handle) (string, error) {
	buffer := make([]uint16, 32768)
	n, err := windows.GetFinalPathNameByHandle(handle, &buffer[0], uint32(len(buffer)), 0)
	if err != nil || n == 0 || n >= uint32(len(buffer)) {
		return "", errors.New("actual directory unavailable")
	}
	return strings.ToLower(strings.TrimRight(windows.UTF16ToString(buffer[:n]), `\`)), nil
}

// Reopening a publishing journal only checks the exact declared final bytes.
// It never renames a leftover temporary package or overwrites another file.
func PublishedDigest(ctx context.Context, path string) (string, error) {
	release, err := pinDirectoryChain(filepath.Dir(path))
	if err != nil {
		return "", err
	}
	defer release()
	file, _, err := openCaptured(path, false, false)
	if err != nil {
		return "", err
	}
	defer file.Close()
	digest := sha256.New()
	if _, err = io.Copy(digest, contextReader{ctx, file}); err != nil {
		return "", err
	}
	return hex.EncodeToString(digest.Sum(nil)), nil
}

// Freeze an internal configuration snapshot through the same path/object rules
// as browser files. The returned release never deletes or modifies the source.
func FreezeFile(path string) (*os.File, func(), error) {
	release, err := pinDirectoryChain(filepath.Dir(path))
	if err != nil {
		return nil, nil, err
	}
	file, _, err := openCaptured(path, false, false)
	if err != nil {
		release()
		return nil, nil, err
	}
	return file, release, nil
}

func PinStagingDirectory(path string) (func(), error) { return pinDirectoryChain(path) }
