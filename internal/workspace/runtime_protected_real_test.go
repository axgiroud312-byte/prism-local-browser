//go:build windows

package workspace

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
	"reflect"
	"testing"
	"time"

	"github.com/axgiroud312-byte/prism-local-browser/internal/kernel"
	"github.com/axgiroud312-byte/prism-local-browser/internal/proxy"
)

// Same application calls as the native UI, without automated UI input. The
// kernel fixture installs and verifies real bytes once; this test reuses them.
func TestRealProtectedRuntimeApplicationReopen(t *testing.T) {
	root := os.Getenv("PRISM_PROTECTED_TEST_ROOT")
	if root == "" {
		t.Skip("controlled real application fixture not selected")
	}
	root, _ = filepath.Abs(root)
	marker, err := os.ReadFile(filepath.Join(root, "synthetic-network-fixture"))
	if err != nil || string(marker) != "prism-owned-synthetic-v1" {
		t.Fatal("not an owned fixture")
	}
	encoded, err := os.ReadFile(filepath.Join(root, "verified-kernel.json"))
	var record kernel.Record
	if err != nil || json.Unmarshal(encoded, &record) != nil || kernel.CheckRecord(record) != nil {
		t.Fatal("run the real kernel fixture first")
	}
	directory, err := kernel.RecordDirectory(root, record)
	if err != nil {
		t.Fatal(err)
	}
	if err = kernel.VerifyFiles(directory, record.Files); err != nil {
		t.Fatal(err)
	}
	observer := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, `{"ip":"203.0.113.9"}`) }))
	defer observer.Close()
	pool := x509.NewCertPool()
	pool.AddCert(observer.Certificate())
	observerURL, _ := url.Parse(observer.URL)
	reports := make(chan map[string]string, 8)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "CONNECT" {
			if r.Host != observerURL.Host {
				http.Error(w, "uncontrolled destination", 403)
				return
			}
			remote, err := net.DialTimeout("tcp4", observerURL.Host, 5*time.Second)
			if err != nil {
				http.Error(w, "observer unavailable", 502)
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
		if r.URL.Hostname() != "prism-application.test" {
			http.Error(w, "uncontrolled target", 403)
			return
		}
		if r.Method == "POST" && r.URL.Path == "/report" {
			var report map[string]string
			if json.NewDecoder(io.LimitReader(r.Body, 4096)).Decode(&report) != nil {
				http.Error(w, "bad report", 400)
				return
			}
			select {
			case reports <- report:
			default:
			}
			w.WriteHeader(204)
			return
		}
		w.Header().Set("Content-Type", "text/html")
		w.Header().Set("Cache-Control", "no-store")
		fmt.Fprint(w, `<!doctype html><title>Prism synthetic application launch</title><script>(async()=>{const before={cookie:document.cookie,local:localStorage.getItem('prism')||''};document.cookie='prism=synthetic; Max-Age=3600; Path=/';localStorage.setItem('prism','synthetic');const db=await new Promise((resolve,reject)=>{const r=indexedDB.open('prism',1);r.onupgradeneeded=()=>r.result.createObjectStore('state');r.onsuccess=()=>resolve(r.result);r.onerror=reject});await new Promise((resolve,reject)=>{const tx=db.transaction('state','readwrite'),s=tx.objectStore('state'),get=s.get('marker');get.onsuccess=()=>{before.indexed=get.result||'';s.put('synthetic','marker')};tx.oncomplete=resolve;tx.onerror=reject});db.close();await fetch('/report',{method:'POST',body:JSON.stringify(before)})})();</script>`)
	}))
	defer upstream.Close()
	options := Options{ProtectedProxyCheck: proxy.CheckOptions{TargetURL: observer.URL, RootCAs: pool}}
	s, err := Open(root, options)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := s.Close(); err != nil {
			t.Error(err)
		}
	}()
	// Seed only the already verified installation, not a fake launcher or bridge.
	tx, err := s.db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	if _, err = tx.Exec("INSERT OR IGNORE INTO kernels(id,version,source,status) VALUES(?,?,?,'verified')", record.ID, record.Version, "fingerprint-chromium"); err != nil {
		tx.Rollback()
		t.Fatal(err)
	}
	if _, err = tx.Exec("INSERT OR IGNORE INTO kernel_evidence(kernel_id,record_json) VALUES(?,?)", record.ID, string(encoded)); err != nil {
		tx.Rollback()
		t.Fatal(err)
	}
	if err = tx.Commit(); err != nil {
		t.Fatal(err)
	}
	bound := importProxyFixture(t, s, upstream.URL)
	p := generateFingerprint(t, s, preview(t, s, "create", ""), record.ID, false)
	p.Environment.Name, p.Environment.URLs, p.Environment.ProxyID = "Synthetic protected application "+id(), "http://prism-application.test/", bound.ID
	p.Environment.RestoreTabs = false
	value[any](t, call(s, "Environment.Create", Mutation{PreviewID: p.PreviewID, Configuration: p.Environment.Configuration, Count: 1, ProfileHash: p.Fingerprint.PreviewProfile.ConfigHash, RequestID: id()}))
	var environment Environment
	for _, e := range view(t, s).State.Environments {
		if e.Name == p.Environment.Name {
			environment = e
		}
	}
	if environment.ID == "" {
		t.Fatal("saved environment missing")
	}
	original := view(t, s).Fingerprints[environment.ID].Profile
	ref := view(t, s).DataReferences[environment.ID]
	evidence := []map[string]any{}
	for round := 0; round < 2; round++ {
		op := acceptRuntimeTest(t, s, "Runtime.Start", runtimeRequest{EnvironmentID: environment.ID, RequestID: id(), NetworkPolicy: "proxy"})
		if done := waitRuntimeReal(t, s, op.ID); done.State != "completed" {
			t.Fatalf("application start: %+v", done.Error)
		}
		session := view(t, s).RuntimeSessions[environment.ID]
		if session.State != "running" || session.PID < 1 || session.ProxyReport == nil || session.ProxyReport.Error != nil {
			t.Fatal("application published invalid readiness")
		}
		var report map[string]string
		select {
		case report = <-reports:
		case <-time.After(30 * time.Second):
			t.Fatal("real page did not report through upstream")
		}
		if round == 1 && (report["cookie"] != "prism=synthetic" || report["local"] != "synthetic" || report["indexed"] != "synthetic") {
			t.Fatalf("reopen data mismatch: %+v", report)
		}
		stop := acceptRuntimeTest(t, s, "Runtime.Stop", runtimeRequest{EnvironmentID: environment.ID, RequestID: id()})
		if done := waitRuntimeReal(t, s, stop.ID); done.State != "completed" {
			t.Fatalf("application stop: %+v", done.Error)
		}
		if view(t, s).RuntimeSessions[environment.ID].ResourcesPending {
			t.Fatal("normal close retained resources")
		}
		evidence = append(evidence, map[string]any{"round": round, "applicationStart": "completed", "applicationStop": "completed", "cookieReadBeforeWrite": report["cookie"], "localStorageReadBeforeWrite": report["local"], "indexedDBReadBeforeWrite": report["indexed"]})
		if err = s.Close(); err != nil {
			t.Fatal(err)
		}
		s, err = Open(root, options)
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(original, view(t, s).Fingerprints[environment.ID].Profile) || ref != view(t, s).DataReferences[environment.ID] {
			t.Fatal("application reopen changed saved identity or data reference")
		}
	}
	// Cookie-purpose launches share the actual provider and FIFO queue, suppress
	// saved URLs/tabs only for this launch, and preserve A/B data isolation.
	other := createRuntimeEnvironment(t, s, record.ID, "Synthetic protected cookie B "+id())
	bindRuntimeProxyFixture(t, s, other, bound)
	previewB := cookiePreviewFixture(t, s, other.ID, `[{"name":"unused","value":"SYNTHETIC_UNUSED","domain":"example.test"}]`)
	previewA := cookiePreviewFixture(t, s, environment.ID, fmt.Sprintf(`[{"name":"empty","value":"","domain":"example.test","httpOnly":true,"session":true},{"name":"persistent","value":"SYNTHETIC_IMPORT","domain":"example.test","secure":true,"expires":%d}]`, time.Now().Unix()+3600))
	starts := []Operation{}
	for _, p := range []CookiePreview{previewA, previewB} {
		starts = append(starts, acceptRuntimeTest(t, s, "Runtime.Start", runtimeRequest{EnvironmentID: p.EnvironmentID, RequestID: id(), NetworkPolicy: "proxy", Purpose: "cookie-import", ExpectedRevision: p.ExpectedRevision}))
	}
	for _, start := range starts {
		if done := waitRuntimeReal(t, s, start.ID); done.State != "completed" {
			t.Fatal("protected queued Cookie start", done.Error)
		}
	}
	select {
	case <-reports:
		t.Fatal("Cookie-purpose start restored saved page URLs")
	default:
	}
	request := cookieCommitFixture(t, s, previewA, []int{1, 2}, "merge")
	imported := acceptCookieFixture(t, s, request)
	final := waitRuntimeReal(t, s, imported.ID)
	if final.State != "completed" || final.CookieReport == nil || final.CookieReport.VerifiedCount != 2 {
		t.Fatal("protected actual Cookie import", final.Error)
	}
	if repeated := acceptCookieFixture(t, s, request); repeated.ID != imported.ID {
		t.Fatal("Cookie request replay produced a second import")
	}
	s.mu.Lock()
	aTransport := s.runtimeSlots[environment.ID].process.(cookieTransport)
	bTransport := s.runtimeSlots[other.ID].process.(cookieTransport)
	s.mu.Unlock()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	aCookies, err := aTransport.ReadCookies(ctx)
	if err != nil {
		t.Fatal(err)
	}
	bCookies, err := bTransport.ReadCookies(ctx)
	if err != nil {
		t.Fatal(err)
	}
	verified := 0
	for _, c := range aCookies {
		if c.Name == "empty" && c.Value == "" && c.HTTPOnly && c.Session {
			verified++
		}
		if c.Name == "persistent" && c.Value == "SYNTHETIC_IMPORT" && c.Secure && !c.Session {
			verified++
		}
	}
	if verified != 2 || len(bCookies) != 0 {
		t.Fatal("actual Cookie semantics or A/B separation failed")
	}
	for _, env := range []string{environment.ID, other.ID} {
		op := acceptRuntimeTest(t, s, "Runtime.Stop", runtimeRequest{EnvironmentID: env, RequestID: id()})
		if done := waitRuntimeReal(t, s, op.ID); done.State != "completed" {
			t.Fatal(done.Error)
		}
	}
	evidence = append(evidence, map[string]any{"protectedQueuedCookieStarts": 2, "savedURLsSuppressed": true, "emptySessionHttpOnlyAndPersistentSecureVerified": true, "cookieImportRequestDeduplicated": true, "otherEnvironmentCookieCount": len(bCookies)})
	if output := os.Getenv("PRISM_PROTECTED_APP_EVIDENCE"); output != "" {
		bytes, _ := json.MarshalIndent(map[string]any{"observedAt": timestamp(), "scope": "native application Runtime.Start/Stop and Service reopen with real production provider; controlled loopback upstream/HTTPS observer; no UI clicks or external egress claim", "kernelVersion": record.Version, "executableSha256": record.ExecutableSHA256, "savedIdentityAndDataReferenceUnchanged": true, "rounds": evidence}, "", "  ")
		if err = os.WriteFile(output, append(bytes, '\n'), 0600); err != nil {
			t.Fatal(err)
		}
	}
}
