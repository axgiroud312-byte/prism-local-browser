package workspace

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"github.com/axgiroud312-byte/prism-local-browser/internal/kernel"
)

func (s *Service) prepareBatchDirectory(input BatchDirectoryInput) (BatchDirectoryLease, error) {
	if s.options.PrepareBatchDirectory != nil {
		return s.options.PrepareBatchDirectory(input)
	}
	return kernel.PrepareEmptyProfile(input.Root, kernel.EmptyProfileOwner{EnvironmentID: input.EnvironmentID, PlanID: input.PlanID, Index: input.Index, DataReference: input.DataReference})
}

func (s *Service) batchSnapshotReady(snapshot batchSnapshot) *Error {
	if snapshot.SourceID != "" {
		if s.profileUses[snapshot.SourceID] || s.runtimeOwnsProfileUse(snapshot.SourceID) || s.cookieTasks[snapshot.SourceID] != nil {
			return &Error{Code: "PROFILE_BUSY", Message: "指定源/目标环境正运行、待核对或维护，未复制或修改其代理。", Retryable: true}
		}
		environment, revision, profileID, err := s.readEnvironment(snapshot.SourceID)
		if err != nil {
			return &Error{Code: "NOT_FOUND", Message: "指定源/目标环境无法读取，未改其他环境。", Retryable: true}
		}
		profile, err := readProfileFrom(s.db, profileID)
		if err != nil || revision != snapshot.ExpectedRevision || profile.Profile.ConfigHash != snapshot.Profile.ConfigHash || environment.CoreID != snapshot.Configuration.CoreID {
			return &Error{Code: "REVISION_CONFLICT", Message: "预览后源/目标修订或固定档案已变；旧计划不覆盖新配置，请重新预览。", Retryable: false}
		}
	}
	_, revision, err := batchProxy(s.db, snapshot.Configuration.ProxyID)
	if err != nil || revision != snapshot.ProxyRevision {
		return &Error{Code: "PROXY_REVISION_CONFLICT", Message: "映射代理已删除或修订已变，未复用其他节点或静默直连；请重新预览。", Retryable: false}
	}
	if snapshot.Configuration.CoreID != PendingKernelID {
		if _, err := savedKernelFrom(s.db, snapshot.Configuration.CoreID, true); err != nil {
			return kernelFailure(err).Error
		}
	}
	return nil
}

func (s *Service) journalBatchIdentity(plan batchPlan, item *batchStoredItem) error {
	if plan.Kind == "assign" || item.Identity != nil {
		return nil
	}
	config := item.Snapshot.Configuration
	if plan.Kind != "create" || item.Index != 0 {
		seed, err := s.newSeed(config.Seed)
		if err != nil {
			return err
		}
		config.Seed = seed
	}
	var record *kernel.Record
	if config.CoreID != PendingKernelID {
		stored, err := savedKernelFrom(s.db, config.CoreID, true)
		if err != nil {
			return err
		}
		record = &stored
	}
	profile, err := frozenProfile(config, record, item.Snapshot.Profile.GeneratorVersion, 1, config.CoreID == PendingKernelID)
	if err != nil {
		return err
	}
	identity := &batchIdentity{EnvironmentID: id(), FingerprintID: id(), CreatedAt: timestamp(), Profile: profile}
	available, err := seedAvailable(s.db, profile.Seed, identity.FingerprintID, identity.EnvironmentID)
	if err != nil {
		return err
	}
	if !available {
		return &kernel.Problem{Code: "SEED_CONFLICT", Reason: "prepared-seed-reserved", Message: "首项固定seed已被保存或预约，未偷偷重抽身份；请重新预览明确身份。", Retryable: false}
	}
	encoded, _ := json.Marshal(identity)
	snapshot, _ := json.Marshal(item.Snapshot)
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err = tx.Exec(`INSERT INTO batch_items(plan_id,item_index,snapshot_json,identity_json,state) VALUES(?,?,?,?,'prepared') ON CONFLICT(plan_id,item_index) DO UPDATE SET identity_json=excluded.identity_json,state='prepared' WHERE batch_items.identity_json IS NULL AND batch_items.state<>'completed'`, plan.ID, item.Index, string(snapshot), string(encoded)); err != nil {
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
	item.Identity, item.State = identity, "prepared"
	return nil
}

func (s *Service) runBatch(ctx context.Context, task *batchTask) {
	defer s.workers.Done()
	defer task.cancel()
	select {
	case s.batchGate <- struct{}{}:
		defer func() { <-s.batchGate }()
	case <-ctx.Done():
		s.finishBatch(task, ctx.Err())
		return
	}
	for {
		s.mu.Lock()
		plan, err := readBatchPlan(s.db, task.planID)
		if err != nil || plan.OperationID != task.operationID {
			s.mu.Unlock()
			s.finishBatch(task, err)
			return
		}
		if ctx.Err() != nil || s.closed || s.closeRequested.Load() || plan.CancelRequested {
			s.mu.Unlock()
			s.finishBatch(task, context.Canceled)
			return
		}
		index, nextErr := s.nextBatchIndex(plan)
		if nextErr != nil {
			s.mu.Unlock()
			s.finishBatch(task, nextErr)
			return
		}
		if index >= plan.Total {
			s.mu.Unlock()
			s.finishBatch(task, nil)
			return
		}
		plan.State = "running"
		plan.Cursor = index
		item, err := s.readBatchItem(plan, plan.Cursor)
		if err != nil {
			s.mu.Unlock()
			s.finishBatch(task, err)
			return
		}
		problem := s.batchSnapshotReady(item.Snapshot)
		if problem == nil {
			err = s.journalBatchIdentity(plan, &item)
			if err != nil {
				var classified *kernel.Problem
				if errors.As(err, &classified) && classified.Code == "SEED_CONFLICT" {
					problem = kernelFailure(err).Error
				} else {
					s.mu.Unlock()
					s.finishBatch(task, err)
					return
				}
			}
		}
		if problem != nil {
			err = s.completeBatchItem(plan, item, problem, nil)
			s.mu.Unlock()
			if err != nil {
				s.finishBatch(task, err)
				return
			}
			continue
		}
		sourceID := item.Snapshot.SourceID
		if sourceID != "" {
			if !s.acquireBatchProfileUse(task, sourceID) {
				s.mu.Unlock()
				s.finishBatch(task, errors.New("batch lease unexpectedly changed"))
				return
			}
		}
		s.mu.Unlock()
		var lease BatchDirectoryLease
		if item.Identity != nil {
			ref, referenceErr := dataReference(item.Identity.EnvironmentID)
			if referenceErr != nil {
				err = referenceErr
			} else {
				lease, err = s.prepareBatchDirectory(BatchDirectoryInput{Root: s.root, DataReference: ref, EnvironmentID: item.Identity.EnvironmentID, PlanID: plan.ID, Index: item.Index})
			}
		}
		s.mu.Lock()
		// This lease is ours (not a running session); no client could acquire it.
		if sourceID != "" {
			s.releaseBatchProfileUse(task, sourceID)
		}
		current, readErr := readBatchPlan(s.db, task.planID)
		if readErr != nil || current.OperationID != task.operationID {
			s.mu.Unlock()
			if lease != nil {
				_ = lease.Close()
			}
			s.finishBatch(task, readErr)
			return
		}
		if ctx.Err() != nil || s.closed || s.closeRequested.Load() || current.CancelRequested {
			s.mu.Unlock()
			if lease != nil {
				_ = lease.Close()
			}
			s.finishBatch(task, context.Canceled)
			return
		}
		if err != nil {
			var classified *kernel.Problem
			if !errors.As(err, &classified) || classified.Code == "DISK_FULL" || classified.Code == "DIRECTORY_WRITE_FAILED" {
				s.mu.Unlock()
				if lease != nil {
					_ = lease.Close()
				}
				s.finishBatch(task, err)
				return
			}
			problem = kernelFailure(err).Error
		} else {
			problem = s.batchSnapshotReady(item.Snapshot)
		}
		err = s.completeBatchItem(current, item, problem, lease)
		s.mu.Unlock()
		if lease != nil {
			_ = lease.Close()
		}
		if err != nil {
			s.finishBatch(task, err)
			return
		}
		// Bound scheduling/lock work, never a product count/instance quota.
		if (item.Index+1)%batchPageSize == 0 {
			select {
			case <-ctx.Done():
			case <-time.After(time.Millisecond):
			}
		}
	}
}

func (s *Service) completeBatchItem(plan batchPlan, item batchStoredItem, problem *Error, lease BatchDirectoryLease) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if problem == nil {
		if plan.Kind == "assign" {
			problem, err = assignBatchEnvironment(tx, item.Snapshot)
		} else {
			problem, err = insertBatchEnvironment(tx, item, lease)
		}
		if err != nil {
			return err
		}
	}
	state := "completed"
	if problem != nil {
		state = "failed"
		plan.Failed++
	} else {
		plan.Completed++
		plan.AttemptCompleted++
	}
	plan.Cursor = item.Index + 1
	plan.Sequence++
	plan.State = "running"
	snapshot, _ := json.Marshal(item.Snapshot)
	identity, _ := json.Marshal(item.Identity)
	issue, _ := json.Marshal(problem)
	if _, err = tx.Exec(`INSERT INTO batch_items(plan_id,item_index,snapshot_json,identity_json,state,error_json) VALUES(?,?,?,?,?,?) ON CONFLICT(plan_id,item_index) DO UPDATE SET state=excluded.state,error_json=excluded.error_json`, plan.ID, item.Index, string(snapshot), nullableJSON(identity), state, nullableJSON(issue)); err != nil {
		return err
	}
	item.State, item.Error = state, problem
	safeItem, _ := json.Marshal(projectBatchItem(plan, item))
	if _, err = tx.Exec(`INSERT INTO batch_item_events(plan_id,item_index,operation_id,state,error_json,item_json,created_at) VALUES(?,?,?,?,?,?,?)`, plan.ID, item.Index, plan.OperationID, state, nullableJSON(issue), string(safeItem), timestamp()); err != nil {
		return err
	}
	operation := batchOperation(plan)
	operation.Stage = "processing-items"
	if err = persistBatchPlan(tx, plan, operation); err != nil {
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
	if task := s.batchTasks[plan.ID]; task != nil {
		task.last = operation
	}
	return nil
}

func assignBatchEnvironment(tx *sql.Tx, snapshot batchSnapshot) (*Error, error) {
	var configJSON string
	var revision int64
	var profileID string
	if err := tx.QueryRow(`SELECT f.config_json,e.revision,e.fingerprint_id FROM environments e JOIN fingerprints f ON f.id=e.fingerprint_id WHERE e.id=? AND NOT EXISTS(SELECT 1 FROM environment_trash WHERE environment_id=e.id)`, snapshot.SourceID).Scan(&configJSON, &revision, &profileID); err != nil {
		return nil, err
	}
	if revision != snapshot.ExpectedRevision {
		return &Error{Code: "REVISION_CONFLICT", Message: "目标修订已变，未覆盖其代理绑定。", Retryable: false}, nil
	}
	var config Configuration
	if decode(json.RawMessage(configJSON), &config) != nil {
		return nil, errors.New("invalid assignment configuration")
	}
	config.ProxyID = snapshot.Configuration.ProxyID
	encoded, _ := json.Marshal(config)
	if _, err := tx.Exec("UPDATE fingerprints SET config_json=? WHERE id=?", string(encoded), profileID); err != nil {
		return nil, err
	}
	result, err := tx.Exec("UPDATE environments SET proxy_id=?,revision=revision+1 WHERE id=? AND revision=?", nullable(config.ProxyID), snapshot.SourceID, revision)
	if err != nil {
		return nil, err
	}
	count, err := result.RowsAffected()
	if err != nil || count != 1 {
		return nil, errors.New("batch assignment CAS failed")
	}
	return nil, nil
}

func insertBatchEnvironment(tx *sql.Tx, item batchStoredItem, lease BatchDirectoryLease) (*Error, error) {
	if item.Identity == nil || lease == nil {
		return nil, errors.New("new profile directory not prepared")
	}
	identity := item.Identity
	config := item.Snapshot.Configuration
	config.Seed = identity.Profile.Seed
	if checkProfile(identity.Profile) != nil || checkProfileEvidence(tx, identity.Profile) != nil || !profileMatchesConfiguration(identity.Profile, config) {
		return nil, errors.New("invalid prepared identity")
	}
	var nameCount int
	if err := tx.QueryRow("SELECT COUNT(*) FROM environments WHERE name=?", config.Name).Scan(&nameCount); err != nil {
		return nil, err
	}
	if nameCount != 0 {
		return &Error{Code: "NAME_CONFLICT", Message: "预览名称已被占用，未偷偷改名或覆盖已有环境；请重新预览。", Retryable: false}, nil
	}
	available, seedErr := seedAvailable(tx, config.Seed, identity.FingerprintID, identity.EnvironmentID)
	if seedErr != nil {
		return nil, seedErr
	}
	if !available {
		return &Error{Code: "SEED_CONFLICT", Message: "已准备身份的seed被占用，未偷偷重生成或复用旧身份。", Retryable: false}, nil
	}
	if err := lease.CheckEmpty(); err != nil {
		return kernelFailure(err).Error, nil
	}
	var code int64
	if err := tx.QueryRow("SELECT COALESCE(MAX(code),0)+1 FROM environments").Scan(&code); err != nil {
		return nil, err
	}
	if code > maxSafeInteger {
		return &Error{Code: "RESOURCE_EXHAUSTED", Message: "环境编号超出当前精确数值存储能力，已完成项保留；不是实例产品配额。", Retryable: true}, nil
	}
	encoded, _ := json.Marshal(config)
	if _, err := tx.Exec(`INSERT INTO fingerprints(id,seed,kernel_id,template_version,generator_version,config_json) VALUES(?,?,?,?,?,?)`, identity.FingerprintID, config.Seed, config.CoreID, identity.Profile.TemplateVersion, identity.Profile.GeneratorVersion, string(encoded)); err != nil {
		return nil, err
	}
	ref, err := dataReference(identity.EnvironmentID)
	if err != nil {
		return nil, err
	}
	if _, err = tx.Exec(`INSERT INTO environments(id,code,name,kernel_id,proxy_id,fingerprint_id,revision,created_at,user_data_ref) VALUES(?,?,?,?,?,?,1,?,?)`, identity.EnvironmentID, code, config.Name, config.CoreID, nullable(config.ProxyID), identity.FingerprintID, identity.CreatedAt, ref); err != nil {
		return nil, err
	}
	if err = insertDataState(tx, identity.EnvironmentID, dataDirectoryPrepared); err != nil {
		return nil, err
	}
	if err = appendProfile(tx, identity.FingerprintID, identity.Profile, "batch-new-identity", 0); err != nil {
		return nil, err
	}
	return nil, nil
}
