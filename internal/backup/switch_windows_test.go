//go:build windows

package backup

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"
)

func switchTreeFixture(t *testing.T) (string, TreeIdentity, []File) {
	t.Helper()
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "source", "empty"), 0700); err != nil {
		t.Fatal(err)
	}
	data := []byte("SYNTHETIC_RESTORE_DATA")
	if err := os.WriteFile(filepath.Join(root, "source", "Cookies"), data, 0600); err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(data)
	identity, exists, err := IdentifyTree(root, "source")
	if err != nil || !exists {
		t.Fatal(err)
	}
	return root, identity, []File{{Path: "Cookies", Size: int64(len(data)), SHA256: hex.EncodeToString(sum[:])}, {Path: "empty", Directory: true}}
}
func TestRestoreMovePreservesDirectoryObjectAndCompleteData(t *testing.T) {
	root, identity, files := switchTreeFixture(t)
	ctx := context.Background()
	if err := MoveTree(ctx, root, "source", "destination", identity, files); err != nil {
		t.Fatal(err)
	}
	if err := VerifyTree(ctx, root, "destination", identity, files); err != nil {
		t.Fatal(err)
	}
	if _, exists, err := IdentifyTree(root, "source"); err != nil || exists {
		t.Fatal("source remained", err)
	}
	if err := MoveTree(ctx, root, "destination", "source", identity, files); err != nil {
		t.Fatal(err)
	}
}
func TestRestoreMoveRejectsChangedBytesAndUnownedDestination(t *testing.T) {
	for _, scenario := range []string{"changed", "destination", "identity", "hardlink"} {
		t.Run(scenario, func(t *testing.T) {
			root, identity, files := switchTreeFixture(t)
			switch scenario {
			case "changed":
				os.WriteFile(filepath.Join(root, "source", "Cookies"), []byte("DIFFERENT"), 0600)
			case "destination":
				os.Mkdir(filepath.Join(root, "destination"), 0700)
			case "identity":
				identity.Low++
			case "hardlink":
				if err := os.Link(filepath.Join(root, "source", "Cookies"), filepath.Join(root, "outside-link")); err != nil {
					t.Fatal(err)
				}
			}
			if err := MoveTree(context.Background(), root, "source", "destination", identity, files); err == nil {
				t.Fatal("unsafe move succeeded")
			}
			if _, err := os.Stat(filepath.Join(root, "source", "Cookies")); err != nil {
				t.Fatal("refusal lost source", err)
			}
		})
	}
}
func TestRestoreVerificationRequiresCompleteSetAndExistingLockExclusivity(t *testing.T) {
	root, identity, files := switchTreeFixture(t)
	if err := VerifyTree(context.Background(), root, "source", identity, files[:1]); err == nil {
		t.Fatal("incomplete inventory accepted")
	}
	_, err := PrepareRestoreLock(root, "source")
	if err != nil {
		t.Fatal(err)
	}
	lock, _, err := openCaptured(filepath.Join(root, "source", ".prism-runtime.lock"), false, true)
	if err != nil {
		t.Fatal(err)
	}
	if err = VerifyTree(context.Background(), root, "source", identity, files); err == nil {
		t.Fatal("occupied profile verified")
	}
	lock.Close()
	if err = VerifyTree(context.Background(), root, "source", identity, files); err != nil {
		t.Fatal(err)
	}
}
