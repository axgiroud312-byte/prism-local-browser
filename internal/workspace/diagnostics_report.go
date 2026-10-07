package workspace

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"regexp"
	"runtime"
	"strings"
	"time"

	"github.com/axgiroud312-byte/prism-local-browser/internal/kernel"
)

const diagnosticFormat = "prism-local-diagnostics"

type DiagnosticReport struct {
	Format          string                `json:"format"`
	SchemaVersion   int                   `json:"schemaVersion"`
	GeneratedAt     string                `json:"generatedAt"`
	Application     DiagnosticApplication `json:"application"`
	Workspace       DiagnosticWorkspace   `json:"workspace"`
	ProxyProtection string                `json:"proxyProtection"`
	Excluded        []string              `json:"excluded"`
}
type DiagnosticApplication struct {
	Version      string `json:"version"`
	Platform     string `json:"platform"`
	Architecture string `json:"architecture"`
	GoVersion    string `json:"goVersion"`
	Signature    string `json:"signature"`
}
type DiagnosticWorkspace struct {
	Status              string                `json:"status"`
	StartupCode         string                `json:"startupCode,omitempty"`
	SchemaVersion       *int                  `json:"schemaVersion,omitempty"`
	Counts              map[string]int64      `json:"counts"`
	Maintenance         []DiagnosticOperation `json:"maintenance"`
	Operations          []DiagnosticOperation `json:"operations"`
	Sessions            []DiagnosticSession   `json:"sessions"`
	Kernels             []DiagnosticKernel    `json:"kernels"`
	UnavailableSections []string              `json:"unavailableSections"`
	OmittedRecords      int                   `json:"omittedRecords"`
	ObservationSource   string                `json:"observationSource"`
	OperationLimit      int                   `json:"operationLimit"`
	SessionLimit        int                   `json:"sessionLimit"`
	KernelLimit         int                   `json:"kernelLimit"`
}
type DiagnosticOperation struct {
	Label              string `json:"label"`
	Kind               string `json:"kind"`
	State              string `json:"state"`
	Stage              string `json:"stage"`
	ErrorCode          string `json:"errorCode,omitempty"`
	PersistencePending bool   `json:"persistencePending"`
	CancelRequested    bool   `json:"cancelRequested"`
	Total              int64  `json:"total"`
	Completed          int64  `json:"completed"`
	ProgressMetric     string `json:"progressMetric"`
}
type DiagnosticSession struct {
	Label              string `json:"label"`
	State              string `json:"state"`
	NetworkPolicy      string `json:"networkPolicy"`
	ErrorCode          string `json:"errorCode,omitempty"`
	NetworkErrorCode   string `json:"networkErrorCode,omitempty"`
	Containment        string `json:"containment"`
	NeedsReconcile     bool   `json:"needsReconcile"`
	PersistencePending bool   `json:"persistencePending"`
}
type DiagnosticKernel struct {
	Label            string `json:"label"`
	Version          string `json:"version"`
	Status           string `json:"status"`
	ArchiveSHA256    string `json:"archiveSha256"`
	ExecutableSHA256 string `json:"executableSha256"`
}

var diagnosticVersion = regexp.MustCompile(`^[0-9]{1,5}\.[0-9]{1,5}\.[0-9]{1,5}(\.[0-9]{1,5}|-preview\.[0-9]{1,5})?$`)
var diagnosticHash = regexp.MustCompile(`^[a-f0-9]{64}$`)

func diagnosticEnum(value, choices string) string {
	for _, candidate := range strings.Fields(choices) {
		if value == candidate {
			return candidate
		}
	}
	return "other"
}
func diagnosticError(err *Error) string {
	if err == nil {
		return ""
	}
	return diagnosticEnum(err.Code, `VALIDATION_FAILED NOT_FOUND PREVIEW_EXPIRED REVISION_CONFLICT REQUEST_ID_REUSED PROFILE_BUSY
	NATIVE_UNAVAILABLE CAPABILITY_UNSUPPORTED WORKSPACE_LOADING STORAGE_READ_FAILED STORAGE_WRITE_FAILED DISK_FULL
	KERNEL_MISSING KERNEL_INTEGRITY_FAILED KERNEL_VERSION_UNSUPPORTED NETWORK_PROTECTION_UNAVAILABLE
	CONTROL_CHANNEL_LOST PROCESS_EXIT_UNCONFIRMED SESSION_IDENTITY_UNCONFIRMED RUNTIME_START_FAILED RUNTIME_STOP_FAILED
	OPERATION_CANCELLED PROXY_AUTH_INVALID PROXY_AUTH_FAILED PROXY_CONNECTION_FAILED PROXY_TLS_FAILED PROXY_CHECK_FAILED
	NETWORK_ERROR RESOURCE_EXHAUSTED BACKUP_INVALID BACKUP_INCOMPLETE BACKUP_RESULT_UNCONFIRMED
	RESTORE_INCOMPLETE RESTORE_RESULT_UNCONFIRMED RECYCLE_INCOMPLETE RECYCLE_RESULT_UNCONFIRMED
	MIGRATION_INCOMPLETE MIGRATION_RESULT_UNCONFIRMED DEFAULT_RESULT_UNCONFIRMED
	APPLICATION_INTERRUPTED PROCESS_CRASHED PROCESS_READY_TIMEOUT PROCESS_STOP_TIMEOUT PROCESS_START_FAILED PROCESS_ID_REUSED PROCESS_IDENTITY_UNAVAILABLE
	BACKUP_EXPORT_FAILED BACKUP_OUTPUT_UNAVAILABLE BACKUP_PREFLIGHT_FAILED BACKUP_PUBLICATION_IN_PROGRESS BACKUP_PUBLICATION_UNCONFIRMED BACKUP_VERSION_UNSUPPORTED
	BATCH_ALREADY_ACCEPTED BATCH_BUSY BATCH_PARTIAL_FAILED
	COOKIE_CLEAR_ALREADY_ATTEMPTED COOKIE_CLEAR_UNCONFIRMED COOKIE_CONTROL_FAILED COOKIE_CONTROL_UNAVAILABLE COOKIE_DUPLICATE_SELECTED
	COOKIE_EXPIRED COOKIE_IMPORT_INCOMPLETE COOKIE_READ_FAILED COOKIE_SELECTION_INVALID COOKIE_SESSION_CHANGED COOKIE_SESSION_REQUIRED COOKIE_VERIFY_MISMATCH COOKIE_WRITE_UNCONFIRMED
	CREDENTIALS_UNAVAILABLE DATA_DIR_LOCKED DATA_DIR_NOT_EMPTY DIRECTORY_WRITE_FAILED FORCE_STOP_NOT_ALLOWED MIGRATION_NOT_ACCEPTED NAME_CONFLICT PATH_OUTSIDE_ROOT
	PROXY_BRIDGE_BUSY PROXY_BRIDGE_UNAVAILABLE PROXY_CHECK_TIMEOUT PROXY_EXIT_INVALID PROXY_IN_USE PROXY_INVALID PROXY_POLICY_MISMATCH
	PROXY_REQUEST_FAILED PROXY_REQUEST_TIMEOUT PROXY_REVISION_CONFLICT PROXY_SOCKS_NEGOTIATION_FAILED PROXY_TARGET_FAILED PROXY_TARGET_UNREACHABLE
	PROXY_TARGET_UNSUPPORTED PROXY_UNAVAILABLE PROXY_UNREACHABLE PROXY_UNSUPPORTED
	RECYCLE_CONFLICT RECYCLE_NOT_ACCEPTED RESTORE_COMMITTED RESTORE_FINALIZING RESTORE_NOT_ACCEPTED SEED_CONFLICT DIAGNOSTICS_RESULT_UNCONFIRMED`)
}
func diagnosticSafeVersion(value string) string {
	if diagnosticVersion.MatchString(value) {
		return value
	}
	return "unknown"
}
func diagnosticSafeHash(value string) string {
	if diagnosticHash.MatchString(value) {
		return value
	}
	return "unknown"
}
func diagnosticNumber(value int) int64 {
	if value < 0 {
		return 0
	}
	return min(int64(value), 9007199254740991)
}
func diagnosticOperation(label string, op Operation) DiagnosticOperation {
	r := DiagnosticOperation{Label: label, Kind: diagnosticEnum(op.Kind, "create edit kernel-install kernel-verify kernel-delete runtime-start runtime-stop runtime-force-stop runtime-reconcile proxy-check cookie-import batch-create batch-clone batch-assign backup-export backup-restore recycle migration"), State: diagnosticEnum(op.State, "accepted running completed failed cancelled"), Stage: diagnosticEnum(op.Stage, `queued accepted preparing starting running stopping completed failed cancelled interrupted protected storage-pending acceptance-pending
	revalidating stopping-environments prepared swapping db-committing db-committed finalized rolled-back recovering workspace-recovery
	backup-ready copy-ready trial-starting trial-running ready committed original-retained purging publishing acquiring-archive extracting probing verifying-files verifying-extracted-files
	closing close-timeout application-interrupted finished configuration-snapshot copying-browser-data verifying-package`), ErrorCode: diagnosticError(op.Error), PersistencePending: op.PersistencePending, CancelRequested: op.CancelRequested, Total: diagnosticNumber(op.Total), Completed: diagnosticNumber(len(op.CompletedIDs)), ProgressMetric: "completed-records"}
	switch {
	case strings.HasPrefix(op.Kind, "batch-") && op.BatchReport != nil:
		r.Total = max(0, min(op.BatchReport.Total, 9007199254740991))
		r.Completed = max(0, min(op.BatchReport.CompletedCount, 9007199254740991))
		r.ProgressMetric = "completed-batch-items"
	case op.Kind == "cookie-import" && op.CookieReport != nil:
		r.Completed = diagnosticNumber(op.CookieReport.VerifiedCount)
		r.ProgressMetric = "verified-cookies"
	case op.Kind == "backup-export" && op.BackupReport != nil:
		r.Total = diagnosticNumber(op.BackupReport.EnvironmentCount)
		r.Completed = diagnosticNumber(op.BackupReport.CopiedEnvironmentCount)
		r.ProgressMetric = "copied-environments"
	case op.Kind == "backup-restore" && op.RestoreReport != nil:
		r.Total = diagnosticNumber(op.RestoreReport.EnvironmentCount)
		r.Completed = diagnosticNumber(op.RestoreReport.SwitchedCount)
		r.ProgressMetric = "switched-environments"
	case op.Kind == "recycle" && op.RecycleReport != nil:
		r.Completed = diagnosticNumber(op.RecycleReport.Completed)
		r.ProgressMetric = "completed-environments"
	}
	return r
}
func emptyDiagnosticWorkspace() DiagnosticWorkspace {
	return DiagnosticWorkspace{Status: "unavailable", Counts: map[string]int64{}, Maintenance: []DiagnosticOperation{}, Operations: []DiagnosticOperation{}, Sessions: []DiagnosticSession{}, Kernels: []DiagnosticKernel{}, UnavailableSections: []string{}, ObservationSource: "saved-records-and-host-maintenance-not-live-probe", OperationLimit: 100, SessionLimit: 100, KernelLimit: 20}
}

func buildDiagnosticReport(ctx context.Context, version string, service *Service, startup *Error) DiagnosticReport {
	r := DiagnosticReport{Format: diagnosticFormat, SchemaVersion: 1, GeneratedAt: timestamp(), Application: DiagnosticApplication{Version: diagnosticSafeVersion(version), Platform: runtime.GOOS, Architecture: runtime.GOARCH, GoVersion: runtime.Version(), Signature: "not-checked"}, Workspace: emptyDiagnosticWorkspace(), ProxyProtection: "unavailable", Excluded: []string{"credentials", "cookies-and-browser-data", "names-notes-urls", "paths-and-hostnames", "proxy-addresses-and-ips", "ids-seeds-and-command-lines", "raw-errors-and-logs"}}
	if kernel.RequireProxyNetworkBoundary() == nil {
		r.ProxyProtection = "available"
	}
	if service == nil {
		r.Workspace.StartupCode = diagnosticError(startup)
		return r
	}
	r.Workspace = service.diagnosticSnapshot(ctx)
	return r
}

// A separate read entry point: never calls Workspace.Read, persistence flush,
// startup recovery, a kernel probe, DPAPI or browser-directory inspection.
func (s *Service) diagnosticSnapshot(ctx context.Context) DiagnosticWorkspace {
	r := emptyDiagnosticWorkspace()
	if !s.mu.TryLock() {
		r.StartupCode = "PROFILE_BUSY"
		return r
	}
	defer s.mu.Unlock()
	if s.closed || s.closeRequested.Load() {
		r.StartupCode = "NATIVE_UNAVAILABLE"
		return r
	}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		r.StartupCode = "STORAGE_READ_FAILED"
		return r
	}
	defer tx.Rollback()
	var schema int
	if err = tx.QueryRowContext(ctx, "PRAGMA user_version").Scan(&schema); err != nil {
		r.StartupCode = "STORAGE_READ_FAILED"
		return r
	}
	r.SchemaVersion = &schema
	r.Status = "available"
	unavailable := func(section string) {
		r.Status = "partial"
		r.UnavailableSections = append(r.UnavailableSections, section)
	}
	for _, q := range []struct{ name, sql string }{
		{"activeEnvironments", "SELECT COUNT(*) FROM environments WHERE id NOT IN (SELECT environment_id FROM environment_trash)"},
		{"trashedEnvironments", "SELECT COUNT(*) FROM environment_trash"},
		{"proxies", "SELECT COUNT(*) FROM proxy_config"},
		{"kernels", "SELECT COUNT(*) FROM kernels WHERE id<>'kernel-pending'"},
		{"operations", "SELECT COUNT(*) FROM operations"},
		{"sessions", "SELECT COUNT(*) FROM runtime_sessions"},
	} {
		var count int64
		if err = tx.QueryRowContext(ctx, q.sql).Scan(&count); err != nil {
			unavailable(q.name)
		} else {
			r.Counts[q.name] = min(count, 9007199254740991)
		}
	}
	// Only bounded individual records enter the collector. Every exported field
	// is projected below; no original identifier or free-form text is serialized.
	read := func(query, section string, consume func(string)) {
		rows, e := tx.QueryContext(ctx, query)
		if e != nil {
			unavailable(section)
			return
		}
		defer rows.Close()
		for rows.Next() {
			var raw sql.NullString
			if e = rows.Scan(&raw); e != nil {
				unavailable(section)
				return
			}
			if !raw.Valid {
				r.OmittedRecords++
				continue
			}
			consume(raw.String)
		}
		if rows.Err() != nil {
			unavailable(section)
		}
	}
	read("SELECT CASE WHEN length(result_json)<=262144 THEN result_json END FROM operations ORDER BY rowid DESC LIMIT 100", "operations", func(raw string) {
		var op Operation
		if json.Unmarshal([]byte(raw), &op) != nil {
			r.OmittedRecords++
			return
		}
		r.Operations = append(r.Operations, diagnosticOperation(fmt.Sprintf("operation-%03d", len(r.Operations)+1), op))
	})
	read("SELECT CASE WHEN length(record_json)<=262144 THEN record_json END FROM runtime_sessions ORDER BY rowid DESC LIMIT 100", "sessions", func(raw string) {
		var v RuntimeSession
		if json.Unmarshal([]byte(raw), &v) != nil {
			r.OmittedRecords++
			return
		}
		item := DiagnosticSession{Label: fmt.Sprintf("session-%03d", len(r.Sessions)+1), State: diagnosticEnum(v.State, "starting running stopping stopped error ready network_error"), NetworkPolicy: diagnosticEnum(v.NetworkPolicy, "direct proxy"), ErrorCode: diagnosticError(v.Error), Containment: "none", NeedsReconcile: v.NeedsReconcile, PersistencePending: v.PersistencePending}
		if v.NetworkFault != nil {
			item.NetworkErrorCode = diagnosticError(v.NetworkFault.Error)
			item.Containment = diagnosticEnum(v.NetworkFault.Containment, "stopping stopped exit-unconfirmed")
		}
		r.Sessions = append(r.Sessions, item)
	})
	read("SELECT CASE WHEN length(e.record_json)<=1048576 THEN json_object('record',json(e.record_json),'status',k.status) END FROM kernel_evidence e JOIN kernels k ON k.id=e.kernel_id ORDER BY k.rowid DESC LIMIT 20", "kernels", func(raw string) {
		var v struct {
			Record kernel.Record `json:"record"`
			Status string        `json:"status"`
		}
		if json.Unmarshal([]byte(raw), &v) != nil {
			r.OmittedRecords++
			return
		}
		r.Kernels = append(r.Kernels, DiagnosticKernel{Label: fmt.Sprintf("kernel-%03d", len(r.Kernels)+1), Version: diagnosticSafeVersion(v.Record.Version), Status: diagnosticEnum(v.Status, "verified missing corrupt failed"), ArchiveSHA256: diagnosticSafeHash(v.Record.ArchiveSHA256), ExecutableSHA256: diagnosticSafeHash(v.Record.ExecutableSHA256)})
	})
	if s.restoreTask != nil {
		r.Maintenance = append(r.Maintenance, diagnosticOperation("restore", s.restoreTask.operation))
	}
	if s.recycleTask != nil {
		r.Maintenance = append(r.Maintenance, diagnosticOperation("recycle", s.recycleTask.operation))
	}
	if s.migrationTask != nil {
		r.Maintenance = append(r.Maintenance, diagnosticOperation("migration", s.migrationTask.operation))
	}
	if s.kernelTask != nil {
		r.Counts["kernelMaintenance"] = 1
	}
	r.Counts["runtimePendingWrites"] = int64(len(s.runtimePending))
	if r.OmittedRecords > 0 {
		r.Status = "partial"
	}
	return r
}

func diagnosticContext() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), 10*time.Second)
}
