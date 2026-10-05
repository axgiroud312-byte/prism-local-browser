//go:build windows

package desktopbase

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/google/uuid"
	"golang.org/x/sys/windows"
)

var releasePattern = regexp.MustCompile(`^0\.3\.0-preview\.[1-9][0-9]{0,4}$`)
var distributedNames = []string{"prism-browser.exe", "prism-maintenance.exe", "LICENSE", "THIRD_PARTY_NOTICES.md", "GO-THIRD-PARTY-NOTICES.txt", "FRONTEND-THIRD-PARTY-NOTICES.txt", "NSIS-LICENSE.txt", "INSTALLATION.md", "USER_GUIDE.md"}

type releaseReceipt struct {
	Version string `json:"version"`
	Files   []struct {
		Name   string `json:"name"`
		SHA256 string `json:"sha256"`
	} `json:"files"`
}

func verifyPayload(source string) (releaseReceipt, error) {
	var receipt releaseReceipt
	data, err := os.ReadFile(filepath.Join(source, "release.json"))
	if err != nil {
		return receipt, err
	}
	if err = json.Unmarshal(data, &receipt); err != nil || !releasePattern.MatchString(receipt.Version) {
		return receipt, errors.New("版本清单无效")
	}
	known := map[string]bool{}
	for _, name := range distributedNames {
		known[name] = true
	}
	for _, file := range receipt.Files {
		if !known[file.Name] || len(file.SHA256) != 64 {
			return receipt, errors.New("文件清单无效")
		}
		bytes, err := os.ReadFile(filepath.Join(source, file.Name))
		if err != nil {
			return receipt, err
		}
		digest := sha256.Sum256(bytes)
		if hex.EncodeToString(digest[:]) != file.SHA256 {
			return receipt, errors.New("安装文件摘要不一致")
		}
		delete(known, file.Name)
	}
	if len(known) != 0 {
		return receipt, errors.New("安装文件清单缺失")
	}
	return receipt, nil
}

// PublishProgram copies only the helper's sibling payload into a fixed known folder.
// Directory handles prevent a wizard-time junction replacement from redirecting writes.
func PublishProgram(source string) error {
	root, err := InstallationRoot()
	if err != nil {
		return err
	}
	shortcuts, err := shortcutPaths()
	if err != nil {
		return err
	}
	receipt, err := verifyPayload(source)
	if err != nil {
		return err
	}
	return publishIntegration(source, root, receipt.Version, shortcuts, productKey, func() error { return publishProgram(source, root) })
}
func publishProgram(source, root string) error {
	receipt, err := verifyPayload(source)
	if err != nil {
		return err
	}
	if err = ValidateTree(root); err != nil {
		return err
	}
	// Pin each existing parent before creating its next child.
	missing := []string{}
	parent := root
	for {
		if _, err = os.Stat(parent); err == nil {
			break
		}
		if !os.IsNotExist(err) {
			return err
		}
		missing = append(missing, parent)
		parent = filepath.Dir(parent)
	}
	guard, err := PinDirectories(parent)
	if err != nil {
		return err
	}
	defer guard()
	for i := len(missing) - 1; i >= 0; i-- {
		if err = os.Mkdir(missing[i], 0700); err != nil && !os.IsExist(err) {
			return err
		}
		pin, err := PinDirectories(missing[i])
		if err != nil {
			return err
		}
		defer pin()
	}
	guardRoot, err := PinDirectories(root)
	if err != nil {
		return err
	}
	defer guardRoot()
	versions := filepath.Join(root, "versions")
	if err = os.Mkdir(versions, 0700); err != nil && !os.IsExist(err) {
		return err
	}
	guardVersions, err := PinDirectories(versions)
	if err != nil {
		return err
	}
	defer guardVersions()
	if err = ValidateTree(versions); err != nil {
		return err
	}
	suffix := uuid.NewString()
	staged := filepath.Join(versions, ".pending-"+receipt.Version+"-"+suffix)
	if err = os.Mkdir(staged, 0700); err != nil {
		return err
	}
	pinStaged, err := PinDirectories(staged)
	if err != nil {
		return err
	}
	for _, name := range append(append([]string{}, distributedNames...), "release.json") {
		bytes, err := os.ReadFile(filepath.Join(source, name))
		if err != nil {
			pinStaged()
			return err
		}
		file, err := os.OpenFile(filepath.Join(staged, name), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
		if err != nil {
			pinStaged()
			return err
		}
		_, writeErr := file.Write(bytes)
		closeErr := file.Close()
		if err = errors.Join(writeErr, closeErr); err != nil {
			pinStaged()
			return err
		}
	}
	if _, err = verifyPayload(staged); err != nil {
		pinStaged()
		return err
	}
	pinStaged()
	destination := filepath.Join(versions, receipt.Version)
	previous := filepath.Join(versions, ".previous-"+receipt.Version+"-"+suffix)
	hadPrevious := false
	if _, err = os.Lstat(destination); err == nil {
		if err = ValidateTree(destination); err != nil {
			return err
		}
		if err = os.Rename(destination, previous); err != nil {
			return err
		}
		hadPrevious = true
	} else if !os.IsNotExist(err) {
		return err
	}
	if err = os.Rename(staged, destination); err != nil {
		if hadPrevious {
			if restoreErr := os.Rename(previous, destination); restoreErr != nil {
				return fmt.Errorf("publish and rollback failed: %w", restoreErr)
			}
		}
		return err
	}
	// The uninstaller is also staged in the trusted payload, never written via an unpinned NSIS path.
	uninstaller, err := os.ReadFile(filepath.Join(source, "uninstall.exe"))
	if err != nil {
		return err
	}
	return replaceFile(filepath.Join(root, "uninstall.exe"), uninstaller)
}

func shortcutPaths() ([]string, error) {
	desktop, err := windows.KnownFolderPath(windows.FOLDERID_Desktop, 0)
	if err != nil {
		return nil, err
	}
	programs, err := windows.KnownFolderPath(windows.FOLDERID_Programs, 0)
	if err != nil {
		return nil, err
	}
	return []string{filepath.Join(desktop, shortcutName), filepath.Join(programs, shortcutName)}, nil
}

func CleanProgram() error {
	root, err := InstallationRoot()
	if err != nil {
		return err
	}
	shortcuts, err := shortcutPaths()
	if err != nil {
		return err
	}
	return cleanIntegration(root, shortcuts, productKey, func() error { return cleanProgram(root) })
}
func cleanProgram(root string) error {
	if err := ValidateTree(root); err != nil {
		return err
	}
	guardRoot, err := PinDirectories(root)
	if err != nil {
		return err
	}
	defer guardRoot()
	versions := filepath.Join(root, "versions")
	entries, err := os.ReadDir(versions)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		if !releasePattern.MatchString(entry.Name()) && !strings.HasPrefix(entry.Name(), ".pending-0.3.0-preview.") && !strings.HasPrefix(entry.Name(), ".previous-0.3.0-preview.") {
			continue
		}
		directory := filepath.Join(versions, entry.Name())
		guard, err := PinDirectories(directory)
		if err != nil {
			return err
		}
		for _, name := range append(append([]string{}, distributedNames...), "release.json") {
			path := filepath.Join(directory, name)
			if err = ValidatePath(path); err != nil {
				guard()
				return err
			}
			if err = os.Remove(path); err != nil && !os.IsNotExist(err) {
				guard()
				return err
			}
		}
		guard()
		// A directory with unknown files is intentionally retained; never recurse into it.
		if err = os.Remove(directory); err != nil && !os.IsNotExist(err) && !errors.Is(err, windows.ERROR_DIR_NOT_EMPTY) {
			return err
		}
	}
	if err = os.Remove(versions); err != nil && !os.IsNotExist(err) && !errors.Is(err, windows.ERROR_DIR_NOT_EMPTY) {
		return err
	}
	// Preserve the uninstaller until entries and the selected data policy succeed.
	return nil
}

func FinishUninstall() error {
	root, err := InstallationRoot()
	if err != nil {
		return err
	}
	return finishIntegration(root, productKey)
}
