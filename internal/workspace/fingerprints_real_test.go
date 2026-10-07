//go:build windows

package workspace

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/axgiroud312-byte/prism-local-browser/internal/kernel"
)

// Explicit fixture selection is required. This never clicks or starts the app UI.
func TestRealSavedFingerprintRevisions(t *testing.T) {
	archive := os.Getenv("PRISM_KERNEL_ARCHIVE")
	if archive == "" {
		t.Skip("real archive not explicitly selected")
	}
	s, root := fixture(t, Options{ChooseArchive: func() (string, error) { return archive, nil }})
	selected := value[struct {
		ArchiveToken string `json:"archiveToken"`
	}](t, call(s, "Kernel.SelectArchive", struct{}{}))
	accepted := value[struct {
		Operation Operation `json:"operation"`
	}](t, call(s, "Kernel.Install", kernel.InstallInput{Source: "local", Version: "148.0.7778.215", ExpectedChecksum: "9ef3f471b7a6641b4224532522b29141ce3746e27d55788d88e2fd951f362579", ArchiveToken: selected.ArchiveToken, Trusted: true, RequestID: id()}))
	deadline := time.Now().Add(2 * time.Minute)
	var operation Operation
	for time.Now().Before(deadline) {
		operation = value[Operation](t, call(s, "Operation.Read", map[string]string{"operationId": accepted.Operation.ID}))
		if operation.State == "completed" || operation.State == "failed" || operation.State == "cancelled" {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if operation.State != "completed" {
		t.Fatalf("real install not completed: %+v / %+v", operation, operation.Error)
	}
	kernelID := operation.KernelID
	record := view(t, s).KernelRecords[0].Record
	p := preview(t, s, "create", "")
	p.Environment.Language, p.Environment.Timezone, p.Environment.CPU = "de-DE", "Europe/Berlin", "8"
	p = generateFingerprint(t, s, p, kernelID, false)
	p.Environment.Name = "合成真实固定档案"
	value[map[string]any](t, call(s, "Environment.Create", Mutation{PreviewID: p.PreviewID, Configuration: p.Environment.Configuration, ProfileHash: p.Fingerprint.PreviewProfile.ConfigHash, Count: 1, RequestID: id()}))
	e := view(t, s).State.Environments[0]
	ref := view(t, s).DataReferences[e.ID]
	sentinel := filepath.Join(root, filepath.FromSlash(ref), "synthetic-data-sentinel")
	if err := os.MkdirAll(filepath.Dir(sentinel), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(sentinel, []byte("synthetic-data-unchanged"), 0600); err != nil {
		t.Fatal(err)
	}
	staging := filepath.Join(root, "staging", "saved-profile-diagnostic")
	if err := os.Mkdir(staging, 0700); err != nil {
		t.Fatal(err)
	}
	profiles := []DeviceProfile{}
	observations := []kernel.Observation{}
	sample := func(service *Service) {
		profile := view(t, service).Fingerprints[e.ID].Profile
		observation, err := kernel.ProbeFingerprint(context.Background(), root, record, profileInput(profile), staging)
		if err != nil {
			t.Fatal(err)
		}
		profiles = append(profiles, profile)
		observations = append(observations, observation)
		t.Logf("saved revision %d, seed %s, PID %d, actual %s, CPU %d, language %s, timezone %s, normal exit %t", profile.ConfigRevision, profile.Seed, observation.PID, observation.BrowserVersion, observation.CPU, observation.Language, observation.Timezone, observation.NormalExit)
	}
	sample(s)
	p = generateFingerprint(t, s, preview(t, s, "edit", e.ID), kernelID, true)
	value[map[string]any](t, commitFingerprint(t, s, p, id()))
	sample(s)
	p = preview(t, s, "edit", e.ID)
	p = value[Preview](t, call(s, "Fingerprint.PreviewRestore", map[string]any{"previewId": p.PreviewID, "revision": 1}))
	value[map[string]any](t, commitFingerprint(t, s, p, id()))
	beforeReopen := view(t, s)
	s.Close()
	reopened, err := Open(root, Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	if !reflect.DeepEqual(beforeReopen, view(t, reopened)) {
		t.Fatal("reopen changed frozen inputs")
	}
	sample(reopened)
	if profiles[0].Seed != profiles[2].Seed || !reflect.DeepEqual(profiles[0].Parameters, profiles[2].Parameters) || profiles[1].Seed == profiles[0].Seed {
		t.Fatal("restore/reopen did not reuse original inputs")
	}
	content, err := os.ReadFile(sentinel)
	if err != nil || string(content) != "synthetic-data-unchanged" || view(t, reopened).DataReferences[e.ID] != ref {
		t.Fatal("profile change altered synthetic browser data")
	}
	entries, err := os.ReadDir(staging)
	if err != nil || len(entries) != 0 {
		t.Fatal("real diagnostic left profile directories")
	}
	if destination := os.Getenv("PRISM_PROFILE_EVIDENCE"); destination != "" {
		evidence := map[string]any{"verifiedAt": timestamp(), "mode": "native", "platform": "windows/amd64", "schemaVersion": 3, "uiClicks": "not-run", "kernelId": kernelID, "kernelVersion": record.Version, "archiveSha256": record.ArchiveSHA256, "executableSha256": record.ExecutableSHA256, "environmentId": e.ID, "profiles": profiles, "observations": observations, "sameDataReference": true, "syntheticDataUnchanged": true, "sameInputsAfterRestoreAndReopen": true, "diagnosticDirectoriesCleaned": true, "transport": "inherited-private-pipe", "sandbox": true, "boundary": "diagnostic temporary profiles only; normal environment launch, real Cookie storage and UI flow are not verified"}
		encoded, err := json.MarshalIndent(evidence, "", "  ")
		if err != nil {
			t.Fatal(err)
		}
		if err = os.WriteFile(destination, append(encoded, '\n'), 0600); err != nil {
			t.Fatal(err)
		}
	}
}
