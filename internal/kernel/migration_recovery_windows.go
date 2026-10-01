//go:build windows

package kernel

import "github.com/google/uuid"

// Only for an unpublished, journal-owned migration work UUID. A host can die
// after creating the exact session Job but before recording lock metadata. For
// this directory, an exclusive existing lock plus that exact Job's empty/gone
// state proves resource release even with absent/old/partial metadata. Ordinary
// environment recovery continues to require its full persisted session identity.
// This does NOT say no browser was created or that it exited normally.
func InspectMigrationWorkProfile(root, workID, sessionID, reference string) (bool, error) {
	if parsed, err := uuid.Parse(sessionID); err != nil || parsed.String() != sessionID {
		return false, problem("SESSION_IDENTITY_UNCONFIRMED", "invalid-migration-session", "迁移会话身份无法核对。")
	}
	lock, err := inspectManagedProfileLock(root, workID, reference, false, true)
	if err != nil {
		return false, err
	}
	defer lock.release()
	return managedJobResourcesExited(sessionID)
}
