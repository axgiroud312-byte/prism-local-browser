import type { FormEvent } from "react";
import type { ProxyConfiguration } from "../application/contract";
import { ProxyKernelModal } from "./ProxyKernelModal";
import { socks5CredentialByteError } from "./proxy-credential-validation";

export interface ProxyEditWindowProps {
  title?: string; configuration: ProxyConfiguration; onConfiguration: (value: ProxyConfiguration) => void;
  credentialAction: "keep" | "replace" | "clear"; onCredentialAction: (value: "keep" | "replace" | "clear") => void;
  username: string; password: string; onUsername: (value: string) => void; onPassword: (value: string) => void;
  hasAuthentication: boolean; native: boolean; busy: boolean; unknown?: boolean; message?: string;
  simulateFailure?: boolean; onSimulateFailure?: (value: boolean) => void;
  onClose: () => void; onSave: (event?: FormEvent) => void;
}
export function ProxyEditWindow({ title = "修改代理", configuration, onConfiguration, credentialAction, onCredentialAction, username, password, onUsername, onPassword, hasAuthentication, native, busy, unknown = false, message, simulateFailure, onSimulateFailure, onClose, onSave }: ProxyEditWindowProps) {
  const locked = busy || unknown;
  // Native remains authoritative, and reconciliation must not validate a new draft.
  const credentialError = native || unknown ? undefined : socks5CredentialByteError(configuration.type, credentialAction, username, password);
  const patch = (change: Partial<ProxyConfiguration>) => onConfiguration({ ...configuration, ...change });
  return <ProxyKernelModal title={title} width={620} height={native && credentialAction === "replace" ? 726 : 614} busy={busy} onClose={onClose} className="proxy35-edit" footer={<><span className="pk35-footer-note">保存使旧检查失效，不改变环境 seed</span><button className="button" disabled={busy} onClick={onClose}>取消</button><button className="button primary" type="submit" form="proxy35-edit-form" disabled={busy}>{unknown ? "核实原保存请求" : "确认保存"}</button></>}>
    <form id="proxy35-edit-form" onSubmit={event => { event.preventDefault(); if (!credentialError) onSave(event); }}>
      <label className="reference-field"><span>代理名称</span><input aria-label="名称" value={configuration.name} required maxLength={256} disabled={locked} onChange={event => patch({ name: event.target.value })} /></label>
      <label className="reference-field"><span>代理类型</span><select aria-label="协议" value={configuration.type} disabled={locked} onChange={event => patch({ type: event.target.value as ProxyConfiguration["type"] })}><option value="socks5">SOCKS5</option><option value="http">HTTP</option><option value="https">HTTPS · TLS 到代理</option></select></label>
      <label className="reference-field"><span>代理主机</span><input aria-label="主机名或IP" value={configuration.host} required disabled={locked} onChange={event => patch({ host: event.target.value })} /></label>
      <label className="reference-field"><span>代理端口</span><input aria-label="端口" type="number" min={1} max={65535} value={configuration.port} required disabled={locked} onChange={event => patch({ port: Number(event.target.value) })} /></label>
      <label className="reference-field"><span>地区标签</span><div><input aria-label="地区标签（自行填写，非实测）" value={configuration.country} maxLength={128} disabled={locked} onChange={event => patch({ country: event.target.value })} /><small>自行填写，不是本次实际 IP 观测。</small></div></label>
      <label className="reference-field"><span>认证处理</span><select aria-label="认证处理" value={credentialAction} disabled={locked} onChange={event => onCredentialAction(event.target.value as ProxyEditWindowProps["credentialAction"])}><option value="keep">保留现有认证（{hasAuthentication ? "已设置" : "未设置"}）</option><option value="replace">明确替换用户名和密码</option><option value="clear">明确清除认证</option></select></label>
      {credentialAction === "replace" && <><label className="reference-field"><span>新代理账号</span><input aria-label="新用户名（不回显旧值）" type="password" autoComplete="off" maxLength={4096} disabled={locked} value={username} onChange={event => onUsername(event.target.value)} /></label><label className="reference-field"><span>新代理密码</span><input aria-label="新密码" type="password" autoComplete="new-password" maxLength={4096} disabled={locked} value={password} onChange={event => onPassword(event.target.value)} /></label></>}
      {!native && onSimulateFailure && <label className="pk35-checkbox"><input type="checkbox" checked={!!simulateFailure} disabled={locked} onChange={event => onSimulateFailure(event.target.checked)} />模拟连接失败（只影响示例检测）</label>}
      <p className="pk35-muted">{native ? "旧账号、密码从不回显；保留与清除不携带新凭据。" : "仅演示配置，请勿使用真实凭据。"}HTTP / SOCKS5 不加密到代理的认证；HTTPS 才提供 TLS。</p>
      {configuration.type === "socks5" && <p className="pk35-muted">目标域名固定由上游解析，代理主机自身仍需本机 DNS；不降级直连。{credentialAction === "replace" && `用户名 ${new TextEncoder().encode(username).length} 字节，密码 ${new TextEncoder().encode(password).length} 字节（各需 1–255）。`}</p>}
      {credentialError && <p role="alert" className="pk35-warning">{credentialError}</p>}
      {message && <p role="status" className="pk35-message">{message}</p>}
    </form>
  </ProxyKernelModal>;
}
