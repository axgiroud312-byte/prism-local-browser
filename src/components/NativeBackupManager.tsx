import { useEffect, useRef, useState } from "react";
import { LoaderCircle } from "lucide-react";
import { mergeOperation, operationIsTerminal, type ApplicationService, type NativeBackupExportRequest, type Operation, type WorkspaceView } from "../application/contract";
import { confirmsBackupRequest } from "../application/backup-model";
import "./native-proxy.css";

const stages: Record<string, string> = { accepted: "等待调度", "stopping-environments": "正常关闭所选会话", "configuration-snapshot": "生成一致配置快照", "copying-browser-data": "复制真实浏览数据", "verifying-package": "完整读回核对文件摘要", publishing: "发布已核对备份", completed: "已发布完整备份", interrupted: "应用退出中断，未自动重做", "storage-pending": "观测待保存，不重复复制", "acceptance-pending": "受理结果待核实，不重复导出" };

export function NativeBackupManager({ application, workspace, selectedIds }: { application: ApplicationService; workspace: WorkspaceView; selectedIds: string[] }) {
  const [pending, setPending] = useState(() => application.getPendingBackupExport?.());
  const [scope, setScope] = useState<"all" | "selected">(pending?.request.scope ?? (selectedIds.length ? "selected" : "all"));
  const [destination, setDestination] = useState<{ token: string; name: string }>();
  const [confirmed, setConfirmed] = useState(false);
  const [operation, setOperation] = useState<Operation>();
  const [busy, setBusy] = useState(false);
  const [message, setMessage] = useState("");
  const [viewId, setViewId] = useState(pending?.operationId ?? workspace.backupOperations?.[0]?.id);
  const [viewGeneration, setViewGeneration] = useState(0);
  const mounted = useRef(true), generation = useRef(0), target = useRef<string | undefined>(viewId);
  const inFlight = useRef(false);
  const submitted = useRef<NativeBackupExportRequest | undefined>(pending?.request);
  const records = workspace.backupOperations ?? [];
  const currentOperation = operation?.id === viewId ? operation : undefined;
  const active = !!currentOperation && !operationIsTerminal(currentOperation);
  const locked = busy || active || !!pending;
  const shownIds = pending?.request.scope === "selected" ? pending.request.environmentIds : selectedIds;
  function syncPending(observed?: Operation) {
    if (confirmsBackupRequest(observed, submitted.current)) {
      setDestination(undefined); setConfirmed(false); submitted.current = undefined;
    }
    setPending(application.getPendingBackupExport?.());
  }
  useEffect(() => { mounted.current = true; return () => { mounted.current = false; generation.current++; }; }, []);
  function apply(next: Operation) { if (next.kind === "backup-export" && next.id === target.current) setOperation(previous => mergeOperation(previous?.id === next.id ? previous : undefined, next)); }
  useEffect(() => {
    const next = records.find(record => record.id === target.current);
    if (next) apply(next);
    syncPending(records.find(record => confirmsBackupRequest(record, submitted.current)));
  }, [workspace.backupOperations, viewId]);
  useEffect(() => {
    if (!currentOperation || operationIsTerminal(currentOperation)) return;
    const wanted = currentOperation.id, current = generation.current;
    let running = false;
    const poll = async () => {
      if (running) return; running = true;
      try {
        const result = await application.getOperation(wanted);
        if (!mounted.current || generation.current !== current || target.current !== wanted) return;
        if (result.ok) { apply(result.data); syncPending(result.data); if (operationIsTerminal(result.data)) await application.refresh?.(); }
        else setMessage(`${result.error.code}：${result.error.message} 可明确重新读取，不重新导出。`);
      } finally { running = false; }
    };
    const timer = setInterval(() => { void poll(); }, 1000);
    return () => clearInterval(timer);
  }, [application, viewId, viewGeneration, currentOperation?.id, active]);
  async function read(id: string) {
    if (inFlight.current) return;
    const current = ++generation.current; target.current = id; setViewId(id); setViewGeneration(current); if (operation?.id !== id) setOperation(undefined); setBusy(true); setMessage("");
    const result = await application.getOperation(id);
    if (!mounted.current || current !== generation.current) return;
    setBusy(false); syncPending(result.ok ? result.data : undefined); if (result.ok) apply(result.data); else setMessage(`${result.error.code}：${result.error.message} 可重读此明确ID，不取消或接管旧任务。`);
  }
  async function choose() {
    if (locked || inFlight.current) return; inFlight.current = true; setBusy(true);
    const result = await application.selectBackupDestination?.();
    inFlight.current = false; if (!mounted.current) return; setBusy(false);
    if (!result?.ok) { setMessage(result ? result.error.message : "本机文件保存能力尚不可用，没有模拟备份。"); return; }
    if (result.data.status === "selected" && result.data.destinationToken) { setDestination({ token: result.data.destinationToken, name: result.data.name ?? "本机备份" }); setConfirmed(false); setMessage(""); }
  }
  async function start() {
    const original = application.getPendingBackupExport?.();
    if (inFlight.current || active && !original || !original && (!destination || !confirmed || scope === "selected" && !selectedIds.length)) return;
    const request: NativeBackupExportRequest = original?.request ?? { scope, environmentIds: scope === "selected" ? [...selectedIds] : [], destinationToken: destination!.token, stopRunning: true, requestId: crypto.randomUUID() };
    submitted.current = request;
    inFlight.current = true; setBusy(true); setMessage("");
    const result = await application.exportBackup?.(request);
    inFlight.current = false; if (!mounted.current) return; setBusy(false); syncPending(result?.ok ? result.data.operation : undefined);
    if (!result?.ok) {
      const notAccepted = !application.getPendingBackupExport?.() && result && !result.operationId;
      if (notAccepted) submitted.current = undefined;
      setMessage(`${result ? result.error.code + "：" + result.error.message : "本机备份服务不可用。"}${notAccepted ? " 未受理；可修正范围或重新选择新输出。" : " 结果未确认时只重发原请求，不重新选择输出。"}`);
      if (result?.operationId) void read(result.operationId); return;
    }
    setDestination(undefined); setConfirmed(false); generation.current++; setViewGeneration(generation.current); target.current = result.data.operation.id; setViewId(result.data.operation.id); apply(result.data.operation);
  }
  async function cancel() {
    if (!currentOperation || currentOperation.id !== target.current || !active || busy || inFlight.current) return;
    const current = generation.current, wanted = currentOperation.id; inFlight.current = true; setBusy(true);
    const result = await application.cancelOperation(wanted);
    inFlight.current = false; if (!mounted.current || current !== generation.current || wanted !== target.current) return;
    setBusy(false); syncPending(result.ok ? result.data : undefined); if (result.ok) apply(result.data); else setMessage(`${result.error.code}：${result.error.message}`);
  }
  const report = currentOperation?.backupReport;
  return <section className="native-proxy-panel" aria-label="完整本机备份">
    <h2>导出完整本机备份</h2>
    <p>包含一致的配置数据库、固定设备档案及真实浏览数据。不是演示 JSON，也不代表恢复能力已验收。</p>
    <p>代理认证仅保存原 Windows 用户的 DPAPI 密文，不解密打包。浏览数据仍含敏感网站信息；请妥善保管，不承诺重装、跨用户或跨电脑恢复登录。</p>
    <p>不携带内核程序；恢复需同一精确版本和校验值。未绑定内核的环境仍是不可启动的待绑定状态。</p>
    <label>备份范围<select disabled={locked} value={scope} onChange={event => { setScope(event.target.value as "all" | "selected"); setConfirmed(false); }}><option value="all">全部已保存环境（不是当前一页）</option><option value="selected" disabled={!shownIds.length}>明确选定的 {shownIds.length} 个环境</option></select></label>
    {scope === "selected" && <p>所选 ID：{shownIds.slice(0, 25).join("、")}{shownIds.length > 25 && "……（其余已选 ID 也在范围内）"}。可返回环境页选择；筛选不会扩展目标。</p>}
    <div className="native-proxy-actions"><button className="button" disabled={locked} onClick={() => { void choose(); }}>选择新备份文件位置</button><span>{destination?.name ?? "尚未选择"}（不覆盖已有文件）</span></div>
    <label><input type="checkbox" disabled={locked} checked={confirmed} onChange={event => setConfirmed(event.target.checked)} />允许正常关闭范围内的运行会话；停止失败中止备份，不强制杀进程。取消后已关闭的环境不会自动重开。</label>
    {pending && <p role="status">原导出请求 {pending.request.requestId} 尚待核实；离开此页或查看旧历史不会丢弃它，也不能改输出后另建任务。</p>}
    <div className="native-proxy-actions"><button className="button primary" disabled={busy || !pending && (active || !destination || !confirmed || scope === "selected" && !selectedIds.length)} onClick={() => { void start(); }}>{pending ? "核实原导出请求（不创建新任务）" : "正常关闭并导出"}</button>{viewId && <button className="button" disabled={busy} onClick={() => { if (viewId) void read(viewId); }}>重新读取此任务 ID</button>}{active && currentOperation && <button className="button" disabled={busy || currentOperation.cancelRequested || currentOperation.stage === "publishing" || currentOperation.persistencePending} onClick={() => { void cancel(); }}>取消此导出</button>}</div>
    {busy && <p role="status"><LoaderCircle size={16} className="spin" />正在与本机服务核对。</p>}
    {message && <p role="alert">{message}</p>}
    {report && <><h3>{operation?.state === "completed" && report.published ? "完整备份已发布" : stages[operation?.stage ?? ""] ?? operation?.state}</h3><p>浏览目录 {report.copiedEnvironmentCount} / {report.environmentCount} · {report.fileCount} 个条目 · {report.byteCount.toLocaleString()} 字节{operation?.persistencePending && " · 原观测待保存，不重复复制"}</p><p>任务 ID：{operation?.id}</p>{operation?.error && <p role="alert">{operation.error.code}：{operation.error.message}</p>}{report.published && <><p>文件：{report.name}</p><p>包 SHA-256：<code>{report.archiveSha256}</code></p><p>清单 SHA-256：<code>{report.manifestSha256}</code></p></>}</>}
    <h3>已发布记录（最近30个）</h3>
    {(workspace.nativeBackups ?? []).map(record => <p key={record.id}>{record.name} · {record.environmentCount} 个环境 · {record.createdAt}<button className="button" disabled={busy || inFlight.current} onClick={() => { void read(record.operationId); }}>读取该任务</button></p>)}
    <h3>导出任务（包括取消和失败）</h3>
    {records.map(record => <p key={record.id}>{record.backupReport?.name} · {record.state}<button className="button" disabled={busy || inFlight.current} onClick={() => { void read(record.id); }}>查看保存结果</button></p>)}
    <p>恢复预检与正式恢复仍待后续任务；此页不会接收原型 JSON 或覆盖现有工作区。</p>
  </section>;
}
