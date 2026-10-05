//go:build windows

package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
	"unsafe"

	"github.com/axgiroud312-byte/prism-local-browser/internal/desktopbase"
	"github.com/google/uuid"
	"golang.org/x/sys/windows"
)

const bfeExperimentFormat = "prism-bfe-normal-stop-v1"

func bfeExperimentCommand(args []string) error {
	switch {
	case len(args) == 2 && args[0] == "--bfe-prepare":
		if os.Getenv("PRISM_BFE_NORMAL_STOP") != "1" || windows.GetCurrentProcessToken().IsElevated() {
			return errors.New("prepare requires explicit opt-in and a non-elevated host")
		}
		return prepareBFEProbe(args[1])
	case len(args) == 2 && args[0] == "--bfe-controller":
		if !windows.GetCurrentProcessToken().IsElevated() {
			return errors.New("controller requires manual UAC elevation")
		}
		return controlBFENormalStop(args[1])
	case len(args) == 3 && args[0] == "--bfe-recover":
		if !windows.GetCurrentProcessToken().IsElevated() {
			return errors.New("recovery requires controller elevation")
		}
		pid, err := strconv.ParseUint(args[2], 10, 32)
		if err != nil || pid == 0 {
			return errors.New("invalid controller identity")
		}
		return recoverBFEWatchdog(args[1], uint32(pid))
	case len(args) == 2 && args[0] == "--bfe-probe-child":
		if os.Getenv("PRISM_BFE_NORMAL_STOP") != "1" {
			return errors.New("child opt-in missing")
		}
		return bfeProbeChild(args[1])
	default:
		return errors.New("invalid narrow BFE experiment command")
	}
}

// Every privileged output is CREATE_NEW beneath an existing pinned evidence
// directory. No input path is a service name, executable or deletion target.
func bfeEvidenceDirectory(path string, create bool) (string, func(), error) {
	self, err := os.Executable()
	if err != nil {
		return "", nil, err
	}
	base := filepath.Join(filepath.Dir(filepath.Dir(self)), "output", "goal", "T11")
	absolute, err := filepath.Abs(path)
	if err != nil {
		return "", nil, err
	}
	name := filepath.Base(absolute)
	if !strings.EqualFold(filepath.Dir(absolute), base) || !strings.HasPrefix(name, "bfe-stop-") {
		return "", nil, errors.New("expected a new bfe-stop-UUID directory under the repository T11 output")
	}
	if _, err = uuid.Parse(strings.TrimPrefix(name, "bfe-stop-")); err != nil {
		return "", nil, err
	}
	parentPin, err := desktopbase.PinDirectories(base)
	if err != nil {
		return "", nil, err
	}
	defer parentPin()
	if create {
		if err = os.Mkdir(absolute, 0700); err != nil {
			return "", nil, err
		}
	}
	pin, err := desktopbase.PinDirectories(absolute)
	return absolute, pin, err
}

func bfeWrite(dir, name string, value any) error {
	file, err := os.OpenFile(filepath.Join(dir, name), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	err = json.NewEncoder(file).Encode(value)
	err = errors.Join(err, file.Sync(), file.Close())
	return err
}

func bfeRead(dir, name string, value any) error {
	file, err := os.Open(filepath.Join(dir, name))
	if err != nil {
		return err
	}
	defer file.Close()
	return json.NewDecoder(io.LimitReader(file, 1<<20)).Decode(value)
}

type bfeServiceState struct {
	Name      string `json:"name"`
	At        string `json:"at"`
	State     uint32 `json:"state"`
	Accepts   uint32 `json:"accepts"`
	PID       uint32 `json:"pid,omitempty"`
	Win32Exit uint32 `json:"win32Exit"`
	Error     string `json:"error,omitempty"`
}

func openBFEService(rights uint32) (windows.Handle, error) {
	scm, err := windows.OpenSCManager(nil, nil, windows.SC_MANAGER_CONNECT)
	if err != nil {
		return 0, err
	}
	defer windows.CloseServiceHandle(scm)
	name, _ := windows.UTF16PtrFromString("BFE")
	return windows.OpenService(scm, name, rights)
}

func bfeState(handle windows.Handle) bfeServiceState {
	r := bfeServiceState{Name: "BFE", At: time.Now().UTC().Format(time.RFC3339Nano)}
	var status windows.SERVICE_STATUS_PROCESS
	var needed uint32
	if err := windows.QueryServiceStatusEx(handle, windows.SC_STATUS_PROCESS_INFO, (*byte)(unsafe.Pointer(&status)), uint32(unsafe.Sizeof(status)), &needed); err != nil {
		r.Error = err.Error()
		return r
	}
	r.State, r.Accepts, r.PID, r.Win32Exit = status.CurrentState, status.ControlsAccepted, status.ProcessId, status.Win32ExitCode
	return r
}

func readBFEState() bfeServiceState {
	h, err := openBFEService(windows.SERVICE_QUERY_STATUS)
	if err != nil {
		return bfeServiceState{Name: "BFE", Error: err.Error()}
	}
	defer windows.CloseServiceHandle(h)
	return bfeState(h)
}

func bfeWaitState(h windows.Handle, want uint32, limit time.Duration) (bfeServiceState, error) {
	deadline := time.Now().Add(limit)
	for {
		state := bfeState(h)
		if state.Error != "" {
			return state, errors.New(state.Error)
		}
		if state.State == want {
			return state, nil
		}
		if time.Now().After(deadline) {
			return state, fmt.Errorf("BFE state %d not confirmed before wait deadline", want)
		}
		time.Sleep(100 * time.Millisecond)
	}
}

// Only restores the RUNNING state that was observed before the one-shot stop.
// Does not change startup configuration, dependencies, policy or host process.
func restoreBFERunning(h windows.Handle, limit time.Duration) error {
	deadline := time.Now().Add(limit)
	for {
		state := bfeState(h)
		if state.Error == "" {
			if state.State == windows.SERVICE_RUNNING {
				return nil
			}
			if state.State == windows.SERVICE_STOPPED {
				if err := windows.StartService(h, 0, nil); err != nil && err != windows.ERROR_SERVICE_ALREADY_RUNNING {
					return fmt.Errorf("BFE restart: %w", err)
				}
			}
		}
		if time.Now().After(deadline) {
			return errors.New("BFE original RUNNING state not yet restored")
		}
		time.Sleep(100 * time.Millisecond)
	}
}
