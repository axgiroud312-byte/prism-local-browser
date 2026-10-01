package workspace

import (
	"database/sql"
	"encoding/json"
	"errors"
)

// The item decision and its configuration changes are committed together. A
// COMMIT error is resolved by the recovery reader before any directory rollback.
// Caller holds s.mu. Purge must finish once destructive work was authorized,
// even when cancellation requests that the remaining items not be started.
func (s *Service) commitRecycleItem(task *recycleTask, plan *recyclePlan, index int) error {
	if s.recycleTask != task {
		return errors.New("recycle owner changed")
	}
	next := copyRecyclePlan(*plan)
	item := &next.Items[index]
	entry := item.Entry
	if !item.Prepared || item.Committed || next.Action == "purge" && !item.DataDeleted {
		return errors.New("invalid recycle commit decision")
	}
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	baseline, err := recycleBaseline(tx, entry.Item.EnvironmentID)
	if err != nil || baseline != entry.Baseline {
		return errors.New("recycle baseline changed before commit")
	}
	if err := recycleMembership(tx, entry, next.Action != "remove", false); err != nil {
		return err
	}
	var profileID string
	if err := tx.QueryRow("SELECT fingerprint_id FROM environments WHERE id=? AND revision=?", entry.Item.EnvironmentID, entry.Item.Revision).Scan(&profileID); err != nil {
		return err
	}
	// Stopped runtime records describe the old live location. Keep activities and
	// operation history, but do not ask startup to inspect that obsolete location.
	for _, table := range []string{"runtime_events", "runtime_sessions"} {
		if _, err := tx.Exec("DELETE FROM "+table+" WHERE environment_id=?", entry.Item.EnvironmentID); err != nil {
			return err
		}
	}
	if next.Action == "purge" {
		for _, q := range []string{"DELETE FROM environment_trash WHERE environment_id=?", "DELETE FROM environments WHERE id=?"} {
			if _, err := tx.Exec(q, entry.Item.EnvironmentID); err != nil {
				return err
			}
		}
		if _, err := tx.Exec("DELETE FROM fingerprints WHERE id=?", profileID); err != nil {
			return err
		}
	} else {
		result, err := tx.Exec("UPDATE environments SET revision=revision+1 WHERE id=? AND revision=?", entry.Item.EnvironmentID, entry.Item.Revision)
		if err != nil {
			return err
		}
		if n, err := result.RowsAffected(); err != nil || n != 1 {
			return errors.New("recycle revision conflict")
		}
		if next.Action == "restore" {
			if _, err := tx.Exec("DELETE FROM environment_trash WHERE environment_id=? AND trash_id=?", entry.Item.EnvironmentID, entry.TrashID); err != nil {
				return err
			}
		}
	}
	item.AfterBaseline, err = recycleBaseline(tx, entry.Item.EnvironmentID)
	if err != nil {
		return err
	}
	if next.Action == "remove" {
		retained := committedRecycleEntry(entry, item.AfterBaseline)
		encoded, err := json.Marshal(retained)
		if err != nil {
			return err
		}
		if _, err := tx.Exec("INSERT INTO environment_trash(environment_id,trash_id,entry_json) VALUES(?,?,?)", entry.Item.EnvironmentID, entry.TrashID, string(encoded)); err != nil {
			return err
		}
	}
	item.Committed = true
	item.Result = map[string]string{"remove": "recycled", "restore": "restored", "purge": "purged"}[next.Action]
	item.Error = nil
	op := recycleCounts(task.operation, next)
	if err := saveRecycleJournal(tx, next, op, "item-committed"); err != nil {
		return err
	}
	if s.options.BeforeCommit != nil {
		if err := s.options.BeforeCommit(); err != nil {
			return err
		}
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	*plan = next
	task.plan = copyRecyclePlan(next)
	task.operation = op
	task.phase = "item-committed"
	return nil
}

func committedRecycleEntry(entry recycleEntry, baseline string) recycleEntry {
	entry.Item.ID = entry.TrashID
	entry.Item.Revision++
	entry.Item.State = "recycled"
	entry.Item.Error = nil
	entry.Baseline = baseline
	return entry
}

// Baselines intentionally omit entry_json (which itself contains the baseline).
// Membership and the complete retained inventory are checked independently.
func recycleMembership(query profileQuery, entry recycleEntry, present, committedRemove bool) error {
	var trashID string
	err := query.QueryRow("SELECT trash_id FROM environment_trash WHERE environment_id=?", entry.Item.EnvironmentID).Scan(&trashID)
	if !present {
		if err != sql.ErrNoRows {
			return errors.New("unexpected recycle membership")
		}
		return nil
	}
	if err != nil || trashID != entry.TrashID {
		return errors.New("recycle membership differs")
	}
	stored, err := readRecycleEntry(query, trashID)
	if err != nil {
		return err
	}
	if committedRemove {
		entry = committedRecycleEntry(entry, entry.Baseline)
	}
	// Backup existence is a preview observation, not part of profile ownership.
	entry.Item.BackupRecorded = stored.Item.BackupRecorded
	a, _ := json.Marshal(entry)
	b, _ := json.Marshal(stored)
	if string(a) != string(b) {
		return errors.New("retained recycle inventory differs")
	}
	return nil
}
