package workspace

import (
	"context"

	"github.com/axgiroud312-byte/prism-local-browser/internal/backup"
	"github.com/axgiroud312-byte/prism-local-browser/internal/proxy"
)

type RestoreRequest struct {
	PreviewID              string `json:"previewId"`
	ArchiveSHA256          string `json:"archiveSha256"`
	ConfirmOverwrite       bool   `json:"confirmOverwrite"`
	AcknowledgeCredentials bool   `json:"acknowledgeCredentials"`
	StopRunning            bool   `json:"stopRunning"`
	RequestID              string `json:"requestId"`
}
type RestoreReport struct {
	Mode                   string `json:"mode"`
	RequestID              string `json:"requestId"`
	PreviewID              string `json:"previewId"`
	ArchiveSHA256          string `json:"archiveSha256"`
	Sequence               int64  `json:"sequence"`
	EnvironmentCount       int    `json:"environmentCount"`
	SwitchedCount          int    `json:"switchedCount"`
	Committed              bool   `json:"committed"`
	RolledBack             bool   `json:"rolledBack"`
	Protected              bool   `json:"protected"`
	CredentialReentryCount int    `json:"credentialReentryCount"`
	RecoveredAfterRestart  bool   `json:"recoveredAfterRestart,omitempty"`
	InterruptedStage       string `json:"interruptedStage,omitempty"`
}
type restoreStoredEnvironment struct {
	Manifest         backup.Environment `json:"manifest"`
	Environment      Environment        `json:"environment"`
	Code             int                `json:"code"`
	DataState        string             `json:"dataState"`
	History          []ProfileRevision  `json:"history"`
	ExistingRevision int64              `json:"existingRevision"`
}
type restoreStoredProxy struct {
	ID            string              `json:"id"`
	Ref           string              `json:"ref"`
	Configuration proxy.Configuration `json:"configuration"`
	Protected     []byte              `json:"protected"`
	Revision      int64               `json:"revision"`
}
type restoreMove struct {
	ID          string              `json:"id"`
	Live        string              `json:"live"`
	Incoming    string              `json:"incoming"`
	Previous    string              `json:"previous"`
	OldPresent  bool                `json:"oldPresent"`
	OldIdentity backup.TreeIdentity `json:"oldIdentity"`
	NewIdentity backup.TreeIdentity `json:"newIdentity"`
	OldFiles    []backup.File       `json:"oldFiles"`
	NewFiles    []backup.File       `json:"newFiles"`
}
type restorePlan struct {
	JournalVersion              int                        `json:"journalVersion"`
	ID                          string                     `json:"id"`
	Source                      string                     `json:"source"` // private journal; never returned through RPC
	Baseline                    string                     `json:"baseline"`
	ArchiveSHA256               string                     `json:"archiveSha256"`
	PreviousConfigurationSHA256 string                     `json:"previousConfigurationSha256"`
	PreviousBaseline            string                     `json:"previousBaseline"`
	CommittedBaseline           string                     `json:"committedBaseline"`
	Prepared                    bool                       `json:"prepared"`
	Environments                []restoreStoredEnvironment `json:"environments"`
	Proxies                     []restoreStoredProxy       `json:"proxies"`
	KernelMapping               map[string]string          `json:"kernelMapping"`
	Moves                       []restoreMove              `json:"moves"`
}
type restoreTask struct {
	operation           Operation
	plan                restorePlan
	phase               string
	cancel              context.CancelFunc
	running             bool
	acceptancePending   bool
	acceptanceOperation string
	signature           string
	finalPending        *Operation
	finalPhase          string
	recoveryAvailable   bool // set after this process has executed and retained a plan
	startup             bool
	bootstrapOutcome    *Operation
	bootstrapReady      bool
	bootstrapLoader     *Service // private startup state, published only after loading
	bootstrapStep       int
	bootstrapCancel     context.CancelFunc
}

func copyRestoreOperation(op Operation) Operation {
	if op.RestoreReport != nil {
		r := *op.RestoreReport
		op.RestoreReport = &r
	}
	return op
}
