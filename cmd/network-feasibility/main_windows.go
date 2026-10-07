//go:build windows

// A narrow opt-in experiment, not a network-protection provider. It never opens
// a normal workspace or clears the product's proxy launch gate.
package main

import (
	"archive/zip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"time"

	"github.com/axgiroud312-byte/prism-local-browser/internal/kernel"
	"github.com/google/uuid"
	"golang.org/x/sys/windows"
)

const archiveDigest = "9ef3f471b7a6641b4224532522b29141ce3746e27d55788d88e2fd951f362579"
const executableDigest = "1867319e56bcabbc4681d8575c002106ce7b61b5290dc5eb34a37676805f6915"

type helperObservation struct {
	AppContainer    uint32               `json:"appContainer"`
	CapabilityCount uint32               `json:"capabilityCount"`
	Integrity       uint32               `json:"integrity"`
	TCPConnected    bool                 `json:"tcpConnected"`
	TCPError        string               `json:"tcpError,omitempty"`
	DNSStatus       uint32               `json:"dnsStatus"`
	TokenError      bool                 `json:"tokenError"`
	Desktop         []desktopObservation `json:"desktop,omitempty"`
	Sockets         []socketObservation  `json:"sockets,omitempty"`
}
type caseObservation struct {
	Mode            string               `json:"mode"`
	Helper          *helperObservation   `json:"helper,omitempty"`
	DNSQueries      int                  `json:"dnsQueries"`
	DNSSender       string               `json:"dnsSender,omitempty"`
	TCPAccepts      int64                `json:"tcpAccepts"`
	BrowserVersion  string               `json:"browserVersion,omitempty"`
	RendererValue   json.RawMessage      `json:"rendererValue,omitempty"`
	BrowserError    string               `json:"browserError,omitempty"`
	BrowserExitCode *uint32              `json:"browserExitCode,omitempty"`
	HelperError     string               `json:"helperError,omitempty"`
	JobsExited      bool                 `json:"jobsExited"`
	Processes       []processObservation `json:"processes,omitempty"`
	Windows         *windowObservation   `json:"windows,omitempty"`
}
type labReport struct {
	Format            string                     `json:"format"`
	GeneratedAt       string                     `json:"generatedAt"`
	Presentation      string                     `json:"presentation"`
	ArchiveSHA256     string                     `json:"archiveSha256,omitempty"`
	ExecutableSHA256  string                     `json:"executableSha256,omitempty"`
	Version           string                     `json:"version,omitempty"`
	Cases             []caseObservation          `json:"cases"`
	Loopback          []loopbackObservation      `json:"loopback,omitempty"`
	BrowserDebug      []debugObservation         `json:"browserDebug,omitempty"`
	StationACLCleanup string                     `json:"stationAclCleanup,omitempty"`
	BrowserBridge     []browserBridgeObservation `json:"browserBridge,omitempty"`
	Cleanup           string                     `json:"cleanup"`
	Error             string                     `json:"error,omitempty"`
}

func main() {
	if len(os.Args) >= 2 && strings.HasPrefix(os.Args[1], "--bfe-") {
		if err := bfeExperimentCommand(os.Args[1:]); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		return
	}
	if os.Getenv("PRISM_NETWORK_FEASIBILITY") != "1" {
		fmt.Fprintln(os.Stderr, "Set PRISM_NETWORK_FEASIBILITY=1 for this synthetic experiment only.")
		os.Exit(2)
	}
	if len(os.Args) == 5 && os.Args[1] == "--helper" {
		helper(os.Args[2], os.Args[3], os.Args[4])
		return
	}
	if len(os.Args) == 8 && os.Args[1] == "--helper" {
		helper(os.Args[2], os.Args[3], os.Args[4], os.Args[5:]...)
		return
	}
	if len(os.Args) == 3 && os.Args[1] == "--tcp-only" {
		helper(os.Args[2], "", "")
		return
	}
	if len(os.Args) == 2 && os.Args[1] == "--token-holder" {
		_, _ = io.Copy(io.Discard, os.Stdin)
		return
	}
	if len(os.Args) == 3 && os.Args[1] == "--cleanup-container" {
		id := strings.TrimPrefix(os.Args[2], "prism-feasibility-")
		if id == os.Args[2] {
			os.Exit(2)
		}
		if _, err := uuid.Parse(id); err != nil {
			os.Exit(2)
		}
		name, _ := windows.UTF16PtrFromString(os.Args[2])
		if err := deleteSyntheticContainer(name); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		fmt.Println("synthetic-container-deleted")
		return
	}
	if len(os.Args) == 4 && os.Args[1] == "--bridge-owner" {
		if err := bridgeOwner(os.Args[2], os.Args[3]); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		return
	}
	archive := flag.String("archive", "", "fixed official 148 ZIP (only read)")
	out := flag.String("report", "", "new report JSON path outside disposable root")
	flag.Parse()
	if *archive == "" || *out == "" {
		fmt.Fprintln(os.Stderr, "--archive and --report are required")
		os.Exit(2)
	}
	r := run(*archive)
	data, _ := json.MarshalIndent(r, "", "  ")
	data = append(data, '\n')
	f, err := os.OpenFile(*out, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		fmt.Fprintln(os.Stderr, "Could not create new experiment report")
		os.Exit(1)
	}
	_, err = f.Write(data)
	err = errors.Join(err, f.Close())
	fmt.Printf("Experiment report saved. Cleanup: %s; cases: %d; browser bridge stages: %d.\n", r.Cleanup, len(r.Cases), len(r.BrowserBridge))
	if err != nil || r.Error != "" || r.Cleanup != "complete" {
		os.Exit(1)
	}
}
func helper(tcp, dns, name string, endpoints ...string) {
	r := helperObservation{}
	var err, error2 error
	r.AppContainer, err = tokenValue(windows.GetCurrentProcessToken(), 29)
	r.CapabilityCount, error2 = tokenValue(windows.GetCurrentProcessToken(), 30)
	r.TokenError = err != nil || error2 != nil
	r.Integrity, err = tokenIntegrity(windows.GetCurrentProcessToken())
	r.TokenError = r.TokenError || err != nil
	c, err := net.DialTimeout("tcp4", tcp, 2*time.Second)
	if err == nil {
		r.TCPConnected = true
		if dns == "" {
			_ = c.SetDeadline(time.Now().Add(2 * time.Second))
			_, e := c.Write([]byte("PRISM"))
			data := make([]byte, 5)
			if e == nil {
				_, e = io.ReadFull(c, data)
			}
			if e != nil || string(data) != "PRISM" {
				r.TCPConnected = false
				r.TCPError = "echo-failed"
			}
		}
		c.Close()
	} else {
		var e windows.Errno
		if errors.As(err, &e) {
			r.TCPError = fmt.Sprint(uint32(e))
		} else {
			r.TCPError = "failed"
		}
	}
	if dns != "" {
		r.DNSStatus = queryDNS(name, dns)
		r.Desktop = probeDesktop()
	}
	if len(endpoints) == 3 {
		for index, protocol := range []string{"tcp6", "udp4", "udp6"} {
			r.Sockets = append(r.Sockets, socketObservation{Protocol: protocol, Echo: socketEcho(protocol, endpoints[index], name)})
		}
	}
	_ = json.NewEncoder(os.Stdout).Encode(r)
}
func fileHash(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err = io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
func copyFile(source, destination string) error {
	in, err := os.Open(source)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(destination, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0700)
	if err != nil {
		return err
	}
	_, err = io.Copy(out, in)
	return errors.Join(err, out.Close())
}
func extract(archive, root string) (string, error) {
	z, err := zip.OpenReader(archive)
	if err != nil {
		return "", err
	}
	defer z.Close()
	executable := ""
	for _, entry := range z.File {
		rel := filepath.FromSlash(entry.Name)
		if !filepath.IsLocal(rel) || strings.Contains(rel, ":") || entry.Mode()&os.ModeSymlink != 0 {
			return "", fmt.Errorf("unsafe archive entry")
		}
		dest := filepath.Join(root, rel)
		if entry.FileInfo().IsDir() {
			if err = os.MkdirAll(dest, 0700); err != nil {
				return "", err
			}
			continue
		}
		if err = os.MkdirAll(filepath.Dir(dest), 0700); err != nil {
			return "", err
		}
		in, e := entry.Open()
		if e != nil {
			return "", e
		}
		out, e := os.OpenFile(dest, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
		if e != nil {
			in.Close()
			return "", e
		}
		_, e = io.Copy(out, in)
		e = errors.Join(e, out.Close(), in.Close())
		if e != nil {
			return "", e
		}
		if strings.EqualFold(filepath.Base(dest), "chrome.exe") {
			if executable != "" {
				return "", fmt.Errorf("multiple browser executables")
			}
			executable = dest
		}
	}
	if executable == "" {
		return "", fmt.Errorf("missing executable")
	}
	return executable, nil
}
func run(archive string) (r labReport) {
	r = labReport{Format: "prism-network-feasibility-v1", GeneratedAt: time.Now().UTC().Format(time.RFC3339Nano), Cases: []caseObservation{}, Cleanup: "not-created"}
	r.Presentation = "headless"
	if os.Getenv("PRISM_NETWORK_VISIBLE") == "1" {
		r.Presentation = "normal-window-start-minimized"
	}
	digest, err := fileHash(archive)
	if err != nil || digest != archiveDigest {
		r.Error = "fixed-archive-verification-failed"
		return
	}
	r.ArchiveSHA256 = digest
	base := filepath.Join("output", "goal", "T11")
	if err = os.MkdirAll(base, 0700); err != nil {
		r.Error = "output-root-failed"
		return
	}
	root, err := os.MkdirTemp(base, "feasibility-")
	if err != nil {
		r.Error = "test-root-failed"
		return
	}
	root, _ = filepath.Abs(root)
	allExited := true
	defer func() {
		if !allExited {
			r.Cleanup = "retained-unconfirmed-job"
			return
		}
		// A signaled/empty Job can briefly precede release of image section
		// references. Retry only cleanup of this newly created synthetic tree.
		var cleanupErr error
		for attempt := 0; attempt < 10; attempt++ {
			cleanupErr = os.RemoveAll(root)
			if cleanupErr == nil {
				break
			}
			if !errors.Is(cleanupErr, windows.ERROR_SHARING_VIOLATION) && !errors.Is(cleanupErr, windows.ERROR_ACCESS_DENIED) {
				break
			}
			time.Sleep(100 * time.Millisecond)
		}
		if cleanupErr != nil {
			r.Cleanup = "test-root-retained"
		} else if !strings.Contains(r.Cleanup, "retained") {
			r.Cleanup = "complete"
		}
	}()
	container, err := createContainer("prism-feasibility-" + uuid.NewString())
	if err != nil {
		r.Error = err.Error()
		return
	}
	defer func() {
		if !allExited {
			r.Cleanup = "retained-unconfirmed-job"
			return
		}
		if err := container.close(); err != nil {
			r.Cleanup = "container-profile-retained"
			r.Error = err.Error()
			_ = os.WriteFile(filepath.Join(base, "container-cleanup-private-"+uuid.NewString()+".txt"), []byte(windows.UTF16PtrToString(container.name)+"\n"+err.Error()), 0600)
		}
	}()
	exe, err := extract(archive, root)
	if err != nil {
		r.Error = "test-kernel-extraction-failed"
		return
	}
	r.ExecutableSHA256, err = fileHash(exe)
	if err != nil || r.ExecutableSHA256 != executableDigest {
		r.Error = "fixed-executable-hash-failed"
		return
	}
	r.Version, err = kernel.FileVersion(exe)
	if err != nil || r.Version != "148.0.7778.215" {
		r.Error = "fixed-executable-version-failed"
		return
	}
	self, err := os.Executable()
	if err != nil {
		r.Error = "helper-path-failed"
		return
	}
	helperPath := filepath.Join(root, "network-helper.exe")
	if copyFile(self, helperPath) != nil {
		r.Error = "helper-copy-failed"
		return
	}
	dns, err := newDNSObserver()
	if err != nil {
		r.Error = "dns-observer-failed"
		return
	}
	defer dns.close()
	observers, err := startSocketObservers()
	if err != nil {
		r.Error = "socket-observers-failed"
		return
	}
	defer func() {
		for _, observer := range observers {
			observer.close()
			<-observer.done
		}
	}()
	tcp, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		r.Error = "tcp-observer-failed"
		return
	}
	defer tcp.Close()
	var accepts atomic.Int64
	go func() {
		for {
			c, e := tcp.Accept()
			if e != nil {
				return
			}
			accepts.Add(1)
			c.Close()
		}
	}()
	for index, isolated := range []bool{false, false, true, true, true, true, true} {
		if os.Getenv("PRISM_NETWORK_FOCUS") == "station" && index != 1 && index != 6 {
			continue
		}
		mode := "unrestricted-control"
		if index == 1 {
			if err = allowSyntheticRoot(root, container.sid); err != nil {
				r.Error = "test-root-access-grant-failed: " + err.Error()
				return
			}
			mode = "unrestricted-container-acl"
		}
		var sid *windows.SID
		if isolated {
			mode = "appcontainer-zero-network"
			sid = container.sid
		}
		medium := index == 3
		if medium {
			mode = "appcontainer-zero-network-medium"
		}
		var primary windows.Token
		if index == 4 {
			mode = "native-lowbox-zero-network"
			primary, err = nativeLowBox(container.sid)
			if err != nil {
				r.Cases = append(r.Cases, caseObservation{Mode: mode, HelperError: err.Error(), JobsExited: true})
				continue
			}
			defer primary.Close()
			sid = nil // token already owns the package, do not create a second box
		}
		if index == 5 {
			mode = "appcontainer-precreated-window-station"
			closeStation, e := createSyntheticWindowStation(container.sid)
			if e != nil {
				r.Cases = append(r.Cases, caseObservation{Mode: mode, HelperError: e.Error(), JobsExited: true})
				continue
			}
			defer closeStation()
		}
		if index == 6 {
			mode = "appcontainer-temporary-window-station-acl"
			revoke, e := grantExistingSyntheticWindowStation(container.sid, filepath.Join(base, "station-acl-private-"+uuid.NewString()+".txt"))
			if e != nil {
				r.Cases = append(r.Cases, caseObservation{Mode: mode, HelperError: e.Error(), JobsExited: true})
				continue
			}
			r.StationACLCleanup = "pending"
			defer func() {
				if e := revoke(); e != nil {
					r.StationACLCleanup = "failed: " + e.Error()
					r.Error = "temporary-station-acl-cleanup-failed"
				} else {
					r.StationACLCleanup = "test-sid-revoked-and-verified"
				}
			}()
		}
		c := caseObservation{Mode: mode, JobsExited: true}
		name := "prism-" + uuid.NewString() + ".example.com"
		before := accepts.Load()
		p, e := launchWithToken(helperPath, []string{"--helper", tcp.Addr().String(), dns.conn.LocalAddr().String(), name, observers[0].address, observers[1].address, observers[2].address}, sid, false, nil, medium, primary)
		if e != nil {
			c.HelperError = "create-process: " + e.Error()
			if p != nil && p.stop() != nil {
				c.JobsExited = false
				allExited = false
			}
		} else {
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			raw, readErr := p.helperReport(ctx)
			cancel()
			if readErr != nil {
				c.HelperError = "helper-timeout-or-pipe-failure"
			} else {
				var observation helperObservation
				if json.Unmarshal(raw, &observation) != nil {
					c.HelperError = "helper-invalid-response"
				} else {
					c.Helper = &observation
				}
			}
			if p.stop() != nil {
				c.JobsExited = false
				allExited = false
			}
		}
		c.DNSQueries = dns.count(name)
		c.DNSSender = dns.owner(name)
		c.TCPAccepts = accepts.Load() - before
		if c.Helper != nil {
			for i := range c.Helper.Sockets {
				c.Helper.Sockets[i].Received = observers[i].count(name)
			}
		}
		if !allExited {
			r.Cases = append(r.Cases, c)
			r.Error = "helper-job-not-exited"
			return
		}
		profile := filepath.Join(root, mode+"-profile")
		if os.Mkdir(profile, 0700) != nil {
			r.Error = "synthetic-profile-failed"
			return
		}
		if isolated {
			if e = allowSyntheticProfile(profile, container.sid); e != nil {
				r.Error = "test-profile-access-grant-failed"
				return
			}
		}
		log, e := os.OpenFile(filepath.Join(root, mode+"-stderr.txt"), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
		if e != nil {
			r.Error = "test-log-failed"
			return
		}
		args := []string{"--headless=new", "--no-first-run", "--no-default-browser-check", "--disable-background-networking", "--disable-component-update", "--disable-sync", "--disable-crash-reporter", "--enable-logging=stderr", "--log-level=0", "--user-data-dir=" + profile, "--proxy-server=http://127.0.0.1:1", "--proxy-bypass-list=<-loopback>", "--fingerprint=1256789", "about:blank"}
		args = append(args, "--fingerprint-platform=windows", "--fingerprint-platform-version=15.0.0", "--fingerprint-brand=Chrome", "--fingerprint-brand-version=148.0.7778.215", "--disable-features=RestartNetworkServiceUnsandboxedForFailedLaunch")
		if r.Presentation == "normal-window-start-minimized" {
			filtered := []string{}
			for _, arg := range args {
				switch arg {
				case "--headless=new", "--disable-background-networking", "--disable-component-update", "--disable-sync":
					continue
				}
				filtered = append(filtered, arg)
			}
			args = append(filtered, "--start-minimized")
		}
		if index == 2 || (index == 6 && os.Getenv("PRISM_NETWORK_DEBUG") == "1") {
			r.BrowserDebug, allExited = debugContainerBrowser(exe, args, sid, log)
			if !allExited {
				log.Close()
				r.Error = "debugged-job-not-exited"
				return
			}
		}
		p, e = launchWithToken(exe, args, sid, true, log, medium, primary)
		if e != nil {
			c.BrowserError = "create-process: " + e.Error()
			if p != nil && p.stop() != nil {
				c.JobsExited = false
				allExited = false
			}
		} else {
			ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
			var browser struct {
				Product string `json:"product"`
			}
			e = p.call(ctx, "Browser.getVersion", map[string]any{}, "", &browser)
			if e != nil {
				c.BrowserError = "browser-pipe: " + e.Error()
			} else {
				c.BrowserVersion = browser.Product
				var target struct {
					ID string `json:"targetId"`
				}
				var session struct {
					ID string `json:"sessionId"`
				}
				var evaluation struct {
					Result struct {
						Value json.RawMessage `json:"value"`
					} `json:"result"`
				}
				e = p.call(ctx, "Target.createTarget", map[string]any{"url": "about:blank", "background": true}, "", &target)
				if e == nil {
					e = p.call(ctx, "Target.attachToTarget", map[string]any{"targetId": target.ID, "flatten": true}, "", &session)
				}
				if e == nil {
					e = p.call(ctx, "Runtime.evaluate", map[string]any{"expression": "1+1", "returnByValue": true}, session.ID, &evaluation)
				}
				if e != nil {
					c.BrowserError = "renderer: " + e.Error()
				} else {
					c.RendererValue = evaluation.Result.Value
					c.Processes = ownedProcessSnapshot(p, container.sid)
					if r.Presentation == "normal-window-start-minimized" {
						value := browserWindows(p.pid)
						c.Windows = &value
					}
					_ = p.call(ctx, "Browser.close", map[string]any{}, "", nil)
					_, _ = p.wait(ctx)
				}
			}
			cancel()
			var exit uint32
			if windows.GetExitCodeProcess(p.process, &exit) == nil && exit != 259 {
				c.BrowserExitCode = &exit
			}
			if p.stop() != nil {
				c.JobsExited = false
				allExited = false
			}
		}
		log.Close()
		if !allExited {
			r.Cases = append(r.Cases, c)
			r.Error = "browser-job-not-exited"
			return
		}
		if index == 6 && c.BrowserError == "" && c.RendererValue != nil {
			var trialExited bool
			r.BrowserBridge, trialExited = browserBridgeTrial(exe, helperPath, root, args, container.sid)
			allExited = allExited && trialExited
			if !allExited {
				r.Error = "browser-bridge-job-exit-unconfirmed"
			}
			for _, stage := range r.BrowserBridge {
				if strings.HasSuffix(stage.Stage, "-cleanup") && stage.Error != "" {
					r.Cleanup = "additional-resources-retained"
					r.Error = "additional-resource-cleanup-failed"
				}
			}
		}
		if c.BrowserError != "" {
			// Ignored local evidence only; raw Chromium logs are never included
			// in the public report because they can include absolute paths.
			_ = copyFile(log.Name(), filepath.Join(base, "stderr-private-"+uuid.NewString()+"-"+mode+".txt"))
			_ = copyFile(filepath.Join(profile, "chrome-lab.log"), filepath.Join(base, "chromelog-private-"+uuid.NewString()+"-"+mode+".txt"))
		}
		// Logs can contain synthetic absolute paths. Preserve only a small safe
		// numeric/code summary in the report; the temporary tree is cleaned.
		r.Cases = append(r.Cases, c)
		if !allExited {
			r.Error = "browser-job-not-exited"
			return
		}
	}
	if os.Getenv("PRISM_NETWORK_FOCUS") != "station" {
		r.Loopback, allExited = samePackageLoopback(helperPath, container.sid)
		if !allExited {
			r.Error = "loopback-job-not-exited"
		}
	}
	return
}
