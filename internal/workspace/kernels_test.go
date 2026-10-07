package workspace

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/axgiroud312-byte/prism-local-browser/internal/kernel"
	"golang.org/x/sys/windows"
)

func syntheticKernelPrepare(ctx context.Context, root string, input kernel.InstallInput, path string, _ kernel.ProbeFunc, progress kernel.ProgressFunc) (*kernel.Prepared, error) {
	progress("synthetic-test-seam")
	staging := filepath.Join(root, "staging", "kernel-"+input.ResourceKey)
	payload := filepath.Join(staging, "payload")
	if err := os.MkdirAll(payload, 0700); err != nil {
		return nil, err
	}
	bytes := []byte("synthetic kernel, never executed")
	if err := os.WriteFile(filepath.Join(payload, "chrome.exe"), bytes, 0600); err != nil {
		return nil, err
	}
	digest := sha256.Sum256(bytes)
	hash := hex.EncodeToString(digest[:])
	kernelID := id()
	// Deliberately synthetic evidence through a test-only host seam. Nothing here
	// is executed or used as public evidence of a real build's capabilities.
	report := kernel.Report{AdapterVersion: kernel.AdapterVersion, Version: kernel.CapabilityVersion, Transport: "synthetic-test-only", Observations: []kernel.Observation{{BrowserVersion: input.Version, HTTPClientHints: map[string]string{}, Brands: []kernel.Brand{}, FullVersionList: []kernel.Brand{}, Languages: []string{}}}, Capabilities: []kernel.Capability{}}
	for _, field := range []string{"identity", "cpu", "acceptLanguages", "timezone"} {
		report.Capabilities = append(report.Capabilities, kernel.Capability{Field: field, Status: "configurable", Source: "observed", Note: "synthetic-test-only"})
	}
	report.Capabilities = append(report.Capabilities, kernel.Capability{Field: "seed", Status: "seed-generated", Source: "observed", Note: "synthetic-test-only"}, kernel.Capability{Field: "uiLanguage", Status: "unverified", Source: "not-probed", Note: "synthetic-test-only"})
	return &kernel.Prepared{Directory: payload, Staging: staging, Record: kernel.Record{ID: kernelID, Version: input.Version, Architecture: "amd64", ArchiveSHA256: input.ExpectedChecksum, ExecutableSHA256: hash, ExecutableRelativePath: "chrome.exe", InstallPath: "kernels/" + kernelID, Source: kernel.Source{Kind: "local", Location: filepath.Base(path), Tag: input.Version}, Files: map[string]string{"chrome.exe": hash}, InstalledAt: timestamp(), Report: report}}, nil
}
func syntheticArchive(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "synthetic.zip")
	if err := os.WriteFile(path, []byte("synthetic"), 0600); err != nil {
		t.Fatal(err)
	}
	return path
}
func installKernel(t *testing.T, s *Service) (Operation, kernel.InstallInput) {
	t.Helper()
	selection := value[struct {
		ArchiveToken string `json:"archiveToken"`
	}](t, call(s, "Kernel.SelectArchive", struct{}{}))
	input := kernel.InstallInput{Source: "local", Version: "148.0.7778.215", ExpectedChecksum: strings.Repeat("a", 64), ArchiveToken: selection.ArchiveToken, Trusted: true, RequestID: id()}
	accepted := value[struct {
		Operation Operation `json:"operation"`
	}](t, call(s, "Kernel.Install", input))
	return accepted.Operation, input
}
func waitKernel(t *testing.T, s *Service, operationID string) Operation {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		operation := value[Operation](t, call(s, "Operation.Read", map[string]string{"operationId": operationID}))
		if operation.State == "completed" || operation.State == "failed" || operation.State == "cancelled" {
			return operation
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("kernel operation did not reach a terminal state")
	return Operation{}
}

func TestV1MigrationPreservesSavedIdentityAndUnknownVersions(t *testing.T) {
	s, root := fixture(t, Options{})
	saved, _ := create(t, s, "迁移保持身份")
	stripFingerprintSchema(t, s)
	for _, statement := range []string{"DROP TRIGGER immutable_kernel_evidence", "DROP TABLE kernel_evidence", "PRAGMA user_version=1"} {
		if _, err := s.db.Exec(statement); err != nil {
			t.Fatal(err)
		}
	}
	s.Close()
	reopened, err := Open(root, Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	if !reflect.DeepEqual(saved, view(t, reopened).State.Environments[0]) {
		t.Fatal("v1 migration changed fixed identity/configuration")
	}
	var version int
	reopened.db.QueryRow("PRAGMA user_version").Scan(&version)
	if version != 10 {
		t.Fatal("schema migration not committed")
	}
}

func TestKernelInstallIsImmutableIdempotentAndReferenceProtected(t *testing.T) {
	archive := syntheticArchive(t)
	options := Options{ChooseArchive: func() (string, error) { return archive, nil }, PrepareKernel: syntheticKernelPrepare}
	s, root := fixture(t, options)
	accepted, input := installKernel(t, s)
	if accepted.State != "accepted" {
		t.Fatal("acceptance falsely claimed completed")
	}
	completed := waitKernel(t, s, accepted.ID)
	if completed.State != "completed" || len(completed.CompletedIDs) != 1 {
		t.Fatalf("install failed: %+v", completed)
	}
	first := view(t, s).KernelRecords[0]
	if first.Source.Location != filepath.Base(archive) || first.ArchiveSHA256 != input.ExpectedChecksum || first.Status != "verified" {
		t.Fatal("precise evidence missing")
	}
	var duplicate struct {
		Operation Operation `json:"operation"`
	}
	duplicate = value[struct {
		Operation Operation `json:"operation"`
	}](t, call(s, "Kernel.Install", input))
	if duplicate.Operation.ID != accepted.ID || len(view(t, s).KernelRecords) != 1 {
		t.Fatal("idempotent install produced another ID")
	}
	if _, err := s.db.Exec("UPDATE kernel_evidence SET record_json='{}' WHERE kernel_id=?", first.ID); err == nil {
		t.Fatal("immutable evidence overwritten in place")
	}
	p := generateFingerprint(t, s, preview(t, s, "create", ""), first.ID, false)
	p.Environment.Name = "固定内核引用"
	value[map[string]any](t, call(s, "Environment.Create", Mutation{PreviewID: p.PreviewID, Configuration: p.Environment.Configuration, Count: 1, RequestID: id()}))
	wantError(t, call(s, "Kernel.Delete", map[string]string{"kernelId": first.ID, "requestId": id()}), "PROFILE_BUSY")
	if err := kernel.VerifyFiles(filepath.Join(root, "kernels", first.ID), first.Files); err != nil {
		t.Fatal(err)
	}
	s.Close()
	reopened, err := Open(root, options)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	if view(t, reopened).State.Environments[0].CoreID != first.ID || view(t, reopened).KernelRecords[0].ID != first.ID {
		t.Fatal("reopen changed exact kernel reference")
	}
	if len(view(t, reopened).KernelRecords[0].UsedBy) != 1 {
		t.Fatal("affected environments not shown")
	}
}

func TestKernelCancelAndCloseDoNotBlockOtherWorkspaceCalls(t *testing.T) {
	started := make(chan struct{})
	archive := syntheticArchive(t)
	s, _ := fixture(t, Options{ChooseArchive: func() (string, error) { return archive, nil }, PrepareKernel: func(ctx context.Context, _ string, _ kernel.InstallInput, _ string, _ kernel.ProbeFunc, progress kernel.ProgressFunc) (*kernel.Prepared, error) {
		progress("waiting")
		close(started)
		<-ctx.Done()
		return nil, ctx.Err()
	}})
	accepted, _ := installKernel(t, s)
	<-started
	if len(view(t, s).State.Environments) != 0 {
		t.Fatal("long task contaminated workspace")
	}
	value[Operation](t, call(s, "Operation.Cancel", map[string]string{"operationId": accepted.ID}))
	if terminal := waitKernel(t, s, accepted.ID); terminal.State != "cancelled" || !terminal.CancelRequested {
		t.Fatalf("cancel failed: %+v", terminal)
	}
	if len(view(t, s).KernelRecords) != 0 {
		t.Fatal("cancelled task installed a half kernel")
	}
}

func TestKernelCommitFailureAndTamperingPreserveTrustedEvidence(t *testing.T) {
	var fail atomic.Bool
	archive := syntheticArchive(t)
	s, root := fixture(t, Options{ChooseArchive: func() (string, error) { return archive, nil }, PrepareKernel: syntheticKernelPrepare, BeforeCommit: func() error {
		if fail.Load() {
			return errors.New("SYNTHETIC_SECRET/private-path")
		}
		return nil
	}})
	first, _ := installKernel(t, s)
	if waitKernel(t, s, first.ID).State != "completed" {
		t.Fatal("fixture install failed")
	}
	record := view(t, s).KernelRecords[0]
	fail.Store(true)
	second, _ := installKernel(t, s)
	if terminal := waitKernel(t, s, second.ID); terminal.State != "failed" {
		t.Fatal("failed commit falsely completed")
	}
	if records := view(t, s).KernelRecords; len(records) != 1 || records[0].ExecutableSHA256 != record.ExecutableSHA256 {
		t.Fatal("failed install changed old kernel")
	}
	if err := kernel.VerifyFiles(filepath.Join(root, "kernels", record.ID), record.Files); err != nil {
		t.Fatal(err)
	}
	fail.Store(false)
	os.WriteFile(filepath.Join(root, "kernels", record.ID, "chrome.exe"), []byte("tampered"), 0600)
	verify := value[struct {
		Operation Operation `json:"operation"`
	}](t, call(s, "Kernel.Verify", map[string]string{"kernelId": record.ID, "requestId": id()}))
	terminal := waitKernel(t, s, verify.Operation.ID)
	if terminal.State != "failed" || terminal.Error.Code != "KERNEL_INTEGRITY_FAILED" || terminal.Error.Details["reason"] != "hash-mismatch" {
		t.Fatalf("tamper was not blocked: %+v", terminal)
	}
	after := view(t, s).KernelRecords[0]
	if after.Status != "missing" || after.ExecutableSHA256 != record.ExecutableSHA256 {
		t.Fatal("tamper was blessed with a new hash")
	}
	staging, _ := os.ReadDir(filepath.Join(root, "staging"))
	if len(staging) != 0 {
		t.Fatal("failed staging not cleaned")
	}
}

func TestKernelInterruptedPublicationCleansOnlyOwnedResources(t *testing.T) {
	s, root := fixture(t, Options{})
	operation := Operation{ID: id(), Kind: "kernel-install", State: "running", Stage: "publishing", KernelID: id(), ResourceKey: id(), CompletedIDs: []string{}}
	bytes, _ := json.Marshal(operation)
	s.db.Exec("INSERT INTO operations(id,result_json) VALUES(?,?)", operation.ID, string(bytes))
	owned := filepath.Join(root, "staging", "kernel-"+operation.ResourceKey)
	unregistered := filepath.Join(root, "kernels", operation.KernelID)
	unknown := filepath.Join(root, "staging", "unrelated")
	for _, path := range []string{owned, unregistered, unknown} {
		os.MkdirAll(path, 0700)
		os.WriteFile(filepath.Join(path, "synthetic.txt"), []byte("retain unknown"), 0600)
	}
	s.Close()
	reopened, err := Open(root, Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	for _, path := range []string{owned, unregistered} {
		if _, err = os.Stat(path); !os.IsNotExist(err) {
			t.Fatal("uncommitted owned resource remained")
		}
	}
	if _, err = os.Stat(filepath.Join(unknown, "synthetic.txt")); err != nil {
		t.Fatal("recovery removed unrelated directory")
	}
	if terminal := waitKernel(t, reopened, operation.ID); terminal.State != "failed" || terminal.Stage != "interrupted" {
		t.Fatal("interrupted operation misreported")
	}
}

func TestFailedRollbackCleanupIsRetriedAfterReopen(t *testing.T) {
	archive := syntheticArchive(t)
	var root string
	var blocked windows.Handle
	s, fixtureRoot := fixture(t, Options{ChooseArchive: func() (string, error) { return archive, nil }, PrepareKernel: syntheticKernelPrepare, BeforeCommit: func() error {
		entries, err := os.ReadDir(filepath.Join(root, "kernels"))
		if err != nil {
			return err
		}
		wide, _ := windows.UTF16PtrFromString(filepath.Join(root, "kernels", entries[0].Name(), "chrome.exe"))
		blocked, err = windows.CreateFile(wide, windows.GENERIC_READ, windows.FILE_SHARE_READ, nil, windows.OPEN_EXISTING, 0, 0)
		if err != nil {
			return err
		}
		return errors.New("synthetic commit failure with deletion sharing denied")
	}})
	root = fixtureRoot
	accepted, _ := installKernel(t, s)
	terminal := waitKernel(t, s, accepted.ID)
	if terminal.State != "failed" || len(terminal.CompletedIDs) != 0 {
		t.Fatal("rolled-back task falsely lists committed kernels")
	}
	if blocked == 0 {
		t.Fatal("deletion-blocking fixture was not acquired")
	}
	windows.CloseHandle(blocked)
	if entries, _ := os.ReadDir(filepath.Join(root, "kernels")); len(entries) != 1 {
		t.Fatal("fixture did not leave the blocked rollback directory")
	}
	s.Close()
	reopened, err := Open(root, Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	if entries, _ := os.ReadDir(filepath.Join(root, "kernels")); len(entries) != 0 {
		t.Fatal("failed terminal operation lost cleanup responsibility")
	}
}

func TestIncompleteOrCancelledReverificationKeepsHealthyKernel(t *testing.T) {
	for _, cancelled := range []bool{false, true} {
		t.Run(map[bool]string{false: "incomplete", true: "cancelled"}[cancelled], func(t *testing.T) {
			archive := syntheticArchive(t)
			started := make(chan struct{})
			s, _ := fixture(t, Options{ChooseArchive: func() (string, error) { return archive, nil }, PrepareKernel: syntheticKernelPrepare, VerifyKernel: func(ctx context.Context, _ string, _ kernel.Record, _ string) (kernel.Report, error) {
				close(started)
				if cancelled {
					<-ctx.Done()
				}
				// Model a pipe-read error racing cancellation, not an integrity contradiction.
				return kernel.Report{}, &kernel.Problem{Code: "PROCESS_READY_TIMEOUT", Reason: "diagnostic-not-ready", Message: "synthetic incomplete diagnostic", Retryable: true}
			}})
			accepted, _ := installKernel(t, s)
			if waitKernel(t, s, accepted.ID).State != "completed" {
				t.Fatal("synthetic fixture installation failed")
			}
			record := view(t, s).KernelRecords[0]
			verify := value[struct {
				Operation Operation `json:"operation"`
			}](t, call(s, "Kernel.Verify", map[string]string{"kernelId": record.ID, "requestId": id()}))
			<-started
			if cancelled {
				call(s, "Operation.Cancel", map[string]string{"operationId": verify.Operation.ID})
			}
			terminal := waitKernel(t, s, verify.Operation.ID)
			if cancelled && terminal.State != "cancelled" {
				t.Fatal("cancellation hidden by a diagnostic wrapper")
			}
			if !cancelled && terminal.State != "failed" {
				t.Fatal("incomplete check falsely completed")
			}
			if after := view(t, s).KernelRecords[0]; after.Status != "verified" || after.ExecutableSHA256 != record.ExecutableSHA256 {
				t.Fatal("incomplete check invalidated an untouched healthy build")
			}
		})
	}
}

func TestConfirmedReparsePointIsIsolatedWithoutExecution(t *testing.T) {
	archive := syntheticArchive(t)
	s, root := fixture(t, Options{ChooseArchive: func() (string, error) { return archive, nil }, PrepareKernel: syntheticKernelPrepare})
	accepted, _ := installKernel(t, s)
	if waitKernel(t, s, accepted.ID).State != "completed" {
		t.Fatal("fixture install failed")
	}
	record := view(t, s).KernelRecords[0]
	directory := filepath.Join(root, "kernels", record.ID)
	if err := os.Rename(directory, directory+"-retained"); err != nil {
		t.Fatal(err)
	}
	outside := t.TempDir()
	if output, err := exec.Command("cmd.exe", "/d", "/c", "mklink", "/J", directory, outside).CombinedOutput(); err != nil {
		t.Fatalf("junction fixture: %v %s", err, output)
	}
	t.Cleanup(func() { os.Remove(directory) })
	verify := value[struct {
		Operation Operation `json:"operation"`
	}](t, call(s, "Kernel.Verify", map[string]string{"kernelId": record.ID, "requestId": id()}))
	terminal := waitKernel(t, s, verify.Operation.ID)
	if terminal.State != "failed" || terminal.Error.Code != "PATH_OUTSIDE_ROOT" || view(t, s).KernelRecords[0].Status != "missing" {
		t.Fatalf("confirmed unsafe boundary remained available: %+v", terminal)
	}
}

func TestConfirmedIntegrityFailureSurvivesCleanupFailureOrCancel(t *testing.T) {
	for _, cancelled := range []bool{false, true} {
		t.Run(map[bool]string{false: "cleanup-fails", true: "cancel-races"}[cancelled], func(t *testing.T) {
			archive := syntheticArchive(t)
			started := make(chan struct{})
			var held windows.Handle
			s, _ := fixture(t, Options{ChooseArchive: func() (string, error) { return archive, nil }, PrepareKernel: syntheticKernelPrepare, VerifyKernel: func(ctx context.Context, _ string, _ kernel.Record, staging string) (kernel.Report, error) {
				if !cancelled {
					file := filepath.Join(staging, "synthetic-delete-block.txt")
					os.WriteFile(file, []byte("synthetic"), 0600)
					wide, _ := windows.UTF16PtrFromString(file)
					var err error
					held, err = windows.CreateFile(wide, windows.GENERIC_READ, windows.FILE_SHARE_READ, nil, windows.OPEN_EXISTING, 0, 0)
					if err != nil {
						return kernel.Report{}, err
					}
				}
				close(started)
				if cancelled {
					<-ctx.Done()
				}
				return kernel.Report{}, &kernel.Problem{Code: "KERNEL_INTEGRITY_FAILED", Reason: "hash-mismatch", Message: "synthetic confirmed mismatch", Retryable: true}
			}})
			accepted, _ := installKernel(t, s)
			if waitKernel(t, s, accepted.ID).State != "completed" {
				t.Fatal("fixture install failed")
			}
			record := view(t, s).KernelRecords[0]
			verify := value[struct {
				Operation Operation `json:"operation"`
			}](t, call(s, "Kernel.Verify", map[string]string{"kernelId": record.ID, "requestId": id()}))
			<-started
			if cancelled {
				call(s, "Operation.Cancel", map[string]string{"operationId": verify.Operation.ID})
			}
			terminal := waitKernel(t, s, verify.Operation.ID)
			if held != 0 {
				windows.CloseHandle(held)
			}
			if terminal.Error == nil || terminal.Error.Details["integrityFailureConfirmed"] != true || view(t, s).KernelRecords[0].Status != "missing" {
				t.Fatal("later cleanup/cancel withdrew a confirmed integrity failure")
			}
		})
	}
}
