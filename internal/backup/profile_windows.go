//go:build windows

package backup

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/google/uuid"
	"golang.org/x/sys/windows"
)

type capturedEntry struct {
	path, relative string
	file           *os.File
	info           windows.ByHandleFileInformation
	directory      bool
}
type Profile struct {
	root    string
	entries []capturedEntry
	lock    *os.File
	release func()
	Missing bool
	// Borrowed rename-capable root, owned by MoveTree. Enumerate a duplicated
	// handle instead of reopening a path without FILE_SHARE_DELETE.
	enumerationRoot *os.File
}

func openCaptured(path string, directory bool, exclusive bool) (*os.File, windows.ByHandleFileInformation, error) {
	var info windows.ByHandleFileInformation
	name, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return nil, info, err
	}
	share := uint32(windows.FILE_SHARE_READ)
	if exclusive {
		share = 0
	}
	handle, err := windows.CreateFile(name, windows.GENERIC_READ, share, nil, windows.OPEN_EXISTING, windows.FILE_FLAG_BACKUP_SEMANTICS|windows.FILE_FLAG_OPEN_REPARSE_POINT, 0)
	if err != nil {
		return nil, info, err
	}
	if err = windows.GetFileInformationByHandle(handle, &info); err != nil || info.FileAttributes&windows.FILE_ATTRIBUTE_REPARSE_POINT != 0 || (info.FileAttributes&windows.FILE_ATTRIBUTE_DIRECTORY != 0) != directory || !directory && info.NumberOfLinks != 1 {
		windows.CloseHandle(handle)
		return nil, info, errors.New("unsafe or shared browser data object")
	}
	return os.NewFile(uintptr(handle), path), info, nil
}

// No source directory/lock is created, truncated or removed. A missing profile
// is only valid when the service proves it has never been initialized.
func CaptureProfile(ctx context.Context, root, environmentID, reference string, allowMissing, allowMissingLock bool) (_ *Profile, resultErr error) {
	parsed, err := uuid.Parse(environmentID)
	if err != nil || parsed.String() != environmentID || reference != "environments/"+environmentID+"/user-data" {
		return nil, errors.New("invalid managed profile reference")
	}
	release, err := pinDirectoryChain(root)
	if err != nil {
		return nil, err
	}
	profile := &Profile{root: filepath.Join(root, filepath.FromSlash(reference)), release: release}
	defer func() {
		if resultErr != nil {
			profile.Close()
		}
	}()
	// Pin each existing component before asking for its child. Missing is not
	// silently converted into empty after a prior managed session/data creation.
	current := root
	for _, part := range strings.Split(reference, "/") {
		current = filepath.Join(current, part)
		file, info, err := openCaptured(current, true, false)
		if os.IsNotExist(err) && allowMissing {
			profile.Missing = true
			return profile, nil
		}
		if err != nil {
			return nil, err
		}
		profile.entries = append(profile.entries, capturedEntry{path: current, file: file, info: info, directory: true})
	}
	lockPath := filepath.Join(profile.root, ".prism-runtime.lock")
	lock, _, err := openCaptured(lockPath, false, true)
	if err != nil && (!os.IsNotExist(err) || !allowMissingLock) {
		return nil, err
	}
	profile.lock = lock
	if err = profile.capture(ctx, profile.root, ""); err != nil {
		return nil, err
	}
	return profile, nil
}

func (p *Profile) capture(ctx context.Context, path, relative string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	// Fresh enumeration handle: prior reads must not leave an exhausted cursor.
	directory, _, err := openCaptured(path, true, false)
	if err != nil {
		return err
	}
	defer directory.Close()
	return p.captureDirectory(ctx, path, relative, directory)
}

func (p *Profile) captureDirectory(ctx context.Context, path, relative string, directory *os.File) error {
	for {
		names, readErr := directory.Readdirnames(256)
		for _, name := range names {
			if relative == "" && name == ".prism-runtime.lock" {
				continue
			}
			entryRelative := name
			if relative != "" {
				entryRelative = relative + "/" + name
			}
			if !ValidName(entryRelative) {
				return errors.New("unsafe browser data path")
			}
			entryPath := filepath.Join(path, name)
			wide, _ := windows.UTF16PtrFromString(entryPath)
			attributes, err := windows.GetFileAttributes(wide)
			if err != nil {
				return err
			}
			isDirectory := attributes&windows.FILE_ATTRIBUTE_DIRECTORY != 0
			file, info, err := openCaptured(entryPath, isDirectory, false)
			if err != nil {
				return err
			}
			p.entries = append(p.entries, capturedEntry{path: entryPath, relative: entryRelative, file: file, info: info, directory: isDirectory})
			if isDirectory {
				if err = p.capture(ctx, entryPath, entryRelative); err != nil {
					return err
				}
			}
		}
		if readErr == io.EOF {
			return nil
		}
		if readErr != nil {
			return readErr
		}
	}
}

func (p *Profile) Write(ctx context.Context, writer *Writer, prefix string) error {
	if err := writer.Add(ctx, prefix+"/", nil, 0, true); err != nil {
		return err
	}
	for _, entry := range p.entries {
		if entry.relative == "" {
			continue
		}
		size := int64(entry.info.FileSizeHigh)<<32 | int64(entry.info.FileSizeLow)
		if entry.directory {
			size = 0
		}
		if _, err := entry.file.Seek(0, io.SeekStart); err != nil && !entry.directory {
			return err
		}
		if err := writer.Add(ctx, prefix+"/"+entry.relative, entry.file, size, entry.directory); err != nil {
			return err
		}
	}
	return p.Validate(ctx)
}

func (p *Profile) Validate(ctx context.Context) error {
	if p.Missing {
		if _, err := os.Lstat(p.root); !os.IsNotExist(err) {
			return errors.New("uninitialized directory appeared during backup")
		}
		return ctx.Err()
	}
	expected := map[string]bool{}
	for _, entry := range p.entries {
		if err := ctx.Err(); err != nil {
			return err
		}
		var info windows.ByHandleFileInformation
		if err := windows.GetFileInformationByHandle(windows.Handle(entry.file.Fd()), &info); err != nil || info.FileAttributes != entry.info.FileAttributes || info.VolumeSerialNumber != entry.info.VolumeSerialNumber || info.FileIndexHigh != entry.info.FileIndexHigh || info.FileIndexLow != entry.info.FileIndexLow || info.LastWriteTime != entry.info.LastWriteTime || info.FileSizeHigh != entry.info.FileSizeHigh || info.FileSizeLow != entry.info.FileSizeLow || info.NumberOfLinks != entry.info.NumberOfLinks {
			return errors.New("captured browser data changed")
		}
		if entry.relative != "" {
			expected[entry.relative] = entry.directory
		}
	}
	seen := 0
	directories := []string{""}
	for _, entry := range p.entries {
		if entry.directory && entry.relative != "" {
			directories = append(directories, entry.relative)
		}
	}
	for _, relative := range directories {
		if err := ctx.Err(); err != nil {
			return err
		}
		path := filepath.Join(p.root, filepath.FromSlash(relative))
		var directory *os.File
		var err error
		if relative == "" && p.enumerationRoot != nil {
			var copy windows.Handle
			process := windows.CurrentProcess()
			err = windows.DuplicateHandle(process, windows.Handle(p.enumerationRoot.Fd()), process, &copy, 0, false, windows.DUPLICATE_SAME_ACCESS)
			if err == nil {
				directory = os.NewFile(uintptr(copy), path)
			}
		} else {
			directory, _, err = openCaptured(path, true, false)
		}
		if err != nil {
			return err
		}
		for {
			names, readErr := directory.Readdirnames(256)
			for _, name := range names {
				if relative == "" && name == ".prism-runtime.lock" {
					continue
				}
				entry := name
				if relative != "" {
					entry = relative + "/" + name
				}
				if _, exists := expected[entry]; !exists {
					directory.Close()
					return errors.New("browser data entry set changed")
				}
				seen++
			}
			if readErr == io.EOF {
				break
			}
			if readErr != nil {
				directory.Close()
				return readErr
			}
			if err = ctx.Err(); err != nil {
				directory.Close()
				return err
			}
		}
		if err = directory.Close(); err != nil {
			return err
		}
	}
	if seen != len(expected) {
		return errors.New("browser data entries disappeared")
	}
	return nil
}
func (p *Profile) Close() error {
	var err error
	for index := len(p.entries) - 1; index >= 0; index-- {
		err = errors.Join(err, p.entries[index].file.Close())
	}
	p.entries = nil
	if p.lock != nil {
		err = errors.Join(err, p.lock.Close())
		p.lock = nil
	}
	if p.release != nil {
		p.release()
		p.release = nil
	}
	return err
}
