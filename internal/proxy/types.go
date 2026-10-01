package proxy

type Configuration struct {
	Name    string `json:"name"`
	Type    string `json:"type"`
	Host    string `json:"host"`
	Port    int    `json:"port"`
	Country string `json:"country"`
}

// Credentials are internal input only. Public records contain a boolean, not
// this type; even the username is encrypted with the password.
type Credentials struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type Candidate struct {
	Line          int
	Configuration Configuration
	Credentials   *Credentials
	Error         string
}

type CheckError struct {
	Code      string `json:"code"`
	Message   string `json:"message"`
	Retryable bool   `json:"retryable"`
}
type Step struct {
	Stage   string `json:"stage"`
	Status  string `json:"status"`
	Time    string `json:"time"`
	Message string `json:"message"`
}
type Report struct {
	Mode           string      `json:"mode"`
	AdapterVersion string      `json:"adapterVersion"`
	ProxyID        string      `json:"proxyId"`
	Revision       int64       `json:"revision"`
	StartedAt      string      `json:"startedAt"`
	FinishedAt     string      `json:"finishedAt"`
	DurationMS     int64       `json:"durationMs"`
	TargetOrigin   string      `json:"targetOrigin"`
	Steps          []Step      `json:"steps"`
	ExitIP         string      `json:"exitIp,omitempty"`
	Error          *CheckError `json:"error,omitempty"`
}
