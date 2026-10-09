package workspace

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/axgiroud312-byte/prism-local-browser/internal/kernel"
	"golang.org/x/sys/windows"
)

func bundledTestArchive(t *testing.T) string {
	t.Helper()
	archive := filepath.Join(t.TempDir(), BundledKernelArchive)
	if err := os.WriteFile(archive, []byte("synthetic bundle, never executed"), 0600); err != nil {
		t.Fatal(err)
	}
	return archive
}

func TestBundledKernelFirstUseAndReopenReuseExactBuild(t *testing.T) {
	archive := bundledTestArchive(t)
	options := Options{PrepareKernel: syntheticKernelPrepare}
	s, root := fixture(t, options)
	if err := s.PrepareBundledKernel(context.Background(), archive); err != nil {
		t.Fatal(err)
	}
	first, err := s.readKernelDefault()
	if err != nil || first.KernelID == "kernel-pending" {
		t.Fatalf("bundle not selected for first use: %+v %v", first, err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := Open(root, options)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	// A registered exact build works even if the original bundle is moved.
	if err := reopened.PrepareBundledKernel(context.Background(), filepath.Join(t.TempDir(), BundledKernelArchive)); err != nil {
		t.Fatal(err)
	}
	current, err := reopened.readKernelDefault()
	records, listErr := reopened.listKernels()
	operations, opErr := reopened.listKernelOperations()
	if err != nil || listErr != nil || opErr != nil || current != first || len(records) != 1 || len(operations) != 1 {
		t.Fatalf("reopen installed or selected a new identity: %+v records=%d operations=%d", current, len(records), len(operations))
	}
	// A damaged registered build must not silently install a replacement ID.
	executable := filepath.Join(root, records[0].InstallPath, "chrome.exe")
	if err := os.WriteFile(executable, []byte("changed bytes"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := reopened.PrepareBundledKernel(context.Background(), archive); err == nil {
		t.Fatal("damaged existing bundle was accepted")
	}
	records, _ = reopened.listKernels()
	current, _ = reopened.readKernelDefault()
	if len(records) != 1 || current != first {
		t.Fatal("damage changed the saved default or created a replacement")
	}
}

func TestBundledKernelPreservesExplicitDefault(t *testing.T) {
	archive := bundledTestArchive(t)
	s, _ := fixture(t, Options{ChooseArchive: func() (string, error) { return archive, nil }, PrepareKernel: syntheticKernelPrepare})
	accepted, _ := installKernel(t, s)
	installed := waitKernel(t, s, accepted.ID)
	prior, _ := s.readKernelDefault()
	value[KernelDefault](t, call(s, "Kernel.SetDefault", KernelDefaultRequest{KernelID: installed.KernelID, ExpectedRevision: prior.Revision, RequestID: id()}))
	selected, _ := s.readKernelDefault()
	if err := s.PrepareBundledKernel(context.Background(), archive); err != nil {
		t.Fatal(err)
	}
	current, _ := s.readKernelDefault()
	if current != selected {
		t.Fatal("bundle overwrote an explicit default")
	}
}

func TestBundledKernelWrongArchiveNeverPublished(t *testing.T) {
	s, _ := fixture(t, Options{})
	err := s.PrepareBundledKernel(context.Background(), bundledTestArchive(t))
	if err == nil {
		t.Fatal("modified archive accepted")
	}
	records, _ := s.listKernels()
	current, _ := s.readKernelDefault()
	operations, _ := s.listKernelOperations()
	if len(records) != 0 || current.KernelID != "kernel-pending" || len(operations) != 1 || operations[0].State != "failed" {
		t.Fatal("wrong archive was published or the failure was not retained")
	}
}

// Explicit opt-in exercises the same host preparation used by the desktop,
// including its worker and publication, without touching the user's workspace.
func TestRealBundledKernelFirstUse(t *testing.T) {
	archive := os.Getenv("PRISM_KERNEL_ARCHIVE")
	if archive == "" {
		t.Skip("real archive not explicitly selected")
	}
	// Temp is exempt from MSIX AppData virtualization. Use an owned LocalAppData
	// directory to catch manifest lookup failures inherited from packaged hosts.
	local, err := windows.KnownFolderPath(windows.FOLDERID_LocalAppData, 0)
	if err != nil {
		t.Fatal(err)
	}
	root, err := os.MkdirTemp(local, "PrismBundledKernelTest-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := kernel.RemoveOwnedTree(root); err != nil {
			t.Error(err)
		}
	})
	s, err := Open(root, Options{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	if err := s.PrepareBundledKernel(ctx, archive); err != nil {
		if failure, ok := err.(*Error); ok {
			t.Fatalf("%s: %s; details=%v", failure.Code, failure.Message, failure.Details)
		}
		t.Fatal(err)
	}
	current, err := s.readKernelDefault()
	records, listErr := s.listKernels()
	if err != nil || listErr != nil || len(records) != 1 || current.KernelID != records[0].ID || records[0].Status != "verified" || records[0].Version != BundledKernelVersion {
		t.Fatalf("real bundle not ready: default=%+v records=%d errors=%v %v", current, len(records), err, listErr)
	}
	t.Logf("verified default %s, version %s, observations %d", current.KernelID, records[0].Version, len(records[0].Report.Observations))
}
