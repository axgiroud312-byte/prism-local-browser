import { useEffect, useRef, useState } from "react";
import { mergeOperation, operationIsTerminal, type ApplicationService, type NativeRestorePreview, type Operation, type WorkspaceView } from "../application/contract";
import { confirmsRestoreRequest } from "../application/restore-model";

const stages: Record<string, string> = { accepted: "恢复已受理", revalidating: "再次核对原包", "stopping-environments": "正常停止受影响环境", prepared: "新目录与旧状态已准备", swapping: "切换目录", "db-committing": "提交配置事务", "db-committed": "确认新状态", finalized: "完整恢复完成", "rolled-back": "恢复未完成，原状态已回滚", protected: "恢复未完成，工作区保持维护保护", "storage-pending": "结果待保存，保护保持", "acceptance-pending": "原受理待核实" };

export function NativeRestoreExecution({ application, workspace, preview, onConsumed, onLockChange }: { application: ApplicationService; workspace: WorkspaceView; preview?: NativeRestorePreview; onConsumed(previewId: string): void; onLockChange(locked: boolean): void }) {
  const [pending, setPending] = useState(() => application.getPendingRestore?.());
  const [operation, setOperation] = useState<Operation>();
  const [busy, setBusy] = useState(false), [message, setMessage] = useState("");
  const [confirm, setConfirm] = useState(false), [credentials, setCredentials] = useState(false), [stop, setStop] = useState(false);
  const [target, setTarget] = useState(pending?.operationId ?? workspace.maintenance?.id ?? workspace.restoreOperations?.[0]?.id);
  const mounted = useRef(true), flight = useRef(false), targetRef = useRef(target);
  const submitted = useRef(pending?.request);
  const active = !!operation && !operationIsTerminal(operation);
  const locked = busy || !!pending || !!workspace.maintenance || active;
  useEffect(() => { onLockChange(locked); }, [locked]);
  useEffect(() => { mounted.current = true; return () => { mounted.current = false; }; }, []);
  useEffect(() => { setConfirm(false); setCredentials(false); setStop(false); }, [preview?.previewId]);
  function observe(op: Operation) {
    if (op.kind !== "backup-restore") return;
    if (op.id === targetRef.current) setOperation(old => mergeOperation(old?.id === op.id ? old : undefined, op));
    if (confirmsRestoreRequest(op, submitted.current)) { const previewId = submitted.current!.previewId; submitted.current = undefined; onConsumed(previewId); }
    setPending(application.getPendingRestore?.());
  }
  function select(id: string) { targetRef.current = id; setTarget(id); setOperation(old => old?.id === id ? old : undefined); }
  useEffect(() => {
    // Acceptance belongs to the adapter session, not the component that sent
    // it. A late explicit refusal may arrive after that component unmounted.
    setPending(application.getPendingRestore?.());
    const original = submitted.current;
    if (original && application.wasRestoreNotAccepted?.(original.requestId)) {
      submitted.current = undefined;
      if (!operation || operation.restoreReport?.requestId === original.requestId) { targetRef.current = undefined; setTarget(undefined); setOperation(undefined); }
    }
    for (const op of workspace.restoreOperations ?? []) observe(op);
    if (!targetRef.current && workspace.maintenance) select(workspace.maintenance.id);
  }, [workspace, target]);
  useEffect(() => {
    if (!target || operation && operationIsTerminal(operation)) return;
    let disposed = false, reading = false;
    const poll = async () => {
      if (reading) return; reading = true;
      try {
        const result = await application.getOperation(target);
        if (disposed || !mounted.current || targetRef.current !== target) return;
        if (result.ok) { observe(result.data); if (operationIsTerminal(result.data)) await application.refresh?.(); }
        else setMessage(`${result.error.code}：${result.error.message}`);
      } finally { reading = false; }
    };
    void poll(); const timer = setInterval(() => { void poll(); }, 1000);
    return () => { disposed = true; clearInterval(timer); };
  }, [application, target, active]);
  async function submit() {
    const original = application.getPendingRestore?.();
    if (flight.current || !original && (locked || !preview?.canRestore || !stop || preview.overwriteCount > 0 && !confirm || preview.credentialReentryCount > 0 && !credentials)) return;
    const request = original?.request ?? { previewId: preview!.previewId, archiveSha256: preview!.archiveSha256, confirmOverwrite: confirm, acknowledgeCredentials: credentials, stopRunning: stop, requestId: crypto.randomUUID() };
    submitted.current = request; flight.current = true; onLockChange(true); setBusy(true); setMessage("");
    const result = await application.applyRestore?.(request);
    flight.current = false; if (!mounted.current) return; setBusy(false); setPending(application.getPendingRestore?.());
    if (result?.ok) { select(result.data.operation.id); observe(result.data.operation); }
    else {
      setMessage(result ? `${result.error.code}：${result.error.message}` : "本机恢复服务不可用。");
      if (result?.operationId) select(result.operationId);
      if (result && result.error.code === "RESTORE_NOT_ACCEPTED" && (!operation || operation.restoreReport?.requestId === request.requestId)) { targetRef.current = undefined; setTarget(undefined); setOperation(undefined); }
      if (!application.getPendingRestore?.()) submitted.current = undefined;
    }
  }
  async function cancel() {
    if (flight.current || !operation || operation.id !== targetRef.current || !active) return;
    const id = operation.id; flight.current = true; setBusy(true);
    const result = await application.cancelOperation(id);
    flight.current = false; if (!mounted.current) return; setBusy(false);
    if (id !== targetRef.current) return;
    if (result.ok) observe(result.data); else setMessage(`${result.error.code}：${result.error.message}`);
  }
  async function retry() {
    if (flight.current || !operation || !application.retryRestore) return;
    const id = operation.id; flight.current = true; setBusy(true); setMessage("");
    const result = await application.retryRestore(id);
    flight.current = false; if (!mounted.current) return; setBusy(false);
    if (id !== targetRef.current) return;
    if (result.ok) observe(result.data); else setMessage(`${result.error.code}：${result.error.message}`);
  }
  const report = operation?.restoreReport;
  return <section aria-label="执行完整恢复">
    {preview && <><h3>确认完整恢复</h3><p>恢复期间配置进入维护保护。包内环境恢复原身份和数据，包外环境保留。已停止的会话不会自动重开。</p>
      {preview.overwriteCount > 0 && <label><input type="checkbox" disabled={locked} checked={confirm} onChange={e => setConfirm(e.target.checked)} />确认覆盖 {preview.overwriteCount} 个环境，撤回备份之后的浏览数据与配置。</label>}
      {preview.credentialReentryCount > 0 && <label><input type="checkbox" disabled={locked} checked={credentials} onChange={e => setCredentials(e.target.checked)} />了解 {preview.credentialReentryCount} 项代理凭据需要重新输入，不能当作已联网验证。</label>}
      <label><input type="checkbox" disabled={locked} checked={stop} onChange={e => setStop(e.target.checked)} />允许正常停止受影响环境；停止失败时中止，不自动强制结束。</label>
    </>}
    {(preview || pending) && <button className="button primary" disabled={busy || !pending && (locked || !preview?.canRestore || !stop || preview.overwriteCount > 0 && !confirm || preview.credentialReentryCount > 0 && !credentials)} onClick={() => void submit()}>{pending ? "核实原恢复请求" : "确认并完整恢复"}</button>}
    {pending && <p role="status">原请求 {pending.request.requestId} 受理未核实；保留原包与确认，不另建恢复。</p>}
    {message && <p role="alert">{message}</p>}
    {report && <><h3>{stages[operation.stage ?? ""] ?? operation.state}</h3><p>任务 {operation.id} · 已切换 {report.switchedCount} / {report.environmentCount}</p>{operation.error && <p role="alert">{operation.error.code}：{operation.error.message}</p>}
      {report.protected && <p role="status">维护保护中；目录和配置核对完成前不能启动或修改环境。</p>}
      {report.rolledBack && <p>旧目录与配置已核对保留；没有恢复成功。修正原因后重新预检。</p>}
      {operation.state === "completed" && <p>完整新状态已提交并核对，原 ID 和 seed 保留。请按原 Windows 用户上下文重新打开验证登录状态。</p>}
      {active && <button className="button" disabled={busy || operation.cancelRequested || report.committed || operation.persistencePending} onClick={() => void cancel()}>取消此恢复并回滚</button>}
      {operation.persistencePending && operation.stage !== "acceptance-pending" && <button className="button" disabled={busy || operation.state === "running"} onClick={() => void retry()}>重试核对/收尾原恢复</button>}
    </>}
    {(workspace.restoreOperations?.length ?? 0) > 0 && <h3>恢复记录（最近30个）</h3>}
    {workspace.restoreOperations?.map(op => <p key={op.id}>{op.id} · {stages[op.stage ?? ""] ?? op.state}<button className="button" disabled={busy} onClick={() => { select(op.id); observe(op); }}>读取该恢复结果</button></p>)}
  </section>;
}
