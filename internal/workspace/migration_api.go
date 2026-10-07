package workspace

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
	"time"

	"github.com/axgiroud312-byte/prism-local-browser/internal/backup"
)

func (s *Service) migrationCall(request Request) Result {
	switch request.Method {
	case "Migration.Preview":
		var input struct {
			EnvironmentID string `json:"environmentId"`
			KernelID      string `json:"kernelId"`
		}
		if decode(request.Payload, &input) != nil {
			return failure("VALIDATION_FAILED", "请选择明确环境与已核验目标构建。", false)
		}
		return s.previewMigration(input.EnvironmentID, input.KernelID)
	case "Migration.Prepare":
		var input MigrationRequest
		if decode(request.Payload, &input) != nil {
			return failure("VALIDATION_FAILED", "迁移准备只接受原预览及明确确认。", false)
		}
		return s.acceptMigration(input)
	case "Migration.Action":
		var input MigrationAction
		if decode(request.Payload, &input) != nil || !input.Confirm || !map[string]bool{"stop": true, "commit": true, "recover": true}[input.Action] {
			return failure("VALIDATION_FAILED", "请选择原任务并明确确认停止、切换或核对。", false)
		}
		task := s.migrationTask
		if task == nil {
			_, op, _, err := s.readMigrationJournal(input.OperationID)
			if err != nil {
				return migrationProblem(err)
			}
			return success(op, op.ID)
		}
		if task.plan.ID != input.OperationID {
			return failure("PROFILE_BUSY", "请先核对当前迁移。", true)
		}
		if task.acceptancePending {
			result := s.confirmMigrationAcceptance(task)
			if !result.OK {
				return result
			}
			return success(copyMigrationOperation(task.operation), task.plan.ID)
		}
		if input.Action == "stop" && task.running && task.phase == "trial-running" {
			select {
			case task.stop <- struct{}{}:
			default:
			}
			return success(copyMigrationOperation(task.operation), task.plan.ID)
		}
		if task.running {
			return success(copyMigrationOperation(task.operation), task.plan.ID)
		}
		if task.finalPending != nil {
			if task.startup && !task.bootstrapReady {
				s.startMigrationWorker(task, s.recoverMigration)
				return success(copyMigrationOperation(task.operation), task.plan.ID)
			}
			s.flushMigrationPersistence()
			return success(copyMigrationOperation(task.operation), task.plan.ID)
		}
		if input.Action == "commit" {
			if task.phase != "ready" || task.operation.CancelRequested || task.operation.PersistencePending {
				return failure("MIGRATION_INCOMPLETE", "必须先正常停止并核对试用副本，再确认切换。", true)
			}
			s.startMigrationWorker(task, s.commitMigration)
		} else if input.Action == "stop" && task.process != nil {
			s.startMigrationWorker(task, s.stopMigrationTrial)
		} else if input.Action == "recover" {
			s.startMigrationWorker(task, s.recoverMigration)
		}
		return success(copyMigrationOperation(task.operation), task.plan.ID)
	}
	return failure("CAPABILITY_UNSUPPORTED", "未知迁移操作。", false)
}

func migrationBaseline(query interface {
	Query(string, ...any) (*sql.Rows, error)
}, plan migrationPlan) (string, error) {
	p := restorePlan{JournalVersion: 3, Environments: []restoreStoredEnvironment{{Manifest: backup.Environment{ID: plan.Environment.ID}}}, Proxies: []restoreStoredProxy{}}
	if plan.Environment.ProxyID != "" {
		p.Proxies = append(p.Proxies, restoreStoredProxy{ID: plan.Environment.ProxyID})
	}
	return restoreAffectedBaseline(query, p)
}

func (s *Service) previewMigration(environmentID, kernelID string) Result {
	if !s.recycleIdle() {
		return failure("PROFILE_BUSY", "请先结束当前维护任务。", true)
	}
	e, revision, fingerprintID, err := s.readEnvironment(environmentID)
	if err != nil {
		return failure("NOT_FOUND", "环境不存在或已移入回收区。", true)
	}
	if err = s.recycleTargetFree(environmentID); err != nil {
		return migrationProblem(err)
	}
	if e.CoreID == kernelID || e.CoreID == PendingKernelID || kernelID == PendingKernelID {
		return failure("VALIDATION_FAILED", "迁移需要当前与目标两个不同的已核验构建。", false)
	}
	old, err := savedKernelFrom(s.db, e.CoreID, true)
	if err != nil {
		return kernelFailure(err)
	}
	newBuild, err := savedKernelFrom(s.db, kernelID, true)
	if err != nil {
		return kernelFailure(err)
	}
	current, err := readProfileFrom(s.db, fingerprintID)
	if err != nil {
		return migrationProblem(err)
	}
	config := e.Configuration
	config.CoreID = kernelID
	next, err := frozenProfile(config, &newBuild, FingerprintGeneratorVersion, current.Profile.ConfigRevision+1, false)
	if err != nil {
		return kernelFailure(err)
	}
	ref, _ := dataReference(e.ID)
	target, err := s.backupDataTarget(e.ID, ref)
	if err != nil {
		return migrationProblem(err)
	}
	workID := id()
	workRef, _ := dataReference(workID)
	operationID := id()
	expires := time.Now().Add(15 * time.Minute)
	p := MigrationPreview{Mode: "native", PreviewID: id(), EnvironmentID: e.ID, Name: e.Name, ExpectedRevision: revision, Before: current.Profile, After: next, BeforeCapabilities: old.Report.Capabilities, AfterCapabilities: newBuild.Report.Capabilities, Changes: profileChanges(&current.Profile, next), ExpiresAt: expires.UTC().Format(time.RFC3339Nano)}
	plan := migrationPlan{Version: 1, ID: operationID, Preview: p, Environment: e, FingerprintID: fingerprintID, CreatedAt: timestamp(), Target: target, WorkID: workID, Move: restoreMove{ID: e.ID, Live: ref, Incoming: workRef, Previous: migrationStageRef(operationID) + "/previous", OldFiles: []backup.File{}, NewFiles: []backup.File{}}}
	plan.Baseline, err = migrationBaseline(s.db, plan)
	if err != nil {
		return migrationProblem(err)
	}
	s.migrationDraft = &migrationDraft{plan: plan, expires: expires}
	return success(p, "")
}

func (s *Service) acceptMigration(input MigrationRequest) Result {
	if !input.Confirm || strings.TrimSpace(input.RequestID) == "" || len(input.RequestID) > 128 {
		return failure("VALIDATION_FAILED", "请确认原环境停止、允许独立副本试用，并提供请求标识。", false)
	}
	signature := migrationSignature(input)
	var priorSignature, receipt string
	err := s.db.QueryRow("SELECT signature,result_json FROM requests WHERE id=?", input.RequestID).Scan(&priorSignature, &receipt)
	if err == nil {
		if signature != priorSignature {
			return failure("REQUEST_ID_REUSED", "原迁移请求不能更换预览。", false)
		}
		if t := s.migrationTask; t != nil && t.operation.MigrationReport.RequestID == input.RequestID && t.acceptancePending {
			return s.confirmMigrationAcceptance(t)
		}
		var result Result
		if json.Unmarshal([]byte(receipt), &result) != nil {
			return migrationProblem(errors.New("invalid migration receipt"))
		}
		return result
	}
	if err != sql.ErrNoRows {
		return migrationProblem(err)
	}
	if t := s.migrationTask; t != nil && t.acceptancePending && t.operation.MigrationReport.RequestID == input.RequestID {
		return s.confirmMigrationAcceptance(t)
	}
	if !s.recycleIdle() {
		return failure("PROFILE_BUSY", "已有维护或未完成任务，请先核对。", true)
	}
	d := s.migrationDraft
	if d == nil || d.plan.Preview.PreviewID != input.PreviewID || time.Now().After(d.expires) {
		return failure("PREVIEW_EXPIRED", "迁移预览已失效，请重新读取。", true)
	}
	plan := d.plan
	if err = s.recycleTargetFree(plan.Environment.ID); err != nil {
		return migrationProblem(err)
	}
	baseline, err := migrationBaseline(s.db, plan)
	if err != nil {
		return migrationProblem(err)
	}
	if baseline != plan.Baseline {
		return failure("REVISION_CONFLICT", "环境配置在预览后已变化，请重新预览。", true)
	}
	for _, kernelID := range []string{plan.Preview.Before.KernelID, plan.Preview.After.KernelID} {
		if _, err = savedKernelFrom(s.db, kernelID, true); err != nil {
			return kernelFailure(err)
		}
	}
	op := Operation{ID: plan.ID, Kind: "migration", State: "accepted", Stage: "accepted", Total: 1, CompletedIDs: []string{}, EnvironmentID: plan.Environment.ID, MigrationReport: &MigrationReport{Mode: "native", RequestID: input.RequestID, PreviewID: input.PreviewID, EnvironmentID: plan.Environment.ID, OldKernelID: plan.Preview.Before.KernelID, NewKernelID: plan.Preview.After.KernelID, Seed: plan.Environment.Seed, Sequence: 1, Protected: true}}
	result := success(map[string]any{"status": "accepted", "operation": op}, op.ID)
	encoded, _ := json.Marshal(result)
	opJSON, _ := json.Marshal(op)
	tx, err := s.db.Begin()
	if err != nil {
		return migrationProblem(err)
	}
	defer tx.Rollback()
	if _, err = tx.Exec("INSERT INTO operations(id,result_json) VALUES(?,?)", op.ID, string(opJSON)); err != nil {
		return migrationProblem(err)
	}
	if _, err = tx.Exec("INSERT INTO requests(id,signature,result_json) VALUES(?,?,?)", input.RequestID, signature, string(encoded)); err != nil {
		return migrationProblem(err)
	}
	if _, err = tx.Exec("INSERT INTO kernel_migrations(operation_id,plan_json,plan_sha256,phase,request_id,signature) VALUES(?,'','','accepted',?,?)", op.ID, input.RequestID, signature); err != nil {
		return migrationProblem(err)
	}
	if err = saveMigration(tx, plan, op, "accepted"); err != nil {
		return migrationProblem(err)
	}
	// Retain all old historical exact builds as well as the trial build, even if
	// this environment is later purged. The complete pre-upgrade package needs them.
	if _, err = tx.Exec("INSERT INTO migration_kernel_refs(operation_id,kernel_id) SELECT ?,kernel_id FROM fingerprint_revisions WHERE fingerprint_id=? UNION SELECT ?,?", op.ID, plan.FingerprintID, op.ID, plan.Preview.After.KernelID); err != nil {
		return migrationProblem(err)
	}
	if s.options.BeforeCommit != nil {
		if err = s.options.BeforeCommit(); err != nil {
			return migrationProblem(err)
		}
	}
	err = s.commitMigrationTransaction(tx)
	task := &migrationTask{plan: plan, operation: op, phase: "accepted", stop: make(chan struct{}, 1), acceptancePending: err != nil}
	s.migrationTask = task
	s.migrationDraft = nil
	if err != nil {
		task.operation.PersistencePending = true
		task.operation.Stage = "acceptance-pending"
		return s.confirmMigrationAcceptance(task)
	}
	s.startMigrationWorker(task, s.runMigrationTrial)
	return result
}

func (s *Service) confirmMigrationAcceptance(task *migrationTask) Result {
	plan, op, phase, err := s.readMigrationJournal(task.plan.ID)
	if err == sql.ErrNoRows {
		var count int
		if s.db.QueryRow(`SELECT (SELECT COUNT(*) FROM kernel_migrations WHERE operation_id=?)+(SELECT COUNT(*) FROM operations WHERE id=?)+(SELECT COUNT(*) FROM requests WHERE id=?)`, task.plan.ID, task.plan.ID, task.operation.MigrationReport.RequestID).Scan(&count) == nil && count == 0 {
			s.migrationTask = nil
			return failure("MIGRATION_NOT_ACCEPTED", "已核实原迁移没有受理，可重新预览。", true)
		}
	}
	if err != nil {
		r := failure("MIGRATION_RESULT_UNCONFIRMED", "原迁移受理尚未核实，保留原请求与维护保护。", true)
		r.OperationID = task.plan.ID
		return r
	}
	task.plan, task.operation, task.phase, task.acceptancePending = plan, op, phase, false
	// An ambiguous acceptance never launched: only the confirmed accepted phase
	// can start the first worker. Later phases always use recovery instead.
	if phase == "accepted" {
		s.startMigrationWorker(task, s.runMigrationTrial)
	} else {
		s.startMigrationWorker(task, s.recoverMigration)
	}
	return success(map[string]any{"status": "accepted", "operation": copyMigrationOperation(task.operation)}, op.ID)
}

func (s *Service) startMigrationWorker(task *migrationTask, worker func(context.Context, *migrationTask)) {
	ctx, cancel := context.WithCancel(context.Background())
	task.running = true
	task.cancel = cancel
	s.workers.Add(1)
	go func() { defer s.workers.Done(); defer cancel(); worker(ctx, task) }()
}
func (s *Service) cancelMigration(task *migrationTask) Result {
	if task.acceptancePending {
		return failure("MIGRATION_RESULT_UNCONFIRMED", "先核实原受理。", true)
	}
	task.operation = copyMigrationOperation(task.operation)
	task.operation.CancelRequested = true
	if task.cancel != nil {
		task.cancel()
	}
	if !task.running {
		if task.process != nil {
			s.startMigrationWorker(task, func(_ context.Context, t *migrationTask) { s.abortMigrationTrial(t, context.Canceled) })
		} else {
			s.startMigrationWorker(task, s.recoverMigration)
		}
	}
	return success(copyMigrationOperation(task.operation), task.plan.ID)
}

// Reuse the complete, read-only restore preview after this migration finalized.
// No direct pointer rollback, and no opening of upgraded data with the old build.
func (s *Service) selectMigrationRollback(payload json.RawMessage) Result {
	var input struct {
		OperationID string `json:"operationId"`
	}
	if decode(payload, &input) != nil {
		return failure("VALIDATION_FAILED", "请选择原迁移备份。", false)
	}
	s.mu.Lock()
	if !s.recycleIdle() || s.restoreScratch != "" || s.closed || s.closeRequested.Load() {
		s.mu.Unlock()
		return failure("PROFILE_BUSY", "请先完成当前维护。", true)
	}
	plan, _, phase, err := s.readMigrationJournal(input.OperationID)
	if err != nil || phase != "finished" || !plan.BackupVerified {
		s.mu.Unlock()
		return failure("MIGRATION_INCOMPLETE", "升级前完整备份尚未确认。", true)
	}
	token := id()
	path := filepath.Join(s.root, filepath.FromSlash(migrationBackupRef(plan.ID)))
	s.restoreSources[token] = restoreSource{path: path, expires: time.Now().Add(30 * time.Minute), expectedSHA256: plan.BackupSHA256}
	s.mu.Unlock()
	return success(map[string]string{"sourceToken": token, "archiveSha256": plan.BackupSHA256, "name": "before" + backup.Extension}, "")
}
