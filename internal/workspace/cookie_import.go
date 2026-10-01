package workspace

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"sort"
	"strings"
	"time"

	"github.com/axgiroud312-byte/prism-local-browser/internal/cookies"
)

func (s *Service) discardCookieImport(previewID string) {
	if previewID == "" || s.cookieImport != nil && s.cookieImport.preview.PreviewID == previewID {
		s.cookieGeneration++
	}
	if draft := s.cookieImport; draft != nil && (previewID == "" || draft.preview.PreviewID == previewID) {
		if draft.timer != nil {
			draft.timer.Stop()
		}
		for index := range draft.values {
			delete(draft.values, index)
		}
		s.cookieImport = nil
	}
}

func (s *Service) parseCookieImport(payload json.RawMessage) Result {
	var input struct {
		EnvironmentID string `json:"environmentId"`
		Text          string `json:"text"`
	}
	if decode(payload, &input) != nil || input.EnvironmentID == "" {
		return failure("VALIDATION_FAILED", "Cookie预览只接受目标环境与文本，不接受路径/会话覆盖。", false)
	}
	s.mu.Lock()
	if s.closed || s.closeRequested.Load() || s.recycleTask != nil {
		s.mu.Unlock()
		return failure("NATIVE_UNAVAILABLE", "工作区正在关闭，未读取Cookie。", true)
	}
	s.discardCookieImport("")
	generation := s.cookieGeneration
	environment, revision, _, err := s.readEnvironment(input.EnvironmentID)
	if err != nil {
		s.mu.Unlock()
		return failure("NOT_FOUND", "目标环境无法读取，未导入Cookie。", true)
	}
	if s.cookieTasks[input.EnvironmentID] != nil {
		s.mu.Unlock()
		return failure("PROFILE_BUSY", "该环境前一次Cookie写入/结果保存未完成，请先等待。", true)
	}
	slot := s.runtimeSlots[input.EnvironmentID]
	var transport cookieTransport
	sessionID := ""
	if slot != nil && slot.process != nil && slot.session.State == "running" && !slot.session.NeedsReconcile && !slot.session.PersistencePending && slot.session.NetworkFault == nil {
		transport, _ = slot.process.(cookieTransport)
		sessionID = slot.session.SessionID
	}
	s.workers.Add(1)
	s.mu.Unlock()
	defer s.workers.Done()
	parsed := cookies.Parse(input.Text, time.Now())
	input.Text = ""
	preview := CookiePreview{Mode: "native", PreviewID: id(), EnvironmentID: environment.ID, EnvironmentName: environment.Name, ExpectedRevision: revision, SessionID: sessionID, RequiresStart: sessionID == "", Format: parsed.Format, Rows: parsed.Rows, Total: len(parsed.Rows)}
	if transport != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		stored, readErr := transport.ReadCookies(ctx)
		cancel()
		if readErr != nil {
			preview.ObservationError = &Error{Code: "COOKIE_READ_FAILED", Message: "现存Cookie键尚未读取，冲突数未知；不会把未知当0，提交仍须重新核对。", Retryable: true}
		} else {
			keys := map[string]bool{}
			for index := range stored {
				if stored[index].SerializableIdentity() {
					keys[cookies.Key(stored[index].Cookie)] = true
				}
				stored[index].Value = ""
			}
			count := 0
			for index := range preview.Rows {
				if value, ok := parsed.Values[preview.Rows[index].Index]; ok && keys[cookies.Key(value)] {
					preview.Rows[index].ExistingConflict = true
					count++
				}
			}
			preview.ExistingConflictCount = &count
		}
	} else if sessionID != "" {
		preview.ObservationError = &Error{Code: "COOKIE_CONTROL_UNAVAILABLE", Message: "当前受控会话尚无Cookie能力，现存冲突数未知；不使用通用CDP或示例数据代替。", Retryable: false}
	}
	for _, row := range preview.Rows {
		if row.ErrorCode != "" {
			preview.ErrorCount++
		} else {
			preview.ValidCount++
		}
		if row.Expired {
			preview.ExpiredCount++
		}
		if row.Conflict {
			preview.ConflictCount++
		}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed || s.closeRequested.Load() || s.recycleTask != nil || generation != s.cookieGeneration {
		return failure("PREVIEW_EXPIRED", "此Cookie预览已被更新输入或退出作废，没有保存秘密。", true)
	}
	_, currentRevision, _, err := s.readEnvironment(environment.ID)
	if err != nil || currentRevision != revision {
		return failure("REVISION_CONFLICT", "解析期间目标环境修订变化，请重新预览，未写入。", true)
	}
	if sessionID != "" && (s.runtimeSlots[environment.ID] != slot || slot.session.SessionID != sessionID || slot.session.State != "running") {
		return failure("COOKIE_SESSION_CHANGED", "读取期间目标会话已改变，请重新预览，未写入。", true)
	}
	expires := time.Now().Add(15 * time.Minute)
	preview.ExpiresAt = expires.UTC().Format(time.RFC3339Nano)
	draft := &cookieImportDraft{preview: preview, values: parsed.Values, expires: expires}
	s.cookieImport = draft
	draft.timer = time.AfterFunc(time.Until(expires), func() {
		s.mu.Lock()
		defer s.mu.Unlock()
		if s.cookieImport == draft {
			s.discardCookieImport(preview.PreviewID)
		}
	})
	return success(preview, "")
}

func (s *Service) cookieCall(request Request) Result {
	if request.Method == "Cookie.DiscardImport" {
		var input struct {
			PreviewID string `json:"previewId"`
		}
		if decode(request.Payload, &input) != nil {
			return failure("VALIDATION_FAILED", "Cookie预览标识无效。", false)
		}
		s.discardCookieImport(input.PreviewID)
		return success(map[string]string{"status": "discarded"}, "")
	}
	var input CookieCommit
	if decode(request.Payload, &input) != nil || input.EnvironmentID == "" || input.SessionID == "" || input.PreviewID == "" || input.ExpectedRevision < 1 || strings.TrimSpace(input.RequestID) == "" || len(input.RequestID) > 128 || (input.Policy != "merge" && input.Policy != "replace-all") || len(input.SelectedRows) == 0 {
		return failure("VALIDATION_FAILED", "导入须指定环境/修订/受控会话、预览、所选行及明确策略，不能覆盖网络或CDP路径。", false)
	}
	// Only metadata is hashed; Cookie values never enter requests/operations.
	encoded, _ := json.Marshal(input)
	digest := sha256.Sum256(append([]byte("Cookie.CommitImport\x00"), encoded...))
	signature := hex.EncodeToString(digest[:])
	if prior, found := s.priorProxyRequest(input.RequestID, signature); found {
		if prior.OK && prior.OperationID != "" {
			if op, ok := s.cookieResults[prior.OperationID]; ok {
				return success(map[string]any{"status": "accepted", "operation": op}, op.ID)
			}
		}
		return prior
	}
	if s.backupUses[input.EnvironmentID] != nil {
		return failure("PROFILE_BUSY", "此环境已预约完整备份；备份期间不接受新的Cookie写入或清空。", true)
	}
	draft := s.cookieImport
	if draft == nil || draft.preview.PreviewID != input.PreviewID || !time.Now().Before(draft.expires) {
		return failure("PREVIEW_EXPIRED", "Cookie预览已失效；请重新读取原文件/输入，不重放浏览器写入。", true)
	}
	if draft.preview.EnvironmentID != input.EnvironmentID || draft.preview.ExpectedRevision != input.ExpectedRevision {
		return failure("REVISION_CONFLICT", "预览不属于当前目标环境修订，未写其他环境。", false)
	}
	_, revision, _, err := s.readEnvironment(input.EnvironmentID)
	if err != nil || revision != input.ExpectedRevision {
		return failure("REVISION_CONFLICT", "目标环境修订已改变，请重新预览。", true)
	}
	slot := s.runtimeSlots[input.EnvironmentID]
	if slot == nil || slot.process == nil || slot.session.State != "running" || slot.session.SessionID != input.SessionID || slot.session.NeedsReconcile || slot.session.PersistencePending || slot.session.NetworkFault != nil || runtimeStopPending(slot) {
		return failure("COOKIE_SESSION_REQUIRED", "须先明确启动并确认指定环境的当前受控会话；没有自动启动、直连回退或写入示例记录。", true)
	}
	if draft.preview.SessionID != "" && draft.preview.SessionID != input.SessionID {
		return failure("COOKIE_SESSION_CHANGED", "预览的受控会话已改变，须重新预览。", true)
	}
	transport, ok := slot.process.(cookieTransport)
	if !ok {
		return failure("COOKIE_CONTROL_UNAVAILABLE", "当前内核会话未提供受控Cookie能力，没有开放通用CDP。", false)
	}
	if s.cookieTasks[input.EnvironmentID] != nil {
		return failure("PROFILE_BUSY", "该环境的Cookie导入或结果保存仍在进行。", true)
	}
	if input.Policy == "replace-all" && draft.clearAttempted {
		return failure("COOKIE_CLEAR_ALREADY_ATTEMPTED", "此预览已受理过清空，重试只能合并失败项；再次清空请重新预览并明确选择。", false)
	}
	selected := map[int]bool{}
	keys := map[string]bool{}
	values := map[int]cookies.Cookie{}
	rows := map[int]cookies.Row{}
	for _, row := range draft.preview.Rows {
		rows[row.Index] = row
	}
	for _, index := range input.SelectedRows {
		value, exists := draft.values[index]
		if !exists || selected[index] || rows[index].Expired {
			return failure("COOKIE_SELECTION_INVALID", "所选行无效、重复或已过期，请明确选择可导入记录。", false)
		}
		if keys[cookies.Key(value)] {
			return failure("COOKIE_DUPLICATE_SELECTED", "所选输入含完全相同Cookie键，请明确只选一条，未静默覆盖。", false)
		}
		selected[index], keys[cookies.Key(value)], values[index] = true, true, value
	}
	indices := append([]int(nil), input.SelectedRows...)
	sort.Ints(indices)
	report := &CookieImportReport{Mode: "native", PreviewID: input.PreviewID, EnvironmentID: input.EnvironmentID, SessionID: input.SessionID, Revision: revision, Policy: input.Policy, ClearState: "not-requested", Items: []CookieItemResult{}}
	if input.Policy == "replace-all" {
		report.ClearState = "pending"
	}
	for _, index := range indices {
		report.Items = append(report.Items, CookieItemResult{Row: rows[index], Status: "pending"})
	}
	operation := Operation{ID: id(), Kind: "cookie-import", State: "accepted", Stage: "queued", EnvironmentID: input.EnvironmentID, SessionID: input.SessionID, Total: len(indices), CompletedIDs: []string{}, CookieReport: report}
	result := success(map[string]any{"status": "accepted", "operation": operation}, operation.ID)
	tx, err := s.db.Begin()
	if err != nil {
		return storageFailure(err)
	}
	defer tx.Rollback()
	encoded, _ = json.Marshal(operation)
	if _, err = tx.Exec("INSERT INTO operations(id,result_json) VALUES(?,?)", operation.ID, string(encoded)); err != nil {
		return storageFailure(err)
	}
	if err = s.commitProxyTransaction(tx, input.RequestID, signature, result, ""); err != nil {
		return storageFailure(err)
	}
	if input.Policy == "replace-all" {
		draft.clearAttempted = true
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	task := &cookieImportTask{operation: operation, cancel: cancel, slot: slot, transport: transport, values: values}
	s.cookieTasks[input.EnvironmentID], s.cookieResults[operation.ID], s.profileUses[input.EnvironmentID] = task, operation, true
	s.workers.Add(1)
	go s.runCookieImport(ctx, task)
	return result
}
