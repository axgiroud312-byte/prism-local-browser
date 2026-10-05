package workspace

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/axgiroud312-byte/prism-local-browser/internal/kernel"
	"golang.org/x/sys/windows"
)

func TestNetworkResourceRecoveryKeepsOnlyAffectedEnvironmentBusy(t *testing.T) {
	s, root := fixture(t, Options{})
	a, _ := create(t, s, "Synthetic pending network A")
	b, _ := create(t, s, "Synthetic unaffected B")
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	j, err := kernel.OpenNetworkJournal(filepath.Join(root, "network-resources", "resources.db"))
	if err != nil {
		t.Fatal(err)
	}
	session := id()
	intent := kernel.NetworkSessionIntent{SessionID: session, EnvironmentID: a.ID, ChannelID: id(), DataReference: "environments/" + a.ID + "/user-data", ContainerName: "prism-session-" + session, PackageSID: "S-1-15-2-123456789-123456789-123456789-123456789-123456789-123456789-123456789", JobName: `Global\PrismManagedSession-` + session, LogonSID: "synthetic-logon"}
	if err = j.PrepareSession(context.Background(), intent); err != nil {
		t.Fatal(err)
	}
	delta, _ := json.Marshal(map[string]any{"sid": intent.PackageSID, "mask": uint32(1), "tree": true})
	// No OS permission is changed. The missing, unconfirmed object must retain
	// ownership even though app.db has no runtime session for this journal.
	path := filepath.Join(root, filepath.FromSlash(intent.DataReference))
	if err = os.MkdirAll(path, 0700); err != nil {
		t.Fatal(err)
	}
	wide, _ := windows.UTF16PtrFromString(path)
	h, err := windows.CreateFile(wide, windows.FILE_READ_ATTRIBUTES, windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE, nil, windows.OPEN_EXISTING, windows.FILE_FLAG_BACKUP_SEMANTICS, 0)
	if err != nil {
		t.Fatal(err)
	}
	var info windows.ByHandleFileInformation
	err = windows.GetFileInformationByHandle(h, &info)
	windows.CloseHandle(h)
	if err != nil {
		t.Fatal(err)
	}
	identity := fmt.Sprintf("%08x:%08x%08x:%08x%08x", info.VolumeSerialNumber, info.FileIndexHigh, info.FileIndexLow, info.CreationTime.HighDateTime, info.CreationTime.LowDateTime)
	resource := kernel.NetworkResourceIntent{ResourceID: "missing-tree", Kind: "file-acl", ObjectIdentity: identity, Locator: path, Delta: string(delta)}
	if err = j.ApplyResource(context.Background(), session, resource, func(context.Context) error { return nil }); err != nil {
		t.Fatal(err)
	}
	if err = j.Close(); err != nil {
		t.Fatal(err)
	}
	if err = os.Rename(path, path+"-retained"); err != nil {
		t.Fatal(err)
	}
	reopened, err := Open(root, Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	if !reopened.hasPendingNetwork(a.ID) || !reopened.profileUses[a.ID] || reopened.hasPendingNetwork(b.ID) || reopened.profileUses[b.ID] {
		t.Fatal("pending journal either released A or blocked B")
	}
	reopened.releaseProfileUse(a.ID)
	if !reopened.profileUses[a.ID] {
		t.Fatal("ordinary release discarded the independent resource owner")
	}
	loader := &Service{profileUses: map[string]bool{}}
	loader.inheritNetworkResources(reopened)
	delete(loader.networkPending, a.ID)
	if !reopened.hasPendingNetwork(a.ID) {
		t.Fatal("bootstrap shared a mutable pending map")
	}
	if len(view(t, reopened).State.Environments) != 2 {
		t.Fatal("resource recovery removed saved environments")
	}
	request := runtimeRequest{EnvironmentID: a.ID, SessionID: intent.SessionID, RequestID: id()}
	op := acceptRuntimeTest(t, reopened, "Runtime.Reconcile", request)
	if done := waitRuntimeReal(t, reopened, op.ID); done.State != "failed" || done.Error.Code != "NETWORK_CLEANUP_PENDING" {
		t.Fatal("missing resource incorrectly recovered")
	}
	if !reopened.hasPendingNetwork(a.ID) || !reopened.profileUses[a.ID] {
		t.Fatal("failed explicit cleanup released A")
	}
	if err = os.Rename(path+"-retained", path); err != nil {
		t.Fatal(err)
	}
	request.RequestID = id()
	op = acceptRuntimeTest(t, reopened, "Runtime.Reconcile", request)
	if done := waitRuntimeReal(t, reopened, op.ID); done.State != "completed" {
		t.Fatalf("in-place cleanup retry failed: %+v", done.Error)
	}
	if reopened.hasPendingNetwork(a.ID) || reopened.profileUses[a.ID] || reopened.runtimeSlots[a.ID] != nil {
		t.Fatal("orphan recovery retained occupancy or invented browser session")
	}
}

func TestNetworkRecoveryUnpersistedResultRetainsOccupancyAndFailsShutdown(t *testing.T) {
	s, _ := fixture(t, Options{})
	e, _ := create(t, s, "Synthetic recovery persistence")
	intent := kernel.NetworkSessionIntent{EnvironmentID: e.ID, SessionID: id()}
	op := Operation{ID: id(), Kind: "runtime-reconcile", State: "accepted", EnvironmentID: e.ID, SessionID: intent.SessionID}
	accepted := s.acceptRuntime("Runtime.Reconcile", runtimeRequest{EnvironmentID: e.ID, SessionID: intent.SessionID, RequestID: id()}, op)
	if !accepted.OK {
		t.Fatal(accepted.Error)
	}
	op.State = "completed"
	s.networkPending[e.ID] = intent
	s.networkRecoveries = map[string]*networkRecoveryTask{e.ID: {intent: intent, operation: op, finished: true, recovered: true}}
	s.profileUses[e.ID] = true
	if _, err := s.db.Exec(`CREATE TRIGGER synthetic_recovery_write_failure BEFORE UPDATE ON operations BEGIN SELECT RAISE(ABORT, 'synthetic disk failure'); END`); err != nil {
		t.Fatal(err)
	}
	s.flushRuntimePersistence()
	if !s.hasPendingNetwork(e.ID) || !s.profileUses[e.ID] || !s.runtimeResults[op.ID].PersistencePending {
		t.Fatal("unpersisted cleanup result released protection")
	}
	if err := s.Close(); err == nil {
		t.Fatal("shutdown hid failed resource result persistence")
	}
}
