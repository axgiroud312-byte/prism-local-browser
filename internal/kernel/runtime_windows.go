//go:build windows

package kernel

import (
	"context"
	"errors"
	"net"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"unsafe"

	"github.com/axgiroud312-byte/prism-local-browser/internal/proxy"
	"github.com/google/uuid"
	"golang.org/x/sys/windows"
)

// ManagedProfile is host-only. A bound proxy MUST supply a private channel;
// no upstream credential or endpoint override is accepted by the RPC.
type ManagedNetwork interface {
	Endpoint() string
	BindBrowser(func(net.Conn) bool) error
	Close() error
	Fault() *proxy.CheckError
	Failed() <-chan struct{}
}
type ManagedProfile struct {
	EnvironmentID string
	SessionID     string
	UserDataRef   string
	Fingerprint   FingerprintInput
	Width, Height int
	RestoreTabs   bool
	URLs          []string
	OnCreated     func(int, string) error
	Network       ManagedNetwork
}

type ManagedProcess struct {
	pipe                 *pipeProcess
	done                 chan struct{}
	release              func()
	stopGate             chan struct{}
	mu                   sync.Mutex
	lastSnapshot         RuntimeSnapshot
	network              ManagedNetwork
	networkCleanupFailed bool
	control              managedProcessControl
	exited               chan struct{}
	jobExited            bool
	cleanup              *managedCleanupAttempt
}

// Root death, control loss, and complete resource exit are distinct facts.
type RuntimeSnapshot struct {
	RootAlive            bool
	ControlReady         bool
	ResourcesExited      bool
	ExitKnown            bool
	ExitCode             uint32
	ProxyError           *proxy.CheckError
	NetworkCleanupFailed bool
}

func (process *ManagedProcess) PID() int              { return int(process.pipe.pid) }
func (process *ManagedProcess) CreatedAt() string     { return process.pipe.createdAt }
func (process *ManagedProcess) Done() <-chan struct{} { return process.done }
func (process *ManagedProcess) Alive() bool {
	return process.Snapshot().RootAlive
}

func (process *ManagedProcess) Snapshot() RuntimeSnapshot {
	process.mu.Lock()
	result := process.lastSnapshot
	if !process.jobExited {
		if process.control.snapshot != nil {
			result = process.control.snapshot()
		} else {
			result = process.pipe.snapshot()
		}
	}
	result.NetworkCleanupFailed = process.networkCleanupFailed
	process.mu.Unlock()
	if process.network != nil {
		result.ProxyError = process.network.Fault()
	}
	return result
}

func (p *pipeProcess) snapshot() RuntimeSnapshot {
	state, err := windows.WaitForSingleObject(p.process, 0)
	result := RuntimeSnapshot{RootAlive: err == nil && state == uint32(windows.WAIT_TIMEOUT), ControlReady: !p.writeLost.Load()}
	select {
	case <-p.readEnded:
		result.ControlReady = false
	default:
	}
	if err == nil && state == windows.WAIT_OBJECT_0 {
		result.ExitKnown = windows.GetExitCodeProcess(p.process, &result.ExitCode) == nil
	}
	return result
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

func LaunchManagedProfile(ctx context.Context, root string, record Record, profile ManagedProfile) (_ *ManagedProcess, resultErr error) {
	var protected *ProtectedProxy
	if profile.Network != nil {
		var ok bool
		protected, ok = profile.Network.(*ProtectedProxy)
		if !ok {
			return nil, RequireProxyNetworkBoundary()
		}
		if err := protected.validate(root, record, profile); err != nil {
			return nil, err
		}
	}
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
	var lock *managedProfileLock
	if protected != nil {
		lock = protected.lock
	} else {
		lock, err = lockManagedProfile(root, profile.EnvironmentID, profile.UserDataRef)
	}
	if err != nil {
		return nil, err
	}
	directory, err := RecordDirectory(root, record)
	if err != nil {
		if protected == nil {
			lock.release()
		}
		return nil, err
	}
	releaseFiles, err := PinFiles(directory, record.Files)
	if err != nil {
		if protected == nil {
			lock.release()
		}
		return nil, err
	}
	var networkGuard *proxyJobGuard
	release := func() {
		if networkGuard != nil {
			networkGuard.close()
		}
		releaseFiles()
		if protected == nil {
			lock.release()
		}
	}
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
	endpoint := ""
	if profile.Network != nil {
		endpoint = profile.Network.Endpoint()
	}
	networkArgs, err := managedNetworkArguments(endpoint)
	if err != nil {
		release()
		return nil, err
	}
	args := []string{"--no-first-run", "--no-default-browser-check", "--user-data-dir=" + lock.path, "--window-size=" + strconv.Itoa(profile.Width) + "," + strconv.Itoa(profile.Height)}
	args = append(args, networkArgs...)
	if profile.RestoreTabs {
		args = append(args, "--restore-last-session")
	}
	args = append(args, parameters...)
	args = append(args, "about:blank")
	var bindJob func(windows.Handle) error
	if profile.Network != nil {
		bindJob = func(job windows.Handle) error {
			guard, err := newProxyJobGuard(job)
			if err != nil {
				return problem("PROXY_BRIDGE_UNAVAILABLE", "caller-guard-unavailable", "本次代理调用进程无法安全核对，未关闭沙箱或切为直连。")
			}
			networkGuard = guard
			return profile.Network.BindBrowser(guard.allow)
		}
	}
	var p *pipeProcess
	if protected == nil {
		p, err = startPipeWithBinding(executable, args, profile.SessionID, bindJob)
	} else {
		resource := NetworkResourceIntent{ResourceID: "job", Kind: "job", ObjectIdentity: protected.intent.JobName, Locator: protected.intent.JobName}
		err = protected.store.journal.ApplyResource(ctx, profile.SessionID, resource, func(context.Context) error {
			var startErr error
			p, startErr = startPipeWithBinding(executable, args, profile.SessionID, bindJob)
			return startErr
		})
	}
	if p == nil {
		release()
		var networkProblem *proxy.CheckError
		var nativeProblem *Problem
		if errors.As(err, &networkProblem) || errors.As(err, &nativeProblem) {
			return nil, err
		}
		return nil, problem("PROCESS_START_FAILED", "native-start-failed", "所选真实内核未能启动，未关闭沙箱或尝试其他内核。")
	}
	identityErr := err
	process := newManagedProcess(p, profile.Network, release, pipeManagedControl(p))
	go process.observeExit()
	if profile.Network != nil {
		go func() {
			select {
			case <-profile.Network.Failed():
				_ = process.closeOwned(false)
			case <-protected.bridge.Done():
				_ = process.closeOwned(false)
			case <-process.done:
			}
		}()
	}
	stopStartup := context.AfterFunc(ctx, func() { _ = process.closeOwned(false) })
	defer func() {
		stopStartup()
		if resultErr != nil {
			resultErr = errors.Join(resultErr, process.closeOwned(false))
		}
	}()
	if err = lock.record(profile.EnvironmentID, profile.SessionID, p.pid, p.createdAt); err != nil {
		return process, err
	}
	if profile.OnCreated != nil {
		if err = profile.OnCreated(int(p.pid), p.createdAt); err != nil {
			return process, err
		}
	}
	if identityErr != nil {
		return process, identityErr
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
	if protected != nil {
		if err = verifyNetworkTree(p, nil); err != nil {
			return process, problem("NETWORK_PROTECTION_UNAVAILABLE", "tree-identity-unconfirmed", "浏览器进程树与本次代理会话未通过核对，已停止本环境；请保留诊断并重试。")
		}
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
		if err := ctx.Err(); err != nil {
			return process, err
		}
		return process, problem("PROCESS_START_FAILED", "startup-cleanup-already-began", "本次启动清理已经开始，未报告运行中。")
	}
	if !process.Alive() {
		return process, problem("PROCESS_START_FAILED", "exited-before-ready", "浏览器主进程在就绪前已退出，未报告运行中；子进程资源仍按本次Job保护。")
	}
	return process, nil
}
