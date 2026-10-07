//go:build windows

package backup

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/google/uuid"
)

func TestMigrationOutputIsFixedOwnedPathWithoutWeakeningExternalBoundary(t *testing.T) {
	root := t.TempDir()
	id := uuid.NewString()
	dir := filepath.Join(root, "backups", "migrations", id)
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	if _, err := NewOutput(root, filepath.Join(dir, "external.prismbackup"), id); err == nil {
		t.Fatal("external destination entered source workspace")
	}
	if _, err := NewMigrationOutput(root, "../other"); err == nil {
		t.Fatal("migration owner accepted traversal")
	}
	output, err := NewMigrationOutput(root, id)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = output.File.WriteString("SYNTHETIC_INTERNAL_BACKUP"); err != nil {
		t.Fatal(err)
	}
	if err = output.Publish(); err != nil {
		t.Fatal(err)
	}
	if err = output.Close(); err != nil {
		t.Fatal(err)
	}
	if bytes, err := os.ReadFile(filepath.Join(dir, "before.prismbackup")); err != nil || string(bytes) != "SYNTHETIC_INTERNAL_BACKUP" {
		t.Fatal("fixed owned publication failed")
	}
	if _, err = NewMigrationOutput(root, id); err == nil {
		t.Fatal("existing retained backup was replaceable")
	}
}
