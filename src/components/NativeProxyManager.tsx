import { useEffect, useState, useSyncExternalStore } from "react";
import { mergeOperation, operationIsTerminal, type ApplicationService, type NativeProxy, type Operation, type ProxyConfiguration, type WorkspaceView } from "../application/contract";
import { proxyResolutionLabel, proxyStageLabel } from "../application/proxy-network";
import { ProxyManagementPage } from "./ProxyManagementPage";
import { ProxyImportWindow } from "./ProxyImportWindow";
import { ProxyEditWindow } from "./ProxyEditWindow";
import { ProxyUsageWindow } from "./ProxyUsageWindow";
import { ProxyKernelModal } from "./ProxyKernelModal";
import { getNativeProxyActions } from "./native-proxy-actions";
import "./native-proxy.css";

export interface NativeProxyManagerProps {
  application: ApplicationService; workspace: WorkspaceView; importOpen: boolean; onImportOpenChange: (open: boolean) => void;
  importOnly?: boolean; onImported?: (ids: string[]) => void; onBusyChange?: (busy: boolean) => void;
  onAssign?: (ids?: string[]) => void; visible?: boolean; modalLifecycle?: "self" | "parent";
}
export function NativeProxyManager({ application, workspace, importOpen, onImportOpenChange, importOnly = false, onImported, onBusyChange, onAssign, visible = true, modalLifecycle = "self" }: NativeProxyManagerProps) {
  const owner = getNativeProxyActions(application), actions = useSyncExternalStore(owner.subscribe, owner.getSnapshot, owner.getSnapshot);
  const records = workspace.nativeProxyRecords ?? [], pending = actions.pending;
  const [editing, setEditing] = useState<NativeProxy>(), [configuration, setConfiguration] = useState<ProxyConfiguration>();
  const [editHidden, setEditHidden] = useState(false);
  const [credentialAction, setCredentialAction] = useState<"keep" | "replace" | "clear">("keep"), [username, setUsername] = useState(""), [password, setPassword] = useState("");
  const [deleting, setDeleting] = useState<NativeProxy>(), [usage, setUsage] = useState<NativeProxy>(), [detail, setDetail] = useState<NativeProxy>();
  const [importMode, setImportMode] = useState<"text" | "file" | "single">("text");
  const operations = new Map<string, Operation>();
  for (const operation of workspace.proxyOperations ?? []) operations.set(operation.id, operation);
  for (const operation of Object.values(actions.operations)) operations.set(operation.id, mergeOperation(operations.get(operation.id), operation));
  const active = [...operations.values()].filter(operation => !operationIsTerminal(operation)), activeKey = active.map(operation => operation.id).sort().join(",");
  const activeByProxy = new Map(active.map(operation => [operation.proxyId, operation]));
  const busy = actions.busy || actions.unknown || !!workspace.issue;
  useEffect(() => { onBusyChange?.(actions.busy); }, [actions.busy, onBusyChange]);
  useEffect(() => {
    if (pending?.kind !== "update") return;
    setConfiguration(pending.request.configuration);
    setCredentialAction(pending.request.credentials.action);
    setUsername(pending.request.credentials.action === "replace" ? pending.request.credentials.username : "");
    setPassword(pending.request.credentials.action === "replace" ? pending.request.credentials.password : "");
  }, [pending]);
  useEffect(() => {
    if (!activeKey) return;
    let disposed = false, timer: ReturnType<typeof setTimeout>;
    async function read() {
      let stillActive = false, completed = false;
      for (const id of activeKey.split(",")) {
        try {
          const response = await application.getOperation(id); if (disposed) return;
          if (!response.ok) { stillActive = true; owner.message(`${response.error.code}：${response.error.message} 结果尚未确认，继续读取。`); continue; }
          owner.observe(response.data);
          if (!operationIsTerminal(owner.getSnapshot().operations[id])) stillActive = true;
          else { completed = true; owner.message(response.data.state === "completed" ? "本次检查已完成；历史结果不保证下次启动可用或永久保护。" : `${response.data.error?.code ?? response.data.state}：${response.data.error?.message ?? "检查未完成"}`); }
        } catch { if (!disposed) { stillActive = true; owner.message("检查任务读取未确认，继续保留原任务。"); } }
      }
      if (completed) await application.refresh?.();
      if (!disposed && stillActive) timer = setTimeout(read, 750);
    }
    void read(); return () => { disposed = true; clearTimeout(timer); };
  }, [application, owner, activeKey]);
  function closeEdit() { setEditHidden(true); setEditing(undefined); setConfiguration(undefined); setUsername(""); setPassword(""); setCredentialAction("keep"); }
  function edit(record: NativeProxy) { setEditHidden(false); setEditing(record); setConfiguration({ name: record.name, type: record.type, host: record.host, port: record.port, country: record.country }); setUsername(""); setPassword(""); setCredentialAction("keep"); }
  async function save() {
    if (actions.unknown) { if (await owner.retry()) closeEdit(); return; }
    if (!editing || !configuration) return;
    if (await owner.run({ kind: "update", request: { proxyId: editing.id, expectedRevision: editing.revision, requestId: crypto.randomUUID(), configuration, credentials: credentialAction === "replace" ? { action: "replace", username, password } : { action: credentialAction } } })) closeEdit();
  }
  async function check(ids: string[]) {
    for (const id of ids) {
      if (owner.getSnapshot().unknown) return;
      const record = records.find(record => record.id === id); if (!record || activeByProxy.has(id)) continue;
      await owner.run({ kind: "check", request: { proxyId: id, expectedRevision: record.revision, requestId: crypto.randomUUID() } });
    }
  }
  const effectiveEdit = editing ?? (!editHidden && pending?.kind === "update" ? records.find(record => record.id === pending.request.proxyId) : undefined);
  const editConfiguration = actions.unknown && pending?.kind === "update" ? pending.request.configuration : configuration;
  const reportRecord = (detail && records.find(record => record.id === detail.id)) ?? detail;
  const reportTask = reportRecord && operations.size ? [...operations.values()].filter(op => op.proxyId === reportRecord.id).at(-1) : undefined;
  const report = reportTask?.proxyReport ?? reportRecord?.checkReport;
  return <>
    {visible && !importOnly && <ProxyManagementPage native records={records.map(record => { const task = activeByProxy.get(record.id); return { ...record, usedCount: record.usedCount ?? record.usedBy.length, deleteProtected: !!(record.usedCount ?? record.usedBy.length), busy: !!task, cancelDisabled: !!task?.cancelRequested || !!task?.persistencePending, statusText: task ? task.persistencePending ? "结果待保存" : proxyStageLabel(task.stage ?? "queued") : undefined, exitIp: record.checkReport?.exitIp }; })} busy={busy} message={actions.message} onImport={mode => { setImportMode(mode); onImportOpenChange(true); }} onEdit={id => { const record = records.find(record => record.id === id); if (record) edit(record); }} onDelete={id => setDeleting(records.find(record => record.id === id))} onCheck={ids => void check(ids)} onUsage={id => setUsage(records.find(record => record.id === id))} onDetails={id => setDetail(records.find(record => record.id === id))} onRefresh={() => void application.refresh?.()} onAssign={onAssign ? () => onAssign() : undefined} onCancelCheck={id => { const task = activeByProxy.get(id); if (!task || task.persistencePending) return; void application.cancelOperation(task.id).then(result => { if (result.ok) owner.observe(result.data); else owner.message(`${result.error.code}：${result.error.message}`); }).catch(() => owner.message("取消结果未知，请核对原检查任务。")); }}>
      {actions.unknown && <button className="button primary" disabled={actions.busy} onClick={() => void owner.retry()}>核实原代理{pending?.kind === "update" ? "保存" : pending?.kind === "delete" ? "删除" : "检查"}请求</button>}
    </ProxyManagementPage>}
    <ProxyImportWindow application={application} open={importOpen && visible} onClose={() => onImportOpenChange(false)} onImported={onImported} onBusyChange={onBusyChange} context={importOnly ? "environment" : "page"} initialMode={importMode} lifecycle={modalLifecycle} />
    {visible && effectiveEdit && editConfiguration && <ProxyEditWindow configuration={editConfiguration} onConfiguration={setConfiguration} credentialAction={credentialAction} onCredentialAction={value => { setUsername(""); setPassword(""); setCredentialAction(value); }} username={username} password={password} onUsername={setUsername} onPassword={setPassword} hasAuthentication={effectiveEdit.hasAuthentication} native busy={actions.busy || !!workspace.issue} unknown={actions.unknown} message={actions.message} onClose={closeEdit} onSave={() => void save()} />}
    {visible && deleting && <ProxyKernelModal title="删除代理确认" width={400} onClose={() => setDeleting(undefined)} busy={actions.busy} footer={<><button className="button" disabled={actions.busy} onClick={() => setDeleting(undefined)}>取消</button><button className="button danger" disabled={actions.busy || !!workspace.issue} onClick={async () => { if (await owner.run({ kind: "delete", request: { proxyId: deleting.id, expectedRevision: deleting.revision, requestId: crypto.randomUUID() } })) setDeleting(undefined); }}>确认删除</button></>}><p>确定删除 {deleting.name}？只删除本机配置及其受保护认证，不改变设备身份；被引用节点不能删除。</p>{actions.message && <p role="status">{actions.message}</p>}</ProxyKernelModal>}
    {visible && usage && <ProxyUsageWindow name={usage.name} ids={usage.usedBy} total={usage.usedCount ?? usage.usedBy.length} workspace={workspace} onClose={() => setUsage(undefined)} onAssign={onAssign ? ids => { setUsage(undefined); onAssign(ids); } : undefined} />}
    {visible && reportRecord && <ProxyKernelModal title={`代理检查详情 · ${reportRecord.name}`} width={620} onClose={() => setDetail(undefined)} footer={<button className="button primary" onClick={() => setDetail(undefined)}>确定</button>}><p>修订 {reportRecord.revision} · {reportTask?.persistencePending ? "结果待保存，尚非持久终态" : reportRecord.status === "unchecked" ? "尚未检查" : reportRecord.status === "failed" ? "检查未通过" : "本次检查通过"}</p>{report ? <><p>本次观测 IP：{report.exitIp ?? "未取得"} · {report.durationMs}ms<br />{report.finishedAt ? new Date(report.finishedAt).toLocaleString() : "未完成"}</p>{report.error && <p className="pk35-warning">{report.error.code}：{report.error.message}</p>}<table className="pk35-table"><thead><tr><th>阶段</th><th>结果</th><th>说明</th></tr></thead><tbody>{report.steps.map((step, index) => <tr key={index}><td>{proxyStageLabel(step.stage)}</td><td>{step.status === "passed" ? "通过" : step.status === "running" ? "进行中" : step.status === "unsupported" ? "不支持" : "失败"}</td><td>{step.message}</td></tr>)}</tbody></table>{report.resolutionPolicy && <p>{proxyResolutionLabel(report.resolutionPolicy)}</p>}<p>目标：{report.targetOrigin || "尚未访问目标"}；不跟随重定向，不跳过 TLS 验证。</p></> : <p>暂无本次安全观测报告，不推断出口或认证成功。</p>}<p>独立临时通道观测不是环境启动许可；每次启动重新核对自己的通道。人工 UI 与独立远端全路径仍待验，不作永久保护承诺。</p></ProxyKernelModal>}
  </>;
}
