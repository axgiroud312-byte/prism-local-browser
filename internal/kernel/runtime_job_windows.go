//go:build windows

package kernel

import (
	"errors"
	"runtime"
	"unsafe"

	"github.com/google/uuid"
	"golang.org/x/sys/windows"
)

const ManagedRuntimeVersion = "windows-managed-job-v1"

var managedJobDLL = windows.NewLazySystemDLL("kernel32.dll")
var createManagedJobProc = managedJobDLL.NewProc("CreateJobObjectW")
var inspectManagedJobProc = managedJobDLL.NewProc("OpenJobObjectW")

func managedJobName(sessionID string) (*uint16, error) {
	parsed, err := uuid.Parse(sessionID)
	if err != nil || parsed.String() != sessionID {
		return nil, errors.New("invalid managed job identity")
	}
	// A user can reopen the same workspace from another Windows logon session.
	// QUERY must not falsely report the old tree absent in a different Local namespace.
	return windows.UTF16PtrFromString(`Global\PrismManagedSession-` + sessionID)
}

func createManagedJob(sessionID string) (windows.Handle, error) {
	if sessionID == "" {
		return windows.CreateJobObject(nil, nil)
	} // T04 diagnostics remain unnamed
	name, err := managedJobName(sessionID)
	if err != nil {
		return 0, err
	}
	user, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		return 0, err
	}
	descriptor, err := windows.SecurityDescriptorFromString("D:P(A;;GA;;;SY)(A;;GA;;;" + user.User.Sid.String() + ")")
	if err != nil {
		return 0, err
	}
	attributes := windows.SecurityAttributes{Length: uint32(unsafe.Sizeof(windows.SecurityAttributes{})), SecurityDescriptor: descriptor}
	value, _, callErr := createManagedJobProc.Call(uintptr(unsafe.Pointer(&attributes)), uintptr(unsafe.Pointer(name)))
	runtime.KeepAlive(descriptor)
	if value == 0 {
		return 0, callErr
	}
	handle := windows.Handle(value)
	if errors.Is(callErr, windows.ERROR_ALREADY_EXISTS) {
		_ = windows.CloseHandle(handle)
		return 0, problem("SESSION_IDENTITY_UNCONFIRMED", "job-name-already-exists", "本次会话的进程树身份已被占用，未接管或修改既有Job。")
	}
	return handle, nil
}

// Recovery obtains QUERY rights only, never TERMINATE or ASSIGN_PROCESS.
// Windows destroys a Job only after handles close and associated processes
// exit; root death plus a free app-owned directory lock alone is insufficient.
func managedJobResourcesExited(sessionID string) (bool, error) {
	name, err := managedJobName(sessionID)
	if err != nil {
		return false, err
	}
	value, _, callErr := inspectManagedJobProc.Call(4 /* JOB_OBJECT_QUERY */, 0, uintptr(unsafe.Pointer(name)))
	if value == 0 {
		if errors.Is(callErr, windows.ERROR_FILE_NOT_FOUND) {
			return true, nil
		}
		return false, callErr
	}
	handle := windows.Handle(value)
	defer windows.CloseHandle(handle)
	// Reuse the live supervisor's accounting layout; x/sys exposes the query
	// call but not the BASIC_ACCOUNTING_INFORMATION struct for this version.
	count, err := (&pipeProcess{job: handle}).activeProcesses()
	if err != nil {
		return false, err
	}
	return count == 0, nil
}
