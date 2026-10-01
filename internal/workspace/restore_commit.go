package workspace

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
)

// A directory is actually installed even for an archived never-used profile.
// Facts about former use are conservative and monotonic across restoration.
func restoredDataState(old, archived string) string {
	if old == dataRuntimeClaimed || archived == dataRuntimeClaimed {
		return dataRuntimeClaimed
	}
	if old == dataLegacyUnconfirmed || old != dataDirectoryPrepared && archived == dataLegacyUnconfirmed {
		return dataLegacyUnconfirmed
	}
	return dataDirectoryPrepared
}

// The configuration and committed marker share ONE transaction. COMMIT errors
// are resolved by reading this marker, never by guessing from the last phase.
// Caller owns s.mu; the global restore barrier excludes new configuration work.
func (s *Service) commitRestoredConfiguration(task *restoreTask, plan *restorePlan) error {
	if s.restoreTask != task || task.operation.CancelRequested || s.closed || s.closeRequested.Load() {
		return context.Canceled
	}
	for _, item := range plan.Environments {
		if s.runtimeOwnsProfileUse(item.Manifest.ID) || s.runtimePending[item.Manifest.ID] != nil || s.cookieTasks[item.Manifest.ID] != nil {
			return errors.New("restore target still controlled")
		}
	}
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	baseline, err := restoreBaseline(tx)
	if err != nil {
		return err
	}
	if baseline != plan.Baseline {
		return &Error{Code: "REVISION_CONFLICT", Message: "恢复提交前配置基线已改变；目录将回滚。", Retryable: true}
	}
	ids := make([]string, 0, len(plan.Environments))
	for _, item := range plan.Environments {
		ids = append(ids, item.Manifest.ID)
	}
	encodedIDs, _ := json.Marshal(ids)
	oldCredentialRefs := []string{}
	for _, p := range plan.Proxies {
		var oldRef, oldJSON string
		var oldRevision int64
		var oldProtected []byte
		err = tx.QueryRow("SELECT COALESCE(p.credential_ref,''),c.config_json,c.revision FROM proxies p JOIN proxy_config c ON c.proxy_id=p.id WHERE p.id=?", p.ID).Scan(&oldRef, &oldJSON, &oldRevision)
		if err != nil && err != sql.ErrNoRows {
			return err
		}
		if oldRef != "" {
			oldCredentialRefs = append(oldCredentialRefs, oldRef)
			if err = tx.QueryRow("SELECT protected FROM proxy_credentials WHERE ref=?", oldRef).Scan(&oldProtected); err != nil {
				return err
			}
		}
		configJSON, _ := json.Marshal(p.Configuration)
		var outside bool
		if err = tx.QueryRow("SELECT EXISTS(SELECT 1 FROM environments WHERE proxy_id=? AND id NOT IN (SELECT value FROM json_each(?)))", p.ID, string(encodedIDs)).Scan(&outside); err != nil {
			return err
		}
		if outside {
			var oldConfig any
			var newConfig any
			json.Unmarshal([]byte(oldJSON), &oldConfig)
			json.Unmarshal(configJSON, &newConfig)
			oldCanonical, _ := json.Marshal(oldConfig)
			newCanonical, _ := json.Marshal(newConfig)
			if !bytes.Equal(oldCanonical, newCanonical) || oldRef != p.Ref || !bytes.Equal(oldProtected, p.Protected) {
				return errors.New("shared proxy changed since preflight")
			}
			continue // Do not invalidate a package-external running channel.
		}
		if p.Ref != "" {
			var existing []byte
			err = tx.QueryRow("SELECT protected FROM proxy_credentials WHERE ref=?", p.Ref).Scan(&existing)
			if err == nil && !bytes.Equal(existing, p.Protected) {
				return errors.New("credential reference conflict")
			}
			if err != nil && err != sql.ErrNoRows {
				return err
			}
			if _, err = tx.Exec("INSERT INTO proxy_credentials(ref,protected) VALUES(?,?) ON CONFLICT(ref) DO NOTHING", p.Ref, p.Protected); err != nil {
				return err
			}
		}
		if _, err = tx.Exec("INSERT INTO proxies(id,credential_ref) VALUES(?,?) ON CONFLICT(id) DO UPDATE SET credential_ref=excluded.credential_ref", p.ID, nullable(p.Ref)); err != nil {
			return err
		}
		revision := max(oldRevision, p.Revision) + 1
		if _, err = tx.Exec("INSERT INTO proxy_config(proxy_id,config_json,revision,check_json) VALUES(?,?,?,NULL) ON CONFLICT(proxy_id) DO UPDATE SET config_json=excluded.config_json,revision=excluded.revision,check_json=NULL", p.ID, string(configJSON), revision); err != nil {
			return err
		}
	}
	// Free just the selected names first, so valid A/B name swaps are atomic.
	for _, item := range plan.Environments {
		if item.ExistingRevision == 0 {
			continue
		}
		temporary := "restore-" + plan.ID + "-" + item.Manifest.ID
		var exists bool
		if err = tx.QueryRow("SELECT EXISTS(SELECT 1 FROM environments WHERE name=?)", temporary).Scan(&exists); err != nil {
			return err
		}
		if exists {
			return errors.New("temporary name conflict")
		}
		if _, err = tx.Exec("UPDATE environments SET name=? WHERE id=? AND revision=?", temporary, item.Manifest.ID, item.ExistingRevision); err != nil {
			return err
		}
	}
	for _, item := range plan.Environments {
		m := item.Manifest
		config := item.Environment.Configuration
		config.CoreID = plan.KernelMapping[m.KernelID]
		if config.CoreID == "" {
			return errors.New("exact kernel mapping missing")
		}
		configJSON, _ := json.Marshal(config)
		if len(item.History) == 0 {
			return errors.New("restored history missing")
		}
		current := item.History[len(item.History)-1].Profile
		var oldState string
		code := item.Code
		if item.ExistingRevision > 0 {
			if err = tx.QueryRow("SELECT e.code,d.state FROM environments e JOIN environment_data_state d ON d.environment_id=e.id WHERE e.id=? AND e.revision=? AND e.fingerprint_id=?", m.ID, item.ExistingRevision, m.FingerprintID).Scan(&code, &oldState); err != nil {
				return err
			}
			if _, err = tx.Exec("DELETE FROM fingerprint_revisions WHERE fingerprint_id=?", m.FingerprintID); err != nil {
				return err
			}
			if _, err = tx.Exec("UPDATE fingerprints SET seed=?,kernel_id=?,template_version=?,generator_version=?,config_json=?,config_revision=? WHERE id=?", m.Seed, config.CoreID, current.TemplateVersion, current.GeneratorVersion, string(configJSON), current.ConfigRevision, m.FingerprintID); err != nil {
				return err
			}
		} else {
			var used bool
			if err = tx.QueryRow("SELECT EXISTS(SELECT 1 FROM environments WHERE code=?)", code).Scan(&used); err != nil {
				return err
			}
			if used {
				if err = tx.QueryRow("SELECT COALESCE(MAX(code),0)+1 FROM environments").Scan(&code); err != nil {
					return err
				}
			}
			if _, err = tx.Exec("INSERT INTO fingerprints(id,seed,kernel_id,template_version,generator_version,config_json,config_revision) VALUES(?,?,?,?,?,?,?)", m.FingerprintID, m.Seed, config.CoreID, current.TemplateVersion, current.GeneratorVersion, string(configJSON), current.ConfigRevision); err != nil {
				return err
			}
		}
		for _, historical := range item.History {
			p := historical.Profile
			p.KernelID = plan.KernelMapping[p.KernelID]
			if p.KernelID == "" {
				return errors.New("historical exact kernel missing")
			}
			p.ConfigHash = profileHash(p)
			if err = checkProfile(p); err != nil {
				return err
			}
			if err = checkProfileEvidence(tx, p); err != nil {
				return err
			}
			text, _ := json.Marshal(p)
			var restored any
			if historical.RestoredFrom > 0 {
				restored = historical.RestoredFrom
			}
			if _, err = tx.Exec("INSERT INTO fingerprint_revisions(fingerprint_id,revision,kernel_id,profile_json,created_at,action,restored_from) VALUES(?,?,?,?,?,?,?)", m.FingerprintID, p.ConfigRevision, p.KernelID, string(text), historical.CreatedAt, historical.Action, restored); err != nil {
				return err
			}
		}
		revision := max(item.ExistingRevision, m.Revision) + 1
		if _, err = tx.Exec(`INSERT INTO environments(id,code,name,kernel_id,proxy_id,fingerprint_id,revision,created_at,user_data_ref) VALUES(?,?,?,?,?,?,?,?,?) ON CONFLICT(id) DO UPDATE SET name=excluded.name,kernel_id=excluded.kernel_id,proxy_id=excluded.proxy_id,revision=excluded.revision,created_at=excluded.created_at,user_data_ref=excluded.user_data_ref`, m.ID, code, config.Name, config.CoreID, nullable(config.ProxyID), m.FingerprintID, revision, item.Environment.CreatedAt, m.DataReference); err != nil {
			return err
		}
		if _, err = tx.Exec("INSERT INTO environment_data_state(environment_id,state) VALUES(?,?) ON CONFLICT(environment_id) DO UPDATE SET state=excluded.state", m.ID, restoredDataState(oldState, item.DataState)); err != nil {
			return err
		}
		if _, err = tx.Exec("DELETE FROM runtime_sessions WHERE environment_id=?", m.ID); err != nil {
			return err
		}
	}
	for _, ref := range oldCredentialRefs {
		if _, err = tx.Exec("DELETE FROM proxy_credentials WHERE ref=? AND NOT EXISTS(SELECT 1 FROM proxies WHERE credential_ref=?)", ref, ref); err != nil {
			return err
		}
	}
	op := copyRestoreOperation(task.operation)
	op.Stage = "db-committed"
	op.RestoreReport.Sequence++
	op.RestoreReport.Committed = true
	plan.CommittedBaseline, err = restoreAffectedBaseline(tx, *plan)
	if err != nil {
		return err
	}
	if err = saveRestoreJournal(tx, *plan, op, "db-committed"); err != nil {
		return err
	}
	marker, err := tx.Exec("UPDATE restore_jobs SET committed=1 WHERE operation_id=? AND committed=0", plan.ID)
	if err != nil {
		return err
	}
	count, err := marker.RowsAffected()
	if err != nil || count != 1 {
		return errors.New("restore commit marker missing or already set")
	}
	if _, err = tx.Exec("INSERT INTO activities(id,created_at,action,target,detail) VALUES(?,?,?,?,?)", id(), timestamp(), "恢复配置已提交", fmt.Sprintf("%d 个环境", len(plan.Environments)), "目录结果仍需最终核对；保留原身份与回滚副本。"); err != nil {
		return err
	}
	if s.options.BeforeCommit != nil {
		if err = s.options.BeforeCommit(); err != nil {
			return err
		}
	}
	if err = tx.Commit(); err != nil {
		return err
	}
	task.operation = op
	task.plan = *plan
	task.phase = "db-committed"
	return nil
}
