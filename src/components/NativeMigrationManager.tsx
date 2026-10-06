import { useEffect, useRef, useState } from "react";
import { mergeOperation, operationIsTerminal, type ApplicationService, type NativeMigrationPreview, type NativeRestorePreview, type Operation, type WorkspaceView } from "../application/contract";
import { NativeRestoreExecution } from "./NativeRestoreExecution";
import { ProxyKernelModal } from "./ProxyKernelModal";
import "./native-kernel.css";

const stages: Record<string, string> = { accepted: "制作升级前完整备份", "backup-ready": "完整备份已核对", "copy-ready": "独立工作副本已准备", "trial-starting": "启动副本并读取实际参数", "trial-running": "正在试用新构建副本", ready: "副本已停止，等待明确确认", prepared: "准备切换目录", committed: "配置已提交，核对目录", completed: "迁移已完成", "original-retained": "原状态已保留", protected: "结果未确认，保持保护", "storage-pending": "结果待保存", "acceptance-pending": "原请求待核实" };

export function NativeMigrationManager({ application, workspace, open, onOpenChange }: { application: ApplicationService; workspace: WorkspaceView; open?: boolean; onOpenChange?: (open: boolean) => void }) {
  const [localOpen, setLocalOpen] = useState(false), [resultOpen, setResultOpen] = useState(false), [confirmation, setConfirmation] = useState<"trial" | "switch">();
  const visible = open ?? localOpen;
  const setVisible = (value: boolean) => { setLocalOpen(value); onOpenChange?.(value); };
  const [environmentId, setEnvironmentId] = useState("");
  const [lookup, setLookup] = useState({ items: [] as Pick<WorkspaceView["state"]["environments"][number], "id" | "name" | "coreId" | "status">[], page: 1, total: 0 });
  const [lookupBusy, setLookupBusy] = useState(true);
  const lookupSequence = useRef(0);
  const [lookupSearch, setLookupSearch] = useState("");
  const [kernelId, setKernelId] = useState("");
  const [preview, setPreview] = useState<NativeMigrationPreview>();
  const [policy, setPolicy] = useState<{ networkPolicy: "direct" | "proxy"; proxyId?: string }>();
  const [restore, setRestore] = useState<NativeRestorePreview>();
  const [restoreLocked, setRestoreLocked] = useState(false);
  const [confirmTrial, setConfirmTrial] = useState(false), [confirmSwitch, setConfirmSwitch] = useState(false);
  const [switchTarget, setSwitchTarget] = useState<string>();
  const [busy, setBusy] = useState(false), [message, setMessage] = useState("");
  const [pending, setPending] = useState(() => application.getPendingMigration?.());
  const [operation, setOperation] = useState<Operation>();
  const [target, setTarget] = useState(workspace.migrationMaintenance?.id ?? application.getPendingMigration?.()?.operationId ?? workspace.migrationOperations?.[0]?.id);
  const alive = useRef(true), flight = useRef(false), targetRef = useRef(target);
  const submitted = useRef(application.getPendingMigration?.()?.request);
  const current = lookup.items.find(e => e.id === environmentId);
  const maintenance = workspace.migrationMaintenance;
  const active = !!maintenance && !(operation?.id === maintenance.id && operationIsTerminal(operation)) || !!operation && !operationIsTerminal(operation);
  const locked = busy || lookupBusy || active || !!pending || restoreLocked || !!workspace.issue || !!workspace.maintenance || !!workspace.recycleMaintenance;
  const report = operation?.migrationReport;
  // The migration's own maintenance lease is expected during trial/ready.
  // A restore/recycle lease, another migration owner, or an unconfirmed phase
  // must revoke an already-open confirmation, not just the selection form.
  const ownsTask = !!operation && operation.id === targetRef.current;
  const commandLocked = busy || restoreLocked || !!workspace.issue || !!workspace.maintenance || !!workspace.recycleMaintenance || !!maintenance && maintenance.id !== operation?.id;
  const canStop = ownsTask && !commandLocked && !operationIsTerminal(operation!) && (operation!.stage === "trial-running" || !!operation!.persistencePending && !!report?.after && !report.trialExited);
  const canSwitch = ownsTask && !commandLocked && !pending && operation!.stage === "ready" && !operation!.persistencePending && !operation!.cancelRequested && !!report?.backupVerified && report.trialExited && !report.committed;
  const canCancel = ownsTask && !commandLocked && active && !operation!.cancelRequested && !report?.committed;
  const canRecover = ownsTask && !commandLocked && !!operation!.persistencePending;
  useEffect(() => { alive.current = true; return () => { alive.current = false; void application.discardMigrationRollback?.(); }; }, []);
  useEffect(() => { void loadEnvironments(1, ""); return () => { lookupSequence.current++; }; }, [application]);
  function observe(op: Operation) { if (op.kind === "migration" && op.id === targetRef.current) setOperation(old => mergeOperation(old?.id === op.id ? old : undefined, op)); }
  function select(id: string) { targetRef.current = id; setTarget(id); setOperation(undefined); setConfirmSwitch(false); setSwitchTarget(undefined); setConfirmation(value => value === "switch" ? undefined : value); }
  useEffect(() => {
    setPending(application.getPendingMigration?.());
    if (submitted.current && application.wasMigrationNotAccepted?.(submitted.current.requestId)) { submitted.current = undefined; targetRef.current = undefined; setTarget(undefined); setOperation(undefined); }
    const owner = workspace.migrationMaintenance;
    if (owner && owner.id !== targetRef.current) select(owner.id);
    for (const op of workspace.migrationOperations ?? []) observe(op);
    if (owner) observe(owner);
  }, [workspace]);
  useEffect(() => {
    if (!target || operation && operationIsTerminal(operation)) return;
    let disposed = false, reading = false;
    const read = async () => {
      if (reading) return; reading = true;
      try {
        const result = await application.getOperation(target);
        if (disposed || !alive.current || targetRef.current !== target) return;
        if (result.ok) { observe(result.data); setPending(application.getPendingMigration?.()); if (operationIsTerminal(result.data)) await application.refresh?.(); }
        else setMessage(`${result.error.code}：${result.error.message}`);
      } catch { if (!disposed && alive.current) setMessage("本机任务读取未确认，请重试。"); }
      finally { reading = false; }
    };
    void read(); const timer = setInterval(() => { void read(); }, 1000);
    return () => { disposed = true; clearInterval(timer); };
  }, [application, target, operation?.state, operation?.persistencePending]);

  async function makePreview() {
    if (flight.current || locked || !environmentId || !kernelId) return;
    flight.current = true; setBusy(true); setMessage(""); setPreview(undefined); setPolicy(undefined); setConfirmTrial(false);
    try {
      // Lookup pages don't include proxy policy. Read this exact target, even
      // off-page, and never infer "direct" from missing workspace rows.
      const saved = await application.previewEnvironment({ kind: "edit", sourceId: environmentId });
      if (!alive.current) { if (saved.ok) void application.discardPreview(saved.data.previewId); return; }
      if (!saved.ok) { setMessage(`${saved.error.code}：${saved.error.message} 网络策略未读取，不允许试用。`); return; }
       if (saved.mode !== "native" || saved.data.environment.id !== environmentId || typeof saved.data.environment.proxyId !== "string" || !Number.isSafeInteger(saved.data.expectedRevision)) { void application.discardPreview(saved.data.previewId); setMessage("迁移目标、修订与网络策略未确认，不能猜测直连。"); return; }
      const savedPolicy = { networkPolicy: saved.data.environment.proxyId ? "proxy" as const : "direct" as const, proxyId: saved.data.environment.proxyId || undefined };
      await application.discardPreview(saved.data.previewId);
      const result = await application.previewMigration?.(environmentId, kernelId);
      if (!alive.current) return;
       if (result?.ok) {
         if (result.data.environmentId !== environmentId || result.data.expectedRevision !== saved.data.expectedRevision) { setMessage("环境配置在读取网络策略后变化；请重新读取迁移差异，不能沿用旧策略确认试用。"); return; }
         setPreview(result.data); setPolicy(savedPolicy);
       } else setMessage(result ? `${result.error.code}：${result.error.message}` : "迁移服务不可用。");
    } catch { if (alive.current) setMessage("迁移预览未确认，请重试。"); }
    finally { flight.current = false; if (alive.current) setBusy(false); }
  }
  async function loadEnvironments(page: number, search: string) {
    const sequence = ++lookupSequence.current; setLookupBusy(true);
    try { const result = await application.lookupMigrationEnvironments?.(page, search); if (!alive.current || sequence !== lookupSequence.current) return; if (result?.ok) { setLookup(result.data); setEnvironmentId(""); setPreview(undefined); } else setMessage(result ? result.error.message : "本机环境查找不可用。"); }
    catch { if (alive.current && sequence === lookupSequence.current) setMessage("环境查找失败，请重试。"); }
    finally { if (alive.current && sequence === lookupSequence.current) setLookupBusy(false); }
  }
  function findEnvironments(page: number) {
    if (flight.current || locked) return;
    return loadEnvironments(page, lookupSearch);
  }
  async function prepare() {
    const original = application.getPendingMigration?.();
    if (flight.current || !original && (locked || !preview || !confirmTrial || !policy)) return;
    flight.current = true; setBusy(true); setMessage("");
    const request = original?.request ?? { previewId: preview!.previewId, confirm: true, requestId: crypto.randomUUID() }; submitted.current = request;
    try {
      const result = await application.prepareMigration?.(request);
      if (!alive.current) return;
      if (result?.ok) { select(result.data.operation.id); observe(result.data.operation); setPreview(undefined); setConfirmation(undefined); setResultOpen(true); }
      else { setMessage(result ? `${result.error.code}：${result.error.message}` : "迁移服务不可用。"); if (result?.operationId) select(result.operationId); if (application.wasMigrationNotAccepted?.(request.requestId)) { submitted.current = undefined; targetRef.current = undefined; setTarget(undefined); setOperation(undefined); } }
    } catch { if (alive.current) setMessage("原迁移结果未确认，请核实原请求。"); }
    finally { flight.current = false; if (alive.current) { setBusy(false); setPending(application.getPendingMigration?.()); } }
  }
  async function action(kind: "stop" | "commit" | "recover" | "cancel") {
    if (flight.current || !targetRef.current || kind === "stop" && !canStop || kind === "cancel" && !canCancel || kind === "recover" && !canRecover || kind === "commit" && (!canSwitch || !confirmSwitch || switchTarget !== targetRef.current)) return;
    const id = targetRef.current; flight.current = true; setBusy(true); setMessage("");
    try {
      const result = kind === "cancel" ? await application.cancelOperation(id) : await application.migrationAction?.(id, kind);
      if (!alive.current || id !== targetRef.current) return;
      if (result?.ok) { observe(result.data); if (operationIsTerminal(result.data)) await application.refresh?.(); } else setMessage(result ? `${result.error.code}：${result.error.message}` : "迁移服务不可用。");
    } catch { if (alive.current) setMessage("迁移操作结果未确认，请读取原任务核对。"); }
    finally { flight.current = false; if (alive.current) { setBusy(false); setPending(application.getPendingMigration?.()); } }
  }
  async function rollback(id: string) {
    if (flight.current || locked) return;
    flight.current = true; setBusy(true); setMessage(""); setRestore(undefined);
    try {
      await application.discardMigrationRollback?.();
      if (!alive.current) return;
      const result = await application.previewMigrationRollback?.(id);
      if (!alive.current) return;
      if (result?.ok) setRestore(result.data); else setMessage(result ? `${result.error.code}：${result.error.message}` : "恢复预检不可用。");
    } catch { if (alive.current) setMessage("升级前恢复预检未确认，请重试。"); }
    finally { flight.current = false; if (alive.current) setBusy(false); }
  }
  return <>
    {open === undefined && <div className="migration35-entry"><button className="button primary" onClick={() => setVisible(true)}>选定环境迁移</button><span>先完整备份，再用独立副本试用；旧身份与默认选择分开。</span>{(operation || pending) && <button className="button" onClick={() => setResultOpen(true)}>查看迁移任务</button>}</div>}
    {visible && <ProxyKernelModal title="选定环境内核迁移" width={1040} height={592} onClose={() => setVisible(false)} className="migration35-selection" footer={<><span className="pk35-footer-note">一次一个明确环境；关闭不取消已受理任务</span><button className="button" onClick={() => setVisible(false)}>关闭</button>{(operation || pending) && <button className="button" onClick={() => setResultOpen(true)}>查看迁移任务</button>}{preview && <button className="button primary" disabled={locked || !policy} onClick={() => { setConfirmTrial(false); setConfirmation("trial"); }}>备份并试用副本</button>}</>}>
    <p>先在环境页正常停止目标。制作并核对完整备份后，用独立副本试用新构建；seed 保持不变，原环境保持停止。确认切换后才使用试用过的数据。一次明确选择一个环境。</p>
    <div className="native-kernel-actions"><input aria-label="查找迁移环境" placeholder="按环境名称或编号查找" disabled={locked} value={lookupSearch} onChange={e => setLookupSearch(e.target.value)} /><button className="button" disabled={locked} onClick={() => void findEnvironments(1)}>查找环境</button><button className="button" disabled={locked || lookup.page <= 1} onClick={() => void findEnvironments(lookup.page - 1)}>上一页</button><button className="button" disabled={locked || lookup.page * 25 >= lookup.total} onClick={() => void findEnvironments(lookup.page + 1)}>下一页</button><span>第 {lookup.page} 页 · 共 {lookup.total} 个</span></div>
    <div className="migration35-fields">
      <label className="reference-field"><span>选定环境</span><select aria-label="选定环境" disabled={locked} value={environmentId} onChange={e => { setEnvironmentId(e.target.value); setPreview(undefined); setPolicy(undefined); }}><option value="">请选择环境</option>{lookup.items.map(e => <option key={e.id} value={e.id}>{e.name} · {e.status}</option>)}</select></label>
      <label className="reference-field"><span>目标精确构建</span><select aria-label="目标精确构建" disabled={locked} value={kernelId} onChange={e => { setKernelId(e.target.value); setPreview(undefined); setPolicy(undefined); }}><option value="">请选择不同构建</option>{workspace.kernelRecords?.filter(k => k.status === "verified" && k.id !== current?.coreId).map(k => <option key={k.id} value={k.id}>{k.version} · {k.id}</option>)}</select></label>
    </div>
    <button className="button" disabled={locked || !environmentId || !kernelId} onClick={() => void makePreview()}>查看迁移差异</button>
    {preview && <>
      <h3>{preview.name}：{preview.before.coreActualVersion} → {preview.after.coreActualVersion}</h3>
      <p>固定 seed：{preview.before.seed} · 原修订：{preview.expectedRevision} · 已保存网络策略：{policy?.networkPolicy === "proxy" ? `绑定代理（${policy.proxyId}），失败不切直连` : policy?.networkPolicy === "direct" ? "未绑定代理，明确直连" : "未确认，不能试用"}</p>
      <div className="migration35-diff"><table className="pk35-table"><thead><tr><th>变化字段</th><th>原构建</th><th>目标构建</th></tr></thead><tbody>{preview.changes.map(c => <tr key={c.field}><td>{c.field}</td><td>{c.before}</td><td>{c.after}</td></tr>)}</tbody></table>
      <details><summary>比较能力与实际下发参数</summary><div className="migration35-fields">{(["before", "after"] as const).map(side => <div key={side}><h4>{side === "before" ? "原构建" : "目标构建"}</h4><pre>{preview[side].parameters.join("\n")}</pre>{preview[side === "before" ? "beforeCapabilities" : "afterCapabilities"].map(c => <p key={c.field}>{c.field} · {c.status} · {c.source}<br />{c.note}</p>)}</div>)}</div></details></div>
    </>}
    {pending && <button className="button primary" disabled={busy} onClick={() => void prepare()}>核实原迁移请求</button>}
    {pending && <p role="status">原请求 {pending.request.requestId} 待核实；保留原请求，不另建迁移。</p>}
    {message && <p role="alert">{message}</p>}
    {!!workspace.migrationOperations?.length && <details><summary>迁移记录与升级前恢复</summary>{workspace.migrationOperations.map(op => <div key={op.id}><p>{op.environmentId} · {stages[op.stage ?? ""] ?? op.state}</p><button className="button" disabled={busy || active} onClick={() => { select(op.id); observe(op); setResultOpen(true); }}>查看记录</button><button className="button" disabled={locked || !operationIsTerminal(op) || !op.migrationReport?.backupVerified} onClick={() => void rollback(op.id)}>预检升级前完整恢复</button></div>)}</details>}
    {restore && <><h3>升级前完整恢复预检</h3><p>备份时间：{new Date(restore.createdAt).toLocaleString()} · 将恢复 {restore.environmentCount} 个原环境，覆盖 {restore.overwriteCount} 项；缺失精确构建 {restore.missingKernelCount} 项，配置冲突 {restore.conflictCount} 项。备份后的浏览数据与配置将被撤回。</p>{restore.kernels.map(k => <p key={k.id}>{k.version} · {k.state}</p>)}</>}
    <NativeRestoreExecution application={application} workspace={workspace} preview={restore} onConsumed={id => { application.consumeMigrationRollback?.(id); setRestore(undefined); }} onLockChange={setRestoreLocked} />
    </ProxyKernelModal>}
    {resultOpen && <ProxyKernelModal title="迁移任务" width={400} onClose={() => setResultOpen(false)} footer={<><button className="button" onClick={() => setResultOpen(false)}>关闭</button>{pending && <button className="button primary" disabled={busy} onClick={() => void prepare()}>核实原迁移请求</button>}</>}>
      {pending && <p role="status">原请求 {pending.request.requestId} 待核实；不另建迁移。</p>}
      {operation && report && <><h3>{stages[operation.stage ?? ""] ?? operation.state}</h3><p className="mono">{operation.id}</p><p>完整备份：{report.backupVerified ? "已核对" : "尚未完成"} · 试用全树退出：{report.trialExited ? "已确认" : "尚未确认"} · 配置切换：{report.committed ? "已提交" : "尚未提交"}</p>{report.archiveSha256 && <p>升级前包 SHA-256：{report.archiveSha256}</p>}{[report.before, report.after].map((value, index) => value && <p key={index}>{index === 0 ? "旧" : "新"}构建副本实际采样：{value.fingerprint.browserVersion} · CPU {value.fingerprint.cpu} · {value.fingerprint.language} · {value.fingerprint.timezone} · Cookie / LocalStorage / IndexedDB：{value.cookie && value.localStorage && value.indexedDB ? "合成标记读回一致" : "未一致"} · {value.sampledAt}</p>)}<p>合成存储标记不代表所有网站登录兼容，需试用必要流程。</p>{operation.error && <p role="alert">{operation.error.code}：{operation.error.message}</p>}{report.protected && <p role="status">维护保护中；未知不会解除目录保护，也不让旧内核打开升级数据。</p>}<div className="native-kernel-actions">{(operation.stage === "trial-running" || operation.persistencePending && !!report.after && !report.trialExited) && <button className="button" disabled={!canStop} onClick={() => void action("stop")}>正常停止试用副本</button>}{operation.stage === "ready" && <button className="button primary" disabled={!canSwitch} onClick={() => { setConfirmSwitch(false); setSwitchTarget(operation.id); setConfirmation("switch"); }}>明确切换</button>}{active && <button className="button" disabled={!canCancel} onClick={() => void action("cancel")}>取消并保留原状态</button>}{operation.persistencePending && <button className="button" disabled={!canRecover} onClick={() => void action("recover")}>核对原任务</button>}</div></>}{message && <p role="alert" className="pk35-message">{message}</p>}
    </ProxyKernelModal>}
    {confirmation && <ProxyKernelModal title={confirmation === "trial" ? "确认备份并试用副本" : "确认切换选定环境"} width={400} onClose={() => setConfirmation(undefined)} busy={busy} footer={<><button className="button" disabled={busy} onClick={() => setConfirmation(undefined)}>取消</button><button className="button primary" disabled={busy || (confirmation === "trial" ? locked || !confirmTrial || !policy : !canSwitch || switchTarget !== operation?.id || !confirmSwitch)} onClick={() => { if (confirmation === "trial") void prepare(); else { setConfirmation(undefined); void action("commit"); } }}>{confirmation === "trial" ? "确认备份并试用" : "确认切换"}</button></>}>
      {confirmation === "trial" ? <><p>仅试用独立副本，原环境保持停止，尚不切换原目录。{policy?.networkPolicy === "proxy" ? "副本沿用保存的代理绑定与保护链，故障不退回直连。" : "此环境已确认为未绑定代理，副本将明确使用直连。"}</p><label className="pk35-checkbox"><input type="checkbox" checked={confirmTrial} disabled={locked} onChange={e => setConfirmTrial(e.target.checked)} />确认停止原环境，并允许以原身份和上述网络策略读回与可见试用。</label></> : <><p>仅切换此环境到已试用且正常停止的副本，固定 seed 不变。默认不变；退回使用升级前完整恢复，不让旧内核直接打开升级数据。</p><label className="pk35-checkbox"><input type="checkbox" checked={confirmSwitch} disabled={!canSwitch} onChange={e => setConfirmSwitch(e.target.checked)} />已完成必要试用，确认用该副本切换选定环境</label>{!canSwitch && <p role="alert">原任务阶段、退出结果或工作区保护已变化；请关闭确认并重新核对，不会切换其他任务。</p>}</>}{message && <p role="alert">{message}</p>}
    </ProxyKernelModal>}
  </>;
}
