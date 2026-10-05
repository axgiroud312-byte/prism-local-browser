//go:build windows

package kernel

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"golang.org/x/sys/windows"
)

func TestNetworkStoreRecoversPartialTreeGrantIncludingNewFiles(t *testing.T) {
	root := t.TempDir()
	store, err := OpenNetworkStore(root)
	if err != nil {
		t.Fatal(err)
	}
	i := journalIntent()
	i.DataReference = "environments/" + i.EnvironmentID + "/user-data"
	i.JobName = `Global\PrismManagedSession-` + i.SessionID
	sid, err := deriveNetworkSID(i.ContainerName)
	if err != nil {
		t.Fatal(err)
	}
	i.PackageSID = sid.String()
	path := filepath.Join(root, filepath.FromSlash(i.DataReference))
	if err = os.MkdirAll(filepath.Join(path, "Default"), 0700); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(path, "Default", "existing.txt")
	if err = os.WriteFile(file, []byte("synthetic-data"), 0600); err != nil {
		t.Fatal(err)
	}
	original, err := windows.GetNamedSecurityInfo(file, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION)
	if err != nil {
		t.Fatal(err)
	}
	if err = store.journal.PrepareSession(context.Background(), i); err != nil {
		t.Fatal(err)
	}
	p := &ProtectedProxy{store: store, intent: i}
	if err = p.grantTree(context.Background(), path, windows.FILE_GENERIC_READ|windows.FILE_GENERIC_WRITE|windows.FILE_GENERIC_EXECUTE|windows.DELETE); err != nil {
		t.Fatal(err)
	}
	newFile := filepath.Join(path, "Default", "new.txt")
	if err = os.WriteFile(newFile, []byte("new-data"), 0600); err != nil {
		t.Fatal(err)
	}
	if err = store.Close(); err != nil {
		t.Fatal(err)
	}
	store, err = OpenNetworkStore(root)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if err = store.Recover(context.Background(), i); err != nil {
		t.Fatal(err)
	}
	after, err := windows.GetNamedSecurityInfo(file, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION)
	if err != nil || after.String() != original.String() {
		redact := regexp.MustCompile(`S-1-[0-9-]+`)
		t.Fatalf("unrelated permissions changed: %v; before=%s after=%s", err, redact.ReplaceAllString(original.String(), "REDACTED-SID"), redact.ReplaceAllString(after.String(), "REDACTED-SID"))
	}
	for _, file := range []string{file, newFile} {
		data, err := os.ReadFile(file)
		if err != nil || len(data) == 0 {
			t.Fatal("data lost", err)
		}
		sd, err := windows.GetNamedSecurityInfo(file, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(sd.String(), i.PackageSID) {
			t.Fatal("owned permission survived recovery")
		}
	}
	pending, err := store.Pending(context.Background())
	if err != nil || len(pending) != 0 {
		t.Fatal("cleanup remained pending", err)
	}
}

func TestNetworkStoreRefusesReplacedPermissionRootAndSecondOwner(t *testing.T) {
	root := t.TempDir()
	s, err := OpenNetworkStore(root)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if other, err := OpenNetworkStore(root); err == nil {
		other.Close()
		t.Fatal("second journal owner accepted")
	}
	i := journalIntent()
	i.DataReference = "environments/" + i.EnvironmentID + "/user-data"
	i.JobName = `Global\PrismManagedSession-` + i.SessionID
	sid, err := deriveNetworkSID(i.ContainerName)
	if err != nil {
		t.Fatal(err)
	}
	i.PackageSID = sid.String()
	path := filepath.Join(root, filepath.FromSlash(i.DataReference))
	if err = os.MkdirAll(path, 0700); err != nil {
		t.Fatal(err)
	}
	h, identity, _, err := openNetworkACLObject(path)
	if err != nil {
		t.Fatal(err)
	}
	windows.CloseHandle(h)
	if err = s.journal.PrepareSession(context.Background(), i); err != nil {
		t.Fatal(err)
	}
	delta, _ := json.Marshal(networkACLDelta{SID: i.PackageSID, Mask: windows.FILE_GENERIC_READ, Tree: true})
	if err = s.journal.ApplyResource(context.Background(), i.SessionID, NetworkResourceIntent{ResourceID: "tree", Kind: "file-acl", ObjectIdentity: identity, Locator: path, Delta: string(delta)}, func(context.Context) error { return nil }); err != nil {
		t.Fatal(err)
	}
	if err = os.Rename(path, path+"-original"); err != nil {
		t.Fatal(err)
	}
	if err = os.Mkdir(path, 0700); err != nil {
		t.Fatal(err)
	}
	if err = s.Recover(context.Background(), i); err == nil {
		t.Fatal("replacement accepted as original ACL object")
	}
	pending, err := s.Pending(context.Background())
	if err != nil || len(pending) != 1 {
		t.Fatal("unknown cleanup released environment", err)
	}
}

func TestNetworkLogonAndBootIdentityAreStableReadOnlyObservations(t *testing.T) {
	logon, station, err := networkLogonIdentity()
	if err != nil || station == "" {
		t.Fatal("current logon identity unavailable", err)
	}
	if ended, err := networkLogonEnded(logon); err != nil || ended {
		t.Fatal("current authentication LUID not recognized", err)
	}
	a, err := networkBootIdentity()
	if err != nil {
		t.Fatal(err)
	}
	b, err := networkBootIdentity()
	if err != nil || a != b || len(a) != 32 {
		t.Fatal("boot identity unstable", err)
	}
}
