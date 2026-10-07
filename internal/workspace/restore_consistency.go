package workspace

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"path/filepath"

	"github.com/axgiroud312-byte/prism-local-browser/internal/backup"
	"github.com/axgiroud312-byte/prism-local-browser/internal/kernel"
)

// Only records this restoration can replace participate. Unrelated stopped
// sessions may be reconciled without invalidating a known configuration outcome.
func restoreAffectedBaseline(query interface {
	Query(string, ...any) (*sql.Rows, error)
}, plan restorePlan) (string, error) {
	ids, proxies := []string{}, []string{}
	for _, e := range plan.Environments {
		ids = append(ids, e.Manifest.ID)
	}
	for _, p := range plan.Proxies {
		proxies = append(proxies, p.ID)
	}
	environmentsJSON, _ := json.Marshal(ids)
	proxiesJSON, _ := json.Marshal(proxies)
	e := "SELECT value FROM json_each(?)"
	f := "SELECT fingerprint_id FROM environments WHERE id IN (" + e + ")"
	queries := []struct{ sql, arg string }{
		{"SELECT id,code,name,kernel_id,COALESCE(proxy_id,''),fingerprint_id,revision,created_at,user_data_ref FROM environments WHERE id IN (" + e + ") ORDER BY id", string(environmentsJSON)},
		{"SELECT id,seed,kernel_id,template_version,generator_version,config_json,config_revision FROM fingerprints WHERE id IN (" + f + ") ORDER BY id", string(environmentsJSON)},
		{"SELECT fingerprint_id,revision,kernel_id,profile_json,created_at,action,COALESCE(restored_from,0) FROM fingerprint_revisions WHERE fingerprint_id IN (" + f + ") ORDER BY fingerprint_id,revision", string(environmentsJSON)},
		{"SELECT environment_id,state FROM environment_data_state WHERE environment_id IN (" + e + ") ORDER BY environment_id", string(environmentsJSON)},
		{"SELECT id,COALESCE(credential_ref,'') FROM proxies WHERE id IN (" + e + ") ORDER BY id", string(proxiesJSON)},
		{"SELECT proxy_id,config_json,revision FROM proxy_config WHERE proxy_id IN (" + e + ") ORDER BY proxy_id", string(proxiesJSON)},
		{"SELECT ref,hex(protected) FROM proxy_credentials WHERE ref IN (SELECT credential_ref FROM proxies WHERE id IN (" + e + ")) ORDER BY ref", string(proxiesJSON)},
	}
	// Preserve v2 interrupted journals across schema9 migration. Only new plans
	// include recycle membership in their versioned configuration digest.
	if plan.JournalVersion >= 3 {
		queries = append(queries, struct{ sql, arg string }{"SELECT environment_id,trash_id,entry_json FROM environment_trash WHERE environment_id IN (" + e + ") ORDER BY environment_id", string(environmentsJSON)})
	}
	h := sha256.New()
	for _, q := range queries {
		h.Write([]byte(q.sql))
		rows, err := query.Query(q.sql, q.arg)
		if err != nil {
			return "", err
		}
		cols, err := rows.Columns()
		if err != nil {
			rows.Close()
			return "", err
		}
		for rows.Next() {
			values := make([]any, len(cols))
			dest := make([]any, len(cols))
			for i := range dest {
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

func (s *Service) pinRestoredKernels(ctx context.Context, plan restorePlan) (func(), error) {
	releases := []func(){}
	release := func() {
		for i := len(releases) - 1; i >= 0; i-- {
			releases[i]()
		}
	}
	checked := map[string]bool{}
	for _, e := range plan.Environments {
		for _, h := range e.History {
			localID := plan.KernelMapping[h.Profile.KernelID]
			if localID == PendingKernelID || checked[localID] {
				continue
			}
			if localID == "" {
				release()
				return nil, errors.New("exact kernel reference missing")
			}
			if err := ctx.Err(); err != nil {
				release()
				return nil, err
			}
			s.mu.Lock()
			record, err := savedKernelFrom(s.db, localID, true)
			s.mu.Unlock()
			if err != nil {
				release()
				return nil, err
			}
			directory, err := kernel.RecordDirectory(s.root, record)
			if err != nil {
				release()
				return nil, err
			}
			pin, err := kernel.PinFiles(directory, record.Files)
			if err != nil {
				release()
				return nil, err
			}
			releases = append(releases, pin)
			if err = kernel.VerifyFiles(directory, record.Files); err != nil {
				release()
				return nil, err
			}
			version, err := kernel.FileVersion(filepath.Join(directory, filepath.FromSlash(record.ExecutableRelativePath)))
			if err != nil || version != record.Version {
				release()
				return nil, errors.New("restored exact kernel version differs")
			}
			checked[localID] = true
		}
	}
	return release, nil
}

func (s *Service) verifyRestoreConfigurationOutcome(plan restorePlan, committed bool) error {
	if !plan.Prepared && !committed {
		return nil
	}
	want := plan.PreviousBaseline
	if committed {
		want = plan.CommittedBaseline
	}
	if !backup.Hash(want) {
		return errors.New("restore configuration baseline unavailable")
	}
	got, err := restoreAffectedBaseline(s.db, plan)
	if err != nil {
		return err
	}
	if got != want {
		return errors.New("configuration does not match durable restore decision")
	}
	return nil
}
