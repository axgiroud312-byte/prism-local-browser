package workspace

import (
	"database/sql"
	"encoding/json"
	"errors"
	"sort"
	"strings"
	"time"

	"github.com/axgiroud312-byte/prism-local-browser/internal/proxy"
)

func validProxyRequestID(value string) bool {
	return strings.TrimSpace(value) != "" && len(value) <= 128
}

func (s *Service) proxyCall(request Request) Result {
	if request.Method == "Proxy.ParseImport" {
		var input struct {
			Text string `json:"text"`
		}
		if decode(request.Payload, &input) != nil {
			return failure("VALIDATION_FAILED", "导入只接受文本内容，不接受路径、状态或凭据引用。", false)
		}
		return s.parseProxyImport(input.Text)
	}
	if request.Method == "Proxy.DiscardImport" {
		var input struct {
			PreviewID string `json:"previewId"`
		}
		if decode(request.Payload, &input) != nil {
			return failure("VALIDATION_FAILED", "导入预览标识无效。", false)
		}
		if s.proxyImport != nil && s.proxyImport.id == input.PreviewID {
			s.discardProxyImport()
		}
		return success(map[string]string{"status": "discarded"}, "")
	}
	var input any
	var requestID string
	switch request.Method {
	case "Proxy.CommitImport":
		var value ProxyCommitImport
		if decode(request.Payload, &value) != nil {
			return failure("VALIDATION_FAILED", "导入提交字段无效；请选择服务预览中的有效行。", false)
		}
		input, requestID = value, value.RequestID
	case "Proxy.Update":
		var value ProxyUpdate
		if decode(request.Payload, &value) != nil {
			return failure("VALIDATION_FAILED", "代理编辑字段无效；不能提交旧检查结果或凭据引用。", false)
		}
		input, requestID = value, value.RequestID
	case "Proxy.Delete", "Proxy.Check":
		var value ProxyTarget
		if decode(request.Payload, &value) != nil {
			return failure("VALIDATION_FAILED", "请选择代理及当前修订，不接受目标URL、命令或跳过TLS选项。", false)
		}
		input, requestID = value, value.RequestID
	default:
		return failure("CAPABILITY_UNSUPPORTED", "代理操作尚未接入。", false)
	}
	if !validProxyRequestID(requestID) {
		return failure("VALIDATION_FAILED", "请提供有效请求标识。", false)
	}
	signature, err := s.proxySignature(request.Method, input)
	if err != nil {
		return failure("CREDENTIALS_UNAVAILABLE", "本机受保护请求凭据不可用；没有执行或降级为明文保存。", true)
	}
	if prior, exists := s.priorProxyRequest(requestID, signature); exists {
		return prior
	}
	switch value := input.(type) {
	case ProxyCommitImport:
		return s.commitProxyImport(value, signature)
	case ProxyUpdate:
		return s.updateProxy(value, signature)
	case ProxyTarget:
		record, ref, err := s.savedProxy(value.ProxyID)
		if errors.Is(err, sql.ErrNoRows) {
			return failure("NOT_FOUND", "所选代理已不存在，请刷新。", true)
		}
		if err != nil {
			return failure("STORAGE_READ_FAILED", "代理配置读取失败，原记录保持。", true)
		}
		if value.ExpectedRevision != record.Revision {
			return failure("REVISION_CONFLICT", "代理已更新，请刷新后重试。", true)
		}
		if request.Method == "Proxy.Check" {
			return s.beginProxyCheck(value, record, ref, signature)
		}
		return s.deleteProxy(value, record, ref, signature)
	}
	return failure("VALIDATION_FAILED", "代理请求无效。", false)
}

func (s *Service) parseProxyImport(text string) Result {
	s.discardProxyImport() // at most one current secret-bearing import dialog
	candidates, ignored, err := proxy.ParseImport(text)
	if err != nil {
		return failure("PROXY_INVALID", err.Error(), false)
	}
	records, err := s.listProxies()
	if err != nil {
		return failure("STORAGE_READ_FAILED", "重复候选无法核对，请修复存储后重试。", true)
	}
	draft := &proxyImportDraft{id: id(), expiresAt: time.Now().Add(15 * time.Minute), rows: map[int]proxy.Candidate{}}
	preview := ProxyImportPreview{Mode: "native", PreviewID: draft.id, ExpiresAt: draft.expiresAt.UTC().Format(time.RFC3339Nano), IgnoredLines: ignored, Rows: []ProxyImportRow{}, DuplicateGroups: map[string]ProxyDuplicateGroup{}}
	byEndpoint := map[string][]int{}
	existing := map[string][]string{}
	for _, record := range records {
		key := proxy.Endpoint(record.Configuration)
		existing[key] = append(existing[key], record.ID)
	}
	for _, row := range candidates {
		if row.Error == "" {
			byEndpoint[proxy.Endpoint(row.Configuration)] = append(byEndpoint[proxy.Endpoint(row.Configuration)], row.Line)
		}
	}
	groupByEndpoint := map[string]string{}
	for key, lines := range byEndpoint {
		if len(lines) > 1 || len(existing[key]) > 0 {
			groupID := id()
			groupByEndpoint[key] = groupID
			prior := existing[key]
			if prior == nil {
				prior = []string{}
			}
			preview.DuplicateGroups[groupID] = ProxyDuplicateGroup{Lines: lines, ExistingProxyIDs: prior}
		}
	}
	for _, candidate := range candidates {
		row := ProxyImportRow{Line: candidate.Line, Error: candidate.Error, HasAuthentication: candidate.Credentials != nil}
		if candidate.Error == "" {
			config := candidate.Configuration
			row.Configuration = &config
			key := proxy.Endpoint(config)
			row.DuplicateGroupID, row.DuplicateCount, row.ExistingCount = groupByEndpoint[key], len(byEndpoint[key])-1, len(existing[key])
		}
		preview.Rows = append(preview.Rows, row)
		draft.rows[row.Line] = candidate
	}
	s.proxyImport = draft
	draft.timer = time.AfterFunc(time.Until(draft.expiresAt), func() {
		s.mu.Lock()
		defer s.mu.Unlock()
		if s.proxyImport == draft {
			s.discardProxyImport()
		}
	})
	return success(preview, "")
}

func (s *Service) commitProxyImport(input ProxyCommitImport, signature string) Result {
	draft := s.proxyImport
	if draft == nil || draft.id != input.PreviewID || !time.Now().Before(draft.expiresAt) {
		if draft != nil && draft.id == input.PreviewID {
			s.discardProxyImport()
		}
		return failure("PREVIEW_EXPIRED", "导入预览已失效，请重新解析；没有保存任何行。", true)
	}
	if len(input.SelectedRows) == 0 {
		return failure("VALIDATION_FAILED", "请选择至少一条有效行。", false)
	}
	seen := map[int]bool{}
	lines := append([]int(nil), input.SelectedRows...)
	sort.Ints(lines)
	for _, line := range lines {
		row, exists := draft.rows[line]
		if !exists || row.Error != "" || seen[line] {
			return failure("VALIDATION_FAILED", "所选行不存在、重复选择或格式错误，未保存此批次。", false)
		}
		seen[line] = true
	}
	tx, err := s.db.Begin()
	if err != nil {
		return storageFailure(err)
	}
	defer tx.Rollback()
	imported := []string{}
	for _, line := range lines {
		row := draft.rows[line]
		ref, err := s.insertProxyCredentials(tx, row.Credentials)
		if err != nil {
			return failure("CREDENTIALS_UNAVAILABLE", "认证保护或保存失败；本批次没有提交，不会明文降级。", true)
		}
		proxyID := id()
		encoded, _ := json.Marshal(row.Configuration)
		if _, err := tx.Exec("INSERT INTO proxies(id,credential_ref) VALUES(?,?)", proxyID, nullable(ref)); err != nil {
			return storageFailure(err)
		}
		if _, err := tx.Exec("INSERT INTO proxy_config(proxy_id,config_json,revision) VALUES(?,?,1)", proxyID, string(encoded)); err != nil {
			return storageFailure(err)
		}
		imported = append(imported, proxyID)
	}
	result := success(map[string]any{"status": "completed", "importedIds": imported, "importedLines": lines}, "")
	if err := s.commitProxyTransaction(tx, input.RequestID, signature, result, "导入代理"); err != nil {
		return storageFailure(err)
	}
	s.discardProxyImport()
	return result
}

func (s *Service) updateProxy(input ProxyUpdate, signature string) Result {
	config, err := proxy.Normalize(input.Configuration)
	if err != nil {
		return failure("PROXY_INVALID", err.Error(), false)
	}
	change := input.Credentials
	if change.Action != "keep" && change.Action != "clear" && change.Action != "replace" || change.Action != "replace" && (change.Username != "" || change.Password != "") {
		return failure("VALIDATION_FAILED", "请选择保留、替换或清除认证；保留和清除不能携带凭据。", false)
	}
	var credentials *proxy.Credentials
	if change.Action == "replace" {
		value := proxy.Credentials{Username: change.Username, Password: change.Password}
		if err := proxy.ValidateProtocolCredentials(config.Type, value); err != nil {
			var protocolError *proxy.CheckError
			if errors.As(err, &protocolError) {
				return failure(protocolError.Code, protocolError.Message, protocolError.Retryable)
			}
			return failure("PROXY_INVALID", "新认证字段无效；没有保存。", false)
		}
		credentials = &value
	}
	// keep never reads or rewrites a secret (including metadata-only edits).
	// If a protocol change makes old credentials incompatible, the protected
	// check/start path rejects them before dialing. Never silently clear them.
	record, oldRef, err := s.savedProxy(input.ProxyID)
	if errors.Is(err, sql.ErrNoRows) {
		return failure("NOT_FOUND", "代理已不存在，请刷新。", true)
	}
	if err != nil {
		return failure("STORAGE_READ_FAILED", "代理配置无法读取。", true)
	}
	if record.Revision != input.ExpectedRevision {
		return failure("REVISION_CONFLICT", "代理已更新，请重新加载；旧编辑没有覆盖新配置。", true)
	}
	if s.proxyChecks[input.ProxyID] != nil {
		return failure("PROFILE_BUSY", "该代理检查或结果保存尚未结束，请等待或取消。", true)
	}
	affectsNetwork := config.Type != record.Type || config.Host != record.Host || config.Port != record.Port || change.Action != "keep"
	if affectsNetwork {
		for _, environmentID := range record.UsedBy {
			if s.profileUses[environmentID] {
				return failure("PROFILE_BUSY", "绑定环境仍运行或待核对，请先停止并核对后修改连接或认证。", true)
			}
		}
	}
	tx, err := s.db.Begin()
	if err != nil {
		return storageFailure(err)
	}
	defer tx.Rollback()
	ref := oldRef
	if change.Action != "keep" {
		ref, err = s.insertProxyCredentials(tx, credentials)
		if err != nil {
			return failure("CREDENTIALS_UNAVAILABLE", "新凭据保护失败，原配置和认证均保持。", true)
		}
	}
	encoded, _ := json.Marshal(config)
	updated, err := tx.Exec("UPDATE proxy_config SET config_json=?,revision=revision+1,check_json=NULL WHERE proxy_id=? AND revision=?", string(encoded), input.ProxyID, input.ExpectedRevision)
	if err != nil {
		return storageFailure(err)
	}
	if count, _ := updated.RowsAffected(); count != 1 {
		return failure("REVISION_CONFLICT", "代理修订已改变，没有覆盖。", true)
	}
	if _, err := tx.Exec("UPDATE proxies SET credential_ref=? WHERE id=?", nullable(ref), input.ProxyID); err != nil {
		return storageFailure(err)
	}
	if oldRef != "" && ref != oldRef {
		if _, err := tx.Exec("DELETE FROM proxy_credentials WHERE ref=? AND NOT EXISTS(SELECT 1 FROM proxies WHERE credential_ref=?)", oldRef, oldRef); err != nil {
			return storageFailure(err)
		}
	}
	record.Configuration, record.Revision, record.HasAuthentication, record.Status, record.CheckReport = config, record.Revision+1, ref != "", "unchecked", nil
	result := success(map[string]any{"status": "completed", "record": record}, "")
	if err := s.commitProxyTransaction(tx, input.RequestID, signature, result, "编辑代理"); err != nil {
		return storageFailure(err)
	}
	return result
}

func (s *Service) deleteProxy(input ProxyTarget, record ProxyView, ref, signature string) Result {
	if s.proxyChecks[record.ID] != nil {
		return failure("PROFILE_BUSY", "代理检查或结果保存仍在进行，未删除。", true)
	}
	if len(record.UsedBy) > 0 {
		return failure("PROXY_IN_USE", "该代理被环境引用，不能直接删除；请先修改环境绑定。", false)
	}
	tx, err := s.db.Begin()
	if err != nil {
		return storageFailure(err)
	}
	defer tx.Rollback()
	deleted, err := tx.Exec("DELETE FROM proxies WHERE id=? AND NOT EXISTS(SELECT 1 FROM environments WHERE proxy_id=?) AND EXISTS(SELECT 1 FROM proxy_config WHERE proxy_id=? AND revision=?)", record.ID, record.ID, record.ID, input.ExpectedRevision)
	if err != nil {
		return storageFailure(err)
	}
	if count, _ := deleted.RowsAffected(); count != 1 {
		return failure("REVISION_CONFLICT", "代理已改变或被引用，未删除。", true)
	}
	if ref != "" {
		if _, err := tx.Exec("DELETE FROM proxy_credentials WHERE ref=? AND NOT EXISTS(SELECT 1 FROM proxies WHERE credential_ref=?)", ref, ref); err != nil {
			return storageFailure(err)
		}
	}
	result := success(map[string]string{"status": "completed", "deletedId": record.ID}, "")
	if err := s.commitProxyTransaction(tx, input.RequestID, signature, result, "删除代理"); err != nil {
		return storageFailure(err)
	}
	return result
}

func (s *Service) discardProxyImport() {
	if draft := s.proxyImport; draft != nil {
		if draft.timer != nil {
			draft.timer.Stop()
		}
		for line, row := range draft.rows {
			if row.Credentials != nil {
				row.Credentials.Username, row.Credentials.Password = "", ""
			}
			delete(draft.rows, line)
		}
		s.proxyImport = nil
	}
}
