// Package backup owns the native package format. It never parses demo snapshots
// and never returns browser contents or protected credentials to the UI.
package backup

import (
	"archive/zip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"strings"
)

const Format = "prism-local-backup"
const Version = 1
const Extension = ".prismbackup"

type File struct {
	Path      string `json:"path"`
	Size      int64  `json:"size"`
	SHA256    string `json:"sha256,omitempty"`
	Directory bool   `json:"directory"`
}
type Environment struct {
	ID               string `json:"id"`
	Seed             string `json:"seed"`
	Revision         int64  `json:"revision"`
	FingerprintID    string `json:"fingerprintId"`
	ProfileRevision  int64  `json:"profileRevision"`
	ProfileHash      string `json:"profileHash"`
	TemplateVersion  string `json:"templateVersion"`
	GeneratorVersion string `json:"generatorVersion"`
	KernelID         string `json:"kernelId"`
	ProxyID          string `json:"proxyId"`
	DataReference    string `json:"dataReference"`
	DataState        string `json:"dataState"` // present or never-initialized; not inferred from a missing used directory
}
type Kernel struct {
	ID               string `json:"id"`
	State            string `json:"state"` // exact or pending; pending has no invented hashes/version
	Version          string `json:"version,omitempty"`
	ArchiveSHA256    string `json:"archiveSha256,omitempty"`
	ExecutableSHA256 string `json:"executableSha256,omitempty"`
	Architecture     string `json:"architecture,omitempty"`
}
type Manifest struct {
	Format                 string        `json:"format"`
	SchemaVersion          int           `json:"schemaVersion"`
	WorkspaceSchema        int           `json:"workspaceSchema"`
	AppVersion             string        `json:"appVersion"`
	BackupID               string        `json:"backupId"`
	CreatedAt              string        `json:"createdAt"`
	Scope                  string        `json:"scope"`
	Configuration          string        `json:"configuration"`
	Credentials            string        `json:"credentials"`
	BrowserData            string        `json:"browserData"`
	KernelBinariesIncluded bool          `json:"kernelBinariesIncluded"`
	Environments           []Environment `json:"environments"`
	Kernels                []Kernel      `json:"kernels"`
	Files                  []File        `json:"files"`
}

func ValidName(name string) bool {
	if name == "" || strings.HasPrefix(name, "/") || strings.ContainsAny(name, `\:<>"|?*`) {
		return false
	}
	for _, part := range strings.Split(strings.TrimSuffix(name, "/"), "/") {
		if part == "" || part == "." || part == ".." || strings.TrimRight(part, ". ") != part {
			return false
		}
		for _, c := range part {
			if c < 32 {
				return false
			}
		}
		stem := strings.ToUpper(strings.SplitN(part, ".", 2)[0])
		if stem == "CON" || stem == "PRN" || stem == "AUX" || stem == "NUL" || stem == "CONIN$" || stem == "CONOUT$" || stem == "CLOCK$" || len([]rune(stem)) == 4 && (strings.HasPrefix(stem, "COM") || strings.HasPrefix(stem, "LPT")) && strings.ContainsRune("123456789¹²³", []rune(stem)[3]) {
			return false
		}
	}
	return true
}

type contextReader struct {
	ctx    context.Context
	reader io.Reader
}

func (r contextReader) Read(p []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	return r.reader.Read(p)
}

type Writer struct {
	writer *zip.Writer
	Files  []File
	names  map[string]bool
}

func NewWriter(output io.Writer) *Writer {
	return &Writer{writer: zip.NewWriter(output), Files: []File{}, names: map[string]bool{}}
}
func (w *Writer) Add(ctx context.Context, name string, reader io.Reader, size int64, directory bool) error {
	key := strings.ToLower(strings.TrimSuffix(name, "/"))
	if !ValidName(name) || w.names[key] || size < 0 {
		return errors.New("unsafe or repeated package entry")
	}
	w.names[key] = true
	header := &zip.FileHeader{Name: name, Method: zip.Deflate}
	header.SetMode(0600)
	if directory {
		header.Name = strings.TrimSuffix(name, "/") + "/"
		header.Method = zip.Store
		header.SetMode(os.ModeDir | 0700)
	}
	output, err := w.writer.CreateHeader(header)
	if err != nil {
		return err
	}
	entry := File{Path: header.Name, Size: size, Directory: directory}
	if !directory {
		digest := sha256.New()
		count, err := io.Copy(io.MultiWriter(output, digest), contextReader{ctx, reader})
		if err != nil {
			return err
		}
		if count != size {
			return errors.New("source size changed during backup")
		}
		entry.SHA256 = hex.EncodeToString(digest.Sum(nil))
	}
	w.Files = append(w.Files, entry)
	return ctx.Err()
}
func (w *Writer) Finish(ctx context.Context, manifest Manifest) (string, error) {
	manifest.Files = w.Files
	encoded, err := json.Marshal(manifest)
	if err != nil {
		return "", err
	}
	if err = w.Add(ctx, "manifest.json", strings.NewReader(string(encoded)), int64(len(encoded)), false); err != nil {
		return "", err
	}
	if err = w.writer.Close(); err != nil {
		return "", err
	}
	digest := sha256.Sum256(encoded)
	return hex.EncodeToString(digest[:]), nil
}

// Read back every byte of the just-written package through the same owned file.
// This is not the untrusted import/preflight service (T16).
func VerifyWritten(ctx context.Context, file *os.File, files []File) error {
	info, err := file.Stat()
	if err != nil {
		return err
	}
	reader, err := zip.NewReader(file, info.Size())
	if err != nil {
		return err
	}
	if len(reader.File) != len(files) {
		return errors.New("package entry count mismatch")
	}
	for index, entry := range reader.File {
		expected := files[index]
		if entry.Name != expected.Path || int64(entry.UncompressedSize64) != expected.Size || entry.FileInfo().IsDir() != expected.Directory {
			return errors.New("package entry differs from captured source")
		}
		input, err := entry.Open()
		if err != nil {
			return err
		}
		digest := sha256.New()
		_, err = io.Copy(digest, contextReader{ctx, input})
		closeErr := input.Close()
		if err != nil {
			return err
		}
		if closeErr != nil {
			return closeErr
		}
		if !expected.Directory && hex.EncodeToString(digest.Sum(nil)) != expected.SHA256 {
			return errors.New("package file digest mismatch")
		}
	}
	return nil
}
