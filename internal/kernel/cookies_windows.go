//go:build windows

package kernel

import (
	"context"

	"github.com/axgiroud312-byte/prism-local-browser/internal/cookies"
)

func (p *ManagedProcess) beginCookieCommand(ctx context.Context) error {
	if err := p.pipe.beginCommand(ctx); err != nil {
		return &cookies.Error{Code: "COOKIE_CONTROL_UNAVAILABLE", Message: "本次私有控制通道不可用，未改用其他会话或数据库。", Retryable: true}
	}
	snapshot := p.Snapshot()
	if !snapshot.RootAlive || !snapshot.ControlReady || snapshot.ResourcesExited || snapshot.ProxyError != nil {
		<-p.pipe.commandGate
		return &cookies.Error{Code: "COOKIE_SESSION_REQUIRED", Message: "本次会话不再可安全控制，未写入其他环境。", Retryable: true}
	}
	return nil
}

func (p *ManagedProcess) readCookiesLocked(ctx context.Context) ([]cookies.Stored, error) {
	var result struct {
		Cookies []cookies.Stored `json:"cookies"`
	}
	if err := p.pipe.callLocked(ctx, "Storage.getCookies", struct{}{}, "", &result); err != nil || result.Cookies == nil {
		return nil, &cookies.Error{Code: "COOKIE_READ_FAILED", Message: "本次完整Cookie读回失败，不能把命令受理或截断结果计为成功。", Retryable: true}
	}
	return result.Cookies, nil
}

// ReadCookies is host-only. Values remain private and never cross the RPC.
func (p *ManagedProcess) ReadCookies(ctx context.Context) ([]cookies.Stored, error) {
	if err := p.beginCookieCommand(ctx); err != nil {
		return nil, err
	}
	defer func() { <-p.pipe.commandGate }()
	return p.readCookiesLocked(ctx)
}

// ApplyCookie serializes the whole sequence, not just individual CDP calls.
// Omitting browserContextId selects this owned browser's default context.
func (p *ManagedProcess) ApplyCookie(ctx context.Context, value cookies.Cookie) (cookies.ApplyResult, error) {
	if err := p.beginCookieCommand(ctx); err != nil {
		return cookies.ApplyResult{}, err
	}
	defer func() { <-p.pipe.commandGate }()
	stored, err := p.readCookiesLocked(ctx)
	if err != nil {
		return cookies.ApplyResult{}, err
	}
	matched := cookies.Matches(value, stored)
	for index := range stored {
		stored[index].Value = ""
	}
	if matched {
		return cookies.ApplyResult{Status: "already-matched"}, nil
	}
	if ctx.Err() != nil {
		return cookies.ApplyResult{}, &cookies.Error{Code: "OPERATION_CANCELLED", Message: "本次Cookie命令尚未发送，已取消后续写入。", Retryable: true}
	}
	// No URL is added: HTTPS URL would silently force secure=true in this build.
	params := struct {
		Cookies []cookies.Cookie `json:"cookies"`
	}{Cookies: []cookies.Cookie{value}}
	if err := p.pipe.callLocked(ctx, "Storage.setCookies", params, "", nil); err != nil {
		return cookies.ApplyResult{}, &cookies.Error{Code: "COOKIE_WRITE_UNCONFIRMED", Message: "本次写命令结果未知，可能已有副作用；重试先读取同键，不清空或补偿其他Cookie。", Retryable: true}
	}
	stored, err = p.readCookiesLocked(ctx)
	if err != nil {
		return cookies.ApplyResult{}, &cookies.Error{Code: "COOKIE_WRITE_UNCONFIRMED", Message: "命令已受理但未完成读回，不能计入真实成功；重试先核对同键。", Retryable: true}
	}
	matched = cookies.Matches(value, stored)
	for index := range stored {
		stored[index].Value = ""
	}
	if !matched {
		return cookies.ApplyResult{}, &cookies.Error{Code: "COOKIE_VERIFY_MISMATCH", Message: "读回的值或属性与输入不一致（含过期/作用域/内核限制），未续期或降低安全属性重试。", Retryable: true}
	}
	return cookies.ApplyResult{Status: "verified"}, nil
}

// Used only after the user explicitly selects whole-environment replacement.
func (p *ManagedProcess) ClearCookies(ctx context.Context) error {
	if err := p.beginCookieCommand(ctx); err != nil {
		return err
	}
	defer func() { <-p.pipe.commandGate }()
	if err := p.pipe.callLocked(ctx, "Storage.clearCookies", struct{}{}, "", nil); err != nil {
		return &cookies.Error{Code: "COOKIE_CLEAR_UNCONFIRMED", Message: "清空命令结果未知，可能已有副作用；没有自动重清或恢复全集。", Retryable: false}
	}
	stored, err := p.readCookiesLocked(ctx)
	count := len(stored)
	for index := range stored {
		stored[index].Value = ""
	}
	if err != nil || count != 0 {
		return &cookies.Error{Code: "COOKIE_CLEAR_UNCONFIRMED", Message: "未读回确认目标Cookie集合为空，未继续写入；再次清空须重新预览和明确选择。", Retryable: false}
	}
	return nil
}
