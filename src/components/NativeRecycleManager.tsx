import { useEffect, useRef, useState } from "react";
import { X } from "lucide-react";
import { mergeOperation, operationIsTerminal, type ApplicationResult, type ApplicationService, type NativeRecycleAction, type NativeRecyclePage, type NativeRecyclePageRequest, type Operation, type WorkspaceView } from "../application/contract";
import { confirmsRecycleRequest } from "../application/recycle-model";
import "./native-proxy.css";
import "./native-cookie.css";

const actions = { remove: "移入回收区", restore: "找回原环境", purge: "永久删除" };
const states: Record<string, string> = { pending: "未执行", recycled: "已回收", restored: "已找回", purged: "已永久删除", failed: "未完成，原状态已核对", protected: "结果待核对，保护保持" };
const errorText = (r: ApplicationResult<unknown> | undefined) => !r ? "本机回收服务不可用。" : r.ok ? "" : `${r.error.code}：${r.error.message}`;

export function NativeRecycleManager({ application, workspace, selectedIds, onClose }: { application: ApplicationService; workspace: WorkspaceView; selectedIds?: string[]; onClose(): void }) {
  const [page, setPage] = useState<NativeRecyclePage>();
  const [operation, setOperation] = useState<Operation>();
  const [pending, setPending] = useState(() => application.getPendingRecycle?.());
  const [selected, setSelected] = useState<string[]>([]), [confirm, setConfirm] = useState(false);
  const [busy, setBusy] = useState(false), [message, setMessage] = useState("");
  const modal = useRef<HTMLDivElement>(null), mounted = useRef(false), flight = useRef(false), generation = useRef(0), started = useRef(false);
  const target = useRef<NativeRecyclePageRequest>({ offset: 0, pageSize: 25 });
  const submitted = useRef(pending?.request);
  const observed = useRef<Operation | undefined>(undefined);
  const closeRef = useRef(onClose); closeRef.current = onClose;
  const active = !!operation && !operationIsTerminal(operation);
  const locked = busy || !!pending || !!workspace.recycleMaintenance || active;
  const isList = !!page && !page.preview && !page.operation;

  useEffect(() => {
    mounted.current = true;
    const previous = document.activeElement as HTMLElement | null, overflow = document.body.style.overflow;
    document.body.style.overflow = "hidden"; modal.current?.querySelector("button")?.focus();
    const key = (event: KeyboardEvent) => {
      if (event.key === "Escape") { event.preventDefault(); event.stopPropagation(); closeRef.current(); }
      if ((event.ctrlKey || event.metaKey) && event.key === "k") { event.preventDefault(); event.stopPropagation(); }
      if (event.key !== "Tab") return;
      const nodes = [...(modal.current?.querySelectorAll<HTMLElement>('button:not(:disabled),input:not(:disabled),[tabindex="0"]') ?? [])].filter(n => n.offsetParent !== null);
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
    return () => { mounted.current = false; document.body.style.overflow = overflow; document.removeEventListener("keydown", key); previous?.focus(); };
  }, [application]);

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

  return <div className="overlay modal-overlay"><div className="modal wide-modal native-cookie-modal" role="dialog" aria-modal="true" aria-labelledby="recycle-title" ref={modal}>
    <div className="modal-header"><h2 id="recycle-title">本机回收区</h2><button className="icon-button" aria-label="关闭回收区" onClick={onClose}><X size={18} /></button></div>
    <div className="modal-body">
      <p>移除后保留原 ID、seed、设备档案、内核绑定和浏览数据。名称仍由回收项占用，找回使用原身份。</p>
      <button className="button" disabled={busy} onClick={() => void load({ offset: 0, pageSize: 25 })}>查看回收列表</button>
      {pending && <p role="status">原请求 {pending.request.requestId} 受理尚待核实。<button className="button" disabled={busy} onClick={() => void commit()}>核实原请求</button></p>}
      {message && <p role="alert">{message}</p>}
      {busy && <p role="status">正在读取或提交…</p>}
      {page?.preview && <section aria-label="回收影响确认"><h3>{actions[page.preview.action]} · {page.total} 项</h3><p>{page.preview.dataCount} 项包含实际浏览目录；{page.preview.backupCount} 项有成功备份记录（未重新核验备份文件）。</p>
        {page.preview.action === "purge" ? <p>永久删除本次选中的回收配置和数据，无法从回收区找回。既有备份及恢复历史副本不在本次删除范围内。删除开始后会完成当前项的核对，取消只阻止后续项。</p> : <p>运行中或退出未确认的环境不能移除；任何未知目录都不会被覆盖。</p>}
        <label><input type="checkbox" disabled={locked} checked={confirm} onChange={e => setConfirm(e.target.checked)} />已核对下方具体 ID，确认{actions[page.preview.action]}全部 {page.total} 项。</label>
        <button className={`button ${page.preview.action === "purge" ? "danger" : "primary"}`} disabled={locked || !confirm} onClick={() => void commit()}>{actions[page.preview.action]}</button>
      </section>}
      {operation?.recycleReport && <section aria-label="回收任务结果"><h3>{actions[operation.recycleReport.action]} · {operation.state}</h3><p>任务 {operation.id}</p><p>完成 {operation.recycleReport.completed} · 失败 {operation.recycleReport.failed} · 未执行 {operation.recycleReport.notExecuted}</p>{operation.error && <p role="alert">{operation.error.code}：{operation.error.message}</p>}{operation.recycleReport.protected && <p>维护保护保持。请释放目录占用、修复空间或权限后，核对原任务；保留原日志与目录。</p>}{active && <button className="button" disabled={busy || operation.cancelRequested || operation.stage === "workspace-recovery"} onClick={() => void taskAction(false)}>取消后续项</button>}{operation.persistencePending && <button className="button" disabled={busy || operation.state === "running"} onClick={() => void taskAction(true)}>重试核对原任务</button>}</section>}
      {page && <><p>共 {page.total} 项 · 本页 {page.items.length} 项</p><div className="table-scroll"><table><thead><tr>{isList && <th>选择</th>}<th>环境 / ID</th><th>身份与内核</th><th>数据</th><th>结果</th></tr></thead><tbody>{page.items.map(item => <tr key={item.id}>{isList && <td><input type="checkbox" aria-label={`选择回收项 ${item.name}`} disabled={locked} checked={selected.includes(item.id)} onChange={e => setSelected(old => e.target.checked ? [...old, item.id] : old.filter(id => id !== item.id))} /></td>}<td>{item.name}<br /><small>{item.environmentId}</small></td><td>seed {item.seed}<br /><small>{item.kernelId}</small></td><td>{item.dataPresent ? "包含浏览数据" : "未初始化数据"}<br />{item.backupRecorded ? "有备份记录" : "无备份记录"}</td><td>{states[item.state] ?? item.state}{item.error && <p>{item.error.message}</p>}</td></tr>)}</tbody></table></div>
        <button className="button" disabled={busy || page.offset === 0} onClick={() => void load({ ...target.current, offset: Math.max(0, page.offset - page.pageSize) })}>上一页</button><button className="button" disabled={busy || page.offset + page.items.length >= page.total} onClick={() => void load({ ...target.current, offset: page.offset + page.pageSize })}>下一页</button>
        {isList && <><button className="button" disabled={locked || !selected.length} onClick={() => void preview("restore", selected)}>找回选中项（{selected.length}）</button><button className="button danger" disabled={locked || !selected.length} onClick={() => void preview("purge", selected)}>查看永久删除影响</button></>}
      </>}
      {!!workspace.recycleOperations?.length && <h3>最近任务</h3>}{workspace.recycleOperations?.map(op => <p key={op.id}>{op.id} · {op.state}<button className="button" disabled={busy} onClick={() => void load({ operationId: op.id, offset: 0, pageSize: 25 })}>读取逐项结果</button></p>)}
    </div>
  </div></div>;
}
