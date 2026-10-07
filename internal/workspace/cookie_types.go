package workspace

import (
	"context"
	"time"

	"github.com/axgiroud312-byte/prism-local-browser/internal/cookies"
)

type cookieTransport interface {
	ReadCookies(context.Context) ([]cookies.Stored, error)
	ApplyCookie(context.Context, cookies.Cookie) (cookies.ApplyResult, error)
	ClearCookies(context.Context) error
}

type CookiePreview struct {
	Mode                  string        `json:"mode"`
	PreviewID             string        `json:"previewId"`
	EnvironmentID         string        `json:"environmentId"`
	EnvironmentName       string        `json:"environmentName"`
	ExpectedRevision      int64         `json:"expectedRevision"`
	SessionID             string        `json:"sessionId,omitempty"`
	RequiresStart         bool          `json:"requiresStart"`
	Format                string        `json:"format"`
	ExpiresAt             string        `json:"expiresAt"`
	Rows                  []cookies.Row `json:"rows"`
	Total                 int           `json:"total"`
	ValidCount            int           `json:"validCount"`
	ErrorCount            int           `json:"errorCount"`
	ExpiredCount          int           `json:"expiredCount"`
	ConflictCount         int           `json:"conflictCount"`
	ExistingConflictCount *int          `json:"existingConflictCount,omitempty"`
	ObservationError      *Error        `json:"observationError,omitempty"`
}

type CookieCommit struct {
	PreviewID        string `json:"previewId"`
	EnvironmentID    string `json:"environmentId"`
	ExpectedRevision int64  `json:"expectedRevision"`
	SessionID        string `json:"sessionId"`
	SelectedRows     []int  `json:"selectedRows"`
	Policy           string `json:"policy"`
	RequestID        string `json:"requestId"`
}

type CookieItemResult struct {
	cookies.Row
	Status string `json:"status"`
}
type CookieImportReport struct {
	Mode                string             `json:"mode"`
	PreviewID           string             `json:"previewId"`
	EnvironmentID       string             `json:"environmentId"`
	SessionID           string             `json:"sessionId"`
	Revision            int64              `json:"revision"`
	Policy              string             `json:"policy"`
	ClearState          string             `json:"clearState"`
	VerifiedCount       int                `json:"verifiedCount"`
	WrittenCount        int                `json:"writtenCount"`
	AlreadyMatchedCount int                `json:"alreadyMatchedCount"`
	FailedCount         int                `json:"failedCount"`
	UnconfirmedCount    int                `json:"unconfirmedCount"`
	SkippedCount        int                `json:"skippedCount"`
	FinishedAt          string             `json:"finishedAt,omitempty"`
	Items               []CookieItemResult `json:"items"`
}
type cookieImportDraft struct {
	preview        CookiePreview
	values         map[int]cookies.Cookie
	expires        time.Time
	timer          *time.Timer
	clearAttempted bool
}
type cookieImportTask struct {
	operation Operation
	cancel    context.CancelFunc
	slot      *runtimeSlot
	transport cookieTransport
	values    map[int]cookies.Cookie
}
