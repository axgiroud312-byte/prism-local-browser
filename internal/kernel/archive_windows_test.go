//go:build windows

package kernel

import (
	"archive/zip"
	"context"
	"os"
	"path/filepath"
	"testing"
)

func zipFixture(t *testing.T, names ...string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "synthetic.zip")
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	w := zip.NewWriter(file)
	for _, name := range names {
		entry, err := w.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = entry.Write([]byte("synthetic " + name)); err != nil {
			t.Fatal(err)
		}
	}
	if err = w.Close(); err != nil {
		t.Fatal(err)
	}
	file.Close()
	return path
}

func TestArchiveRejectsWindowsAliasesBeforeWriting(t *testing.T) {
	for _, name := range []string{"../outside.exe", "/chrome.exe", "C:/chrome.exe", `folder\chrome.exe`, "folder/chrome.exe:payload", "folder/CON.txt", "folder/file. ", "folder/./chrome.exe", "folder//chrome.exe", "folder/NUL", "folder/a?b", "folder/COM¹.txt"} {
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			if _, _, err := extractArchive(context.Background(), zipFixture(t, "package/chrome.exe", name), root); err == nil {
				t.Fatal("unsafe archive accepted")
			}
			entries, _ := os.ReadDir(root)
			if len(entries) != 0 {
				t.Fatal("invalid archive wrote partial files before validation")
			}
		})
	}
	for _, names := range [][]string{{"chrome.exe", "CHROME.EXE"}, {"file", "file/chrome.exe"}, {"package/chrome.exe", "other/chrome.exe"}} {
		if _, _, err := extractArchive(context.Background(), zipFixture(t, names...), t.TempDir()); err == nil {
			t.Fatal("ambiguous archive accepted")
		}
	}
}

func TestArchiveRejectsLinksAndOversizedEntries(t *testing.T) {
	for _, mode := range []os.FileMode{os.ModeSymlink | 0700, os.ModeNamedPipe | 0600} {
		path := filepath.Join(t.TempDir(), "link.zip")
		file, _ := os.Create(path)
		w := zip.NewWriter(file)
		header := &zip.FileHeader{Name: "chrome.exe", Method: zip.Store}
		header.SetMode(mode)
		entry, _ := w.CreateHeader(header)
		entry.Write([]byte("outside"))
		w.Close()
		file.Close()
		if _, _, err := extractArchive(context.Background(), path, t.TempDir()); err == nil {
			t.Fatal("link/special file accepted")
		}
	}
	if validArchiveName("chrome.exe") != nil {
		t.Fatal("ordinary executable rejected")
	}
}

func TestArchiveManifestAndCancellation(t *testing.T) {
	root := t.TempDir()
	files, exe, err := extractArchive(context.Background(), zipFixture(t, "package/chrome.exe", "package/chrome.dll", "package/LICENSE"), root)
	if err != nil {
		t.Fatal(err)
	}
	if exe != "package/chrome.exe" || len(files) != 3 || len(files[exe]) != 64 {
		t.Fatal("archive manifest missing exact executable/hash")
	}
	if err = VerifyFiles(root, files); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(root, "package", "chrome.dll"), []byte("tampered"), 0600)
	if err = VerifyFiles(root, files); err == nil {
		t.Fatal("changed DLL accepted")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, _, err = extractArchive(ctx, zipFixture(t, "chrome.exe"), t.TempDir()); err == nil {
		t.Fatal("cancelled extraction succeeded")
	}
}
