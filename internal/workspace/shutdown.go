package workspace

import (
	"errors"

	"github.com/axgiroud312-byte/prism-local-browser/internal/proxy"
)

// ShutdownError exposes only an exit classification to the desktop. CanExit
// means owned workers/resources have ended and the storage close calls have
// returned. A close error is retained; neither it nor protected maintenance is
// promoted to success by a later idempotent sql.DB.Close call.
type ShutdownError struct {
	Code    string
	CanExit bool
	cause   error
}

func (e *ShutdownError) Error() string { return e.cause.Error() }
func (e *ShutdownError) Unwrap() error { return e.cause }

// Caller holds mu, after all workers and original resource owners have ended.
// Retry only retained observations and the exact preflight scratch. Normal RPC
// remains rejected by closeRequested, including while the database is retained.
func (s *Service) finishShutdownLocked() error {
	if s.closeStorageClosed {
		return s.closeError
	}
	// A COMMIT acknowledgement may have been lost before shutdown. Reuse only
	// the original receipt/journal confirmation. Worker entrances reject closing
	// services, so confirmation never schedules a fresh directory action.
	if task := s.restoreTask; task != nil && task.acceptancePending {
		_ = s.confirmRestoreAcceptance(task, task.signature)
	}
	if task := s.recycleTask; task != nil && task.acceptancePending {
		_ = s.confirmRecycleAcceptance(task)
	}
	if task := s.migrationTask; task != nil && task.acceptancePending {
		_ = s.confirmMigrationAcceptance(task)
	}
	s.flushRuntimePersistence()
	s.flushProxyPersistence()
	s.flushCookiePersistence()
	s.flushBatchPersistence()
	s.flushBackupPersistence()
	s.flushRestorePersistence()
	s.flushRecyclePersistence()
	s.flushMigrationPersistence()
	s.restoreCloseError = s.finishRestorePreflightCleanup()
	if pending := s.shutdownPendingWrites(); pending != nil {
		return &ShutdownError{Code: "EXIT_STORAGE_PENDING", cause: errors.Join(pending, s.restoreCloseError)}
	}
	if s.restoreCloseError != nil {
		return &ShutdownError{Code: "EXIT_SCRATCH_PENDING", cause: s.restoreCloseError}
	}
	proxy.Wipe(s.proxyRequestKey)
	s.proxyRequestKey = nil
	storageError := s.db.Close()
	if s.networkStore != nil {
		storageError = errors.Join(storageError, s.networkStore.Close())
	}
	s.closeStorageClosed = true
	protected := s.shutdownProtectedOutcome()
	if storageError != nil {
		// sql.DB.Close marks the DB closed before the driver returns; subsequent
		// calls return nil without retrying that driver. Retain the first error
		// and offer only an acknowledged incomplete exit, not a false retry.
		s.closeError = &ShutdownError{Code: "EXIT_CLOSE_FAILED", CanExit: true, cause: errors.Join(storageError, protected)}
	} else if protected != nil {
		s.closeError = &ShutdownError{Code: "EXIT_RECOVERY_REQUIRED", CanExit: true, cause: protected}
	}
	return s.closeError
}

func (s *Service) shutdownPendingWrites() error {
	var pending error
	add := func(needed bool, reason string) {
		if needed {
			pending = errors.Join(pending, errors.New(reason))
		}
	}
	add(len(s.runtimePending) != 0, "runtime observations could not all be persisted before shutdown")
	add(len(s.networkRecoveries) != 0, "network cleanup results could not all be persisted before shutdown")
	add(len(s.proxyPending) != 0, "proxy check results could not all be persisted before shutdown")
	add(len(s.cookiePending) != 0, "cookie observations could not all be persisted")
	add(len(s.batchTasks) != 0 || len(s.batchAcceptances) != 0, "batch journal observations could not all be persisted")
	add(len(s.backupTasks) != 0, "backup observations could not all be persisted")
	add(s.restoreTask != nil && (s.restoreTask.finalPending != nil || s.restoreTask.acceptancePending), "restore outcome remains protected or could not be persisted")
	add(s.recycleTask != nil && (s.recycleTask.finalPending != nil || s.recycleTask.acceptancePending), "recycle outcome remains protected or could not be persisted")
	add(s.migrationTask != nil && (s.migrationTask.finalPending != nil || s.migrationTask.acceptancePending), "migration outcome remains protected for next startup")
	return pending
}

func (s *Service) shutdownProtectedOutcome() error {
	var protected error
	if s.restoreTask != nil {
		protected = errors.Join(protected, errors.New("restore outcome remains protected or could not be persisted"))
	}
	if s.recycleTask != nil {
		protected = errors.Join(protected, errors.New("recycle outcome remains protected or could not be persisted"))
	}
	if s.migrationTask != nil {
		protected = errors.Join(protected, errors.New("migration outcome remains protected for next startup"))
	}
	return protected
}
