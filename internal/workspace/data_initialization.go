package workspace

import (
	"context"
	"database/sql"
	"errors"
)

const (
	dataNeverInitialized  = "never-initialized"
	dataDirectoryPrepared = "directory-prepared"
	dataRuntimeClaimed    = "runtime-claimed"
	dataLegacyUnconfirmed = "legacy-unconfirmed"
)

func insertDataState(tx *sql.Tx, environmentID, state string) error {
	_, err := tx.Exec("INSERT INTO environment_data_state(environment_id,state) VALUES(?,?)", environmentID, state)
	return err
}

// Persistent permission to initialize data is separate from accepting a start.
// A failure after this marker is conservative: absence can no longer mean empty.
// The marker must commit before the launcher may create directories or locks.
func (s *Service) claimRuntimeData(ctx context.Context, slot *runtimeSlot) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if ctx.Err() != nil || s.closed || s.closeRequested.Load() || s.runtimeSlots[slot.session.EnvironmentID] != slot {
		return context.Canceled
	}
	tx, err := s.db.Begin()
	if err != nil {
		return &runtimePersistenceFailure{cause: err}
	}
	defer tx.Rollback()
	result, err := tx.Exec("UPDATE environment_data_state SET state=? WHERE environment_id=?", dataRuntimeClaimed, slot.session.EnvironmentID)
	if err == nil {
		var count int64
		count, err = result.RowsAffected()
		if err == nil && count != 1 {
			err = errors.New("data initialization fact missing")
		}
	}
	if err == nil && s.options.BeforeCommit != nil {
		err = s.options.BeforeCommit()
	}
	if err == nil {
		err = tx.Commit()
	}
	if err != nil {
		return &runtimePersistenceFailure{cause: err}
	}
	return nil
}

func (s *Service) backupDataTarget(environmentID, reference string) (backupTarget, error) {
	return backupDataTargetFrom(s.db, environmentID, reference)
}
func backupDataTargetFrom(query profileQuery, environmentID, reference string) (backupTarget, error) {
	target := backupTarget{ID: environmentID, Reference: reference}
	var state string
	if err := query.QueryRow("SELECT state FROM environment_data_state WHERE environment_id=?", environmentID).Scan(&state); err != nil {
		return target, err
	}
	switch state {
	case dataNeverInitialized:
		target.NeverUsed = true
	case dataDirectoryPrepared:
		target.NeverUsed, target.DirectoryRequired = true, true
	case dataRuntimeClaimed, dataLegacyUnconfirmed:
		target.DirectoryRequired = true
	default:
		return target, errors.New("data initialization fact unconfirmed")
	}
	return target, nil
}
