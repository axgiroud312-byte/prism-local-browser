//go:build windows

// Package kernel installs immutable, exact Windows builds and performs isolated
// diagnostics and managed native sessions with isolated persistent data.
package kernel

import (
	"context"
	"fmt"
)

const AdapterVersion = "windows-kernel-v1"
const CapabilityVersion = "fingerprint-capabilities-v1"

type Problem struct {
	Code      string
	Reason    string
	Message   string
	Retryable bool
}

func (p *Problem) Error() string { return p.Message }
func problem(code, reason, message string) error {
	return &Problem{Code: code, Reason: reason, Message: message, Retryable: true}
}

type Source struct {
	Kind     string  `json:"kind"`
	Location string  `json:"location"`
	Tag      string  `json:"tag"`
	Commit   *string `json:"commit"`
}
type Observation struct {
	Seed             int               `json:"seed"`
	BrowserVersion   string            `json:"browserVersion"`
	HTTPUserAgent    string            `json:"httpUserAgent"`
	HTTPClientHints  map[string]string `json:"httpClientHints"`
	UserAgent        string            `json:"userAgent"`
	Platform         string            `json:"platform"`
	Brands           []Brand           `json:"brands"`
	FullVersionList  []Brand           `json:"fullVersionList"`
	PlatformVersion  string            `json:"platformVersion"`
	Language         string            `json:"language"`
	Languages        []string          `json:"languages"`
	AcceptLanguage   string            `json:"acceptLanguage"`
	Timezone         string            `json:"timezone"`
	CPU              int               `json:"cpu"`
	Memory           float64           `json:"memory"`
	GPUVendor        string            `json:"gpuVendor"`
	GPURenderer      string            `json:"gpuRenderer"`
	PID              uint32            `json:"pid"`
	ProcessCreatedAt string            `json:"processCreatedAt"`
	NormalExit       bool              `json:"normalExit"`
}
type Brand struct {
	Brand   string `json:"brand"`
	Version string `json:"version"`
}
type Capability struct {
	Field  string `json:"field"`
	Status string `json:"status"`
	Source string `json:"source"`
	Note   string `json:"note"`
}
type Report struct {
	AdapterVersion string        `json:"adapterVersion"`
	Version        string        `json:"version"`
	SampledAt      string        `json:"sampledAt"`
	Transport      string        `json:"transport"`
	Sandbox        bool          `json:"sandbox"`
	Observations   []Observation `json:"observations"`
	Capabilities   []Capability  `json:"capabilities"`
}
type Record struct {
	ID                     string            `json:"id"`
	Version                string            `json:"version"`
	Architecture           string            `json:"architecture"`
	Source                 Source            `json:"source"`
	ArchiveSHA256          string            `json:"archiveSha256"`
	ExecutableSHA256       string            `json:"executableSha256"`
	ExecutableRelativePath string            `json:"executableRelativePath"`
	InstallPath            string            `json:"installPath"`
	Files                  map[string]string `json:"files"`
	Report                 Report            `json:"report"`
	InstalledAt            string            `json:"installedAt"`
}
type InstallInput struct {
	Source           string `json:"source"`
	Version          string `json:"version"`
	ExpectedChecksum string `json:"expectedChecksum"`
	ArchiveToken     string `json:"archiveToken,omitempty"`
	Trusted          bool   `json:"trusted"`
	RequestID        string `json:"requestId"`
	// Internal staging owner token, never accepted from RPC JSON.
	ResourceKey string `json:"-"`
}
type ProbeFunc func(context.Context, string, string, string) (Report, error)
type ProgressFunc func(stage string)

func validateRecord(r Record) error {
	if r.ID == "" || !versionPattern.MatchString(r.Version) || r.Architecture != "amd64" || !digestPattern.MatchString(r.ArchiveSHA256) || !digestPattern.MatchString(r.ExecutableSHA256) || r.Files[r.ExecutableRelativePath] != r.ExecutableSHA256 || r.Report.AdapterVersion != AdapterVersion || r.Report.Version != CapabilityVersion || len(r.Report.Observations) == 0 {
		return fmt.Errorf("invalid kernel evidence")
	}
	return nil
}

func CheckRecord(r Record) error {
	if err := validateRecord(r); err != nil {
		return err
	}
	if _, err := RecordDirectory(".", r); err != nil {
		return err
	}
	if validArchiveName(r.ExecutableRelativePath) != nil || r.Source.Tag != r.Version || (r.Source.Kind != "official" && r.Source.Kind != "local") {
		return fmt.Errorf("invalid kernel source/path evidence")
	}
	for name, digest := range r.Files {
		if validArchiveName(name) != nil || !digestPattern.MatchString(digest) {
			return fmt.Errorf("invalid kernel manifest")
		}
	}
	return nil
}
