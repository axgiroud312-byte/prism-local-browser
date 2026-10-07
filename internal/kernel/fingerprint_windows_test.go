//go:build windows

package kernel

import (
	"errors"
	"reflect"
	"strings"
	"testing"
)

func compilerRecord() Record {
	kernelID := "11111111-2222-4333-8444-555555555555"
	return Record{ID: kernelID, Version: "148.0.7778.215", Architecture: "amd64", Source: Source{Kind: "local", Tag: "148.0.7778.215"}, ArchiveSHA256: strings.Repeat("a", 64), ExecutableSHA256: strings.Repeat("b", 64), ExecutableRelativePath: "chrome.exe", InstallPath: "kernels/" + kernelID, Files: map[string]string{"chrome.exe": strings.Repeat("b", 64)}, Report: Report{AdapterVersion: AdapterVersion, Version: CapabilityVersion, Transport: "synthetic-test-only", Observations: []Observation{{BrowserVersion: "148.0.7778.215"}}, Capabilities: probeCapabilities("148.0.7778.215")}}
}

func TestFingerprintCompilerFiltersUnverifiedAndGroupsIdentityAndLocale(t *testing.T) {
	record := compilerRecord()
	input := FingerprintInput{Seed: "1256789", Language: "de-DE", Timezone: "Europe/Berlin", CPU: "8"}
	args, err := CompileFingerprint(record, input)
	if err != nil {
		t.Fatal(err)
	}
	wanted := []string{"--fingerprint=1256789", "--fingerprint-platform=windows", "--fingerprint-platform-version=15.0.0", "--fingerprint-brand=Chrome", "--fingerprint-brand-version=148.0.7778.215", "--accept-lang=de-DE,de,en", "--timezone=Europe/Berlin", "--fingerprint-hardware-concurrency=8"}
	if !reflect.DeepEqual(args, wanted) {
		t.Fatalf("wrong canonical args: %v", args)
	}
	for _, forbidden := range []string{"--lang=", "--fingerprint-screen", "--fingerprint-gpu", "--disable-spoofing", "--user-data-dir", "--no-sandbox", "--remote-debugging-port"} {
		if strings.Contains(strings.Join(args, " "), forbidden) {
			t.Fatalf("compiled unverified/host-only field: %s", forbidden)
		}
	}
	input.CPU = "auto"
	args, err = CompileFingerprint(record, input)
	if err != nil || len(args) != 7 {
		t.Fatal("auto CPU was converted into a made-up explicit output")
	}
}

func TestFingerprintCompilerRejectsMissingCapabilityAndInvalidInputs(t *testing.T) {
	input := FingerprintInput{Seed: "123", Language: "en-US", Timezone: "America/New_York", CPU: "8"}
	for _, change := range []func(*FingerprintInput){func(i *FingerprintInput) { i.Seed = "00123" }, func(i *FingerprintInput) { i.Language = "en-US,--no-sandbox" }, func(i *FingerprintInput) { i.CPU = "128" }, func(i *FingerprintInput) { i.Timezone = "not/a/timezone" }} {
		candidate := input
		change(&candidate)
		var p *Problem
		if _, err := CompileFingerprint(compilerRecord(), candidate); !errors.As(err, &p) || p.Code != "VALIDATION_FAILED" {
			t.Fatalf("bad input accepted: %+v %v", candidate, err)
		}
	}
	for _, timezone := range []string{"", "Local"} {
		candidate := input
		candidate.Timezone = timezone
		if _, err := CompileFingerprint(compilerRecord(), candidate); err == nil {
			t.Fatalf("unfrozen timezone accepted: %q", timezone)
		}
	}
	record := compilerRecord()
	for index := range record.Report.Capabilities {
		if record.Report.Capabilities[index].Field == "cpu" {
			record.Report.Capabilities[index].Status = "unverified"
		}
	}
	if _, err := CompileFingerprint(record, input); err == nil {
		t.Fatal("unverified CPU override was compiled")
	}
	input.CPU = "auto"
	if _, err := CompileFingerprint(record, input); err != nil {
		t.Fatal("unsupported explicit CPU also blocked safe auto")
	}
	record.Report.Capabilities = nil
	if _, err := CompileFingerprint(record, input); err == nil {
		t.Fatal("version string substituted for required capability evidence")
	}
}
