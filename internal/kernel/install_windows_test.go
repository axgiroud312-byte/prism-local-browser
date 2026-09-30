//go:build windows

package kernel

import (
	"archive/zip"
	"context"
	"encoding/binary"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type roundTrip func(*http.Request) (*http.Response, error)

func (f roundTrip) RoundTrip(request *http.Request) (*http.Response, error) { return f(request) }

func TestExactOfficialAssetNeverFallsBackAndChecksMetadataDigest(t *testing.T) {
	requested := []string{}
	client := &http.Client{Transport: roundTrip(func(request *http.Request) (*http.Response, error) {
		requested = append(requested, request.URL.Path)
		return &http.Response{StatusCode: 404, Body: io.NopCloser(strings.NewReader("{}")), Header: http.Header{}}, nil
	})}
	input := InstallInput{Source: "official", Version: "150.0.7871.186", ExpectedChecksum: strings.Repeat("a", 64), RequestID: "synthetic"}
	_, err := officialSource(context.Background(), input, client)
	var p *Problem
	if !errors.As(err, &p) || p.Code != "KERNEL_MISSING" || p.Reason != "asset-unavailable" {
		t.Fatalf("missing asset reported incorrectly: %v", err)
	}
	if len(requested) != 1 || strings.Contains(requested[0], "148.") {
		t.Fatal("unavailable exact request fell back")
	}
	client.Transport = roundTrip(func(request *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"tag_name":"150.0.7871.186","assets":[{"name":"ungoogled-chromium_150.0.7871.186-1.1_windows_x64.zip","size":100,"digest":"sha256:wrong","browser_download_url":"https://github.com/adryfish/fingerprint-chromium/releases/download/150.0.7871.186/ungoogled-chromium_150.0.7871.186-1.1_windows_x64.zip"}]}`)), Header: http.Header{}}, nil
	})
	_, err = officialSource(context.Background(), input, client)
	if !errors.As(err, &p) || p.Code != "KERNEL_INTEGRITY_FAILED" || p.Reason != "hash-mismatch" {
		t.Fatalf("digest mismatch reported incorrectly: %v", err)
	}
}

func TestActualArchiveHashMismatchNeverExtractsOrExecutes(t *testing.T) {
	root := t.TempDir()
	called := false
	_, err := Prepare(context.Background(), root, InstallInput{Source: "local", Version: "148.0.7778.215", ExpectedChecksum: strings.Repeat("a", 64), ArchiveToken: "synthetic-picker-token", Trusted: true, RequestID: "synthetic"}, zipFixture(t, "chrome.exe"), func(context.Context, string, string, string) (Report, error) { called = true; return Report{}, nil }, nil)
	var p *Problem
	if !errors.As(err, &p) || p.Reason != "hash-mismatch" || called {
		t.Fatal("bad archive digest was not blocked before execution")
	}
	entries, _ := os.ReadDir(filepath.Join(root, "staging"))
	if len(entries) != 0 {
		t.Fatal("failed preparation left staged bytes")
	}
}

func TestIdentityMismatchDoesNotAcceptSpoofedVersion(t *testing.T) {
	sample := Observation{BrowserVersion: "148.0.7778.215", UserAgent: "Mozilla Chrome/148.0.0.0", HTTPUserAgent: "Mozilla Chrome/148.0.0.0", Platform: "Windows", PlatformVersion: "15.0.0", Brands: []Brand{{"Google Chrome", "148"}}, FullVersionList: []Brand{{"Google Chrome", "148.0.7778.215"}}, HTTPClientHints: map[string]string{"sec-ch-ua-full-version-list": `"Google Chrome";v="148.0.7778.215"`, "sec-ch-ua-platform": `"Windows"`, "sec-ch-ua-platform-version": `"15.0.0"`}}
	sample.HTTPClientHints["sec-ch-ua"] = `"Not)A;Brand";v="99", "Google Chrome";v="148"`
	if err := checkIdentity(sample, "148.0.7778.215"); err != nil {
		t.Fatal(err)
	}
	for _, invalid := range []struct{ header, value string }{{"sec-ch-ua", `"Google Chrome";v="147"`}, {"sec-ch-ua-full-version-list", `"Not-Chrome";v="148.0.7778.215"`}, {"sec-ch-ua-full-version-list", `"Google Chrome";v="147.0.0.1", "GREASE";v="148.0.7778.215"`}, {"sec-ch-ua-platform", `"Not-Windows"`}} {
		previous := sample.HTTPClientHints[invalid.header]
		sample.HTTPClientHints[invalid.header] = invalid.value
		if err := checkIdentity(sample, "148.0.7778.215"); err == nil {
			t.Fatalf("mismatched HTTP Client Hint accepted: %s", invalid.value)
		}
		sample.HTTPClientHints[invalid.header] = previous
	}
	sample.BrowserVersion = "147.0.0.1"
	var p *Problem
	if err := checkIdentity(sample, "148.0.7778.215"); !errors.As(err, &p) || p.Reason != "identity-mismatch" {
		t.Fatal("UA-only version accepted")
	}
}

func unknownPEArchive(t *testing.T) (string, InstallInput) {
	t.Helper()
	bytes := make([]byte, 0x98)
	copy(bytes, "MZ")
	binary.LittleEndian.PutUint32(bytes[0x3c:], 0x80)
	copy(bytes[0x80:], "PE\x00\x00")
	binary.LittleEndian.PutUint16(bytes[0x84:], 0x8664)
	path := filepath.Join(t.TempDir(), "synthetic-unknown-pe.zip")
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	w := zip.NewWriter(file)
	entry, _ := w.Create("package/chrome.exe")
	entry.Write(bytes)
	w.Close()
	file.Close()
	hash, _ := fileHash(path)
	return path, InstallInput{Source: "local", Version: "148.0.7778.215", ExpectedChecksum: hash, ArchiveToken: "synthetic-picker", Trusted: true, RequestID: "synthetic-version", ResourceKey: "88888888-1111-4111-8111-888888888888"}
}

func TestUnknownActualPEVersionFailsBeforeProbe(t *testing.T) {
	archive, input := unknownPEArchive(t)
	called := false
	_, err := Prepare(context.Background(), t.TempDir(), input, archive, func(context.Context, string, string, string) (Report, error) { called = true; return Report{}, nil }, nil)
	var p *Problem
	if !errors.As(err, &p) || p.Reason != "version-unknown" || called {
		t.Fatalf("unknown PE version did not fail distinctly before execution: %v", err)
	}
}

func TestPreparationPinsStageArchiveAndChecksExtractedBytesBeforeExecution(t *testing.T) {
	archive, input := unknownPEArchive(t)
	root := t.TempDir()
	stage := filepath.Join(root, "staging", "kernel-"+input.ResourceKey)
	stageProtected, archiveProtected, called := false, false, false
	_, err := Prepare(context.Background(), root, input, archive, func(context.Context, string, string, string) (Report, error) { called = true; return Report{}, nil }, func(step string) {
		switch step {
		case "acquiring-archive":
			stageProtected = os.Rename(stage, stage+"-renamed") != nil
		case "extracting":
			archiveProtected = os.WriteFile(filepath.Join(stage, "archive.zip"), []byte("replacement"), 0600) != nil
		case "verifying-extracted-files":
			if err := os.WriteFile(filepath.Join(stage, "payload", "package", "chrome.exe"), []byte("modified after extraction"), 0600); err != nil {
				t.Fatal(err)
			}
		}
	})
	var p *Problem
	if !stageProtected || !archiveProtected || called || !errors.As(err, &p) || p.Reason != "hash-mismatch" {
		t.Fatalf("unsafe preparation boundary: stage=%t archive=%t probe=%t err=%v", stageProtected, archiveProtected, called, err)
	}
	entries, _ := os.ReadDir(filepath.Join(root, "staging"))
	if len(entries) != 0 {
		t.Fatal("failed preparation not cleaned")
	}
}

func TestProbeCancellationNormalizationPreservesConfirmedIntegrity(t *testing.T) {
	confirmed := checkIdentity(Observation{}, "148.0.7778.215")
	joined := preserveProbeFailure(confirmed, context.Canceled)
	var p *Problem
	if !errors.Is(joined, context.Canceled) || !errors.As(joined, &p) || p.Code != "KERNEL_INTEGRITY_FAILED" {
		t.Fatal("inner probe cancellation normalization erased a confirmed unsafe identity")
	}
}
