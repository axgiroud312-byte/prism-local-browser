//go:build windows

package backup

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/google/uuid"
)

func TestDiagnosticOutputRefusesWorkspaceAndExistingFiles(t *testing.T) {
	root, out := t.TempDir(), t.TempDir()
	existing := filepath.Join(out, "existing.json")
	if err := os.WriteFile(existing, []byte("SYNTHETIC_EXISTING"), 0600); err != nil {
		t.Fatal(err)
	}
	for _, destination := range []string{existing, filepath.Join(root, "report.json"), filepath.Join(out, "report.prismbackup"), filepath.Join(out, "report.json:stream")} {
		if output, err := NewDiagnosticOutput(root, destination, uuid.NewString()); err == nil {
			output.Close()
			t.Fatalf("unsafe output accepted: %s", destination)
		}
	}
	if bytes, err := os.ReadFile(existing); err != nil || string(bytes) != "SYNTHETIC_EXISTING" {
		t.Fatal("existing destination changed")
	}
}

func TestDiagnosticDiscardOnlyDeletesUnpublishedOwnedFile(t *testing.T) {
	root, out := t.TempDir(), t.TempDir()
	for _, publish := range []bool{false, true} {
		destination := filepath.Join(out, uuid.NewString()+".json")
		o, err := NewDiagnosticOutput(root, destination, uuid.NewString())
		if err != nil {
			t.Fatal(err)
		}
		if _, err = o.File.WriteString("SYNTHETIC_DIAGNOSTIC"); err != nil {
			t.Fatal(err)
		}
		if publish {
			if err = o.Publish(); err != nil {
				t.Fatal(err)
			}
		}
		err = o.Discard()
		if publish && err == nil || !publish && err != nil {
			t.Fatal("wrong discard boundary")
		}
		if err = o.Close(); err != nil {
			t.Fatal(err)
		}
		if _, err = os.Stat(o.Temporary); !os.IsNotExist(err) {
			t.Fatal("temporary object retained")
		}
		if publish {
			if bytes, err := os.ReadFile(destination); err != nil || string(bytes) != "SYNTHETIC_DIAGNOSTIC" {
				t.Fatal("published file lost")
			}
		} else if _, err = os.Stat(destination); !os.IsNotExist(err) {
			t.Fatal("discard published a destination")
		}
	}
}
