package workspace

import (
	"context"
	"time"

	"github.com/axgiroud312-byte/prism-local-browser/internal/backup"
	"github.com/axgiroud312-byte/prism-local-browser/internal/kernel"
	"github.com/axgiroud312-byte/prism-local-browser/internal/proxy"
)

type RestoreEnvironment struct {
	ID              string   `json:"id"`
	Name            string   `json:"name"`
	Seed            string   `json:"seed"`
	Action          string   `json:"action"`
	CurrentRevision int64    `json:"currentRevision"`
	BackupRevision  int64    `json:"backupRevision"`
	DataState       string   `json:"dataState"`
	Busy            bool     `json:"busy"`
	Conflicts       []string `json:"conflicts"`
}
type RestoreKernel struct {
	ID               string `json:"id"`
	Version          string `json:"version"`
	ArchiveSHA256    string `json:"archiveSha256"`
	ExecutableSHA256 string `json:"executableSha256"`
	LocalID          string `json:"localId"`
	State            string `json:"state"`
	Required         bool   `json:"required"`
}
type RestoreCredential struct {
	ProxyID string `json:"proxyId"`
	State   string `json:"state"` // none / available-current-user / reentry-required
}
type RestorePreview struct {
	Mode                   string              `json:"mode"`
	PreviewID              string              `json:"previewId"`
	Format                 string              `json:"format"`
	Name                   string              `json:"name"`
	ArchiveSHA256          string              `json:"archiveSha256"`
	ManifestSHA256         string              `json:"manifestSha256"`
	Scope                  string              `json:"scope"`
	CreatedAt              string              `json:"createdAt"`
	ExpiresAt              string              `json:"expiresAt"`
	EnvironmentCount       int                 `json:"environmentCount"`
	AddCount               int                 `json:"addCount"`
	OverwriteCount         int                 `json:"overwriteCount"`
	ConflictCount          int                 `json:"conflictCount"`
	MissingKernelCount     int                 `json:"missingKernelCount"`
	CredentialReentryCount int                 `json:"credentialReentryCount"`
	Bytes                  int64               `json:"bytes"`
	CanRestore             bool                `json:"canRestore"`
	Kernels                []RestoreKernel     `json:"kernels"`
	Credentials            []RestoreCredential `json:"credentials"`
}
type restoreSource struct {
	path           string
	expires        time.Time
	expectedSHA256 string // host-owned migration backup; never a caller path
	scratch        string // exact owned preflight directory whose cleanup failed
}
type restoreEnvironmentData struct {
	manifest    backup.Environment
	environment Environment
	code        int
	dataState   string
	history     []ProfileRevision
}
type restoreProxyData struct {
	ID, Ref         string
	Configuration   proxy.Configuration
	Revision        int64
	Protected       []byte
	CredentialState string
}
type restoreData struct {
	environments []restoreEnvironmentData
	proxies      []restoreProxyData
	kernels      map[string]kernel.Record
}
type restoreDraft struct {
	preview                     RestorePreview
	impacts                     []RestoreEnvironment
	data                        restoreData
	manifest                    backup.Manifest
	path, sourceToken, baseline string
	expires                     time.Time
	kernelMapping               map[string]string
	kernelCandidates            map[string][]string
}
type restorePreflight struct {
	id     string
	cancel context.CancelFunc
}
