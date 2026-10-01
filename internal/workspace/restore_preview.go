package workspace

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"time"

	"github.com/axgiroud312-byte/prism-local-browser/internal/backup"
	"github.com/axgiroud312-byte/prism-local-browser/internal/kernel"
)

func (s *Service) restorePreviewCall(request Request) Result {
	s.mu.Lock()
	blocked, migrating := s.recycleTask != nil, s.migrationTask != nil
	s.mu.Unlock()
	if migrating {
		return failure("MIGRATION_INCOMPLETE", "迁移维护尚未收尾，请先核对原任务。", true)
	}
	if blocked {
		return failure("RECYCLE_INCOMPLETE", "回收维护尚未收尾，请先核对原任务。", true)
	}
	switch request.Method {
	case "Backup.SelectRestoreSource":
		if decode(request.Payload, &struct{}{}) != nil {
			return failure("VALIDATION_FAILED", "备份只能通过本机文件选择器读取。", false)
		}
		if s.options.ChooseBackupSource == nil {
			return failure("CAPABILITY_UNSUPPORTED", "本机备份选择器不可用。", false)
		}
		path, err := s.options.ChooseBackupSource()
		if err != nil {
			return preflightFailure(err)
		}
		if path == "" {
			return success(map[string]string{"status": "cancelled"}, "")
		}
		path, err = filepath.Abs(path)
		if err != nil || strings.HasPrefix(path, `\\`) || !strings.EqualFold(filepath.Ext(path), backup.Extension) {
			return failure("BACKUP_INVALID", "请选择本机 .prismbackup 完整备份；演示 JSON 不能恢复桌面数据。", false)
		}
		s.mu.Lock()
		defer s.mu.Unlock()
		if s.closed || s.closeRequested.Load() {
			return failure("NATIVE_UNAVAILABLE", "应用正在退出。", true)
		}
		if s.restorePreflight != nil || s.recycleTask != nil || s.migrationTask != nil {
			return failure("PROFILE_BUSY", "先取消或等待当前只读预检。", true)
		}
		token := id()
		s.restoreSources = map[string]restoreSource{token: {path: path, expires: time.Now().Add(30 * time.Minute)}}
		s.restorePreview = nil
		return success(map[string]string{"status": "selected", "sourceToken": token, "name": filepath.Base(path)}, "")
	case "Backup.PreviewRestore":
		var input struct {
			SourceToken string `json:"sourceToken"`
		}
		if decode(request.Payload, &input) != nil || input.SourceToken == "" {
			return failure("VALIDATION_FAILED", "只接受本机文件选择标识，不接受路径或包内容。", false)
		}
		return s.previewRestore(input.SourceToken)
	case "Backup.DiscardRestore":
		var input struct {
			PreviewID   string `json:"previewId"`
			SourceToken string `json:"sourceToken"`
		}
		if decode(request.Payload, &input) != nil {
			return failure("VALIDATION_FAILED", "取消预检标识无效。", false)
		}
		s.mu.Lock()
		defer s.mu.Unlock()
		if input.SourceToken != "" {
			delete(s.restoreSources, input.SourceToken)
			if s.restorePreflight != nil && s.restorePreflight.id == input.SourceToken {
				s.restorePreflight.cancel()
			}
		}
		if s.restorePreview != nil && (s.restorePreview.preview.PreviewID == input.PreviewID || input.SourceToken != "" && s.restorePreview.sourceToken == input.SourceToken) {
			s.restorePreview = nil
		}
		return success(map[string]string{"status": "discarded"}, "")
	case "Backup.ReadRestorePage":
		var input struct {
			PreviewID string `json:"previewId"`
			Offset    int    `json:"offset"`
			PageSize  int    `json:"pageSize"`
		}
		if decode(request.Payload, &input) != nil || input.Offset < 0 || input.PageSize < 1 || input.PageSize > 100 {
			return failure("VALIDATION_FAILED", "恢复预览每页1–100条，位置必须有效。", false)
		}
		s.mu.Lock()
		defer s.mu.Unlock()
		d := s.restorePreview
		if d == nil || d.preview.PreviewID != input.PreviewID || time.Now().After(d.expires) {
			return failure("PREVIEW_EXPIRED", "恢复预览已失效，请重新校验原文件。", true)
		}
		if input.Offset > len(d.impacts) {
			return failure("VALIDATION_FAILED", "预览分页位置超出范围。", false)
		}
		end := input.Offset + input.PageSize
		if end > len(d.impacts) {
			end = len(d.impacts)
		}
		return success(map[string]any{"mode": "native", "previewId": input.PreviewID, "offset": input.Offset, "total": len(d.impacts), "items": d.impacts[input.Offset:end]}, "")
	}
	return failure("CAPABILITY_UNSUPPORTED", "没有此恢复预检方法。", false)
}

func (s *Service) previewRestore(token string) (result Result) {
	s.mu.Lock()
	source, exists := s.restoreSources[token]
	if !exists || time.Now().After(source.expires) || s.closed || s.closeRequested.Load() {
		s.mu.Unlock()
		return failure("PREVIEW_EXPIRED", "文件选择已失效，请重新选择。", true)
	}
	if s.restorePreflight != nil || s.recycleTask != nil || s.migrationTask != nil {
		s.mu.Unlock()
		return failure("PROFILE_BUSY", "已有只读预检正在进行。", true)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	owner := &restorePreflight{id: token, cancel: cancel}
	s.restorePreflight = owner
	s.restorePreview = nil
	s.workers.Add(1)
	s.mu.Unlock()
	defer s.workers.Done()
	defer cancel()
	defer func() {
		s.mu.Lock()
		if s.restorePreflight == owner {
			s.restorePreflight = nil
		}
		s.mu.Unlock()
	}()
	if s.restoreScratch != "" {
		if err := kernel.RemoveOwnedTree(s.restoreScratch); err != nil {
			return failure("BACKUP_PREFLIGHT_FAILED", "上次私有预检暂存尚无法清理，请解除占用/修复权限后重试；未继续占用空间。", true)
		}
		s.restoreScratch = ""
	}
	file, release, err := backup.FreezeFile(source.path)
	if err != nil {
		return preflightFailure(err)
	}
	defer release()
	defer file.Close()
	pkg, err := backup.Read(ctx, file)
	if err != nil {
		return preflightFailure(err)
	}
	if source.expectedSHA256 != "" && source.expectedSHA256 != pkg.ArchiveSHA256 {
		return failure("BACKUP_INVALID", "保留的升级前备份与原摘要不一致，未恢复。", false)
	}
	previewID := id()
	stageRef := "backups/preflight/" + previewID
	unpin, err := kernel.EnsureDirectory(s.root, "backups/preflight")
	if err != nil {
		return preflightFailure(err)
	}
	defer unpin()
	directory := filepath.Join(s.root, filepath.FromSlash(stageRef))
	if err = os.Mkdir(directory, 0700); err != nil {
		return preflightFailure(err)
	}
	defer func() {
		// Runs after all scratch file/directory pins have closed. Only this newly
		// allocated UUID directory is owned; never clean the archive or profiles.
		if err := kernel.RemoveOwnedTree(directory); err != nil {
			s.mu.Lock()
			s.restoreScratch = directory
			s.restorePreview = nil
			s.mu.Unlock()
			result = failure("BACKUP_PREFLIGHT_FAILED", "本次私有预检暂存未能清理，当前工作区未修改；请解除占用后重试。", true)
		}
	}()
	freeze, err := backup.PinStagingDirectory(directory)
	if err != nil {
		return preflightFailure(err)
	}
	defer freeze()
	configuration := filepath.Join(directory, "configuration.sqlite")
	output, err := os.OpenFile(configuration, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return preflightFailure(err)
	}
	err = pkg.Copy(ctx, pkg.Manifest.Configuration, output)
	if err == nil {
		err = output.Sync()
	}
	err = errors.Join(err, output.Close())
	if err != nil {
		return preflightFailure(err)
	}
	// This owned scratch file is the only payload extracted by preflight. Never
	// open/migrate the current app.db, create a runtime lock, or stop a session.
	pin, releaseConfig, err := backup.FreezeFile(configuration)
	if err != nil {
		return preflightFailure(err)
	}
	defer releaseConfig()
	defer pin.Close()
	data, err := s.readRestoreConfiguration(ctx, configuration, pkg.Manifest)
	if err != nil {
		return preflightFailure(err)
	}
	d := &restoreDraft{data: data, manifest: pkg.Manifest, path: source.path, sourceToken: token, expires: time.Now().Add(30 * time.Minute), kernelMapping: map[string]string{}, kernelCandidates: map[string][]string{}, impacts: []RestoreEnvironment{}}
	d.preview = RestorePreview{Mode: "native", PreviewID: previewID, Format: backup.Format, Name: filepath.Base(source.path), ArchiveSHA256: pkg.ArchiveSHA256, ManifestSHA256: pkg.ManifestSHA256, Scope: pkg.Manifest.Scope, CreatedAt: pkg.Manifest.CreatedAt, ExpiresAt: d.expires.UTC().Format(time.RFC3339Nano), EnvironmentCount: len(data.environments), Bytes: pkg.Bytes, Kernels: []RestoreKernel{}, Credentials: []RestoreCredential{}}
	s.mu.Lock()
	local, err := s.planRestoreImpact(d)
	s.mu.Unlock()
	if err != nil {
		return preflightFailure(err)
	}
	for i := range d.preview.Kernels {
		entry := &d.preview.Kernels[i]
		if entry.State == "pending" {
			continue
		}
		candidates := d.kernelCandidates[entry.ID]
		entry.LocalID = ""
		entry.State = "missing"
		for _, candidate := range candidates {
			r := local[candidate]
			directory, problem := kernel.RecordDirectory(s.root, r)
			var release func()
			if problem == nil {
				release, problem = kernel.PinFiles(directory, r.Files)
			}
			if problem == nil {
				problem = kernel.VerifyFiles(directory, r.Files)
				if problem == nil {
					var version string
					version, problem = kernel.FileVersion(filepath.Join(directory, filepath.FromSlash(r.ExecutableRelativePath)))
					if problem == nil && version != r.Version {
						problem = errors.New("kernel version mismatch")
					}
				}
				release()
			}
			if problem == nil {
				entry.LocalID = candidate
				entry.State = "verified-bytes"
				break
			}
			entry.State = "unavailable"
			if err = ctx.Err(); err != nil {
				return preflightFailure(err)
			}
		}
		d.kernelMapping[entry.ID] = entry.LocalID
		if entry.LocalID == "" && entry.Required {
			d.preview.MissingKernelCount++
		}
		if err = ctx.Err(); err != nil {
			return preflightFailure(err)
		}
	}
	d.preview.CanRestore = d.preview.ConflictCount == 0 && d.preview.MissingKernelCount == 0
	s.mu.Lock()
	defer s.mu.Unlock()
	if ctx.Err() != nil || s.closed || s.closeRequested.Load() || s.restorePreflight != owner {
		return preflightFailure(context.Canceled)
	}
	s.restorePreview = d
	return success(d.preview, "")
}

// This digest is host-private. It binds both existing records and absence claims
// including unrelated holders of seed/name/credential refs. It is rechecked at
// commit; any intervening configuration write requires a fresh impact preview.
func restoreBaseline(query interface {
	Query(string, ...any) (*sql.Rows, error)
}) (string, error) {
	h := sha256.New()
	for _, statement := range []string{
		"SELECT id,code,name,kernel_id,COALESCE(proxy_id,''),fingerprint_id,revision,created_at,user_data_ref FROM environments ORDER BY id",
		"SELECT id,seed,kernel_id,template_version,generator_version,config_json,config_revision FROM fingerprints ORDER BY id",
		"SELECT fingerprint_id,revision,kernel_id,profile_json,created_at,action,COALESCE(restored_from,0) FROM fingerprint_revisions ORDER BY fingerprint_id,revision",
		"SELECT id,COALESCE(credential_ref,'') FROM proxies ORDER BY id", "SELECT proxy_id,config_json,revision FROM proxy_config ORDER BY proxy_id", "SELECT ref,hex(protected) FROM proxy_credentials ORDER BY ref",
		"SELECT id,version,source,status FROM kernels ORDER BY id", "SELECT kernel_id,record_json FROM kernel_evidence ORDER BY kernel_id",
		"SELECT environment_id,state FROM environment_data_state ORDER BY environment_id",
		"SELECT plan_id,item_index,state,identity_json FROM batch_items ORDER BY plan_id,item_index",
		"SELECT environment_id,trash_id,entry_json FROM environment_trash ORDER BY environment_id",
	} {
		rows, err := query.Query(statement)
		if err != nil {
			return "", err
		}
		cols, err := rows.Columns()
		if err != nil {
			rows.Close()
			return "", err
		}
		io.WriteString(h, statement)
		for rows.Next() {
			values := make([]any, len(cols))
			dest := make([]any, len(cols))
			for i := range values {
				dest[i] = &values[i]
			}
			if err = rows.Scan(dest...); err != nil {
				break
			}
			encoded, _ := json.Marshal(values)
			h.Write(encoded)
			h.Write([]byte{0})
		}
		readErr := rows.Err()
		rows.Close()
		if err != nil {
			return "", err
		}
		if readErr != nil {
			return "", readErr
		}
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func (s *Service) planRestoreImpact(d *restoreDraft) (map[string]kernel.Record, error) {
	local := map[string]kernel.Record{}
	tx, err := s.db.Begin()
	if err != nil {
		return local, err
	}
	defer tx.Rollback()
	d.baseline, err = restoreBaseline(tx)
	if err != nil {
		return local, err
	}
	ids := []string{}
	for _, e := range d.data.environments {
		ids = append(ids, e.manifest.ID)
	}
	encoded, _ := json.Marshal(ids)
	for _, e := range d.data.environments {
		impact := RestoreEnvironment{ID: e.manifest.ID, Name: e.environment.Name, Seed: e.environment.Seed, Action: "add", BackupRevision: e.manifest.Revision, DataState: e.manifest.DataState, Busy: s.profileUses[e.manifest.ID], Conflicts: []string{}}
		var existingProfile string
		err = tx.QueryRow("SELECT revision,fingerprint_id FROM environments WHERE id=?", impact.ID).Scan(&impact.CurrentRevision, &existingProfile)
		if err == nil {
			impact.Action = "overwrite"
			d.preview.OverwriteCount++
		} else if err == sql.ErrNoRows {
			d.preview.AddCount++
		} else {
			return local, err
		}
		for _, check := range []struct {
			query, reason string
			args          []any
		}{
			{"SELECT EXISTS(SELECT 1 FROM environment_trash WHERE environment_id=?)", "environment-in-recycle-bin", []any{impact.ID}},
			{"SELECT EXISTS(SELECT 1 FROM environments WHERE name=? AND id NOT IN (SELECT value FROM json_each(?)))", "name-conflict", []any{impact.Name, string(encoded)}},
			{"SELECT EXISTS(SELECT 1 FROM environments WHERE fingerprint_id=? AND id<>?)", "fingerprint-id-conflict", []any{e.manifest.FingerprintID, impact.ID}},
			{"SELECT EXISTS(SELECT 1 FROM batch_items WHERE identity_json IS NOT NULL AND json_extract(identity_json,'$.environmentId')=? AND state<>'completed')", "prepared-identity-conflict", []any{impact.ID}},
		} {
			var conflict bool
			if err = tx.QueryRow(check.query, check.args...).Scan(&conflict); err != nil {
				return local, err
			}
			if conflict {
				impact.Conflicts = append(impact.Conflicts, check.reason)
			}
		}
		for _, history := range e.history {
			available, err := seedAvailable(tx, history.Profile.Seed, e.manifest.FingerprintID, impact.ID)
			if err != nil {
				return local, err
			}
			if !available {
				impact.Conflicts = append(impact.Conflicts, "seed-or-reserved-identity-conflict")
				break
			}
		}
		if existingProfile != "" && existingProfile != e.manifest.FingerprintID {
			impact.Conflicts = append(impact.Conflicts, "original-fingerprint-reference-conflict")
		}
		if len(impact.Conflicts) > 0 {
			d.preview.ConflictCount++
		}
		d.impacts = append(d.impacts, impact)
	}
	for _, p := range d.data.proxies {
		d.preview.Credentials = append(d.preview.Credentials, RestoreCredential{ProxyID: p.ID, State: p.CredentialState})
		if p.CredentialState == "reentry-required" {
			d.preview.CredentialReentryCount++
		}
		var ref, text string
		var rev int64
		err = tx.QueryRow(`SELECT COALESCE(p.credential_ref,''),c.config_json,c.revision FROM proxies p JOIN proxy_config c ON c.proxy_id=p.id WHERE p.id=?`, p.ID).Scan(&ref, &text, &rev)
		if err != nil && err != sql.ErrNoRows {
			return local, err
		}
		if err == nil {
			var c any
			var desired any
			json.Unmarshal([]byte(text), &c)
			b, _ := json.Marshal(p.Configuration)
			json.Unmarshal(b, &desired)
			var protected []byte
			if ref != "" {
				if err = tx.QueryRow("SELECT protected FROM proxy_credentials WHERE ref=?", ref).Scan(&protected); err != nil {
					return local, err
				}
			}
			if !reflect.DeepEqual(c, desired) || ref != p.Ref || !bytes.Equal(protected, p.Protected) {
				var outside bool
				if err = tx.QueryRow("SELECT EXISTS(SELECT 1 FROM environments WHERE proxy_id=? AND id NOT IN (SELECT value FROM json_each(?)))", p.ID, string(encoded)).Scan(&outside); err != nil {
					return local, err
				}
				if outside {
					d.preview.ConflictCount++
				}
			}
		}
		if p.Ref != "" {
			var protected []byte
			err = tx.QueryRow("SELECT protected FROM proxy_credentials WHERE ref=?", p.Ref).Scan(&protected)
			if err != nil && err != sql.ErrNoRows {
				return local, err
			}
			if err == nil && !bytes.Equal(protected, p.Protected) {
				d.preview.ConflictCount++
			}
		}
	}
	rows, err := tx.Query("SELECT kernel_id FROM kernel_evidence ORDER BY kernel_id")
	if err != nil {
		return local, err
	}
	kernelIDs := []string{}
	for rows.Next() {
		var k string
		if err = rows.Scan(&k); err != nil {
			break
		}
		kernelIDs = append(kernelIDs, k)
	}
	readErr := rows.Err()
	rows.Close()
	if err != nil {
		return local, err
	}
	if readErr != nil {
		return local, readErr
	}
	for _, k := range kernelIDs {
		record, err := savedKernelFrom(tx, k, true)
		if err == nil {
			local[k] = record
		}
	}
	for _, m := range d.manifest.Kernels {
		entry := RestoreKernel{ID: m.ID, Version: m.Version, ArchiveSHA256: m.ArchiveSHA256, ExecutableSHA256: m.ExecutableSHA256, State: "missing"}
		for _, e := range d.data.environments {
			for _, r := range e.history {
				if r.Profile.KernelID == m.ID {
					entry.Required = true
				}
			}
		}
		if m.ID == PendingKernelID {
			entry.State = "pending"
			entry.LocalID = PendingKernelID
		} else {
			for _, k := range kernelIDs {
				r, ok := local[k]
				if !ok {
					continue
				}
				source := d.data.kernels[m.ID]
				if r.Version == m.Version && r.Architecture == m.Architecture && r.ArchiveSHA256 == m.ArchiveSHA256 && r.ExecutableSHA256 == m.ExecutableSHA256 && reflect.DeepEqual(r.Files, source.Files) && reflect.DeepEqual(r.Report.Capabilities, source.Report.Capabilities) {
					d.kernelCandidates[m.ID] = append(d.kernelCandidates[m.ID], k)
				}
			}
		}
		d.kernelMapping[m.ID] = entry.LocalID
		d.preview.Kernels = append(d.preview.Kernels, entry)
	}
	return local, nil
}
