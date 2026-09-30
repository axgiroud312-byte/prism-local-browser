//go:build windows

package kernel

import (
	"archive/zip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/axgiroud312-byte/prism-local-browser/internal/desktopbase"
)

const maxArchiveBytes = int64(512 << 20)
const maxExtractedBytes = uint64(2 << 30)
const maxEntryBytes = uint64(1 << 30)

func validArchiveName(name string) error {
	if name == "" || strings.HasPrefix(name, "/") || strings.ContainsAny(name, `\:<>"|?*`) {
		return errors.New("unsafe archive path")
	}
	for _, part := range strings.Split(strings.TrimSuffix(name, "/"), "/") {
		if part == "" || part == "." || part == ".." || strings.TrimRight(part, ". ") != part {
			return errors.New("unsafe Windows path")
		}
		for _, c := range part {
			if c < 32 {
				return errors.New("control character in path")
			}
		}
		stem := strings.ToUpper(strings.SplitN(part, ".", 2)[0])
		if stem == "CON" || stem == "PRN" || stem == "AUX" || stem == "NUL" || stem == "CONIN$" || stem == "CONOUT$" || stem == "CLOCK$" || (len(stem) > 3 && (strings.HasPrefix(stem, "COM") || strings.HasPrefix(stem, "LPT")) && strings.Contains("123456789¹²³", stem[3:])) {
			return errors.New("reserved Windows name")
		}
	}
	return nil
}

type contextReader struct {
	ctx    context.Context
	reader io.Reader
}

func (r contextReader) Read(bytes []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	return r.reader.Read(bytes)
}
func fileHash(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()
	digest := sha256.New()
	if _, err = io.Copy(digest, file); err != nil {
		return "", err
	}
	return hex.EncodeToString(digest.Sum(nil)), nil
}

func extractArchive(ctx context.Context, archive, root string) (map[string]string, string, error) {
	reader, err := zip.OpenReader(archive)
	if err != nil {
		return nil, "", problem("KERNEL_INTEGRITY_FAILED", "invalid-archive", "归档不是完整有效的ZIP，未安装。")
	}
	defer reader.Close()
	if len(reader.File) == 0 || len(reader.File) > 10000 {
		return nil, "", problem("KERNEL_INTEGRITY_FAILED", "archive-limit", "归档条目数异常，未解包。")
	}
	paths := map[string]bool{}
	var total uint64
	executable := ""
	for _, entry := range reader.File {
		if validArchiveName(entry.Name) != nil || entry.Mode()&(os.ModeType&^os.ModeDir) != 0 || entry.ExternalAttrs&0x400 != 0 {
			return nil, "", problem("PATH_OUTSIDE_ROOT", "unsafe-archive-entry", "归档包含越界、链接或不安全的Windows路径，未解包。")
		}
		name := strings.ToLower(strings.TrimSuffix(entry.Name, "/"))
		if _, exists := paths[name]; exists {
			return nil, "", problem("PATH_OUTSIDE_ROOT", "duplicate-entry", "归档包含大小写重复条目，未解包。")
		}
		paths[name] = entry.FileInfo().IsDir()
		if entry.UncompressedSize64 > maxEntryBytes || entry.UncompressedSize64 > maxExtractedBytes-total {
			return nil, "", problem("KERNEL_INTEGRITY_FAILED", "archive-limit", "归档解包大小超过安全边界，未解包。")
		}
		total += entry.UncompressedSize64
		if !entry.FileInfo().IsDir() && strings.EqualFold(filepath.Base(entry.Name), "chrome.exe") {
			if executable != "" {
				return nil, "", problem("KERNEL_INTEGRITY_FAILED", "ambiguous-executable", "归档有多个chrome.exe，不能确定内核。")
			}
			executable = entry.Name
		}
	}
	for name := range paths {
		parent := name
		for strings.Contains(parent, "/") {
			parent = parent[:strings.LastIndex(parent, "/")]
			if directory, exists := paths[parent]; exists && !directory {
				return nil, "", problem("PATH_OUTSIDE_ROOT", "entry-collision", "归档文件与目录冲突，未解包。")
			}
		}
	}
	if executable == "" {
		return nil, "", problem("KERNEL_INTEGRITY_FAILED", "executable-missing", "归档缺少唯一chrome.exe，未安装。")
	}
	if err = desktopbase.ValidateTree(root); err != nil {
		return nil, "", problem("PATH_OUTSIDE_ROOT", "reparse-point", "暂存目录包含链接，未解包。")
	}
	release, err := desktopbase.PinDirectories(root)
	if err != nil {
		return nil, "", err
	}
	defer release()
	files := map[string]string{}
	for _, entry := range reader.File {
		if err = ctx.Err(); err != nil {
			return nil, "", err
		}
		path := filepath.Join(root, filepath.FromSlash(entry.Name))
		parent := filepath.Dir(path)
		if entry.FileInfo().IsDir() {
			parent = path
		}
		relativeParent, err := filepath.Rel(root, parent)
		if err != nil {
			return nil, "", err
		}
		if relativeParent == "." {
			relativeParent = ""
		}
		releaseParent, err := EnsureDirectory(root, relativeParent)
		if err != nil {
			return nil, "", err
		}
		if entry.FileInfo().IsDir() {
			releaseParent()
			continue
		}
		err = func() error {
			defer releaseParent()
			input, err := entry.Open()
			if err != nil {
				return err
			}
			defer input.Close()
			output, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
			if err != nil {
				return err
			}
			defer output.Close()
			digest := sha256.New()
			count, err := io.Copy(io.MultiWriter(output, digest), contextReader{ctx, io.LimitReader(input, int64(entry.UncompressedSize64)+1)})
			if err != nil || uint64(count) != entry.UncompressedSize64 {
				return errors.New("archive size or checksum mismatch")
			}
			if err = output.Sync(); err != nil {
				return err
			}
			files[entry.Name] = hex.EncodeToString(digest.Sum(nil))
			return nil
		}()
		if err != nil {
			return nil, "", err
		}
	}
	return files, executable, nil
}

// VerifyFiles never replaces a trusted digest with the digest of changed bytes.
func VerifyFiles(root string, files map[string]string) error {
	if len(files) == 0 {
		return problem("KERNEL_INTEGRITY_FAILED", "missing-evidence", "内核没有完整文件清单，不能使用。")
	}
	if err := desktopbase.ValidateTree(root); err != nil {
		return problem("PATH_OUTSIDE_ROOT", "reparse-point", "内核目录含链接，不能使用。")
	}
	seen := 0
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		expected, known := files[filepath.ToSlash(relative)]
		if !known || validArchiveName(filepath.ToSlash(relative)) != nil {
			return errors.New("unexpected file")
		}
		actual, err := fileHash(path)
		if err != nil || actual != expected {
			return errors.New("file hash mismatch")
		}
		seen++
		return nil
	})
	if err != nil || seen != len(files) {
		return problem("KERNEL_INTEGRITY_FAILED", "hash-mismatch", "内核文件缺失或校验不符，已阻止使用；不会重算并覆盖原可信摘要。")
	}
	return nil
}

// RemoveOwnedTree is used only for internally allocated staging/kernel IDs.
// Revalidate and pin the actual parent at every deletion, never follow links.
func RemoveOwnedTree(path string) error {
	if _, err := os.Lstat(path); os.IsNotExist(err) {
		return nil
	}
	if err := desktopbase.ValidateTree(path); err != nil {
		return err
	}
	paths := []string{}
	if err := filepath.WalkDir(path, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		paths = append(paths, path)
		return nil
	}); err != nil {
		return err
	}
	for i := len(paths) - 1; i >= 0; i-- {
		release, err := desktopbase.PinDirectories(filepath.Dir(paths[i]))
		if err != nil {
			return err
		}
		err = desktopbase.ValidatePath(paths[i])
		if err == nil {
			err = os.Remove(paths[i])
		}
		release()
		if err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	return nil
}
