package workspace

import (
	"context"
	"time"

	"github.com/axgiroud312-byte/prism-local-browser/internal/proxy"
)

type ProxyView struct {
	proxy.Configuration
	ID                string        `json:"id"`
	Revision          int64         `json:"revision"`
	HasAuthentication bool          `json:"hasAuthentication"`
	Status            string        `json:"status"`
	UsedBy            []string      `json:"usedBy"`
	CheckReport       *proxy.Report `json:"checkReport,omitempty"`
}
type ProxyImportRow struct {
	Line              int                  `json:"line"`
	Configuration     *proxy.Configuration `json:"configuration,omitempty"`
	HasAuthentication bool                 `json:"hasAuthentication"`
	DuplicateGroupID  string               `json:"duplicateGroupId,omitempty"`
	DuplicateCount    int                  `json:"duplicateCount"`
	ExistingCount     int                  `json:"existingCount"`
	Error             string               `json:"error,omitempty"`
}
type ProxyImportPreview struct {
	Mode            string                         `json:"mode"`
	PreviewID       string                         `json:"previewId"`
	ExpiresAt       string                         `json:"expiresAt"`
	IgnoredLines    int                            `json:"ignoredLines"`
	Rows            []ProxyImportRow               `json:"rows"`
	DuplicateGroups map[string]ProxyDuplicateGroup `json:"duplicateGroups"`
}
type ProxyDuplicateGroup struct {
	Lines            []int    `json:"lines"`
	ExistingProxyIDs []string `json:"existingProxyIds"`
}
type proxyImportDraft struct {
	id        string
	expiresAt time.Time
	timer     *time.Timer
	rows      map[int]proxy.Candidate
}
type ProxyCommitImport struct {
	PreviewID    string `json:"previewId"`
	SelectedRows []int  `json:"selectedRows"`
	RequestID    string `json:"requestId"`
}
type ProxyCredentialChange struct {
	Action   string `json:"action"`
	Username string `json:"username,omitempty"`
	Password string `json:"password,omitempty"`
}
type ProxyUpdate struct {
	ProxyID          string                `json:"proxyId"`
	ExpectedRevision int64                 `json:"expectedRevision"`
	Configuration    proxy.Configuration   `json:"configuration"`
	Credentials      ProxyCredentialChange `json:"credentials"`
	RequestID        string                `json:"requestId"`
}
type ProxyTarget struct {
	ProxyID          string `json:"proxyId"`
	ExpectedRevision int64  `json:"expectedRevision"`
	RequestID        string `json:"requestId"`
}
type proxyCheckTask struct {
	operation Operation
	cancel    context.CancelFunc
	revision  int64
}
type proxyCheckWrite struct {
	operation Operation
	report    proxy.Report
}
