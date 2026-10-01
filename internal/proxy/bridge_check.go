package proxy

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"net/url"
	"strconv"
	"sync"
	"time"
)

// Preflight uses the SAME listening instance and authenticated forwarding
// path that is later bound to the browser's exact Job. A fresh host-only token
// and TCP caller guard admit the probe; loopback alone is never authorization.
func (b *Bridge) Preflight(ctx context.Context, progress func(Step)) Report {
	secret := make([]byte, 32)
	if _, err := rand.Read(secret); err != nil {
		return Report{Mode: "native", AdapterVersion: BridgeVersion, ChannelID: b.ID(), StartedAt: time.Now().UTC().Format(time.RFC3339Nano), FinishedAt: time.Now().UTC().Format(time.RFC3339Nano), Steps: []Step{}, Error: &CheckError{Code: "PROXY_BRIDGE_UNAVAILABLE", Message: "无法生成本次安全前检标识，未发起请求。", Retryable: true}}
	}
	token := base64.RawURLEncoding.EncodeToString(secret)
	Wipe(secret)
	authorization := "Basic " + base64.StdEncoding.EncodeToString([]byte("prism-probe:"+token))
	var mu sync.Mutex
	steps := []Step{}
	finished := false
	collect := func(step Step) {
		mu.Lock()
		defer mu.Unlock()
		if finished {
			return
		}
		if step.Stage == "validation" {
			step.Message = "本次桥接入口已校验；上游配置已在建桥时核验。"
		}
		if step.Stage == "connection" {
			step.Stage = "local-channel"
			step.Message = "本次受保护前检通过该环境的独立本机桥接入口。"
		}
		if step.Stage == "authentication" {
			step.Stage = "local-authorization"
			step.Message = "本机通道接受当前应用的受保护前检；上游认证另行记录。"
		}
		steps = append(steps, step)
		if progress != nil {
			progress(step)
		}
	}
	probe := &bridgeProbe{progress: collect}
	b.mu.Lock()
	if b.closing {
		b.mu.Unlock()
		return Report{Mode: "native", AdapterVersion: BridgeVersion, ChannelID: b.ID(), StartedAt: time.Now().UTC().Format(time.RFC3339Nano), FinishedAt: time.Now().UTC().Format(time.RFC3339Nano), Steps: []Step{}, Error: &CheckError{Code: "PROXY_BRIDGE_UNAVAILABLE", Message: "本次桥接已关闭，未重新建立或改为直连。", Retryable: true}}
	}
	b.probes[authorization] = probe
	b.mu.Unlock()
	u, _ := url.Parse(b.Endpoint())
	port, _ := strconv.Atoi(u.Port())
	report := Check(ctx, Configuration{Name: "本次独立桥接", Type: "http", Host: u.Hostname(), Port: port}, &Credentials{Username: "prism-probe", Password: token}, CheckOptions{TargetURL: b.opts.TargetURL, RootCAs: b.opts.RootCAs}, collect)
	b.mu.Lock()
	observed := probe.err
	delete(b.probes, authorization)
	b.mu.Unlock()
	mu.Lock()
	finished = true
	report.Steps = append([]Step(nil), steps...)
	mu.Unlock()
	report.ChannelID, report.AdapterVersion = b.ID(), BridgeVersion
	if observed != nil && (report.Error == nil || report.Error.Code != "OPERATION_CANCELLED" && report.Error.Code != "PROXY_CHECK_TIMEOUT") {
		copy := *observed
		report.Error = &copy
	}
	return report
}
