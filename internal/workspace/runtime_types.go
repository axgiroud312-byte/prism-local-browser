package workspace

import (
	"context"

	"github.com/axgiroud312-byte/prism-local-browser/internal/kernel"
	"github.com/axgiroud312-byte/prism-local-browser/internal/proxy"
)

// RuntimeLaunch is host-only. RPC never accepts executable paths, parameters,
// data directories, process identities or an arbitrary network fallback.
type RuntimeLaunch struct {
	Root               string
	EnvironmentID      string
	SessionID          string
	DataReference      string
	Kernel             kernel.Record
	Profile            DeviceProfile
	Configuration      Configuration
	OnCreated          func(int, string) error
	Proxy              *ProxyView
	ProxyCredentialRef string
	Network            RuntimeProxyChannel
}

type RuntimeProxyChannel interface {
	kernel.ManagedNetwork
	ID() string
	Preflight(context.Context, func(proxy.Step)) proxy.Report
}

type RuntimeProcess interface {
	PID() int
	CreatedAt() string
	Done() <-chan struct{}
	Alive() bool
	Snapshot() kernel.RuntimeSnapshot
	Stop(context.Context) error
	Close() error
}

type RuntimeSession struct {
	Mode                string               `json:"mode"`
	EnvironmentID       string               `json:"environmentId"`
	SessionID           string               `json:"sessionId"`
	OperationID         string               `json:"operationId"`
	State               string               `json:"state"`
	Revision            int64                `json:"revision"`
	FingerprintRevision int64                `json:"fingerprintRevision"`
	KernelID            string               `json:"kernelId"`
	UserDataRef         string               `json:"userDataRef"`
	NetworkPolicy       string               `json:"networkPolicy"`
	ProxyID             string               `json:"proxyId,omitempty"`
	ProxyRevision       int64                `json:"proxyRevision,omitempty"`
	ProxyChannelID      string               `json:"proxyChannelId,omitempty"`
	ProxyReport         *proxy.Report        `json:"proxyReport,omitempty"`
	NetworkFault        *RuntimeNetworkFault `json:"networkFault,omitempty"`
	PID                 int                  `json:"pid,omitempty"`
	ProcessCreatedAt    string               `json:"processCreatedAt,omitempty"`
	StartedAt           string               `json:"startedAt,omitempty"`
	Error               *Error               `json:"error,omitempty"`
	RootPID             int                  `json:"rootPid,omitempty"`
	CanControl          bool                 `json:"canControl"`
	CanForce            bool                 `json:"canForce"`
	NeedsReconcile      bool                 `json:"needsReconcile"`
	NextAction          string               `json:"nextAction,omitempty"`
	ReconciledAt        string               `json:"reconciledAt,omitempty"`
	LastExitCode        *uint32              `json:"lastExitCode,omitempty"`
	LaunchStage         string               `json:"launchStage,omitempty"`
	ResourceVersion     string               `json:"resourceVersion,omitempty"`
	PersistencePending  bool                 `json:"persistencePending"`
}

type RuntimeNetworkFault struct {
	State       string `json:"state"`
	Error       *Error `json:"error"`
	ObservedAt  string `json:"observedAt"`
	Containment string `json:"containment"`
}

type runtimeRequest struct {
	EnvironmentID string `json:"environmentId"`
	RequestID     string `json:"requestId"`
	NetworkPolicy string `json:"networkPolicy,omitempty"`
	SessionID     string `json:"sessionId,omitempty"`
}
