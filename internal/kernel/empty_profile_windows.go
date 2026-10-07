//go:build windows

package kernel

import (
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/google/uuid"
	"golang.org/x/sys/windows"
)

type EmptyProfileOwner struct {
	EnvironmentID string `json:"environmentId"`
	PlanID        string `json:"planId"`
	Index         int64  `json:"index"`
	DataReference string `json:"dataReference"`
}
type EmptyProfileLease struct {
	directory string
	marker    *os.File
	releases  []func()
	once      sync.Once
}

func (lease *EmptyProfileLease) Close() error {
	var err error
	lease.once.Do(func() {
		if lease.marker != nil {
			err = lease.marker.Close()
		}
		for index := len(lease.releases) - 1; index >= 0; index-- {
			lease.releases[index]()
		}
	})
	return err
}
func emptyProfileProblem() error {
	return &Problem{Code: "DATA_DIR_NOT_EMPTY", Reason: "batch-directory-unconfirmed", Message: "新目录的归属或空集合不能确认，原目录保留；不会清空、覆盖或认领外来目录。", Retryable: true}
}
func emptyProfileIO(err error) error {
	code, message := "DIRECTORY_WRITE_FAILED", "新目录准备受权限或文件系统资源限制，已创建的准备目录保留；没有清空、覆盖或复制其他环境。"
	if errors.Is(err, windows.ERROR_DISK_FULL) || errors.Is(err, windows.ERROR_HANDLE_DISK_FULL) {
		code, message = "DISK_FULL", "磁盘空间不足，批次保留已完成项与准备目录；释放空间后明确继续，不重做已完成环境。"
	}
	return &Problem{Code: code, Reason: "batch-directory-io", Message: message, Retryable: true}
}

// This verifies a visible empty set, not isolation from a hostile same-SID
// writer creating children between this observation and a SQLite commit.
func (lease *EmptyProfileLease) CheckEmpty() error {
	path, err := windows.UTF16PtrFromString(lease.directory)
	if err != nil {
		return emptyProfileProblem()
	}
	handle, err := windows.CreateFile(path, windows.GENERIC_READ, windows.FILE_SHARE_READ, nil, windows.OPEN_EXISTING, windows.FILE_FLAG_BACKUP_SEMANTICS|windows.FILE_FLAG_OPEN_REPARSE_POINT, 0)
	if err != nil {
		return emptyProfileProblem()
	}
	file := os.NewFile(uintptr(handle), lease.directory)
	defer file.Close()
	var info windows.ByHandleFileInformation
	if windows.GetFileInformationByHandle(handle, &info) != nil || info.FileAttributes&windows.FILE_ATTRIBUTE_REPARSE_POINT != 0 || info.FileAttributes&windows.FILE_ATTRIBUTE_DIRECTORY == 0 {
		return emptyProfileProblem()
	}
	entries, err := file.Readdirnames(1)
	if len(entries) != 0 || err != io.EOF {
		return emptyProfileProblem()
	}
	return nil
}

func openEmptyProfileMarker(path string, create bool) (*os.File, error) {
	name, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return nil, err
	}
	access, mode := uint32(windows.GENERIC_READ), uint32(windows.OPEN_EXISTING)
	if create {
		access |= windows.GENERIC_WRITE
		mode = windows.CREATE_NEW
	}
	handle, err := windows.CreateFile(name, access, 0, nil, mode, windows.FILE_ATTRIBUTE_NORMAL|windows.FILE_FLAG_OPEN_REPARSE_POINT, 0)
	if err != nil {
		return nil, err
	}
	file := os.NewFile(uintptr(handle), path)
	var info windows.ByHandleFileInformation
	if windows.GetFileInformationByHandle(handle, &info) != nil || info.FileAttributes&(windows.FILE_ATTRIBUTE_DIRECTORY|windows.FILE_ATTRIBUTE_REPARSE_POINT) != 0 || info.NumberOfLinks != 1 {
		file.Close()
		return nil, errors.New("marker object not regular and unique")
	}
	if !create && (info.FileSizeHigh != 0 || info.FileSizeLow > 4096) {
		file.Close()
		return nil, errors.New("marker larger than contract")
	}
	return file, nil
}

// The caller MUST first durably journal this exact identity. No source profile
// files are read. Existing unmarked or nonempty directories are never adopted.
func PrepareEmptyProfile(root string, owner EmptyProfileOwner) (_ *EmptyProfileLease, resultErr error) {
	environment, err := uuid.Parse(owner.EnvironmentID)
	if err != nil || environment.String() != owner.EnvironmentID {
		return nil, emptyProfileProblem()
	}
	plan, err := uuid.Parse(owner.PlanID)
	if err != nil || plan.String() != owner.PlanID || owner.Index < 0 || owner.DataReference != "environments/"+owner.EnvironmentID+"/user-data" {
		return nil, emptyProfileProblem()
	}
	root, err = filepath.Abs(root)
	if err != nil || filepath.VolumeName(root) == "" || strings.HasPrefix(root, `\\`) {
		return nil, emptyProfileProblem()
	}
	lease := &EmptyProfileLease{}
	defer func() {
		if resultErr != nil {
			_ = lease.Close()
		}
	}()
	// Pin each prefix before accessing its child. Sharing READ alone also
	// rejects an in-place reparse conversion; pins never delete directories.
	volume := filepath.VolumeName(root) + string(filepath.Separator)
	current := volume
	segments := strings.Split(strings.TrimPrefix(root, volume), string(filepath.Separator))
	for index := -1; index < len(segments); index++ {
		if index >= 0 {
			if segments[index] == "" {
				continue
			}
			current = filepath.Join(current, segments[index])
		}
		release, err := pinManagedDirectories(current)
		if err != nil {
			return nil, emptyProfileProblem()
		}
		lease.releases = append(lease.releases, release)
	}
	canonicalRoot, err := finalDirectory(root)
	if err != nil {
		return nil, emptyProfileProblem()
	}
	current = filepath.Join(root, "environments")
	if err = os.Mkdir(current, 0700); err != nil && !os.IsExist(err) {
		return nil, emptyProfileIO(err)
	}
	release, err := pinManagedDirectories(current)
	if err != nil {
		return nil, emptyProfileProblem()
	}
	lease.releases = append(lease.releases, release)
	environmentRoot := filepath.Join(current, owner.EnvironmentID)
	err = os.Mkdir(environmentRoot, 0700)
	created := err == nil
	if err != nil && !os.IsExist(err) {
		return nil, emptyProfileIO(err)
	}
	release, err = pinManagedDirectories(environmentRoot)
	if err != nil {
		return nil, emptyProfileProblem()
	}
	lease.releases = append(lease.releases, release)
	actual, err := finalDirectory(environmentRoot)
	if err != nil || actual != canonicalRoot+`\environments\`+owner.EnvironmentID {
		return nil, emptyProfileProblem()
	}
	marker, err := openEmptyProfileMarker(filepath.Join(environmentRoot, ".prism-batch.json"), created)
	if err != nil {
		return nil, emptyProfileProblem()
	}
	lease.marker = marker
	if created {
		encoded, err := json.Marshal(owner)
		if err != nil {
			return nil, emptyProfileProblem()
		}
		written, err := marker.Write(encoded)
		if err != nil || written != len(encoded) {
			return nil, emptyProfileIO(err)
		}
		if err = marker.Sync(); err != nil {
			return nil, emptyProfileIO(err)
		}
	} else {
		var stored EmptyProfileOwner
		decoder := json.NewDecoder(io.LimitReader(marker, 4097))
		decoder.DisallowUnknownFields()
		if decoder.Decode(&stored) != nil || decoder.Decode(&struct{}{}) != io.EOF || stored != owner {
			return nil, emptyProfileProblem()
		}
	}
	lease.directory = filepath.Join(environmentRoot, "user-data")
	if err = os.Mkdir(lease.directory, 0700); err != nil && !os.IsExist(err) {
		return nil, emptyProfileIO(err)
	}
	release, err = pinManagedDirectories(lease.directory)
	if err != nil {
		return nil, emptyProfileProblem()
	}
	lease.releases = append(lease.releases, release)
	actual, err = finalDirectory(lease.directory)
	if err != nil || actual != canonicalRoot+`\environments\`+owner.EnvironmentID+`\user-data` {
		return nil, emptyProfileProblem()
	}
	if err = lease.CheckEmpty(); err != nil {
		return nil, err
	}
	return lease, nil
}
