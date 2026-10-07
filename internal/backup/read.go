package backup

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"math"
	"os"
	"path"
	"reflect"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
)

// These are parser resource limits, not environment/save quotas. Browser files
// are streamed, never accumulated in memory or extracted during preflight.
const maxManifestBytes = 32 << 20
const MaxConfigurationBytes = 256 << 20

type Invalid struct {
	Reason      string
	Unsupported bool
}

func (e *Invalid) Error() string  { return e.Reason }
func invalid(reason string) error { return &Invalid{Reason: reason} }

// Reject duplicate keys as well as unknown fields: ordinary encoding/json would
// silently let the last value replace a previously validated identity/version.
func DecodeJSON(data []byte, target any) error {
	d := json.NewDecoder(bytes.NewReader(data))
	depth := 0
	var value func() error
	value = func() error {
		depth++
		defer func() { depth-- }()
		if depth > 100 {
			return invalid("json-depth-limit")
		}
		token, err := d.Token()
		if err != nil {
			return err
		}
		if delimiter, ok := token.(json.Delim); ok {
			switch delimiter {
			case '{':
				keys := map[string]bool{}
				for d.More() {
					key, err := d.Token()
					if err != nil {
						return err
					}
					text, ok := key.(string)
					folded := strings.ToLower(text)
					if !ok || keys[folded] {
						return invalid("duplicate-json-key")
					}
					keys[folded] = true
					if err = value(); err != nil {
						return err
					}
				}
			case '[':
				for d.More() {
					if err = value(); err != nil {
						return err
					}
				}
			default:
				return invalid("invalid-json")
			}
			_, err = d.Token()
			return err
		}
		return nil
	}
	if err := value(); err != nil {
		return err
	}
	if _, err := d.Token(); err != io.EOF {
		return invalid("trailing-json")
	}
	if !utf8.Valid(data) {
		return invalid("invalid-json-utf8")
	}
	if err := jsonShape(data, reflect.TypeOf(target).Elem()); err != nil {
		return err
	}
	d = json.NewDecoder(bytes.NewReader(data))
	d.DisallowUnknownFields()
	return d.Decode(target)
}

func jsonShape(data []byte, kind reflect.Type) error {
	if kind.Kind() == reflect.Pointer {
		if bytes.Equal(bytes.TrimSpace(data), []byte("null")) {
			return nil
		}
		return jsonShape(data, kind.Elem())
	}
	if bytes.Equal(bytes.TrimSpace(data), []byte("null")) {
		return invalid("null-required-field")
	}
	switch kind.Kind() {
	case reflect.Struct:
		var object map[string]json.RawMessage
		if json.Unmarshal(data, &object) != nil {
			return invalid("expected-object")
		}
		fields := map[string]reflect.StructField{}
		var gather func(reflect.Type)
		gather = func(t reflect.Type) {
			for i := 0; i < t.NumField(); i++ {
				f := t.Field(i)
				if f.Anonymous {
					gather(f.Type)
					continue
				}
				name := strings.Split(f.Tag.Get("json"), ",")[0]
				if name != "-" && f.IsExported() {
					if name == "" {
						name = f.Name
					}
					fields[name] = f
				}
			}
		}
		gather(kind)
		for name, raw := range object {
			f, ok := fields[name]
			if !ok {
				return invalid("unknown-json-field")
			}
			if err := jsonShape(raw, f.Type); err != nil {
				return err
			}
		}
		for name, f := range fields {
			if _, ok := object[name]; !ok && !strings.Contains(f.Tag.Get("json"), "omitempty") {
				return invalid("missing-json-field")
			}
		}
	case reflect.Slice, reflect.Array:
		var entries []json.RawMessage
		if json.Unmarshal(data, &entries) != nil {
			return invalid("expected-array")
		}
		for _, raw := range entries {
			if err := jsonShape(raw, kind.Elem()); err != nil {
				return err
			}
		}
	case reflect.Map:
		var entries map[string]json.RawMessage
		if json.Unmarshal(data, &entries) != nil {
			return invalid("expected-map")
		}
		for _, raw := range entries {
			if err := jsonShape(raw, kind.Elem()); err != nil {
				return err
			}
		}
	}
	return nil
}

type Package struct {
	Manifest                      Manifest
	ArchiveSHA256, ManifestSHA256 string
	Bytes                         int64
	files                         map[string]*zip.File
	declared                      map[string]File
}

func CanonicalID(text string) bool {
	value, err := uuid.Parse(text)
	return err == nil && value.String() == text
}
func Hash(text string) bool {
	value, err := hex.DecodeString(text)
	return err == nil && len(value) == sha256.Size && strings.ToLower(text) == text
}

// Read verifies every byte, every path and the complete manifest closure. The
// caller owns a pinned read-only file for the duration (including configuration
// extraction). No entry name is ever used as a filesystem destination here.
func Read(ctx context.Context, file *os.File) (*Package, error) {
	info, err := file.Stat()
	if err != nil {
		return nil, err
	}
	if err = checkZIPBudget(ctx, file, info.Size()); err != nil {
		return nil, err
	}
	reader, err := zip.NewReader(file, info.Size())
	if err != nil {
		return nil, invalid("not-native-zip")
	}
	if len(reader.File) > 500000 {
		return nil, invalid("metadata-resource-limit")
	}
	entries := map[string]*zip.File{}
	names := map[string]string{}
	for _, f := range reader.File {
		if err = ctx.Err(); err != nil {
			return nil, err
		}
		key := strings.ToLower(strings.TrimSuffix(f.Name, "/"))
		if !utf8.ValidString(f.Name) || len(f.Name) > 32700 || strings.Count(f.Name, "/") > 100 || !ValidName(f.Name) || names[key] != "" || f.Flags&1 != 0 || f.Mode()&(os.ModeSymlink|os.ModeDevice|os.ModeNamedPipe|os.ModeSocket|os.ModeIrregular) != 0 || f.ExternalAttrs&0x400 != 0 || f.UncompressedSize64 >= math.MaxInt64 || f.Method != zip.Store && f.Method != zip.Deflate {
			return nil, invalid("unsafe-duplicate-or-unsupported-entry")
		}
		if strings.HasSuffix(f.Name, "/") != f.FileInfo().IsDir() || f.FileInfo().IsDir() && f.UncompressedSize64 != 0 {
			return nil, invalid("invalid-entry-type")
		}
		names[key], entries[f.Name] = f.Name, f
	}
	parents := map[string]string{}
	for key, original := range names {
		for parent := path.Dir(strings.TrimSuffix(original, "/")); parent != "."; parent = path.Dir(parent) {
			folded := strings.ToLower(parent)
			if prior := parents[folded]; prior != "" && prior != parent {
				return nil, invalid("case-alias-parent")
			}
			parents[folded] = parent
			if explicit := names[folded]; explicit != "" && strings.TrimSuffix(explicit, "/") != parent {
				return nil, invalid("case-alias-parent")
			}
		}
		for parent := path.Dir(key); parent != "."; parent = path.Dir(parent) {
			if original := names[parent]; original != "" && !entries[original].FileInfo().IsDir() {
				return nil, invalid("file-directory-collision")
			}
		}
	}
	manifestFile := entries["manifest.json"]
	if manifestFile == nil || manifestFile.FileInfo().IsDir() || manifestFile.UncompressedSize64 > maxManifestBytes {
		return nil, invalid("manifest-missing-or-too-large")
	}
	input, err := manifestFile.Open()
	if err != nil {
		return nil, invalid("manifest-unreadable")
	}
	data, err := io.ReadAll(io.LimitReader(contextReader{ctx, input}, maxManifestBytes+1))
	closeErr := input.Close()
	if err != nil || closeErr != nil || len(data) > maxManifestBytes {
		return nil, invalid("manifest-corrupt")
	}
	var header struct {
		Format          string `json:"format"`
		SchemaVersion   int    `json:"schemaVersion"`
		WorkspaceSchema int    `json:"workspaceSchema"`
	}
	if json.Unmarshal(data, &header) != nil || header.Format != Format {
		return nil, invalid("not-native-backup")
	}
	if header.SchemaVersion != Version || header.WorkspaceSchema != 7 {
		return nil, &Invalid{Reason: "unsupported-package-version", Unsupported: true}
	}
	var m Manifest
	if DecodeJSON(data, &m) != nil {
		return nil, invalid("manifest-unknown-or-invalid-field")
	}
	if !CanonicalID(m.BackupID) || m.AppVersion == "" || m.Scope != "all" && m.Scope != "selected" || m.Configuration != "configuration.sqlite" || m.Credentials != "windows-current-user-dpapi" || m.BrowserData != "sensitive-same-user-not-portable" || m.KernelBinariesIncluded || m.Environments == nil || m.Kernels == nil || m.Files == nil {
		return nil, invalid("manifest-contract")
	}
	if _, err = time.Parse(time.RFC3339Nano, m.CreatedAt); err != nil {
		return nil, invalid("manifest-time")
	}
	roots := map[string]Environment{}
	identities := map[string]bool{}
	for _, e := range m.Environments {
		if !CanonicalID(e.ID) || !CanonicalID(e.FingerprintID) || identities[e.ID] || identities[e.FingerprintID] || e.DataReference != "environments/"+e.ID+"/user-data" || e.Revision < 1 || e.ProfileRevision < 1 || !Hash(e.ProfileHash) || e.DataState != "present" && e.DataState != "never-initialized" {
			return nil, invalid("environment-identity")
		}
		identities[e.ID], identities[e.FingerprintID], roots[e.DataReference] = true, true, e
	}
	kernels := map[string]bool{}
	for _, k := range m.Kernels {
		if kernels[k.ID] {
			return nil, invalid("duplicate-kernel")
		}
		kernels[k.ID] = true
		if k.ID == "kernel-pending" {
			if k.State != "pending" || k.Version != "" || k.ArchiveSHA256 != "" || k.ExecutableSHA256 != "" || k.Architecture != "" {
				return nil, invalid("pending-kernel-evidence")
			}
		} else if !CanonicalID(k.ID) || k.State != "exact" || k.Version == "" || !Hash(k.ArchiveSHA256) || !Hash(k.ExecutableSHA256) || k.Architecture != "amd64" {
			return nil, invalid("kernel-evidence")
		}
	}
	seen := map[string]bool{}
	p := &Package{Manifest: m, files: entries, declared: map[string]File{}}
	for _, entry := range m.Files {
		f := entries[entry.Path]
		if seen[entry.Path] || entry.Path == "manifest.json" || f == nil || entry.Size < 0 || uint64(entry.Size) != f.UncompressedSize64 || entry.Directory != f.FileInfo().IsDir() || entry.Directory && (entry.Size != 0 || entry.SHA256 != "") || !entry.Directory && !Hash(entry.SHA256) {
			return nil, invalid("file-manifest-mismatch")
		}
		seen[entry.Path] = true
		p.declared[entry.Path] = entry
		if entry.Path == m.Configuration {
			if entry.Directory || entry.Size > MaxConfigurationBytes {
				return nil, invalid("configuration-resource-limit")
			}
		} else {
			parts := strings.SplitN(strings.TrimSuffix(entry.Path, "/"), "/", 4)
			if len(parts) < 3 {
				return nil, invalid("unexpected-payload")
			}
			root := strings.Join(parts[:3], "/")
			e, exists := roots[root]
			if !exists || len(parts) == 3 && !entry.Directory || len(parts) == 4 && (e.DataState == "never-initialized" || strings.EqualFold(parts[3], ".prism-runtime.lock")) {
				return nil, invalid("unexpected-profile-payload")
			}
		}
		if p.Bytes > math.MaxInt64-entry.Size {
			return nil, invalid("size-overflow")
		}
		p.Bytes += entry.Size
		if err = p.Copy(ctx, entry.Path, io.Discard); err != nil {
			return nil, err
		}
	}
	if len(seen)+1 != len(entries) || !seen[m.Configuration] {
		return nil, invalid("payload-closure-mismatch")
	}
	for root := range roots {
		if !seen[root+"/"] {
			return nil, invalid("profile-root-missing")
		}
	}
	if _, err = file.Seek(0, io.SeekStart); err != nil {
		return nil, err
	}
	h := sha256.New()
	if _, err = io.Copy(h, contextReader{ctx, file}); err != nil {
		return nil, err
	}
	p.ArchiveSHA256 = hex.EncodeToString(h.Sum(nil))
	sum := sha256.Sum256(data)
	p.ManifestSHA256 = hex.EncodeToString(sum[:])
	return p, nil
}

func (p *Package) Copy(ctx context.Context, name string, output io.Writer) error {
	f := p.files[name]
	if f == nil {
		return invalid("entry-missing")
	}
	expected, exists := p.declared[name]
	if !exists {
		return invalid("entry-not-declared")
	}
	input, err := f.Open()
	if err != nil {
		return invalid("entry-unreadable")
	}
	defer input.Close()
	h := sha256.New()
	if expected.Size == math.MaxInt64 {
		return invalid("size-overflow")
	}
	n, err := io.Copy(io.MultiWriter(output, h), io.LimitReader(contextReader{ctx, input}, expected.Size+1))
	if err != nil || n != expected.Size || !expected.Directory && hex.EncodeToString(h.Sum(nil)) != expected.SHA256 {
		return invalid("file-digest-or-size-mismatch")
	}
	return ctx.Err()
}

var _ error = (*Invalid)(nil)
