package workspace

import (
	"path/filepath"
	"strings"
	"time"

	"github.com/axgiroud312-byte/prism-local-browser/internal/backup"
)

// Caller holds s.mu. Expired sources remain identifiable for exact cleanup.
func (s *Service) restoreSourceState(requestID string, owner *restoreSelection) RestoreSourceState {
	state := RestoreSourceState{Mode: "native", RequestID: requestID, Status: owner.status, SourceToken: owner.token, Name: owner.name}
	if owner.token != "" {
		state.PreflightRunning = s.restorePreflight != nil && s.restorePreflight.id == owner.token
		state.CleanupPending = s.restoreSources[owner.token].scratch != ""
		if d := s.restorePreview; d != nil && d.sourceToken == owner.token && time.Now().Before(d.expires) && !state.PreflightRunning && !state.CleanupPending {
			p := d.preview
			state.Preview = &p
		}
	}
	return state
}

func (s *Service) selectRestoreSource(payload []byte) Result {
	var input struct {
		RequestID string `json:"requestId"`
	}
	if decode(payload, &input) != nil || len(input.RequestID) > 128 {
		return failure("VALIDATION_FAILED", "备份只能通过本机文件选择器读取。", false)
	}
	// Legacy callers still receive a host-owned identity; the desktop adapter
	// supplies its original request ID before dispatch so it can recover it.
	if input.RequestID == "" {
		input.RequestID = id()
	}
	s.mu.Lock()
	if owner := s.restoreSelections[input.RequestID]; owner != nil {
		state := s.restoreSourceState(input.RequestID, owner)
		s.mu.Unlock()
		if state.Status == "selecting" {
			return failure("PROFILE_BUSY", "原文件选择尚未结束，请核实原选择请求。", true)
		}
		return success(state, "")
	}
	// Even a refusal is tied to this original request, so a transport loss can
	// later prove that no source was created by it.
	owner := &restoreSelection{status: "failed"}
	s.restoreSelections[input.RequestID] = owner
	if s.closed || s.closeRequested.Load() {
		s.mu.Unlock()
		return failure("NATIVE_UNAVAILABLE", "应用正在退出。", true)
	}
	if s.restorePreflight != nil || s.restoreTask != nil || s.recycleTask != nil || s.migrationTask != nil {
		s.mu.Unlock()
		return failure("PROFILE_BUSY", "先结束原预检或维护任务。", true)
	}
	if s.restoreScratch != "" {
		s.mu.Unlock()
		return failure("BACKUP_PREFLIGHT_FAILED", "原预检暂存清理尚未确认，请先重试原清理；未选择或替换来源。", true)
	}
	for _, prior := range s.restoreSelections {
		if prior.status == "selecting" || prior.status == "selected" {
			s.mu.Unlock()
			return failure("PROFILE_BUSY", "原来源仍保留，请先核实并丢弃原来源。", true)
		}
	}
	for _, source := range s.restoreSources {
		if source.expectedSHA256 != "" {
			s.mu.Unlock()
			return failure("PROFILE_BUSY", "升级前恢复仍拥有原来源，请先核实原任务。", true)
		}
	}
	if s.options.ChooseBackupSource == nil {
		s.mu.Unlock()
		return failure("CAPABILITY_UNSUPPORTED", "本机备份选择器不可用。", false)
	}
	owner.status = "selecting"
	s.workers.Add(1)
	s.mu.Unlock()
	defer s.workers.Done()
	path, err := s.options.ChooseBackupSource()
	s.mu.Lock()
	defer s.mu.Unlock()
	owner.status = "failed"
	if err != nil {
		return preflightFailure(err)
	}
	if path == "" {
		owner.status = "cancelled"
		return success(s.restoreSourceState(input.RequestID, owner), "")
	}
	path, err = filepath.Abs(path)
	if err != nil || strings.HasPrefix(path, `\\`) || !strings.EqualFold(filepath.Ext(path), backup.Extension) {
		return failure("BACKUP_INVALID", "请选择本机 .prismbackup 完整备份；演示 JSON 不能恢复桌面数据。", false)
	}
	if s.closed || s.closeRequested.Load() {
		return failure("NATIVE_UNAVAILABLE", "应用正在退出。", true)
	}
	if s.restorePreflight != nil || s.restoreTask != nil || s.recycleTask != nil || s.migrationTask != nil || s.restoreScratch != "" {
		return failure("PROFILE_BUSY", "先结束原预检或维护任务；未替换来源。", true)
	}
	owner.status, owner.token, owner.name = "selected", id(), filepath.Base(path)
	s.restoreSources[owner.token] = restoreSource{path: path, expires: time.Now().Add(30 * time.Minute)}
	s.restorePreview = nil
	return success(s.restoreSourceState(input.RequestID, owner), "")
}

func (s *Service) readRestoreSource(payload []byte) Result {
	var input struct {
		RequestID string `json:"requestId"`
	}
	if decode(payload, &input) != nil || input.RequestID == "" {
		return failure("VALIDATION_FAILED", "只接受原文件选择请求标识。", false)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed || s.closeRequested.Load() {
		return failure("NATIVE_UNAVAILABLE", "应用正在退出，原来源不接入新的恢复操作。", true)
	}
	owner := s.restoreSelections[input.RequestID]
	if owner == nil {
		return failure("RESTORE_SOURCE_UNCONFIRMED", "未找到原选择请求的可靠所有权记录；未释放或另选来源。", true)
	}
	return success(s.restoreSourceState(input.RequestID, owner), "")
}

// Once accepted, the original task owns its archive. This only consumes that
// exact source; it does not clean scratch or affect any other source.
func (s *Service) consumeRestoreSource(token string) {
	delete(s.restoreSources, token)
	for _, owner := range s.restoreSelections {
		if owner.token == token {
			owner.status, owner.token = "discarded", ""
		}
	}
}
