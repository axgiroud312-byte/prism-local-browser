//go:build windows

package kernel

import (
	"os"
	"path/filepath"
	"testing"
)

func TestPhysicalExecutableKeepsSameFilePinned(t *testing.T) {
	path := filepath.Join(t.TempDir(), "synthetic.exe")
	if err := os.WriteFile(path, []byte("not executable"), 0600); err != nil {
		t.Fatal(err)
	}
	resolved, release, err := physicalExecutable(path)
	if err != nil {
		t.Fatal(err)
	}
	before, err := os.Stat(path)
	if err != nil {
		release()
		t.Fatal(err)
	}
	after, err := os.Stat(resolved)
	if err != nil || !os.SameFile(before, after) {
		release()
		t.Fatal("resolved a different physical file", err)
	}
	if err := os.Remove(path); err == nil {
		release()
		t.Fatal("executable could be replaced during launch")
	}
	release()
	if err := os.Remove(path); err != nil {
		t.Fatal("launch handle was not released", err)
	}
}

func TestPhysicalExecutableRejectsHardlinkedFile(t *testing.T) {
	directory := t.TempDir()
	path := filepath.Join(directory, "synthetic.exe")
	if err := os.WriteFile(path, []byte("not executable"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Link(path, filepath.Join(directory, "alias.exe")); err != nil {
		t.Fatal(err)
	}
	_, release, err := physicalExecutable(path)
	if err == nil {
		release()
		t.Fatal("hardlinked executable accepted")
	}
}
