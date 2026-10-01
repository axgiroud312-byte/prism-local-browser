package workspace

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"sync"
	"time"

	"github.com/axgiroud312-byte/prism-local-browser/internal/backup"
)

type DiagnosticPreview struct {
	ReportID  string           `json:"reportId"`
	SHA256    string           `json:"sha256"`
	Bytes     int              `json:"bytes"`
	ExpiresAt string           `json:"expiresAt"`
	Report    DiagnosticReport `json:"report"`
}
type DiagnosticExportRequest struct {
	ReportID  string `json:"reportId"`
	RequestID string `json:"requestId"`
}
type diagnosticExport struct {
	input       DiagnosticExportRequest
	result      Result
	destination string
	digest      string
	published   bool
}

// Host lives independently of Service so a failed database open still permits
// a minimal report. Chooser and root are host-only; RPC never accepts a path.
type DiagnosticHost struct {
	mu            sync.Mutex
	root, version string
	service       *Service
	startup       *Error
	choose        func() (string, error)
	busy          bool
	preview       *DiagnosticPreview
	bytes         []byte
	expires       time.Time
	exports       map[string]*diagnosticExport
}

func NewDiagnosticHost(root, version string, service *Service, startup *Error, choose func() (string, error)) *DiagnosticHost {
	return &DiagnosticHost{root: root, version: version, service: service, startup: startup, choose: choose, exports: map[string]*diagnosticExport{}}
}
func (h *DiagnosticHost) Call(request Request) Result {
	if request.Mode != "native" {
		return failure("CAPABILITY_UNSUPPORTED", "诊断只接受本机请求。", false)
	}
	switch request.Method {
	case "Diagnostics.Preview":
		if decode(request.Payload, &struct{}{}) != nil {
			return failure("VALIDATION_FAILED", "诊断预览不接受路径或原始数据。", false)
		}
		return h.read()
	case "Diagnostics.Export", "Diagnostics.EndVerification":
		var input DiagnosticExportRequest
		if decode(request.Payload, &input) != nil || !backup.CanonicalID(input.ReportID) || !backup.CanonicalID(input.RequestID) {
			return failure("VALIDATION_FAILED", "请选择原诊断预览并提供请求标识。", false)
		}
		if request.Method == "Diagnostics.EndVerification" {
			return h.endVerification(input)
		}
		return h.save(input)
	}
	return failure("CAPABILITY_UNSUPPORTED", "未知诊断操作。", false)
}
func (h *DiagnosticHost) read() Result {
	h.mu.Lock()
	if h.busy {
		h.mu.Unlock()
		return failure("DIAGNOSTICS_RESULT_UNCONFIRMED", "原诊断操作正在处理，请稍后重试。", true)
	}
	h.busy = true
	h.mu.Unlock()
	defer func() { h.mu.Lock(); h.busy = false; h.mu.Unlock() }()
	ctx, cancel := diagnosticContext()
	defer cancel()
	report := buildDiagnosticReport(ctx, h.version, h.service, h.startup)
	encoded, err := json.MarshalIndent(report, "", "  ")
	if err != nil || len(encoded) > 1024*1024 {
		return failure("STORAGE_READ_FAILED", "无法生成有界脱敏诊断。", true)
	}
	encoded = append(encoded, '\n')
	digest := sha256.Sum256(encoded)
	expires := time.Now().Add(15 * time.Minute)
	p := DiagnosticPreview{ReportID: id(), SHA256: hex.EncodeToString(digest[:]), Bytes: len(encoded), ExpiresAt: expires.UTC().Format(time.RFC3339Nano), Report: report}
	h.mu.Lock()
	h.preview = &p
	h.bytes = encoded
	h.expires = expires
	h.mu.Unlock()
	return success(p, "")
}
func (h *DiagnosticHost) save(input DiagnosticExportRequest) Result {
	h.mu.Lock()
	if h.busy {
		h.mu.Unlock()
		return failure("DIAGNOSTICS_RESULT_UNCONFIRMED", "原诊断操作仍在进行，请核实原请求。", true)
	}
	if x := h.exports[input.RequestID]; x != nil {
		if x.input != input {
			h.mu.Unlock()
			return failure("REQUEST_ID_REUSED", "原导出请求不能更换报告。", false)
		}
		if x.result.OK || !x.published {
			r := x.result
			h.mu.Unlock()
			return r
		}
		h.busy = true
		h.mu.Unlock()
		ctx, cancel := diagnosticContext()
		digest, err := backup.PublishedDigest(ctx, x.destination)
		cancel()
		h.mu.Lock()
		defer h.mu.Unlock()
		h.busy = false
		if err == nil && digest == x.digest {
			x.result = success(map[string]any{"status": "saved", "reportId": input.ReportID, "sha256": x.digest}, "")
		}
		return x.result
	}
	if h.preview == nil || h.preview.ReportID != input.ReportID || time.Now().After(h.expires) {
		h.mu.Unlock()
		return failure("PREVIEW_EXPIRED", "请重新生成诊断预览。", true)
	}
	if h.choose == nil {
		h.mu.Unlock()
		return failure("CAPABILITY_UNSUPPORTED", "桌面保存对话框不可用。", false)
	}
	encoded := append([]byte(nil), h.bytes...)
	x := &diagnosticExport{input: input, digest: h.preview.SHA256}
	h.exports[input.RequestID] = x
	h.busy = true
	h.mu.Unlock()
	result := failure("STORAGE_WRITE_FAILED", "诊断导出失败，请选择工作区外的新JSON文件；未覆盖已有文件。", true)
	defer func() { h.mu.Lock(); x.result = result; h.busy = false; h.mu.Unlock() }()
	destination, err := h.choose()
	if err != nil {
		return result
	}
	if destination == "" {
		result = success(map[string]any{"status": "cancelled", "reportId": input.ReportID}, "")
		return result
	}
	ctx, cancel := diagnosticContext()
	defer cancel()
	output, err := backup.NewDiagnosticOutput(h.root, destination, input.RequestID)
	if err != nil {
		return result
	}
	closed := false
	defer func() {
		if !closed {
			_ = output.Close()
		}
	}()
	if _, err = output.File.Write(encoded); err == nil {
		var digest string
		digest, err = output.Digest(ctx)
		if err == nil && digest != x.digest {
			err = errors.New("diagnostic digest differs")
		}
	}
	if err != nil {
		_ = output.Discard()
		return result
	}
	if err = output.Publish(); err != nil {
		_ = output.Discard()
		return result
	}
	x.destination = destination
	x.published = true
	result = failure("DIAGNOSTICS_RESULT_UNCONFIRMED", "文件已发布，正在核对关闭结果；重试仅核对原文件。", true)
	err = output.Close()
	closed = true
	if err != nil {
		return result
	}
	result = success(map[string]any{"status": "saved", "reportId": input.ReportID, "sha256": x.digest}, "")
	return result
}

// Ends UI verification only. The original receipt remains in this host's
// ledger, including unknown outcomes; retrying that ID never writes again.
func (h *DiagnosticHost) endVerification(input DiagnosticExportRequest) Result {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.busy {
		return failure("DIAGNOSTICS_RESULT_UNCONFIRMED", "原操作仍在进行，尚不能结束核实。", true)
	}
	if x := h.exports[input.RequestID]; x != nil {
		if x.input != input {
			return failure("REQUEST_ID_REUSED", "原请求不能更换报告。", false)
		}
		if x.result.OK {
			return x.result
		}
	} else {
		// A request can arrive late after its transport already failed at the
		// caller. Seal even unseen IDs so ending verification cannot be followed
		// by a delayed chooser or a new write from the old request.
		h.exports[input.RequestID] = &diagnosticExport{input: input, result: failure("DIAGNOSTICS_RESULT_UNCONFIRMED", "此请求已结束核实，旧文件是否发布未确认；原请求不会再次写入。", false)}
	}
	return success(map[string]any{"status": "unconfirmed", "reportId": input.ReportID}, "")
}
