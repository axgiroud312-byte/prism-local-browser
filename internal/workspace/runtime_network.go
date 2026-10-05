package workspace

import (
	"context"
	"errors"
	"net/netip"
	"sync"
	"time"

	"github.com/axgiroud312-byte/prism-local-browser/internal/cookies"
	"github.com/axgiroud312-byte/prism-local-browser/internal/kernel"
	"github.com/axgiroud312-byte/prism-local-browser/internal/proxy"
)

func (s *Service) prepareRuntimeNetwork(ctx context.Context, slot *runtimeSlot, input RuntimeLaunch) (RuntimeProxyChannel, error) {
	if err := s.runtimeStage(slot, "network-protection"); err != nil {
		return nil, err
	}
	// LaunchRuntime is a trusted host-only synthetic seam. It cannot make the
	// real kernel launcher bypass its independent protection gate, and desktop
	// production never injects it. RPC has no unsafe/protection-ready override.
	if s.options.LaunchRuntime == nil {
		if err := kernel.RequireProxyNetworkBoundary(); err != nil {
			return nil, err
		}
	}
	if err := s.runtimeStage(slot, "proxy-preflight"); err != nil {
		return nil, err
	}
	s.mu.Lock()
	protected, err := s.readProtectedProxyCredentials(input.ProxyCredentialRef)
	channelID := slot.session.ProxyChannelID
	s.mu.Unlock()
	var credentials *proxy.Credentials
	if err == nil {
		credentials, err = s.decodeProtectedProxyCredentials(input.ProxyCredentialRef, protected)
	}
	if credentials != nil {
		defer func() { credentials.Username, credentials.Password = "", "" }()
	}
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	if err != nil {
		return nil, &proxy.CheckError{Code: "CREDENTIALS_UNAVAILABLE", Message: "当前Windows用户不能读取绑定代理认证，未无认证重试或直连启动。", Retryable: true}
	}
	s.mu.Lock()
	current := s.runtimeSlots[input.EnvironmentID] == slot && slot.session.State == "starting" && !s.closed && !s.closeRequested.Load()
	s.mu.Unlock()
	if !current {
		return nil, context.Canceled
	}
	factory := s.options.OpenProxyChannel
	if factory == nil {
		factory = func(config proxy.Configuration, credentials *proxy.Credentials, options proxy.BridgeOptions) (RuntimeProxyChannel, error) {
			bridge, err := proxy.OpenBridge(config, credentials, options)
			if bridge == nil {
				return nil, err
			}
			return bridge, err
		}
	}
	channel, err := factory(input.Proxy.Configuration, credentials, proxy.BridgeOptions{ChannelID: channelID, AuthorizeProbe: kernel.AuthorizeProxyProbe})
	if credentials != nil {
		credentials.Username, credentials.Password = "", ""
	}
	if err != nil {
		return channel, err
	}
	if channel == nil || channel.ID() != channelID {
		return channel, &proxy.CheckError{Code: "PROXY_BRIDGE_UNAVAILABLE", Message: "实际代理通道与本次会话身份不匹配，未启动。", Retryable: true}
	}
	progress := func(step proxy.Step) {
		s.mu.Lock()
		defer s.mu.Unlock()
		if s.runtimeSlots[input.EnvironmentID] != slot || slot.session.State != "starting" || slot.session.PersistencePending {
			return
		}
		report := proxy.Report{Mode: "native", AdapterVersion: proxy.BridgeVersion, ChannelID: channelID, ProxyID: input.Proxy.ID, Revision: input.Proxy.Revision, StartedAt: step.Time, Steps: []proxy.Step{}}
		if slot.session.ProxyReport != nil {
			report = *slot.session.ProxyReport
		}
		report.Steps = append(append([]proxy.Step(nil), report.Steps...), step)
		slot.start.ProxyReport, slot.session.ProxyReport, slot.start.Stage = &report, &report, "proxy-preflight/"+step.Stage
	}
	report := channel.Preflight(ctx, progress)
	if report.Mode != "native" || report.ChannelID != channelID {
		return channel, &proxy.CheckError{Code: "CAPABILITY_UNSUPPORTED", Message: "前检不是本次原生代理通道的结果，未启动或使用演示结果。", Retryable: false}
	}
	report.ProxyID, report.Revision = input.Proxy.ID, input.Proxy.Revision
	if report.Error == nil {
		ip, err := netip.ParseAddr(report.ExitIP)
		if err != nil || !ip.IsGlobalUnicast() || ip.Zone() != "" || report.FinishedAt == "" {
			report.Error = &proxy.CheckError{Code: "PROXY_EXIT_INVALID", Message: "本次同通道前检没有有效出口观测，未启动。", Retryable: true}
		}
	}
	s.mu.Lock()
	if s.runtimeSlots[input.EnvironmentID] != slot || slot.session.State != "starting" || s.closed || s.closeRequested.Load() {
		s.mu.Unlock()
		return channel, context.Canceled
	}
	slot.session.ProxyReport, slot.start.ProxyReport, slot.start.Stage = &report, &report, "proxy-ready"
	if report.Error != nil {
		slot.session.Error = &Error{Code: report.Error.Code, Message: report.Error.Message, Retryable: report.Error.Retryable}
	}
	err = s.persistRuntime(slot, &slot.start, "代理通道前检")
	s.mu.Unlock()
	if err != nil {
		return channel, err
	}
	if report.Error != nil {
		return channel, report.Error
	}
	if ctx.Err() != nil {
		return channel, ctx.Err()
	}
	if fault := channel.Fault(); fault != nil {
		return channel, fault
	}
	return channel, nil
}

// Even a host-only launcher seam must retain the same channel until the exact
// browser tree exits. Done means BOTH the Job and channel resources are gone.
type networkRuntimeProcess struct {
	RuntimeProcess
	channel       RuntimeProxyChannel
	done          chan struct{}
	mu            sync.Mutex
	cleanupFailed bool
	processExited bool
	channelClosed bool
	doneClosed    bool
	attempt       *networkCloseAttempt
}

type networkCloseAttempt struct {
	done     chan struct{}
	err      error
	finished bool // protected by the owner's mu
}

func (p *networkRuntimeProcess) ReadCookies(ctx context.Context) ([]cookies.Stored, error) {
	if transport, ok := p.RuntimeProcess.(cookieTransport); ok {
		return transport.ReadCookies(ctx)
	}
	return nil, &cookies.Error{Code: "COOKIE_CONTROL_UNAVAILABLE", Message: "本次会话未提供Cookie控制能力。", Retryable: false}
}
func (p *networkRuntimeProcess) ApplyCookie(ctx context.Context, value cookies.Cookie) (cookies.ApplyResult, error) {
	if transport, ok := p.RuntimeProcess.(cookieTransport); ok {
		return transport.ApplyCookie(ctx, value)
	}
	return cookies.ApplyResult{}, &cookies.Error{Code: "COOKIE_CONTROL_UNAVAILABLE", Message: "本次会话未提供Cookie控制能力。", Retryable: false}
}
func (p *networkRuntimeProcess) ClearCookies(ctx context.Context) error {
	if transport, ok := p.RuntimeProcess.(cookieTransport); ok {
		return transport.ClearCookies(ctx)
	}
	return &cookies.Error{Code: "COOKIE_CONTROL_UNAVAILABLE", Message: "本次会话未提供Cookie控制能力。", Retryable: false}
}

func ownRuntimeNetwork(process RuntimeProcess, channel RuntimeProxyChannel) RuntimeProcess {
	if native, ok := process.(*kernel.ManagedProcess); ok && native.ManagesNetwork() {
		return process
	}
	owned := &networkRuntimeProcess{RuntimeProcess: process, channel: channel, done: make(chan struct{})}
	go func() {
		if process != nil {
			<-process.Done()
		}
		owned.markProcessExited()
		owned.beginChannelClose(false)
	}()
	go func() {
		select {
		case <-channel.Failed():
			if owned.Close() != nil {
				owned.recordCleanupFailure()
			}
		case <-owned.done:
		}
	}()
	return owned
}
func (p *networkRuntimeProcess) PID() int {
	if p.RuntimeProcess == nil {
		return 0
	}
	return p.RuntimeProcess.PID()
}
func (p *networkRuntimeProcess) CreatedAt() string {
	if p.RuntimeProcess == nil {
		return ""
	}
	return p.RuntimeProcess.CreatedAt()
}
func (p *networkRuntimeProcess) Alive() bool {
	return p.RuntimeProcess != nil && p.RuntimeProcess.Alive()
}

// Only this concrete owner can assert no browser was ever created. A missing
// PID on an arbitrary process implementation is not that evidence.
func runtimeHasBrowser(process RuntimeProcess) bool {
	if process == nil {
		return false
	}
	owned, ok := process.(*networkRuntimeProcess)
	return !ok || owned.RuntimeProcess != nil
}

func networkCleanupUnconfirmed() *proxy.CheckError {
	return &proxy.CheckError{Code: "PROXY_BRIDGE_UNAVAILABLE", Message: "本次代理通道资源清理尚未确认，环境占用保持；可重试关闭，没有重建通道或改为直连。", Retryable: true}
}

func (p *networkRuntimeProcess) completeLocked() {
	if p.processExited && p.channelClosed && !p.doneClosed {
		p.cleanupFailed = false
		p.doneClosed = true
		close(p.done)
	}
}

func (p *networkRuntimeProcess) recordCleanupFailure() {
	p.mu.Lock()
	defer p.mu.Unlock()
	if !p.doneClosed {
		p.cleanupFailed = true
	}
}

func (p *networkRuntimeProcess) markProcessExited() {
	p.mu.Lock()
	p.processExited = true
	p.completeLocked()
	p.mu.Unlock()
}

// Failed attempts remain observable and retryable. Do not put a fallible
// cleanup behind sync.Once, and do not close Done just because the root/Job
// exited. Concurrent Stop/ForceStop/exit observation share an in-flight close.
func (p *networkRuntimeProcess) beginChannelClose(retry bool) *networkCloseAttempt {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.attempt != nil && (!retry || p.channelClosed || !p.attempt.finished) {
		return p.attempt
	}
	attempt := &networkCloseAttempt{done: make(chan struct{})}
	p.attempt = attempt
	go func() {
		err := p.channel.Close()
		p.mu.Lock()
		defer p.mu.Unlock()
		attempt.err, attempt.finished = err, true
		if err == nil {
			p.channelClosed = true
		} else {
			p.cleanupFailed = true
		}
		p.completeLocked()
		close(attempt.done)
	}()
	return attempt
}

func (p *networkRuntimeProcess) waitProcessExit(ctx context.Context) error {
	if p.RuntimeProcess != nil {
		select {
		case <-p.RuntimeProcess.Done():
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	p.markProcessExited()
	return nil
}

func (p *networkRuntimeProcess) waitChannelClose(ctx context.Context, attempt *networkCloseAttempt) error {
	select {
	case <-attempt.done:
		if attempt.err != nil {
			return networkCleanupUnconfirmed()
		}
		return nil
	case <-ctx.Done():
		return errors.Join(networkCleanupUnconfirmed(), ctx.Err())
	}
}

func (p *networkRuntimeProcess) Done() <-chan struct{} { return p.done }
func (p *networkRuntimeProcess) Snapshot() kernel.RuntimeSnapshot {
	p.mu.Lock()
	done, cleanupFailed := p.doneClosed, p.cleanupFailed
	p.mu.Unlock()
	snapshot := kernel.RuntimeSnapshot{}
	if p.RuntimeProcess != nil {
		snapshot = p.RuntimeProcess.Snapshot()
	}
	if fault := p.channel.Fault(); fault != nil {
		snapshot.ProxyError = fault
	}
	snapshot.NetworkCleanupFailed = snapshot.NetworkCleanupFailed || cleanupFailed
	snapshot.ResourcesExited = done
	return snapshot
}
func (p *networkRuntimeProcess) Close() error {
	select {
	case <-p.done:
		return nil
	default:
	}
	// Start socket cleanup independently: a failure there must not prevent the
	// browser's exact Job from being stopped, nor can Job exit hide that failure.
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	attempt := p.beginChannelClose(true)
	var processErr error
	if p.RuntimeProcess != nil {
		processErr = p.RuntimeProcess.Close()
	}
	if processErr == nil {
		processErr = p.waitProcessExit(ctx)
	}
	channelErr := p.waitChannelClose(ctx, attempt)
	select {
	case <-p.done:
		return nil
	default:
	}
	if err := errors.Join(processErr, channelErr); err != nil {
		p.recordCleanupFailure()
		return err
	}
	return networkCleanupUnconfirmed()
}
func (p *networkRuntimeProcess) Stop(ctx context.Context) error {
	select {
	case <-p.done:
		return nil
	default:
	}
	if p.RuntimeProcess != nil {
		if err := p.RuntimeProcess.Stop(ctx); err != nil {
			return err
		}
	}
	if err := p.waitProcessExit(ctx); err != nil {
		return err
	}
	return p.waitChannelClose(ctx, p.beginChannelClose(true))
}
