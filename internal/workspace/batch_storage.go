package workspace

import (
	"database/sql"
	"encoding/json"
	"errors"
)

func (s *Service) initializeBatches() error {
	var version int
	if err := s.db.QueryRow("PRAGMA user_version").Scan(&version); err != nil {
		return err
	}
	if version == 5 {
		tx, err := s.db.Begin()
		if err != nil {
			return err
		}
		defer tx.Rollback()
		for _, statement := range []string{
			`CREATE TABLE batch_plans(id TEXT PRIMARY KEY,kind TEXT NOT NULL CHECK(kind IN ('create','clone','assign')),total INTEGER NOT NULL CHECK(total>0 AND total<=9007199254740991),template_json TEXT NOT NULL,expires_at TEXT NOT NULL,operation_id TEXT,state TEXT NOT NULL CHECK(state IN ('preview','accepted','running','completed','cancelled','failed')),cancel_requested INTEGER NOT NULL DEFAULT 0,cursor INTEGER NOT NULL DEFAULT 0,completed INTEGER NOT NULL DEFAULT 0,failed INTEGER NOT NULL DEFAULT 0,attempt_completed INTEGER NOT NULL DEFAULT 0,shared INTEGER NOT NULL DEFAULT 0,direct INTEGER NOT NULL DEFAULT 0,sequence INTEGER NOT NULL DEFAULT 0,error_json TEXT)`,
			`CREATE TABLE batch_items(plan_id TEXT NOT NULL REFERENCES batch_plans(id),item_index INTEGER NOT NULL CHECK(item_index>=0),snapshot_json TEXT NOT NULL,identity_json TEXT,state TEXT NOT NULL CHECK(state IN ('not-executed','prepared','completed','failed')),error_json TEXT,PRIMARY KEY(plan_id,item_index))`,
			`CREATE INDEX batch_item_state ON batch_items(plan_id,state,item_index)`,
			`CREATE TABLE batch_item_events(plan_id TEXT NOT NULL REFERENCES batch_plans(id),item_index INTEGER NOT NULL,operation_id TEXT NOT NULL REFERENCES operations(id),state TEXT NOT NULL,error_json TEXT,item_json TEXT NOT NULL,created_at TEXT NOT NULL,PRIMARY KEY(plan_id,item_index,operation_id))`,
			`CREATE INDEX batch_unfinished_item ON batch_items(plan_id,item_index) WHERE state<>'completed'`,
			`CREATE INDEX fingerprint_seed_history ON fingerprint_revisions(json_extract(profile_json,'$.seed'))`,
			`CREATE UNIQUE INDEX batch_reserved_identity ON batch_items(json_extract(identity_json,'$.environmentId')) WHERE identity_json IS NOT NULL`,
			`CREATE UNIQUE INDEX batch_reserved_seed ON batch_items(json_extract(identity_json,'$.profile.seed')) WHERE identity_json IS NOT NULL`,
			`CREATE INDEX environment_proxy_usage ON environments(proxy_id,code)`,
			`CREATE INDEX environment_kernel_usage ON environments(kernel_id,code)`,
			`PRAGMA user_version=6`,
		} {
			if _, err = tx.Exec(statement); err != nil {
				return err
			}
		}
		if err = tx.Commit(); err != nil {
			return err
		}
	}
	for _, statement := range []string{"SELECT id,kind,total,template_json,expires_at,operation_id,state,cancel_requested,cursor,completed,failed,attempt_completed,shared,direct,sequence,error_json FROM batch_plans LIMIT 0", "SELECT plan_id,item_index,snapshot_json,identity_json,state,error_json FROM batch_items LIMIT 0", "SELECT plan_id,item_index,operation_id,state,error_json,item_json,created_at FROM batch_item_events LIMIT 0"} {
		rows, err := s.db.Query(statement)
		if err != nil {
			return err
		}
		rows.Close()
	}
	return nil
}

func readBatchPlan(query profileQuery, planID string) (batchPlan, error) {
	var plan batchPlan
	var template string
	var operationID, problem sql.NullString
	err := query.QueryRow(`SELECT id,kind,total,template_json,expires_at,operation_id,state,cancel_requested,cursor,completed,failed,attempt_completed,shared,direct,sequence,error_json FROM batch_plans WHERE id=?`, planID).Scan(&plan.ID, &plan.Kind, &plan.Total, &template, &plan.ExpiresAt, &operationID, &plan.State, &plan.CancelRequested, &plan.Cursor, &plan.Completed, &plan.Failed, &plan.AttemptCompleted, &plan.Shared, &plan.Direct, &plan.Sequence, &problem)
	if err != nil {
		return plan, err
	}
	plan.OperationID = operationID.String
	if decode(json.RawMessage(template), &plan.Template) != nil || plan.Total < 1 || plan.Total > maxSafeInteger || plan.Completed < 0 || plan.Failed < 0 || plan.Completed+plan.Failed > plan.Total || plan.Cursor < 0 || plan.Cursor > plan.Total {
		return plan, errors.New("invalid batch plan")
	}
	if problem.Valid && json.Unmarshal([]byte(problem.String), &plan.Error) != nil {
		return plan, errors.New("invalid batch error")
	}
	return plan, nil
}
func batchReport(plan batchPlan) BatchReport {
	return BatchReport{Mode: "native", PlanID: plan.ID, Kind: plan.Kind, Total: plan.Total, CompletedCount: plan.Completed, FailedCount: plan.Failed, NotExecutedCount: plan.Total - plan.Completed - plan.Failed, AttemptCompletedCount: plan.AttemptCompleted, SharedProxyAssignments: plan.Shared, DirectAssignments: plan.Direct, Sequence: plan.Sequence}
}
func batchOperation(plan batchPlan) Operation {
	report := batchReport(plan)
	return Operation{ID: plan.OperationID, Kind: "batch-" + plan.Kind, State: plan.State, Total: int(plan.Total), CompletedIDs: []string{}, CancelRequested: plan.CancelRequested, Error: plan.Error, BatchReport: &report}
}
func persistBatchPlan(tx *sql.Tx, plan batchPlan, operation Operation) error {
	problem, _ := json.Marshal(plan.Error)
	if _, err := tx.Exec(`UPDATE batch_plans SET operation_id=?,state=?,cancel_requested=?,cursor=?,completed=?,failed=?,attempt_completed=?,shared=?,sequence=?,error_json=? WHERE id=?`, plan.OperationID, plan.State, plan.CancelRequested, plan.Cursor, plan.Completed, plan.Failed, plan.AttemptCompleted, plan.Shared, plan.Sequence, nullableJSON(problem), plan.ID); err != nil {
		return err
	}
	encoded, err := json.Marshal(operation)
	if err != nil {
		return err
	}
	_, err = tx.Exec(`UPDATE operations SET result_json=? WHERE id=?`, string(encoded), operation.ID)
	return err
}
func nullableJSON(encoded []byte) any {
	if string(encoded) == "null" {
		return nil
	}
	return string(encoded)
}

func (s *Service) readBatchItem(plan batchPlan, index int64) (batchStoredItem, error) {
	item := batchStoredItem{Index: index, State: "not-executed"}
	var snapshot string
	var identity, problem sql.NullString
	err := s.db.QueryRow(`SELECT snapshot_json,identity_json,state,error_json FROM batch_items WHERE plan_id=? AND item_index=?`, plan.ID, index).Scan(&snapshot, &identity, &item.State, &problem)
	if err == sql.ErrNoRows && plan.Kind == "create" {
		item.Snapshot = plan.Template
		if plan.Total > 1 {
			item.Snapshot.Configuration.Name = batchName(plan.Template.Configuration.Name, index)
		}
		return item, nil
	}
	if err != nil {
		return item, err
	}
	if decode(json.RawMessage(snapshot), &item.Snapshot) != nil {
		return item, errors.New("invalid batch item")
	}
	if identity.Valid {
		if decode(json.RawMessage(identity.String), &item.Identity) != nil || item.Identity == nil {
			return item, errors.New("invalid prepared batch identity")
		}
	}
	if problem.Valid && json.Unmarshal([]byte(problem.String), &item.Error) != nil {
		return item, errors.New("invalid batch item error")
	}
	return item, nil
}

// Completed rows are skipped by the unfinished index, without rewriting one
// transaction per prior success. Create's unmaterialized suffix remains work.
func (s *Service) nextBatchIndex(plan batchPlan) (int64, error) {
	next := plan.Total
	err := s.db.QueryRow(`SELECT item_index FROM batch_items WHERE plan_id=? AND state<>'completed' AND item_index>=? ORDER BY item_index LIMIT 1`, plan.ID, plan.Cursor).Scan(&next)
	if err != nil && err != sql.ErrNoRows {
		return 0, err
	}
	if plan.Kind == "create" {
		var frontier int64
		if err = s.db.QueryRow(`SELECT COALESCE(MAX(item_index),-1)+1 FROM batch_items WHERE plan_id=?`, plan.ID).Scan(&frontier); err != nil {
			return 0, err
		}
		if frontier < plan.Cursor {
			frontier = plan.Cursor
		}
		if frontier < next {
			next = frontier
		}
	}
	return next, nil
}
