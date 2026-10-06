import { useEffect, useRef, useState } from "react";
import { Download, LoaderCircle, Plus } from "lucide-react";
import { mergeOperation, operationIsTerminal, type ApplicationService, type NativeBackupExportRequest, type Operation, type WorkspaceView } from "../application/contract";
import { confirmsBackupRequest } from "../application/backup-model";
import { NativeRestoreManager } from "./NativeRestoreManager";
import { ReferenceButton } from "./ReferenceUi";
import { LocalPageTable, LocalPageWindow, LocalPagination, LocalResultStats, localTime } from "./LocalPageUi";

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
  const [window, setWindow] = useState<"create" | "result">();
  const [list, setList] = useState<"published" | "tasks">("published"), [search, setSearch] = useState(""), [listPage, setListPage] = useState(1);
  const [frozenIds, setFrozenIds] = useState(() => [...selectedIds]);
  const mounted = useRef(true), generation = useRef(0), target = useRef<string | undefined>(viewId);
  const inFlight = useRef(false);
  const submitted = useRef<NativeBackupExportRequest | undefined>(pending?.request);
  const records = workspace.backupOperations ?? [];
  const currentOperation = operation?.id === viewId ? operation : undefined;
  const active = !!currentOperation && !operationIsTerminal(currentOperation);
  const locked = busy || active || !!pending;
  const shownIds = pending?.request.scope === "selected" ? pending.request.environmentIds : frozenIds;
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
    setBusy(false); syncPending(result.ok ? result.data : undefined);
    if (result.ok) { apply(result.data); if (operationIsTerminal(result.data)) await application.refresh?.(); }
    else setMessage(`${result.error.code}：${result.error.message} 可重读此明确ID，不取消或接管旧任务。`);
  }
  async function choose() {
    if (locked || inFlight.current) return; inFlight.current = true; setBusy(true);
    const result = await application.selectBackupDestination?.();
    inFlight.current = false; if (!mounted.current) return; setBusy(false);
    if (!result?.ok) { setMessage(result ? result.error.message : "本机文件保存能力尚不可用，没有模拟备份。"); return; }
    if (result.data.status === "selected" && result.data.destinationToken) { setDestination({ token: result.data.destinationToken, name: result.data.name ?? "本机备份" }); setConfirmed(false); setMessage(""); }
    else setMessage("已取消文件选择；未受理备份，原输入保留。");
  }
  async function start() {
    const original = application.getPendingBackupExport?.();
    if (inFlight.current || active && !original || !original && (!destination || !confirmed || scope === "selected" && !shownIds.length)) return;
    const request: NativeBackupExportRequest = original?.request ?? { scope, environmentIds: scope === "selected" ? [...shownIds] : [], destinationToken: destination!.token, stopRunning: true, requestId: crypto.randomUUID() };
    submitted.current = request;
    inFlight.current = true; setBusy(true); setMessage("");
    const result = await application.exportBackup?.(request);
    inFlight.current = false; if (!mounted.current) return; setBusy(false); syncPending(result?.ok ? result.data.operation : undefined);
    if (!result?.ok) {
      const notAccepted = !application.getPendingBackupExport?.() && result && !result.operationId;
      if (notAccepted) submitted.current = undefined;
      setMessage(`${result ? result.error.code + "：" + result.error.message : "本机备份服务不可用。"}${notAccepted ? " 未受理；可修正范围或重新选择新输出。" : " 结果未确认时只重发原请求，不重新选择输出。"}`);
      if (result?.operationId) { setWindow("result"); void read(result.operationId); }
      else if (application.getPendingBackupExport?.()) setWindow("result");
      return;
    }
    setDestination(undefined); setConfirmed(false); generation.current++; setViewGeneration(generation.current); target.current = result.data.operation.id; setViewId(result.data.operation.id); apply(result.data.operation);
    setWindow("result");
  }
  async function cancel() {
    if (!currentOperation || currentOperation.id !== target.current || !active || busy || inFlight.current) return;
    const current = generation.current, wanted = currentOperation.id; inFlight.current = true; setBusy(true);
    const result = await application.cancelOperation(wanted);
    inFlight.current = false; if (!mounted.current || current !== generation.current || wanted !== target.current) return;
    setBusy(false); syncPending(result.ok ? result.data : undefined); if (result.ok) apply(result.data); else setMessage(`${result.error.code}：${result.error.message}`);
  }
  const report = currentOperation?.backupReport;
  const publishedComplete = currentOperation?.state === "completed" && !!report?.published && !currentOperation.persistencePending;
  const published = (workspace.nativeBackups ?? []).filter(record => record.name.includes(search));
  const tasks = records.filter(record => (record.backupReport?.name ?? record.id).includes(search));
  const total = list === "published" ? published.length : tasks.length;
  const page = Math.min(listPage, Math.max(1, Math.ceil(total / 10)));
  const openResult = (id: string) => { setWindow("result"); void read(id); };
  return <section className="local-page local-page-backups" aria-label="完整本机备份">
    <div className="local-page-tabs"><button className={list === "published" ? "selected" : ""} onClick={() => { setList("published"); setListPage(1); }}>已发布备份</button><button className={list === "tasks" ? "selected" : ""} onClick={() => { setList("tasks"); setListPage(1); }}>导出任务</button></div>
    <div className="local-page-panel">
      <div className="local-page-toolbar"><ReferenceButton className="primary" disabled={busy || active || !!pending || !!workspace.maintenance} onClick={() => { setFrozenIds([...selectedIds]); setScope(selectedIds.length ? "selected" : "all"); setWindow("create"); setMessage(""); }}><Plus size={15} />创建完整备份</ReferenceButton><NativeRestoreManager application={application} workspace={workspace} /><input className="local-page-search" aria-label="搜索备份名称" placeholder="输入备份名称搜索" value={search} onChange={event => { setSearch(event.target.value); setListPage(1); }} /><span className="local-page-tail subtle-text">本机完整包 · 非演示 JSON</span></div>
      {pending && <p className="local-page-note" role="status">原导出尚待核实；不允许另建任务。<button className="local-page-link" disabled={busy} onClick={() => setWindow("result")}>继续核实原导出</button></p>}
      {list === "published" ? <LocalPageTable headings={["备份名称", "环境数量", "创建时间", "格式", "操作"]} empty={!published.length ? "尚无已发布完整备份；创建后须等待核对与发布。" : undefined}>{published.slice((page - 1) * 10, page * 10).map(record => <tr key={record.id}><td>{record.name}</td><td>{record.environmentCount} 个</td><td>{localTime(record.createdAt)}</td><td>.prismbackup</td><td><button className="local-page-link" disabled={busy} onClick={() => openResult(record.operationId)}>读取该任务</button></td></tr>)}</LocalPageTable> : <LocalPageTable headings={["任务 / 文件", "环境数量", "阶段", "结果", "操作"]} empty={!tasks.length ? "没有匹配的导出任务。" : undefined}>{tasks.slice((page - 1) * 10, page * 10).map(record => <tr key={record.id}><td>{record.backupReport?.name || "本机备份"}</td><td>{record.total}</td><td>{record.persistencePending ? "结果待保存 / 核实" : stages[record.stage ?? ""] ?? record.stage}</td><td>{record.persistencePending ? "未确认完成" : record.state === "cancelled" ? "已取消" : record.state === "failed" ? "失败" : record.state === "completed" && record.backupReport?.published ? "已发布" : "未完成"}</td><td><button className="local-page-link" disabled={busy} onClick={() => openResult(record.id)}>查看保存结果</button></td></tr>)}</LocalPageTable>}
      <LocalPagination total={total} page={page} onPage={setListPage} />
      <p className="local-page-note">最近 30 条本机记录。包含敏感浏览数据，不携带内核；不承诺跨 Windows 用户或跨机器恢复登录。</p>
    </div>
    {window === "create" && <LocalPageWindow title="创建完整本机备份" height={340} onClose={() => setWindow(undefined)} busy={busy} footer={<><ReferenceButton disabled={busy} onClick={() => setWindow(undefined)}>取消</ReferenceButton><ReferenceButton className="primary" disabled={locked || !destination || !confirmed || scope === "selected" && !shownIds.length} onClick={() => void start()}>正常关闭并导出</ReferenceButton></>}>
      <label className="reference-field"><span>备份范围</span><select aria-label="备份范围" disabled={locked} value={scope} onChange={event => { setScope(event.target.value as "all" | "selected"); setConfirmed(false); }}><option value="all">全部已保存环境（不是当前一页）</option><option value="selected" disabled={!shownIds.length}>明确选定的 {shownIds.length} 个环境</option></select></label>
      {scope === "selected" && <p>已冻结 {shownIds.length} 个所选 ID；搜索、翻页和离页不会扩大范围。</p>}
      <div className="local-page-row-actions"><ReferenceButton disabled={locked} onClick={() => void choose()}>选择新备份文件位置</ReferenceButton><span>{destination?.name ?? "尚未选择"}</span></div>
      <label className="local-page-check"><input type="checkbox" disabled={locked} checked={confirmed} onChange={event => setConfirmed(event.target.checked)} /><span>允许正常关闭范围内会话；停止失败中止，不强制杀进程。取消后已关闭环境不会自动重开。</span></label>
      <p className="subtle-text">新 .prismbackup，不覆盖已有文件；DPAPI 密文仅适用于原 Windows 用户。</p>
      {busy && <p role="status"><LoaderCircle size={14} className="spin" />正在与本机服务核对。</p>}{message && <p role="alert">{message}</p>}
    </LocalPageWindow>}
    {window === "result" && <LocalPageWindow title="本机备份结果" width={400} className="local-page-result" onClose={() => setWindow(undefined)} footer={<><ReferenceButton onClick={() => setWindow(undefined)}>返回列表</ReferenceButton>{pending ? <ReferenceButton className="primary" disabled={busy} onClick={() => void start()}>核实原导出请求（不创建新任务）</ReferenceButton> : viewId && <ReferenceButton disabled={busy} onClick={() => { if (viewId) void read(viewId); }}>重新读取此任务 ID</ReferenceButton>}</>}>
      <p>{publishedComplete ? "完整备份已发布" : currentOperation?.persistencePending ? "备份结果待保存 / 核实" : currentOperation?.stage === "completed" ? "发布结果待核实" : stages[currentOperation?.stage ?? ""] ?? (currentOperation?.state === "cancelled" ? "导出已取消，未发布完整备份" : currentOperation?.state === "failed" ? "导出失败，未发布完整备份" : "正在读取原任务")}</p>
      {report && <><LocalResultStats items={[{ label: "环境", value: report.environmentCount }, { label: "已复制", value: report.copiedEnvironmentCount }, { label: "已发布", value: report.published ? "是" : "否", tone: report.published ? "success" : undefined }]} /><p>{report.fileCount} 个条目 · {report.byteCount.toLocaleString()} 字节</p>{report.published && <><p><Download size={12} /> {report.name}</p><p>包 SHA-256：<code>{report.archiveSha256}</code></p><p>清单 SHA-256：<code>{report.manifestSha256}</code></p></>}</>}
      {currentOperation?.persistencePending && <p role="status">原观测待保存或受理待核实，不重复复制。</p>}
      {pending && <p role="status">保留原 requestId、范围和输出授权。离开此页不取消已受理任务。</p>}
      {message && <p role="alert">{message}</p>}{currentOperation?.error && <p role="alert">{currentOperation.error.code}：{currentOperation.error.message}</p>}
      {active && currentOperation && <ReferenceButton disabled={busy || currentOperation.cancelRequested || currentOperation.stage === "publishing" || currentOperation.persistencePending} onClick={() => void cancel()}>取消此导出</ReferenceButton>}
    </LocalPageWindow>}
  </section>;
}
