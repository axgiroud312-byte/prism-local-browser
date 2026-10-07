package kernel

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"path/filepath"
	"sync"

	"github.com/google/uuid"
	_ "modernc.org/sqlite"
)

// NetworkJournal is private control state, outside app.db and configuration
// backups. Its caller must hold the application lock and pin its parent directory
// for the journal's lifetime. It contains no credentials or browser contents.
// A prepared resource is potentially live: an OS call may have completed just
// before the process died or its acknowledgement failed to persist.
type NetworkJournal struct {
	db      *sql.DB
	mu      sync.Mutex
	actions map[string]*networkJournalAction
}

type networkJournalAction struct {
	mu   sync.Mutex
	refs int
}

func (j *NetworkJournal) lockSession(sessionID string) func() {
	j.mu.Lock()
	action := j.actions[sessionID]
	if action == nil {
		action = &networkJournalAction{}
		j.actions[sessionID] = action
	}
	action.refs++
	j.mu.Unlock()
	action.mu.Lock()
	return func() {
		action.mu.Unlock()
		j.mu.Lock()
		action.refs--
		if action.refs == 0 {
			delete(j.actions, sessionID)
		}
		j.mu.Unlock()
	}
}

type NetworkSessionIntent struct {
	SessionID     string `json:"sessionId"`
	EnvironmentID string `json:"environmentId"`
	ChannelID     string `json:"channelId"`
	DataReference string `json:"dataReference"`
	ContainerName string `json:"containerName"`
	PackageSID    string `json:"packageSid"`
	JobName       string `json:"jobName"`
	LogonSID      string `json:"logonSid"`
	BootID        string `json:"bootId,omitempty"`
}

// ObjectIdentity is the provider's stable OS identity, not merely a path. For a
// file it includes volume and file ID; for a window station, login identity and
// object name. Recovery must reopen and compare it before changing permissions.
// Delta holds only the intended, session-specific permission change.
type NetworkResourceIntent struct {
	ResourceID     string `json:"resourceId"`
	Kind           string `json:"kind"`
	ObjectIdentity string `json:"objectIdentity"`
	Locator        string `json:"locator"`
	Delta          string `json:"delta"`
}

type NetworkResourceRecord struct {
	NetworkResourceIntent
	Sequence int64
	State    string
}

var ErrNetworkJournalConflict = errors.New("network resource journal identity or phase conflict")

// OpenNetworkJournal neither resets corrupt state nor migrates unknown formats.
// SQLite FULL commits are the write-ahead barrier before any external resource
// action. The dedicated database must never be restored from a profile backup.
func OpenNetworkJournal(path string) (*NetworkJournal, error) {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	dbPath := filepath.ToSlash(absolute)
	if dbPath[0] != '/' {
		dbPath = "/" + dbPath
	}
	u := url.URL{Scheme: "file", Path: dbPath}
	q := u.Query()
	for _, pragma := range []string{"foreign_keys(1)", "busy_timeout(5000)", "synchronous(FULL)"} {
		q.Add("_pragma", pragma)
	}
	u.RawQuery = q.Encode()
	db, err := sql.Open("sqlite", u.String())
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	j := &NetworkJournal{db: db, actions: map[string]*networkJournalAction{}}
	if err = j.initialize(); err != nil {
		db.Close()
		return nil, err
	}
	return j, nil
}

func (j *NetworkJournal) initialize() error {
	tx, err := j.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var version, application int
	if err = tx.QueryRow("PRAGMA user_version").Scan(&version); err != nil {
		return err
	}
	if err = tx.QueryRow("PRAGMA application_id").Scan(&application); err != nil {
		return err
	}
	const appID = 0x50524e4a // PRNJ; distinct from the restorable configuration DB
	if version == 0 && application == 0 {
		var tables int
		if err = tx.QueryRow("SELECT count(*) FROM sqlite_master WHERE name NOT LIKE 'sqlite_%'").Scan(&tables); err != nil {
			return err
		}
		if tables != 0 {
			return ErrNetworkJournalConflict
		}
		for _, statement := range []string{
			`CREATE TABLE sessions(session_id TEXT PRIMARY KEY, environment_id TEXT NOT NULL, intent TEXT NOT NULL, phase TEXT NOT NULL CHECK(phase IN ('preparing','cleaning','closed')))`,
			`CREATE UNIQUE INDEX one_live_network_session ON sessions(environment_id) WHERE phase != 'closed'`,
			`CREATE TABLE resources(sequence INTEGER PRIMARY KEY AUTOINCREMENT, session_id TEXT NOT NULL REFERENCES sessions(session_id), resource_id TEXT NOT NULL, intent TEXT NOT NULL, state TEXT NOT NULL CHECK(state IN ('prepared','applied','released')), UNIQUE(session_id,resource_id))`,
			fmt.Sprintf("PRAGMA application_id=%d", appID),
			"PRAGMA user_version=1",
		} {
			if _, err = tx.Exec(statement); err != nil {
				return err
			}
		}
	} else if version != 1 || application != appID {
		return ErrNetworkJournalConflict
	}
	// Even a database with the right version must have the expected columns.
	for _, statement := range []string{"SELECT session_id,environment_id,intent,phase FROM sessions LIMIT 0", "SELECT sequence,session_id,resource_id,intent,state FROM resources LIMIT 0"} {
		rows, queryErr := tx.Query(statement)
		if queryErr != nil {
			return queryErr
		}
		if err = rows.Close(); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (j *NetworkJournal) Close() error { return j.db.Close() }

func (j *NetworkJournal) Session(ctx context.Context, sessionID string) (NetworkSessionIntent, error) {
	var intent NetworkSessionIntent
	var encoded string
	err := j.db.QueryRowContext(ctx, "SELECT intent FROM sessions WHERE session_id=?", sessionID).Scan(&encoded)
	if err != nil {
		return intent, err
	}
	if json.Unmarshal([]byte(encoded), &intent) != nil || !validNetworkSession(intent) || intent.SessionID != sessionID {
		return intent, ErrNetworkJournalConflict
	}
	return intent, nil
}

func validNetworkSession(intent NetworkSessionIntent) bool {
	for _, value := range []string{intent.SessionID, intent.EnvironmentID, intent.ChannelID} {
		parsed, err := uuid.Parse(value)
		if err != nil || parsed == uuid.Nil || parsed.String() != value {
			return false
		}
	}
	return intent.DataReference != "" && intent.ContainerName == "prism-session-"+intent.SessionID &&
		intent.PackageSID != "" && intent.JobName != "" && intent.LogonSID != ""
}

// PrepareSession is idempotent only for the exact original intent. A completed
// session ID is never reusable, and a second session cannot acquire its env.
func (j *NetworkJournal) PrepareSession(ctx context.Context, intent NetworkSessionIntent) error {
	if !validNetworkSession(intent) {
		return ErrNetworkJournalConflict
	}
	encoded, err := json.Marshal(intent)
	if err != nil {
		return err
	}
	tx, err := j.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var old, phase string
	err = tx.QueryRowContext(ctx, "SELECT intent,phase FROM sessions WHERE session_id=?", intent.SessionID).Scan(&old, &phase)
	if err == nil {
		if old != string(encoded) || phase != "preparing" {
			return ErrNetworkJournalConflict
		}
		return tx.Commit()
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	if _, err = tx.ExecContext(ctx, "INSERT INTO sessions(session_id,environment_id,intent,phase) VALUES(?,?,?,'preparing')", intent.SessionID, intent.EnvironmentID, string(encoded)); err != nil {
		return err
	}
	return tx.Commit()
}

func validNetworkResource(intent NetworkResourceIntent) bool {
	if intent.ResourceID == "" || intent.ObjectIdentity == "" || intent.Locator == "" {
		return false
	}
	switch intent.Kind {
	case "container", "job", "bridge":
		return intent.Delta == ""
	case "file-acl", "window-station-acl":
		return intent.Delta != ""
	}
	return false
}

// prepareResource must return success before its corresponding OS operation.
// Applied and released entries cannot be re-prepared to replay an OS action.
func (j *NetworkJournal) prepareResource(ctx context.Context, sessionID string, intent NetworkResourceIntent) error {
	if !validNetworkResource(intent) {
		return ErrNetworkJournalConflict
	}
	encoded, err := json.Marshal(intent)
	if err != nil {
		return err
	}
	tx, err := j.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var phase string
	if err = tx.QueryRowContext(ctx, "SELECT phase FROM sessions WHERE session_id=?", sessionID).Scan(&phase); err != nil {
		return err
	}
	if phase != "preparing" {
		return ErrNetworkJournalConflict
	}
	var old, state string
	err = tx.QueryRowContext(ctx, "SELECT intent,state FROM resources WHERE session_id=? AND resource_id=?", sessionID, intent.ResourceID).Scan(&old, &state)
	if err == nil {
		if old != string(encoded) || state != "prepared" {
			return ErrNetworkJournalConflict
		}
		return tx.Commit()
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	if _, err = tx.ExecContext(ctx, "INSERT INTO resources(session_id,resource_id,intent,state) VALUES(?,?,?,'prepared')", sessionID, intent.ResourceID, string(encoded)); err != nil {
		return err
	}
	return tx.Commit()
}

func (j *NetworkJournal) markApplied(ctx context.Context, sessionID, resourceID string) error {
	result, err := j.db.ExecContext(ctx, `UPDATE resources SET state='applied' WHERE session_id=? AND resource_id=? AND state IN ('prepared','applied') AND EXISTS(SELECT 1 FROM sessions WHERE session_id=? AND phase='preparing')`, sessionID, resourceID, sessionID)
	return networkJournalUpdated(result, err)
}

// ApplyResource serializes the complete intent -> OS effect -> acknowledgement
// interval with Cleanup for this session. A failed callback/acknowledgement
// leaves its intent pending. Ensure must inspect actual identity first because
// a previous prepared operation may have taken effect without acknowledgement.
// Ensure is synchronous: it must not leave resource-creating goroutines behind.
// It must not reenter ApplyResource or Cleanup for the same session.
func (j *NetworkJournal) ApplyResource(ctx context.Context, sessionID string, intent NetworkResourceIntent, ensure func(context.Context) error) error {
	if ensure == nil {
		return ErrNetworkJournalConflict
	}
	release := j.lockSession(sessionID)
	defer release()
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := j.prepareResource(ctx, sessionID, intent); err != nil {
		return err
	}
	if err := ensure(ctx); err != nil {
		return err
	}
	return j.markApplied(ctx, sessionID, intent.ResourceID)
}

func networkJournalUpdated(result sql.Result, err error) error {
	if err != nil {
		return err
	}
	count, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if count != 1 {
		return ErrNetworkJournalConflict
	}
	return nil
}

// PendingSessions is paged by immutable session ID, avoiding an unbounded read
// as the number of saved environments grows. Closed records remain tombstones.
func (j *NetworkJournal) PendingSessions(ctx context.Context, after string) ([]NetworkSessionIntent, error) {
	rows, err := j.db.QueryContext(ctx, "SELECT session_id,environment_id,intent FROM sessions WHERE phase!='closed' AND session_id>? ORDER BY session_id LIMIT 100", after)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []NetworkSessionIntent
	for rows.Next() {
		var sessionID, environmentID, encoded string
		var intent NetworkSessionIntent
		if err = rows.Scan(&sessionID, &environmentID, &encoded); err != nil {
			return nil, err
		}
		if err = json.Unmarshal([]byte(encoded), &intent); err != nil || !validNetworkSession(intent) || intent.SessionID != sessionID || intent.EnvironmentID != environmentID {
			return nil, ErrNetworkJournalConflict
		}
		result = append(result, intent)
	}
	return result, rows.Err()
}

// Cleanup seals creation, then calls confirmStopped under the lifecycle lock
// so a late in-flight launch cannot invalidate an earlier Job-empty observation.
// Undo must inspect actual OS state even for "prepared", remove only its own
// delta, and confirm absence. Unknown identity/state is an error, never success.
// Cleanup waits for in-flight ApplyResource calls and prevents late creation;
// callers retain their owner on any error. Undo cannot reenter same-session
// ApplyResource/Cleanup. Other sessions remain independently operable.
// Resource creation order must be container -> ACLs -> bridge -> Job so reverse
// cleanup stops process ownership before revoking access or deleting identity.
func (j *NetworkJournal) Cleanup(ctx context.Context, sessionID string, confirmStopped func(context.Context) error, undo func(context.Context, NetworkResourceRecord) error) error {
	if confirmStopped == nil || undo == nil {
		return ErrNetworkJournalConflict
	}
	release := j.lockSession(sessionID)
	defer release()
	if err := ctx.Err(); err != nil {
		return err
	}
	var phase string
	if err := j.db.QueryRowContext(ctx, "SELECT phase FROM sessions WHERE session_id=?", sessionID).Scan(&phase); err != nil {
		return err
	}
	if phase == "closed" {
		// The final COMMIT may have succeeded before its acknowledgement was
		// lost. Retrying completion must not resurrect or strand this owner.
		return nil
	}
	result, err := j.db.ExecContext(ctx, "UPDATE sessions SET phase='cleaning' WHERE session_id=? AND phase IN ('preparing','cleaning')", sessionID)
	if err = networkJournalUpdated(result, err); err != nil {
		return err
	}
	if err = confirmStopped(ctx); err != nil {
		return err
	}
	for {
		var record NetworkResourceRecord
		var resourceID, encoded string
		err = j.db.QueryRowContext(ctx, "SELECT sequence,resource_id,intent,state FROM resources WHERE session_id=? AND state!='released' ORDER BY sequence DESC LIMIT 1", sessionID).Scan(&record.Sequence, &resourceID, &encoded, &record.State)
		if errors.Is(err, sql.ErrNoRows) {
			break
		}
		if err != nil {
			return err
		}
		if err = json.Unmarshal([]byte(encoded), &record.NetworkResourceIntent); err != nil || !validNetworkResource(record.NetworkResourceIntent) || record.ResourceID != resourceID {
			return ErrNetworkJournalConflict
		}
		if err = undo(ctx, record); err != nil {
			return err
		}
		// A failed acknowledgement leaves the entry potentially live. Recovery
		// re-observes it; it never assumes the previous undo did not happen.
		result, err = j.db.ExecContext(ctx, "UPDATE resources SET state='released' WHERE session_id=? AND sequence=? AND state=?", sessionID, record.Sequence, record.State)
		if err = networkJournalUpdated(result, err); err != nil {
			return err
		}
	}
	result, err = j.db.ExecContext(ctx, `UPDATE sessions SET phase='closed' WHERE session_id=? AND phase='cleaning' AND NOT EXISTS(SELECT 1 FROM resources WHERE session_id=? AND state!='released')`, sessionID, sessionID)
	return networkJournalUpdated(result, err)
}
