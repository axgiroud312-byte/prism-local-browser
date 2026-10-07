//go:build windows

package kernel

import (
	"context"
	"crypto/sha256"
	"crypto/x509"
	"encoding/hex"
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
	"testing"
	"time"

	"github.com/axgiroud312-byte/prism-local-browser/internal/proxy"
	"github.com/google/uuid"
	"golang.org/x/sys/windows"
)

// This PE fixture is only read/hashed. No browser or copied executable is run.
func standardProxyRecord(t *testing.T, root string) Record {
	t.Helper()
	r := compilerRecord()
	source := filepath.Join(os.Getenv("SystemRoot"), "System32", "whoami.exe")
	version, err := FileVersion(source)
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(source)
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(data)
	r.Version, r.Source.Tag = version, version
	r.ExecutableSHA256 = hex.EncodeToString(sum[:])
	r.Files = map[string]string{r.ExecutableRelativePath: r.ExecutableSHA256}
	directory, err := RecordDirectory(root, r)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.MkdirAll(directory, 0700); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(directory, r.ExecutableRelativePath), data, 0600); err != nil {
		t.Fatal(err)
	}
	return r
}

func standardProxyFixture(t *testing.T) (proxy.Configuration, proxy.BridgeOptions) {
	t.Helper()
	target := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"ip":"203.0.113.54"}`)
	}))
	t.Cleanup(target.Close)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodConnect || r.Host != target.Listener.Addr().String() {
			w.WriteHeader(http.StatusForbidden)
			return
		}
		remote, err := net.DialTimeout("tcp4", r.Host, time.Second)
		if err != nil {
			w.WriteHeader(http.StatusBadGateway)
			return
		}
		defer remote.Close()
		client, buffered, err := w.(http.Hijacker).Hijack()
		if err != nil {
			return
		}
		defer client.Close()
		remote.SetDeadline(time.Now().Add(5 * time.Second))
		client.SetDeadline(time.Now().Add(5 * time.Second))
		fmt.Fprint(buffered, "HTTP/1.1 200 Connection Established\r\n\r\n")
		buffered.Flush()
		done := make(chan struct{})
		go func() { io.Copy(remote, buffered); close(done) }()
		io.Copy(client, remote)
		remote.Close()
		client.Close()
		<-done
	}))
	t.Cleanup(upstream.Close)
	address, _ := url.Parse(upstream.URL)
	port, _ := strconv.Atoi(address.Port())
	pool := x509.NewCertPool()
	pool.AddCert(target.Certificate())
	return proxy.Configuration{Name: "Synthetic local proxy", Type: "http", Host: address.Hostname(), Port: port}, proxy.BridgeOptions{TargetURL: target.URL, RootCAs: pool}
}

func TestStandardProxyOwnersShareKernelWithoutPermissionOrContainerChanges(t *testing.T) {
	root := t.TempDir()
	record := standardProxyRecord(t, root)
	store, err := OpenNetworkStore(root)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	config, options := standardProxyFixture(t)
	directory, _ := RecordDirectory(root, record)
	security := func(path string) string {
		t.Helper()
		sd, err := windows.GetNamedSecurityInfo(path, windows.SE_FILE_OBJECT, windows.OWNER_SECURITY_INFORMATION|windows.GROUP_SECURITY_INFORMATION|windows.DACL_SECURITY_INFORMATION)
		if err != nil {
			t.Fatal(err)
		}
		return sd.String()
	}
	beforeRoot, beforeFile := security(directory), security(filepath.Join(directory, record.ExecutableRelativePath))
	var owners []*ProtectedProxy
	var profiles []ManagedProfile
	for index := 0; index < 2; index++ {
		environment := uuid.NewString()
		profile := ManagedProfile{SessionID: uuid.NewString(), EnvironmentID: environment, UserDataRef: "environments/" + environment + "/user-data"}
		path := filepath.Join(root, filepath.FromSlash(profile.UserDataRef))
		if err = os.MkdirAll(path, 0700); err != nil {
			t.Fatal(err)
		}
		beforeProfile := security(path)
		owner, err := store.openProtectedProxy(context.Background(), record, profile, uuid.NewString(), config, nil, options)
		if owner != nil {
			defer func() {
				if err := owner.Close(); err != nil {
					t.Error(err)
				}
			}()
		}
		if err != nil {
			t.Fatal(err)
		}
		if owner.intent.Mode != proxyBridgeSessionMode || owner.sid != nil || owner.token != 0 || owner.station != 0 || owner.intent.PackageSID != "" || owner.intent.ContainerName != "" || security(path) != beforeProfile {
			t.Fatal("standard owner introduced AppContainer or permission changes")
		}
		var count int
		var encoded string
		var resource NetworkResourceIntent
		err = store.journal.db.QueryRow("SELECT COUNT(*) FROM resources WHERE session_id=?", profile.SessionID).Scan(&count)
		if err != nil || count != 1 {
			t.Fatal("standard owner journal contains unexpected resource count", count, err)
		}
		err = store.journal.db.QueryRow("SELECT intent FROM resources WHERE session_id=?", profile.SessionID).Scan(&encoded)
		if err != nil || json.Unmarshal([]byte(encoded), &resource) != nil || resource.Kind != "bridge" {
			t.Fatal("standard owner journal contains unexpected resource", resource, err)
		}
		if owner.validate(root, record, profile) == nil {
			t.Fatal("unchecked owner authorized launch")
		}
		report := owner.Preflight(context.Background(), nil)
		if report.Error != nil || report.ExitIP != "203.0.113.54" || report.ChannelID != owner.ID() || owner.validate(root, record, profile) != nil {
			t.Fatal("real loopback proxy preflight did not authorize exact owner", report)
		}
		owners, profiles = append(owners, owner), append(profiles, profile)
	}
	if owners[0].Endpoint() == owners[1].Endpoint() || owners[0].ID() == owners[1].ID() || owners[0].validate(root, record, profiles[1]) == nil {
		t.Fatal("two environment channels or launch owners were conflated")
	}
	endpoint, _ := url.Parse(owners[0].Endpoint())
	transport := &http.Transport{Proxy: http.ProxyURL(endpoint)}
	defer transport.CloseIdleConnections()
	response, err := (&http.Client{Transport: transport, Timeout: 3 * time.Second}).Get("http://synthetic-not-forwarded.invalid/")
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusForbidden {
		t.Fatal("ordinary unbound host request entered browser channel", response.StatusCode)
	}
	if err = owners[0].Close(); err != nil {
		t.Fatal(err)
	}
	if owners[0].validate(root, record, profiles[0]) == nil || owners[1].validate(root, record, profiles[1]) != nil {
		t.Fatal("closing A released or reauthorized the wrong owner")
	}
	if security(directory) != beforeRoot || security(filepath.Join(directory, record.ExecutableRelativePath)) != beforeFile {
		t.Fatal("shared kernel permissions changed")
	}
	if _, err = os.Stat(filepath.Join(root, "network-resources", "kernel-copies")); !os.IsNotExist(err) {
		t.Fatal("standard session allocated a private kernel copy", err)
	}
	if err = owners[1].Close(); err != nil {
		t.Fatal(err)
	}
	pending, err := store.Pending(context.Background())
	if err != nil || len(pending) != 0 {
		t.Fatal("standard proxy session cleanup incomplete", err)
	}
}

func TestStandardProxyFailedPreflightKeepsLaunchBlocked(t *testing.T) {
	root := t.TempDir()
	record := standardProxyRecord(t, root)
	store, err := OpenNetworkStore(root)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	listener.Close()
	environment := uuid.NewString()
	profile := ManagedProfile{SessionID: uuid.NewString(), EnvironmentID: environment, UserDataRef: "environments/" + environment + "/user-data"}
	owner, err := store.openProtectedProxy(context.Background(), record, profile, uuid.NewString(), proxy.Configuration{Name: "Synthetic unavailable", Type: "http", Host: "127.0.0.1", Port: port}, nil, proxy.BridgeOptions{})
	if owner != nil {
		defer owner.Close()
	}
	if err != nil {
		t.Fatal(err)
	}
	report := owner.Preflight(context.Background(), nil)
	if report.Error == nil || owner.preflighted || owner.validate(root, record, profile) == nil {
		t.Fatal("failed proxy preflight authorized launch or direct fallback", report)
	}
}
