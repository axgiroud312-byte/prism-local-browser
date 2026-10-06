import { useEffect, useRef, useState } from "react";
import { ChevronLeft, ChevronRight, LoaderCircle } from "lucide-react";
import { mergeOperation, operationIsTerminal, type ApplicationResult, type ApplicationService, type NativeBatchPage, type NativeBatchPreviewRequest, type Operation, type WorkspaceView } from "../application/contract";
import { mergeBatchPage } from "../application/batch-model";
import { EnvironmentTaskResult, EnvironmentWindowFrame } from "./EnvironmentDialogParts";
import "./environment-batch.css";

export interface NativeBatchDialogInput { kind: "create" | "clone" | "assign" | "history"; sourceIds?: string[]; initialPage?: NativeBatchPage }
const label = { create: "创建新环境", clone: "复制配置为新身份", assign: "逐项分配代理", history: "持久批次与逐项结果" };
const errorText = (result: ApplicationResult<unknown> | undefined) => !result ? "此桌面版本未提供批次能力，没有模拟成功。" : result.ok ? "" : `${result.error.code}：${result.error.message}`;

export function NativeBatchDialog({ application, workspace, input, onClose }: { application: ApplicationService; workspace: WorkspaceView; input: NativeBatchDialogInput; onClose: () => void }) {
  const [page, setPage] = useState<NativeBatchPage | undefined>(input.initialPage);
  const [operation, setOperation] = useState<Operation | undefined>();
  const [mapping, setMapping] = useState<Record<string, string>>({});
  const [commonProxy, setCommonProxy] = useState("__unset__");
  const [confirmDirect, setConfirmDirect] = useState(false);
  const [confirmShared, setConfirmShared] = useState(false);
  const [message, setMessage] = useState("");
  const [busy, setBusy] = useState(false);
  const [readingPage, setReadingPage] = useState(false);
  const [detailsIssue, setDetailsIssue] = useState(false);
  const [lookup, setLookup] = useState("");
  const [showResult, setShowResult] = useState(false);
  const mounted = useRef(true);
  const reading = useRef(false);
  const generation = useRef(0);
  const target = useRef<{ planId: string; operationId?: string; offset: number } | undefined>(input.initialPage && { planId: input.initialPage.planId, operationId: input.initialPage.operationId, offset: input.initialPage.offset });
  const mutating = useRef(false);
  const automaticLoads = useRef(new Set<string>());
  const submit = useRef<{ key: string; requestId: string } | undefined>(undefined);
  const modal = useRef<HTMLDivElement>(null);
  const resultTrigger = useRef<HTMLButtonElement>(null);
  const previousResultOpen = useRef(false);
  const closeRef = useRef(onClose); closeRef.current = () => showResult ? setShowResult(false) : onClose();
  const pageRef = useRef(page); pageRef.current = page;
  const records = workspace.batchOperations ?? [];
  const acceptedRecord = records.find(record => record.batchReport?.planId === page?.planId);
  const acceptedId = page?.operationId ?? (target.current?.planId === page?.planId ? target.current?.operationId : undefined) ?? acceptedRecord?.id;
  const pageIsSelected = !!page && target.current?.planId === page.planId && target.current.offset === page.offset && (!target.current.operationId || target.current.operationId === page.operationId);
  const operationReady = pageIsSelected && !!page?.operationId && operation?.id === page.operationId && operation.batchReport?.planId === page.planId;
  const active = operationReady && !!operation && !operationIsTerminal(operation);
  const locked = busy || readingPage || active || !pageIsSelected;
  const targetIds = input.sourceIds ?? [];
  const needsDirect = (page?.directAssignments ?? 0) > 0;

  useEffect(() => {
    mounted.current = true;
    const previous = document.activeElement as HTMLElement | null;
    const overflow = document.body.style.overflow; document.body.style.overflow = "hidden";
    modal.current?.querySelector<HTMLElement>("button")?.focus();
    const key = (event: KeyboardEvent) => {
      if (event.key === "Escape") { event.preventDefault(); event.stopPropagation(); closeRef.current(); }
      if ((event.ctrlKey || event.metaKey) && event.key === "k") { event.preventDefault(); event.stopPropagation(); }
      if (event.key !== "Tab") return;
      const focusable = [...(modal.current?.querySelectorAll<HTMLElement>('button:not(:disabled),input:not(:disabled),select:not(:disabled),summary,[tabindex="0"]') ?? [])].filter(element => element.offsetParent !== null);
      const first = focusable[0], last = focusable.at(-1);
      if (!modal.current?.contains(document.activeElement)) { event.preventDefault(); (event.shiftKey ? last : first)?.focus(); return; }
      if (event.shiftKey && document.activeElement === first) { event.preventDefault(); last?.focus(); }
      if (!event.shiftKey && document.activeElement === last) { event.preventDefault(); first?.focus(); }
    };
    document.addEventListener("keydown", key); void application.refresh?.();
    return () => { mounted.current = false; generation.current++; document.body.style.overflow = overflow; document.removeEventListener("keydown", key); if (previous?.isConnected) previous.focus(); };
  }, [application]);

  useEffect(() => {
    if (showResult) modal.current?.querySelector<HTMLElement>("button:not(:disabled)")?.focus();
    else if (previousResultOpen.current) resultTrigger.current?.focus();
    previousResultOpen.current = showResult;
  }, [showResult]);

  async function loadPage(planId: string, offset = 0, operationId?: string) {
    const current = ++generation.current;
    target.current = { planId, offset, operationId }; setReadingPage(true); setMessage("");
    const result = await application.readBatchPage?.({ planId, operationId, offset, pageSize: 25 });
    if (!mounted.current || current !== generation.current) return;
    if (!result?.ok) { setMessage(errorText(result)); setDetailsIssue(true); setReadingPage(false); return; }
    const tracked = result.data.operationId ? await application.getOperation(result.data.operationId) : undefined;
    if (!mounted.current || current !== generation.current) return;
    if (tracked && (!tracked.ok || tracked.data.batchReport?.planId !== planId || tracked.data.id !== result.data.operationId)) { setMessage(tracked.ok ? "计划和尝试身份不能确认，没有接管其他任务。" : errorText(tracked)); setDetailsIssue(true); setReadingPage(false); return; }
    target.current = { planId, offset, operationId: result.data.operationId };
    setPage(previous => mergeBatchPage(previous, result.data));
    setOperation(previous => tracked?.ok ? previous?.id === tracked.data.id ? mergeOperation(previous, tracked.data) : tracked.data : undefined);
    setDetailsIssue(false); setReadingPage(false);
  }

  async function findHistory() {
    const current = ++generation.current; target.current = undefined; setReadingPage(true); setMessage("");
    const found = await application.getOperation(lookup.trim());
    if (!mounted.current || current !== generation.current) return;
    if (!found.ok || !found.data.batchReport) { setMessage(!found.ok ? errorText(found) : "这个ID不是持久批次。"); setDetailsIssue(true); setReadingPage(false); return; }
    void loadPage(found.data.batchReport.planId, 0, found.data.id);
  }

  function matches(current: number, planId: string, operationId?: string) {
    return mounted.current && generation.current === current && target.current?.planId === planId && (!operationId || target.current.operationId === operationId);
  }

  function automaticallyLoad(key: string, planId: string, offset: number, operationId: string) {
    if (automaticLoads.current.has(key)) return;
    automaticLoads.current.add(key); void loadPage(planId, offset, operationId);
  }

  useEffect(() => {
    if (input.kind !== "clone") return;
    let cancelled = false;
    setBusy(true);
    void (async () => {
      const result = await application.previewBatch?.({ kind: "clone", sourceIds: [...targetIds] });
      if (cancelled || !mounted.current) return; setBusy(false);
      if (result?.ok) { target.current = { planId: result.data.planId, offset: result.data.offset }; setPage(result.data); } else setMessage(errorText(result));
    })();
    return () => { cancelled = true; };
  }, [application, input]);

  useEffect(() => {
    if (readingPage) return;
    const matching = records.find(record => page?.operationId ? record.id === page.operationId : record.batchReport?.planId === page?.planId);
    if (matching?.batchReport && page && matching.batchReport.planId === page.planId) {
      if (!page.operationId) { automaticallyLoad(`adopt:${page.planId}:${matching.id}`, matching.batchReport.planId, page.offset, matching.id); return; }
      if (target.current?.planId === page.planId && target.current.operationId === matching.id) setOperation(previous => mergeOperation(previous?.id === matching.id ? previous : undefined, matching));
      return;
    }
    if (input.kind === "history" && !page && records[0]?.batchReport) automaticallyLoad("initial-history", records[0].batchReport.planId, 0, records[0].id);
  }, [workspace.batchOperations, page?.planId, page?.operationId, readingPage]);

  useEffect(() => {
    if (!operation || !active || readingPage) return;
    const currentGeneration = generation.current;
    const poll = async () => {
      if (reading.current) return; reading.current = true;
      try {
        const result = await application.getOperation(operation.id);
        if (!matches(currentGeneration, operation.batchReport!.planId, operation.id)) return;
        if (!result.ok) { setMessage(errorText(result)); return; }
        const current = pageRef.current;
        if (current && result.data.batchReport?.planId === current.planId) {
          const details = await application.readBatchPage?.({ planId: current.planId, operationId: operation.id, offset: current.offset, pageSize: 25 });
          if (matches(currentGeneration, current.planId, operation.id)) {
            if (details?.ok && pageRef.current?.offset === details.data.offset) { setPage(previous => mergeBatchPage(previous, details.data)); setDetailsIssue(false); }
            else if (!details?.ok) { setDetailsIssue(true); setMessage(`逐项明细未读取成功：${errorText(details)} 请明确重新读取本页，不将旧行当最终结果。`); }
          }
        }
        if (!matches(currentGeneration, operation.batchReport!.planId, operation.id)) return;
        setOperation(previous => previous?.id === operation.id ? mergeOperation(previous, result.data) : previous);
        if (operationIsTerminal(result.data)) await application.refresh?.();
      } finally { reading.current = false; }
    };
    const timer = setInterval(() => { void poll(); }, 750); void poll();
    return () => { clearInterval(timer); };
  }, [application, operation?.id, active, readingPage]);

  // Terminal workspace observations also refresh the exact final page. An
  // active->terminal effect cleanup must never discard this final readback.
  useEffect(() => {
    if (!operationReady || !operation || !operationIsTerminal(operation) || readingPage || !page) return;
    const current = generation.current;
    void (async () => {
      const result = await application.readBatchPage?.({ planId: page.planId, operationId: operation.id, offset: page.offset, pageSize: 25 });
      if (!matches(current, page.planId, operation.id)) return;
      if (result?.ok && pageRef.current?.offset === result.data.offset) { setPage(previous => mergeBatchPage(previous, result.data)); setDetailsIssue(false); }
      else { setDetailsIssue(true); setMessage(`终态计数已确认，但最终逐项明细尚未读取成功：${errorText(result)} 请重新读取本页。`); }
    })();
  }, [application, operationReady, operation?.id, operation?.state, operation?.persistencePending, operation?.batchReport?.sequence, readingPage, page?.offset]);

  async function previewAssignment() {
    if (targetIds.some(environmentId => mapping[environmentId] === undefined || mapping[environmentId] === "__unset__")) { setMessage("请为每个选中环境明确选择节点，或明确选择不绑定代理。不会自动轮询或复用未选节点。"); return; }
    if (mutating.current) return; mutating.current = true; setBusy(true); setMessage("");
    const request: NativeBatchPreviewRequest = { kind: "assign", mappings: targetIds.map(environmentId => ({ environmentId, proxyId: mapping[environmentId] })) };
    const result = await application.previewBatch?.(request);
    mutating.current = false; if (!mounted.current) return; setBusy(false);
    if (result?.ok) { generation.current++; target.current = { planId: result.data.planId, offset: result.data.offset }; setPage(result.data); setOperation(undefined); setConfirmDirect(false); setConfirmShared(false); submit.current = undefined; } else setMessage(errorText(result));
  }

  async function accept(retry = false) {
    if (!page || locked || mutating.current || retry && (!operationReady || page.history || !operation || !operationIsTerminal(operation)) || !retry && (acceptedId || needsDirect && !confirmDirect || page.sharedProxyAssignments > 0 && !confirmShared)) return;
    const current = generation.current; const planId = page.planId; const priorId = retry ? operation?.id : undefined;
    const key = retry ? `retry:${operation?.id}` : `commit:${page.planId}`;
    if (submit.current?.key !== key) submit.current = { key, requestId: crypto.randomUUID() };
    mutating.current = true; setBusy(true); setMessage("");
    const result = retry && operation ? await application.retryBatch?.({ operationId: operation.id, requestId: submit.current.requestId }) : await application.commitBatch?.({ planId: page.planId, requestId: submit.current.requestId });
    mutating.current = false; if (!mounted.current) return; setBusy(false);
    if (!matches(current, planId, priorId)) return;
    if (!result?.ok) { setMessage(errorText(result)); return; }
    if (result.data.operation.batchReport?.planId !== planId) { setMessage("受理响应不是当前计划，未接管它的任务。"); return; }
    submit.current = undefined; setConfirmDirect(false); setConfirmShared(false);
    void loadPage(planId, page.offset, result.data.operation.id);
  }

  async function cancel() {
    if (!operation || !page || !operationReady || locked && !active || busy || readingPage || mutating.current) return;
    const current = generation.current; const planId = page.planId; const operationId = operation.id;
    mutating.current = true; setBusy(true);
    const result = await application.cancelOperation(operation.id);
    mutating.current = false; if (!mounted.current) return; setBusy(false);
    if (!matches(current, planId, operationId)) return;
    if (result.ok && result.data.id === operationId && result.data.batchReport?.planId === planId) setOperation(previous => previous?.id === operationId ? mergeOperation(previous, result.data) : previous); else setMessage(errorText(result));
  }
  const report = operationReady ? operation?.batchReport : undefined;
  const executionActions = <>
    {page && !acceptedId && <button className="button primary" disabled={locked || needsDirect && !confirmDirect || page.sharedProxyAssignments > 0 && !confirmShared} onClick={() => { void accept(); }}>确认执行整个预览计划 {page.total} 项</button>}
    {active && operation && <button className="button" disabled={busy || readingPage || operation.cancelRequested || operation.persistencePending} onClick={() => { void cancel(); }}>只取消此批次剩余项（保留已完成）</button>}
    {operationReady && !page?.history && operation && operationIsTerminal(operation) && operation.state !== "completed" && <button className="button primary" disabled={busy || readingPage} onClick={() => { void accept(true); }}>明确继续未完成项（不重复已完成项）</button>}
  </>;
  return <div className="overlay modal-overlay env34-batch-overlay">
    {showResult && page ? <EnvironmentWindowFrame title="批次执行结果" titleId="native-batch-title" dialogRef={modal} onClose={() => setShowResult(false)} width={400} className="env34-batch-result" footer={<><button className="button" onClick={() => setShowResult(false)}>返回逐项明细</button>{executionActions}</>}>
      <EnvironmentTaskResult title={page.history ? "旧尝试冻结结果" : operation?.persistencePending ? "观测待保存，不重复提交" : active ? "本次进行中" : "本次保存结果"} total={page.total} completed={report?.completedCount ?? page.completedCount} failed={report?.failedCount ?? page.failedCount}>
        <p>未执行 {report?.notExecutedCount ?? page.notExecutedCount} 项；关闭窗口不取消任务，已完成项不撤回。</p>{operation?.error && <p className="env34-error">{operation.error.code}：{operation.error.message}</p>}
        {detailsIssue && <p role="status">计数已确认，最终逐项明细尚未读取成功，返回后重新读取本页。</p>}
      </EnvironmentTaskResult>
    </EnvironmentWindowFrame> : <EnvironmentWindowFrame title={label[input.kind]} titleId="native-batch-title" dialogRef={modal} onClose={onClose} closeLabel="关闭持久批次" width={1040} height={592.25} className="env34-batch-window" footer={<>
      <button className="button" onClick={onClose}>关闭</button>
      {input.kind === "assign" && !page && <button className="button primary" disabled={busy || !targetIds.length} onClick={() => { void previewAssignment(); }}>查看逐项映射预览（不保存绑定）</button>}
      {page && <><button ref={resultTrigger} className="button" disabled={busy || readingPage} onClick={() => setShowResult(true)}>查看批次结果</button>{input.kind === "assign" && !acceptedId && <button className="button" disabled={busy || readingPage} onClick={() => { setPage(undefined); setOperation(undefined); generation.current++; target.current = undefined; }}>返回修改映射（重新预览）</button>}{executionActions}</>}
    </>}>
    <div className="env34-batch-body">
      <p className="env34-batch-policy">关闭不取消任务；继续只处理未完成项，已创建 ID、seed 和目录不重做。native 服务结果，不代表真实桌面已验收。</p>
      {input.kind === "history" && <div className="env34-batch-query"><label>最近30次尝试（旧逐项结果保留，可按任务ID查询）<select aria-label="最近30次尝试" value={page?.operationId ?? ""} disabled={busy || readingPage} onChange={event => { const found = records.find(record => record.id === event.target.value); if (found?.batchReport) void loadPage(found.batchReport.planId, 0, found.id); }}><option value="">选择一次尝试</option>{page?.operationId && !records.some(record => record.id === page.operationId) && <option value={page.operationId}>{page.operationId}</option>}{records.map(record => <option key={record.id} value={record.id}>{label[record.batchReport?.kind ?? "history"]} · {record.state} · {record.id}</option>)}</select></label><label>任务ID<input aria-label="任务ID" value={lookup} onChange={event => setLookup(event.target.value)} /></label><button className="button" disabled={busy || readingPage || !lookup.trim()} onClick={() => { void findHistory(); }}>读取该次保存结果</button></div>}
      {input.kind === "assign" && !page && <>
        <p>范围只包含下面明确选择的 {targetIds.length} 个ID。每个环境可各选不同节点；重复选择同一个节点会明确标注共享，不会自动循环使用代理。</p>
        <div className="env34-batch-query"><label>明确为全部所选环境使用同一节点<select value={commonProxy} disabled={busy} onChange={event => setCommonProxy(event.target.value)}><option value="__unset__">先选择</option><option value="">不绑定代理（明确直连）</option>{workspace.nativeProxyRecords?.map(record => <option key={record.id} value={record.id}>{record.name}</option>)}</select></label><button className="button" disabled={commonProxy === "__unset__" || busy} onClick={() => setMapping(Object.fromEntries(targetIds.map(environmentId => [environmentId, commonProxy])))}>应用这个明确选择（共享节点）</button></div>
        <div className="env34-table env34-batch-table"><table><thead><tr><th>序号</th><th>明确目标环境 / ID</th><th>代理节点</th></tr></thead><tbody>{targetIds.map((environmentId, index) => <tr key={environmentId}><td>{index + 1}</td><td>{workspace.state.environments.find(environment => environment.id === environmentId)?.name ?? "未在当前页"}<br />{environmentId}</td><td><select aria-label={`为 ${environmentId} 分配代理`} value={mapping[environmentId] ?? "__unset__"} disabled={busy} onChange={event => setMapping(previous => ({ ...previous, [environmentId]: event.target.value }))}><option value="__unset__" disabled>必须明确选择</option><option value="">不绑定代理（明确直连）</option>{workspace.nativeProxyRecords?.map(record => <option key={record.id} value={record.id}>{record.name} · 修订{record.revision}</option>)}</select></td></tr>)}</tbody></table></div>
      </>}
      {(busy || readingPage) && <p role="status"><LoaderCircle size={16} className="spin" />正在读取或受理，尚未确认全部完成。</p>}
      {message && <p className="env34-error" role="alert">{message}</p>}
      {!page && target.current && <button className="button" disabled={busy || readingPage} onClick={() => { const wanted = target.current; if (wanted) void loadPage(wanted.planId, wanted.offset, wanted.operationId); }}>明确重新读取这个批次（不重新执行）</button>}
      {page && <>
        <p>计划ID：{page.planId} · {label[page.kind]} · 共 {page.total} 项。</p>
        {page.history && <p>这是旧尝试 {page.operationId} 的冻结结果，不是后来继续的逐项状态。<button className="button" disabled={busy || readingPage} onClick={() => { void loadPage(page.planId); }}>查看当前尝试</button></p>}
        <p>已完成 {report?.completedCount ?? page.completedCount} · 失败 {report?.failedCount ?? page.failedCount} · 未执行 {report?.notExecutedCount ?? page.notExecutedCount}{operationReady && operation?.persistencePending && " · 观测待保存，不能重复提交"}</p>
        {detailsIssue && <p role="status">下方是上一次读取的逐项观测，不是已确认的最终明细；读取失败不会改变真实任务结果。</p>}
        {page.sharedProxyAssignments > 0 && <p>其中 {page.sharedProxyAssignments} 项使用共享节点（同批次或已有环境），不保证独立公网IP。</p>}
        <div className="env34-table env34-batch-table" tabIndex={0}><table><thead><tr><th>序号</th><th>环境 / ID</th><th>身份 / 修订</th><th>明确代理</th><th>实际结果</th></tr></thead><tbody>{page.items.map(item => <tr key={item.index}><td>#{item.index + 1}</td><td>{item.name}<br /><small>{item.environmentId ?? "执行时分配新ID"}</small></td><td>{item.newIdentity ? "新seed · 空数据" : `保持seed/档案 · 原修订${item.expectedRevision}`}{item.sourceId && <><br /><small>源 {item.sourceId} · 修订{item.sourceRevision}</small></>}</td><td>{item.proxyName}<br /><small>{item.proxyId || "不绑定代理"}</small></td><td>{item.state === "completed" ? "已事务提交" : item.state === "failed" ? "失败，其他项保留" : "未执行"}{item.error && <><br />{item.error.code}：{item.error.message}</>}</td></tr>)}</tbody></table></div>
        <div className="env34-batch-pagination"><button className="button" disabled={busy || readingPage || page.offset <= 0} onClick={() => { void loadPage(page.planId, Math.max(0, page.offset - 25), acceptedId); }}><ChevronLeft size={15} />上一页</button><span>第 {Math.floor(page.offset / 25) + 1} / {Math.max(1, Math.ceil(page.total / 25))} 页</span><button className="button" disabled={busy || readingPage || page.offset + 25 >= page.total} onClick={() => { void loadPage(page.planId, page.offset + 25, acceptedId); }}>下一页<ChevronRight size={15} /></button><button className="button" disabled={busy || readingPage} onClick={() => { void loadPage(page.planId, page.offset, acceptedId); }}>重新读取本页（不重新执行）</button></div>
        {!acceptedId && <>
          {needsDirect && <label className="native-proxy-inline"><input type="checkbox" checked={confirmDirect} onChange={event => setConfirmDirect(event.target.checked)} />确认整个计划中 {page.directAssignments} 项不绑定代理（不仅当前页）；以后启动需要再次明确直连。</label>}
          {page.sharedProxyAssignments > 0 && <label className="native-proxy-inline"><input type="checkbox" checked={confirmShared} onChange={event => setConfirmShared(event.target.checked)} />确认以上明确的节点共享，不使用自动轮询映射。</label>}
        </>}
        {operationReady && operation?.error && <p className="env34-error">{operation.error.code}：{operation.error.message}</p>}
        {operationReady && !page.history && operation && operationIsTerminal(operation) && operation.state !== "completed" && <p>继续同一冻结计划，只重试失败和未执行项；旧修订冲突不会覆盖新配置。</p>}
      </>}
      <details className="env34-batch-details"><summary>身份与任务边界</summary><p>数量不设产品上限；磁盘、内存或目录故障会暂停并保留已提交结果。每页25条只是显示/调度节奏，不是创建配额。创建首项保留草稿明确seed，后续与克隆每项新ID、新seed和独立空目录；不复制Cookie或网站存储。本流程不启动浏览器，不绕过代理门禁。</p></details>
    </div>
  </EnvironmentWindowFrame>}</div>;
}
