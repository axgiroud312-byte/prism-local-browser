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
	"sync/atomic"
	"testing"
	"time"

	"github.com/axgiroud312-byte/prism-local-browser/internal/cookies"
	"github.com/axgiroud312-byte/prism-local-browser/internal/kernel"
	"github.com/axgiroud312-byte/prism-local-browser/internal/proxy"
)

// These opt-in acceptance tests always use the real provider, private pipe and
// pinned Chromium. Only an owned synthetic workspace and local upstream are
// used; neither external egress nor native UI clicking is claimed.
type localAcceptanceFixture struct {
	s            *Service
	record       kernel.Record
	bound        ProxyView
	reports      chan syntheticBrowserReport
	changed      atomic.Bool
	readOnly     atomic.Bool
	destination  string
	source       string
	checkBarrier atomic.Pointer[localAcceptanceCheckBarrier]
}

type localAcceptanceCheckBarrier struct {
	entered chan struct{}
	release chan struct{}
}

func openLocalAcceptanceFixture(t *testing.T) *localAcceptanceFixture {
	t.Helper()
	root := os.Getenv("PRISM_LOCAL_ACCEPTANCE_ROOT")
	if root == "" {
		t.Skip("owned local real acceptance fixture not explicitly selected")
	}
	root, err := filepath.Abs(root)
	if err != nil {
		t.Fatal(err)
	}
	marker := filepath.Join(root, "synthetic-network-fixture")
	if _, err = os.Stat(root); os.IsNotExist(err) {
		if err = os.MkdirAll(root, 0700); err != nil {
			t.Fatal(err)
		}
		if err = os.WriteFile(marker, []byte("prism-owned-synthetic-v1"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	if bytes, err := os.ReadFile(marker); err != nil || string(bytes) != "prism-owned-synthetic-v1" {
		t.Fatal("refusing unowned acceptance workspace")
	}
	var record kernel.Record
	recordPath := filepath.Join(root, "verified-kernel.json")
	encoded, err := os.ReadFile(recordPath)
	if os.IsNotExist(err) {
		archive := os.Getenv("PRISM_KERNEL_ARCHIVE")
		if archive == "" {
			t.Fatal("explicit verified archive required to prepare the owned fixture")
		}
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
		defer cancel()
		prepared, err := kernel.Prepare(ctx, root, kernel.InstallInput{Source: "local", Version: "148.0.7778.215", ExpectedChecksum: "9ef3f471b7a6641b4224532522b29141ce3746e27d55788d88e2fd951f362579", ArchiveToken: "owned-local-acceptance", Trusted: true, RequestID: id()}, archive, nil, nil)
		if err != nil {
			t.Fatal(err)
		}
		destination, err := kernel.RecordDirectory(root, prepared.Record)
		if err != nil {
			t.Fatal(err)
		}
		if err = os.MkdirAll(filepath.Dir(destination), 0700); err != nil {
			t.Fatal(err)
		}
		if err = os.Rename(prepared.Directory, destination); err != nil {
			t.Fatal(err)
		}
		record = prepared.Record
		encoded, err = json.Marshal(record)
		if err != nil {
			t.Fatal(err)
		}
		if err = os.WriteFile(recordPath, encoded, 0600); err != nil {
			t.Fatal(err)
		}
		if err = os.RemoveAll(prepared.Staging); err != nil {
			t.Fatal(err)
		}
	} else if err != nil || json.Unmarshal(encoded, &record) != nil {
		t.Fatal("invalid owned kernel fixture")
	}
	if err := kernel.CheckRecord(record); err != nil {
		t.Fatal(err)
	}
	directory, err := kernel.RecordDirectory(root, record)
	if err != nil {
		t.Fatal(err)
	}
	if err = kernel.VerifyFiles(directory, record.Files); err != nil {
		t.Fatal(err)
	}
	f := &localAcceptanceFixture{record: record, reports: make(chan syntheticBrowserReport, 128)}
	observer := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if barrier := f.checkBarrier.Load(); barrier != nil {
			select {
			case barrier.entered <- struct{}{}:
			default:
			}
			select {
			case <-barrier.release:
			case <-r.Context().Done():
				return
			}
		}
		fmt.Fprint(w, `{"ip":"203.0.113.11"}`)
	}))
	t.Cleanup(observer.Close)
	pool := x509.NewCertPool()
	pool.AddCert(observer.Certificate())
	observerURL, _ := url.Parse(observer.URL)
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
		if r.URL.Hostname() != "prism-local-acceptance.test" {
			http.Error(w, "uncontrolled target", 403)
			return
		}
		if r.Method == "POST" && r.URL.Path == "/report" {
			var report syntheticBrowserReport
			if json.NewDecoder(io.LimitReader(r.Body, 16384)).Decode(&report) != nil {
				http.Error(w, "invalid synthetic report", 400)
				return
			}
			select {
			case f.reports <- report:
			default:
			}
			w.WriteHeader(204)
			return
		}
		name := r.URL.Query().Get("name")
		nameJSON, _ := json.Marshal(name)
		phase, generation := "write", name+"-OLD"
		if f.changed.Load() {
			generation = name + "-NEW"
		}
		if f.readOnly.Load() {
			phase = "read"
		}
		generationJSON, _ := json.Marshal(generation)
		w.Header().Set("Content-Type", "text/html")
		w.Header().Set("Cache-Control", "no-store")
		fmt.Fprintf(w, `<!doctype html><title>Owned local acceptance</title><script>(async()=>{const name=%s,generation=%s,phase=%q;const before={cookie:document.cookie,local:localStorage.getItem('prism'),indexed:null};const db=await new Promise((resolve,reject)=>{const r=indexedDB.open('prism',1);r.onupgradeneeded=()=>r.result.createObjectStore('state');r.onsuccess=()=>resolve(r.result);r.onerror=reject});await new Promise((resolve,reject)=>{const tx=db.transaction('state','readwrite'),s=tx.objectStore('state'),get=s.get('marker');get.onsuccess=()=>{before.indexed=get.result||null;if(phase==='write')s.put(generation,'marker')};tx.oncomplete=resolve;tx.onerror=reject});if(phase==='write'){document.cookie='prism='+generation+'; Max-Age=3600; Path=/';localStorage.setItem('prism',generation)}const after={cookie:document.cookie,local:localStorage.getItem('prism'),indexed:phase==='write'?generation:before.indexed};db.close();await fetch('/report',{method:'POST',body:JSON.stringify({name,generation,phase,before,after})})})();</script>`, nameJSON, generationJSON, phase)
	}))
	t.Cleanup(upstream.Close)
	options := Options{
		ProtectedProxyCheck:     proxy.CheckOptions{TargetURL: observer.URL, RootCAs: pool},
		ChooseBackupDestination: func() (string, error) { return f.destination, nil },
		ChooseBackupSource:      func() (string, error) { return f.source, nil },
	}
	s, err := Open(root, options)
	if err != nil {
		t.Fatal(err)
	}
	f.s = s
	t.Cleanup(func() {
		if err := f.s.Close(); err != nil {
			t.Error("owned fixture shutdown:", err)
		}
	})
	tx, err := s.db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	if _, err = tx.Exec("INSERT OR IGNORE INTO kernels(id,version,source,status) VALUES(?,?,?,'verified')", record.ID, record.Version, "fingerprint-chromium"); err != nil {
		t.Fatal(err)
	}
	if _, err = tx.Exec("INSERT OR IGNORE INTO kernel_evidence(kernel_id,record_json) VALUES(?,?)", record.ID, string(encoded)); err != nil {
		t.Fatal(err)
	}
	if err = tx.Commit(); err != nil {
		t.Fatal(err)
	}
	proxyPreview := value[ProxyImportPreview](t, call(s, "Proxy.ParseImport", map[string]string{"text": upstream.URL}))
	imported := value[struct {
		ImportedIDs []string `json:"importedIds"`
	}](t, call(s, "Proxy.CommitImport", ProxyCommitImport{PreviewID: proxyPreview.PreviewID, SelectedRows: []int{proxyPreview.Rows[0].Line}, RequestID: id()}))
	if len(imported.ImportedIDs) != 1 {
		t.Fatal("controlled upstream binding missing")
	}
	for _, bound := range view(t, s).NativeProxyRecords {
		if bound.ID == imported.ImportedIDs[0] {
			f.bound = bound
		}
	}
	if f.bound.ID == "" {
		t.Fatal("controlled upstream record missing")
	}
	return f
}

func (f *localAcceptanceFixture) environment(t *testing.T, name string, page bool) Environment {
	t.Helper()
	p := generateFingerprint(t, f.s, preview(t, f.s, "create", ""), f.record.ID, false)
	p.Environment.Name, p.Environment.ProxyID = "Owned local acceptance "+name+" "+id(), f.bound.ID
	p.Environment.RestoreTabs = false
	if page {
		p.Environment.URLs = "http://prism-local-acceptance.test/?name=" + url.QueryEscape(name)
	}
	value[any](t, call(f.s, "Environment.Create", Mutation{PreviewID: p.PreviewID, Configuration: p.Environment.Configuration, Count: 1, ProfileHash: p.Fingerprint.PreviewProfile.ConfigHash, RequestID: id()}))
	for _, e := range view(t, f.s).State.Environments {
		if e.Name == p.Environment.Name {
			return e
		}
	}
	t.Fatal("owned environment creation missing")
	return Environment{}
}

func (f *localAcceptanceFixture) start(t *testing.T, environment Environment, blank bool) RuntimeSession {
	t.Helper()
	request := runtimeRequest{EnvironmentID: environment.ID, RequestID: id(), NetworkPolicy: "proxy"}
	if blank {
		request.Purpose, request.ExpectedRevision = "cookie-import", preview(t, f.s, "edit", environment.ID).ExpectedRevision
	}
	op := acceptRuntimeTest(t, f.s, "Runtime.Start", request)
	if done := waitLocalAcceptanceDurable(t, f.s, op.ID); done.State != "completed" {
		t.Fatal("actual protected start:", done.Error)
	}
	session := localAcceptanceSession(t, f.s, environment.ID)
	if session.State != "running" || session.ProxyReport == nil || session.ProxyReport.Error != nil {
		t.Fatal("actual protected readiness missing")
	}
	return session
}

func (f *localAcceptanceFixture) stop(t *testing.T, environment Environment) {
	t.Helper()
	op := acceptRuntimeTest(t, f.s, "Runtime.Stop", runtimeRequest{EnvironmentID: environment.ID, RequestID: id()})
	if done := waitLocalAcceptanceDurable(t, f.s, op.ID); done.State != "completed" {
		t.Fatal("actual normal stop:", done.Error)
	}
	if localAcceptanceSession(t, f.s, environment.ID).ResourcesPending {
		t.Fatal("normal stop did not release owned resources")
	}
}

func localAcceptanceSession(t *testing.T, s *Service, environmentID string) RuntimeSession {
	t.Helper()
	// Workspace.Read intentionally exposes only the current eight-row page.
	// Runtime.Inspect is the actual ID-addressed service path for queue owners
	// outside that page; absence from a page is not a missing process/session.
	sessions := value[[]RuntimeSession](t, call(s, "Runtime.Inspect", map[string]any{"ids": []string{environmentID}}))
	if len(sessions) != 1 || sessions[0].EnvironmentID != environmentID {
		t.Fatal("exact ID-addressed runtime observation missing")
	}
	return sessions[0]
}

func waitLocalAcceptanceDurable(t *testing.T, s *Service, operationID string) Operation {
	t.Helper()
	deadline := time.Now().Add(2 * time.Minute)
	for time.Now().Before(deadline) {
		op := value[Operation](t, call(s, "Operation.Read", map[string]string{"operationId": operationID}))
		if !op.PersistencePending && (op.State == "completed" || op.State == "failed" || op.State == "cancelled") {
			return op
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("actual operation did not reach a durable terminal outcome")
	return Operation{}
}

func (f *localAcceptanceFixture) transport(t *testing.T, environment Environment) cookieTransport {
	t.Helper()
	f.s.mu.Lock()
	defer f.s.mu.Unlock()
	process := f.s.runtimeSlots[environment.ID].process
	if _, ok := process.(*kernel.ManagedProcess); !ok {
		t.Fatal("fixture is not the production ManagedProcess")
	}
	return process.(cookieTransport)
}

func readLocalCookies(t *testing.T, transport cookieTransport) []cookies.Stored {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	stored, err := transport.ReadCookies(ctx)
	if err != nil {
		t.Fatal("actual private Cookie readback:", err)
	}
	return stored
}

func sameLocalCookieCollection(a, b []cookies.Stored) bool {
	if len(a) != len(b) {
		return false
	}
	wanted := map[string]cookies.Stored{}
	for _, value := range a {
		key := cookies.Key(value.Cookie)
		if _, duplicate := wanted[key]; duplicate {
			return false
		}
		wanted[key] = value
	}
	for _, value := range b {
		key := cookies.Key(value.Cookie)
		if previous, found := wanted[key]; !found || !reflect.DeepEqual(previous, value) {
			return false
		}
		delete(wanted, key)
	}
	return len(wanted) == 0
}

func writeLocalAcceptanceEvidence(t *testing.T, name string, facts map[string]any) {
	t.Helper()
	directory := os.Getenv("PRISM_LOCAL_ACCEPTANCE_EVIDENCE")
	if directory == "" {
		return
	}
	facts["observedAt"] = timestamp()
	if _, exists := facts["scope"]; !exists {
		facts["scope"] = "real production provider and exact 148; owned synthetic workspace and controlled local upstream; no UI clicks or independent external egress claim"
	}
	facts["automatedUIClicks"] = false
	encoded, err := json.MarshalIndent(facts, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err = os.MkdirAll(directory, 0700); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(directory, name+".json"), append(encoded, '\n'), 0600); err != nil {
		t.Fatal(err)
	}
}

func assertLocalIdentityUnchanged(t *testing.T, s *Service, environment Environment, original DeviceProfile, reference string) {
	t.Helper()
	state := view(t, s)
	if !reflect.DeepEqual(state.Fingerprints[environment.ID].Profile, original) || state.DataReferences[environment.ID] != reference {
		t.Fatal("local acceptance changed fixed identity or original data reference")
	}
}
