//go:build windows

package kernel

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"strings"
	"time"
	"unsafe"

	"github.com/axgiroud312-byte/prism-local-browser/internal/desktopbase"
	"github.com/google/uuid"
	"golang.org/x/sys/windows"
)

func FileVersion(executable string) (string, error) {
	size, err := windows.GetFileVersionInfoSize(executable, nil)
	if err != nil || size == 0 {
		return "", fmt.Errorf("missing PE version")
	}
	bytes := make([]byte, size)
	if err = windows.GetFileVersionInfo(executable, 0, size, unsafe.Pointer(&bytes[0])); err != nil {
		return "", err
	}
	var fixed *windows.VS_FIXEDFILEINFO
	var length uint32
	if err = windows.VerQueryValue(unsafe.Pointer(&bytes[0]), `\`, unsafe.Pointer(&fixed), &length); err != nil || length < uint32(unsafe.Sizeof(windows.VS_FIXEDFILEINFO{})) || fixed == nil || fixed.Signature != 0xFEEF04BD {
		return "", fmt.Errorf("invalid PE version")
	}
	return fmt.Sprintf("%d.%d.%d.%d", fixed.FileVersionMS>>16, fixed.FileVersionMS&0xffff, fixed.FileVersionLS>>16, fixed.FileVersionLS&0xffff), nil
}

const probeScript = `(async()=>{
 const headers=await (await fetch('capture',{cache:'no-store'})).json();
 const hints=await navigator.userAgentData.getHighEntropyValues(['fullVersionList','platformVersion']);
 const gl=document.createElement('canvas').getContext('webgl'); const debug=gl?.getExtension('WEBGL_debug_renderer_info');
 return {httpUserAgent:headers['user-agent']||'',httpClientHints:headers,userAgent:navigator.userAgent,platform:navigator.userAgentData.platform,brands:navigator.userAgentData.brands,fullVersionList:hints.fullVersionList,platformVersion:hints.platformVersion,language:navigator.language,languages:navigator.languages,acceptLanguage:headers['accept-language']||'',timezone:Intl.DateTimeFormat().resolvedOptions().timeZone,cpu:navigator.hardwareConcurrency,memory:navigator.deviceMemory,gpuVendor:debug?gl.getParameter(debug.UNMASKED_VENDOR_WEBGL):'',gpuRenderer:debug?gl.getParameter(debug.UNMASKED_RENDERER_WEBGL):''};
})()`

func Probe(ctx context.Context, executable, version, staging string) (result Report, resultErr error) {
	ctx, cancel := context.WithTimeout(ctx, 90*time.Second)
	defer cancel()
	defer func() {
		if resultErr != nil && ctx.Err() != nil {
			resultErr = preserveProbeFailure(resultErr, ctx.Err())
		}
	}()
	url, closePage, err := newProbePage()
	if err != nil {
		return Report{}, err
	}
	defer closePage()
	report := Report{AdapterVersion: AdapterVersion, Version: CapabilityVersion, SampledAt: time.Now().UTC().Format(time.RFC3339Nano), Transport: "inherited-private-pipe", Sandbox: true, Observations: []Observation{}, Capabilities: []Capability{}}
	for index, seed := range []int{1256789, 1256790, 1256789} {
		language, timezone := "en-US,en", "America/New_York"
		if index == 2 {
			language, timezone = "de-DE,en", "Europe/Berlin"
		}
		observation, err := probeOne(ctx, executable, version, staging, url, seed, language, timezone, index == 2)
		if err != nil {
			return Report{}, err
		}
		if err = checkIdentity(observation, version); err != nil {
			return Report{}, err
		}
		if observation.Language != strings.Split(language, ",")[0] || !strings.HasPrefix(observation.AcceptLanguage, observation.Language) || observation.Timezone != timezone || (index == 2 && observation.CPU != 8) {
			return Report{}, problem("KERNEL_INTEGRITY_FAILED", "parameter-mismatch", "语言、时区或CPU参数未在网页/请求中生效，未验收该构建。")
		}
		report.Observations = append(report.Observations, observation)
	}
	if report.Observations[0].CPU == report.Observations[1].CPU {
		return Report{}, problem("KERNEL_INTEGRITY_FAILED", "seed-entry-unverified", "不同种子的CPU诊断输出未显示指纹入口生效，未发布该构建。")
	}
	report.Capabilities = probeCapabilities(version)
	return report, nil
}

func newProbePage() (string, func(), error) {
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		return "", nil, problem("PROCESS_START_FAILED", "probe-server-unavailable", "本机诊断页面无法建立，未验收内核。")
	}
	token := uuid.NewString()
	server := &http.Server{ReadHeaderTimeout: 5 * time.Second, Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/"+token+"/" && r.URL.Path != "/"+token+"/capture" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Accept-CH", "Sec-CH-UA-Full-Version-List, Sec-CH-UA-Platform-Version")
		if strings.HasSuffix(r.URL.Path, "/capture") {
			w.Header().Set("Content-Type", "application/json")
			headers := map[string]string{}
			for _, name := range []string{"user-agent", "accept-language", "sec-ch-ua", "sec-ch-ua-platform", "sec-ch-ua-full-version-list", "sec-ch-ua-platform-version"} {
				headers[name] = r.Header.Get(name)
			}
			json.NewEncoder(w).Encode(headers)
		} else {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			fmt.Fprint(w, "<!doctype html><title>Prism isolated kernel diagnostic</title>")
		}
	})}
	go server.Serve(listener)
	url := "http://" + listener.Addr().String() + "/" + token + "/"
	return url, func() { server.Close() }, nil
}

func probeCapabilities(version string) []Capability {
	capabilities := []Capability{
		{"identity", "configurable", "observed", "Windows/Chrome品牌与实际PE/CDP版本、HTTP UA、网页UA及UA-CH已核对；允许UA reduction/GREASE。"},
		{"seed", "seed-generated", "observed", "两份固定测试种子使用隔离目录，自动CPU输出不同；只证明该诊断条件下入口生效。"},
		{"cpu", "configurable", "observed", "显式8核心参数在独立诊断网页回读为8；自动值由固定内核和seed决定。"},
		{"acceptLanguages", "configurable", "observed", "en-US和de-DE测试下网页语言及HTTP Accept-Language核对。"},
		{"timezone", "configurable", "observed", "New_York/Berlin测试下IANA时区回读；跟随代理未实现。"},
		{"uiLanguage", "unverified", "not-probed", "无头诊断不证明浏览器菜单界面语言生效。"},
		{"memory", "unverified", "observed", "记录诊断deviceMemory；单次读值不证明seed机制，没有任意内存编辑入口。"},
		{"gpu", "unverified", "observed", "记录诊断WebGL元数据；单次读值不证明所有GPU字段的seed机制，不开放手填vendor/renderer。"},
		{"font/canvas/audio/clientrects", "unverified", "not-probed", "本票未执行这些输出/兼容开关验收，不标成可编辑或已验证。"},
		{"screen/location/webgpu/tls/mac", "unverified", "not-probed", "未核验或不支持，不下发伪有效字段。"},
		{"proxy/webrtc", "unverified", "not-probed", "本票诊断未验证代理或网络泄漏保护，留后续票。"},
	}
	if version == "148.0.7778.215" {
		capabilities[6] = Capability{"memory", "seed-generated", "source-derived", "148的021内存补丁按seed从8/16/32生成；具体观测列在本次诊断记录中，不开放任意内存设置。"}
		capabilities[7] = Capability{"gpu", "seed-generated", "source-derived", "148的011 GPU补丁按seed选配置池；具体WebGL读值另列，不推导跨机器一致性，不开放手填GPU。"}
	}
	return capabilities
}

func preserveProbeFailure(failure, contextError error) error {
	// Cancellation does not erase a previously observed unsafe identity. Worker
	// classification must still discover it with errors.As, before cleanup/status.
	return errors.Join(failure, contextError)
}

func probeOne(ctx context.Context, executable, version, staging, url string, seed int, language, timezone string, explicitCPU bool) (_ Observation, resultErr error) {
	args := []string{"--fingerprint=" + fmt.Sprint(seed), "--fingerprint-platform=windows", "--fingerprint-platform-version=15.0.0", "--fingerprint-brand=Chrome", "--fingerprint-brand-version=" + version, "--lang=" + strings.Split(language, ",")[0], "--accept-lang=" + language, "--timezone=" + timezone}
	if explicitCPU {
		args = append(args, "--fingerprint-hardware-concurrency=8")
	}
	return probeOneArguments(ctx, executable, staging, url, seed, args)
}

func probeOneArguments(ctx context.Context, executable, staging, url string, seed int, fingerprintArguments []string) (_ Observation, resultErr error) {
	directory, err := os.MkdirTemp(staging, "probe-")
	if err != nil {
		return Observation{}, err
	}
	defer func() {
		if cleanupErr := RemoveOwnedTree(directory); cleanupErr != nil {
			resultErr = errors.Join(resultErr, problem("STORAGE_WRITE_FAILED", "probe-cleanup-failed", "本次诊断资源无法清理；内核未发布，旧数据未修改。"))
		}
	}()
	releaseDirectory, err := desktopbase.PinDirectories(directory)
	if err != nil {
		return Observation{}, problem("PATH_OUTSIDE_ROOT", "reparse-point", "本次诊断目录无法安全固定，未启动内核。")
	}
	defer releaseDirectory()
	args := []string{"--headless=new", "--no-first-run", "--no-default-browser-check", "--disable-background-networking", "--disable-component-update", "--disable-sync", "--disable-crash-reporter", "--user-data-dir=" + directory, "--proxy-server=http://127.0.0.1:1", "--proxy-bypass-list=127.0.0.1"}
	args = append(args, fingerprintArguments...)
	args = append(args, "about:blank")
	p, err := startPipe(executable, args)
	if err != nil {
		return Observation{}, problem("PROCESS_START_FAILED", "diagnostic-start-failed", "隔离诊断进程无法启动；未关闭沙箱或尝试其他内核。")
	}
	defer p.close()
	stop := context.AfterFunc(ctx, p.close)
	defer stop()
	var browser struct {
		Product string `json:"product"`
	}
	if err = p.call(ctx, "Browser.getVersion", map[string]any{}, "", &browser); err != nil {
		return Observation{}, problem("PROCESS_READY_TIMEOUT", "diagnostic-not-ready", "内核未在时限内响应私有诊断pipe，已清理本次进程树。")
	}
	var target struct {
		ID string `json:"targetId"`
	}
	if err = p.call(ctx, "Target.createTarget", map[string]any{"url": "about:blank"}, "", &target); err != nil {
		return Observation{}, err
	}
	var session struct {
		ID string `json:"sessionId"`
	}
	if err = p.call(ctx, "Target.attachToTarget", map[string]any{"targetId": target.ID, "flatten": true}, "", &session); err != nil {
		return Observation{}, err
	}
	if err = p.call(ctx, "Page.navigate", map[string]any{"url": url}, session.ID, nil); err != nil {
		return Observation{}, err
	}
	// Wait for this exact local page without an arbitrary fixed startup sleep.
	var observation Observation
	for {
		var evaluation struct {
			Result struct {
				Value json.RawMessage `json:"value"`
			} `json:"result"`
			Exception json.RawMessage `json:"exceptionDetails"`
		}
		err = p.call(ctx, "Runtime.evaluate", map[string]any{"expression": "location.href===" + quoted(url) + " ? " + probeScript + " : null", "returnByValue": true, "awaitPromise": true}, session.ID, &evaluation)
		if err != nil {
			return Observation{}, err
		}
		if len(evaluation.Exception) > 0 {
			return Observation{}, problem("KERNEL_INTEGRITY_FAILED", "probe-evaluation-failed", "诊断网页无法读取UA-CH或指纹值，未验收该构建。")
		}
		if len(evaluation.Result.Value) > 0 && string(evaluation.Result.Value) != "null" {
			if err = json.Unmarshal(evaluation.Result.Value, &observation); err != nil {
				return Observation{}, err
			}
			break
		}
		select {
		case <-ctx.Done():
			return Observation{}, ctx.Err()
		case <-time.After(50 * time.Millisecond):
		}
	}
	observation.Seed = seed
	observation.BrowserVersion = strings.TrimPrefix(strings.TrimPrefix(browser.Product, "Chrome/"), "HeadlessChrome/")
	observation.PID = p.pid
	observation.ProcessCreatedAt = p.createdAt
	observation.NormalExit = p.normalClose()
	if !observation.NormalExit {
		return Observation{}, problem("PROCESS_START_FAILED", "probe-close-failed", "诊断浏览器未正常退出，已清理本次进程树，未发布内核。")
	}
	return observation, nil
}
func quoted(value string) string { bytes, _ := json.Marshal(value); return string(bytes) }
