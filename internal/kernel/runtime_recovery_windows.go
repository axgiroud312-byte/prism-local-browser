//go:build windows

package kernel

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"time"

	"github.com/google/uuid"
	"golang.org/x/sys/windows"
)

type ManagedRecovery struct {
	ProcessState     string // not-created, exited, reused, alive, unconfirmed
	DirectoryFree    bool
	SessionMatches   bool
	RootPID          int
	ProcessCreatedAt string
	ResourcesExited  bool
}

// Never terminates a PID, adopts an unrelated process, or deletes a lock file.
// Private anonymous pipes cannot be reconstructed after an app restart.
func InspectManagedProfile(root, environmentID, sessionID, reference string, pid int, createdAt string, definitelyNotCreated bool) (ManagedRecovery, error) {
	result := ManagedRecovery{ProcessState: "not-created"}
	if parsed, err := uuid.Parse(sessionID); err != nil || parsed.String() != sessionID {
		return result, problem("VALIDATION_FAILED", "invalid-session-identity", "保存的会话标识无效，未操作任何进程。")
	}
	if pid < 0 {
		return result, problem("VALIDATION_FAILED", "invalid-process-identity", "保存的进程身份无效，未操作任何进程。")
	}
	if pid > 0 {
		if _, err := time.Parse(time.RFC3339Nano, createdAt); err != nil {
			return result, problem("VALIDATION_FAILED", "invalid-process-time", "保存的进程创建时间无法核对，未操作任何进程。")
		}
		handle, err := windows.OpenProcess(windows.SYNCHRONIZE|windows.PROCESS_QUERY_LIMITED_INFORMATION, false, uint32(pid))
		if err != nil {
			result.ProcessState = "unconfirmed"
			if errors.Is(err, windows.ERROR_INVALID_PARAMETER) {
				result.ProcessState = "exited"
			}
		} else {
			var created, exited, kernelTime, userTime windows.Filetime
			if err := windows.GetProcessTimes(handle, &created, &exited, &kernelTime, &userTime); err != nil {
				result.ProcessState = "unconfirmed"
			} else if time.Unix(0, created.Nanoseconds()).UTC().Format(time.RFC3339Nano) != createdAt {
				result.ProcessState = "reused"
			} else {
				state, err := windows.WaitForSingleObject(handle, 0)
				result.ProcessState = "unconfirmed"
				if err == nil && state == uint32(windows.WAIT_TIMEOUT) {
					result.ProcessState = "alive"
				}
				if err == nil && state == windows.WAIT_OBJECT_0 {
					result.ProcessState = "exited"
				}
			}
			_ = windows.CloseHandle(handle)
		}
	}
	lock, err := lockManagedProfile(root, environmentID, reference)
	if err != nil {
		var p *Problem
		if errors.As(err, &p) && p.Code == "DATA_DIR_LOCKED" {
			return result, nil
		}
		return result, err
	}
	defer lock.release()
	result.DirectoryFree = true
	result.ResourcesExited, err = managedJobResourcesExited(sessionID)
	if err != nil {
		return result, problem("SESSION_IDENTITY_UNCONFIRMED", "job-query-unavailable", "本次进程树资源无法核对，尚未解除目录保护；未接管或结束其他进程。")
	}
	if pid == 0 && definitelyNotCreated {
		result.SessionMatches = true
		return result, nil
	}
	info, err := lock.file.Stat()
	if err != nil || info.Size() < 1 || info.Size() > 4096 {
		return result, problem("SESSION_IDENTITY_UNCONFIRMED", "invalid-lock-record", "目录会话记录无法核对，未清除或覆盖原记录。")
	}
	encoded := make([]byte, info.Size())
	if _, err = lock.file.ReadAt(encoded, 0); err != nil && err != io.EOF {
		return result, err
	}
	var metadata struct {
		EnvironmentID   string `json:"environmentId"`
		SessionID       string `json:"sessionId"`
		PID             int    `json:"pid"`
		CreatedAt       string `json:"processCreatedAt"`
		ResourceVersion string `json:"resourceVersion"`
	}
	decoder := json.NewDecoder(bytes.NewReader(encoded))
	decoder.DisallowUnknownFields()
	if err = decoder.Decode(&metadata); err != nil {
		return result, problem("SESSION_IDENTITY_UNCONFIRMED", "invalid-lock-record", "目录会话记录无法核对，未清除或覆盖原记录。")
	}
	if err = decoder.Decode(&struct{}{}); err != io.EOF {
		return result, problem("SESSION_IDENTITY_UNCONFIRMED", "invalid-lock-record", "目录会话记录包含额外内容，未清除或覆盖原记录。")
	}
	result.SessionMatches = metadata.EnvironmentID == environmentID && metadata.SessionID == sessionID && metadata.PID == pid && metadata.CreatedAt == createdAt && metadata.ResourceVersion == ManagedRuntimeVersion
	if pid == 0 {
		if metadata.EnvironmentID != environmentID || metadata.SessionID != sessionID || metadata.PID < 1 || metadata.ResourceVersion != ManagedRuntimeVersion {
			return result, nil
		}
		// Creation preceded readiness. Inspect the actual identity from the
		// still-retained metadata instead of interpreting a zero journal PID.
		lock.release()
		observed, err := InspectManagedProfile(root, environmentID, sessionID, reference, metadata.PID, metadata.CreatedAt, false)
		observed.RootPID, observed.ProcessCreatedAt = metadata.PID, metadata.CreatedAt
		return observed, err
	}
	return result, nil
}
