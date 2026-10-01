package workspace

import (
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"github.com/axgiroud312-byte/prism-local-browser/internal/kernel"
	"github.com/google/uuid"
)

func (s *Service) migrateRuntimeSessions() error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, statement := range []string{
		`CREATE TABLE runtime_sessions(environment_id TEXT PRIMARY KEY REFERENCES environments(id),record_json TEXT NOT NULL)`,
		`CREATE TABLE runtime_events(activity_id TEXT PRIMARY KEY REFERENCES activities(id),environment_id TEXT NOT NULL REFERENCES environments(id),session_id TEXT NOT NULL,error_code TEXT NOT NULL,next_action TEXT NOT NULL)`,
		`PRAGMA user_version=4`,
	} {
		if _, err = tx.Exec(statement); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *Service) checkRuntimeSchema() error {
	for _, statement := range []string{"SELECT environment_id,record_json FROM runtime_sessions LIMIT 0", "SELECT activity_id,environment_id,session_id,error_code,next_action FROM runtime_events LIMIT 0"} {
		rows, err := s.db.Query(statement)
		if err != nil {
			return err
		}
		if err = rows.Close(); err != nil {
			return err
		}
	}
	return nil
}

var errRuntimeSessionChanged = errors.New("runtime session generation changed")

type runtimePersistenceFailure struct{ cause error }

func (err *runtimePersistenceFailure) Error() string { return "runtime state persistence failed" }
func (err *runtimePersistenceFailure) Unwrap() error { return err.cause }

type runtimePendingEvent struct {
	ID, Action, Time string
	Session          RuntimeSession
}
type runtimePendingWrite struct {
	Slot       *runtimeSlot
	Session    RuntimeSession
	Operations map[string]Operation
	Events     []runtimePendingEvent
}

func saveRuntimeSession(tx *sql.Tx, session RuntimeSession, replace bool) error {
	encoded, err := json.Marshal(session)
	if err != nil {
		return err
	}
	if replace {
		_, err = tx.Exec("INSERT INTO runtime_sessions(environment_id,record_json) VALUES(?,?) ON CONFLICT(environment_id) DO UPDATE SET record_json=excluded.record_json", session.EnvironmentID, string(encoded))
		return err
	}
	updated, err := tx.Exec("UPDATE runtime_sessions SET record_json=? WHERE environment_id=? AND json_extract(record_json,'$.sessionId')=?", string(encoded), session.EnvironmentID, session.SessionID)
	if err != nil {
		return err
	}
	count, err := updated.RowsAffected()
	if err != nil {
		return err
	}
	if count != 1 {
		return errRuntimeSessionChanged
	}
	return nil
}

// A durable observation and its actionable, redacted activity are one commit.
// Raw OS errors, executable paths, URLs and control handles never enter logs.
func (s *Service) persistRuntime(slot *runtimeSlot, operation *Operation, action string) error {
	if s.runtimeSlots[slot.session.EnvironmentID] != slot {
		return errRuntimeSessionChanged
	}
	pending := s.runtimePending[slot.session.EnvironmentID]
	if pending == nil || pending.Slot != slot {
		pending = &runtimePendingWrite{Slot: slot, Operations: map[string]Operation{}}
	}
	pending.Session = slot.session
	pending.Session.PersistencePending = false
	if operation != nil {
		wanted := *operation
		wanted.PersistencePending = false
		pending.Operations[operation.ID] = wanted
	}
	if action != "" {
		pending.Events = append(pending.Events, runtimePendingEvent{ID: id(), Action: action, Time: timestamp(), Session: pending.Session})
	}
	s.runtimePending[slot.session.EnvironmentID] = pending
	err := s.flushOneRuntimeWrite(pending)
	if err != nil && operation != nil {
		if failed, exists := s.runtimeResults[operation.ID]; exists {
			*operation = failed
		}
	}
	if err != nil {
		return &runtimePersistenceFailure{cause: err}
	}
	return nil
}

func (s *Service) flushOneRuntimeWrite(pending *runtimePendingWrite) error {
	slot := pending.Slot
	if s.runtimeSlots[pending.Session.EnvironmentID] != slot {
		delete(s.runtimePending, pending.Session.EnvironmentID)
		return errRuntimeSessionChanged
	}
	if err := s.commitRuntimeWrite(pending); err != nil {
		slot.session.State, slot.session.Error, slot.session.PersistencePending = "error", storageFailure(err).Error, true
		slot.session.NextAction = "实际结果尚未写入数据库，仍保留保护；修复存储后重新读取可重试保存，不会重复外部启停操作。"
		s.profileUses[slot.session.EnvironmentID] = true
		for operationID, wanted := range pending.Operations {
			wanted.State, wanted.Stage, wanted.Error = "failed", "storage-pending", storageFailure(err).Error
			wanted.PersistencePending = true
			wanted.Error.Details = map[string]any{"reason": "runtime-state-unpersisted"}
			s.runtimeResults[operationID] = wanted
		}
		return err
	}
	slot.session = pending.Session
	for operationID, wanted := range pending.Operations {
		delete(s.runtimeResults, operationID)
		if slot.start.ID == operationID {
			slot.start = wanted
		}
		if slot.stop != nil && slot.stop.ID == operationID {
			*slot.stop = wanted
		}
		if slot.reconcile != nil && slot.reconcile.ID == operationID {
			*slot.reconcile = wanted
		}
	}
	delete(s.runtimePending, slot.session.EnvironmentID)
	reconciling := slot.reconcile != nil && (slot.reconcile.State == "accepted" || slot.reconcile.State == "running")
	if slot.process == nil && !slot.session.NeedsReconcile && !reconciling && (slot.session.State == "ready" || slot.session.State == "error") {
		delete(s.profileUses, slot.session.EnvironmentID)
	}
	return nil
}

func (s *Service) flushRuntimePersistence() {
	for _, pending := range s.runtimePending {
		_ = s.flushOneRuntimeWrite(pending)
	}
}

func (s *Service) commitRuntimeWrite(pending *runtimePendingWrite) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err = saveRuntimeSession(tx, pending.Session, false); err != nil {
		return err
	}
	for _, operation := range pending.Operations {
		operation.PersistencePending = false
		encoded, encodeErr := json.Marshal(operation)
		if encodeErr != nil {
			return encodeErr
		}
		if _, err = tx.Exec("UPDATE operations SET result_json=? WHERE id=?", string(encoded), operation.ID); err != nil {
			return err
		}
	}
	for _, event := range pending.Events {
		detail, code := "本次会话状态已核对；固定档案和浏览数据保持。", ""
		if event.Session.Error != nil {
			detail, code = event.Session.Error.Message, event.Session.Error.Code
		}
		if _, err = tx.Exec("INSERT INTO activities(id,created_at,action,target,detail) VALUES(?,?,?,?,?) ON CONFLICT(id) DO NOTHING", event.ID, event.Time, event.Action, event.Session.EnvironmentID, detail); err != nil {
			return err
		}
		if _, err = tx.Exec("INSERT INTO runtime_events(activity_id,environment_id,session_id,error_code,next_action) VALUES(?,?,?,?,?) ON CONFLICT(activity_id) DO NOTHING", event.ID, event.Session.EnvironmentID, event.Session.SessionID, code, event.Session.NextAction); err != nil {
			return err
		}
	}
	if s.options.BeforeCommit != nil {
		if err = s.options.BeforeCommit(); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func validateSavedRuntime(session RuntimeSession, environmentID string) error {
	parseID := func(value string) bool {
		parsed, err := uuid.Parse(value)
		return err == nil && parsed.String() == value
	}
	ref, err := dataReference(environmentID)
	if err != nil || session.Mode != "native" || session.EnvironmentID != environmentID || !parseID(session.SessionID) || !parseID(session.OperationID) || session.UserDataRef != ref || (session.NetworkPolicy != "direct" && session.NetworkPolicy != "proxy") || session.Revision < 1 || session.FingerprintRevision < 1 || session.PID < 0 || session.RootPID < 0 {
		return errors.New("invalid saved runtime identity")
	}
	if session.NetworkPolicy == "proxy" && (!parseID(session.ProxyID) || !parseID(session.ProxyChannelID) || session.ProxyRevision < 1) || session.NetworkPolicy == "direct" && (session.ProxyID != "" || session.ProxyChannelID != "" || session.ProxyReport != nil) {
		return errors.New("invalid saved proxy channel identity")
	}
	if report := session.ProxyReport; report != nil && (report.Mode != "native" || report.ChannelID != session.ProxyChannelID || report.ProxyID != session.ProxyID || report.Revision != session.ProxyRevision) {
		return errors.New("saved preflight does not match the proxy session")
	}
	if session.State != "ready" && session.State != "starting" && session.State != "running" && session.State != "stopping" && session.State != "error" {
		return errors.New("invalid saved runtime state")
	}
	if session.RootPID > 0 {
		if _, err := time.Parse(time.RFC3339Nano, session.ProcessCreatedAt); err != nil && !(session.ProcessCreatedAt == "" && session.LaunchStage == "identity-unconfirmed") {
			return errors.New("invalid saved process creation time")
		}
	}
	if session.PID > 0 && session.PID != session.RootPID {
		return errors.New("saved active PID does not match the root identity")
	}
	return nil
}

func (s *Service) inspectSavedRuntime(session RuntimeSession) (kernel.ManagedRecovery, error) {
	if s.options.InspectRuntime != nil {
		return s.options.InspectRuntime(session)
	}
	if session.ResourceVersion != kernel.ManagedRuntimeVersion {
		return kernel.ManagedRecovery{ProcessState: "unconfirmed"}, &kernel.Problem{Code: "SESSION_IDENTITY_UNCONFIRMED", Message: "旧记录没有可核对的进程树身份，暂不解除数据保护；未接管或结束任何进程。", Retryable: true}
	}
	return kernel.InspectManagedProfile(s.root, session.EnvironmentID, session.SessionID, session.UserDataRef, session.RootPID, session.ProcessCreatedAt, session.LaunchStage == "queued" || session.LaunchStage == "proxy-preflight" || session.LaunchStage == "no-process-created")
}

func (s *Service) reconcileRuntimeSlot(slot *runtimeSlot) error {
	recovery, inspectionErr := s.inspectSavedRuntime(slot.session)
	return s.applyReconciledRuntime(slot, recovery, inspectionErr, nil)
}

func (s *Service) applyReconciledRuntime(slot *runtimeSlot, recovery kernel.ManagedRecovery, inspectionErr error, operation *Operation) error {
	if s.runtimeSlots[slot.session.EnvironmentID] != slot {
		return errRuntimeSessionChanged
	}
	if recovery.RootPID > 0 {
		slot.session.RootPID, slot.session.ProcessCreatedAt = recovery.RootPID, recovery.ProcessCreatedAt
		if recovery.ProcessCreatedAt == "" {
			slot.session.LaunchStage = "identity-unconfirmed"
		}
	}
	slot.session.CanControl, slot.session.CanForce = false, false
	slot.session.ReconciledAt = timestamp()
	safeProcessState := recovery.ProcessState == "not-created" || recovery.ProcessState == "exited" || recovery.ProcessState == "reused"
	if inspectionErr != nil || !recovery.DirectoryFree || !recovery.ResourcesExited || !recovery.SessionMatches || !safeProcessState {
		slot.session.State, slot.session.NeedsReconcile = "error", true
		slot.session.Error = &Error{Code: "SESSION_IDENTITY_UNCONFIRMED", Message: "应用重开后尚未确认原会话已退出，仍保护目录；没有接管或结束其他进程。", Retryable: true}
		slot.session.NextAction = "请正常关闭原浏览器后重新核对；不会按锁文件年龄清理或按PID强制结束。"
		if inspectionErr != nil {
			var p *kernel.Problem
			if errors.As(inspectionErr, &p) {
				slot.session.Error = kernelFailure(inspectionErr).Error
			}
		}
		if !recovery.DirectoryFree && inspectionErr == nil {
			slot.session.Error.Code, slot.session.Error.Message = "DATA_DIR_LOCKED", "实际浏览数据目录仍被占用，尚未确认停止；没有删除旧锁或浏览数据。"
		}
		s.profileUses[slot.session.EnvironmentID] = true
	} else {
		priorError := slot.session.Error
		slot.session.PID, slot.session.NeedsReconcile = 0, false
		slot.session.State, slot.session.Error = "ready", nil
		slot.session.NextAction = "实际进程身份和目录锁已核对，可使用原档案重试启动。"
		if recovery.ProcessState == "reused" {
			slot.session.State = "error"
			slot.session.Error = &Error{Code: "PROCESS_ID_REUSED", Message: "旧PID已经属于另一创建时间的进程；没有操作该进程，原环境目录已确认空闲。", Retryable: true}
		} else if slot.start.State == "accepted" || slot.start.State == "running" || (slot.stop != nil && (slot.stop.State == "accepted" || slot.stop.State == "running")) {
			slot.session.State = "error"
			slot.session.Error = &Error{Code: "APPLICATION_INTERRUPTED", Message: "上次操作因应用中断未完成；原进程和目录已确认不再占用，可重试。", Retryable: true}
		} else if priorError != nil && (priorError.Code == "PROCESS_CRASHED" || priorError.Code == "PROCESS_READY_TIMEOUT" || priorError.Code == "PROCESS_START_FAILED") {
			slot.session.State, slot.session.Error = "error", priorError
			slot.session.NextAction = "原进程和目录已确认空闲；保留上次异常原因，修复后可使用原档案重试。"
		}
		delete(s.profileUses, slot.session.EnvironmentID)
	}
	if operation != nil {
		operation.State, operation.Stage, operation.CompletedIDs = "completed", "identity-checked", []string{slot.session.EnvironmentID}
	}
	if err := s.persistRuntime(slot, operation, "重开核对会话"); err != nil {
		slot.session.State, slot.session.Error, slot.session.NeedsReconcile = "error", storageFailure(err).Error, true
		slot.session.NextAction = "核对结果尚未写入本机数据库，仍保留保护；修复存储后重新核对，不假报完成。"
		s.profileUses[slot.session.EnvironmentID] = true
		return err
	}
	return nil
}

func (s *Service) recoverRuntimeSessions() error {
	rows, err := s.db.Query("SELECT environment_id,record_json FROM runtime_sessions")
	if err != nil {
		return err
	}
	sessions := []RuntimeSession{}
	for rows.Next() {
		var environmentID, encoded string
		if err = rows.Scan(&environmentID, &encoded); err != nil {
			rows.Close()
			return err
		}
		var session RuntimeSession
		if err = decode(json.RawMessage(encoded), &session); err != nil {
			rows.Close()
			return err
		}
		if err = validateSavedRuntime(session, environmentID); err != nil {
			rows.Close()
			return err
		}
		sessions = append(sessions, session)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	for _, session := range sessions {
		environment, revision, profileID, err := s.readEnvironment(session.EnvironmentID)
		if err != nil {
			return err
		}
		profile, err := readProfileFrom(s.db, profileID)
		if err != nil {
			return err
		}
		if session.KernelID != environment.CoreID || session.Revision > revision || session.FingerprintRevision > profile.Profile.ConfigRevision {
			return errors.New("saved runtime references do not match the environment")
		}
		var encoded string
		if err = s.db.QueryRow("SELECT result_json FROM operations WHERE id=?", session.OperationID).Scan(&encoded); err != nil {
			return err
		}
		var start Operation
		if err = decode(json.RawMessage(encoded), &start); err != nil || start.EnvironmentID != session.EnvironmentID || start.SessionID != session.SessionID || start.Kind != "runtime-start" {
			return errors.New("saved runtime operation identity does not match")
		}
		done := make(chan struct{})
		close(done)
		slot := &runtimeSlot{session: session, start: start, cancel: func() {}, launchDone: done}
		var stopJSON string
		if err = s.db.QueryRow("SELECT result_json FROM operations WHERE json_extract(result_json,'$.sessionId')=? AND json_extract(result_json,'$.kind') IN ('runtime-stop','runtime-force-stop') ORDER BY rowid DESC LIMIT 1", session.SessionID).Scan(&stopJSON); err == nil {
			var stop Operation
			if err = decode(json.RawMessage(stopJSON), &stop); err != nil || stop.EnvironmentID != session.EnvironmentID {
				return errors.New("saved stop operation identity does not match")
			}
			slot.stop = &stop
		} else if err != sql.ErrNoRows {
			return err
		}
		s.runtimeSlots[session.EnvironmentID] = slot
		if err = s.reconcileRuntimeSlot(slot); err != nil {
			return err
		}
	}
	return s.recoverPendingRuntimeOperations()
}

func (s *Service) recoverPendingRuntimeOperations() error {
	rows, err := s.db.Query("SELECT result_json FROM operations WHERE json_extract(result_json,'$.kind') LIKE 'runtime-%' AND (json_extract(result_json,'$.state') IN ('accepted','running') OR json_extract(result_json,'$.persistencePending')=1)")
	if err != nil {
		return err
	}
	operations := []Operation{}
	for rows.Next() {
		var encoded string
		if err = rows.Scan(&encoded); err != nil {
			rows.Close()
			return err
		}
		var operation Operation
		if err = decode(json.RawMessage(encoded), &operation); err != nil {
			rows.Close()
			return err
		}
		operations = append(operations, operation)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	for _, operation := range operations {
		operation.State, operation.Stage = "failed", "application-interrupted"
		operation.PersistencePending = false
		operation.Error = &Error{Code: "APPLICATION_INTERRUPTED", Message: "应用重开后已重新核对；旧受理记录不代表操作完成，请查看当前会话状态再重试。", Retryable: true}
		if err = s.storeOperation(operation); err != nil {
			return err
		}
		if slot := s.runtimeSlots[operation.EnvironmentID]; slot != nil && slot.session.SessionID == operation.SessionID {
			if slot.start.ID == operation.ID {
				slot.start = operation
			}
			if slot.stop != nil && slot.stop.ID == operation.ID {
				*slot.stop = operation
			}
		}
	}
	return nil
}
