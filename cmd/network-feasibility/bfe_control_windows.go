//go:build windows

package main

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
	"unsafe"

	"github.com/axgiroud312-byte/prism-local-browser/internal/kernel"
	"golang.org/x/sys/windows"
)

type bfeControlReport struct {
	Format            string           `json:"format"`
	Nonce             string           `json:"nonce"`
	StartedAt         string           `json:"startedAt"`
	FinishedAt        string           `json:"finishedAt"`
	Stage             string           `json:"stage"`
	Rights            uint32           `json:"rights"`
	StopCalls         int              `json:"stopCalls"`
	StopStartedAt     string           `json:"stopStartedAt,omitempty"`
	StopReturnedAt    string           `json:"stopReturnedAt,omitempty"`
	StopAccepted      bool             `json:"stopAccepted"`
	StopError         uint32           `json:"stopError"`
	Before            bfeServiceState  `json:"before"`
	AfterRequest      bfeServiceState  `json:"afterRequest"`
	Stopped           *bfeServiceState `json:"stopped,omitempty"`
	Final             bfeServiceState  `json:"final"`
	WatchdogReady     bool             `json:"watchdogReady"`
	WatchdogExited    bool             `json:"watchdogExited"`
	ProbeCompleted    bool             `json:"probeCompleted"`
	Restored          bool             `json:"restored"`
	RecoveryUncertain bool             `json:"recoveryUncertain"`
	Error             string           `json:"error,omitempty"`
}

func bfeKnownRefusal(code uint32) bool {
	switch windows.Errno(code) {
	case windows.ERROR_ACCESS_DENIED, windows.ERROR_DEPENDENT_SERVICES_RUNNING,
		windows.ERROR_INVALID_SERVICE_CONTROL, windows.ERROR_SERVICE_CANNOT_ACCEPT_CTRL,
		windows.ERROR_SERVICE_NOT_ACTIVE:
		return true
	default:
		return false
	}
}

func controlBFENormalStop(path string) (resultErr error) {
	dir, unpin, err := bfeEvidenceDirectory(path, false)
	if err != nil {
		return err
	}
	defer unpin()
	var ready bfeProbeReady
	if err = bfeRead(dir, "probe-ready.json", &ready); err != nil {
		return err
	}
	nonce := strings.TrimPrefix(filepath.Base(dir), "bfe-stop-")
	if ready.Format != bfeExperimentFormat || ready.Nonce != nonce || !ready.BaselineConfirmed {
		return errors.New("invalid non-elevated readiness record")
	}
	when, err := time.Parse(time.RFC3339Nano, ready.At)
	if err != nil || time.Since(when) < 0 || time.Since(when) > 4*time.Minute {
		return errors.New("probe readiness expired; no service request made")
	}
	probe, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION|windows.SYNCHRONIZE, false, ready.PID)
	if err != nil {
		return err
	}
	defer windows.CloseHandle(probe)
	var created, exited, kt, ut windows.Filetime
	if err = windows.GetProcessTimes(probe, &created, &exited, &kt, &ut); err != nil || ready.Created != uint64(created.HighDateTime)<<32|uint64(created.LowDateTime) {
		return errors.New("probe process identity changed")
	}
	var probeToken windows.Token
	if err = windows.OpenProcessToken(probe, windows.TOKEN_QUERY, &probeToken); err != nil {
		return err
	}
	elevated := probeToken.IsElevated()
	probeToken.Close()
	if elevated {
		return errors.New("probe host must be non-elevated")
	}
	self, err := os.Executable()
	if err != nil {
		return err
	}
	imagePin, err := kernel.PinFiles(filepath.Dir(self), map[string]string{filepath.Base(self): ready.ExecutableSHA256})
	if err != nil {
		return err
	}
	defer imagePin()
	digest, err := fileHash(self)
	if err != nil || digest != ready.ExecutableSHA256 {
		return errors.New("controller and ready probe executable digests differ")
	}
	if err = bfeWrite(dir, "controller-claim.json", map[string]any{"nonce": nonce, "at": time.Now().UTC().Format(time.RFC3339Nano)}); err != nil {
		return err
	}
	r := bfeControlReport{Format: bfeExperimentFormat, Nonce: nonce, StartedAt: time.Now().UTC().Format(time.RFC3339Nano), Stage: "open-service", Rights: windows.SERVICE_QUERY_STATUS | windows.SERVICE_STOP | windows.SERVICE_START}
	defer func() {
		r.FinishedAt = time.Now().UTC().Format(time.RFC3339Nano)
		r.Final = readBFEState()
		r.Restored = !r.RecoveryUncertain && r.Final.Error == "" && r.Final.State == windows.SERVICE_RUNNING
		if resultErr != nil {
			r.Error = resultErr.Error()
		}
		resultErr = errors.Join(resultErr, bfeWrite(dir, "controller-report.json", r))
		resultErr = errors.Join(resultErr, bfeWrite(dir, "controller-finished.json", map[string]any{"nonce": nonce, "restored": r.Restored, "at": r.FinishedAt}))
	}()
	h, err := openBFEService(r.Rights)
	if err != nil {
		var code windows.Errno
		if errors.As(err, &code) {
			r.StopError = uint32(code)
		}
		return fmt.Errorf("OpenService (STOP not called): %w", err)
	}
	defer windows.CloseServiceHandle(h)
	r.Before = bfeState(h)
	if r.Before.Error != "" || r.Before.State != windows.SERVICE_RUNNING {
		return errors.New("BFE original RUNNING state not confirmed")
	}
	r.Stage = "start-independent-recovery"
	guard := exec.Command(self, "--bfe-recover", dir, strconv.Itoa(os.Getpid()))
	guard.SysProcAttr = &syscall.SysProcAttr{CreationFlags: windows.DETACHED_PROCESS | windows.CREATE_NEW_PROCESS_GROUP}
	input, err := guard.StdinPipe()
	if err != nil {
		return err
	}
	defer input.Close()
	output, err := guard.StdoutPipe()
	if err != nil {
		return err
	}
	defer output.Close()
	guardLog, err := os.OpenFile(filepath.Join(dir, "recovery-stderr.txt"), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	defer guardLog.Close()
	guard.Stderr = guardLog
	if err = guard.Start(); err != nil {
		return err
	}
	guardDone := make(chan error, 1)
	go func() { guardDone <- guard.Wait() }()
	decoder := json.NewDecoder(output)
	readiness := make(chan error, 1)
	go func() {
		var reply struct {
			Ready bool   `json:"ready"`
			Nonce string `json:"nonce"`
		}
		err := decoder.Decode(&reply)
		if err == nil && (!reply.Ready || reply.Nonce != nonce) {
			err = errors.New("recovery handshake mismatch")
		}
		readiness <- err
	}()
	select {
	case err = <-readiness:
	case <-time.After(10 * time.Second):
		err = errors.New("independent recovery readiness timed out")
	}
	if err != nil {
		input.Close() // No arm command was sent, so this exits without service writes.
		select {
		case <-guardDone:
			r.WatchdogExited = true
		case <-time.After(5 * time.Second):
		}
		return err
	}
	r.WatchdogReady = true
	// Registered before STOP. Even evidence/probe failure immediately restores;
	// the independent guard remains armed until this exact completion handshake.
	defer func() {
		noEffect := r.StopCalls == 0 || !r.StopAccepted && bfeKnownRefusal(r.StopError)
		mayDisarm := noEffect || r.StopAccepted && r.Stopped != nil
		r.RecoveryUncertain = !mayDisarm
		var restoreErr error
		if !noEffect {
			restoreErr = restoreBFERunning(h, 30*time.Second)
		}
		resultErr = errors.Join(resultErr, restoreErr)
		if restoreErr == nil && mayDisarm {
			command := "release-after-confirmed-running"
			if noEffect {
				command = "cancel-no-stop-effect"
			}
			_, _ = fmt.Fprintln(input, command)
			if !r.WatchdogExited {
				select {
				case err := <-guardDone:
					r.WatchdogExited = true
					resultErr = errors.Join(resultErr, err)
				case <-time.After(5 * time.Second):
					resultErr = errors.Join(resultErr, errors.New("independent recovery exit not yet confirmed"))
				}
			}
		}
		if !mayDisarm {
			resultErr = errors.Join(resultErr, errors.New("STOP outcome remains uncertain; independent recovery stays armed after controller exit"))
		}
	}()
	state, err := windows.WaitForSingleObject(probe, 0)
	if err != nil || state != uint32(windows.WAIT_TIMEOUT) {
		return errors.New("non-elevated probe exited before service control")
	}
	select {
	case <-guardDone:
		r.WatchdogExited = true
		return errors.New("recovery process exited before control")
	default:
	}
	current := bfeState(h)
	if current.State != windows.SERVICE_RUNNING || current.Error != "" || current.PID != r.Before.PID {
		return errors.New("BFE identity/state changed before request")
	}
	r.Stage, r.StopStartedAt = "ControlService-STOP", time.Now().UTC().Format(time.RFC3339Nano)
	if err = bfeWrite(dir, "stop-request.json", map[string]any{"api": "ControlService", "control": "STOP", "service": "BFE", "at": r.StopStartedAt, "nonce": nonce}); err != nil {
		return err
	}
	if _, err = fmt.Fprintln(input, "arm-stop-intent"); err != nil {
		return err
	}
	armed := make(chan error, 1)
	go func() {
		var reply struct {
			Armed bool   `json:"armed"`
			Nonce string `json:"nonce"`
		}
		err := decoder.Decode(&reply)
		if err == nil && (!reply.Armed || reply.Nonce != nonce) {
			err = errors.New("recovery arm handshake mismatch")
		}
		armed <- err
	}()
	select {
	case err = <-armed:
	case <-time.After(5 * time.Second):
		err = errors.New("recovery arm handshake timeout")
	}
	if err != nil {
		return err
	}
	// Readiness can change while evidence is flushed or the recovery ACK waits.
	// Recheck immediately before the only STOP; an abort still has StopCalls=0
	// and therefore cancels the guard without starting an unrelated service.
	state, err = windows.WaitForSingleObject(probe, 0)
	if err != nil || state != uint32(windows.WAIT_TIMEOUT) {
		return errors.New("non-elevated probe exited during recovery arm handshake")
	}
	current = bfeState(h)
	if current.Error != "" || current.State != windows.SERVICE_RUNNING || current.PID != r.Before.PID {
		return errors.New("BFE identity/state changed during recovery arm handshake")
	}
	var ignored windows.SERVICE_STATUS
	r.StopCalls = 1
	err = windows.ControlService(h, windows.SERVICE_CONTROL_STOP, &ignored) // THE ONLY STOP CALL; no retry/cascade.
	r.StopReturnedAt = time.Now().UTC().Format(time.RFC3339Nano)
	r.StopAccepted = err == nil
	if err != nil {
		var code windows.Errno
		if errors.As(err, &code) {
			r.StopError = uint32(code)
		}
	}
	r.AfterRequest = bfeState(h) // Never trust undefined ControlService output on failure.
	if writeErr := bfeWrite(dir, "stop-returned.json", r); writeErr != nil {
		return writeErr
	}
	if err != nil {
		r.Stage = "request-refused"
		if !bfeKnownRefusal(r.StopError) {
			// Unknown/timeout does not prove the request was never forwarded.
			// Keep the recovery watcher armed through a bounded observation period.
			r.Stage = "request-result-uncertain"
			until := time.Now().Add(60 * time.Second)
			for time.Now().Before(until) {
				_ = restoreBFERunning(h, time.Second)
				time.Sleep(100 * time.Millisecond)
			}
		}
		return nil // A recorded Windows refusal is an experiment outcome, not a retry instruction.
	}
	r.Stage = "wait-stopped"
	stopped, err := bfeWaitState(h, windows.SERVICE_STOPPED, 10*time.Second)
	if err != nil {
		return err
	}
	r.Stopped = &stopped
	if err = bfeWrite(dir, "probe-stopped-request.json", map[string]any{"nonce": nonce, "at": stopped.At}); err != nil {
		return err
	}
	r.Stage = "probe-stopped-window"
	deadline := time.Now().Add(13 * time.Second)
	for time.Now().Before(deadline) {
		var phase bfeProbePhase
		if bfeRead(dir, "probe-stopped.json", &phase) == nil {
			if phase.Phase != "stopped" || phase.Before.State != windows.SERVICE_STOPPED || phase.After.State != windows.SERVICE_STOPPED || len(phase.Samples) != 2 {
				return errors.New("probe did not complete within confirmed STOPPED observations")
			}
			for _, sample := range phase.Samples {
				if sample.Error != "" || sample.TokenError || len(sample.Sockets) != 4 {
					return errors.New("STOPPED probe failed to collect complete helper evidence")
				}
			}
			r.ProbeCompleted = true
			break
		}
		if state, err := windows.WaitForSingleObject(probe, 0); err != nil || state != uint32(windows.WAIT_TIMEOUT) {
			return errors.New("probe host exited during STOPPED window")
		}
		if state := bfeState(h); state.Error != "" || state.State != windows.SERVICE_STOPPED {
			return errors.New("BFE left STOPPED before probe completion")
		}
		time.Sleep(100 * time.Millisecond)
	}
	if !r.ProbeCompleted {
		return errors.New("STOPPED probe result deadline exceeded")
	}
	r.Stage = "restore-running"
	return nil
}

// Independent executable with only QUERY|START service rights. It is never
// placed in the probe/controller Job, and never terminates any process.
func recoverBFEWatchdog(path string, parentPID uint32) (resultErr error) {
	dir, unpin, err := bfeEvidenceDirectory(path, false)
	if err != nil {
		return err
	}
	defer unpin()
	nonce := strings.TrimPrefix(filepath.Base(dir), "bfe-stop-")
	var inJob uint32
	ok, _, e := windows.NewLazySystemDLL("kernel32.dll").NewProc("IsProcessInJob").Call(uintptr(windows.CurrentProcess()), 0, uintptr(unsafe.Pointer(&inJob)))
	if ok == 0 || inJob != 0 {
		return fmt.Errorf("recovery must be outside any kill-on-close Job: %v", e)
	}
	parent, err := windows.OpenProcess(windows.SYNCHRONIZE, false, parentPID)
	if err != nil {
		return err
	}
	defer windows.CloseHandle(parent)
	h, err := openBFEService(windows.SERVICE_QUERY_STATUS | windows.SERVICE_START)
	if err != nil {
		return err
	}
	defer windows.CloseServiceHandle(h)
	original := bfeState(h)
	if original.Error != "" || original.State != windows.SERVICE_RUNNING {
		return errors.New("recovery original state not RUNNING")
	}
	armedAt := time.Time{}
	if err = bfeWrite(dir, "recovery-ready.json", map[string]any{"nonce": nonce, "pid": os.Getpid(), "outsideJob": true, "rights": windows.SERVICE_QUERY_STATUS | windows.SERVICE_START, "original": original}); err != nil {
		return err
	}
	if err = json.NewEncoder(os.Stdout).Encode(map[string]any{"ready": true, "nonce": nonce}); err != nil {
		return err
	}
	commands := make(chan string, 4)
	go func() {
		scanner := bufio.NewScanner(os.Stdin)
		for scanner.Scan() {
			commands <- scanner.Text()
		}
		commands <- "controller-pipe-ended"
	}()
	armed, released, recovery, parentEnded := false, false, false, time.Time{}
	attempts := 0
	lastError := ""
	defer func() {
		resultErr = errors.Join(resultErr, bfeWrite(dir, "recovery-finished.json", map[string]any{"nonce": nonce, "at": time.Now().UTC().Format(time.RFC3339Nano), "attempts": attempts, "final": bfeState(h), "lastError": lastError, "armed": armed, "released": released}))
	}()
	for {
		select {
		case command := <-commands:
			switch command {
			case "arm-stop-intent":
				if armed {
					return errors.New("recovery arm replay refused")
				}
				armed, armedAt = true, time.Now()
				if err := json.NewEncoder(os.Stdout).Encode(map[string]any{"armed": true, "nonce": nonce}); err != nil {
					recovery = true
				}
			case "cancel-no-stop-effect":
				return nil
			case "release-after-confirmed-running":
				released = true
			default:
				if !armed {
					return nil
				}
				recovery = true
			}
		default:
		}
		state, waitErr := windows.WaitForSingleObject(parent, 0)
		if waitErr != nil || state == windows.WAIT_OBJECT_0 {
			if !armed {
				return nil
			}
			recovery = true
			if parentEnded.IsZero() {
				parentEnded = time.Now()
			}
		}
		if armed && time.Since(armedAt) >= 25*time.Second {
			recovery = true
		}
		current := bfeState(h)
		if armed && (recovery || released) {
			if current.Error == "" && current.State == windows.SERVICE_STOPPED {
				attempts++
				if err := windows.StartService(h, 0, nil); err != nil && err != windows.ERROR_SERVICE_ALREADY_RUNNING {
					lastError = err.Error()
				}
			}
			// An early RUNNING query while STOP is in flight cannot disarm us.
			// After unexpected parent exit, monitor a full minute for a late stop.
			if current.Error == "" && current.State == windows.SERVICE_RUNNING && (released || !parentEnded.IsZero() && time.Since(parentEnded) >= time.Minute) {
				return nil
			}
		}
		time.Sleep(100 * time.Millisecond)
	}
}
