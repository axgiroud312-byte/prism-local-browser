package backup

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/uuid"
	"golang.org/x/sys/windows"
)

func TestWrittenPackageReadsEveryEntryAndManifestDigest(t *testing.T) {
	file, err := os.CreateTemp(t.TempDir(), "synthetic-*.partial")
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	writer := NewWriter(file)
	data := "SYNTHETIC_COOKIE_FILE_BYTES"
	if err = writer.Add(context.Background(), "configuration.sqlite", strings.NewReader(data), int64(len(data)), false); err != nil {
		t.Fatal(err)
	}
	if err = writer.Add(context.Background(), "environments/synthetic/empty/", nil, 0, true); err != nil {
		t.Fatal(err)
	}
	digest, err := writer.Finish(context.Background(), Manifest{Format: Format, SchemaVersion: Version})
	if err != nil || len(digest) != 64 {
		t.Fatal("manifest digest missing", err)
	}
	if err = VerifyWritten(context.Background(), file, writer.Files); err != nil {
		t.Fatal(err)
	}
	altered := append([]File(nil), writer.Files...)
	altered[0].SHA256 = strings.Repeat("0", 64)
	if VerifyWritten(context.Background(), file, altered) == nil {
		t.Fatal("readback accepted a mismatched file digest")
	}
}

type fullSyntheticDisk struct{}

func (fullSyntheticDisk) Write([]byte) (int, error) { return 0, windows.ERROR_DISK_FULL }

func TestDiskFullOutputCannotFinishAValidBackup(t *testing.T) {
	writer := NewWriter(fullSyntheticDisk{})
	err := writer.Add(context.Background(), "configuration.sqlite", strings.NewReader("SYNTHETIC_CONFIG"), 16, false)
	if err == nil {
		_, err = writer.Finish(context.Background(), Manifest{Format: Format, SchemaVersion: Version})
	}
	if !errors.Is(err, windows.ERROR_DISK_FULL) {
		t.Fatal("disk full did not propagate out of the package writer", err)
	}
}

func TestOutputPublishesOwnedObjectWithoutOverwritingExistingFile(t *testing.T) {
	root, destination := t.TempDir(), filepath.Join(t.TempDir(), "synthetic.prismbackup")
	output, err := NewOutput(root, destination, uuid.NewString())
	if err != nil {
		t.Fatal(err)
	}
	output.File.WriteString("SYNTHETIC_COMPLETE_BYTES")
	if err = output.Publish(); err != nil {
		output.Close()
		t.Fatal(err)
	}
	output.Close()
	data, err := os.ReadFile(destination)
	if err != nil || string(data) != "SYNTHETIC_COMPLETE_BYTES" {
		t.Fatal("published object differs", err)
	}
	if _, err = NewOutput(root, destination, uuid.NewString()); err == nil {
		t.Fatal("existing destination can be overwritten")
	}
	if _, err = NewOutput(root, filepath.Join(root, "synthetic.prismbackup"), uuid.NewString()); err == nil {
		t.Fatal("output can overwrite/source-contaminate workspace")
	}
}

func TestCancelledTemporaryOutputDoesNotBecomePublished(t *testing.T) {
	root, destination := t.TempDir(), filepath.Join(t.TempDir(), "synthetic.prismbackup")
	output, err := NewOutput(root, destination, uuid.NewString())
	if err != nil {
		t.Fatal(err)
	}
	writer := NewWriter(output.File)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if writer.Add(ctx, "synthetic-browser-data", strings.NewReader("value"), 5, false) == nil {
		t.Fatal("cancelled data copy succeeded")
	}
	output.Close()
	if _, err = os.Lstat(destination); !os.IsNotExist(err) {
		t.Fatal("cancelled export created a final file")
	}
	if _, err = os.Lstat(output.Temporary); err != nil {
		t.Fatal("owned partial was not retained distinctly", err)
	}
}

func TestCapturePreservesEmptyDirectoriesAndRejectsWritesAndLateEntries(t *testing.T) {
	root, id := t.TempDir(), uuid.NewString()
	ref := "environments/" + id + "/user-data"
	directory := filepath.Join(root, filepath.FromSlash(ref))
	if err := os.MkdirAll(filepath.Join(directory, "empty"), 0700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(directory, "Cookies")
	os.WriteFile(path, []byte("SYNTHETIC_BROWSER_DATA"), 0600)
	profile, err := CaptureProfile(context.Background(), root, id, ref, false, true)
	if err != nil {
		t.Fatal(err)
	}
	defer profile.Close()
	if file, err := os.OpenFile(path, os.O_WRONLY|os.O_TRUNC, 0600); err == nil {
		file.Close()
		t.Fatal("captured file remained writable")
	}
	os.WriteFile(filepath.Join(directory, "late"), []byte("SYNTHETIC_LATE_ENTRY"), 0600)
	if profile.Validate(context.Background()) == nil {
		t.Fatal("new child entry was silently omitted")
	}
}

func TestMissingUninitializedDirectoryDoesNotGetCreatedAndUsedMissingFails(t *testing.T) {
	root, id := t.TempDir(), uuid.NewString()
	ref := "environments/" + id + "/user-data"
	profile, err := CaptureProfile(context.Background(), root, id, ref, true, true)
	if err != nil || !profile.Missing {
		t.Fatal("uninitialized data not distinguished", err)
	}
	profile.Close()
	if _, err = os.Lstat(filepath.Join(root, "environments")); !os.IsNotExist(err) {
		t.Fatal("read-only backup initialized source directories")
	}
	if _, err = CaptureProfile(context.Background(), root, id, ref, false, false); err == nil {
		t.Fatal("missing used data was turned into an empty environment")
	}
}

func TestBackupPathsRejectWindowsAmbiguityAndHardlinkedBrowserData(t *testing.T) {
	for _, name := range []string{"../escape", "C:/absolute", "a:stream", "NUL.txt", "LPT¹.txt", "trailing. ", "bad\\path", "double//entry"} {
		if ValidName(name) {
			t.Fatalf("unsafe name accepted: %q", name)
		}
	}
	root, id := t.TempDir(), uuid.NewString()
	ref := "environments/" + id + "/user-data"
	directory := filepath.Join(root, filepath.FromSlash(ref))
	os.MkdirAll(directory, 0700)
	outside := filepath.Join(t.TempDir(), "synthetic-original")
	os.WriteFile(outside, []byte("SYNTHETIC_UNRELATED"), 0600)
	if err := os.Link(outside, filepath.Join(directory, "Cookies")); err != nil {
		t.Skip("test filesystem cannot create a hardlink")
	}
	if _, err := CaptureProfile(context.Background(), root, id, ref, false, true); err == nil {
		t.Fatal("shared physical browser file treated as independent data")
	}
	data, _ := os.ReadFile(outside)
	if string(data) != "SYNTHETIC_UNRELATED" {
		t.Fatal("outside original was modified")
	}
}
