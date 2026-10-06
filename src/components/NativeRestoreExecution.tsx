import { useEffect, useRef, useState } from "react";
import { mergeOperation, operationIsTerminal, type ApplicationService, type NativeRestorePreview, type Operation, type WorkspaceView } from "../application/contract";
import { confirmsRestoreRequest } from "../application/restore-model";
import { ReferenceButton } from "./ReferenceUi";
import { LocalPageTable, LocalPageWindow, LocalResultStats, RestoreConfirmWindow } from "./LocalPageUi";

const stages: Record<string, string> = { accepted: "恢复已受理", revalidating: "再次核对原包", "stopping-environments": "正常停止受影响环境", prepared: "新目录与旧状态已准备", swapping: "切换目录", "db-committing": "提交配置事务", "db-committed": "确认新状态", finalized: "完整恢复完成", "rolled-back": "恢复未完成，原状态已回滚", protected: "恢复未完成，工作区保持维护保护", "storage-pending": "结果待保存，保护保持", "acceptance-pending": "原受理待核实", recovering: "按日志找回完整状态", "workspace-recovery": "目录已核对，正在恢复其余本机记录" };

export function NativeRestoreExecution({ application, workspace, preview, onConsumed, onLockChange, confirmationRequested }: { application: ApplicationService; workspace: WorkspaceView; preview?: NativeRestorePreview; onConsumed(previewId: string): void; onLockChange(locked: boolean): void; confirmationRequested?: number }) {
  const [pending, setPending] = useState(() => application.getPendingRestore?.());
  const [operation, setOperation] = useState<Operation>();
  const [busy, setBusy] = useState(false), [message, setMessage] = useState("");
  const [confirm, setConfirm] = useState(false), [credentials, setCredentials] = useState(false), [stop, setStop] = useState(false);
  const [window, setWindow] = useState<"confirm" | "result" | "history">();
  const [target, setTarget] = useState(pending?.operationId ?? workspace.maintenance?.id ?? workspace.restoreOperations?.[0]?.id);
  const mounted = useRef(true), flight = useRef(false), targetRef = useRef(target);
  const submitted = useRef(pending?.request);
  const active = !!operation && !operationIsTerminal(operation);
  const locked = busy || !!pending || !!workspace.maintenance || active;
  useEffect(() => { onLockChange(locked); }, [locked]);
  useEffect(() => { mounted.current = true; return () => { mounted.current = false; }; }, []);
  useEffect(() => { setConfirm(false); setCredentials(false); setStop(false); }, [preview?.previewId]);
  useEffect(() => { if (confirmationRequested && preview?.canRestore) setWindow("confirm"); }, [confirmationRequested]);
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
    setWindow("result");
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
  const succeeded = operation?.state === "completed" && !!report?.committed && !report.protected && !report.rolledBack && !operation.persistencePending && report.switchedCount === report.environmentCount;
  const title = succeeded ? "完整恢复完成" : report?.rolledBack ? "恢复未完成，原状态已回滚" : report?.protected ? "恢复未完成，工作区保持维护保护" : operation?.state === "cancelled" ? "恢复已取消" : operation?.stage === "finalized" ? "完整提交结果待核实" : stages[operation?.stage ?? ""] ?? "恢复结果待核实";
  return <div className="local-page-execution-entry" aria-label="执行完整恢复">
    {preview && confirmationRequested === undefined && <ReferenceButton className="primary" disabled={locked || !preview.canRestore} onClick={() => setWindow("confirm")}>确认完整恢复</ReferenceButton>}
    {pending && <ReferenceButton disabled={busy} onClick={() => setWindow("result")}>继续核实原恢复</ReferenceButton>}
    {!pending && (workspace.restoreOperations?.length || operation || workspace.maintenance) ? <ReferenceButton disabled={busy} onClick={() => setWindow("history")}>恢复记录</ReferenceButton> : null}
    {window === "confirm" && preview && <RestoreConfirmWindow busy={busy} onClose={() => setWindow(undefined)} disabled={locked || !preview.canRestore || !stop || preview.overwriteCount > 0 && !confirm || preview.credentialReentryCount > 0 && !credentials} onConfirm={() => void submit()}>
      <p>只恢复包内 {preview.environmentCount} 个环境，包外记录保留。原 ID、seed 与数据保留；已停止会话不会自动重开。</p>
      {preview.overwriteCount > 0 && <label className="local-page-check"><input type="checkbox" disabled={locked} checked={confirm} onChange={e => setConfirm(e.target.checked)} /><span>确认覆盖 {preview.overwriteCount} 个环境，撤回备份之后的浏览数据与配置。</span></label>}
      {preview.credentialReentryCount > 0 && <label className="local-page-check"><input type="checkbox" disabled={locked} checked={credentials} onChange={e => setCredentials(e.target.checked)} /><span>了解 {preview.credentialReentryCount} 项代理凭据需要重新输入，不能当作已联网验证。</span></label>}
      <label className="local-page-check"><input type="checkbox" disabled={locked} checked={stop} onChange={e => setStop(e.target.checked)} /><span>允许正常停止受影响环境；停止失败中止，不自动强制结束。</span></label>
    </RestoreConfirmWindow>}
    {window === "result" && <LocalPageWindow title="完整恢复结果" width={400} className="local-page-result" onClose={() => setWindow(undefined)} footer={<><ReferenceButton onClick={() => setWindow(undefined)}>返回</ReferenceButton>{pending && <ReferenceButton className="primary" disabled={busy} onClick={() => void submit()}>核实原恢复请求</ReferenceButton>}{!pending && preview && !locked && <ReferenceButton onClick={() => setWindow("confirm")}>返回影响确认</ReferenceButton>}</>}>
      <p>{title}</p>
      {pending && <p role="status">原受理尚未核实；保留原 requestId、包摘要与覆盖/停止授权，不另建恢复。关闭窗口不取消任务。</p>}
      {report && <><LocalResultStats items={[{ label: "环境", value: report.environmentCount }, { label: "已切换", value: report.switchedCount }, { label: "已确认", value: succeeded ? "完成" : "未完成", tone: succeeded ? "success" : "error" }]} />
        {report.recoveredAfterRestart && <p>上次应用在“{stages[report.interruptedStage ?? ""] ?? report.interruptedStage}”退出。本次按提交标记与实际目录核对，未重新导入原包。</p>}
        {report.protected && <p role="status">维护保护中；目录与配置核对完成前不能启动或修改环境。</p>}
        {report.protected && operation?.state === "failed" && <p>先检查空间、原权限或目录占用，再重试原核对。保留日志、incoming、previous 与配置副本；不要删锁、移动未知目录或用旧内核打开新数据。</p>}
        {report.rolledBack && <p>旧目录与配置已核对保留；没有恢复成功。修正原因后重新预检。</p>}
        {succeeded && <p>完整新状态已提交并核对，原 ID 和 seed 保留。请在原 Windows 用户上下文重新打开核验登录。</p>}
      </>}
      {busy && <p role="status">正在核对本机任务…</p>}{message && <p role="alert">{message}</p>}{operation?.error && <p role="alert">{operation.error.code}：{operation.error.message}</p>}
      {active && operation && <ReferenceButton disabled={busy || operation.cancelRequested || report?.committed || operation.persistencePending} onClick={() => void cancel()}>取消此恢复并回滚</ReferenceButton>}
      {operation?.persistencePending && operation.stage !== "acceptance-pending" && <ReferenceButton disabled={busy || operation.state === "running"} onClick={() => void retry()}>重试核对/收尾原恢复</ReferenceButton>}
    </LocalPageWindow>}
    {window === "history" && <LocalPageWindow title="恢复记录（最近30个）" width={1040} height={592} className="local-page-preflight" onClose={() => setWindow(undefined)} footer={<ReferenceButton onClick={() => setWindow(undefined)}>返回</ReferenceButton>}>
      <LocalPageTable headings={["任务", "范围", "阶段", "恢复状态", "操作"]} empty={!workspace.restoreOperations?.length ? "暂无已读取的恢复记录。" : undefined}>{workspace.restoreOperations?.map(op => <tr key={op.id}><td>{op.id}</td><td>{op.total} 个环境</td><td>{stages[op.stage ?? ""] ?? op.stage}</td><td>{op.restoreReport?.rolledBack ? "未完成，已回滚" : op.restoreReport?.protected || op.persistencePending ? "保护/待保存" : op.state === "completed" ? "已完成" : "未完成"}</td><td><button className="local-page-link" disabled={busy} onClick={() => { select(op.id); observe(op); setWindow("result"); }}>读取该恢复结果</button></td></tr>)}</LocalPageTable>
      <p className="local-page-note">查看旧记录不消费未决原请求，也不接管后来会话；核对与取消只作用当前明确任务 ID。</p>
    </LocalPageWindow>}
  </div>;
}
