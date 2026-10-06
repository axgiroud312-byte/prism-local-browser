import { useEffect, useRef, useState, useSyncExternalStore } from "react";
import { mergeOperation, operationIsTerminal, type ApplicationService, type Operation, type WorkspaceView } from "../application/contract";
import { KernelManagementPage } from "./KernelManagementPage";
import { KernelDetailsWindow } from "./KernelDetailsWindow";
import { ProxyKernelModal } from "./ProxyKernelModal";
import { getKernelTaskOwner } from "./kernel-task-owner";
import "./native-kernel.css";

const stageLabels: Record<string, string> = { queued: "已受理，等待执行", "acquiring-archive": "取得精确归档并校验 SHA-256", extracting: "在安全暂存目录解包", probing: "隔离探测实际身份和参数", "verifying-files": "核验既有文件摘要", publishing: "发布不可替换的新内核 ID", "verifying-extracted-files": "锁定解包文件并复核全部摘要", completed: "已完成", failed: "失败，查看具体原因", cancelled: "已取消", interrupted: "上次任务被中断" };
export function NativeKernelManager({ application, workspace, onMigration }: { application: ApplicationService; workspace: WorkspaceView; onMigration?: () => void }) {
  const owner = getKernelTaskOwner(application), task = useSyncExternalStore(owner.subscribe, owner.getSnapshot, owner.getSnapshot);
  const [source, setSource] = useState<"official" | "local">("official"), [version, setVersion] = useState(""), [checksum, setChecksum] = useState("");
  const [archive, setArchive] = useState<{ token: string; name: string }>(), [trusted, setTrusted] = useState(false);
  const [prepareOpen, setPrepareOpen] = useState(false), [progressOpen, setProgressOpen] = useState(false), [historyOpen, setHistoryOpen] = useState(false), [detailId, setDetailId] = useState("");
  const [confirmDelete, setConfirmDelete] = useState(""), [defaultBusy, setDefaultBusy] = useState(false), [defaultRequest, setDefaultRequest] = useState(() => application.getPendingKernelDefault?.());
  const alive = useRef(true), defaultFlight = useRef(false);
  const records = workspace.kernelRecords ?? [], ongoing = workspace.kernelOperations?.find(operation => !operationIsTerminal(operation));
  const operation = task.operation && ongoing?.id === task.operation.id ? mergeOperation(task.operation, ongoing) : ongoing ?? task.operation;
  const activeId = operation && !operationIsTerminal(operation) ? operation.id : undefined;
  const waiting = task.busy || task.unknown || !!activeId || defaultBusy || !!defaultRequest || !!workspace.issue;
  useEffect(() => { alive.current = true; return () => { alive.current = false; }; }, []);
  useEffect(() => { setDefaultRequest(application.getPendingKernelDefault?.()); }, [application, workspace]);
  useEffect(() => {
    if (ongoing && !task.operation) owner.select(ongoing);
    if (task.operation) for (const item of workspace.kernelOperations ?? []) if (item.id === task.operation.id) owner.observe(item, item.id);
  }, [workspace, owner, task.operation?.id]);
  useEffect(() => {
    if (!activeId) return;
    if (owner.getSnapshot().operation?.id !== activeId) owner.select(operation!);
    let disposed = false, timer: ReturnType<typeof setTimeout>;
    async function read() {
      try {
        const response = await application.getOperation(activeId!);
        if (disposed || owner.getSnapshot().operation?.id !== activeId) return;
        if (!response.ok) { owner.message(`${response.error.code}：${response.error.message} 状态未确认，继续读取。`); timer = setTimeout(read, 1000); return; }
        owner.observe(response.data, activeId!);
        const current = owner.getSnapshot().operation!;
        if (operationIsTerminal(current)) {
          owner.message(current.state === "completed" ? "内核任务已完成；默认变化不修改旧环境。本次页面检查不是新的真实桌面验收。" : `${current.error?.code ?? current.state}：${current.error?.message ?? "任务未完成"}${current.error?.details?.reason ? `（${current.error.details.reason}）` : ""}`);
          await application.refresh?.();
        } else timer = setTimeout(read, 500);
      } catch { if (!disposed) { owner.message("任务读取未确认，保留原任务继续核实。"); timer = setTimeout(read, 1000); } }
    }
    void read(); return () => { disposed = true; clearTimeout(timer); };
  }, [application, owner, activeId]);
  async function install() {
    if (task.unknown) { if (await owner.retry()) { setPrepareOpen(false); setProgressOpen(true); } return; }
    if (waiting) return;
    if (await owner.begin({ kind: "install", request: { source, version: version.trim(), expectedChecksum: checksum.trim().toLowerCase(), archiveToken: source === "local" ? archive?.token : undefined, trusted: source === "local" && trusted, requestId: crypto.randomUUID() } })) { setPrepareOpen(false); setProgressOpen(true); }
  }
  async function selectDefault(kernelId: string) {
    if (!workspace.defaultKernel || !application.setDefaultKernel || defaultFlight.current) return;
    const request = application.getPendingKernelDefault?.() ?? { kernelId, expectedRevision: workspace.defaultKernel.revision, requestId: crypto.randomUUID() };
    defaultFlight.current = true; setDefaultRequest(request); setDefaultBusy(true);
    try { const result = await application.setDefaultKernel(request); if (!alive.current) return; setDefaultRequest(application.getPendingKernelDefault?.()); owner.message(result.ok ? "默认构建已保存，只影响后续新建；旧环境保持原构建。" : `${result.error.code}：${result.error.message}`); }
    catch { if (alive.current) owner.message("默认选择结果未知，只核实原请求。"); }
    finally { defaultFlight.current = false; if (alive.current) setDefaultBusy(false); }
  }
  async function verify(kernelId: string) { if (await owner.begin({ kind: "verify", kernelId, requestId: crypto.randomUUID() })) setProgressOpen(true); }
  const detail = records.find(record => record.id === detailId), deleting = records.find(record => record.id === confirmDelete);
  const sourceRequest = task.pending?.kind === "install" ? task.pending.request : undefined;
  return <><KernelManagementPage native records={records.map(record => ({ id: record.id, version: record.version, source: record.source.kind === "official" ? "官方发行" : "可信本地 ZIP", architecture: record.architecture, available: record.status === "verified" && workspace.state.kernels.some(kernel => kernel.id === record.id && kernel.available), statusText: record.status === "verified" ? "真实诊断已核验" : "核验失败 · 不可用", archiveHash: record.archiveSha256, executableHash: record.executableSha256, usedCount: record.usedCount ?? record.usedBy.length, isDefault: workspace.defaultKernel?.kernelId === record.id, deleteProtected: !!(record.usedCount ?? record.usedBy.length) || workspace.defaultKernel?.kernelId === record.id }))} waiting={waiting} onPrepare={() => setPrepareOpen(true)} onInspect={setDetailId} onVerify={id => void verify(id)} onDelete={setConfirmDelete} onDefault={workspace.defaultKernel ? id => void selectDefault(id) : undefined} onManualDefault={workspace.defaultKernel ? () => void selectDefault("kernel-pending") : undefined} onRefresh={() => void application.refresh?.()} onHistory={() => setHistoryOpen(true)} onMigration={onMigration} message={task.message}>
    {operation && <button className="button" onClick={() => setProgressOpen(true)}>查看内核任务 · {stageLabels[operation.stage ?? operation.state] ?? operation.state}</button>}
    {task.unknown && <button className="button primary" disabled={task.busy} onClick={async () => { if (await owner.retry()) setProgressOpen(true); }}>核实原内核请求</button>}
    {defaultRequest && <button className="button primary" disabled={defaultBusy} onClick={() => void selectDefault(defaultRequest.kernelId)}>核实原默认选择请求</button>}
    {operation && operationIsTerminal(operation) && operation.state !== "completed" && task.lastRequest && <button className="button primary" disabled={waiting} onClick={async () => { if (await owner.repeat()) setProgressOpen(true); }}>重试同一精确构建任务</button>}
  </KernelManagementPage>
  {prepareOpen && <ProxyKernelModal title="安装精确内核" width={620} height={530} onClose={() => setPrepareOpen(false)} busy={task.busy} className="kernel35-prepare" footer={<><span className="pk35-footer-note">只登记新的精确 ID，不替换旧构建</span><button className="button" disabled={task.busy} onClick={() => setPrepareOpen(false)}>取消</button><button className="button primary" type="submit" form="kernel35-install" disabled={task.busy || !task.unknown && (waiting || source === "local" && (!archive || !trusted))}>{task.unknown ? "核实原内核请求" : "安装并核验"}</button></>}>
    <form id="kernel35-install" onSubmit={event => { event.preventDefault(); void install(); }}><label className="reference-field"><span>归档来源</span><select aria-label="归档来源" value={sourceRequest?.source ?? source} disabled={waiting} onChange={event => { setSource(event.target.value as "official" | "local"); setTrusted(false); }}><option value="official">官方发行 ZIP</option><option value="local">本地可信 ZIP</option></select></label><label className="reference-field"><span>精确发行版本</span><input aria-label="精确发行版本" placeholder="填写准确的四段版本" required pattern="[0-9]+\.[0-9]+\.[0-9]+\.[0-9]+" value={sourceRequest?.version ?? version} disabled={waiting} onChange={event => { setVersion(event.target.value); setChecksum(""); }} /></label><label className="reference-field"><span>归档 SHA-256</span><input aria-label="预期归档 SHA-256" required pattern="[a-fA-F0-9]{64}" placeholder="64 位十六进制摘要" value={sourceRequest?.expectedChecksum ?? checksum} disabled={waiting} onChange={event => setChecksum(event.target.value)} /></label>
    {(sourceRequest?.source ?? source) === "local" ? <><div className="native-kernel-actions"><button className="button" type="button" disabled={waiting} onClick={async () => { try { const result = await application.selectKernelArchive?.(); if (!result) owner.message("文件选择服务不可用。"); else if (!result.ok) owner.message(`${result.error.code}：${result.error.message}`); else if (result.data.status === "selected") { setArchive({ token: result.data.archiveToken!, name: result.data.name! }); setTrusted(false); } } catch { owner.message("文件选择未确认，没有安装。"); } }}>选择可信 ZIP</button><span>{archive?.name ?? "尚未选择文件"}</span></div><label className="pk35-checkbox"><input type="checkbox" checked={sourceRequest?.trusted ?? trusted} disabled={waiting} onChange={event => setTrusted(event.target.checked)} />我已核对来源和摘要，确认此 ZIP 可信，允许执行其中内核进行诊断。</label></> : <p>版本与摘要由用户核对官方发行资产填写；上游说明不保证资产可获取。失败不自动换版本；没有硬编码参考归档版本。</p>}
    <p>安装会执行该精确构建的隔离诊断。归档摘要、程序身份和参数读回通过后才登记，不把下载或元数据当已安装。真实进程、网络和兼容性仍按已有证据范围验收。</p>{task.message && <p role="status" className="pk35-message">{task.message}</p>}</form>
  </ProxyKernelModal>}
  {progressOpen && operation && <ProxyKernelModal title="内核任务" width={400} onClose={() => setProgressOpen(false)} className="kernel35-progress" footer={<><button className="button" onClick={() => setProgressOpen(false)}>关闭</button>{!operationIsTerminal(operation) && <button className="button" disabled={operation.cancelRequested || operation.persistencePending} onClick={async () => { const targetId = operation.id; try { const response = await application.cancelOperation(targetId); if (owner.getSnapshot().operation?.id !== targetId) return; if (response.ok) owner.observe(response.data, targetId); else owner.message(`${response.error.code}：${response.error.message}`); } catch { if (owner.getSnapshot().operation?.id === targetId) owner.message("取消结果未知，继续核对原任务。"); } }}>取消此任务</button>}{operationIsTerminal(operation) && operation.state !== "completed" && <button className="button primary" disabled={waiting} onClick={() => { setProgressOpen(false); setPrepareOpen(true); }}>重新准备</button>}</>}><p>任务：{stageLabels[operation.stage ?? operation.state] ?? operation.state}</p><p className="mono">{operation.id}</p><div className="kernel35-progress-states"><span>受理 1 项</span><span className={operation.state === "completed" && !operation.persistencePending ? "success" : ""}>已完成 {operation.state === "completed" && !operation.persistencePending ? 1 : 0}</span><span className={operation.state === "failed" ? "failure" : ""}>失败 {operation.state === "failed" ? 1 : 0}</span></div>{operation.persistencePending && <p className="pk35-warning">结果待保存，尚非持久终态；继续保留原任务保护。</p>}{operation.error && <p className="pk35-warning">{operation.error.code}：{operation.error.message}</p>}<p>关闭只隐藏窗口，不取消已受理任务。取消仅影响此任务；迟到回执不能覆盖后续任务。</p>{task.message && <p className="pk35-message" role="status">{task.message}</p>}</ProxyKernelModal>}
  {deleting && <ProxyKernelModal title="移除内核确认" width={400} onClose={() => setConfirmDelete("")} footer={<><button className="button" onClick={() => setConfirmDelete("")}>保留构建</button><button className="button danger" disabled={waiting || !!(deleting.usedCount ?? deleting.usedBy.length) || workspace.defaultKernel?.kernelId === deleting.id} onClick={async () => { if (await owner.begin({ kind: "delete", kernelId: deleting.id, requestId: crypto.randomUUID() })) { setConfirmDelete(""); setProgressOpen(true); } }}>确认移除</button></>}><p>只移除 {deleting.version} 的精确构建，不删除浏览数据。当前、历史引用及默认构建均受服务保护。</p>{task.message && <p role="status">{task.message}</p>}</ProxyKernelModal>}
  {detail && <KernelDetailsWindow record={detail} onClose={() => setDetailId("")} />}
  {historyOpen && <ProxyKernelModal title="最近内核任务（持久记录）" width={620} height={540} onClose={() => setHistoryOpen(false)} footer={<button className="button primary" onClick={() => setHistoryOpen(false)}>确定</button>}><table className="pk35-table"><thead><tr><th>任务 / 精确构建</th><th>状态</th><th>查看</th></tr></thead><tbody>{(workspace.kernelOperations ?? []).map(item => <tr key={item.id}><td>{item.id}<span className="pk35-secondary">{item.kernelId ?? "尚未登记"}</span></td><td>{stageLabels[item.stage ?? item.state] ?? item.state}{item.persistencePending && " · 待保存"}{item.error && <p>{item.error.code}：{item.error.message}</p>}{item.report && <details><summary>本次复验采样 {item.report.sampledAt}</summary><p>{item.report.adapterVersion} · {item.report.version}</p>{item.report.observations.map((sample, index) => <p key={index}>seed {sample.seed} · CPU {sample.cpu} · {sample.language} · {sample.timezone} · 实际版本 {sample.browserVersion} · 正常退出 {sample.normalExit ? "是" : "否"}</p>)}</details>}</td><td><button className="pk35-link" disabled={!!activeId || task.unknown} onClick={() => { owner.select(item); setHistoryOpen(false); setProgressOpen(true); }}>查看任务</button></td></tr>)}</tbody></table>{!workspace.kernelOperations?.length && <p>暂无持久内核任务记录。</p>}</ProxyKernelModal>}</>;
}
