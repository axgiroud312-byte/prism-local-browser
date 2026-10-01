package workspace

import (
	"context"
	"database/sql"
	"errors"
	"reflect"
	"strings"

	"github.com/axgiroud312-byte/prism-local-browser/internal/backup"
	"github.com/axgiroud312-byte/prism-local-browser/internal/kernel"
	"github.com/axgiroud312-byte/prism-local-browser/internal/proxy"
)

func restoreInvalid(reason string) error { return &backup.Invalid{Reason: reason} }

// Imported SQL is never executed or attached to the live database. Compare its
// entire schema against a fresh trusted in-memory schema before reading records;
// reject views, virtual tables, custom triggers and unknown additions in v1.
func configurationSchema(ctx context.Context, db *sql.DB) (map[string]string, error) {
	rows, err := db.QueryContext(ctx, "SELECT type,name,tbl_name,sql FROM sqlite_master WHERE sql IS NOT NULL ORDER BY type,name")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := map[string]string{}
	for rows.Next() {
		var kind, name, table, text string
		if err = rows.Scan(&kind, &name, &table, &text); err != nil {
			return nil, err
		}
		result[kind+":"+name+":"+table] = strings.Join(strings.Fields(text), " ")
	}
	return result, rows.Err()
}
func validateConfigurationSchema(ctx context.Context, db *sql.DB) error {
	trusted, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		return err
	}
	defer trusted.Close()
	trusted.SetMaxOpenConns(1)
	template := &Service{db: trusted}
	if err = template.initialize(); err != nil {
		return err
	}
	if _, err = trusted.Exec("DROP TABLE restore_jobs; PRAGMA user_version=7"); err != nil {
		return err
	}
	want, err := configurationSchema(ctx, trusted)
	if err != nil {
		return err
	}
	got, err := configurationSchema(ctx, db)
	if err != nil {
		return err
	}
	if !reflect.DeepEqual(want, got) {
		return restoreInvalid("configuration-schema-mismatch")
	}
	var version int
	if err = db.QueryRowContext(ctx, "PRAGMA user_version").Scan(&version); err != nil || version != 7 {
		return restoreInvalid("configuration-version")
	}
	var integrity string
	if err = db.QueryRowContext(ctx, "PRAGMA integrity_check").Scan(&integrity); err != nil || integrity != "ok" {
		return restoreInvalid("configuration-integrity")
	}
	rows, err := db.QueryContext(ctx, "PRAGMA foreign_key_check")
	if err != nil {
		return err
	}
	bad := rows.Next()
	err = rows.Err()
	rows.Close()
	if bad || err != nil {
		return restoreInvalid("configuration-references")
	}
	for _, table := range []string{"operations", "requests", "activities", "runtime_sessions", "runtime_events", "batch_plans", "batch_items", "batch_item_events", "backup_exports", "proxy_request_key"} {
		var count int
		if err = db.QueryRowContext(ctx, "SELECT COUNT(*) FROM "+table).Scan(&count); err != nil || count != 0 {
			return restoreInvalid("configuration-operational-history")
		}
	}
	return nil
}

func (s *Service) readRestoreConfiguration(ctx context.Context, file string, manifest backup.Manifest) (restoreData, error) {
	data := restoreData{environments: []restoreEnvironmentData{}, proxies: []restoreProxyData{}, kernels: map[string]kernel.Record{}}
	// immutable prevents even journal/shm creation alongside the private copy.
	db, err := sql.Open("sqlite", sqliteFileURI(file, "ro")+"&immutable=1&_pragma=query_only(1)&_pragma=trusted_schema(0)")
	if err != nil {
		return data, err
	}
	defer db.Close()
	db.SetMaxOpenConns(1)
	if err = validateConfigurationSchema(ctx, db); err != nil {
		return data, err
	}
	reader := &Service{db: db}
	counts := map[string]int{"environments": len(manifest.Environments), "fingerprints": len(manifest.Environments), "environment_data_state": len(manifest.Environments), "kernels": len(manifest.Kernels)}
	for table, want := range counts {
		var got int
		if err = db.QueryRowContext(ctx, "SELECT COUNT(*) FROM "+table).Scan(&got); err != nil || got != want {
			return data, restoreInvalid("configuration-manifest-count")
		}
	}
	for _, m := range manifest.Kernels {
		if err = ctx.Err(); err != nil {
			return data, err
		}
		if m.ID == PendingKernelID {
			var version, source, state string
			if err = db.QueryRowContext(ctx, "SELECT version,source,status FROM kernels WHERE id=?", m.ID).Scan(&version, &source, &state); err != nil || version != "未安装" || source != "fingerprint-chromium" || state != "missing" {
				return data, restoreInvalid("pending-kernel-record")
			}
			continue
		}
		r, err := savedKernelFrom(db, m.ID, false)
		if err != nil || r.Version != m.Version || r.ArchiveSHA256 != m.ArchiveSHA256 || r.ExecutableSHA256 != m.ExecutableSHA256 || r.Architecture != m.Architecture {
			return data, restoreInvalid("configuration-kernel-mismatch")
		}
		data.kernels[m.ID] = r
		var evidence, version, source string
		if err = db.QueryRowContext(ctx, "SELECT e.record_json,k.version,k.source FROM kernel_evidence e JOIN kernels k ON k.id=e.kernel_id WHERE e.kernel_id=?", m.ID).Scan(&evidence, &version, &source); err != nil {
			return data, err
		}
		var strict kernel.Record
		if backup.DecodeJSON([]byte(evidence), &strict) != nil || version != m.Version || source != "fingerprint-chromium" {
			return data, restoreInvalid("kernel-record-fields")
		}
	}
	var evidenceCount int
	if err = db.QueryRowContext(ctx, "SELECT COUNT(*) FROM kernel_evidence").Scan(&evidenceCount); err != nil || evidenceCount != len(data.kernels) {
		return data, restoreInvalid("unexpected-kernel-evidence")
	}
	seedOwners := map[string]string{}
	for _, m := range manifest.Environments {
		if err = ctx.Err(); err != nil {
			return data, err
		}
		e, revision, profileID, err := reader.readEnvironment(m.ID)
		if err != nil || revision != m.Revision || profileID != m.FingerprintID || e.Seed != m.Seed || e.ProxyID != m.ProxyID || e.CoreID != m.KernelID {
			return data, restoreInvalid("configuration-environment-mismatch")
		}
		item := restoreEnvironmentData{manifest: m, environment: e, history: []ProfileRevision{}}
		var template, generator, configJSON string
		if err = db.QueryRowContext(ctx, `SELECT e.code,d.state,f.template_version,f.generator_version,f.config_json FROM environments e JOIN environment_data_state d ON d.environment_id=e.id JOIN fingerprints f ON f.id=e.fingerprint_id WHERE e.id=?`, m.ID).Scan(&item.code, &item.dataState, &template, &generator, &configJSON); err != nil {
			return data, err
		}
		var config Configuration
		if backup.DecodeJSON([]byte(configJSON), &config) != nil || item.code < 1 || template != m.TemplateVersion || generator != m.GeneratorVersion {
			return data, restoreInvalid("configuration-field-mismatch")
		}
		if _, err = backupDataTargetFrom(db, m.ID, m.DataReference); err != nil || m.DataState == "never-initialized" && item.dataState != dataNeverInitialized {
			return data, restoreInvalid("data-initialization-mismatch")
		}
		rows, err := db.QueryContext(ctx, "SELECT revision,kernel_id,profile_json,created_at,action,COALESCE(restored_from,0) FROM fingerprint_revisions WHERE fingerprint_id=? ORDER BY revision", profileID)
		if err != nil {
			return data, err
		}
		for rows.Next() {
			var row ProfileRevision
			var number int64
			var kernelID, text string
			if err = rows.Scan(&number, &kernelID, &text, &row.CreatedAt, &row.Action, &row.RestoredFrom); err != nil {
				break
			}
			if backup.DecodeJSON([]byte(text), &row.Profile) != nil || number != row.Profile.ConfigRevision || kernelID != row.Profile.KernelID || number != int64(len(item.history)+1) || row.RestoredFrom < 0 || row.RestoredFrom >= number {
				err = restoreInvalid("historical-profile-record")
				break
			}
			item.history = append(item.history, row)
		}
		readErr := rows.Err()
		rows.Close()
		if err != nil {
			return data, err
		}
		if readErr != nil {
			return data, readErr
		}
		found := false
		for _, r := range item.history {
			if owner := seedOwners[r.Profile.Seed]; owner != "" && owner != profileID {
				return data, restoreInvalid("historical-seed-owner-conflict")
			}
			seedOwners[r.Profile.Seed] = profileID
			if checkProfile(r.Profile) != nil || checkProfileEvidence(db, r.Profile) != nil {
				return data, restoreInvalid("historical-profile-evidence")
			}
			if r.Profile.ConfigRevision == m.ProfileRevision {
				found = r.Profile.ConfigHash == m.ProfileHash && r.Profile.TemplateVersion == m.TemplateVersion && r.Profile.GeneratorVersion == m.GeneratorVersion && profileMatchesConfiguration(r.Profile, e.Configuration)
			}
		}
		current, err := readProfileFrom(db, profileID)
		if err != nil || !found || m.ProfileRevision != int64(len(item.history)) || current.Profile.ConfigRevision != m.ProfileRevision || current.Profile.ConfigHash != m.ProfileHash {
			return data, restoreInvalid("current-profile-manifest")
		}
		data.environments = append(data.environments, item)
	}
	rows, err := db.QueryContext(ctx, `SELECT p.id,COALESCE(p.credential_ref,''),c.config_json,c.revision,c.check_json FROM proxies p LEFT JOIN proxy_config c ON c.proxy_id=p.id ORDER BY p.id`)
	if err != nil {
		return data, err
	}
	for rows.Next() {
		var p restoreProxyData
		var text string
		var check sql.NullString
		if err = rows.Scan(&p.ID, &p.Ref, &text, &p.Revision, &check); err != nil {
			break
		}
		if !backup.CanonicalID(p.ID) || p.Ref != "" && !backup.CanonicalID(p.Ref) || p.Revision < 1 || check.Valid || backup.DecodeJSON([]byte(text), &p.Configuration) != nil {
			err = restoreInvalid("proxy-record")
			break
		}
		normalized, problem := proxy.Normalize(p.Configuration)
		if problem != nil || normalized != p.Configuration {
			err = restoreInvalid("proxy-configuration")
			break
		}
		data.proxies = append(data.proxies, p)
	}
	readErr := rows.Err()
	rows.Close()
	if err != nil {
		return data, err
	}
	if readErr != nil {
		return data, readErr
	}
	refs := map[string]bool{}
	for i := range data.proxies {
		p := &data.proxies[i]
		p.CredentialState = "none"
		if p.Ref == "" {
			continue
		}
		if refs[p.Ref] {
			return data, restoreInvalid("shared-credential-reference")
		}
		refs[p.Ref] = true
		if err = db.QueryRowContext(ctx, "SELECT protected FROM proxy_credentials WHERE ref=?", p.Ref).Scan(&p.Protected); err != nil || len(p.Protected) == 0 || len(p.Protected) > 1<<20 {
			return data, restoreInvalid("protected-credential-missing")
		}
		credentials, problem := s.decodeProtectedProxyCredentials(p.Ref, p.Protected)
		p.CredentialState = "reentry-required"
		if problem == nil && credentials != nil && proxy.ValidateProtocolCredentials(p.Configuration.Type, *credentials) == nil {
			p.CredentialState = "available-current-user"
		}
		if credentials != nil {
			credentials.Username = ""
			credentials.Password = ""
		}
	}
	var credentialCount int
	if err = db.QueryRowContext(ctx, "SELECT COUNT(*) FROM proxy_credentials").Scan(&credentialCount); err != nil || credentialCount != len(refs) {
		return data, restoreInvalid("credential-closure")
	}
	if manifest.Scope == "selected" {
		usedProxies, usedKernels := map[string]bool{}, map[string]bool{PendingKernelID: true}
		for _, e := range data.environments {
			usedProxies[e.environment.ProxyID] = true
			usedKernels[e.environment.CoreID] = true
			for _, r := range e.history {
				usedKernels[r.Profile.KernelID] = true
			}
		}
		for _, p := range data.proxies {
			if !usedProxies[p.ID] {
				return data, restoreInvalid("unselected-proxy")
			}
		}
		for k := range data.kernels {
			if !usedKernels[k] {
				return data, restoreInvalid("unselected-kernel")
			}
		}
	}
	return data, nil
}

func preflightFailure(err error) Result {
	var invalid *backup.Invalid
	if errors.As(err, &invalid) {
		code := "BACKUP_INVALID"
		if invalid.Unsupported {
			code = "BACKUP_VERSION_UNSUPPORTED"
		}
		result := failure(code, "备份格式、文件或配置校验未通过；当前工作区未修改。请选择本软件支持的完整本机备份。", false)
		result.Error.Details = map[string]any{"reason": invalid.Reason}
		return result
	}
	if errors.Is(err, context.Canceled) {
		return failure("OPERATION_CANCELLED", "预检已取消；当前数据库与浏览数据未修改。", true)
	}
	return failure("BACKUP_PREFLIGHT_FAILED", "无法完成只读预检；当前工作区未修改。请检查包文件、空间和读取权限。", true)
}
