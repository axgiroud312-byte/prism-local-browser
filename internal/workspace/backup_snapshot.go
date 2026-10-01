package workspace

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/url"
	"path/filepath"
	"strings"

	"github.com/axgiroud312-byte/prism-local-browser/internal/backup"
	"modernc.org/sqlite"
)

func sqliteFileURI(path, mode string) string {
	path = filepath.ToSlash(path)
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	uri := url.URL{Scheme: "file", Path: path}
	query := uri.Query()
	if mode != "" {
		query.Set("mode", mode)
	}
	query.Add("_pragma", "busy_timeout(5000)")
	query.Add("_pragma", "synchronous(FULL)")
	uri.RawQuery = query.Encode()
	return uri.String()
}

// A separate read connection observes active WAL and pins one SQLite snapshot.
// The service's single connection/mutex remain available for Cancel/shutdown.
func (s *Service) createBackupSnapshot(ctx context.Context, input backupExecution, path string) (backup.Manifest, error) {
	source, err := sql.Open("sqlite", sqliteFileURI(filepath.Join(s.root, "app.db"), "ro"))
	if err != nil {
		return backup.Manifest{}, err
	}
	defer source.Close()
	connection, err := source.Conn(ctx)
	if err != nil {
		return backup.Manifest{}, err
	}
	defer connection.Close()
	if _, err = connection.ExecContext(ctx, "BEGIN"); err != nil {
		return backup.Manifest{}, err
	}
	defer connection.ExecContext(context.Background(), "ROLLBACK")
	var schema int
	if err = connection.QueryRowContext(ctx, "PRAGMA schema_version").Scan(&schema); err != nil {
		return backup.Manifest{}, err
	}
	err = connection.Raw(func(driver any) (resultErr error) {
		maker, ok := driver.(interface {
			NewBackup(string) (*sqlite.Backup, error)
		})
		if !ok {
			return errors.New("sqlite snapshot API unavailable")
		}
		copy, err := maker.NewBackup(sqliteFileURI(path, ""))
		if err != nil {
			return err
		}
		defer func() { resultErr = errors.Join(resultErr, copy.Finish()) }()
		for {
			if err = ctx.Err(); err != nil {
				return err
			}
			more, err := copy.Step(64)
			if err != nil {
				return err
			}
			if !more {
				return nil
			}
		}
	})
	if err != nil {
		return backup.Manifest{}, err
	}
	copy, err := sql.Open("sqlite", sqliteFileURI(path, "rw"))
	if err != nil {
		return backup.Manifest{}, err
	}
	copy.SetMaxOpenConns(1)
	defer copy.Close()
	ids := []string{}
	for _, target := range input.Targets {
		ids = append(ids, target.ID)
	}
	encoded, _ := json.Marshal(ids)
	// No operational history, old control identity, output path, mixed batch plan,
	// or dedup key is a restorable browser configuration. Clear them even in full
	// scope; keep all fixed profile revisions and original credential refs/bytes.
	tx, err := copy.BeginTx(ctx, nil)
	if err != nil {
		return backup.Manifest{}, err
	}
	defer tx.Rollback()
	// Native v1 configuration remains schema7. New runtime journals are local
	// control state, never exported or executed when restoring a package.
	if _, err = tx.ExecContext(ctx, "DROP TABLE IF EXISTS restore_jobs; PRAGMA user_version=7"); err != nil {
		return backup.Manifest{}, err
	}
	for _, statement := range []string{
		"DELETE FROM runtime_events", "DELETE FROM runtime_sessions", "DELETE FROM activities", "DELETE FROM requests", "DELETE FROM batch_item_events", "DELETE FROM batch_items", "DELETE FROM batch_plans", "DELETE FROM backup_exports", "DELETE FROM operations", "DELETE FROM proxy_request_key",
	} {
		if _, err = tx.ExecContext(ctx, statement); err != nil {
			return backup.Manifest{}, err
		}
	}
	if _, err = tx.ExecContext(ctx, "DELETE FROM environment_data_state WHERE environment_id NOT IN (SELECT value FROM json_each(?))", string(encoded)); err != nil {
		return backup.Manifest{}, err
	}
	if _, err = tx.ExecContext(ctx, "DELETE FROM environments WHERE id NOT IN (SELECT value FROM json_each(?))", string(encoded)); err != nil {
		return backup.Manifest{}, err
	}
	if _, err = tx.ExecContext(ctx, "DELETE FROM fingerprint_revisions WHERE fingerprint_id NOT IN (SELECT fingerprint_id FROM environments)"); err != nil {
		return backup.Manifest{}, err
	}
	if _, err = tx.ExecContext(ctx, "DELETE FROM fingerprints WHERE id NOT IN (SELECT fingerprint_id FROM environments)"); err != nil {
		return backup.Manifest{}, err
	}
	if input.Scope == "selected" {
		for _, statement := range []string{
			"DELETE FROM proxy_config WHERE proxy_id NOT IN (SELECT proxy_id FROM environments WHERE proxy_id IS NOT NULL)",
			"DELETE FROM proxies WHERE id NOT IN (SELECT proxy_id FROM environments WHERE proxy_id IS NOT NULL)",
			"DELETE FROM kernel_evidence WHERE kernel_id NOT IN (SELECT kernel_id FROM fingerprint_revisions UNION SELECT kernel_id FROM environments)",
			"DELETE FROM kernels WHERE id<>'kernel-pending' AND id NOT IN (SELECT kernel_id FROM fingerprint_revisions UNION SELECT kernel_id FROM environments)",
		} {
			if _, err = tx.ExecContext(ctx, statement); err != nil {
				return backup.Manifest{}, err
			}
		}
	}
	if _, err = tx.ExecContext(ctx, "DELETE FROM proxy_credentials WHERE ref NOT IN (SELECT credential_ref FROM proxies WHERE credential_ref IS NOT NULL)"); err != nil {
		return backup.Manifest{}, err
	}
	if _, err = tx.ExecContext(ctx, "UPDATE proxy_config SET check_json=NULL"); err != nil {
		return backup.Manifest{}, err
	}
	if err = tx.Commit(); err != nil {
		return backup.Manifest{}, err
	}
	// Rebuild the offline file: deleted unselected browser/configuration secrets
	// must not survive as freelist bytes in a selected-environment package.
	if _, err = copy.ExecContext(ctx, "PRAGMA journal_mode=DELETE"); err != nil {
		return backup.Manifest{}, err
	}
	if _, err = copy.ExecContext(ctx, "VACUUM"); err != nil {
		return backup.Manifest{}, err
	}
	manifest := backup.Manifest{Format: backup.Format, SchemaVersion: backup.Version, WorkspaceSchema: 7, AppVersion: s.options.AppVersion, BackupID: input.OperationID, CreatedAt: input.CreatedAt, Scope: input.Scope, Configuration: "configuration.sqlite", Credentials: "windows-current-user-dpapi", BrowserData: "sensitive-same-user-not-portable", Environments: []backup.Environment{}, Kernels: []backup.Kernel{}, Files: []backup.File{}}
	if manifest.AppVersion == "" {
		manifest.AppVersion = "development-unversioned"
	}
	var result string
	if err = copy.QueryRowContext(ctx, "PRAGMA integrity_check").Scan(&result); err != nil || result != "ok" {
		return manifest, errors.New("configuration snapshot integrity failed")
	}
	rows, err := copy.QueryContext(ctx, "PRAGMA foreign_key_check")
	if err != nil {
		return manifest, err
	}
	invalid := rows.Next()
	checkErr := rows.Err()
	rows.Close()
	if invalid || checkErr != nil {
		return manifest, errors.New("configuration references invalid")
	}
	var dangling int
	if err = copy.QueryRowContext(ctx, `SELECT COUNT(*) FROM proxies p LEFT JOIN proxy_credentials c ON c.ref=p.credential_ref LEFT JOIN proxy_config v ON v.proxy_id=p.id WHERE v.proxy_id IS NULL OR (p.credential_ref IS NOT NULL AND (c.ref IS NULL OR length(c.protected)=0))`).Scan(&dangling); err != nil || dangling != 0 {
		return manifest, errors.New("protected credential reference missing")
	}
	for _, target := range input.Targets {
		var entry backup.Environment
		var configJSON string
		fact, err := backupDataTargetFrom(copy, target.ID, target.Reference)
		if err != nil || fact != target {
			return manifest, errors.New("snapshot data initialization fact changed or missing")
		}
		if err = copy.QueryRowContext(ctx, `SELECT e.id,CAST(f.seed AS TEXT),e.revision,e.fingerprint_id,e.kernel_id,COALESCE(e.proxy_id,''),e.user_data_ref,f.config_json FROM environments e JOIN fingerprints f ON f.id=e.fingerprint_id WHERE e.id=?`, target.ID).Scan(&entry.ID, &entry.Seed, &entry.Revision, &entry.FingerprintID, &entry.KernelID, &entry.ProxyID, &entry.DataReference, &configJSON); err != nil {
			return manifest, err
		}
		var config Configuration
		if decode(json.RawMessage(configJSON), &config) != nil || validate(config) != "" || config.Seed != entry.Seed || config.CoreID != entry.KernelID || config.ProxyID != entry.ProxyID || entry.DataReference != target.Reference {
			return manifest, errors.New("snapshot configuration mismatch")
		}
		revision, err := readProfileFrom(copy, entry.FingerprintID)
		profile := revision.Profile
		if err != nil || !profileMatchesConfiguration(profile, config) || checkProfileEvidence(copy, profile) != nil {
			return manifest, errors.New("snapshot fingerprint mismatch")
		}
		entry.ProfileRevision, entry.ProfileHash, entry.TemplateVersion, entry.GeneratorVersion = profile.ConfigRevision, profile.ConfigHash, profile.TemplateVersion, profile.GeneratorVersion
		entry.DataState = "present"
		manifest.Environments = append(manifest.Environments, entry)
		// Every immutable history revision, not just its current pointer.
		history, err := copy.QueryContext(ctx, "SELECT profile_json FROM fingerprint_revisions WHERE fingerprint_id=? ORDER BY revision", entry.FingerprintID)
		if err != nil {
			return manifest, err
		}
		profiles := []DeviceProfile{}
		for history.Next() {
			var text string
			var value DeviceProfile
			if err = history.Scan(&text); err != nil || decode(json.RawMessage(text), &value) != nil {
				history.Close()
				return manifest, errors.New("invalid snapshot revision")
			}
			profiles = append(profiles, value)
		}
		err = history.Err()
		history.Close()
		if err != nil {
			return manifest, err
		}
		for _, value := range profiles {
			if checkProfile(value) != nil || checkProfileEvidence(copy, value) != nil {
				return manifest, errors.New("snapshot historical profile evidence mismatch")
			}
		}
	}
	rows, err = copy.QueryContext(ctx, "SELECT id FROM kernels ORDER BY id")
	if err != nil {
		return manifest, err
	}
	kernels := []string{}
	for rows.Next() {
		var value string
		if err = rows.Scan(&value); err != nil {
			rows.Close()
			return manifest, err
		}
		kernels = append(kernels, value)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return manifest, err
	}
	for _, kernelID := range kernels {
		entry := backup.Kernel{ID: kernelID, State: "pending"}
		if kernelID != PendingKernelID {
			record, err := savedKernelFrom(copy, kernelID, false)
			if err != nil {
				return manifest, err
			}
			entry.State, entry.Version, entry.ArchiveSHA256, entry.ExecutableSHA256, entry.Architecture = "exact", record.Version, record.ArchiveSHA256, record.ExecutableSHA256, record.Architecture
		}
		manifest.Kernels = append(manifest.Kernels, entry)
	}
	return manifest, copy.Close()
}
