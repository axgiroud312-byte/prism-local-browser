//go:build windows

package kernel

import (
	"strconv"
	"strings"
	"time"
	_ "time/tzdata"
)

// FingerprintInput is a small allowlist, not a command-line escape hatch.
// Website language is independent of the still-unverified browser menu language.
type FingerprintInput struct {
	Seed     string
	Language string
	Timezone string
	CPU      string
}

var languageLists = map[string][]string{
	"en-US": {"en-US", "en"}, "en-GB": {"en-GB", "en"},
	"de-DE": {"de-DE", "de", "en"}, "ja-JP": {"ja-JP", "ja", "en"},
	"en-SG": {"en-SG", "en"}, "zh-CN": {"zh-CN", "zh", "en"},
}

func AcceptLanguages(language string) []string {
	return append([]string{}, languageLists[language]...)
}

// Go treats "" as UTC and "Local" as the host zone. Neither is an explicit,
// frozen IANA choice, even though LoadLocation accepts both special values.
func ValidTimezone(value string) bool {
	if value == "" || value == "Local" {
		return false
	}
	_, err := time.LoadLocation(value)
	return err == nil
}

func hasCapability(record Record, field, status string) bool {
	for _, capability := range record.Report.Capabilities {
		if capability.Field == field && capability.Status == status && capability.Source == "observed" {
			return true
		}
	}
	return false
}

// CompileFingerprint uses only the selected immutable build's verified entries.
// Window size, GPU, memory, fonts, menu language and compatibility toggles are
// deliberately absent. Normal runtime and networking arguments belong elsewhere.
func CompileFingerprint(record Record, input FingerprintInput) ([]string, error) {
	if err := CheckRecord(record); err != nil {
		return nil, problem("KERNEL_INTEGRITY_FAILED", "invalid-evidence", "精确内核证据无效，未生成设备档案。")
	}
	seed, err := strconv.ParseInt(input.Seed, 10, 64)
	if err != nil || seed < 1 || seed > 2147483647 || strconv.FormatInt(seed, 10) != input.Seed || len(AcceptLanguages(input.Language)) == 0 {
		return nil, problem("VALIDATION_FAILED", "invalid-fingerprint-input", "种子或网站语言不在支持的Windows模板内。")
	}
	if !ValidTimezone(input.Timezone) {
		return nil, problem("VALIDATION_FAILED", "invalid-timezone", "请选择有效的IANA时区。")
	}
	if !map[string]bool{"auto": true, "4": true, "8": true, "12": true, "16": true}[input.CPU] {
		return nil, problem("VALIDATION_FAILED", "invalid-cpu", "请选择支持的CPU偏好。")
	}
	for field, status := range map[string]string{"identity": "configurable", "seed": "seed-generated", "acceptLanguages": "configurable", "timezone": "configurable"} {
		if !hasCapability(record, field, status) {
			return nil, problem("CAPABILITY_UNSUPPORTED", "not-probed", "该精确构建尚未核验必要身份、语言或时区入口，不下发未经验证的参数。")
		}
	}
	args := []string{"--fingerprint=" + input.Seed, "--fingerprint-platform=windows", "--fingerprint-platform-version=15.0.0", "--fingerprint-brand=Chrome", "--fingerprint-brand-version=" + record.Version, "--accept-lang=" + strings.Join(AcceptLanguages(input.Language), ","), "--timezone=" + input.Timezone}
	if input.CPU != "auto" {
		if !hasCapability(record, "cpu", "configurable") {
			return nil, problem("CAPABILITY_UNSUPPORTED", "field-unsupported", "该精确构建尚未核验显式CPU入口，请保留自动生成。")
		}
		args = append(args, "--fingerprint-hardware-concurrency="+input.CPU)
	}
	return args, nil
}
