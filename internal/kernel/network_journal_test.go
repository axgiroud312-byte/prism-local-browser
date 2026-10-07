package kernel

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"testing"
	"time"

	"github.com/google/uuid"
)

func journalIntent() NetworkSessionIntent {
	session := uuid.NewString()
	return NetworkSessionIntent{SessionID: session, EnvironmentID: uuid.NewString(), ChannelID: uuid.NewString(), DataReference: "profiles/synthetic/userdata", ContainerName: "prism-session-" + session, PackageSID: "synthetic-package", JobName: "synthetic-job-" + session, LogonSID: "synthetic-login"}
}

// Journal-only tests own no actual browser/Job; real providers must query the
// exact named Job here, after creation has been sealed by the lifecycle lock.
func noJournalTestProcesses(context.Context) error { return nil }

func TestNetworkJournalRecoversUnacknowledgedResourcesAndRetainsFailedCleanup(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "network-resources.db")
	j, err := OpenNetworkJournal(path)
	if err != nil {
		t.Fatal(err)
	}
	intent := journalIntent()
	if err = j.PrepareSession(ctx, intent); err != nil {
		t.Fatal(err)
	}
	for _, resource := range []NetworkResourceIntent{
		{ResourceID: "container", Kind: "container", ObjectIdentity: intent.PackageSID, Locator: intent.ContainerName},
		{ResourceID: "permission", Kind: "file-acl", ObjectIdentity: "volume:file-id", Locator: "synthetic/profile", Delta: "synthetic-session-ace"},
	} {
		if err = j.prepareResource(ctx, intent.SessionID, resource); err != nil {
			t.Fatal(err)
		}
	}
	// Simulate OS success followed by loss of acknowledgement: no MarkApplied.
	if err = j.Close(); err != nil {
		t.Fatal(err)
	}
	j, err = OpenNetworkJournal(path)
	if err != nil {
		t.Fatal(err)
	}
	defer j.Close()
	blocked := errors.New("object could not be positively identified")
	var calls []string
	err = j.Cleanup(ctx, intent.SessionID, noJournalTestProcesses, func(_ context.Context, r NetworkResourceRecord) error {
		calls = append(calls, r.ResourceID)
		if r.State != "prepared" {
			t.Fatal("lost acknowledgement became success")
		}
		return blocked
	})
	if !errors.Is(err, blocked) || !reflect.DeepEqual(calls, []string{"permission"}) {
		t.Fatalf("cleanup continued past unknown permission: %v %v", calls, err)
	}
	pending, err := j.PendingSessions(ctx, "")
	if err != nil || len(pending) != 1 || pending[0] != intent {
		t.Fatalf("ownership lost: %+v %v", pending, err)
	}
	other := journalIntent()
	other.EnvironmentID = intent.EnvironmentID
	if err = j.PrepareSession(ctx, other); err == nil {
		t.Fatal("unconfirmed cleanup released environment")
	}
	calls = nil
	if err = j.Cleanup(ctx, intent.SessionID, noJournalTestProcesses, func(_ context.Context, r NetworkResourceRecord) error {
		calls = append(calls, r.ResourceID)
		return nil
	}); err != nil || !reflect.DeepEqual(calls, []string{"permission", "container"}) {
		t.Fatalf("retry did not reverse owned resources: %v %v", calls, err)
	}
	if err = j.PrepareSession(ctx, other); err != nil {
		t.Fatal("confirmed cleanup failed to release environment", err)
	}
	if err = j.PrepareSession(ctx, intent); !errors.Is(err, ErrNetworkJournalConflict) {
		t.Fatal("old session resurrected", err)
	}
}

func TestNetworkJournalRejectsChangedIntentAndLateCreation(t *testing.T) {
	ctx := context.Background()
	j, err := OpenNetworkJournal(filepath.Join(t.TempDir(), "resources.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer j.Close()
	intent := journalIntent()
	if err = j.PrepareSession(ctx, intent); err != nil {
		t.Fatal(err)
	}
	if err = j.PrepareSession(ctx, intent); err != nil {
		t.Fatal("same request should be recoverable", err)
	}
	changed := intent
	changed.PackageSID = "different-package"
	if err = j.PrepareSession(ctx, changed); !errors.Is(err, ErrNetworkJournalConflict) {
		t.Fatal("changed session accepted", err)
	}
	r := NetworkResourceIntent{ResourceID: "container", Kind: "container", ObjectIdentity: intent.PackageSID, Locator: intent.ContainerName}
	if err = j.prepareResource(ctx, intent.SessionID, r); err != nil {
		t.Fatal(err)
	}
	changedResource := r
	changedResource.ObjectIdentity = "replacement"
	if err = j.prepareResource(ctx, intent.SessionID, changedResource); !errors.Is(err, ErrNetworkJournalConflict) {
		t.Fatal("resource ownership changed", err)
	}
	if err = j.markApplied(ctx, intent.SessionID, r.ResourceID); err != nil {
		t.Fatal(err)
	}
	if err = j.prepareResource(ctx, intent.SessionID, r); !errors.Is(err, ErrNetworkJournalConflict) {
		t.Fatal("applied resource may be replayed", err)
	}
	if err = j.Cleanup(ctx, intent.SessionID, noJournalTestProcesses, func(_ context.Context, r NetworkResourceRecord) error {
		late := r.NetworkResourceIntent
		late.ResourceID = "late"
		if err := j.prepareResource(ctx, intent.SessionID, late); !errors.Is(err, ErrNetworkJournalConflict) {
			t.Fatal("late resource created while cleaning", err)
		}
		if err := j.markApplied(ctx, intent.SessionID, r.ResourceID); !errors.Is(err, ErrNetworkJournalConflict) {
			t.Fatal("late acknowledgement undid cleanup", err)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

func TestNetworkJournalWriteFailurePreventsResourceAction(t *testing.T) {
	ctx := context.Background()
	j, err := OpenNetworkJournal(filepath.Join(t.TempDir(), "resources.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer j.Close()
	intent := journalIntent()
	if err = j.PrepareSession(ctx, intent); err != nil {
		t.Fatal(err)
	}
	if _, err = j.db.Exec("PRAGMA query_only=ON"); err != nil {
		t.Fatal(err)
	}
	r := NetworkResourceIntent{ResourceID: "container", Kind: "container", ObjectIdentity: intent.PackageSID, Locator: intent.ContainerName}
	if err = j.prepareResource(ctx, intent.SessionID, r); err == nil {
		t.Fatal("write failure authorized an OS action")
	}
	pending, err := j.PendingSessions(ctx, "")
	if err != nil || len(pending) != 1 {
		t.Fatal("write failure lost prior durable ownership", err)
	}
}

func TestNetworkJournalCleanupAcknowledgementFailureReobservesActualResource(t *testing.T) {
	ctx := context.Background()
	j, err := OpenNetworkJournal(filepath.Join(t.TempDir(), "resources.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer j.Close()
	i := journalIntent()
	if err = j.PrepareSession(ctx, i); err != nil {
		t.Fatal(err)
	}
	r := NetworkResourceIntent{ResourceID: "container", Kind: "container", ObjectIdentity: i.PackageSID, Locator: i.ContainerName}
	if err = j.prepareResource(ctx, i.SessionID, r); err != nil {
		t.Fatal(err)
	}
	live := true
	err = j.Cleanup(ctx, i.SessionID, noJournalTestProcesses, func(context.Context, NetworkResourceRecord) error {
		live = false // external cleanup succeeds; its acknowledgement cannot write
		_, err := j.db.Exec("PRAGMA query_only=ON")
		return err
	})
	if err == nil || live {
		t.Fatal("cleanup acknowledgement failure hidden", err)
	}
	if _, err = j.db.Exec("PRAGMA query_only=OFF"); err != nil {
		t.Fatal(err)
	}
	calls := 0
	if err = j.Cleanup(ctx, i.SessionID, noJournalTestProcesses, func(_ context.Context, r NetworkResourceRecord) error {
		calls++
		if live || r.State != "prepared" {
			t.Fatal("retry must inspect the resource, not replay creation")
		}
		return nil
	}); err != nil || calls != 1 {
		t.Fatal("could not recover lost cleanup acknowledgement", err)
	}
	if err = j.Cleanup(ctx, i.SessionID, noJournalTestProcesses, func(context.Context, NetworkResourceRecord) error {
		t.Fatal("closed session replayed cleanup")
		return nil
	}); err != nil {
		t.Fatal("closed completion not idempotent", err)
	}
}

func TestNetworkJournalAbruptProcessExitRetainsPreparedIntent(t *testing.T) {
	root := t.TempDir()
	child := exec.Command(os.Args[0], "-test.run=^TestNetworkJournalCrashHelper$")
	child.Env = append(os.Environ(), "PRISM_JOURNAL_CRASH_ROOT="+root)
	output, err := child.CombinedOutput()
	var exit *exec.ExitError
	if !errors.As(err, &exit) || exit.ExitCode() != 77 {
		t.Fatalf("child failed before intended abrupt exit: %v %s", err, output)
	}
	j, err := OpenNetworkJournal(filepath.Join(root, "resources.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer j.Close()
	ctx := context.Background()
	pending, err := j.PendingSessions(ctx, "")
	if err != nil || len(pending) != 1 {
		t.Fatalf("crash lost durable ownership: %+v %v", pending, err)
	}
	if err = j.Cleanup(ctx, pending[0].SessionID, noJournalTestProcesses, func(_ context.Context, r NetworkResourceRecord) error {
		if r.State != "prepared" || r.ObjectIdentity != "synthetic-object" {
			t.Fatal("unexpected crash state", r)
		}
		// The file stands for an OS side effect, not an actual ACL or container.
		return os.Remove(filepath.Join(root, "synthetic-resource"))
	}); err != nil {
		t.Fatal(err)
	}
	if _, err = os.Stat(filepath.Join(root, "synthetic-resource")); !os.IsNotExist(err) {
		t.Fatal("synthetic resource was not recovered", err)
	}
}

func TestNetworkJournalCrashHelper(t *testing.T) {
	root := os.Getenv("PRISM_JOURNAL_CRASH_ROOT")
	if root == "" {
		t.Skip("only used by the isolated abrupt-exit parent test")
	}
	j, err := OpenNetworkJournal(filepath.Join(root, "resources.db"))
	if err != nil {
		t.Fatal(err)
	}
	i := journalIntent()
	if err = j.PrepareSession(context.Background(), i); err != nil {
		t.Fatal(err)
	}
	r := NetworkResourceIntent{ResourceID: "container", Kind: "container", ObjectIdentity: "synthetic-object", Locator: "synthetic-resource"}
	if err = j.prepareResource(context.Background(), i.SessionID, r); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(root, "synthetic-resource"), []byte("synthetic"), 0600); err != nil {
		t.Fatal(err)
	}
	os.Exit(77) // intentionally no Close, MarkApplied or deferred cleanup
}

func TestNetworkJournalCleanupWaitsForCreationWithoutBlockingAnotherSession(t *testing.T) {
	ctx := context.Background()
	j, err := OpenNetworkJournal(filepath.Join(t.TempDir(), "resources.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer j.Close()
	a, b := journalIntent(), journalIntent()
	for _, i := range []NetworkSessionIntent{a, b} {
		if err = j.PrepareSession(ctx, i); err != nil {
			t.Fatal(err)
		}
	}
	r := NetworkResourceIntent{ResourceID: "container", Kind: "container", ObjectIdentity: "synthetic-object", Locator: "synthetic-container"}
	entered, proceed := make(chan struct{}), make(chan struct{})
	defer func() {
		select {
		case <-proceed:
		default:
			close(proceed)
		}
	}()
	created, cleaned := make(chan error, 1), make(chan error, 1)
	live := false // synchronized by the journal's per-session action lock
	go func() {
		created <- j.ApplyResource(ctx, a.SessionID, r, func(context.Context) error {
			close(entered)
			<-proceed
			live = true
			return nil
		})
	}()
	<-entered
	go func() {
		cleaned <- j.Cleanup(ctx, a.SessionID, noJournalTestProcesses, func(_ context.Context, resource NetworkResourceRecord) error {
			if !live || resource.State != "applied" {
				return errors.New("cleanup overtook in-flight creation")
			}
			live = false
			return nil
		})
	}()
	deadline := time.Now().Add(5 * time.Second)
	for {
		j.mu.Lock()
		waiting := j.actions[a.SessionID] != nil && j.actions[a.SessionID].refs == 2
		j.mu.Unlock()
		if waiting {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("cleanup did not reach the lifecycle lock")
		}
		runtime.Gosched()
	}
	if err = j.ApplyResource(ctx, b.SessionID, r, func(context.Context) error { return nil }); err != nil {
		t.Fatal("A's in-flight creation blocked B", err)
	}
	close(proceed)
	if err = <-created; err != nil {
		t.Fatal(err)
	}
	if err = <-cleaned; err != nil {
		t.Fatal(err)
	}
	if err = j.ApplyResource(ctx, a.SessionID, r, func(context.Context) error {
		t.Error("late creation executed after cleanup")
		return nil
	}); !errors.Is(err, ErrNetworkJournalConflict) {
		t.Fatal("closed session allowed late creation", err)
	}
}

func TestNetworkJournalPartialCleanupReopenSkipsConfirmedResources(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "resources.db")
	j, err := OpenNetworkJournal(path)
	if err != nil {
		t.Fatal(err)
	}
	i := journalIntent()
	if err = j.PrepareSession(ctx, i); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"old", "new"} {
		r := NetworkResourceIntent{ResourceID: name, Kind: "container", ObjectIdentity: name, Locator: "synthetic-" + name}
		if err = j.ApplyResource(ctx, i.SessionID, r, func(context.Context) error { return nil }); err != nil {
			t.Fatal(err)
		}
	}
	blocked := errors.New("old resource cleanup failed")
	if err = j.Cleanup(ctx, i.SessionID, noJournalTestProcesses, func(_ context.Context, r NetworkResourceRecord) error {
		if r.ResourceID == "old" {
			return blocked
		}
		return nil
	}); !errors.Is(err, blocked) {
		t.Fatal(err)
	}
	if err = j.Close(); err != nil {
		t.Fatal(err)
	}
	j, err = OpenNetworkJournal(path)
	if err != nil {
		t.Fatal(err)
	}
	defer j.Close()
	var calls []string
	if err = j.Cleanup(ctx, i.SessionID, noJournalTestProcesses, func(_ context.Context, r NetworkResourceRecord) error {
		calls = append(calls, r.ResourceID)
		return nil
	}); err != nil || !reflect.DeepEqual(calls, []string{"old"}) {
		t.Fatalf("replayed confirmed cleanup: %v %v", calls, err)
	}
}

func TestNetworkJournalPagesEveryPendingSessionAndRejectsForeignFormat(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "resources.db")
	j, err := OpenNetworkJournal(path)
	if err != nil {
		t.Fatal(err)
	}
	defer j.Close()
	wanted := map[string]bool{}
	for n := 0; n < 103; n++ {
		i := journalIntent()
		wanted[i.SessionID] = true
		if err = j.PrepareSession(ctx, i); err != nil {
			t.Fatal(err)
		}
	}
	after := ""
	for {
		page, err := j.PendingSessions(ctx, after)
		if err != nil {
			t.Fatal(err)
		}
		if len(page) == 0 {
			break
		}
		if len(page) > 100 {
			t.Fatal("unbounded page")
		}
		for _, i := range page {
			if !wanted[i.SessionID] || i.SessionID <= after {
				t.Fatal("duplicated or nonmonotonic page")
			}
			delete(wanted, i.SessionID)
			after = i.SessionID
		}
	}
	if len(wanted) != 0 {
		t.Fatal("pending sessions omitted")
	}
	if _, err = j.db.Exec("PRAGMA user_version=999"); err != nil {
		t.Fatal(err)
	}
	if err = j.Close(); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if other, err := OpenNetworkJournal(path); err == nil {
		other.Close()
		t.Fatal("unknown future schema was accepted")
	}
	afterBytes, err := os.ReadFile(path)
	if err != nil || !reflect.DeepEqual(before, afterBytes) {
		t.Fatal("rejected journal was changed", err)
	}
}

func TestNetworkJournalUnconfirmedProcessTreePreservesAllResources(t *testing.T) {
	ctx := context.Background()
	j, err := OpenNetworkJournal(filepath.Join(t.TempDir(), "resources.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer j.Close()
	i := journalIntent()
	if err = j.PrepareSession(ctx, i); err != nil {
		t.Fatal(err)
	}
	r := NetworkResourceIntent{ResourceID: "container", Kind: "container", ObjectIdentity: "synthetic-object", Locator: "synthetic-container"}
	if err = j.ApplyResource(ctx, i.SessionID, r, func(context.Context) error { return nil }); err != nil {
		t.Fatal(err)
	}
	unknown := errors.New("exact Job query unavailable")
	err = j.Cleanup(ctx, i.SessionID, func(context.Context) error { return unknown }, func(context.Context, NetworkResourceRecord) error {
		t.Error("resources revoked before process tree was confirmed empty")
		return nil
	})
	if !errors.Is(err, unknown) {
		t.Fatal(err)
	}
	pending, err := j.PendingSessions(ctx, "")
	if err != nil || len(pending) != 1 {
		t.Fatal("unknown process tree released ownership", err)
	}
	r.ResourceID = "late"
	if err = j.ApplyResource(ctx, i.SessionID, r, func(context.Context) error {
		t.Error("creation proceeded after cleanup had sealed the session")
		return nil
	}); !errors.Is(err, ErrNetworkJournalConflict) {
		t.Fatal("sealed session accepted creation", err)
	}
}
