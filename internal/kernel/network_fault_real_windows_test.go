//go:build windows

package kernel

import (
	"bufio"
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
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/axgiroud312-byte/prism-local-browser/internal/proxy"
	"github.com/google/uuid"
)

type protectedFaultFixture struct {
	root               string
	record             Record
	store              *NetworkStore
	upstream, observer *httptest.Server
	options            proxy.BridgeOptions
	config             proxy.Configuration
	pages              atomic.Int64
}

func newProtectedFaultFixture(t *testing.T) *protectedFaultFixture {
	t.Helper()
	root := os.Getenv("PRISM_PROTECTED_TEST_ROOT")
	if root == "" {
		t.Skip("controlled fault fixture not selected")
	}
	root, _ = filepath.Abs(root)
	marker, err := os.ReadFile(filepath.Join(root, "synthetic-network-fixture"))
	if err != nil || string(marker) != "prism-owned-synthetic-v1" {
		t.Fatal("not an owned synthetic root")
	}
	f := &protectedFaultFixture{root: root}
	data, err := os.ReadFile(filepath.Join(root, "verified-kernel.json"))
	if err != nil || json.Unmarshal(data, &f.record) != nil || CheckRecord(f.record) != nil {
		t.Fatal("verified fixture installation required")
	}
	f.store, err = OpenNetworkStore(root)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := f.store.Close(); err != nil {
			t.Error(err)
		}
	})
	pending, err := f.store.Pending(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	for _, intent := range pending {
		if err := f.store.Recover(context.Background(), intent); err != nil {
			t.Fatal("previous fixture recovery", err)
		}
	}
	f.observer = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, `{"ip":"203.0.113.17"}`) }))
	t.Cleanup(f.observer.Close)
	pool := x509.NewCertPool()
	pool.AddCert(f.observer.Certificate())
	f.options = proxy.BridgeOptions{TargetURL: f.observer.URL, RootCAs: pool}
	observerURL, _ := url.Parse(f.observer.URL)
	f.upstream = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "CONNECT" {
			if r.Host != observerURL.Host {
				http.Error(w, "uncontrolled destination", 403)
				return
			}
			remote, err := net.DialTimeout("tcp4", observerURL.Host, 3*time.Second)
			if err != nil {
				http.Error(w, "observer unavailable", 502)
				return
			}
			client, buffer, err := w.(http.Hijacker).Hijack()
			if err != nil {
				remote.Close()
				return
			}
			fmt.Fprint(buffer, "HTTP/1.1 200 Connection Established\r\n\r\n")
			buffer.Flush()
			go func() { defer remote.Close(); defer client.Close(); io.Copy(remote, buffer) }()
			io.Copy(client, remote)
			client.Close()
			remote.Close()
			return
		}
		f.pages.Add(1)
		w.Header().Set("Content-Type", "text/html")
		w.Header().Set("Cache-Control", "no-store")
		fmt.Fprint(w, "<!doctype html><title>Synthetic fault fixture</title>")
	}))
	t.Cleanup(f.upstream.Close)
	u, _ := url.Parse(f.upstream.URL)
	port, _ := strconv.Atoi(u.Port())
	f.config = proxy.Configuration{Name: "Synthetic fault upstream", Type: "http", Host: u.Hostname(), Port: port}
	return f
}

func (f *protectedFaultFixture) open(t *testing.T, launch bool) (*ProtectedProxy, *ManagedProcess) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 35*time.Second)
	defer cancel()
	env := uuid.NewString()
	profile := ManagedProfile{EnvironmentID: env, SessionID: uuid.NewString(), UserDataRef: "environments/" + env + "/user-data", Fingerprint: FingerprintInput{Seed: "1256789", Language: "en-US", Timezone: "America/New_York", CPU: "8"}, Width: 800, Height: 600}
	p, err := f.store.openProtectedProxy(ctx, f.record, profile, uuid.NewString(), f.config, nil, f.options)
	if p != nil {
		t.Cleanup(func() {
			if err := p.Close(); err != nil {
				t.Error("channel cleanup", err)
			}
		})
	}
	if err != nil {
		t.Fatal("protected prepare", err)
	}
	if !launch {
		return p, nil
	}
	if report := p.Preflight(ctx, nil); report.Error != nil {
		t.Fatal(report.Error)
	}
	profile.Network = p
	process, err := LaunchManagedProfile(ctx, f.root, f.record, profile)
	if process != nil {
		t.Cleanup(func() {
			if err := process.Close(); err != nil {
				t.Error("process cleanup", err)
			}
		})
	}
	if err != nil {
		t.Fatal("protected launch", err)
	}
	return p, process
}

func faultNavigate(ctx context.Context, p *ManagedProcess, address string) error {
	return p.pipe.call(ctx, "Target.createTarget", map[string]any{"url": address}, "", nil)
}

func waitFaultCondition(t *testing.T, condition func() bool) {
	t.Helper()
	deadline := time.Now().Add(12 * time.Second)
	for time.Now().Before(deadline) {
		if condition() {
			return
		}
		time.Sleep(40 * time.Millisecond)
	}
	t.Fatal("fault observation did not converge")
}

func TestRealProtectedFaultContainmentAndIndependence(t *testing.T) {
	f := newProtectedFaultFixture(t)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	a, pa := f.open(t, true)
	b, pb := f.open(t, true)
	if err := faultNavigate(ctx, pa, "http://prism-fault.test/baseline"); err != nil {
		t.Fatal(err)
	}
	waitFaultCondition(t, func() bool { return f.pages.Load() > 0 })
	// Closing A's bridge must stop its exact Job before the old endpoint can
	// be treated as a usable channel. Standard proxy mode does not claim OS
	// isolation against an arbitrary replacement listener.
	u, _ := url.Parse(a.Endpoint())
	if err := a.bridge.Close(); err != nil {
		t.Fatal(err)
	}
	select {
	case <-pa.Done():
	case <-ctx.Done():
		t.Fatal("bridge disposal did not stop exact A tree")
	}
	listener, err := net.Listen("tcp4", u.Host)
	if err != nil {
		t.Fatal("old port takeover", err)
	}
	var hijacked atomic.Int64
	takeover := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { hijacked.Add(1); fmt.Fprint(w, "unexpected takeover") })}
	go takeover.Serve(listener)
	defer takeover.Close()
	// Positive control proves the replacement listener is reachable by an
	// ordinary caller after the owned browser Job has stopped.
	response, err := http.Get("http://" + u.Host + "/ordinary-control")
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	waitFaultCondition(t, func() bool { return hijacked.Load() == 1 })
	time.Sleep(700 * time.Millisecond)
	if pa.Alive() || hijacked.Load() != 1 {
		t.Fatal("bridge-loss Job remained alive or an owned request reached replacement listener")
	}
	if err := pa.Stop(ctx); err != nil {
		t.Fatal(err)
	}
	before := f.pages.Load()
	if err := faultNavigate(ctx, pb, "http://prism-fault.test/b-remains-usable"); err != nil {
		t.Fatal(err)
	}
	waitFaultCondition(t, func() bool { return f.pages.Load() > before })
	if !pb.Alive() || b.Fault() != nil {
		t.Fatal("A cleanup affected B")
	}
	// The real upstream stops accepting requests. A positive direct control
	// listener must not see browser navigation through a failed proxy.
	var bypass atomic.Int64
	direct := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { bypass.Add(1); fmt.Fprint(w, "control") }))
	defer direct.Close()
	response, err = http.Get(direct.URL)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	f.upstream.CloseClientConnections()
	f.upstream.Close()
	_ = faultNavigate(ctx, pb, direct.URL+"/blocked")
	select {
	case <-pb.Done():
	case <-ctx.Done():
		t.Fatal("upstream loss did not stop exact tree")
	}
	if b.Fault() == nil || bypass.Load() != 1 {
		t.Fatal("upstream loss missing fault or direct bypass observed")
	}
	pending, err := f.store.Pending(ctx)
	if err != nil || len(pending) != 0 {
		t.Fatal("fault cleanup unresolved", err)
	}
	t.Log("bridge disposal stopped exact A Job; ordinary replacement control reached; A cleanup preserved B; upstream loss stopped B; no direct fixture request; pending=0")
}

// Parent kills this exact owned helper after a flushed readiness line. No
// deferred cleanup runs: journal and OS resources are genuinely interrupted.
func TestRealProtectedCrashHelper(t *testing.T) {
	mode := os.Getenv("PRISM_PROTECTED_CRASH_MODE")
	if mode != "prepared" && mode != "running" {
		t.Skip("child only")
	}
	f := newProtectedFaultFixture(t)
	p, process := f.open(t, mode == "running")
	if process != nil {
		if err := verifyNetworkTree(process.pipe, p.sid); err != nil {
			t.Fatal(err)
		}
	}
	bytes, _ := json.Marshal(p.intent)
	fmt.Println("PRISM-CRASH-READY " + string(bytes))
	// A parent-held stdin keeps this child waiting without timers or cleanup.
	_, _ = io.Copy(io.Discard, os.Stdin)
	t.Fatal("parent failed to terminate the prepared helper")
}

func TestRealProtectedManagerCrashRecovery(t *testing.T) {
	if os.Getenv("PRISM_PROTECTED_TEST_ROOT") == "" {
		t.Skip("controlled crash fixture not selected")
	}
	for _, mode := range []string{"prepared", "running"} {
		t.Run(mode, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 65*time.Second)
			defer cancel()
			cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestRealProtectedCrashHelper$", "-test.v")
			cmd.Env = append(os.Environ(), "PRISM_PROTECTED_CRASH_MODE="+mode)
			stdout, err := cmd.StdoutPipe()
			if err != nil {
				t.Fatal(err)
			}
			stdin, err := cmd.StdinPipe()
			if err != nil {
				t.Fatal(err)
			}
			defer stdin.Close()
			if err = cmd.Start(); err != nil {
				t.Fatal(err)
			}
			defer func() { _ = cmd.Process.Kill() }()
			var intent NetworkSessionIntent
			scanner := bufio.NewScanner(stdout)
			for scanner.Scan() {
				line := scanner.Text()
				if strings.HasPrefix(line, "PRISM-CRASH-READY ") {
					if json.Unmarshal([]byte(strings.TrimPrefix(line, "PRISM-CRASH-READY ")), &intent) != nil {
						t.Fatal("invalid child evidence")
					}
					break
				}
			}
			if intent.SessionID == "" {
				t.Fatal("child not ready", scanner.Err())
			}
			if err = cmd.Process.Kill(); err != nil {
				t.Fatal(err)
			}
			if err = cmd.Wait(); err == nil {
				t.Fatal("helper exited normally instead of hard termination")
			}
			waitFaultCondition(t, func() bool { gone, err := managedJobResourcesExited(intent.SessionID); return err == nil && gone })
			root, _ := filepath.Abs(os.Getenv("PRISM_PROTECTED_TEST_ROOT"))
			store, err := OpenNetworkStore(root)
			if err != nil {
				t.Fatal(err)
			}
			defer store.Close()
			pending, err := store.Pending(ctx)
			if err != nil || pending[intent.EnvironmentID].SessionID != intent.SessionID {
				t.Fatal("crash lost durable ownership", err)
			}
			if err = store.Recover(ctx, intent); err != nil {
				// Job empty can precede completion of Windows' final delete-on-
				// close work. Unknown cleanup must retain ownership and be retryable.
				pending, readErr := store.Pending(ctx)
				if readErr != nil || pending[intent.EnvironmentID].SessionID != intent.SessionID {
					t.Fatal("uncertain cleanup lost ownership", readErr)
				}
				t.Log("first cleanup uncertain; ownership retained; retrying actual observation")
				waitFaultCondition(t, func() bool { return store.Recover(ctx, intent) == nil })
			}
			if pending, err = store.Pending(ctx); err != nil || len(pending) != 0 {
				t.Fatal("crash retained unresolved resources", err)
			}
			if exists, err := networkContainerExists(intent.PackageSID); err != nil || exists {
				t.Fatal("container survived recovery", err)
			}
			if _, err = os.Stat(filepath.Join(root, filepath.FromSlash(intent.DataReference))); err != nil {
				t.Fatal("original data reference lost", err)
			}
			t.Log("hard exit observed; exact Job absent/empty; durable pending found; actual ACL/container recovery completed; original data tree preserved")
		})
	}
}
