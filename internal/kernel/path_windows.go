//go:build windows

package kernel

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/axgiroud312-byte/prism-local-browser/internal/desktopbase"
	"golang.org/x/sys/windows"
)

// Distinguish a positively identified unsafe boundary from a transient I/O or
// sharing failure. Ordinary Windows errors must not certify corruption.
func CheckBoundary(path string, directory bool) error {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	for current := absolute; ; current = filepath.Dir(current) {
		wide, err := windows.UTF16PtrFromString(current)
		if err != nil {
			return err
		}
		attributes, err := windows.GetFileAttributes(wide)
		if err != nil {
			return err
		}
		if attributes&windows.FILE_ATTRIBUTE_REPARSE_POINT != 0 {
			return problem("PATH_OUTSIDE_ROOT", "reparse-point", "内核路径包含已确认的重解析点，不能使用。")
		}
		wantDirectory := current != absolute || directory
		if (attributes&windows.FILE_ATTRIBUTE_DIRECTORY != 0) != wantDirectory {
			return problem("KERNEL_INTEGRITY_FAILED", "invalid-file-type", "内核路径的文件/目录类型不符合可信清单，不能使用。")
		}
		if filepath.Dir(current) == current {
			break
		}
	}
	return nil
}

// EnsureDirectory creates one component at a time under a pinned real parent.
// MkdirAll alone could follow a concurrently introduced junction before checking.
func EnsureDirectory(root, relative string) (func(), error) {
	releaseRoot, err := desktopbase.PinDirectories(root)
	if err != nil {
		return nil, err
	}
	releases := []func(){releaseRoot}
	release := func() {
		for i := len(releases) - 1; i >= 0; i-- {
			releases[i]()
		}
	}
	current := root
	for _, part := range strings.Split(filepath.ToSlash(relative), "/") {
		if part == "" {
			continue
		}
		if validArchiveName(part) != nil {
			release()
			return nil, problem("PATH_OUTSIDE_ROOT", "unsafe-path", "目录路径不安全，未继续操作。")
		}
		current = filepath.Join(current, part)
		if err = desktopbase.ValidatePath(current); err != nil {
			release()
			return nil, err
		}
		if err = os.Mkdir(current, 0700); err != nil && !os.IsExist(err) {
			release()
			return nil, err
		}
		pin, err := desktopbase.PinDirectories(current)
		if err != nil {
			release()
			return nil, err
		}
		releases = append(releases, pin)
	}
	return release, nil
}

// PinFiles denies writes/replacement while hashes, file version and the probe
// are reading the selected executable and its DLL/resources, not just paths.
func PinFiles(root string, files map[string]string) (func(), error) {
	if err := CheckBoundary(root, true); err != nil {
		return nil, err
	}
	releaseRoot, err := desktopbase.PinDirectories(root)
	if err != nil {
		if boundaryErr := CheckBoundary(root, true); boundaryErr != nil {
			return nil, boundaryErr
		}
		return nil, err
	}
	handles := []windows.Handle{}
	directoryPins := []func(){}
	release := func() {
		for _, handle := range handles {
			windows.CloseHandle(handle)
		}
		for _, release := range directoryPins {
			release()
		}
		releaseRoot()
	}
	for relative := range files {
		if validArchiveName(relative) != nil {
			release()
			return nil, problem("PATH_OUTSIDE_ROOT", "unsafe-manifest", "内核文件清单路径不安全。")
		}
		path := filepath.Join(root, filepath.FromSlash(relative))
		if err := CheckBoundary(path, false); err != nil {
			release()
			return nil, err
		}
		pin, err := desktopbase.PinDirectories(filepath.Dir(path))
		if err != nil {
			release()
			if boundaryErr := CheckBoundary(path, false); boundaryErr != nil {
				return nil, boundaryErr
			}
			return nil, err
		}
		directoryPins = append(directoryPins, pin)
		if err = desktopbase.ValidatePath(path); err != nil {
			release()
			if boundaryErr := CheckBoundary(path, false); boundaryErr != nil {
				return nil, boundaryErr
			}
			return nil, err
		}
		wide, _ := windows.UTF16PtrFromString(path)
		handle, err := windows.CreateFile(wide, windows.GENERIC_READ, windows.FILE_SHARE_READ, nil, windows.OPEN_EXISTING, windows.FILE_FLAG_OPEN_REPARSE_POINT, 0)
		if err != nil {
			release()
			return nil, err
		}
		var info windows.ByHandleFileInformation
		if err = windows.GetFileInformationByHandle(handle, &info); err != nil {
			windows.CloseHandle(handle)
			release()
			return nil, err
		}
		if info.FileAttributes&(windows.FILE_ATTRIBUTE_REPARSE_POINT|windows.FILE_ATTRIBUTE_DIRECTORY) != 0 {
			windows.CloseHandle(handle)
			release()
			return nil, problem("PATH_OUTSIDE_ROOT", "reparse-point", "内核文件包含链接，未执行。")
		}
		handles = append(handles, handle)
	}
	return release, nil
}
