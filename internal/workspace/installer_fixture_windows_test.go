//go:build windows

package workspace

import (
	"database/sql"
	"os"
	"path/filepath"
	"testing"

	"github.com/axgiroud312-byte/prism-local-browser/internal/desktopbase"
	"github.com/google/uuid"
)

// Used only by verify-installer -NoClicks after it proves both actual default
// roots are absent. Never part of the installed executable or an ordinary test.
func TestCandidateInstallerWorkspaceFixture(t *testing.T) {
	nonce := os.Getenv("PRISM_INSTALLER_FIXTURE_NONCE")
	if nonce == "" {
		t.Skip("owned no-click installer fixture not selected")
	}
	if _, err := uuid.Parse(nonce); err != nil {
		t.Fatal("invalid synthetic ownership nonce")
	}
	root, err := desktopbase.DefaultRoot()
	if err != nil {
		t.Fatal(err)
	}
	lock, err := desktopbase.Acquire(root)
	if err != nil {
		t.Fatal(err)
	}
	defer lock.Close()
	marker, err := os.ReadFile(filepath.Join(root, ".prism-installer-fixture"))
	if err != nil || string(marker) != nonce {
		t.Fatal("default workspace is not owned by the current synthetic verification")
	}
	if err := desktopbase.ValidateTree(root); err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("sqlite", "file:"+filepath.ToSlash(filepath.Join(root, "app.db"))+"?mode=ro&_pragma=query_only(1)")
	if err != nil {
		t.Fatal(err)
	}
	var version, kernelCount int
	if err := db.QueryRow("PRAGMA user_version").Scan(&version); err != nil || version != 10 {
		db.Close()
		t.Fatal("refusing to migrate an unrecognized default workspace", err)
	}
	for _, table := range []string{"environments", "fingerprints", "proxies", "proxy_credentials", "operations", "runtime_sessions", "batch_plans", "backup_exports", "restore_jobs", "recycle_jobs", "kernel_migrations", "kernel_evidence"} {
		var count int
		if err := db.QueryRow("SELECT COUNT(*) FROM " + table).Scan(&count); err != nil || count != 0 {
			db.Close()
			t.Fatal("read-only guard refused nonempty default workspace:", table, err)
		}
	}
	var allKernels int
	if err := db.QueryRow("SELECT COUNT(*) FROM kernels").Scan(&allKernels); err != nil || allKernels != 1 {
		db.Close()
		t.Fatal("read-only guard refused additional kernel configuration", err)
	}
	if err := db.QueryRow("SELECT COUNT(*) FROM kernels WHERE id=? AND status='missing'", PendingKernelID).Scan(&kernelCount); err != nil || kernelCount != 1 {
		db.Close()
		t.Fatal("default workspace does not contain only the pending bootstrap kernel", err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	journal, err := sql.Open("sqlite", "file:"+filepath.ToSlash(filepath.Join(root, "network-resources", "resources.db"))+"?mode=ro&_pragma=query_only(1)")
	if err != nil {
		t.Fatal(err)
	}
	var sessions int
	if err := journal.QueryRow("SELECT COUNT(*) FROM sessions").Scan(&sessions); err != nil || sessions != 0 {
		journal.Close()
		t.Fatal("refusing to recover an existing default network-resource session", err)
	}
	if err := journal.Close(); err != nil {
		t.Fatal(err)
	}
	s, err := Open(root, Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	initial := view(t, s)
	if len(initial.State.Environments) != 0 || len(initial.State.Proxies) != 0 || len(initial.State.Kernels) != 1 || initial.State.Kernels[0].Available {
		t.Fatal("refusing to modify a nonempty or previously configured default workspace")
	}
	created, _ := create(t, s, "SYNTHETIC-INSTALLER-ONLY")
	p := preview(t, s, "edit", created.ID)
	config := p.Environment.Configuration
	config.Group, config.Note, config.Width, config.Height = "SYNTHETIC", "SYNTHETIC-INSTALLER-ONLY", 1440, 900
	value[map[string]any](t, call(s, "Environment.Update", Mutation{PreviewID: p.PreviewID, Configuration: config, ExpectedRevision: p.ExpectedRevision, RequestID: id()}))
	saved := view(t, s).State.Environments[0]
	if saved.ID != created.ID || saved.Seed != created.Seed || saved.CoreID != created.CoreID {
		t.Fatal("synthetic API edit changed saved identity")
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
}
