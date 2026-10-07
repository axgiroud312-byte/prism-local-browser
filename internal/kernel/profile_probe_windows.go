//go:build windows

package kernel

import (
	"context"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/axgiroud312-byte/prism-local-browser/internal/desktopbase"
)

// ProbeFingerprint is a host/test-only diagnostic of already frozen inputs.
// It never opens an environment's data directory and is not exposed through RPC.
func ProbeFingerprint(ctx context.Context, root string, record Record, input FingerprintInput, staging string) (Observation, error) {
	args, err := CompileFingerprint(record, input)
	if err != nil {
		return Observation{}, err
	}
	directory, err := RecordDirectory(root, record)
	if err != nil {
		return Observation{}, err
	}
	release, err := PinFiles(directory, record.Files)
	if err != nil {
		return Observation{}, err
	}
	defer release()
	if err = VerifyFiles(directory, record.Files); err != nil {
		return Observation{}, err
	}
	executable := filepath.Join(directory, filepath.FromSlash(record.ExecutableRelativePath))
	actual, err := FileVersion(executable)
	if err != nil || actual != record.Version {
		return Observation{}, problem("KERNEL_INTEGRITY_FAILED", "version-mismatch", "保存档案的精确内核实际版本不匹配，未运行诊断。")
	}
	releaseStage, err := desktopbase.PinDirectories(staging)
	if err != nil {
		return Observation{}, err
	}
	defer releaseStage()
	ctx, cancel := context.WithTimeout(ctx, 45*time.Second)
	defer cancel()
	url, closePage, err := newProbePage()
	if err != nil {
		return Observation{}, err
	}
	defer closePage()
	seed, _ := strconv.Atoi(input.Seed)
	observation, err := probeOneArguments(ctx, executable, staging, url, seed, args)
	if err != nil {
		return Observation{}, err
	}
	if err = checkIdentity(observation, record.Version); err != nil {
		return Observation{}, err
	}
	if observation.Language != input.Language || !strings.HasPrefix(observation.AcceptLanguage, input.Language) || observation.Timezone != input.Timezone || (input.CPU != "auto" && strconv.Itoa(observation.CPU) != input.CPU) {
		return Observation{}, problem("KERNEL_INTEGRITY_FAILED", "parameter-mismatch", "保存档案的语言、时区或CPU回读不一致。")
	}
	if err = VerifyFiles(directory, record.Files); err != nil {
		return Observation{}, err
	}
	return observation, nil
}
