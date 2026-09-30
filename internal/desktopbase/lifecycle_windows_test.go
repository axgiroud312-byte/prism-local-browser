//go:build windows

package desktopbase

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestRuntimeRequirements(t *testing.T) {
	for _, version := range []string{"", "0.0.0.0", "93.0.0.0", "invalid"} {
		if ValidateRuntimeVersion(version) == nil {
			t.Fatalf("accepted unavailable/old version %q", version)
		}
	}
	for _, version := range []string{"94.0.992.31", "154.0.4258.37"} {
		if err := ValidateRuntimeVersion(version); err != nil {
			t.Fatal(err)
		}
	}
}

func TestLifecycleLockBlocksSecondOpenAndRemovalThenReleases(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := os.Mkdir(root, 0700); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(root, "app.db")
	if err := os.WriteFile(file, []byte("synthetic retained data"), 0600); err != nil {
		t.Fatal(err)
	}
	lock, err := Acquire(root)
	if err != nil {
		t.Fatal(err)
	}
	if other, err := Acquire(root); err == nil {
		other.Close()
		t.Fatal("second open accepted")
	}
	if err := removeWorkspace(root); err == nil {
		t.Fatal("removed running workspace")
	}
	if data, err := os.ReadFile(file); err != nil || string(data) != "synthetic retained data" {
		t.Fatal("busy removal changed data")
	}
	lock.Close()
	if err := removeWorkspace(root); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(root); !os.IsNotExist(err) {
		t.Fatal("workspace still exists")
	}
}

func TestRemovalRejectsRootAndNestedJunctionWithoutTouchingTarget(t *testing.T) {
	base := t.TempDir()
	outside := filepath.Join(base, "not-prism")
	if err := os.Mkdir(outside, 0700); err != nil {
		t.Fatal(err)
	}
	sentinel := filepath.Join(outside, "keep.txt")
	os.WriteFile(sentinel, []byte("unrelated synthetic data"), 0600)
	for _, nested := range []bool{false, true} {
		root := filepath.Join(base, "workspace-root")
		link := root
		if nested {
			os.Mkdir(root, 0700)
			link = filepath.Join(root, "linked-child")
		}
		if output, err := exec.Command("cmd.exe", "/c", "mklink", "/J", link, outside).CombinedOutput(); err != nil {
			t.Fatalf("junction fixture: %v %s", err, output)
		}
		if err := removeWorkspace(root); err == nil {
			t.Fatal("junction removal accepted")
		}
		if data, err := os.ReadFile(sentinel); err != nil || string(data) != "unrelated synthetic data" {
			t.Fatal("junction target changed")
		}
		if err := os.Remove(link); err != nil {
			t.Fatal(err)
		}
		if nested {
			os.Remove(root)
		}
	}
}

func TestRemovalOfMissingWorkspaceIsIdempotent(t *testing.T) {
	root := filepath.Join(t.TempDir(), "absent")
	if err := removeWorkspace(root); err != nil {
		t.Fatal(err)
	}
	if err := removeWorkspace(root); err != nil {
		t.Fatal(err)
	}
}
