package workspace

import (
	"context"

	"github.com/axgiroud312-byte/prism-local-browser/internal/kernel"
)

// RuntimeLaunch is host-only. RPC never accepts executable paths, parameters,
// data directories, process identities or an arbitrary network fallback.
type RuntimeLaunch struct {
	Root          string
	EnvironmentID string
	SessionID     string
	DataReference string
	Kernel        kernel.Record
	Profile       DeviceProfile
	Configuration Configuration
}

type RuntimeProcess interface {
	PID() int
	CreatedAt() string
	Done() <-chan struct{}
	Alive() bool
	Stop(context.Context) error
	Close() error
}

type RuntimeSession struct {
	Mode                string `json:"mode"`
	EnvironmentID       string `json:"environmentId"`
	SessionID           string `json:"sessionId"`
	OperationID         string `json:"operationId"`
	State               string `json:"state"`
	Revision            int64  `json:"revision"`
	FingerprintRevision int64  `json:"fingerprintRevision"`
	KernelID            string `json:"kernelId"`
	UserDataRef         string `json:"userDataRef"`
	NetworkPolicy       string `json:"networkPolicy"`
	PID                 int    `json:"pid,omitempty"`
	ProcessCreatedAt    string `json:"processCreatedAt,omitempty"`
	StartedAt           string `json:"startedAt,omitempty"`
	Error               *Error `json:"error,omitempty"`
}

type runtimeRequest struct {
	EnvironmentID string `json:"environmentId"`
	RequestID     string `json:"requestId"`
	NetworkPolicy string `json:"networkPolicy,omitempty"`
}
