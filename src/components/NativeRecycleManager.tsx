import { useEffect, useRef, useState } from "react";
import { RotateCcw, Trash2, X } from "lucide-react";
import { mergeOperation, operationIsTerminal, type ApplicationResult, type ApplicationService, type NativeRecycleAction, type NativeRecyclePage, type NativeRecyclePageRequest, type Operation, type WorkspaceView } from "../application/contract";
import { confirmsRecycleRequest } from "../application/recycle-model";
import { EnvironmentConfirmation, EnvironmentTaskResult, EnvironmentWindowFrame } from "./EnvironmentDialogParts";
import { lockBodyScroll, maintainModalFocus, ownsTopModal, restoreModalFocus } from "./modal-lifecycle";
import "./environment-recycle.css";

const actions = { remove: "移入回收区", restore: "找回原环境", purge: "永久删除" };
const states: Record<string, string> = { pending: "未执行", recycled: "已回收", restored: "已找回", purged: "已永久删除", failed: "未完成，原状态已核对", protected: "结果待核对，保护保持" };
const errorText = (r: ApplicationResult<unknown> | undefined) => !r ? "本机回收服务不可用。" : r.ok ? "" : `${r.error.code}：${r.error.message}`;

export function NativeRecycleManager({ application, workspace, selectedIds, onClose }: { application: ApplicationService; workspace: WorkspaceView; selectedIds?: string[]; onClose(): void }) {
  const [page, setPage] = useState<NativeRecyclePage>();
  const [listPage, setListPage] = useState<NativeRecyclePage>();
  const [operation, setOperation] = useState<Operation>();
  const [pending, setPending] = useState(() => application.getPendingRecycle?.());
  const [selected, setSelected] = useState<string[]>([]), [confirm, setConfirm] = useState(false);
  const [busy, setBusy] = useState(false), [message, setMessage] = useState("");
  const [resultsExpanded, setResultsExpanded] = useState(false);
  const modal = useRef<HTMLDivElement>(null), mounted = useRef(false), flight = useRef(false), generation = useRef(0), started = useRef(false);
  const target = useRef<NativeRecyclePageRequest>({ offset: 0, pageSize: 25 });
  const submitted = useRef(pending?.request);
  const observed = useRef<Operation | undefined>(undefined);
  const closeRef = useRef(onClose); closeRef.current = onClose;
  const active = !!operation && !operationIsTerminal(operation);
  const locked = busy || !!pending || !!workspace.recycleMaintenance || active;
  const isList = !!page && !page.preview && !page.operation;
  const viewKey = page?.preview ? `preview:${page.preview.previewId}` : operation ? `operation:${operation.id}` : "list";
  const previousView = useRef(viewKey);

  useEffect(() => {
    mounted.current = true;
    const previous = document.activeElement as HTMLElement | null, releaseScroll = lockBodyScroll();
    const releaseFocus = maintainModalFocus(() => modal.current);
    if (ownsTopModal(modal.current)) modal.current?.querySelector("button")?.focus();
    const key = (event: KeyboardEvent) => {
      if (event.defaultPrevented || !ownsTopModal(modal.current)) return;
      if (event.key === "Escape") { event.preventDefault(); event.stopPropagation(); closeRef.current(); }
      if ((event.ctrlKey || event.metaKey) && event.key === "k") { event.preventDefault(); event.stopPropagation(); }
      if (event.key !== "Tab") return;
      const nodes = [...(modal.current?.querySelectorAll<HTMLElement>('button:not(:disabled),input:not(:disabled),select:not(:disabled),summary,[tabindex="0"]') ?? [])].filter(n => n.offsetParent !== null);
      const first = nodes[0], last = nodes.at(-1);
      if (!modal.current?.contains(document.activeElement)) { event.preventDefault(); (event.shiftKey ? last : first)?.focus(); }
      else if (event.shiftKey && document.activeElement === first) { event.preventDefault(); last?.focus(); }
      else if (!event.shiftKey && document.activeElement === last) { event.preventDefault(); first?.focus(); }
    };
    document.addEventListener("keydown", key);
    if (!started.current) {
      started.current = true;
      const original = application.getPendingRecycle?.();
      const id = original?.operationId ?? workspace.recycleMaintenance?.id;
      if (id) void load({ operationId: id, offset: 0, pageSize: 25 });
      else if (!original && selectedIds?.length) void preview("remove", selectedIds);
      else void load({ offset: 0, pageSize: 25 });
    }
    return () => { mounted.current = false; releaseScroll(); releaseFocus(); document.removeEventListener("keydown", key); restoreModalFocus(previous); };
  }, [application]);

  useEffect(() => {
    if (!ownsTopModal(modal.current)) return;
    if (previousView.current !== viewKey) modal.current?.querySelector<HTMLElement>('button:not(:disabled), input:not(:disabled), summary')?.focus();
    previousView.current = viewKey;
  }, [viewKey]);

  function observe(op: Operation) {
    if (target.current.operationId === op.id) { observed.current = mergeOperation(observed.current?.id === op.id ? observed.current : undefined, op); setOperation(observed.current); }
    if (confirmsRecycleRequest(op, submitted.current)) { submitted.current = undefined; setConfirm(false); if (page?.preview) setPage(undefined); }
    setPending(application.getPendingRecycle?.());
  }
  useEffect(() => {
    setPending(application.getPendingRecycle?.());
    const original = submitted.current;
    if (original && application.wasRecycleNotAccepted?.(original.requestId)) { submitted.current = undefined; setConfirm(false); }
    for (const op of workspace.recycleOperations ?? []) observe(op);
  }, [workspace]);

  async function load(request: NativeRecyclePageRequest) {
    const current = ++generation.current; target.current = request; setBusy(true); setPage(undefined); setSelected([]); setConfirm(false); setMessage("");
    observed.current = observed.current?.id === request.operationId ? observed.current : undefined; setOperation(observed.current);
    const result = await application.readRecyclePage?.(request);
    if (!mounted.current || current !== generation.current) return;
    setBusy(false);
    if (!result?.ok) { setMessage(errorText(result)); return; }
    if (!result.data.preview && !result.data.operation) setListPage(result.data);
    if (!result.data.operation || result.data.operation.recycleReport!.sequence >= (observed.current?.recycleReport?.sequence ?? 0)) setPage(result.data);
    if (result.data.operation) observe(result.data.operation);
  }
  async function preview(action: NativeRecycleAction, ids: string[]) {
    if (flight.current || application.getPendingRecycle?.()) return;
    const current = ++generation.current; flight.current = true; setBusy(true); setPage(undefined); observed.current = undefined; setOperation(undefined); setConfirm(false); setMessage("");
    const result = await application.previewRecycle?.(action, [...ids]);
    flight.current = false;
    if (!mounted.current || current !== generation.current) return;
    setBusy(false);
    if (!result?.ok) { setMessage(errorText(result)); return; }
    target.current = { previewId: result.data.preview!.previewId, offset: result.data.offset, pageSize: result.data.pageSize }; setPage(result.data);
  }
  async function commit() {
    const original = application.getPendingRecycle?.();
    if (flight.current || !original && (locked || !page?.preview || !confirm)) return;
    const request = original?.request ?? { previewId: page!.preview!.previewId, confirm: true, requestId: crypto.randomUUID() };
    submitted.current = request; flight.current = true; setBusy(true); setMessage("");
    const result = await application.commitRecycle?.(request);
    flight.current = false;
    if (!mounted.current) return;
    setBusy(false); setPending(application.getPendingRecycle?.());
    if (result?.ok) { observe(result.data.operation); void load({ operationId: result.data.operation.id, offset: 0, pageSize: 25 }); }
    else { setMessage(errorText(result)); if (result?.operationId) void load({ operationId: result.operationId, offset: 0, pageSize: 25 }); }
  }
  async function taskAction(recover: boolean) {
    if (flight.current || !operation) return;
    const id = operation.id; flight.current = true; setBusy(true); setMessage("");
    const result = recover ? await application.recoverRecycle?.(id) : await application.cancelOperation(id);
    flight.current = false; if (!mounted.current) return; setBusy(false);
    if (id !== target.current.operationId) return;
    if (result?.ok) observe(result.data); else setMessage(errorText(result));
  }
  const operationId = operation?.id ?? target.current.operationId;
  useEffect(() => {
    if (!operationId || operation && operationIsTerminal(operation) && page?.operation?.id === operationId && (page.operation.recycleReport?.sequence ?? 0) >= (operation.recycleReport?.sequence ?? 0)) return;
    let disposed = false, reading = false;
    const poll = async () => {
      if (reading) return; reading = true;
      try {
        const request = { ...target.current, operationId };
        const current = generation.current;
        const result = await application.readRecyclePage?.(request);
        if (disposed || !mounted.current || current !== generation.current || target.current.operationId !== operationId) return;
        if (result?.ok && result.data.operation) { if (result.data.operation.recycleReport!.sequence >= (observed.current?.recycleReport?.sequence ?? 0)) setPage(result.data); observe(result.data.operation); if (operationIsTerminal(result.data.operation)) await application.refresh?.(); }
        else setMessage(errorText(result));
      } finally { reading = false; }
    };
    void poll();
    if (operation && operationIsTerminal(operation)) return () => { disposed = true; };
    const timer = setInterval(() => void poll(), 1000);
    return () => { disposed = true; clearInterval(timer); };
  }, [application, operationId, active]);

  const notices = <>{pending && <p role="status">原请求 {pending.request.requestId} 受理尚待核实。<button className="button" disabled={busy} onClick={() => void commit()}>核实原请求</button></p>}{message && <p className="env34-error" role="alert">{message}</p>}{busy && <p role="status">正在读取或提交…</p>}</>;
  const pagination = page && <div className="env34-recycle-pagination"><span>共 {page.total} 项 · 本页 {page.items.length} 项</span><button className="button compact" disabled={busy || page.offset === 0} onClick={() => void load({ ...target.current, offset: Math.max(0, page.offset - page.pageSize) })}>上一页</button><button className="button compact" disabled={busy || page.offset + page.items.length >= page.total} onClick={() => void load({ ...target.current, offset: page.offset + page.pageSize })}>下一页</button></div>;
  function rows(records: NativeRecyclePage, selectable: boolean) {
    return <div className="env34-recycle-table" tabIndex={0}><table><thead><tr><th>选择</th><th>序号</th><th>环境名称 / 原身份</th><th>精确内核</th><th>数据 / 备份</th><th>删除时间 / 结果</th><th>操作</th></tr></thead><tbody>{records.items.map((item, index) => <tr key={item.id}><td>{selectable && <input type="checkbox" aria-label={`选择回收项 ${item.name}`} disabled={locked} checked={selected.includes(item.id)} onChange={e => setSelected(old => e.target.checked ? [...old, item.id] : old.filter(id => id !== item.id))} />}</td><td>{records.offset + index + 1}</td><td>{item.name}<details><summary>原身份详情</summary><p>{item.environmentId}<br />seed {item.seed} · 修订 {item.revision}</p></details></td><td>{item.kernelId}</td><td>{item.dataPresent ? "包含浏览数据" : "未初始化数据"}<br /><small>{item.backupRecorded ? "有备份记录" : "无备份记录"}</small></td><td>{item.removedAt ?? "—"}<br /><small>{states[item.state] ?? item.state}</small>{item.error && <p className="env34-error">{item.error.code}：{item.error.message}</p>}</td><td>{selectable && <button className="icon-button" aria-label={`永久删除影响 ${item.name}`} disabled={locked} onClick={() => void preview("purge", [item.id])}><Trash2 size={16} /></button>}</td></tr>)}</tbody></table></div>;
  }
  function list(records?: NativeRecyclePage, background = false) {
    return <div className={`env34-recycle-panel ${background ? "env34-recycle-background" : ""}`} ref={background ? undefined : modal} tabIndex={background ? undefined : -1} role={background ? undefined : "dialog"} aria-modal={background ? undefined : true} aria-labelledby={background ? undefined : "recycle-title"} aria-hidden={background || undefined} inert={background || undefined}>
      <header className="env34-recycle-header"><button className="button" onClick={onClose}>返回</button><h2 id={background ? undefined : "recycle-title"}>本机回收区</h2><span>找回沿用原 ID、seed、内核和浏览数据；名称仍由回收项占用。</span><button className="icon-button" aria-label="关闭回收区" onClick={onClose}><X size={16} /></button></header>
      <div className="env34-recycle-toolbar"><button className="button" disabled={busy} onClick={() => void load({ offset: 0, pageSize: 25 })}>查看回收列表</button><label>最近任务<select aria-label="最近回收任务" value={operation?.id ?? ""} disabled={busy} onChange={event => { if (event.target.value) void load({ operationId: event.target.value, offset: 0, pageSize: 25 }); }}><option value="">选择读取逐项结果</option>{workspace.recycleOperations?.map(op => <option key={op.id} value={op.id}>{op.state} · {op.id}</option>)}</select></label><span className="env34-recycle-toolbar-gap" /><button className="button primary" disabled={locked || !selected.length} onClick={() => void preview("restore", [...selected])}><RotateCcw size={15} />找回选中项（{selected.length}）</button><button className="button danger" disabled={locked || !selected.length} onClick={() => void preview("purge", [...selected])}><Trash2 size={15} />查看永久删除影响</button></div>
      {!background && notices}{records && rows(records, background || isList)}{!records && !busy && <p className="env34-recycle-empty">尚未读取回收列表，未更改任何记录。</p>}{records && !records.total && <p className="env34-recycle-empty">回收区为空</p>}{background ? <div className="env34-recycle-pagination">共 {records?.total ?? 0} 项 · 本页 {records?.items.length ?? 0} 项</div> : pagination}
    </div>;
  }
  const showConfirmation = !!page?.preview;
  const backedByList = !!listPage && (showConfirmation || !!operation);
  return <div className={`overlay env34-recycle-overlay ${showConfirmation || operation ? "modal-overlay" : ""} ${backedByList ? "env34-recycle-over-list" : ""}`}>
    {backedByList && <>{list(listPage, true)}<div className="env34-recycle-confirm-scrim" aria-hidden="true" /></>}
    {page?.preview?.action === "restore" ? <EnvironmentWindowFrame title="找回原环境" titleId="recycle-title" dialogRef={modal} onClose={onClose} closeLabel="关闭回收区" width={440} className="env34-recycle-restore" footer={<><button className="button" disabled={busy} onClick={() => void load({ offset: 0, pageSize: 25 })}>取消</button><button className="button primary" disabled={locked || !confirm} onClick={() => void commit()}>找回原环境</button></>}>
      <section aria-label="回收影响确认"><div className="env34-field"><span>恢复分组</span><div className="env34-recycle-original-group">沿用原配置分组（只读）</div></div><p>找回保留原 ID、seed、精确内核和数据引用，不走新建复制。接口未提供改分组，找回后可普通编辑。</p><details><summary>核对 {page.total} 个原环境</summary>{page.items.map(item => <p key={item.id}>{item.name} · {item.environmentId}<br />seed {item.seed}</p>)}{pagination}</details><label className="env34-inline"><input type="checkbox" disabled={locked} checked={confirm} onChange={event => setConfirm(event.target.checked)} />已核对下方具体 ID，确认找回原环境全部 {page.total} 项。</label></section>{notices}
    </EnvironmentWindowFrame> : page?.preview ? <EnvironmentConfirmation title={page.preview.action === "purge" ? "永久删除回收项" : "移入本机回收区"} titleId="recycle-title" dialogRef={modal} danger={page.preview.action === "purge"} onCancel={() => selectedIds?.length ? onClose() : void load({ offset: 0, pageSize: 25 })} onConfirm={() => void commit()} disabled={locked || !confirm} busy={busy} confirmLabel={actions[page.preview.action]}>
      <section aria-label="回收影响确认"><p>{page.preview.action === "purge" ? "永久删除不可恢复；既有备份和恢复副本不在删除范围。" : "移除后仍保留原身份和浏览数据，可在回收区找回。运行中或退出未确认时拒绝移除。"}</p><p>{page.total} 项 · 数据目录 {page.preview.dataCount} · 有备份记录 {page.preview.backupCount}（未重验文件）</p><details open={page.total === 1}><summary>核对具体 ID</summary>{page.items.map(item => <p key={item.id}>{item.name}<br />{item.environmentId} · seed {item.seed}</p>)}{pagination}</details><label className="env34-inline"><input type="checkbox" disabled={locked} checked={confirm} onChange={event => setConfirm(event.target.checked)} />已核对下方具体 ID，确认{actions[page.preview.action]}全部 {page.total} 项。</label></section>{notices}
    </EnvironmentConfirmation> : operation?.recycleReport ? <EnvironmentWindowFrame title="回收任务结果" titleId="recycle-title" dialogRef={modal} onClose={onClose} closeLabel="关闭回收区" width={400} className="env34-recycle-result" footer={<><button className="button" disabled={busy} onClick={() => void load({ offset: 0, pageSize: 25 })}>查看回收列表</button>{active && <button className="button" disabled={busy || operation.cancelRequested || operation.stage === "workspace-recovery"} onClick={() => void taskAction(false)}>取消后续项</button>}{operation.persistencePending && <button className="button primary" disabled={busy || operation.state === "running"} onClick={() => void taskAction(true)}>重试核对原任务</button>}</>}>
      <EnvironmentTaskResult title={`${actions[operation.recycleReport.action]} · ${operation.state}`} total={operation.total} completed={operation.recycleReport.completed} failed={operation.recycleReport.failed}><p>任务 {operation.id} · 未执行 {operation.recycleReport.notExecuted}</p>{operation.error && <p className="env34-error" role="alert">{operation.error.code}：{operation.error.message}</p>}{operation.recycleReport.protected && <p>维护保护保持。请释放目录占用、修复空间或权限后，核对原任务；保留原日志与目录。</p>}<p>取消只阻止后续项，已完成项不撤回。合成 bridge / 页面不等于真实目录验证。</p></EnvironmentTaskResult>{notices}{page && <details open={resultsExpanded} onToggle={event => setResultsExpanded(event.currentTarget.open)}><summary>逐项结果</summary>{page.items.map(item => <p key={item.id}>{item.name} · {item.environmentId}<br />{states[item.state]}{item.error && `：${item.error.message}`}</p>)}{pagination}</details>}
    </EnvironmentWindowFrame> : list(page)}
  </div>;
}
