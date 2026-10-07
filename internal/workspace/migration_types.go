package workspace

import (
	"context"
	"time"

	"github.com/axgiroud312-byte/prism-local-browser/internal/backup"
	"github.com/axgiroud312-byte/prism-local-browser/internal/kernel"
	"github.com/axgiroud312-byte/prism-local-browser/internal/proxy"
)

type MigrationPreview struct {
	Mode               string              `json:"mode"`
	PreviewID          string              `json:"previewId"`
	EnvironmentID      string              `json:"environmentId"`
	Name               string              `json:"name"`
	ExpectedRevision   int64               `json:"expectedRevision"`
	Before             DeviceProfile       `json:"before"`
	After              DeviceProfile       `json:"after"`
	BeforeCapabilities []kernel.Capability `json:"beforeCapabilities"`
	AfterCapabilities  []kernel.Capability `json:"afterCapabilities"`
	Changes            []ProfileChange     `json:"changes"`
	ExpiresAt          string              `json:"expiresAt"`
}
type MigrationRequest struct {
	PreviewID string `json:"previewId"`
	Confirm   bool   `json:"confirm"`
	RequestID string `json:"requestId"`
}
type MigrationAction struct {
	OperationID string `json:"operationId"`
	Action      string `json:"action"`
	Confirm     bool   `json:"confirm"`
}
type MigrationReport struct {
	Mode           string                       `json:"mode"`
	RequestID      string                       `json:"requestId"`
	PreviewID      string                       `json:"previewId"`
	EnvironmentID  string                       `json:"environmentId"`
	OldKernelID    string                       `json:"oldKernelId"`
	NewKernelID    string                       `json:"newKernelId"`
	Seed           string                       `json:"seed"`
	Sequence       int64                        `json:"sequence"`
	BackupVerified bool                         `json:"backupVerified"`
	ArchiveSHA256  string                       `json:"archiveSha256"`
	TrialExited    bool                         `json:"trialExited"`
	Committed      bool                         `json:"committed"`
	Protected      bool                         `json:"protected"`
	Before         *kernel.MigrationObservation `json:"before,omitempty"`
	After          *kernel.MigrationObservation `json:"after,omitempty"`
	ProxyReport    *proxy.Report                `json:"proxyReport,omitempty"`
}
type migrationPlan struct {
	Version           int                 `json:"version"`
	ID                string              `json:"id"`
	Preview           MigrationPreview    `json:"preview"`
	Environment       Environment         `json:"environment"`
	FingerprintID     string              `json:"fingerprintId"`
	CreatedAt         string              `json:"createdAt"`
	Target            backupTarget        `json:"target"`
	Baseline          string              `json:"baseline"`
	CommittedBaseline string              `json:"committedBaseline"`
	WorkID            string              `json:"workId"`
	WorkIdentity      backup.TreeIdentity `json:"workIdentity"`
	Move              restoreMove         `json:"move"`
	BackupSHA256      string              `json:"backupSha256"`
	BackupVerified    bool                `json:"backupVerified"`
	SessionID         string              `json:"sessionId"`
	LaunchPermitted   bool                `json:"launchPermitted"`
	LaunchNoProcess   bool                `json:"launchNoProcess"`
	PID               int                 `json:"pid"`
	ProcessCreatedAt  string              `json:"processCreatedAt"`
	TrialExited       bool                `json:"trialExited"`
	Prepared          bool                `json:"prepared"`
	Committed         bool                `json:"committed"`
}
type migrationDraft struct {
	plan    migrationPlan
	expires time.Time
}
type migrationTask struct {
	plan              migrationPlan
	operation         Operation
	phase             string
	running           bool
	cancel            context.CancelFunc
	process           RuntimeProcess
	stop              chan struct{}
	acceptancePending bool
	startup           bool
	bootstrapReady    bool
	bootstrapLoader   *Service
	bootstrapStep     int
	finalPending      *Operation
}

func copyMigrationOperation(op Operation) Operation {
	if op.MigrationReport != nil {
		r := *op.MigrationReport
		op.MigrationReport = &r
	}
	return op
}
func migrationBackupRef(id string) string {
	return "backups/migrations/" + id + "/before" + backup.Extension
}
func migrationStageRef(id string) string { return "backups/migrations/" + id }
