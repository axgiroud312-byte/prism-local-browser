//go:build windows

package kernel

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/google/uuid"
	"golang.org/x/sys/windows"
)

// Recovery observes only the existing directory chain and ownership record.
// It must not create an absent pre-launch directory/lock or scan browser data.
// Normal Start separately validates the complete browser tree before use.
func inspectManagedProfileLock(root, environmentID, reference string, allowMissingDirectory, allowMissingLock bool) (*managedProfileLock, error) {
	parsed, err := uuid.Parse(environmentID)
	if err != nil || parsed.String() != environmentID || reference != "environments/"+environmentID+"/user-data" {
		return nil, problem("PATH_OUTSIDE_ROOT", "invalid-profile-reference", "环境目录引用不在受管理的独立路径中。")
	}
	releases := []func(){}
	var file *os.File
	var once sync.Once
	release := func() {
		once.Do(func() {
			if file != nil {
				file.Close()
			}
			for i := len(releases) - 1; i >= 0; i-- {
				releases[i]()
			}
		})
	}
	fail := func(err error) (*managedProfileLock, error) { release(); return nil, err }
	directory := root
	parts := append([]string{""}, strings.Split(reference, "/")...)
	for index, part := range parts {
		if index != 0 {
			directory = filepath.Join(directory, part)
		}
		pin, err := pinManagedDirectories(directory)
		if err != nil {
			if index > 0 && allowMissingDirectory && os.IsNotExist(err) {
				return &managedProfileLock{path: directory, release: release}, nil
			}
			return fail(err)
		}
		releases = append(releases, pin)
	}
	canonicalRoot, rootErr := finalDirectory(root)
	canonicalProfile, profileErr := finalDirectory(directory)
	if rootErr != nil || profileErr != nil || !strings.HasPrefix(canonicalProfile, canonicalRoot+`\`) {
		return fail(problem("PATH_OUTSIDE_ROOT", "profile-outside-root", "无法确认实际目录位于工作区内，保持保护。"))
	}
	lockPath := filepath.Join(directory, ".prism-runtime.lock")
	wide, _ := windows.UTF16PtrFromString(lockPath)
	handle, err := windows.CreateFile(wide, windows.GENERIC_READ, 0, nil, windows.OPEN_EXISTING, windows.FILE_ATTRIBUTE_NORMAL|windows.FILE_FLAG_OPEN_REPARSE_POINT, 0)
	if err != nil {
		if allowMissingLock && os.IsNotExist(err) {
			return &managedProfileLock{path: directory, release: release}, nil
		}
		if errors.Is(err, windows.ERROR_SHARING_VIOLATION) || errors.Is(err, windows.ERROR_LOCK_VIOLATION) {
			return fail(problem("DATA_DIR_LOCKED", "profile-in-use", "环境实际数据锁仍被占用，没有删除或覆盖原锁。"))
		}
		return fail(err)
	}
	file = os.NewFile(uintptr(handle), lockPath)
	var info windows.ByHandleFileInformation
	if err = windows.GetFileInformationByHandle(handle, &info); err != nil || info.NumberOfLinks != 1 || info.FileAttributes&(windows.FILE_ATTRIBUTE_REPARSE_POINT|windows.FILE_ATTRIBUTE_DIRECTORY) != 0 {
		return fail(problem("PATH_OUTSIDE_ROOT", "unsafe-profile-lock", "原环境锁文件不能安全确认，保持保护。"))
	}
	return &managedProfileLock{file: file, path: directory, release: release}, nil
}
