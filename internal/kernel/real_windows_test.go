//go:build windows

package kernel

import (
	"context"
	"encoding/json"
	"os"
	"testing"
	"time"
)

// Explicit opt-in only; ordinary tests do not download or execute a kernel.
func TestRealFingerprintChromiumArchive(t *testing.T) {
	archive := os.Getenv("PRISM_KERNEL_ARCHIVE")
	if archive == "" {
		t.Skip("real archive not explicitly selected")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	prepared, err := Prepare(ctx, t.TempDir(), InstallInput{Source: "local", Version: "148.0.7778.215", ExpectedChecksum: "9ef3f471b7a6641b4224532522b29141ce3746e27d55788d88e2fd951f362579", ArchiveToken: "explicit-test-selection", Trusted: true, RequestID: "synthetic-real-probe"}, archive, nil, func(stage string) { t.Log(stage) })
	if err != nil {
		t.Fatal(err)
	}
	if err = VerifyFiles(prepared.Directory, prepared.Record.Files); err != nil {
		t.Fatal(err)
	}
	if destination := os.Getenv("PRISM_KERNEL_EVIDENCE"); destination != "" {
		bytes, _ := json.MarshalIndent(prepared.Record, "", "  ")
		if err = os.WriteFile(destination, bytes, 0600); err != nil {
			t.Fatal(err)
		}
	}
	if len(prepared.Record.Report.Observations) != 3 {
		t.Fatal("incomplete real probe evidence")
	}
	for _, sample := range prepared.Record.Report.Observations {
		t.Logf("PID %d, actual %s, seed %d, CPU %d, language %s, timezone %s, normal exit %t", sample.PID, sample.BrowserVersion, sample.Seed, sample.CPU, sample.Language, sample.Timezone, sample.NormalExit)
	}
}
