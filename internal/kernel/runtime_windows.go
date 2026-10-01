//go:build windows

package kernel

import (
	"context"
	"errors"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
	"unsafe"

	"github.com/google/uuid"
	"golang.org/x/sys/windows"
)

// ManagedProfile is internal, not an RPC argument. The only supported network
// policy here is explicitly selected direct; the workspace blocks proxy IDs.
type ManagedProfile struct {
	EnvironmentID string
	SessionID     string
	UserDataRef   string
	Fingerprint   FingerprintInput
	Width, Height int
	RestoreTabs   bool
	URLs          []string
}

type ManagedProcess struct {
	pipe     *pipeProcess
	done     chan struct{}
	release  func()
	stopGate chan struct{}
	mu       sync.Mutex
}

func (process *ManagedProcess) PID() int              { return int(process.pipe.pid) }
func (process *ManagedProcess) CreatedAt() string     { return process.pipe.createdAt }
func (process *ManagedProcess) Done() <-chan struct{} { return process.done }
func (process *ManagedProcess) Alive() bool {
	process.mu.Lock()
	defer process.mu.Unlock()
	select {
	case <-process.done:
		return false
	default:
	}
	state, err := windows.WaitForSingleObject(process.pipe.process, 0)
	return err == nil && state == uint32(windows.WAIT_TIMEOUT)
}

// The accounting structure must remain alive while the Job handle is open.
// ActiveProcesses, not just the browser root PID, determines lock release.
func (p *pipeProcess) activeProcesses() (uint32, error) {
	var accounting struct {
		TotalUserTime, TotalKernelTime, PeriodUserTime, PeriodKernelTime int64
		PageFaults, TotalProcesses, ActiveProcesses, TerminatedProcesses uint32
	}
	err := windows.QueryInformationJobObject(p.job, windows.JobObjectBasicAccountingInformation, uintptr(unsafe.Pointer(&accounting)), uint32(unsafe.Sizeof(accounting)), nil)
	return accounting.ActiveProcesses, err
}

func (process *ManagedProcess) observeExit() {
	// Closing a window is observable even without a Stop RPC. Keep the Job,
	// directory locks and immutable file pins until every owned child is gone.
	for {
		process.mu.Lock()
		state, err := windows.WaitForSingleObject(process.pipe.process, 100)
		if err == nil && state == windows.WAIT_OBJECT_0 {
			count, err := process.pipe.activeProcesses()
			if err == nil && count == 0 {
				process.pipe.close()
				process.release()
				close(process.done)
				process.mu.Unlock()
				return
			}
		}
		process.mu.Unlock()
		time.Sleep(50 * time.Millisecond)
	}
}

// Stop requests normal Browser.close. A timeout is NOT permission to kill and
// does not release the data locks; the caller must retain the live session.
func (process *ManagedProcess) Stop(ctx context.Context) error {
	select {
	case <-process.done:
		return nil
	case process.stopGate <- struct{}{}:
		defer func() { <-process.stopGate }()
	case <-ctx.Done():
		return ctx.Err()
	}
	if err := process.pipe.beginCommand(ctx); err != nil {
		select {
		case <-process.done:
			return nil
		default:
			return err
		}
	}
	_, sendErr := process.pipe.sendContext(ctx, "Browser.close", map[string]any{}, "")
	<-process.pipe.commandGate
	if sendErr != nil {
		select {
		case <-process.done:
			return nil
		default:
			return sendErr
		}
	}
	select {
	case <-process.done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// Close is for failed startup/app shutdown and affects only this private Job.
// It never searches by process name or closes an unrelated profile.
func (process *ManagedProcess) Close() error {
	process.mu.Lock()
	select {
	case <-process.done:
		process.mu.Unlock()
		return nil
	default:
	}
	if err := windows.TerminateJobObject(process.pipe.job, 1); err != nil {
		process.mu.Unlock()
		select {
		case <-process.done:
			return nil
		default:
			return err
		}
	}
	process.mu.Unlock()
	_ = windows.CancelIoEx(windows.Handle(process.pipe.write.Fd()), nil)
	_ = process.pipe.write.Close()
	select {
	case <-process.done:
		return nil
	case <-time.After(8 * time.Second):
		return problem("PROCESS_STOP_TIMEOUT", "owned-job-not-empty", "本次进程树退出尚未确认，目录与档案锁仍保留。")
	}
}

func LaunchManagedProfile(ctx context.Context, root string, record Record, profile ManagedProfile) (_ *ManagedProcess, resultErr error) {
	if sessionID, err := uuid.Parse(profile.SessionID); err != nil || sessionID.String() != profile.SessionID {
		return nil, problem("VALIDATION_FAILED", "invalid-session-id", "受控会话标识无效，未启动。")
	}
	parameters, err := CompileFingerprint(record, profile.Fingerprint)
	if err != nil {
		return nil, err
	}
	if profile.Width < 400 || profile.Width > 7680 || profile.Height < 400 || profile.Height > 7680 {
		return nil, problem("VALIDATION_FAILED", "invalid-window", "窗口偏好不在支持范围内。")
	}
	for _, value := range profile.URLs {
		parsed, err := url.Parse(value)
		if err != nil || parsed.Hostname() == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.User != nil {
			return nil, problem("VALIDATION_FAILED", "invalid-start-url", "启动网址必须是无凭据的HTTP/HTTPS地址。")
		}
	}
	lock, err := lockManagedProfile(root, profile.EnvironmentID, profile.UserDataRef)
	if err != nil {
		return nil, err
	}
	directory, err := RecordDirectory(root, record)
	if err != nil {
		lock.release()
		return nil, err
	}
	releaseFiles, err := PinFiles(directory, record.Files)
	if err != nil {
		lock.release()
		return nil, err
	}
	release := func() { releaseFiles(); lock.release() }
	if err = VerifyFiles(directory, record.Files); err != nil {
		release()
		return nil, err
	}
	executable := filepath.Join(directory, filepath.FromSlash(record.ExecutableRelativePath))
	actual, err := FileVersion(executable)
	if err != nil || actual != record.Version {
		release()
		return nil, problem("KERNEL_INTEGRITY_FAILED", "version-mismatch", "固定内核的实际PE版本不匹配，未改用其他构建。")
	}
	args := []string{"--no-first-run", "--no-default-browser-check", "--no-proxy-server", "--user-data-dir=" + lock.path, "--window-size=" + strconv.Itoa(profile.Width) + "," + strconv.Itoa(profile.Height)}
	if profile.RestoreTabs {
		args = append(args, "--restore-last-session")
	}
	args = append(args, parameters...)
	args = append(args, "about:blank")
	p, err := startPipe(executable, args)
	if err != nil {
		release()
		return nil, problem("PROCESS_START_FAILED", "native-start-failed", "所选真实内核未能启动，未关闭沙箱或尝试其他内核。")
	}
	var releaseOnce sync.Once
	process := &ManagedProcess{pipe: p, done: make(chan struct{}), release: func() { releaseOnce.Do(release) }, stopGate: make(chan struct{}, 1)}
	go process.observeExit()
	stopStartup := context.AfterFunc(ctx, func() { _ = process.Close() })
	defer func() {
		stopStartup()
		if resultErr != nil {
			resultErr = errors.Join(resultErr, process.Close())
		}
	}()
	if err = lock.record(profile.EnvironmentID, profile.SessionID, p.pid, p.createdAt); err != nil {
		return process, err
	}
	var browser struct {
		Product string `json:"product"`
	}
	if err = p.call(ctx, "Browser.getVersion", map[string]any{}, "", &browser); err != nil {
		return process, err
	}
	if strings.TrimPrefix(browser.Product, "Chrome/") != record.Version {
		return process, problem("KERNEL_INTEGRITY_FAILED", "identity-mismatch", "私有控制通道的真实版本与固定构建不匹配。")
	}
	var targets struct {
		Items []struct {
			ID   string `json:"targetId"`
			Type string `json:"type"`
		} `json:"targetInfos"`
	}
	if err = p.call(ctx, "Target.getTargets", map[string]any{}, "", &targets); err != nil {
		return process, err
	}
	pageFound := false
	for _, target := range targets.Items {
		if target.Type == "page" {
			pageFound = true
			break
		}
	}
	if !pageFound {
		return process, problem("PROCESS_READY_TIMEOUT", "no-page-target", "真实进程尚无可响应的网页目标，未报告运行中。")
	}
	if err = VerifyFiles(directory, record.Files); err != nil {
		return process, err
	}
	// User URLs are opened only after exact-build/process/control readiness.
	for _, value := range profile.URLs {
		if err = p.call(ctx, "Target.createTarget", map[string]any{"url": value}, "", nil); err != nil {
			return process, err
		}
	}
	if !stopStartup() || ctx.Err() != nil {
		return process, context.Canceled
	}
	if !process.Alive() {
		return process, problem("PROCESS_START_FAILED", "exited-before-ready", "浏览器主进程在就绪前已退出，未报告运行中；子进程资源仍按本次Job保护。")
	}
	return process, nil
}
