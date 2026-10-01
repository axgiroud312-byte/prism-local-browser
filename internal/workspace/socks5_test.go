package workspace

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/axgiroud312-byte/prism-local-browser/internal/proxy"
)

func TestNativeSOCKS5ImportReplaceClearAndProtocolChangesPreserveSecrets(t *testing.T) {
	s, _ := fixture(t, Options{})
	record := importProxyFixture(t, s, "socks5://synthetic%3Auser:SYNTHETIC_SOCKS_STORED@localhost:1080")
	_, ref, err := s.savedProxy(record.ID)
	if err != nil {
		t.Fatal(err)
	}
	credentials, err := s.storedProxyCredentials(ref)
	if err != nil || credentials.Username != "synthetic:user" {
		t.Fatal("protocol-neutral protected envelope rejected SOCKS5 username")
	}
	marker := "SYNTHETIC_SOCKS_STORED"
	input := ProxyUpdate{ProxyID: record.ID, ExpectedRevision: record.Revision, RequestID: id(), Configuration: record.Configuration, Credentials: ProxyCredentialChange{Action: "replace", Username: "user", Password: strings.Repeat("a", 256)}}
	wantError(t, call(s, "Proxy.Update", input), "PROXY_AUTH_INVALID")
	if view(t, s).NativeProxyRecords[0].Revision != record.Revision {
		t.Fatal("invalid SOCKS5 authentication changed durable revision")
	}
	input.Configuration.Type, input.Credentials, input.RequestID = "http", ProxyCredentialChange{Action: "keep"}, id()
	updated := value[struct {
		Record ProxyView `json:"record"`
	}](t, call(s, "Proxy.Update", input)).Record
	_, nextRef, err := s.savedProxy(record.ID)
	if err != nil || nextRef != ref || !updated.HasAuthentication {
		t.Fatal("protocol switch silently cleared/replaced saved authentication")
	}
	credentials, err = s.storedProxyCredentials(nextRef)
	if err != nil || credentials.Password != marker {
		t.Fatal("protocol keep did not retain original protected value")
	}
	input.Configuration, input.ExpectedRevision, input.RequestID, input.Credentials = updated.Configuration, updated.Revision, id(), ProxyCredentialChange{Action: "clear"}
	cleared := value[struct {
		Record ProxyView `json:"record"`
	}](t, call(s, "Proxy.Update", input)).Record
	if cleared.HasAuthentication {
		t.Fatal("explicit clear left encrypted authentication referenced")
	}
	encoded, _ := json.Marshal(view(t, s))
	if strings.Contains(string(encoded), marker) || strings.Contains(string(encoded), "synthetic:user") {
		t.Fatal("protocol change leaked authentication to ordinary view")
	}
}

func TestNativeProtocolKeepNeverDecryptsAndIncompatibleAuthUsesSameCheckStartError(t *testing.T) {
	channel := &syntheticRuntimeChannel{}
	s, _, kernelID := fingerprintFixture(t, Options{OpenProxyChannel: func(config proxy.Configuration, credentials *proxy.Credentials, opts proxy.BridgeOptions) (RuntimeProxyChannel, error) {
		if credentials != nil {
			if err := proxy.ValidateProtocolCredentials(config.Type, *credentials); err != nil {
				return nil, err
			}
		}
		channel.channelID = opts.ChannelID
		return channel, nil
	}})
	record := importProxyFixture(t, s, "socks5://synthetic%3Auser:synthetic-pass@localhost:1080")
	config := record.Configuration
	config.Type = "http"
	decode := s.options.UnprotectProxySecret
	// Mutation keep must not inspect the envelope. The cached request-HMAC
	// key was already loaded by the fixture/import, so this hook is secret-only.
	s.options.UnprotectProxySecret = func(string, []byte) ([]byte, error) {
		t.Error("keep decoded existing proxy secret")
		return nil, errors.New("synthetic decoder unavailable")
	}
	updated := value[struct {
		Record ProxyView `json:"record"`
	}](t, call(s, "Proxy.Update", ProxyUpdate{ProxyID: record.ID, ExpectedRevision: record.Revision, RequestID: id(), Configuration: config, Credentials: ProxyCredentialChange{Action: "keep"}})).Record
	s.options.UnprotectProxySecret = decode
	check := acceptProxyCheck(t, s, updated)
	checked := waitKernel(t, s, check.ID)
	if checked.Error == nil || checked.Error.Code != "PROXY_AUTH_INVALID" || checked.Error.Retryable {
		t.Fatal("protocol-invalid kept credentials were misreported as bridge outage")
	}
	environment := createRuntimeEnvironment(t, s, kernelID, "合成切协议认证拒绝")
	bindRuntimeProxyFixture(t, s, environment, updated)
	start := acceptRuntimeTest(t, s, "Runtime.Start", runtimeRequest{EnvironmentID: environment.ID, RequestID: id(), NetworkPolicy: "proxy"})
	started := waitKernel(t, s, start.ID)
	if started.Error == nil || started.Error.Code != checked.Error.Code || started.Error.Retryable || view(t, s).RuntimeSessions[environment.ID].PID != 0 {
		t.Fatal("runtime misclassified incompatible saved auth or launched a browser")
	}
}

func TestNativeSOCKS5StartupUsesSameChannelAndRetainsSeedHistoryAndDataReference(t *testing.T) {
	for _, authenticated := range []bool{false, true} {
		channel := &syntheticRuntimeChannel{}
		process := newSyntheticRuntimeProcess()
		var starts atomic.Int32
		s, root, kernelID := fingerprintFixture(t, Options{OpenProxyChannel: func(config proxy.Configuration, credentials *proxy.Credentials, opts proxy.BridgeOptions) (RuntimeProxyChannel, error) {
			if config.Type != "socks5" || (credentials != nil) != authenticated {
				t.Error("SOCKS5 protocol or explicit auth strategy changed")
			}
			channel.channelID = opts.ChannelID
			return channel, nil
		}, LaunchRuntime: func(_ context.Context, input RuntimeLaunch) (RuntimeProcess, error) {
			starts.Add(1)
			if input.Network != channel || input.Proxy.Type != "socks5" || channel.checks.Load() != 1 {
				t.Error("browser launcher did not reuse checked SOCKS5 bridge")
			}
			return process, nil
		}})
		text := "socks5://localhost:1080"
		if authenticated {
			text = "socks5://synthetic-user:SYNTHETIC_RUNTIME_SOCKS@localhost:1080"
		}
		record := importProxyFixture(t, s, text)
		environment := createRuntimeEnvironment(t, s, kernelID, "合成SOCKS5身份保留")
		before := view(t, s)
		bindRuntimeProxyFixture(t, s, environment, record)
		operation := acceptRuntimeTest(t, s, "Runtime.Start", runtimeRequest{EnvironmentID: environment.ID, RequestID: id(), NetworkPolicy: "proxy"})
		if waitKernel(t, s, operation.ID).State != "completed" || starts.Load() != 1 {
			t.Fatal("SOCKS5 start was rejected or repeated")
		}
		stopped := acceptRuntimeTest(t, s, "Runtime.Stop", runtimeRequest{EnvironmentID: environment.ID, RequestID: id()})
		if waitKernel(t, s, stopped.ID).State != "completed" || channel.closes.Load() == 0 {
			t.Fatal("SOCKS5 channel survived stop")
		}
		after := view(t, s)
		if after.State.Environments[0].Seed != before.State.Environments[0].Seed || after.Fingerprints[environment.ID].Profile.ConfigHash != before.Fingerprints[environment.ID].Profile.ConfigHash || after.DataReferences[environment.ID] != before.DataReferences[environment.ID] {
			t.Fatal("SOCKS5 bind/start/stop changed original identity or data reference")
		}
		if err := s.Close(); err != nil {
			t.Fatal(err)
		}
		reopened, err := Open(root, Options{})
		if err != nil {
			t.Fatal(err)
		}
		defer reopened.Close()
		if saved := view(t, reopened); saved.RuntimeSessions[environment.ID].ProxyChannelID != channel.ID() || saved.State.Environments[0].Seed != before.State.Environments[0].Seed {
			t.Fatal("reopen lost channel history or original seed")
		}
	}
}

func TestNativeSOCKS5FailuresBlockLaunchWithPreciseSharedContract(t *testing.T) {
	for _, code := range []string{"PROXY_AUTH_FAILED", "PROXY_SOCKS_NEGOTIATION_FAILED", "PROXY_TARGET_UNSUPPORTED", "PROXY_TARGET_UNREACHABLE", "PROXY_UNREACHABLE"} {
		channel := &syntheticRuntimeChannel{result: &proxy.CheckError{Code: code, Message: "合成SOCKS5前检失败。", Retryable: true}}
		var launches atomic.Int32
		s, _, kernelID := fingerprintFixture(t, Options{OpenProxyChannel: func(_ proxy.Configuration, _ *proxy.Credentials, opts proxy.BridgeOptions) (RuntimeProxyChannel, error) {
			channel.channelID = opts.ChannelID
			return channel, nil
		}, LaunchRuntime: func(context.Context, RuntimeLaunch) (RuntimeProcess, error) {
			launches.Add(1)
			return newSyntheticRuntimeProcess(), nil
		}})
		record := importProxyFixture(t, s, "socks5://localhost:1080")
		environment := createRuntimeEnvironment(t, s, kernelID, "合成SOCKS5错误"+code)
		bindRuntimeProxyFixture(t, s, environment, record)
		operation := acceptRuntimeTest(t, s, "Runtime.Start", runtimeRequest{EnvironmentID: environment.ID, RequestID: id(), NetworkPolicy: "proxy"})
		final := waitKernel(t, s, operation.ID)
		if final.Error == nil || final.Error.Code != code || final.ProxyReport.Error.Code != code || launches.Load() != 0 || channel.closes.Load() == 0 || view(t, s).RuntimeSessions[environment.ID].PID != 0 {
			t.Fatal("SOCKS5 failure became another error, direct browser launch, or retained bridge")
		}
	}
}

func TestNativeSOCKS5IndependentCheckReportPersistsSafeRemoteDNSPolicy(t *testing.T) {
	s, root := fixture(t, Options{CheckProxy: func(_ context.Context, config proxy.Configuration, _ *proxy.Credentials, progress func(proxy.Step)) proxy.Report {
		if config.Type != "socks5" {
			t.Error("check changed protocol")
		}
		report := syntheticProxyCheck(context.Background(), config, nil, progress)
		report.ChannelID, report.ResolutionPolicy = id(), proxy.SOCKS5ResolutionPolicy
		return report
	}})
	record := importProxyFixture(t, s, "socks5://localhost:1080")
	operation := acceptProxyCheck(t, s, record)
	final := waitKernel(t, s, operation.ID)
	if final.State != "completed" || final.ProxyReport.ResolutionPolicy != proxy.SOCKS5ResolutionPolicy {
		t.Fatal("SOCKS5 remote DNS policy missing from check operation")
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := Open(root, Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	saved := view(t, reopened).NativeProxyRecords[0]
	if saved.CheckReport.ChannelID != final.ProxyReport.ChannelID || saved.CheckReport.ResolutionPolicy != proxy.SOCKS5ResolutionPolicy {
		t.Fatal("persisted check lost exact temporary channel or policy")
	}
}

func TestNativeProxyRPCRejectsSOCKS5ResolverAndAuthenticationOverrides(t *testing.T) {
	s, _, kernelID := fingerprintFixture(t, Options{LaunchRuntime: func(context.Context, RuntimeLaunch) (RuntimeProcess, error) { return newSyntheticRuntimeProcess(), nil }})
	record := importProxyFixture(t, s, "socks5://localhost:1080")
	environment := createRuntimeEnvironment(t, s, kernelID, "合成DNS入口边界")
	bindRuntimeProxyFixture(t, s, environment, record)
	wantError(t, call(s, "Proxy.Check", map[string]any{"proxyId": record.ID, "expectedRevision": record.Revision, "requestId": id(), "resolveLocally": true, "targetUrl": "https://synthetic.invalid"}), "VALIDATION_FAILED")
	wantError(t, call(s, "Runtime.Start", map[string]any{"environmentId": environment.ID, "requestId": id(), "networkPolicy": "proxy", "resolutionPolicy": "local", "socksAuthFallback": true}), "VALIDATION_FAILED")
}
