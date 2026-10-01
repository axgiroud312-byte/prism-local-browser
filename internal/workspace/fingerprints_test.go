package workspace

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/axgiroud312-byte/prism-local-browser/internal/kernel"
)

func fingerprintFixture(t *testing.T, options Options) (*Service, string, string) {
	t.Helper()
	archive := syntheticArchive(t)
	options.ChooseArchive = func() (string, error) { return archive, nil }
	options.PrepareKernel = syntheticKernelPrepare
	s, root := fixture(t, options)
	op, _ := installKernel(t, s)
	completed := waitKernel(t, s, op.ID)
	if completed.State != "completed" {
		t.Fatalf("synthetic fixture install: %+v", completed)
	}
	return s, root, completed.KernelID
}

func stripFingerprintSchema(t *testing.T, s *Service) {
	t.Helper()
	// Convert only this synthetic fixture to the actual v2 shape; merely lowering
	// user_version on a v3 schema would not be a valid migration regression.
	for _, statement := range []string{"DROP TRIGGER immutable_fingerprint_revision", "DROP TABLE fingerprint_revisions", "ALTER TABLE fingerprints DROP COLUMN config_revision", "ALTER TABLE environments DROP COLUMN user_data_ref", "PRAGMA user_version=2"} {
		if _, err := s.db.Exec(statement); err != nil {
			t.Fatal(err)
		}
	}
}

func TestFingerprintV2MigrationPreservesSeedAndExistingExpandedInputs(t *testing.T) {
	s, root := fixture(t, Options{})
	e, _ := create(t, s, "合成旧v2档案")
	stripFingerprintSchema(t, s)
	s.Close()
	reopened, err := Open(root, Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	actual := view(t, reopened)
	if !reflect.DeepEqual(e, actual.State.Environments[0]) || actual.Fingerprints[e.ID].Profile.Seed != e.Seed || actual.Fingerprints[e.ID].Action != "migrated" || actual.Fingerprints[e.ID].Profile.GeneratorVersion != "native-initial-v1" {
		t.Fatal("migration regenerated or relabelled the old identity")
	}
	if len(actual.Fingerprints[e.ID].Profile.Parameters) != 0 {
		t.Fatal("pending migrated kernel became launchable")
	}
}
func generateFingerprint(t *testing.T, s *Service, p Preview, kernelID string, regenerate bool) Preview {
	t.Helper()
	c := p.Environment.Configuration
	return value[Preview](t, call(s, "Fingerprint.Generate", GenerateFingerprint{PreviewID: p.PreviewID, KernelID: kernelID, TemplateID: "windows-desktop-v1", Overrides: FingerprintOverrides{Language: c.Language, Timezone: c.Timezone, CPU: c.CPU, Width: c.Width, Height: c.Height}, Regenerate: regenerate}))
}
func createFingerprint(t *testing.T, s *Service, kernelID string) Environment {
	t.Helper()
	p := generateFingerprint(t, s, preview(t, s, "create", ""), kernelID, false)
	p.Environment.Name = "合成固定档案"
	value[map[string]any](t, call(s, "Environment.Create", Mutation{PreviewID: p.PreviewID, Configuration: p.Environment.Configuration, Count: 1, RequestID: id(), ProfileHash: p.Fingerprint.PreviewProfile.ConfigHash}))
	return view(t, s).State.Environments[0]
}
func commitFingerprint(t *testing.T, s *Service, p Preview, requestID string) Result {
	t.Helper()
	return call(s, "Fingerprint.CommitRevision", Mutation{PreviewID: p.PreviewID, EnvironmentID: p.Environment.ID, Configuration: p.Environment.Configuration, ExpectedRevision: p.ExpectedRevision, RequestID: requestID, ProfileHash: p.Fingerprint.PreviewProfile.ConfigHash})
}

func TestFingerprintGenerateIsReadOnlyAndUsesExactCapabilities(t *testing.T) {
	s, _, kernelID := fingerprintFixture(t, Options{})
	before := view(t, s)
	p := generateFingerprint(t, s, preview(t, s, "create", ""), kernelID, false)
	profile := p.Fingerprint.PreviewProfile
	if profile.Seed == "" || profile.GeneratorVersion != FingerprintGeneratorVersion || profile.CoreActualVersion != "148.0.7778.215" || profile.CoreExecutableSHA256 == "" || profile.CapabilityVersion != kernel.CapabilityVersion || profile.UILanguage != "system" {
		t.Fatalf("incomplete frozen profile: %+v", profile)
	}
	if p.Fingerprint.CapabilityReport.ObservedFingerprint != nil || p.Fingerprint.CapabilityReport.CanLaunchNative {
		t.Fatal("preview invented a live environment observation")
	}
	for _, arg := range profile.Parameters {
		if arg == "--lang=en-US" || arg == "--fingerprint-screen-width=1280" {
			t.Fatal("unverified UI/screen capability compiled")
		}
	}
	if !reflect.DeepEqual(before, view(t, s)) {
		t.Fatal("Generate wrote saved data")
	}
	repeated := generateFingerprint(t, s, p, kernelID, false)
	if !reflect.DeepEqual(profile, repeated.Fingerprint.PreviewProfile) {
		t.Fatal("ordinary preview changed frozen inputs")
	}
	value[map[string]string](t, call(s, "Preview.Discard", map[string]string{"previewId": p.PreviewID}))
	if !reflect.DeepEqual(before, view(t, s)) {
		t.Fatal("cancel wrote saved data")
	}
}

func TestFingerprintRevisionRoundTripAndSameKernelRestorePreserveData(t *testing.T) {
	s, root, kernelID := fingerprintFixture(t, Options{})
	e := createFingerprint(t, s, kernelID)
	initial := view(t, s)
	ref := initial.DataReferences[e.ID]
	if ref == "" {
		t.Fatal("missing independent data reference")
	}
	path := filepath.Join(root, filepath.FromSlash(ref), "synthetic-cookie-sentinel")
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("synthetic login data, not a real cookie"), 0600); err != nil {
		t.Fatal(err)
	}
	p := generateFingerprint(t, s, preview(t, s, "edit", e.ID), kernelID, true)
	if p.Fingerprint.PreviewProfile.Seed == e.Seed || len(p.Fingerprint.Changes) == 0 {
		t.Fatal("regeneration has no explicit change preview")
	}
	value[map[string]any](t, commitFingerprint(t, s, p, id()))
	after := view(t, s)
	if after.Fingerprints[e.ID].Profile.ConfigRevision != 2 || after.DataReferences[e.ID] != ref {
		t.Fatal("revision/data reference not committed atomically")
	}
	// Rollback restores only device inputs, not current names or proxy metadata.
	edit := preview(t, s, "edit", e.ID)
	edit.Environment.Name = "合成保留新名称"
	value[map[string]any](t, call(s, "Environment.Update", Mutation{PreviewID: edit.PreviewID, Configuration: edit.Environment.Configuration, ExpectedRevision: edit.ExpectedRevision, RequestID: id()}))
	edit = preview(t, s, "edit", e.ID)
	restored := value[Preview](t, call(s, "Fingerprint.PreviewRestore", map[string]any{"previewId": edit.PreviewID, "revision": 1}))
	if restored.Fingerprint.Action != "restore" || restored.Fingerprint.PreviewProfile.Seed != e.Seed || restored.Environment.Name != "合成保留新名称" {
		t.Fatal("restore did not preview original identity independently of metadata")
	}
	value[map[string]any](t, commitFingerprint(t, s, restored, id()))
	final := view(t, s)
	if final.Fingerprints[e.ID].Profile.ConfigRevision != 3 || final.Fingerprints[e.ID].Profile.Seed != e.Seed || final.Fingerprints[e.ID].RestoredFrom != 1 || final.DataReferences[e.ID] != ref {
		t.Fatal("restore did not append a new monotonic revision")
	}
	content, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(content, []byte("synthetic login data, not a real cookie")) {
		t.Fatal("profile commit altered browser data")
	}
	s.Close()
	reopened, err := Open(root, Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	if !reflect.DeepEqual(final, view(t, reopened)) {
		t.Fatal("reopen regenerated identity/evidence/data reference")
	}
}

func TestFingerprintCommitCannotBypassPreviewConflictOrBusyLease(t *testing.T) {
	s, _, kernelID := fingerprintFixture(t, Options{})
	e := createFingerprint(t, s, kernelID)
	before := view(t, s)
	p := preview(t, s, "edit", e.ID)
	forged := p.Environment.Configuration
	forged.CPU = "16"
	wantError(t, call(s, "Environment.Update", Mutation{PreviewID: p.PreviewID, Configuration: forged, ExpectedRevision: p.ExpectedRevision, RequestID: id()}), "VALIDATION_FAILED")
	release, err := s.AcquireProfileUse(e.ID)
	if err != nil {
		t.Fatal(err)
	}
	wantError(t, commitFingerprint(t, s, p, id()), "PROFILE_BUSY")
	release()
	first := generateFingerprint(t, s, p, kernelID, true)
	stale := generateFingerprint(t, s, preview(t, s, "edit", e.ID), kernelID, true)
	value[map[string]any](t, commitFingerprint(t, s, first, id()))
	wantError(t, commitFingerprint(t, s, stale, id()), "REVISION_CONFLICT")
	if before.Fingerprints[e.ID].Profile.Seed == view(t, s).Fingerprints[e.ID].Profile.Seed {
		t.Fatal("committed regeneration was lost")
	}
}

func TestFingerprintTransactionFailureAndHashForgeryKeepOldHistory(t *testing.T) {
	fail := false
	s, _, kernelID := fingerprintFixture(t, Options{BeforeCommit: func() error {
		if fail {
			return errors.New("SYNTHETIC_SECRET")
		}
		return nil
	}})
	e := createFingerprint(t, s, kernelID)
	before := view(t, s)
	p := generateFingerprint(t, s, preview(t, s, "edit", e.ID), kernelID, true)
	forged := Mutation{PreviewID: p.PreviewID, EnvironmentID: e.ID, ExpectedRevision: p.ExpectedRevision, Configuration: p.Environment.Configuration, ProfileHash: "forged", RequestID: id()}
	wantError(t, call(s, "Fingerprint.CommitRevision", forged), "VALIDATION_FAILED")
	fail = true
	requestID := id()
	wantError(t, commitFingerprint(t, s, p, requestID), "STORAGE_WRITE_FAILED")
	if !reflect.DeepEqual(before, view(t, s)) {
		t.Fatal("failed commit changed current profile or activity")
	}
	history := value[[]ProfileRevision](t, call(s, "Fingerprint.ListRevisions", map[string]string{"environmentId": e.ID}))
	if len(history) != 1 {
		t.Fatal("failed commit left history rows")
	}
	fail = false
	value[map[string]any](t, commitFingerprint(t, s, p, requestID))
	value[map[string]any](t, commitFingerprint(t, s, p, requestID))
	if history = value[[]ProfileRevision](t, call(s, "Fingerprint.ListRevisions", map[string]string{"environmentId": e.ID})); len(history) != 2 {
		t.Fatal("idempotent commit added duplicate history")
	}
}

func TestFingerprintCannotSwitchKernelOrAcceptUnverifiedFields(t *testing.T) {
	s, _, kernelID := fingerprintFixture(t, Options{})
	e := createFingerprint(t, s, kernelID)
	op, _ := installKernel(t, s)
	other := waitKernel(t, s, op.ID).KernelID
	p := preview(t, s, "edit", e.ID)
	c := p.Environment.Configuration
	request := GenerateFingerprint{PreviewID: p.PreviewID, KernelID: other, TemplateID: c.FingerprintVersion, Overrides: FingerprintOverrides{Language: c.Language, Timezone: c.Timezone, CPU: c.CPU, Width: c.Width, Height: c.Height}}
	wantError(t, call(s, "Fingerprint.Generate", request), "CAPABILITY_UNSUPPORTED")
	encoded, _ := json.Marshal(request)
	var raw map[string]any
	json.Unmarshal(encoded, &raw)
	raw["kernelId"] = kernelID
	raw["overrides"].(map[string]any)["gpuVendor"] = "invented"
	wantError(t, call(s, "Fingerprint.Generate", raw), "VALIDATION_FAILED")
}

func TestFingerprintTimezoneMustBeExplicitAndFrozen(t *testing.T) {
	s, _, kernelID := fingerprintFixture(t, Options{})
	p := preview(t, s, "create", "")
	before := view(t, s)
	for _, timezone := range []string{"", "Local"} {
		input := GenerateFingerprint{PreviewID: p.PreviewID, KernelID: kernelID, TemplateID: "windows-desktop-v1", Overrides: FingerprintOverrides{Language: "en-US", Timezone: timezone, CPU: "auto", Width: 1280, Height: 800}}
		wantError(t, call(s, "Fingerprint.Generate", input), "VALIDATION_FAILED")
		c := p.Environment.Configuration
		c.Name, c.Timezone = "合成非法时区", timezone
		wantError(t, call(s, "Environment.Create", Mutation{PreviewID: p.PreviewID, Configuration: c, Count: 1, RequestID: id()}), "VALIDATION_FAILED")
	}
	if !reflect.DeepEqual(before, view(t, s)) {
		t.Fatal("invalid timezone modified saved data")
	}
}
