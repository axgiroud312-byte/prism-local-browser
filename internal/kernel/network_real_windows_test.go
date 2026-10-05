//go:build windows

package kernel

import (
	"context"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"sync/atomic"
	"testing"
	"time"

	"github.com/axgiroud312-byte/prism-local-browser/internal/proxy"
	"github.com/google/uuid"
	"golang.org/x/sys/windows"
)

// Explicit opt-in. Uses the production provider/launcher, a real pinned kernel,
// and controlled local upstream/HTTPS observer. No mouse/keyboard automation or
// user profile is used. This is not an independent external-egress observation.
func TestRealProtectedProxyLaunchAndReopen(t *testing.T) {
	root, archive := os.Getenv("PRISM_PROTECTED_TEST_ROOT"), os.Getenv("PRISM_KERNEL_ARCHIVE")
	if root == "" || archive == "" {
		t.Skip("production isolation fixture not explicitly selected")
	}
	root, _ = filepath.Abs(root)
	marker := filepath.Join(root, "synthetic-network-fixture")
	if _, err := os.Stat(root); os.IsNotExist(err) {
		if err = os.MkdirAll(root, 0700); err != nil {
			t.Fatal(err)
		}
		if err = os.WriteFile(marker, []byte("prism-owned-synthetic-v1"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	if bytes, err := os.ReadFile(marker); err != nil || string(bytes) != "prism-owned-synthetic-v1" {
		t.Fatal("refusing non-fixture root")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	var record Record
	recordPath := filepath.Join(root, "verified-kernel.json")
	if bytes, err := os.ReadFile(recordPath); err == nil {
		if json.Unmarshal(bytes, &record) != nil || CheckRecord(record) != nil {
			t.Fatal("invalid saved fixture kernel")
		}
	} else if os.IsNotExist(err) {
		prepared, err := Prepare(ctx, root, InstallInput{Source: "local", Version: "148.0.7778.215", ExpectedChecksum: "9ef3f471b7a6641b4224532522b29141ce3746e27d55788d88e2fd951f362579", ArchiveToken: "explicit-controlled-fixture", Trusted: true, RequestID: uuid.NewString()}, archive, nil, func(stage string) { t.Log(stage) })
		if err != nil {
			t.Fatal(err)
		}
		destination, _ := RecordDirectory(root, prepared.Record)
		if err = os.MkdirAll(filepath.Dir(destination), 0700); err != nil {
			t.Fatal(err)
		}
		if err = os.Rename(prepared.Directory, destination); err != nil {
			t.Fatal(err)
		}
		record = prepared.Record
		bytes, _ := json.Marshal(record)
		if err = os.WriteFile(recordPath, bytes, 0600); err != nil {
			t.Fatal(err)
		}
		if err = os.RemoveAll(prepared.Staging); err != nil {
			t.Fatal(err)
		}
	} else {
		t.Fatal(err)
	}
	directory, _ := RecordDirectory(root, record)
	if err := VerifyFiles(directory, record.Files); err != nil {
		t.Fatal(err)
	}
	observer := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"ip":"203.0.113.7"}`)
	}))
	defer observer.Close()
	pool := x509.NewCertPool()
	pool.AddCert(observer.Certificate())
	observerURL, _ := url.Parse(observer.URL)
	var pageRequests atomic.Int64
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "CONNECT" {
			if r.Host != observerURL.Host {
				http.Error(w, "fixture destination refused", 403)
				return
			}
			remote, err := net.DialTimeout("tcp4", observerURL.Host, 5*time.Second)
			if err != nil {
				http.Error(w, "fixture observer unavailable", 502)
				return
			}
			client, buffered, err := w.(http.Hijacker).Hijack()
			if err != nil {
				remote.Close()
				return
			}
			fmt.Fprint(buffered, "HTTP/1.1 200 Connection Established\r\n\r\n")
			buffered.Flush()
			go func() { defer client.Close(); defer remote.Close(); io.Copy(remote, buffered) }()
			io.Copy(client, remote)
			client.Close()
			remote.Close()
			return
		}
		if r.URL.Hostname() != "prism-protected.test" {
			http.Error(w, "fixture target refused", 403)
			return
		}
		pageRequests.Add(1)
		w.Header().Set("Content-Type", "text/html")
		w.Header().Set("Cache-Control", "no-store")
		fmt.Fprint(w, `<!doctype html><title>Prism synthetic protected session</title><script>(async()=>{window.before={cookie:document.cookie,local:localStorage.getItem('prism')};document.cookie='prism=synthetic; Max-Age=3600; Path=/';localStorage.setItem('prism','synthetic');const db=await new Promise((resolve,reject)=>{const r=indexedDB.open('prism',1);r.onupgradeneeded=()=>r.result.createObjectStore('state');r.onerror=()=>reject(r.error);r.onsuccess=()=>resolve(r.result)});await new Promise((resolve,reject)=>{const tx=db.transaction('state','readwrite');const store=tx.objectStore('state');const get=store.get('marker');get.onsuccess=()=>{window.before.indexed=get.result||'';store.put('synthetic','marker')};tx.oncomplete=resolve;tx.onerror=reject});db.close();window.ready=true;})();</script>`)
	}))
	defer upstream.Close()
	u, _ := url.Parse(upstream.URL)
	port, _ := strconv.Atoi(u.Port())
	store, err := OpenNetworkStore(root)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	pending, err := store.Pending(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, intent := range pending {
		if err = store.Recover(ctx, intent); err != nil {
			t.Fatal("prior fixture cleanup", err)
		}
	}
	environment := uuid.NewString()
	profile := ManagedProfile{EnvironmentID: environment, UserDataRef: "environments/" + environment + "/user-data", Fingerprint: FingerprintInput{Seed: "1256789", Language: "en-US", Timezone: "America/New_York", CPU: "8"}, Width: 1000, Height: 700}
	evidence := []map[string]any{}
	canary, err := NewMigrationProbe()
	if err != nil {
		t.Fatal(err)
	}
	defer canary.Close()
	for round := 0; round < 2; round++ {
		profile.SessionID = uuid.NewString()
		channel, err := store.openProtectedProxy(ctx, record, profile, uuid.NewString(), proxy.Configuration{Name: "Synthetic controlled upstream", Type: "http", Host: u.Hostname(), Port: port}, nil, proxy.BridgeOptions{TargetURL: observer.URL, RootCAs: pool})
		if channel != nil {
			defer func() {
				if err := channel.Close(); err != nil {
					t.Error("retained fixture resources", err)
				}
			}()
		}
		if err != nil {
			t.Fatal("prepare protected session", err)
		}
		report := channel.Preflight(ctx, nil)
		if report.Error != nil {
			t.Fatal("same-channel preflight", report.Error)
		}
		profile.Network = channel
		process, err := LaunchManagedProfile(ctx, root, record, profile)
		if process != nil {
			defer func() {
				if err := process.Close(); err != nil {
					t.Error("fixture process cleanup", err)
				}
			}()
		}
		if err != nil {
			t.Fatal("production launch", err)
		}
		if err = verifyNetworkTree(process.pipe, channel.sid); err != nil {
			t.Fatal("actual tree", err)
		}
		if observed, sampleErr := canary.Sample(ctx, process, record, profile.Fingerprint, round == 0); sampleErr != nil {
			t.Fatalf("protected canary: %v; cookie=%v local=%v indexed=%v language=%s accept=%s timezone=%s cpu=%d", sampleErr, observed.Cookie, observed.LocalStorage, observed.IndexedDB, observed.Fingerprint.Language, observed.Fingerprint.AcceptLanguage, observed.Fingerprint.Timezone, observed.Fingerprint.CPU)
		}
		var target struct {
			ID string `json:"targetId"`
		}
		if err = process.pipe.call(ctx, "Target.createTarget", map[string]any{"url": "http://prism-protected.test/"}, "", &target); err != nil {
			t.Fatal(err)
		}
		var attached struct {
			ID string `json:"sessionId"`
		}
		if err = process.pipe.call(ctx, "Target.attachToTarget", map[string]any{"targetId": target.ID, "flatten": true}, "", &attached); err != nil {
			t.Fatal(err)
		}
		var actual struct {
			Result struct {
				Value struct {
					Ready   bool   `json:"ready"`
					Cookie  string `json:"cookie"`
					Local   string `json:"local"`
					Indexed string `json:"indexed"`
					CPU     int    `json:"cpu"`
				} `json:"value"`
			} `json:"result"`
		}
		deadline := time.Now().Add(20 * time.Second)
		for time.Now().Before(deadline) {
			err = process.pipe.call(ctx, "Runtime.evaluate", map[string]any{"expression": "({ready:window.ready===true,cookie:window.before?.cookie||'',local:window.before?.local||'',indexed:window.before?.indexed||'',cpu:navigator.hardwareConcurrency})", "returnByValue": true}, attached.ID, &actual)
			if err != nil {
				t.Fatal(err)
			}
			if actual.Result.Value.Ready {
				break
			}
			time.Sleep(50 * time.Millisecond)
		}
		value := actual.Result.Value
		if !value.Ready || value.CPU != 8 || (round == 1 && (value.Cookie != "prism=synthetic" || value.Local != "synthetic" || value.Indexed != "synthetic")) {
			t.Fatalf("storage/identity round %d mismatch: %+v", round, value)
		}
		if err = process.Stop(ctx); err != nil {
			t.Fatal("normal close", err)
		}
		select {
		case <-process.Done():
		case <-ctx.Done():
			t.Fatal("resources not fully released")
		}
		if pending, err = store.Pending(ctx); err != nil || len(pending) != 0 {
			t.Fatal("journal not closed", err)
		}
		snapshot := process.Snapshot()
		if !snapshot.ExitKnown || snapshot.ExitCode != 0 {
			t.Fatal("browser did not exit normally", snapshot.ExitCode)
		}
		exists, err := networkContainerExists(channel.intent.PackageSID)
		if err != nil || exists {
			t.Fatal("container cleanup unconfirmed", err)
		}
		t.Logf("round=%d normal-close=true same-seed=1256789 actual-cpu=%d proxy-observed-pages=%d journal-pending=0", round, value.CPU, pageRequests.Load())
		evidence = append(evidence, map[string]any{"round": round, "normalExitCode": snapshot.ExitCode, "cpu": value.CPU, "cookieReadBeforeWrite": value.Cookie, "localStorageReadBeforeWrite": value.Local, "indexedDBReadBeforeWrite": value.Indexed, "pendingResources": len(pending), "containerAbsent": !exists, "allObservedJobMembersSamePackageZeroCapabilities": true})
	}
	if pageRequests.Load() < 2 {
		t.Fatal("controlled upstream did not observe browser requests")
	}
	if output := os.Getenv("PRISM_PROTECTED_EVIDENCE"); output != "" {
		data, _ := json.MarshalIndent(map[string]any{"observedAt": time.Now().UTC().Format(time.RFC3339Nano), "scope": "production kernel provider and launcher; controlled loopback upstream/HTTPS observer; no UI clicks or external egress claim", "windowsBuild": windows.RtlGetVersion().BuildNumber, "kernelVersion": record.Version, "executableSha256": record.ExecutableSHA256, "syntheticSeed": "1256789", "rounds": evidence, "proxyObservedPageRequests": pageRequests.Load()}, "", "  ")
		if err = os.WriteFile(output, append(data, '\n'), 0600); err != nil {
			t.Fatal(err)
		}
	}
}
