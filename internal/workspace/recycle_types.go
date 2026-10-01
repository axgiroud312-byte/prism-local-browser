package workspace

import (
	"context"
	"time"

	"github.com/axgiroud312-byte/prism-local-browser/internal/backup"
)

type RecycleRequest struct {
	PreviewID string `json:"previewId"`
	Confirm   bool   `json:"confirm"`
	RequestID string `json:"requestId"`
}
type RecycleReport struct {
	Mode        string `json:"mode"`
	Action      string `json:"action"`
	RequestID   string `json:"requestId"`
	PreviewID   string `json:"previewId"`
	Sequence    int64  `json:"sequence"`
	Completed   int    `json:"completed"`
	Failed      int    `json:"failed"`
	NotExecuted int    `json:"notExecuted"`
	Protected   bool   `json:"protected"`
}
type RecycleItem struct {
	ID             string `json:"id"`
	EnvironmentID  string `json:"environmentId"`
	Name           string `json:"name"`
	Seed           string `json:"seed"`
	KernelID       string `json:"kernelId"`
	Revision       int64  `json:"revision"`
	DataPresent    bool   `json:"dataPresent"`
	BackupRecorded bool   `json:"backupRecorded"`
	RemovedAt      string `json:"removedAt,omitempty"`
	State          string `json:"state"`
	Error          *Error `json:"error,omitempty"`
}
type RecyclePreview struct {
	Mode        string `json:"mode"`
	PreviewID   string `json:"previewId"`
	Action      string `json:"action"`
	ExpiresAt   string `json:"expiresAt"`
	Total       int    `json:"total"`
	DataCount   int    `json:"dataCount"`
	BackupCount int    `json:"backupCount"`
}
type RecyclePage struct {
	Mode      string          `json:"mode"`
	Offset    int             `json:"offset"`
	PageSize  int             `json:"pageSize"`
	Total     int             `json:"total"`
	Items     []RecycleItem   `json:"items"`
	Preview   *RecyclePreview `json:"preview,omitempty"`
	Operation *Operation      `json:"operation,omitempty"`
}
type recyclePageRequest struct {
	Offset      int    `json:"offset"`
	PageSize    int    `json:"pageSize"`
	PreviewID   string `json:"previewId,omitempty"`
	OperationID string `json:"operationId,omitempty"`
}
type recycleEntry struct {
	Item      RecycleItem          `json:"item"`
	TrashID   string               `json:"trashId"`
	Reference string               `json:"reference"`
	Identity  backup.TreeIdentity  `json:"identity"`
	Files     []backup.File        `json:"files"`
	Lock      *backup.RetainedLock `json:"lock"`
	Baseline  string               `json:"baseline"`
}
type recyclePlanItem struct {
	Entry         recycleEntry `json:"entry"`
	Prepared      bool         `json:"prepared"`
	PurgeStarted  bool         `json:"purgeStarted"`
	DataDeleted   bool         `json:"dataDeleted"`
	Committed     bool         `json:"committed"`
	AfterBaseline string       `json:"afterBaseline"`
	Result        string       `json:"result"`
	Error         *Error       `json:"error,omitempty"`
}
type recyclePlan struct {
	Version int               `json:"version"`
	ID      string            `json:"id"`
	Action  string            `json:"action"`
	Items   []recyclePlanItem `json:"items"`
}
type recycleDraft struct {
	preview RecyclePreview
	plan    recyclePlan
	expires time.Time
}
type recycleTask struct {
	plan              recyclePlan
	operation         Operation
	phase             string
	running           bool
	cancel            context.CancelFunc
	acceptancePending bool
	signature         string
	acceptedJSON      string
	finalPending      *Operation
	startup           bool
	bootstrapOutcome  *Operation
	bootstrapLoader   *Service
	bootstrapStep     int
	bootstrapReady    bool
}

func copyRecycleOperation(op Operation) Operation {
	if op.RecycleReport != nil {
		report := *op.RecycleReport
		op.RecycleReport = &report
	}
	return op
}
func recycleReference(trashID string) string { return "recycle/" + trashID + "/user-data" }
