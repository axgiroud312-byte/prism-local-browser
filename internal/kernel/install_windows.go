//go:build windows

package kernel

import (
	"context"
	"crypto/sha256"
	"debug/pe"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/axgiroud312-byte/prism-local-browser/internal/desktopbase"
	"github.com/google/uuid"
)

var versionPattern = regexp.MustCompile(`^[1-9][0-9]{1,2}\.[0-9]+\.[0-9]+\.[0-9]+$`)
var digestPattern = regexp.MustCompile(`^[a-f0-9]{64}$`)

func ValidateInput(input InstallInput) error {
	if (input.Source != "official" && input.Source != "local") || !versionPattern.MatchString(input.Version) || !digestPattern.MatchString(input.ExpectedChecksum) || input.RequestID == "" || len(input.RequestID) > 200 {
		return problem("VALIDATION_FAILED", "invalid-install-request", "请选择来源并填写精确四段版本、64位小写SHA-256和请求标识。")
	}
	if input.Source == "local" && (!input.Trusted || input.ArchiveToken == "") {
		return problem("VALIDATION_FAILED", "untrusted-archive", "本地ZIP需要通过文件选择器选择，并明确确认可信；将执行其中的内核。")
	}
	return nil
}

func apiJSON(ctx context.Context, client *http.Client, url string, target any) error {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	request.Header.Set("User-Agent", "PrismBrowser-kernel-installer")
	request.Header.Set("Accept", "application/vnd.github+json")
	response, err := client.Do(request)
	if err != nil {
		return problem("KERNEL_MISSING", "asset-unavailable", "无法取得所选官方发行资产；检查网络后重试，不会改装其他版本。")
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return problem("KERNEL_MISSING", "asset-unavailable", "所选官方发行资产不可获取，未安装；不会自动换版本。")
	}
	return json.NewDecoder(io.LimitReader(response.Body, 2<<20)).Decode(target)
}
func officialSource(ctx context.Context, input InstallInput, client *http.Client) (Source, error) {
	var release struct {
		Tag    string `json:"tag_name"`
		Assets []struct {
			Name   string `json:"name"`
			URL    string `json:"browser_download_url"`
			Digest string `json:"digest"`
			Size   int64  `json:"size"`
		} `json:"assets"`
	}
	if err := apiJSON(ctx, client, "https://api.github.com/repos/adryfish/fingerprint-chromium/releases/tags/"+input.Version, &release); err != nil {
		return Source{}, err
	}
	if release.Tag != input.Version {
		return Source{}, problem("KERNEL_INTEGRITY_FAILED", "version-mismatch", "官方发行tag与选择版本不一致，未安装。")
	}
	source := Source{Kind: "official", Tag: input.Version}
	for _, asset := range release.Assets {
		if !strings.HasPrefix(asset.Name, "ungoogled-chromium_"+input.Version+"-") || !strings.HasSuffix(asset.Name, "_windows_x64.zip") {
			continue
		}
		if source.Location != "" || asset.Size < 1 || asset.Size > maxArchiveBytes || asset.URL != "https://github.com/adryfish/fingerprint-chromium/releases/download/"+input.Version+"/"+asset.Name {
			return Source{}, problem("KERNEL_INTEGRITY_FAILED", "invalid-asset-metadata", "官方资产信息不唯一或不符合Windows x64归档规则，未安装。")
		}
		if asset.Digest != "sha256:"+input.ExpectedChecksum {
			return Source{}, problem("KERNEL_INTEGRITY_FAILED", "hash-mismatch", "填写的归档摘要与官方发行摘要不符，未下载或执行。")
		}
		source.Location = asset.URL
	}
	if source.Location == "" {
		return Source{}, problem("KERNEL_MISSING", "asset-unavailable", "所选版本没有可获取的Windows x64 ZIP，不会自动换版本。")
	}
	var tag struct {
		Object struct {
			Type string `json:"type"`
			SHA  string `json:"sha"`
		} `json:"object"`
	}
	if apiJSON(ctx, client, "https://api.github.com/repos/adryfish/fingerprint-chromium/git/ref/tags/"+input.Version, &tag) == nil && tag.Object.Type == "commit" && regexp.MustCompile(`^[a-f0-9]{40}$`).MatchString(tag.Object.SHA) {
		source.Commit = &tag.Object.SHA
	}
	return source, nil
}

// Prepared contains only a new internally allocated directory, never an old
// installed ID. The workspace transaction owns publication and rollback.
type Prepared struct {
	Record    Record
	Directory string
	Staging   string
}

func Prepare(ctx context.Context, root string, input InstallInput, localPath string, probe ProbeFunc, progress ProgressFunc) (_ *Prepared, resultErr error) {
	if err := ValidateInput(input); err != nil {
		return nil, err
	}
	if progress == nil {
		progress = func(string) {}
	}
	if probe == nil {
		probe = Probe
	}
	if err := desktopbase.ValidateTree(filepath.Join(root, "staging")); err != nil {
		return nil, problem("PATH_OUTSIDE_ROOT", "reparse-point", "工作区暂存目录含链接，未安装。")
	}
	releaseParent, err := EnsureDirectory(root, "staging")
	if err != nil {
		return nil, err
	}
	defer releaseParent()
	resourceKey := input.ResourceKey
	if resourceKey == "" {
		resourceKey = uuid.NewString()
	}
	if parsed, err := uuid.Parse(resourceKey); err != nil || parsed.String() != resourceKey {
		return nil, fmt.Errorf("invalid internal staging token")
	}
	staging := filepath.Join(root, "staging", "kernel-"+resourceKey)
	if err = os.Mkdir(staging, 0700); err != nil {
		return nil, err
	}
	defer func() {
		if resultErr != nil {
			if err := RemoveOwnedTree(staging); err != nil {
				resultErr = problem("STORAGE_WRITE_FAILED", "cleanup-failed", "安装失败且本次暂存资源无法清理；旧内核未修改。请关闭占用后重试。")
			}
		}
	}()
	releaseStaging, err := desktopbase.PinDirectories(staging)
	if err != nil {
		return nil, problem("PATH_OUTSIDE_ROOT", "reparse-point", "本次暂存目录无法安全固定，未写入归档。")
	}
	defer releaseStaging()
	archive := filepath.Join(staging, "archive.zip")
	client := &http.Client{Timeout: 10 * time.Minute}
	source := Source{Kind: "local", Location: filepath.Base(localPath), Tag: input.Version}
	progress("acquiring-archive")
	var reader io.ReadCloser
	if input.Source == "official" {
		source, err = officialSource(ctx, input, client)
		if err != nil {
			return nil, err
		}
		request, _ := http.NewRequestWithContext(ctx, http.MethodGet, source.Location, nil)
		response, downloadErr := client.Do(request)
		if downloadErr != nil {
			return nil, problem("KERNEL_MISSING", "asset-unavailable", "所选归档下载失败，未安装；不会自动换版本。")
		}
		if response.StatusCode != http.StatusOK {
			response.Body.Close()
			return nil, problem("KERNEL_MISSING", "asset-unavailable", "所选归档不可获取，未安装；不会自动换版本。")
		}
		reader = response.Body
	} else {
		if localPath == "" {
			return nil, problem("VALIDATION_FAILED", "archive-selection-expired", "本地归档选择已失效，请重新选择ZIP。")
		}
		reader, err = os.Open(localPath)
		if err != nil {
			return nil, problem("KERNEL_MISSING", "archive-unavailable", "选择的本地ZIP无法读取，请重新选择。")
		}
	}
	err = func() error {
		defer reader.Close()
		output, err := os.OpenFile(archive, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
		if err != nil {
			return err
		}
		defer output.Close()
		digest := sha256.New()
		count, err := io.Copy(io.MultiWriter(output, digest), contextReader{ctx, io.LimitReader(reader, maxArchiveBytes+1)})
		if err != nil {
			return err
		}
		if count > maxArchiveBytes {
			return problem("KERNEL_INTEGRITY_FAILED", "archive-limit", "归档超过安全大小边界，未安装。")
		}
		if hex.EncodeToString(digest.Sum(nil)) != input.ExpectedChecksum {
			return problem("KERNEL_INTEGRITY_FAILED", "hash-mismatch", "实际ZIP的SHA-256与预期不符，未解包或执行。")
		}
		return output.Sync()
	}()
	if err != nil {
		return nil, err
	}
	releaseArchive, err := PinFiles(staging, map[string]string{"archive.zip": input.ExpectedChecksum})
	if err != nil {
		return nil, err
	}
	defer releaseArchive()
	actualDigest, err := fileHash(archive)
	if err != nil || actualDigest != input.ExpectedChecksum {
		return nil, problem("KERNEL_INTEGRITY_FAILED", "hash-mismatch", "已复制的归档被改动，未解包或执行。")
	}
	progress("extracting")
	payload := filepath.Join(staging, "payload")
	if err = os.Mkdir(payload, 0700); err != nil {
		return nil, err
	}
	files, exe, err := extractArchive(ctx, archive, payload)
	if err != nil {
		return nil, err
	}
	progress("verifying-extracted-files")
	releaseFiles, err := PinFiles(payload, files)
	if err != nil {
		return nil, err
	}
	defer releaseFiles()
	if err = VerifyFiles(payload, files); err != nil {
		return nil, err
	}
	executable := filepath.Join(payload, filepath.FromSlash(exe))
	image, err := pe.Open(executable)
	if err != nil {
		return nil, problem("KERNEL_INTEGRITY_FAILED", "architecture-mismatch", "chrome.exe不是有效Windows程序，未执行。")
	}
	machine := image.Machine
	image.Close()
	if machine != pe.IMAGE_FILE_MACHINE_AMD64 {
		return nil, problem("KERNEL_INTEGRITY_FAILED", "architecture-mismatch", "内核不是Windows amd64构建，未执行。")
	}
	actualVersion, err := FileVersion(executable)
	if err != nil || !versionPattern.MatchString(actualVersion) {
		return nil, problem("KERNEL_INTEGRITY_FAILED", "version-unknown", "无法从程序文件读取实际四段版本，未执行。")
	}
	if actualVersion != input.Version {
		return nil, problem("KERNEL_INTEGRITY_FAILED", "version-mismatch", "实际程序版本与所选tag不符，未执行。")
	}
	progress("probing")
	report, err := probe(ctx, executable, input.Version, staging)
	if err != nil {
		return nil, err
	}
	if err = VerifyFiles(payload, files); err != nil {
		return nil, err
	}
	kernelID := uuid.NewString()
	record := Record{ID: kernelID, Version: actualVersion, Architecture: "amd64", Source: source, ArchiveSHA256: input.ExpectedChecksum, ExecutableSHA256: files[exe], ExecutableRelativePath: exe, InstallPath: filepath.ToSlash(filepath.Join("kernels", kernelID)), Files: files, Report: report, InstalledAt: time.Now().UTC().Format(time.RFC3339Nano)}
	if err = CheckRecord(record); err != nil {
		return nil, problem("KERNEL_INTEGRITY_FAILED", "invalid-probe-evidence", "内核能力探测证据不完整，未发布。")
	}
	return &Prepared{Record: record, Directory: payload, Staging: staging}, nil
}

func RecordDirectory(root string, record Record) (string, error) {
	parsed, err := uuid.Parse(record.ID)
	if err != nil || parsed.String() != record.ID || record.InstallPath != "kernels/"+record.ID {
		return "", fmt.Errorf("invalid internal kernel ID/path")
	}
	return filepath.Join(root, "kernels", record.ID), nil
}

func Verify(ctx context.Context, root string, record Record, staging string) (Report, error) {
	if err := CheckRecord(record); err != nil {
		return Report{}, problem("KERNEL_INTEGRITY_FAILED", "invalid-evidence", "已保存的内核证据不完整，未执行程序。")
	}
	directory, err := RecordDirectory(root, record)
	if err != nil {
		return Report{}, err
	}
	release, err := PinFiles(directory, record.Files)
	if err != nil {
		if os.IsNotExist(err) {
			return Report{}, problem("KERNEL_INTEGRITY_FAILED", "hash-mismatch", "内核文件缺失，未执行程序。")
		}
		var boundary *Problem
		if errors.As(err, &boundary) {
			return Report{}, err
		}
		return Report{}, problem("STORAGE_READ_FAILED", "verification-incomplete", "无法安全锁定内核文件，复验未完成；既有证据未改变。请关闭文件占用后重试。")
	}
	defer release()
	if err = VerifyFiles(directory, record.Files); err != nil {
		return Report{}, err
	}
	executable := filepath.Join(directory, filepath.FromSlash(record.ExecutableRelativePath))
	actualVersion, err := FileVersion(executable)
	if err != nil || actualVersion != record.Version {
		return Report{}, problem("KERNEL_INTEGRITY_FAILED", "version-mismatch", "内核实际程序版本已变化，未执行程序。")
	}
	report, err := Probe(ctx, executable, record.Version, staging)
	if err != nil {
		return Report{}, err
	}
	if err = VerifyFiles(directory, record.Files); err != nil {
		return Report{}, err
	}
	return report, nil
}
