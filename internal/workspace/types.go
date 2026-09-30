package workspace

import "encoding/json"

const PendingKernelID = "kernel-pending"

type Error struct {
	Code      string `json:"code"`
	Message   string `json:"message"`
	Retryable bool   `json:"retryable"`
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
	ID        string `json:"id"`
	Code      string `json:"code"`
	Status    string `json:"status"`
	Cookies   []any  `json:"cookies"`
	CreatedAt string `json:"createdAt"`
}
type Preview struct {
	PreviewID        string      `json:"previewId"`
	Environment      Environment `json:"environment"`
	ExpectedRevision int64       `json:"expectedRevision,omitempty"`
}
type Operation struct {
	ID              string   `json:"id"`
	Kind            string   `json:"kind"`
	State           string   `json:"state"`
	Total           int      `json:"total"`
	CompletedIDs    []string `json:"completedIds"`
	CancelRequested bool     `json:"cancelRequested"`
}
type Mutation struct {
	PreviewID        string        `json:"previewId"`
	Configuration    Configuration `json:"configuration"`
	RequestID        string        `json:"requestId"`
	ExpectedRevision int64         `json:"expectedRevision,omitempty"`
	Count            int           `json:"count,omitempty"`
}
type Kernel struct {
	ID        string `json:"id"`
	Version   string `json:"version"`
	Available bool   `json:"available"`
	Source    string `json:"source"`
	Note      string `json:"note"`
}
type Activity struct {
	ID     string `json:"id"`
	Time   string `json:"time"`
	Action string `json:"action"`
	Target string `json:"target"`
	Result string `json:"result"`
	Detail string `json:"detail"`
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
	Mode  string `json:"mode"`
	State State  `json:"state"`
}
