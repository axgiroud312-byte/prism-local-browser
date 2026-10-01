package workspace

import (
	"context"
	"net/netip"

	"github.com/axgiroud312-byte/prism-local-browser/internal/kernel"
	"github.com/axgiroud312-byte/prism-local-browser/internal/proxy"
)

func (s *Service) prepareRuntimeNetwork(ctx context.Context, slot *runtimeSlot, input RuntimeLaunch) (RuntimeProxyChannel, error) {
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
			return proxy.OpenBridge(config, credentials, options)
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
	channel RuntimeProxyChannel
	done    chan struct{}
}

func ownRuntimeNetwork(process RuntimeProcess, channel RuntimeProxyChannel) RuntimeProcess {
	owned := &networkRuntimeProcess{RuntimeProcess: process, channel: channel, done: make(chan struct{})}
	go func() { <-process.Done(); _ = channel.Close(); close(owned.done) }()
	return owned
}
func (p *networkRuntimeProcess) Done() <-chan struct{} { return p.done }
func (p *networkRuntimeProcess) Snapshot() kernel.RuntimeSnapshot {
	snapshot := p.RuntimeProcess.Snapshot()
	snapshot.ProxyError = p.channel.Fault()
	select {
	case <-p.done:
	default:
		snapshot.ResourcesExited = false
	}
	return snapshot
}
func (p *networkRuntimeProcess) Close() error {
	_ = p.channel.Close()
	if err := p.RuntimeProcess.Close(); err != nil {
		return err
	}
	<-p.done
	return nil
}
func (p *networkRuntimeProcess) Stop(ctx context.Context) error {
	if err := p.RuntimeProcess.Stop(ctx); err != nil {
		return err
	}
	select {
	case <-p.done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
