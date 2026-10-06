import { useEffect, useRef, useState, useSyncExternalStore } from "react";
import { FileText, LoaderCircle } from "lucide-react";
import type { ApplicationService } from "../application/contract";
import { getProxyImportSession, type ProxyImportSession } from "./proxy-import-session";
import { ProxyKernelModal } from "./ProxyKernelModal";

export interface ProxyImportWindowProps {
  application: ApplicationService; open: boolean; onClose: () => void; session?: ProxyImportSession;
  onImported?: (ids: string[]) => void; onBusyChange?: (busy: boolean) => void;
  context?: "page" | "environment"; initialMode?: "text" | "file" | "single"; lifecycle?: "self" | "parent";
}

/** The single native/demo import window, including environment return. */
export function ProxyImportWindow({ application, open, onClose, session = getProxyImportSession(application), onImported, onBusyChange, context = "page", initialMode = "text", lifecycle = "self" }: ProxyImportWindowProps) {
  const state = useSyncExternalStore(session.subscribe, session.getSnapshot, session.getSnapshot);
  const alive = useRef(true), visible = useRef(open); visible.current = open;
  useEffect(() => { alive.current = true; return () => { alive.current = false; }; }, []);
  const [mode, setMode] = useState(initialMode);
  useEffect(() => { if (open) setMode(initialMode === "single" && state.text.trim() && !state.singleDraft ? "text" : initialMode); }, [open, initialMode]);
  useEffect(() => { onBusyChange?.(state.busy); }, [state.busy, onBusyChange]);
  if (!open) return null;
  const locked = state.busy || state.unknown || !!application.getSnapshot().issue;
  const close = () => { session.setShowText(false); onClose(); };
  const save = async () => { const ids = await session.commit(); if (ids && alive.current && visible.current) { onImported?.(ids); if (!session.getSnapshot().text.trim()) close(); } };
  const rows = state.preview?.rows ?? [];
  const message = state.busy ? "本机服务正在处理，尚未确认保存。" : state.message;
  const footer = <>{state.failure && message && <p className="proxy35-footer-message pk35-warning" role="status" aria-live="polite">{message}</p>}{mode !== "file" && <><span className="pk35-footer-note">{context === "environment" ? "环境草稿与固定身份已保留" : "关闭保留本次输入；不撤销已保存节点"}</span><button className="button" disabled={locked} onClick={session.clear}>清除输入与预览</button></>}<button className="button" onClick={close}>{context === "environment" ? "返回环境配置" : "取消"}</button>{mode === "file" ? <button className="button primary" disabled={!state.text.trim() || state.busy} onClick={() => setMode("text")}>继续解析预览</button> : <button className="button primary" disabled={state.busy || !state.selected.length || !!application.getSnapshot().issue} onClick={() => void save()}>{state.unknown ? "核实原导入请求" : "保存所选有效行"}</button>}</>;
  return <ProxyKernelModal title={mode === "file" ? "批量导入代理" : mode === "single" ? "添加代理" : "批量添加代理"} width={mode === "file" ? 500 : mode === "single" ? 620 : 1050} height={mode === "file" ? 360 : mode === "single" ? 552 + rows.length * 58 : Math.min(552 + Math.max(0, rows.length - 1) * 58, 726)} onClose={close} footer={footer} lifecycle={lifecycle} className={`proxy35-import proxy35-import-${mode}`}>
    {mode === "file" ? <>
      <p className="pk35-warning">仅从本地读取 UTF-8 文本文件，先解析再确认保存；不会执行真实检测，不支持商业 Excel 模板或云代理接口。</p>
      <div className="proxy35-file-box"><div><FileText size={18} /><strong>单个 UTF-8 文本文件 · 最多 2MiB</strong><label className="button proxy35-file-pick">选择文件<input aria-label="选择UTF-8文本文件" type="file" accept=".txt,text/plain" disabled={locked} onChange={event => { const file = event.target.files?.[0]; event.target.value = ""; if (file) void session.loadFile(file); }} /></label></div><p><span>{state.fileName ?? "尚未选择文件"}</span><button className="text-button" disabled={locked} onClick={session.clear}>清除输入与预览</button></p></div>
      <p>关闭保留本次输入；不撤销已保存节点。文件只在本次内存会话保留；本机凭据预览不回显。</p>
    </> : mode === "single" ? <>
      <label className="reference-field"><span>代理类型</span><select aria-label="添加代理类型" disabled={locked} value={state.singleDraft?.type ?? "socks5"} onChange={event => session.setSingle({ type: event.target.value as "http" | "https" | "socks5" })}><option value="socks5">SOCKS5</option><option value="http">HTTP</option><option value="https">HTTPS · TLS 到代理</option></select></label>
      <label className="reference-field"><span>代理主机</span><input aria-label="添加代理主机" disabled={locked} value={state.singleDraft?.host ?? ""} onChange={event => session.setSingle({ host: event.target.value })} /></label>
      <label className="reference-field"><span>代理端口</span><input aria-label="添加代理端口" disabled={locked} type="number" min={1} max={65535} value={state.singleDraft?.port ?? "1080"} onChange={event => session.setSingle({ port: event.target.value })} /></label>
      <label className="reference-field"><span>代理账号</span><input aria-label="添加代理账号" disabled={locked} type="password" autoComplete="off" value={state.singleDraft?.username ?? ""} onChange={event => session.setSingle({ username: event.target.value })} /></label>
      <label className="reference-field"><span>代理密码</span><input aria-label="添加代理密码" disabled={locked} type="password" autoComplete="new-password" value={state.singleDraft?.password ?? ""} onChange={event => session.setSingle({ password: event.target.value })} /></label>
      <p>与批量导入共用同一预览及保存请求。名称由服务解析生成，可在保存后编辑；HTTP / SOCKS5 不加密到代理的认证，HTTPS 才提供 TLS。</p>
      <button className="button primary" disabled={locked || !state.singleDraft?.host} onClick={() => void session.parse()}>解析预览</button><button className="text-button" disabled={locked} onClick={() => setMode("text")}>转为批量文本窗口</button>
      {rows.map(row => <p key={row.line} className={row.error ? "pk35-warning" : ""}>{row.error ?? `${row.configuration?.type.toUpperCase()} · ${row.configuration?.host}:${row.configuration?.port} · ${row.hasAuthentication ? "已输入认证（不回显）" : "无认证"}`}{!row.error && <label className="pk35-checkbox"><input type="checkbox" aria-label={`保存第${row.line}行`} disabled={locked} checked={state.selected.includes(row.line)} onChange={event => session.select(row.line, event.target.checked)} />明确保存此节点{row.duplicateCount || row.existingCount ? `（已存 ${row.existingCount} 个同地址节点；地址相同不代表认证相同）` : ""}</label>}</p>)}
    </> : <>
      <div className="proxy35-import-columns"><div><label className="proxy35-input-title" htmlFor="proxy35-text">代理信息：（按右侧说明填写，每行一个）</label><textarea id="proxy35-text" aria-label="原始代理导入文本" className={state.showText ? "" : "proxy35-secret"} autoComplete="off" spellCheck={false} value={state.text} disabled={locked} onChange={event => session.setText(event.target.value)} /><div className="proxy35-input-options"><label><input type="checkbox" checked={state.showText} onChange={event => session.setShowText(event.target.checked)} />显示原始输入（可能含凭据）</label><button className="text-button" disabled={locked} onClick={() => setMode("file")}>选择文本文件</button></div></div>
        <div className="proxy35-import-help"><strong>填写说明：</strong><ol><li>支持 HTTP、HTTPS、SOCKS5；协议后加 <code>://</code>。</li><li>每行一条，空白与 # 注释不保存；坏行保留原始行号。</li><li>支持 <code>host:port:username:password</code>。IPv6 使用 URI，主机放在 [ ] 内。</li><li>包含分隔符的认证须 URI 编码；SOCKS5 账号和密码各为 1–255 个 UTF-8 字节。</li><li>HTTPS 才加密到代理的认证；历史检查不保证下次启动或永久保护。</li></ol><strong>填写格式举例：</strong><p>http://198.51.100.90:8080<br />socks5://example:synthetic@192.0.2.90:1080<br />socks5://[2001:db8::1]:1080<br />198.51.100.90:8080:example:synthetic</p><span>{application.mode === "demo" ? "演示模式 · 只保存示例配置，不进行真实检测" : "本机模式 · 凭据安全预览，不回显用户名或密码"}</span></div>
      </div>
      <div className="proxy35-preview-toolbar"><strong>已识别 {rows.filter(row => row.configuration && !row.error).length} 行 · 错误 {rows.filter(row => row.error).length} 行 · 已选 {state.selected.length} 行</strong><button className="button primary" disabled={locked || !state.text.trim()} onClick={() => void session.parse()}>解析预览</button></div>
      <div className="pk35-table-scroll proxy35-preview-scroll"><table className="pk35-table proxy35-preview-table"><thead><tr><th>保存</th><th>原始行号</th><th>代理类型</th><th>代理主机 / 错误</th><th>代理端口</th><th>认证</th><th>重复候选</th></tr></thead><tbody>{rows.length ? rows.map(row => <tr key={row.line} className={row.error ? "pk35-error-row" : ""}><td><input aria-label={`保存第${row.line}行`} type="checkbox" disabled={locked || !row.configuration || !!row.error} checked={state.selected.includes(row.line)} onChange={event => session.select(row.line, event.target.checked)} /></td><td>{row.line}</td><td>{row.configuration?.type.toUpperCase() ?? "—"}</td><td>{row.error ?? row.configuration?.host}</td><td>{row.configuration?.port ?? "—"}</td><td>{row.configuration ? row.hasAuthentication ? "已输入（不回显）" : "无认证" : "—"}</td><td>{row.duplicateCount || row.existingCount ? `本批另有 ${row.duplicateCount} 行 / 已存 ${row.existingCount} 个同地址节点` : "—"}</td></tr>) : <tr><td colSpan={7} className="pk35-empty">暂无预览数据</td></tr>}</tbody></table></div>
      {state.preview && <div className="proxy35-preview-meta">忽略空白/注释 {state.preview.ignoredLines} 行{application.mode === "native" ? " · 服务预览 15 分钟后失效" : " · 演示解析"}{Object.entries(state.preview.duplicateGroups).map(([id, group]) => <span key={id}>重复组：行 {group.lines.join("、")}；已存 {group.existingProxyIds.length} 个同地址节点。</span>)}</div>}
    </>}
    {!state.failure && message && <p className="pk35-message" role="status" aria-live="polite">{state.busy && <LoaderCircle size={14} className="spin" />}{message}</p>}
  </ProxyKernelModal>;
}
