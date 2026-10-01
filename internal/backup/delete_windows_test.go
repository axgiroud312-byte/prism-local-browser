//go:build windows

package backup

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"golang.org/x/sys/windows"
)

func TestRecycleDeleteRejectsUnknownChangedAndSharedContentsBeforeDeleting(t *testing.T) {
	for _, scenario := range []string{"extra", "changed", "hardlink", "unknown-lock", "readonly", "occupied"} {
		t.Run(scenario, func(t *testing.T) {
			root, identity, files := switchTreeFixture(t)
			original := filepath.Join(root, "source", "Cookies")
			var closeHandle func()
			switch scenario {
			case "extra":
				if err := os.WriteFile(filepath.Join(root, "source", "unknown"), []byte("KEEP"), 0600); err != nil {
					t.Fatal(err)
				}
			case "changed":
				if err := os.WriteFile(original, []byte("KEEP_CHANGED"), 0600); err != nil {
					t.Fatal(err)
				}
			case "hardlink":
				if err := os.Link(original, filepath.Join(root, "outside")); err != nil {
					t.Fatal(err)
				}
			case "unknown-lock":
				if err := os.WriteFile(filepath.Join(root, "source", ".prism-runtime.lock"), []byte("UNAUTHORIZED"), 0600); err != nil {
					t.Fatal(err)
				}
			case "readonly":
				if err := os.Chmod(original, 0400); err != nil {
					t.Fatal(err)
				}
				defer os.Chmod(original, 0600)
			case "occupied":
				f, _, err := openCaptured(original, false, true)
				if err != nil {
					t.Fatal(err)
				}
				closeHandle = func() { f.Close() }
			}
			if closeHandle != nil {
				defer closeHandle()
			}
			if err := DeleteRetainedTree(context.Background(), root, "source", identity, files, nil, true); err == nil {
				t.Fatal("unsafe purge accepted", scenario)
			}
			if _, err := os.Stat(filepath.Join(root, "source", "empty")); err != nil {
				t.Fatal("refusal partially deleted directory", err)
			}
			if _, err := os.Stat(original); err != nil {
				t.Fatal("refusal lost original", err)
			}
		})
	}
}

func TestRecycleDeleteResumesOnlyAuthorizedMissingEntries(t *testing.T) {
	root, identity, files := switchTreeFixture(t)
	if err := os.Remove(filepath.Join(root, "source", "Cookies")); err != nil {
		t.Fatal(err)
	}
	if err := DeleteRetainedTree(context.Background(), root, "source", identity, files, nil, false); err == nil {
		t.Fatal("first pass accepted missing inventory")
	}
	if err := DeleteRetainedTree(context.Background(), root, "source", identity, files, nil, true); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, "source")); !os.IsNotExist(err) {
		t.Fatal("retained root not absent", err)
	}
	if err := DeleteRetainedTree(context.Background(), root, "source", identity, files, nil, true); err != nil {
		t.Fatal("idempotent authorized retry failed", err)
	}
}

func TestRecycleDeleteBindsRuntimeLockObjectAndDigest(t *testing.T) {
	for _, changed := range []bool{false, true} {
		t.Run(map[bool]string{false: "matching", true: "replacement"}[changed], func(t *testing.T) {
			root, identity, files := switchTreeFixture(t)
			path := filepath.Join(root, "source", ".prism-runtime.lock")
			data := []byte("SYNTHETIC_SESSION_OWNER")
			if err := os.WriteFile(path, data, 0600); err != nil {
				t.Fatal(err)
			}
			f, info, err := openCaptured(path, false, true)
			if err != nil {
				t.Fatal(err)
			}
			f.Close()
			sum := sha256.Sum256(data)
			lock := &RetainedLock{Identity: treeIdentity(info), Size: int64(len(data)), SHA256: hex.EncodeToString(sum[:])}
			if changed {
				if err := os.Rename(path, path+".original"); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(path, data, 0600); err != nil {
					t.Fatal(err)
				}
				if err := os.Rename(path+".original", filepath.Join(root, "preserved-lock")); err != nil {
					t.Fatal(err)
				}
			}
			err = DeleteRetainedTree(context.Background(), root, "source", identity, files, lock, true)
			if changed && err == nil || !changed && err != nil {
				t.Fatal("lock authorization differs", err)
			}
		})
	}
}

func TestRecycleDeleteDoesNotCompleteWhileRootIsDeletePending(t *testing.T) {
	root, identity, files := switchTreeFixture(t)
	name, _ := windows.UTF16PtrFromString(filepath.Join(root, "source"))
	h, err := windows.CreateFile(name, windows.GENERIC_READ, windows.FILE_SHARE_READ|windows.FILE_SHARE_DELETE, nil, windows.OPEN_EXISTING, windows.FILE_FLAG_BACKUP_SEMANTICS, 0)
	if err != nil {
		t.Fatal(err)
	}
	err = DeleteRetainedTree(context.Background(), root, "source", identity, files, nil, true)
	windows.CloseHandle(h)
	if err == nil {
		t.Fatal("delete-pending root incorrectly reported absent")
	}
	if err = DeleteRetainedTree(context.Background(), root, "source", identity, files, nil, true); err != nil {
		t.Fatal("released reader did not permit completion", err)
	}
}

func TestRecycleMoveBetweenDifferentPinnedParents(t *testing.T) {
	root, identity, files := switchTreeFixture(t)
	if err := os.Mkdir(filepath.Join(root, "destination-parent"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := MoveTree(context.Background(), root, "source", "destination-parent/retained", identity, files); err != nil {
		t.Fatal(err)
	}
	if err := VerifyTree(context.Background(), root, "destination-parent/retained", identity, files); err != nil {
		t.Fatal(err)
	}
}

func TestRecycleDeleteRejectsTraversalAndJunctionWithoutTouchingOutside(t *testing.T) {
	for _, scenario := range []string{"traversal", "junction"} {
		t.Run(scenario, func(t *testing.T) {
			root, identity, files := switchTreeFixture(t)
			outside := t.TempDir()
			marker := filepath.Join(outside, "keep.txt")
			if err := os.WriteFile(marker, []byte("SYNTHETIC_OUTSIDE_KEEP"), 0600); err != nil {
				t.Fatal(err)
			}
			relative := "../outside"
			if scenario == "junction" {
				relative = "source"
				link := filepath.Join(root, "source", "empty")
				if err := os.Remove(link); err != nil {
					t.Fatal(err)
				}
				if output, err := exec.Command("cmd.exe", "/c", "mklink", "/J", link, outside).CombinedOutput(); err != nil {
					t.Fatalf("synthetic junction fixture: %v %s", err, output)
				}
				defer os.Remove(link)
			}
			if err := DeleteRetainedTree(context.Background(), root, relative, identity, files, nil, true); err == nil {
				t.Fatal("unsafe path authorized")
			}
			if data, err := os.ReadFile(marker); err != nil || string(data) != "SYNTHETIC_OUTSIDE_KEEP" {
				t.Fatal("outside bytes changed", err)
			}
		})
	}
}
