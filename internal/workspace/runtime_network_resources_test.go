package workspace

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/axgiroud312-byte/prism-local-browser/internal/kernel"
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
	resource := kernel.NetworkResourceIntent{ResourceID: "missing-tree", Kind: "file-acl", ObjectIdentity: "synthetic-unknown-object", Locator: filepath.Join(root, filepath.FromSlash(intent.DataReference)), Delta: string(delta)}
	if err = j.ApplyResource(context.Background(), session, resource, func(context.Context) error { return nil }); err != nil {
		t.Fatal(err)
	}
	if err = j.Close(); err != nil {
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
}
