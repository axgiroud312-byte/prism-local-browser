import { useEffect, useRef, useState } from "react";
import type { ApplicationService, NativeRestorePage, NativeRestorePreview, WorkspaceView } from "../application/contract";
import { NativeRestoreExecution } from "./NativeRestoreExecution";
import { ReferenceButton } from "./ReferenceUi";
import { LocalPageTable, LocalPageWindow } from "./LocalPageUi";

const reason: Record<string, string> = { "name-conflict": "名称已由另一环境占用", "fingerprint-id-conflict": "档案ID属于另一环境", "prepared-identity-conflict": "批次仍保留此身份", "seed-or-reserved-identity-conflict": "其他身份的当前、历史或预约seed冲突", "original-fingerprint-reference-conflict": "原档案引用不一致" };
const kernelState: Record<string, string> = { pending: "未绑定（仍不能启动）", missing: "缺少同版本同摘要内核", unavailable: "本机文件无法核对", "verified-bytes": "本机文件与精确构建相符" };
const credentialState: Record<string, string> = { none: "无认证", "available-current-user": "当前Windows用户可解密（未联网检查）", "reentry-required": "当前密钥上下文不可用，恢复后须重新输入认证" };

export function NativeRestoreManager({ application, workspace }: { application: ApplicationService; workspace: WorkspaceView }) {
  const [source, setSource] = useState<{ token: string; name: string }>();
  const [preview, setPreview] = useState<NativeRestorePreview>();
  const [page, setPage] = useState<NativeRestorePage>();
  const [busy, setBusy] = useState(false), [message, setMessage] = useState(""), [discarding, setDiscarding] = useState(false);
  const [executionLocked, setExecutionLocked] = useState(false);
  const [window, setWindow] = useState<"import" | "preflight">();
  const [section, setSection] = useState<"environments" | "kernels" | "credentials">("environments");
  const [confirmationRequested, setConfirmationRequested] = useState(0), [pageBusy, setPageBusy] = useState(false);
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
    if (result.data.status === "selected" && result.data.sourceToken) {
      const previous = owned.current;
      if (previous.sourceToken) void application.discardRestore?.(previous.previewId, previous.sourceToken);
      owned.current = { previewId: "", sourceToken: result.data.sourceToken }; setSource({ token: result.data.sourceToken, name: result.data.name ?? "本机备份" }); setPreview(undefined); setPage(undefined);
    } else setMessage("已取消文件选择；未预检或恢复，原输入保留。");
  }
  async function readPage(p: NativeRestorePreview, offset: number, current: number) {
    setPageBusy(true); setPage(undefined);
    const result = await application.readRestorePage?.({ previewId: p.previewId, offset, pageSize: 25 });
    if (!mounted.current || current !== generation.current || owned.current.previewId !== p.previewId) return;
    setPageBusy(false);
    if (result?.ok) { setPage(result.data); setMessage(""); } else setMessage(result ? `${result.error.code}：${result.error.message} 可重读影响清单，不提交恢复。` : "无法读取影响清单。");
  }
  async function inspect() {
    if (!source || flight.current || discardFlight.current || executionLock.current) return; flight.current = true; setBusy(true); setMessage(""); setPreview(undefined); setPage(undefined); const current = ++generation.current;
    const result = await application.previewRestore?.(source.token); flight.current = false;
    if (!mounted.current || current !== generation.current) { if (result?.ok) void application.discardRestore?.(result.data.previewId, source.token); return; }
    setBusy(false);
    if (!result?.ok) { setMessage(result ? `${result.error.code}：${result.error.message}` : "恢复预检能力不可用。"); return; }
    owned.current.previewId = result.data.previewId; setPreview(result.data); setSection("environments"); setWindow("preflight"); await readPage(result.data, 0, current);
  }
  async function discard() {
    if (discardFlight.current || executionLock.current) return;
    discardFlight.current = true; setDiscarding(true); generation.current++;
    const current = owned.current;
    const result = await application.discardRestore?.(current.previewId, current.sourceToken);
    discardFlight.current = false;
    if (!mounted.current) return; setDiscarding(false);
    if (owned.current !== current) return;
    if (!result?.ok) { setMessage(result?.error.message ?? "取消结果未确认。"); return; }
    generation.current++;
    owned.current = { previewId: "", sourceToken: "" }; setSource(undefined); setPreview(undefined); setPage(undefined); setBusy(false); setPageBusy(false); setWindow(undefined); setMessage("已丢弃预检；当前数据库与浏览数据未修改。");
  }
  return <div className="local-page-restore-entry" aria-label="恢复只读预检">
    <ReferenceButton disabled={busy || discarding || executionLocked} onClick={() => { setWindow(preview ? "preflight" : "import"); setMessage(""); }}>导入完整备份</ReferenceButton>
    {window === "import" && <LocalPageWindow title="导入本机备份" height={359} busy={busy || discarding} onClose={() => { if (source) void discard(); else setWindow(undefined); }} footer={<><ReferenceButton disabled={discarding || executionLocked} onClick={() => { if (source) void discard(); else setWindow(undefined); }}>{busy ? "取消只读预检" : "取消"}</ReferenceButton><ReferenceButton className="primary" disabled={busy || discarding || executionLocked || !source} onClick={() => void inspect()}>完整校验并预览</ReferenceButton></>}>
      <p>只读核对完整 .prismbackup；不会停止环境、写数据库或替换目录。演示 JSON 不可用。</p>
      <div className="local-page-upload"><div className="local-page-upload-head"><span>本机完整备份包<br />不上传到云端</span><ReferenceButton disabled={busy || discarding || executionLocked} onClick={() => void choose()}>选择本机备份包</ReferenceButton></div><p>{source?.name ?? "尚未选择文件"}</p></div>
      <p className="subtle-text">摘要、路径与精确内核由服务校验；取消系统选择器不会开始恢复。</p>
      {busy && <p role="status">正在流式读取全部文件并核对摘要…</p>}{message && <p role="alert">{message}</p>}
    </LocalPageWindow>}
    {window === "preflight" && preview && <LocalPageWindow title="恢复前只读预检" width={1040} height={592} className="local-page-preflight" busy={discarding} onClose={() => { if (executionLock.current) setWindow(undefined); else void discard(); }} footer={<><ReferenceButton disabled={discarding || executionLocked} onClick={() => void discard()}>丢弃此预览</ReferenceButton><ReferenceButton className="primary" disabled={!preview.canRestore || pageBusy || !page || executionLocked || discarding} onClick={() => setConfirmationRequested(value => value + 1)}>下一步：确认恢复</ReferenceButton></>}>
      <p>包校验通过 · 尚未恢复 · {preview.name}</p><div className="local-page-scope"><span>新增 {preview.addCount}</span><span>覆盖 {preview.overwriteCount}</span><span>冲突 {preview.conflictCount}</span><span>缺失/未核对精确内核 {preview.missingKernelCount}</span><span>凭据需重输 {preview.credentialReentryCount}</span></div>
      <div className="local-page-section-tabs">{([{ id: "environments", label: "影响环境" }, { id: "kernels", label: "精确内核" }, { id: "credentials", label: "代理凭据" }] as const).map(tab => <button key={tab.id} className={section === tab.id ? "selected" : ""} onClick={() => setSection(tab.id)}>{tab.label}</button>)}</div>
      {section === "environments" && <><LocalPageTable headings={["环境 / ID", "影响", "浏览数据", "冲突与状态"]} empty={!page ? pageBusy ? "正在读取影响清单…" : "影响清单读取失败；请重读，不提交恢复。" : !page.items.length ? "包中没有影响环境。" : undefined}>{page?.items.map(item => <tr key={item.id}><td>{item.name}<small>{item.id}</small></td><td>{item.action === "add" ? "新增" : `覆盖修订 ${item.currentRevision}`}<small>备份修订 {item.backupRevision}</small></td><td>{item.dataState === "present" ? "真实浏览目录" : "从未初始化"}</td><td>{item.conflicts.map(c => reason[c] ?? c).join("；") || "无身份冲突"}{item.busy && "；运行或维护中，须正常停止"}</td></tr>)}</LocalPageTable><div className="local-page-pagination">{page ? <><ReferenceButton disabled={pageBusy || page.offset === 0} onClick={() => void readPage(preview, Math.max(0, page.offset - 25), ++generation.current)}>上一页</ReferenceButton><span>{page.total ? page.offset + 1 : 0}–{page.offset + page.items.length} / {page.total}</span><ReferenceButton disabled={pageBusy || page.offset + page.items.length >= page.total} onClick={() => void readPage(preview, page.offset + 25, ++generation.current)}>下一页</ReferenceButton></> : <ReferenceButton disabled={pageBusy} onClick={() => void readPage(preview, 0, ++generation.current)}>重新读取影响清单</ReferenceButton>}</div></>}
      {section === "kernels" && <LocalPageTable headings={["精确版本", "本机核对", "主程序 SHA-256"]}>{preview.kernels.map(k => <tr key={k.id}><td>{k.version || "未绑定"}</td><td>{kernelState[k.state]}{!k.required && "（未引用）"}{k.localId && k.localId !== k.id && " · 映射同摘要构建"}</td><td><code>{k.executableSha256 || "未绑定，不可启动"}</code></td></tr>)}</LocalPageTable>}
      {section === "credentials" && <><LocalPageTable headings={["代理引用", "凭据处理"]}>{preview.credentials.map(c => <tr key={c.proxyId}><td>{c.proxyId}</td><td>{credentialState[c.state]}</td></tr>)}</LocalPageTable><p>可解密不代表已联网或跨用户登录可恢复；没有回显认证值。</p></>}
      <p className="local-page-note">只处理包内明确环境，包外环境保留；原 ID、seed 和设备参数不变。覆盖撤回备份后改动，不自动重开会话。</p><p>包 SHA-256：<code>{preview.archiveSha256}</code></p><p className="subtle-text">有效至 {preview.expiresAt}；正式提交前重验包和配置。摘要不是来源签名。</p>
      {preview.conflictCount > 0 && <p role="alert">存在身份或共享引用冲突。先处理当前配置并重新预检，不生成新 seed 绕过。</p>}{preview.missingKernelCount > 0 && <p role="alert">请补齐同版本、同校验值的精确内核后重新预检。</p>}{message && <p role="alert">{message}</p>}
    </LocalPageWindow>}
    <NativeRestoreExecution application={application} workspace={workspace} preview={preview} confirmationRequested={confirmationRequested} onLockChange={locked => { executionLock.current = locked; setExecutionLocked(locked); if (locked) setWindow(undefined); }} onConsumed={previewId => { if (owned.current.previewId !== previewId) return; generation.current++; owned.current = { previewId: "", sourceToken: "" }; setSource(undefined); setPreview(undefined); setPage(undefined); setWindow(undefined); }} />
  </div>;
}
