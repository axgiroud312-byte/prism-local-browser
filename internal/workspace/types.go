package workspace

import (
	"encoding/json"
	"github.com/axgiroud312-byte/prism-local-browser/internal/kernel"
)

const PendingKernelID = "kernel-pending"

type Error struct {
	Code      string         `json:"code"`
	Message   string         `json:"message"`
	Retryable bool           `json:"retryable"`
	Details   map[string]any `json:"details,omitempty"`
}
type Result struct {
	OK          bool   `json:"ok"`
	Mode        string `json:"mode"`
	Data        any    `json:"data,omitempty"`
	Error       *Error `json:"error,omitempty"`
	OperationID string `json:"operationId,omitempty"`
}
type Request struct {
	Mode    string          `json:"mode"`
	Method  string          `json:"method"`
	Payload json.RawMessage `json:"payload"`
}
type Configuration struct {
	Name               string `json:"name"`
	Group              string `json:"group"`
	Note               string `json:"note"`
	ProxyID            string `json:"proxyId"`
	CoreID             string `json:"coreId"`
	Seed               string `json:"seed"`
	Language           string `json:"language"`
	Timezone           string `json:"timezone"`
	CPU                string `json:"cpu"`
	Width              int    `json:"width"`
	Height             int    `json:"height"`
	URLs               string `json:"urls"`
	RestoreTabs        bool   `json:"restoreTabs"`
	FingerprintVersion string `json:"fingerprintVersion"`
}
type Environment struct {
	Configuration
	ID         string `json:"id"`
	Code       string `json:"code"`
	Status     string `json:"status"`
	Error      string `json:"error,omitempty"`
	Cookies    []any  `json:"cookies"`
	CreatedAt  string `json:"createdAt"`
	LastOpened string `json:"lastOpened,omitempty"`
}
type Preview struct {
	PreviewID        string              `json:"previewId"`
	Environment      Environment         `json:"environment"`
	ExpectedRevision int64               `json:"expectedRevision,omitempty"`
	Fingerprint      *FingerprintPreview `json:"fingerprint,omitempty"`
	UserDataRef      string              `json:"userDataRef,omitempty"`
}
type Operation struct {
	ID                 string         `json:"id"`
	Kind               string         `json:"kind"`
	State              string         `json:"state"`
	Total              int            `json:"total"`
	CompletedIDs       []string       `json:"completedIds"`
	CancelRequested    bool           `json:"cancelRequested"`
	Stage              string         `json:"stage,omitempty"`
	PersistencePending bool           `json:"persistencePending,omitempty"`
	Error              *Error         `json:"error,omitempty"`
	KernelID           string         `json:"kernelId,omitempty"`
	ResourceKey        string         `json:"resourceKey,omitempty"`
	Report             *kernel.Report `json:"report,omitempty"`
	EnvironmentID      string         `json:"environmentId,omitempty"`
	SessionID          string         `json:"sessionId,omitempty"`
}
type Mutation struct {
	PreviewID        string        `json:"previewId"`
	Configuration    Configuration `json:"configuration"`
	RequestID        string        `json:"requestId"`
	ExpectedRevision int64         `json:"expectedRevision,omitempty"`
	Count            int           `json:"count,omitempty"`
	EnvironmentID    string        `json:"environmentId,omitempty"`
	ProfileHash      string        `json:"profileHash,omitempty"`
}
type Kernel struct {
	ID        string `json:"id"`
	Version   string `json:"version"`
	Available bool   `json:"available"`
	Source    string `json:"source"`
	Note      string `json:"note"`
}
type Activity struct {
	ID            string `json:"id"`
	Time          string `json:"time"`
	Action        string `json:"action"`
	Target        string `json:"target"`
	Result        string `json:"result"`
	Detail        string `json:"detail"`
	EnvironmentID string `json:"environmentId,omitempty"`
	SessionID     string `json:"sessionId,omitempty"`
	ErrorCode     string `json:"errorCode,omitempty"`
	NextAction    string `json:"nextAction,omitempty"`
}
type State struct {
	SchemaVersion int           `json:"schemaVersion"`
	Environments  []Environment `json:"environments"`
	Proxies       []any         `json:"proxies"`
	Kernels       []Kernel      `json:"kernels"`
	Backups       []any         `json:"backups"`
	Activities    []Activity    `json:"activities"`
}
type View struct {
	Mode             string                     `json:"mode"`
	State            State                      `json:"state"`
	KernelRecords    []KernelView               `json:"kernelRecords"`
	KernelOperations []Operation                `json:"kernelOperations"`
	Fingerprints     map[string]ProfileRevision `json:"fingerprints"`
	DataReferences   map[string]string          `json:"dataReferences"`
	RuntimeSessions  map[string]RuntimeSession  `json:"runtimeSessions"`
}
type KernelView struct {
	kernel.Record
	Status string   `json:"status"`
	UsedBy []string `json:"usedBy"`
}
