import { useEffect, useRef, useState } from "react";
import type { ApplicationService, NativeRestorePage, NativeRestorePreview, WorkspaceView } from "../application/contract";
import { NativeRestoreExecution } from "./NativeRestoreExecution";

const reason: Record<string, string> = { "name-conflict": "名称已由另一环境占用", "fingerprint-id-conflict": "档案ID属于另一环境", "prepared-identity-conflict": "批次仍保留此身份", "seed-or-reserved-identity-conflict": "其他身份的当前、历史或预约seed冲突", "original-fingerprint-reference-conflict": "原档案引用不一致" };
const kernelState: Record<string, string> = { pending: "未绑定（仍不能启动）", missing: "缺少同版本同摘要内核", unavailable: "本机文件无法核对", "verified-bytes": "本机文件与精确构建相符" };
const credentialState: Record<string, string> = { none: "无认证", "available-current-user": "当前Windows用户可解密（未联网检查）", "reentry-required": "当前密钥上下文不可用，恢复后须重新输入认证" };

export function NativeRestoreManager({ application, workspace }: { application: ApplicationService; workspace: WorkspaceView }) {
  const [source, setSource] = useState<{ token: string; name: string }>();
  const [preview, setPreview] = useState<NativeRestorePreview>();
  const [page, setPage] = useState<NativeRestorePage>();
  const [busy, setBusy] = useState(false), [message, setMessage] = useState(""), [discarding, setDiscarding] = useState(false);
  const [executionLocked, setExecutionLocked] = useState(false);
  const executionLock = useRef(false);
  const mounted = useRef(true), generation = useRef(0), flight = useRef(false);
  const discardFlight = useRef(false);
  const owned = useRef({ previewId: "", sourceToken: "" });
  useEffect(() => { mounted.current = true; return () => { mounted.current = false; generation.current++; const current = owned.current; if (!executionLock.current && application.getPendingRestore?.()?.request.previewId !== current.previewId) void application.discardRestore?.(current.previewId, current.sourceToken); }; }, [application]);
  async function choose() {
    if (flight.current || discardFlight.current || executionLock.current) return; flight.current = true; setBusy(true); setMessage(""); const current = ++generation.current;
    const result = await application.selectRestoreSource?.(); flight.current = false;
    if (!mounted.current || current !== generation.current) { if (result?.ok && result.data.sourceToken) void application.discardRestore?.("", result.data.sourceToken); return; }
    setBusy(false);
    if (!result?.ok) { setMessage(result?.error.message ?? "本机文件选择不可用。"); return; }
    if (result.data.status === "selected" && result.data.sourceToken) { owned.current = { previewId: "", sourceToken: result.data.sourceToken }; setSource({ token: result.data.sourceToken, name: result.data.name ?? "本机备份" }); setPreview(undefined); setPage(undefined); }
  }
  async function readPage(p: NativeRestorePreview, offset: number, current: number) {
    const result = await application.readRestorePage?.({ previewId: p.previewId, offset, pageSize: 25 });
    if (!mounted.current || current !== generation.current || owned.current.previewId !== p.previewId) return;
    if (result?.ok) setPage(result.data); else setMessage(result?.error.message ?? "无法读取影响清单。");
  }
  async function inspect() {
    if (!source || flight.current || discardFlight.current || executionLock.current) return; flight.current = true; setBusy(true); setMessage(""); setPreview(undefined); setPage(undefined); const current = ++generation.current;
    const result = await application.previewRestore?.(source.token); flight.current = false;
    if (!mounted.current || current !== generation.current) { if (result?.ok) void application.discardRestore?.(result.data.previewId, source.token); return; }
    setBusy(false);
    if (!result?.ok) { setMessage(result ? `${result.error.code}：${result.error.message}` : "恢复预检能力不可用。"); return; }
    owned.current.previewId = result.data.previewId; setPreview(result.data); await readPage(result.data, 0, current);
  }
  async function discard() {
    if (discardFlight.current || executionLock.current) return;
    discardFlight.current = true; setDiscarding(true);
    const current = owned.current;
    const result = await application.discardRestore?.(current.previewId, current.sourceToken);
    discardFlight.current = false;
    if (!mounted.current) return; setDiscarding(false);
    if (owned.current !== current) return;
    if (!result?.ok) { setMessage(result?.error.message ?? "取消结果未确认。"); return; }
    generation.current++;
    owned.current = { previewId: "", sourceToken: "" }; setSource(undefined); setPreview(undefined); setPage(undefined); setBusy(false); setMessage("已丢弃预检；当前数据库与浏览数据未修改。");
  }
  return <section className="native-proxy-panel" aria-label="恢复只读预检">
    <h2>恢复前只读预检</h2>
    <p>先完整核对文件、配置、原身份和覆盖范围。不会停止环境或修改当前数据库、浏览目录；演示 JSON 不可用。</p>
    <div className="native-proxy-actions"><button className="button" disabled={busy || discarding || executionLocked} onClick={() => void choose()}>选择本机备份包</button><span>{source?.name ?? "尚未选择"}</span><button className="button" disabled={busy || discarding || executionLocked || !source} onClick={() => void inspect()}>完整校验并预览</button>{source && <button className="button" disabled={discarding || executionLocked} onClick={() => void discard()}>{busy ? "取消只读预检" : "丢弃此预览"}</button>}</div>
    {busy && <p role="status">正在流式读取全部文件并核对摘要…</p>}{message && <p role="alert">{message}</p>}
    {preview && <>
      <h3>包校验通过 · 尚未恢复</h3><p>备份时间：{preview.createdAt} · 新增 {preview.addCount} · 覆盖 {preview.overwriteCount} · 冲突 {preview.conflictCount} · 缺失/未核对精确内核 {preview.missingKernelCount}</p>
      <p>只处理包中的明确环境，保留包外环境。恢复保留原 ID、seed、设备参数和实际数据；编号和相同精确内核的内部引用可能重新映射。覆盖会撤回备份之后的数据。</p>
      <p>包 SHA-256：<code>{preview.archiveSha256}</code></p><p>预览有效至 {preview.expiresAt}。提交前需再次核对包及当前配置；摘要不是来源签名。</p>
      {page && <><div className="native-proxy-table"><table><thead><tr><th>环境 / ID</th><th>影响</th><th>原身份</th><th>冲突与状态</th></tr></thead><tbody>{page.items.map(item => <tr key={item.id}><td>{item.name}<small>{item.id}</small></td><td>{item.action === "add" ? "新增" : `覆盖修订 ${item.currentRevision}`} · 备份修订 {item.backupRevision}</td><td>seed {item.seed} · {item.dataState === "present" ? "真实浏览目录" : "从未初始化"}</td><td>{item.conflicts.map(c => reason[c] ?? c).join("；") || "无身份冲突"}{item.busy && "；当前运行或维护中，正式恢复前须安全停止"}</td></tr>)}</tbody></table></div><div className="native-proxy-actions"><button className="button" disabled={page.offset === 0} onClick={() => void readPage(preview, Math.max(0, page.offset - 25), ++generation.current)}>上一页</button><span>{page.offset + 1}–{page.offset + page.items.length} / {page.total}</span><button className="button" disabled={page.offset + page.items.length >= page.total} onClick={() => void readPage(preview, page.offset + 25, ++generation.current)}>下一页</button></div></>}
      <h3>精确内核</h3>{preview.kernels.map(k => <p key={k.id}>{k.version || "未绑定"} · {kernelState[k.state]}{!k.required && "（未被恢复环境引用）"}{k.localId && k.localId !== k.id && " · 将映射本机同摘要构建"}<br /><code>{k.executableSha256}</code></p>)}
      <h3>代理凭据</h3>{preview.credentials.map(c => <p key={c.proxyId}>{c.proxyId} · {credentialState[c.state]}</p>)}
      <p>代理可解密不代表浏览器登录可跨用户恢复；浏览数据仍限定原Windows用户/密钥上下文，不承诺跨机便携。</p>
      {preview.conflictCount > 0 && <p role="alert">存在身份或共享代理/凭据冲突。请先处理当前配置，再重新预检；不通过生成新seed绕过。</p>}
    </>}
    <NativeRestoreExecution application={application} workspace={workspace} preview={preview} onLockChange={locked => { executionLock.current = locked; setExecutionLocked(locked); }} onConsumed={previewId => { if (owned.current.previewId !== previewId) return; generation.current++; owned.current = { previewId: "", sourceToken: "" }; setSource(undefined); setPreview(undefined); setPage(undefined); }} />
  </section>;
}
