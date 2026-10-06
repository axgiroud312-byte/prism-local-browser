import { useEffect, useRef, useState } from "react";
import { LoaderCircle } from "lucide-react";
import type { Environment } from "../domain";
import { mergeOperation, operationIsTerminal, type ApplicationResult, type ApplicationService, type CookieCommitRequest, type CookieImportPreview, type CookieItemResult, type Operation, type WorkspaceView } from "../application/contract";
import { cookieStartupAllowed, cookieWriteAllowed, currentCookieOperation } from "../application/cookie-import";
import { EnvironmentTaskResult, EnvironmentWindowFrame } from "./EnvironmentDialogParts";
import "./native-cookie.css";

const errorText = (result: ApplicationResult<unknown> | undefined) => !result ? "当前桌面服务不支持此操作，没有模拟成功。" : result.ok ? "" : `${result.error.code}：${result.error.message}`;
const itemLabel: Record<CookieItemResult["status"], string> = { pending: "待处理", verified: "写后读回通过", "already-matched": "已匹配，未重写", failed: "未核对通过", unknown: "结果未知（可能已写入）", expired: "过期跳过", cancelled: "取消，未发送" };
// These are pre-acceptance refusals in the existing Cookie service. An unknown
// transport/storage/receipt error is NOT evidence that no write was accepted.
const commitWasRefused = (result: ApplicationResult<unknown> | undefined) => result && !result.ok && !result.operationId && ["VALIDATION_FAILED", "PREVIEW_EXPIRED", "REVISION_CONFLICT", "PROFILE_BUSY", "COOKIE_SESSION_REQUIRED", "COOKIE_SESSION_CHANGED", "COOKIE_CONTROL_UNAVAILABLE", "COOKIE_SELECTION_INVALID", "COOKIE_DUPLICATE_SELECTED"].includes(result.error.code);

export function NativeCookieImport({ application, workspace, environment, onClose }: { application: ApplicationService; workspace: WorkspaceView; environment: Environment; onClose: () => void }) {
  const [text, setText] = useState("");
  const [showText, setShowText] = useState(false);
  const [fileOpen, setFileOpen] = useState(false);
  const [unconfirmedCommit, setUnconfirmedCommit] = useState(false);
  const [preview, setPreview] = useState<CookieImportPreview>();
  const [selected, setSelected] = useState<number[]>([]);
  const [policy, setPolicy] = useState<"merge" | "replace-all">("merge");
  const [confirmClear, setConfirmClear] = useState(false);
  const [confirmDirect, setConfirmDirect] = useState(false);
  const [message, setMessage] = useState("");
  const [busy, setBusy] = useState(false);
  const [operation, setOperation] = useState<Operation | undefined>(() => currentCookieOperation(undefined, workspace.cookieOperations ?? [], environment.id));
  const [startOperation, setStartOperation] = useState<Operation>();
  const generation = useRef(0);
  const mounted = useRef(true);
  const fileInput = useRef<HTMLInputElement>(null);
  const fileTrigger = useRef<HTMLButtonElement>(null);
  const previousFileOpen = useRef(false);
  const modal = useRef<HTMLDivElement>(null);
  const commitRetry = useRef<{ key: string; request: CookieCommitRequest } | undefined>(undefined);
  const closeLocked = unconfirmedCommit || busy && !!commitRetry.current;
  const closeRef = useRef(onClose); closeRef.current = () => { if (fileOpen) setFileOpen(false); else if (!closeLocked) onClose(); };
  const session = workspace.runtimeSessions?.[environment.id];
  const currentEnvironment = workspace.state.environments.find(record => record.id === environment.id) ?? environment;
  const canWrite = cookieWriteAllowed(session);
  const canStart = cookieStartupAllowed(currentEnvironment.proxyId, confirmDirect, session);
  const active = !!operation && !operationIsTerminal(operation);
  const starting = !!startOperation && !operationIsTerminal(startOperation);
  const locked = busy || active || starting || unconfirmedCommit;
  const report = operation?.cookieReport;

  useEffect(() => {
    mounted.current = true;
    void application.refresh?.();
    const before = document.activeElement as HTMLElement | null;
    const overflow = document.body.style.overflow; document.body.style.overflow = "hidden";
    modal.current?.querySelector<HTMLElement>("button")?.focus();
    const key = (event: KeyboardEvent) => {
      if (event.key === "Escape") { event.preventDefault(); event.stopPropagation(); closeRef.current(); }
      if ((event.ctrlKey || event.metaKey) && event.key === "k") { event.preventDefault(); event.stopPropagation(); }
      if (event.key !== "Tab") return;
      const buttons = [...(modal.current?.querySelectorAll<HTMLElement>('button:not(:disabled), input:not(:disabled), textarea:not(:disabled), select:not(:disabled), summary, [tabindex="0"]') ?? [])].filter(element => element.offsetParent !== null);
      const first = buttons[0], last = buttons.at(-1);
      if (!modal.current?.contains(document.activeElement)) { event.preventDefault(); (event.shiftKey ? last : first)?.focus(); return; }
      if (event.shiftKey && document.activeElement === first) { event.preventDefault(); last?.focus(); }
      if (!event.shiftKey && document.activeElement === last) { event.preventDefault(); first?.focus(); }
    };
    document.addEventListener("keydown", key);
    return () => {
      mounted.current = false; generation.current++;
      void application.discardCookieImport?.("");
      if (fileInput.current) fileInput.current.value = "";
      document.body.style.overflow = overflow; document.removeEventListener("keydown", key); if (before?.isConnected) before.focus();
    };
  }, [application]);

  useEffect(() => {
    if (fileOpen) modal.current?.querySelector<HTMLElement>("button:not(:disabled)")?.focus();
    else if (previousFileOpen.current) fileTrigger.current?.focus();
    previousFileOpen.current = fileOpen;
  }, [fileOpen]);

  useEffect(() => {
    setOperation(previous => currentCookieOperation(previous, workspace.cookieOperations ?? [], environment.id, preview?.previewId));
  }, [workspace.cookieOperations, environment.id, preview?.previewId]);

  useEffect(() => {
    if (!active && !starting) return;
    let cancelled = false, reading = false;
    const poll = async () => {
      if (reading) return; reading = true;
      try {
        const tracked = starting ? startOperation : operation;
        if (!tracked) return;
        const result = await application.getOperation(tracked.id);
        if (cancelled) return;
        if (!result.ok) { setMessage(errorText(result)); return; }
        if (starting) setStartOperation(previous => mergeOperation(previous, result.data));
        else setOperation(previous => mergeOperation(previous, result.data));
        if (operationIsTerminal(result.data)) { await application.refresh?.(); if (starting && result.data.state !== "completed") setMessage(result.data.error ? `${result.data.error.code}：${result.data.error.message}` : "指定环境未启动，未写Cookie。"); }
      } finally { reading = false; }
    };
    const timer = setInterval(() => { void poll(); }, 700); void poll();
    return () => { cancelled = true; clearInterval(timer); };
  }, [application, operation?.id, startOperation?.id, active, starting]);

  function invalidate() {
    generation.current++; setPreview(undefined); setSelected([]); setMessage(""); setConfirmClear(false); commitRetry.current = undefined; setUnconfirmedCommit(false);
    void application.discardCookieImport?.("");
  }

  async function parse() {
    const requestGeneration = ++generation.current;
    setBusy(true); setMessage("");
    const result = await application.parseCookieImport?.(environment.id, text);
    if (!mounted.current || requestGeneration !== generation.current) return;
    setBusy(false);
    if (!result?.ok) { setMessage(errorText(result)); return; }
    setPreview(result.data); setText(""); setShowText(false); if (fileInput.current) fileInput.current.value = "";
    setSelected(result.data.rows.filter(row => !row.errorCode && !row.expired && !row.conflict).map(row => row.index));
    setOperation(undefined); setPolicy("merge"); setConfirmClear(false); commitRetry.current = undefined;
  }

  async function start() {
    if (!preview || locked || !canStart) return;
    setBusy(true); setMessage("");
    const result = await application.startRuntime?.({ environmentId: environment.id, expectedRevision: preview.expectedRevision, purpose: "cookie-import", networkPolicy: currentEnvironment.proxyId ? "proxy" : "direct", requestId: crypto.randomUUID() });
    if (!mounted.current) return; setBusy(false);
    if (result?.ok) setStartOperation(result.data.operation); else setMessage(errorText(result));
  }

  async function commit(retryOnly = false, verifyOriginal = false) {
    if (verifyOriginal && commitRetry.current) {
      setBusy(true); setMessage("");
      const result = await application.commitCookieImport?.(commitRetry.current.request);
      if (!mounted.current) return; setBusy(false);
      if (result?.ok) { commitRetry.current = undefined; setUnconfirmedCommit(false); setOperation(previous => previous?.id === result.data.operation.id ? mergeOperation(previous, result.data.operation) : result.data.operation); setConfirmClear(false); }
      else { setMessage(errorText(result)); if (commitWasRefused(result)) { commitRetry.current = undefined; setUnconfirmedCommit(false); } }
      return;
    }
    if (!preview || !canWrite || !session || !selected.length || policy === "replace-all" && !confirmClear && !retryOnly) return;
    if (retryOnly && (!report || report.previewId !== preview.previewId)) return;
    if (!application.commitCookieImport) { setMessage(errorText(undefined)); return; }
    const rows = retryOnly && report ? report.items.filter(item => selected.includes(item.index) && !["verified", "already-matched", "expired"].includes(item.status)).map(item => item.index) : selected;
    if (!rows.length) return;
    const request = { previewId: preview.previewId, environmentId: environment.id, expectedRevision: preview.expectedRevision, sessionId: session.sessionId, selectedRows: rows, policy: retryOnly ? "merge" as const : policy };
    const key = JSON.stringify(request);
    if (commitRetry.current?.key !== key) commitRetry.current = { key, request: { ...request, requestId: crypto.randomUUID() } };
    setBusy(true); setMessage("");
    const result = await application.commitCookieImport?.(commitRetry.current.request);
    if (!mounted.current) return; setBusy(false);
    if (result?.ok) { commitRetry.current = undefined; setOperation(previous => previous?.id === result.data.operation.id ? mergeOperation(previous, result.data.operation) : result.data.operation); setText(""); setConfirmClear(false); if (retryOnly) setPolicy("merge"); }
    else { setMessage(errorText(result)); if (commitWasRefused(result)) { commitRetry.current = undefined; setUnconfirmedCommit(false); } else setUnconfirmedCommit(true); }
  }

  async function cancel() { if (!operation) return; const result = await application.cancelOperation(operation.id); if (!mounted.current) return; if (result.ok) setOperation(previous => mergeOperation(previous, result.data)); else setMessage(errorText(result)); }

  const readFile = async (file?: File) => {
    if (!file) return; invalidate(); setText(""); const current = generation.current;
    if (file.size > 2 * 1024 * 1024) { setMessage("Cookie文件超过2MiB，未读取。"); if (fileInput.current) fileInput.current.value = ""; return; }
    try { const content = new TextDecoder("utf-8", { fatal: true }).decode(await file.arrayBuffer()); if (mounted.current && current === generation.current) { setText(content); setFileOpen(false); } } catch { if (mounted.current && current === generation.current) setMessage("Cookie文件无法读取或不是有效UTF-8，没有写入。"); }
  };
  const retryPossible = preview && report?.previewId === preview.previewId && operation && operationIsTerminal(operation) && report.items.some(item => selected.includes(item.index) && ["failed", "unknown", "cancelled"].includes(item.status));
  return <div className="overlay modal-overlay env34-cookie-overlay">
    {fileOpen ? <EnvironmentWindowFrame title="读取Cookie文件" titleId="native-cookie-title" dialogRef={modal} onClose={() => setFileOpen(false)} width={520} height={375.84} className="env34-cookie-file" footer={<button className="button" onClick={() => setFileOpen(false)}>返回文本导入</button>}>
      <div className="env34-cookie-file-zone"><strong>选择一个 JSON / Netscape UTF-8 文本文件</strong><input ref={fileInput} type="file" accept=".json,.txt" aria-label="读取Cookie文件" disabled={locked} onChange={event => { void readFile(event.target.files?.[0]); }} /></div>
      <p>只读取所选文本，不上传、不写入。2MiB 是输入安全边界，不是环境或Cookie数量配额。此入口不支持 ZIP。</p><p>文件读入后返回同一文本草稿，再解析并核对明确的目标环境。</p>{message && <p className="env34-error" role="alert">{message}</p>}
    </EnvironmentWindowFrame> : <EnvironmentWindowFrame title="导入真实环境 Cookie" titleId="native-cookie-title" dialogRef={modal} onClose={() => closeRef.current()} closeLabel="关闭Cookie导入" closeDisabled={closeLocked} width={1050} height={preview || report ? 609.72 : 551.72} className="env34-cookie-window" footer={<>
      <button className="button" disabled={closeLocked} onClick={() => closeRef.current()}>关闭</button>
      {active && <button className="button" disabled={operation?.cancelRequested || operation?.persistencePending} onClick={() => { void cancel(); }}>取消剩余条目（已写入不撤销）</button>}
      {unconfirmedCommit ? <button className="button primary" disabled={busy} onClick={() => { void commit(false, true); }}>核实原导入请求（不改变范围或清空策略）</button> : retryPossible ? <button className="button primary" disabled={locked || !canWrite} onClick={() => { void commit(true); }}>只重试所选未核对通过项（不重清、不改其他键）</button> : preview && !report ? <button className="button primary" disabled={locked || !canWrite || !selected.length || policy === "replace-all" && !confirmClear} onClick={() => { void commit(); }}>确认导入 {selected.length} 条并读回核对</button> : !preview && <button className="button primary" disabled={locked || !text.trim()} onClick={() => { void parse(); }}>{busy && <LoaderCircle size={15} className="spin" />}解析预览（不写入）</button>}
    </>}>
      <div className="native-cookie-body">
        <p><strong>目标：{currentEnvironment.name}</strong> · ID：{environment.id}{preview && <> · 修订 {preview.expectedRevision}</>}</p>
        <div className="env34-cookie-input-grid"><div><label>Cookie 内容<textarea aria-label="Cookie 内容" className={!showText ? "env34-cookie-secret" : ""} autoComplete="off" spellCheck={false} rows={7} value={text} disabled={locked} placeholder="粘贴完整Cookie数组或Netscape文本" onChange={event => { invalidate(); setText(event.target.value); }} /></label><div className="env34-inline"><label className="env34-inline"><input type="checkbox" checked={showText} onChange={event => setShowText(event.target.checked)} />临时显示输入（注意屏幕隐私）</label><button ref={fileTrigger} className="button compact" disabled={locked} onClick={() => setFileOpen(true)}>选择文件</button></div></div><aside><h3>导入说明</h3><p>完整 JSON Cookie 数组 / Netscape 7列文本；不接受缺少域名的请求头 Cookie。</p><p>空值和 session 语义保留；过期行不续期。值不显示在预览、结果和日志中；解析成功后清除原文。</p><p>15分钟短期预览；关闭清除输入和预览，不撤销已发送写入。仅导入Cookie不等于恢复全部登录。</p><p>native 服务结果；合成 bridge / 页面不能替代真实桌面验收。</p></aside></div>
        {message && <p className="native-cookie-error" role="alert">{message}</p>}
        {unconfirmedCommit && <p role="status">原导入受理尚未确认，可能已有写入。输入、所选行和策略已冻结，只核实原 requestId；核实前不能关闭并丢弃原请求。</p>}
        {preview && <>
          <p>共{preview.total}条 · 有效{preview.validCount} · 错误{preview.errorCount} · 过期{preview.expiredCount} · 输入同键冲突{preview.conflictCount} · 现存冲突{preview.existingConflictCount === undefined ? "未知（提交时重检）" : preview.existingConflictCount}</p>
          {preview.observationError && <p>{preview.observationError.message}</p>}
          <p>域名：{[...new Set(preview.rows.map(row => row.domain).filter(Boolean))].join("、") || "尚无有效域名"}。输入同键冲突须只选一条；过期行不会续期或删除旧Cookie。</p>
          <div className="native-cookie-table env34-table" tabIndex={0}><table><thead><tr><th>选择</th><th>名称 / 域名 / 路径</th><th>属性</th><th>状态</th></tr></thead><tbody>{preview.rows.map(row => <tr key={row.index}>
            <td><input type="checkbox" aria-label={`选择第${row.index}行`} disabled={locked || !!row.errorCode || row.expired} checked={selected.includes(row.index)} onChange={event => setSelected(previous => event.target.checked ? [...previous, row.index] : previous.filter(index => index !== row.index))} /></td>
            <td>#{row.index} {row.name}<br />{row.domain} {row.path}</td>
            <td>{row.hostOnly ? "仅当前host" : "域Cookie，含子域"}<br />{row.session ? "session" : row.expires === undefined ? "—" : `Unix秒 ${row.expires}`}<br />{row.secure ? "Secure " : ""}{row.httpOnly ? "HttpOnly " : ""}{row.sameSite ?? "SameSite未指定"}{row.partitionKey && <><br />分区 {row.partitionKey.topLevelSite} · 跨站祖先 {String(row.partitionKey.hasCrossSiteAncestor)}</>}</td>
            <td>{row.errorCode ? `${row.errorCode}：${row.message}` : row.expired ? "已过期，不导入" : row.conflict ? "输入冲突，请只选一条" : row.existingConflict ? "现存同键，将替换" : "可导入"}{!row.errorCode && row.message && <><br />{row.message}</>}</td>
          </tr>)}</tbody></table></div>
          {!canWrite && <div className="info-strip native-cookie-start">
            <p>写入前需要指定环境的受控会话。启动只打开空白页，不恢复旧标签或保存网址；seed、数据目录、内核和代理绑定不变。</p>
            {currentEnvironment.proxyId ? <p>启动仍使用已绑定代理，核对本次实际隔离资源、同通道前检及进程树；保护缺失、失败或未知时拒绝，不会切为直连。</p> : <label className="native-proxy-inline"><input type="checkbox" checked={confirmDirect} disabled={locked} onChange={event => setConfirmDirect(event.target.checked)} />我确认：这个环境未绑定代理，启动会使用本机直连网络。</label>}
            <button className="button" disabled={locked || !canStart} onClick={() => { void start(); }}>{starting ? "正在启动，尚未写入…" : "明确启动空白环境（还不写Cookie）"}</button>
          </div>}
          <div className="native-proxy-actions">
            <label className="native-proxy-inline"><input type="radio" name="native-cookie-policy" checked={policy === "merge"} disabled={locked} onChange={() => { setPolicy("merge"); setConfirmClear(false); }} />合并，仅替换完全相同键（默认）</label>
            <label className="native-proxy-inline"><input type="radio" name="native-cookie-policy" checked={policy === "replace-all"} disabled={locked || !!report} onChange={() => setPolicy("replace-all")} />先清空本环境全部Cookie</label>
          </div>
          {policy === "replace-all" && <label className="native-proxy-inline native-cookie-error"><input type="checkbox" checked={confirmClear} disabled={locked} onChange={event => setConfirmClear(event.target.checked)} />确认删除此环境全部Cookie（含未选条目及分区），仅导入所选行；无法撤销，失败不自动恢复。</label>}
        </>}
        {report && <EnvironmentTaskResult title={`本次实际结果 · ${operation?.persistencePending ? "观测待保存，不能重复写入" : operation?.state === "completed" ? "全部核对通过" : active ? "进行中" : "未全部成功"}`} total={report.items.length} completed={report.verifiedCount} failed={report.failedCount}>
          <p>读回通过{report.verifiedCount}（新写{report.writtenCount} / 已匹配未重写{report.alreadyMatchedCount}） · 未通过{report.failedCount}（结果未知{report.unconfirmedCount}） · 跳过{report.skippedCount}</p>
          {report.policy === "replace-all" && <p>清空结果：{report.clearState === "verified-empty" ? "已读回确认空集合" : "未确认或未发送，可能已有副作用"}；重试只合并失败项，不自动再次清空。</p>}
          {operation?.error && <p className="native-cookie-error">{operation.error.code}：{operation.error.message}</p>}
          <ul>{report.items.map(item => <li key={item.index}>#{item.index} {item.name} · {item.domain}{item.path}：{itemLabel[item.status]}{item.message && ` — ${item.message}`}</li>)}</ul>
        </EnvironmentTaskResult>}
        <p>关闭不会撤销已发送的写入；后台任务可继续，重新打开可查看安全结果。预览过期后须重新读取输入。</p>
      </div>
    </EnvironmentWindowFrame>}
  </div>;
}
