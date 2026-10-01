package workspace

import (
	"context"
	"io"
)

const maxSafeInteger int64 = 9007199254740991
const batchPageSize = 25

type BatchPreviewRequest struct {
	Kind      string         `json:"kind"`
	Create    *Mutation      `json:"create,omitempty"`
	SourceIDs []string       `json:"sourceIds,omitempty"`
	Mappings  []BatchMapping `json:"mappings,omitempty"`
}
type BatchMapping struct {
	EnvironmentID string `json:"environmentId"`
	ProxyID       string `json:"proxyId"`
}
type BatchItem struct {
	Index            int64  `json:"index"`
	Name             string `json:"name"`
	EnvironmentID    string `json:"environmentId,omitempty"`
	SourceID         string `json:"sourceId,omitempty"`
	SourceRevision   int64  `json:"sourceRevision,omitempty"`
	ExpectedRevision int64  `json:"expectedRevision,omitempty"`
	ProxyID          string `json:"proxyId"`
	ProxyName        string `json:"proxyName"`
	NewIdentity      bool   `json:"newIdentity"`
	State            string `json:"state"`
	Error            *Error `json:"error,omitempty"`
}
type BatchReport struct {
	Mode                   string `json:"mode"`
	PlanID                 string `json:"planId"`
	Kind                   string `json:"kind"`
	Total                  int64  `json:"total"`
	CompletedCount         int64  `json:"completedCount"`
	FailedCount            int64  `json:"failedCount"`
	NotExecutedCount       int64  `json:"notExecutedCount"`
	AttemptCompletedCount  int64  `json:"attemptCompletedCount"`
	SharedProxyAssignments int64  `json:"sharedProxyAssignments"`
	DirectAssignments      int64  `json:"directAssignments"`
	Sequence               int64  `json:"sequence"`
	FinishedAt             string `json:"finishedAt,omitempty"`
}
type BatchPage struct {
	BatchReport
	Offset             int64       `json:"offset"`
	PageSize           int         `json:"pageSize"`
	ExpiresAt          string      `json:"expiresAt"`
	OperationID        string      `json:"operationId,omitempty"`
	CurrentOperationID string      `json:"currentOperationId,omitempty"`
	History            bool        `json:"history"`
	Items              []BatchItem `json:"items"`
}

// Immutable safe configuration snapshots. No browsing files or credentials.
type batchSnapshot struct {
	Configuration    Configuration `json:"configuration"`
	Profile          DeviceProfile `json:"profile"`
	SourceID         string        `json:"sourceId,omitempty"`
	ExpectedRevision int64         `json:"expectedRevision,omitempty"`
	ProxyRevision    int64         `json:"proxyRevision,omitempty"`
	ProxyName        string        `json:"proxyName"`
}
type batchIdentity struct {
	EnvironmentID string        `json:"environmentId"`
	FingerprintID string        `json:"fingerprintId"`
	CreatedAt     string        `json:"createdAt"`
	Profile       DeviceProfile `json:"profile"`
}
type batchPlan struct {
	ID, Kind, ExpiresAt, OperationID, State                                      string
	Template                                                                     batchSnapshot
	Total, Cursor, Completed, Failed, AttemptCompleted, Shared, Direct, Sequence int64
	CancelRequested                                                              bool
	Error                                                                        *Error
}
type batchStoredItem struct {
	Index    int64
	Snapshot batchSnapshot
	Identity *batchIdentity
	State    string
	Error    *Error
}
type batchTask struct {
	planID, operationID string
	cancel              context.CancelFunc
	// One bounded terminal observation; DB item side effects are never replayed.
	pending    *Operation
	last       Operation
	finalState string
	finalCause *Error
}

type batchAcceptance struct {
	requestID, signature, planID string
	operation                    Operation
	cause                        error
}

// PrepareBatchDirectory is a trusted host-only seam; the production host uses
// the kernel's exclusive, journal-bound empty-directory preparation.
type BatchDirectoryInput struct {
	Root, DataReference, EnvironmentID, PlanID string
	Index                                      int64
}
type BatchDirectoryLease interface {
	io.Closer
	CheckEmpty() error
}
