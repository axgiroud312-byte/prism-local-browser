package backup

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

func importPackageFixture(t *testing.T, change func(*Manifest), extra string) *os.File {
	t.Helper()
	file, err := os.Create(filepath.Join(t.TempDir(), "synthetic.prismbackup"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { file.Close() })
	content := []byte("SYNTHETIC_CONFIGURATION")
	sum := sha256.Sum256(content)
	m := Manifest{Format: Format, SchemaVersion: Version, WorkspaceSchema: 7, AppVersion: "synthetic", BackupID: uuid.NewString(), CreatedAt: time.Now().UTC().Format(time.RFC3339Nano), Scope: "all", Configuration: "configuration.sqlite", Credentials: "windows-current-user-dpapi", BrowserData: "sensitive-same-user-not-portable", Environments: []Environment{}, Kernels: []Kernel{}, Files: []File{{Path: "configuration.sqlite", Size: int64(len(content)), SHA256: hex.EncodeToString(sum[:])}}}
	if change != nil {
		change(&m)
	}
	w := zip.NewWriter(file)
	entry, err := w.Create("configuration.sqlite")
	if err != nil {
		t.Fatal(err)
	}
	entry.Write(content)
	if extra != "" {
		entry, err = w.Create(extra)
		if err != nil {
			t.Fatal(err)
		}
		entry.Write([]byte("SYNTHETIC_EXTRA"))
	}
	entry, err = w.Create("manifest.json")
	if err != nil {
		t.Fatal(err)
	}
	encoded, _ := json.Marshal(m)
	entry.Write(encoded)
	if err = w.Close(); err != nil {
		t.Fatal(err)
	}
	return file
}

func TestImportVerifiesSameFileFullDigestAndStreamsDeclaredConfiguration(t *testing.T) {
	file := importPackageFixture(t, nil, "")
	p, err := Read(context.Background(), file)
	if err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	if err = p.Copy(context.Background(), "configuration.sqlite", &output); err != nil || output.String() != "SYNTHETIC_CONFIGURATION" {
		t.Fatal("verified payload differs", err)
	}
	whole, err := os.ReadFile(file.Name())
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(whole)
	if p.ArchiveSHA256 != hex.EncodeToString(sum[:]) {
		t.Fatal("archive digest not bound to selected bytes")
	}
}
func TestImportRejectsUntrustedPathsExtraPayloadAndDigestChanges(t *testing.T) {
	for _, name := range []string{"../escape", "C:/escape", "configuration.sqlite/child", "CONFIGURATION.SQLITE", "environments/foreign/user-data/secret", "safe:stream", "NUL.txt"} {
		t.Run(name, func(t *testing.T) {
			if _, err := Read(context.Background(), importPackageFixture(t, nil, name)); err == nil {
				t.Fatal("unsafe or undeclared payload accepted")
			}
		})
	}
	for _, change := range []func(*Manifest){func(m *Manifest) { m.Files[0].SHA256 = strings.Repeat("0", 64) }, func(m *Manifest) { m.Files = append(m.Files, m.Files[0]) }, func(m *Manifest) { m.WorkspaceSchema = 999 }, func(m *Manifest) { m.Format = "prism-prototype" }, func(m *Manifest) { m.KernelBinariesIncluded = true }} {
		if _, err := Read(context.Background(), importPackageFixture(t, change, "")); err == nil {
			t.Fatal("invalid manifest accepted")
		}
	}
}
func TestImportStrictJSONRejectsDuplicateCaseUnknownNullAndMissingFields(t *testing.T) {
	for _, text := range []string{`{"path":"x","path":"y","size":0,"directory":true}`, `{"path":"x","Path":"y","size":0,"directory":true}`, `{"Path":"x","size":0,"directory":true}`, `{"path":"x","size":0,"directory":true,"extra":1}`, `{"path":"x","size":null,"directory":true}`, `{"path":"x","size":0}`} {
		var entry File
		if DecodeJSON([]byte(text), &entry) == nil {
			t.Fatalf("accepted ambiguous JSON: %s", text)
		}
	}
}
func TestImportCancellationDoesNotProduceSuccess(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := Read(ctx, importPackageFixture(t, nil, "")); err == nil {
		t.Fatal("cancelled preflight reported successful validation")
	}
}

func TestImportBoundsCentralDirectoryBeforeArchiveAllocation(t *testing.T) {
	file := importPackageFixture(t, nil, "")
	data, err := os.ReadFile(file.Name())
	if err != nil {
		t.Fatal(err)
	}
	end := len(data) - 22
	binary.LittleEndian.PutUint32(data[end+12:], 129<<20)
	if checkZIPBudget(context.Background(), bytes.NewReader(data), int64(len(data))) == nil {
		t.Fatal("unbounded directory declaration accepted")
	}
	data, _ = os.ReadFile(file.Name())
	binary.LittleEndian.PutUint16(data[end+8:], 1)
	binary.LittleEndian.PutUint16(data[end+10:], 1)
	if checkZIPBudget(context.Background(), bytes.NewReader(data), int64(len(data))) == nil {
		t.Fatal("declared count hid additional directory entries")
	}
}

func TestImportRejectsAmbiguousEndRecordsAndTrailingDataBeforeZIPParser(t *testing.T) {
	file := importPackageFixture(t, nil, "")
	original, err := os.ReadFile(file.Name())
	if err != nil {
		t.Fatal(err)
	}
	// EOCD A's comment contains EOCD B and a tail byte. A requires a small
	// directory; B advertises a huge one. Different end-record selection must
	// never let the standard parser bypass the checked resource budget.
	data := append([]byte(nil), original...)
	a := len(data) - 22
	binary.LittleEndian.PutUint16(data[a+20:], 23)
	b := append([]byte(nil), original[a:]...)
	binary.LittleEndian.PutUint32(b[12:], 129<<20)
	data = append(data, b...)
	data = append(data, 0)
	if checkZIPBudget(context.Background(), bytes.NewReader(data), int64(len(data))) == nil {
		t.Fatal("ambiguous EOCD accepted")
	}
}
