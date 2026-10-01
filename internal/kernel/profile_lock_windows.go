//go:build windows

package kernel

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/axgiroud312-byte/prism-local-browser/internal/desktopbase"
	"github.com/google/uuid"
	"golang.org/x/sys/windows"
)

type managedProfileLock struct {
	file    *os.File
	path    string
	release func()
}

func finalDirectory(path string) (string, error) {
	wide, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return "", err
	}
	handle, err := windows.CreateFile(wide, windows.GENERIC_READ, windows.FILE_SHARE_READ, nil, windows.OPEN_EXISTING, windows.FILE_FLAG_BACKUP_SEMANTICS|windows.FILE_FLAG_OPEN_REPARSE_POINT, 0)
	if err != nil {
		return "", err
	}
	defer windows.CloseHandle(handle)
	var info windows.ByHandleFileInformation
	if err = windows.GetFileInformationByHandle(handle, &info); err != nil || info.FileAttributes&windows.FILE_ATTRIBUTE_REPARSE_POINT != 0 || info.FileAttributes&windows.FILE_ATTRIBUTE_DIRECTORY == 0 {
		return "", problem("PATH_OUTSIDE_ROOT", "unsafe-profile-directory", "实际环境目录含链接或无法核对，未启动。")
	}
	buffer := make([]uint16, 32768)
	n, err := windows.GetFinalPathNameByHandle(handle, &buffer[0], uint32(len(buffer)), 0)
	if err != nil || n == 0 || n >= uint32(len(buffer)) {
		return "", errors.New("directory identity unavailable")
	}
	return strings.TrimRight(strings.ToLower(windows.UTF16ToString(buffer[:n])), `\`), nil
}

// Deny both DELETE and WRITE access on the directory chain. Denying rename
// alone does not stop FSCTL_SET_REPARSE_POINT on the same directory object.
// File creation/writes *inside* these directories remain possible for Chromium.
func pinManagedDirectories(directory string) (func(), error) {
	handles := []windows.Handle{}
	release := func() {
		for index := len(handles) - 1; index >= 0; index-- {
			_ = windows.CloseHandle(handles[index])
		}
	}
	for current := directory; ; current = filepath.Dir(current) {
		wide, err := windows.UTF16PtrFromString(current)
		if err != nil {
			release()
			return nil, err
		}
		handle, err := windows.CreateFile(wide, windows.GENERIC_READ, windows.FILE_SHARE_READ, nil, windows.OPEN_EXISTING, windows.FILE_FLAG_BACKUP_SEMANTICS|windows.FILE_FLAG_OPEN_REPARSE_POINT, 0)
		if err != nil {
			release()
			return nil, err
		}
		var info windows.ByHandleFileInformation
		if err = windows.GetFileInformationByHandle(handle, &info); err != nil || info.FileAttributes&windows.FILE_ATTRIBUTE_REPARSE_POINT != 0 || info.FileAttributes&windows.FILE_ATTRIBUTE_DIRECTORY == 0 {
			_ = windows.CloseHandle(handle)
			release()
			return nil, problem("PATH_OUTSIDE_ROOT", "unsafe-profile-directory", "环境目录链无法安全固定，未启动。")
		}
		handles = append(handles, handle)
		if current == filepath.Dir(current) {
			break
		}
	}
	var once sync.Once
	return func() { once.Do(release) }, nil
}

func checkManagedProfileFiles(directory, ownedLock string) error {
	return filepath.WalkDir(directory, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if path == ownedLock {
			return nil
		} // its exclusive handle is checked separately
		wide, err := windows.UTF16PtrFromString(path)
		if err != nil {
			return err
		}
		handle, err := windows.CreateFile(wide, windows.GENERIC_READ, windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE, nil, windows.OPEN_EXISTING, windows.FILE_FLAG_BACKUP_SEMANTICS|windows.FILE_FLAG_OPEN_REPARSE_POINT, 0)
		if err != nil {
			return err
		}
		defer windows.CloseHandle(handle)
		var info windows.ByHandleFileInformation
		if err = windows.GetFileInformationByHandle(handle, &info); err != nil {
			return err
		}
		if info.FileAttributes&windows.FILE_ATTRIBUTE_REPARSE_POINT != 0 || (info.FileAttributes&windows.FILE_ATTRIBUTE_DIRECTORY != 0) != entry.IsDir() {
			return problem("PATH_OUTSIDE_ROOT", "unsafe-profile-entry", "已有浏览数据包含链接或类型变化，未启动。")
		}
		if !entry.IsDir() && info.NumberOfLinks != 1 {
			return problem("PATH_OUTSIDE_ROOT", "hardlinked-browser-data", "已有浏览数据包含共享物理文件的硬链接，不能当作独立环境启动。")
		}
		return nil
	})
}

func lockManagedProfile(root, environmentID, reference string) (*managedProfileLock, error) {
	parsed, err := uuid.Parse(environmentID)
	if err != nil || parsed.String() != environmentID || reference != "environments/"+environmentID+"/user-data" {
		return nil, problem("PATH_OUTSIDE_ROOT", "invalid-profile-reference", "环境目录引用不在受管理的独立路径中。")
	}
	release, err := EnsureDirectory(root, reference)
	if err != nil {
		return nil, err
	}
	directory := filepath.Join(root, filepath.FromSlash(reference))
	releasePinned, err := pinManagedDirectories(directory)
	if err != nil {
		release()
		return nil, err
	}
	releaseInitial := release
	release = func() { releasePinned(); releaseInitial() }
	failed := func(err error) (*managedProfileLock, error) { release(); return nil, err }
	if err := desktopbase.ValidateTree(directory); err != nil {
		return failed(problem("PATH_OUTSIDE_ROOT", "reparse-point", "已有浏览数据目录包含链接，未启动或修改数据。"))
	}
	canonicalRoot, rootErr := finalDirectory(root)
	canonicalProfile, profileErr := finalDirectory(directory)
	if rootErr != nil || profileErr != nil || !strings.HasPrefix(canonicalProfile, canonicalRoot+`\`) {
		return failed(problem("PATH_OUTSIDE_ROOT", "profile-outside-root", "无法确认浏览数据实际目录在工作区内，未启动。"))
	}
	lockPath := filepath.Join(directory, ".prism-runtime.lock")
	wide, _ := windows.UTF16PtrFromString(lockPath)
	handle, err := windows.CreateFile(wide, windows.GENERIC_READ|windows.GENERIC_WRITE, 0, nil, windows.OPEN_ALWAYS, windows.FILE_ATTRIBUTE_NORMAL|windows.FILE_FLAG_OPEN_REPARSE_POINT, 0)
	if err != nil {
		if errors.Is(err, windows.ERROR_SHARING_VIOLATION) || errors.Is(err, windows.ERROR_LOCK_VIOLATION) {
			return failed(problem("DATA_DIR_LOCKED", "profile-in-use", "本环境的实际数据目录正被占用，没有重复打开。"))
		}
		return failed(err)
	}
	var info windows.ByHandleFileInformation
	if err = windows.GetFileInformationByHandle(handle, &info); err != nil || info.NumberOfLinks != 1 || info.FileAttributes&(windows.FILE_ATTRIBUTE_REPARSE_POINT|windows.FILE_ATTRIBUTE_DIRECTORY) != 0 {
		windows.CloseHandle(handle)
		return failed(problem("PATH_OUTSIDE_ROOT", "unsafe-profile-lock", "环境锁文件不能安全确认，未启动。"))
	}
	file := os.NewFile(uintptr(handle), lockPath)
	if err := checkManagedProfileFiles(directory, lockPath); err != nil {
		_ = file.Close()
		return failed(err)
	}
	var once sync.Once
	return &managedProfileLock{file: file, path: directory, release: func() { once.Do(func() { _ = file.Close(); release() }) }}, nil
}

func (lock *managedProfileLock) record(environmentID, sessionID string, pid uint32, createdAt string) error {
	data, err := json.Marshal(map[string]any{"environmentId": environmentID, "sessionId": sessionID, "pid": pid, "processCreatedAt": createdAt})
	if err != nil {
		return err
	}
	if err = lock.file.Truncate(0); err != nil {
		return err
	}
	if _, err = lock.file.WriteAt(append(data, '\n'), 0); err != nil {
		return err
	}
	return lock.file.Sync()
}
