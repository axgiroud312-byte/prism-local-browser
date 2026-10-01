package workspace

import (
	"context"
	"time"

	"github.com/axgiroud312-byte/prism-local-browser/internal/backup"
)

type BackupExportRequest struct {
	Scope            string   `json:"scope"`
	EnvironmentIDs   []string `json:"environmentIds"`
	DestinationToken string   `json:"destinationToken"`
	StopRunning      bool     `json:"stopRunning"`
	RequestID        string   `json:"requestId"`
}
type BackupReport struct {
	RequestID              string `json:"requestId"`
	Mode                   string `json:"mode"`
	Format                 string `json:"format"`
	SchemaVersion          int    `json:"schemaVersion"`
	Scope                  string `json:"scope"`
	EnvironmentCount       int    `json:"environmentCount"`
	CopiedEnvironmentCount int    `json:"copiedEnvironmentCount"`
	FileCount              int64  `json:"fileCount"`
	ByteCount              int64  `json:"byteCount"`
	Sequence               int64  `json:"sequence"`
	Published              bool   `json:"published"`
	Name                   string `json:"name"`
	ArchiveSHA256          string `json:"archiveSha256,omitempty"`
	ManifestSHA256         string `json:"manifestSha256,omitempty"`
	Credentials            string `json:"credentials"`
	BrowserData            string `json:"browserData"`
	KernelBinariesIncluded bool   `json:"kernelBinariesIncluded"`
}
type BackupRecord struct {
	ID               string `json:"id"`
	OperationID      string `json:"operationId"`
	Name             string `json:"name"`
	CreatedAt        string `json:"createdAt"`
	Scope            string `json:"scope"`
	EnvironmentCount int    `json:"environmentCount"`
	ArchiveSHA256    string `json:"archiveSha256"`
	ManifestSHA256   string `json:"manifestSha256"`
}
type backupDestination struct {
	path    string
	expires time.Time
}
type backupTarget struct {
	ID                string `json:"id"`
	Reference         string `json:"reference"`
	NeverUsed         bool   `json:"neverUsed"`
	DirectoryRequired bool   `json:"directoryRequired"`
}
type backupTask struct {
	operation                                                      Operation
	targets                                                        []backupTarget
	destination, temporary, createdAt, requestID, signature        string
	cancel                                                         context.CancelFunc
	started, acceptancePending, finalPending, publishing, recovery bool
	final                                                          Operation
}

// Values read by the worker outside s.mu never alias the mutable observation.
type backupExecution struct {
	OperationID, Scope, CreatedAt, Destination string
	Targets                                    []backupTarget
}

func copyBackupOperation(operation Operation) Operation {
	if operation.BackupReport != nil {
		report := *operation.BackupReport
		operation.BackupReport = &report
	}
	return operation
}
func backupReport(scope, name string, total int) *BackupReport {
	return &BackupReport{Mode: "native", Format: backup.Format, SchemaVersion: backup.Version, Scope: scope, EnvironmentCount: total, Name: name, Sequence: 1, Credentials: "windows-current-user-dpapi", BrowserData: "sensitive-same-user-not-portable"}
}
