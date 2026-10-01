import { useEffect, useRef, useState, type FormEvent } from "react";
import { LoaderCircle, RefreshCw, ShieldCheck, Trash2 } from "lucide-react";
import { mergeOperation, operationIsTerminal, type ApplicationResult, type ApplicationService, type NativeProxy, type Operation, type ProxyConfiguration, type ProxyImportPreview, type WorkspaceView } from "../application/contract";
import "./native-proxy.css";

const stageLabels: Record<string, string> = { queued: "等待网络调度", validation: "格式校验", credentials: "读取受保护认证", connection: "连接代理", "proxy-tls": "代理TLS", authentication: "隧道与认证", tunnel: "建立隧道", "target-tls": "目标TLS", target: "目标访问", exit: "读取出口", result: "检查结果", "storage-pending": "观测结果待保存", completed: "已完成", failed: "检查失败", cancelled: "已取消", "application-interrupted": "上次检查被中断" };
const statusLabels = { unchecked: "尚未检查", connected: "本次检查通过", failed: "检查未通过" };
const failureMessage = (result: ApplicationResult<unknown> | undefined) => !result ? "当前桌面服务不支持此操作，没有模拟保存。" : result.ok ? "" : `${result.error.code}：${result.error.message}`;

export function NativeProxyManager({ application, workspace, importOpen, onImportOpenChange }: { application: ApplicationService; workspace: WorkspaceView; importOpen: boolean; onImportOpenChange: (open: boolean) => void }) {
  const records = workspace.nativeProxyRecords ?? [];
  const [text, setText] = useState("");
  const [showText, setShowText] = useState(false);
  const [preview, setPreview] = useState<ProxyImportPreview>();
  const [selected, setSelected] = useState<number[]>([]);
  const [editing, setEditing] = useState<NativeProxy>();
  const [configuration, setConfiguration] = useState<ProxyConfiguration>();
  const [credentialAction, setCredentialAction] = useState<"keep" | "replace" | "clear">("keep");
  const [username, setUsername] = useState("");
  const [password, setPassword] = useState("");
  const [deleting, setDeleting] = useState<NativeProxy>();
  const [busy, setBusy] = useState(false);
  const [message, setMessage] = useState("");
  const [localOperations, setLocalOperations] = useState<Record<string, Operation>>({});
  const alive = useRef(true), busyRef = useRef(false), generation = useRef(0), previewId = useRef<string | undefined>(undefined);
  const importOpenRef = useRef(importOpen); importOpenRef.current = importOpen;
  const operations = new Map<string, Operation>();
  for (const operation of workspace.proxyOperations ?? []) operations.set(operation.id, operation);
  for (const operation of Object.values(localOperations)) operations.set(operation.id, mergeOperation(operations.get(operation.id), operation));
  const active = [...operations.values()].filter(operation => !operationIsTerminal(operation));
  const activeKey = active.map(operation => operation.id).sort().join(",");
  const activeByProxy = new Map(active.map(operation => [operation.proxyId, operation]));
  const waiting = busy || !!editing || !!deleting;

  function discardPreview() {
    generation.current++;
    const old = previewId.current; previewId.current = undefined;
    if (old) void application.discardProxyImport?.(old);
    setPreview(undefined); setSelected([]);
  }
  function clearCredentials() { setUsername(""); setPassword(""); setCredentialAction("keep"); }
  function closeImport() { discardPreview(); setText(""); setShowText(false); onImportOpenChange(false); }
  useEffect(() => {
    alive.current = true;
    return () => { alive.current = false; generation.current++; if (previewId.current) void application.discardProxyImport?.(previewId.current); previewId.current = undefined; };
  }, [application]);
  useEffect(() => { if (!importOpen) { discardPreview(); setText(""); setShowText(false); } }, [importOpen]);

  useEffect(() => {
    if (!activeKey) return;
    let disposed = false, timer: ReturnType<typeof setTimeout>;
    const ids = activeKey.split(",");
    async function read() {
      let stillActive = false, completed = false;
      for (const id of ids) {
        const response = await application.getOperation(id);
        if (disposed) return;
        if (!response.ok) { stillActive = true; setMessage(`${response.error.code}：${response.error.message} 结果尚未确认，继续读取。`); continue; }
        setLocalOperations(previous => ({ ...previous, [id]: mergeOperation(previous[id], response.data) }));
        if (!operationIsTerminal(response.data)) stillActive = true;
        else { completed = true; setMessage(response.data.state === "completed" ? "本次代理检查已完成；历史结果不保证下次启动可用。" : `${response.data.error?.code ?? response.data.state}：${response.data.error?.message ?? "检查未完成"}`); }
      }
      if (completed) await application.refresh?.();
      if (!disposed && stillActive) timer = setTimeout(read, 750);
    }
    void read();
    return () => { disposed = true; clearTimeout(timer); };
  }, [application, activeKey]);

  async function perform<T>(action: () => Promise<ApplicationResult<T>> | undefined, done: (data: T) => void) {
    if (busyRef.current) return;
    busyRef.current = true; setBusy(true); setMessage("");
    try {
      const result = await action();
      if (!alive.current) return;
      if (!result?.ok) { setMessage(failureMessage(result)); return; }
      done(result.data);
    } finally { busyRef.current = false; if (alive.current) setBusy(false); }
  }
  async function parse() {
    if (busyRef.current) return;
    discardPreview();
    const current = generation.current;
    busyRef.current = true; setBusy(true); setMessage("");
    try {
      const result = await application.parseProxyImport?.(text);
      if (!alive.current || !importOpenRef.current || generation.current !== current) { if (result?.ok) void application.discardProxyImport?.(result.data.previewId); return; }
      if (!result?.ok) { setMessage(failureMessage(result)); return; }
      const data = result.data; previewId.current = data.previewId; setPreview(data);
      setSelected(data.rows.filter(row => row.configuration && !row.error && !row.duplicateCount && !row.existingCount).map(row => row.line));
      setMessage("已解析，尚未保存。重复候选默认不选，可明确选择另存节点。");
    } finally { busyRef.current = false; if (alive.current) setBusy(false); }
  }
  async function loadFile(file: File | undefined) {
    if (!file || busyRef.current) return;
    discardPreview();
    if (file.size > 2 * 1024 * 1024) { setMessage("文本文件不能超过2MiB；请分文件导入，这不是代理数量限制。"); return; }
    const current = generation.current;
    busyRef.current = true; setBusy(true);
    try {
      const value = new TextDecoder("utf-8", { fatal: true }).decode(await file.arrayBuffer());
      if (!alive.current || generation.current !== current || !importOpenRef.current) return;
      setText(value); setMessage("文件内容已读取，尚未保存；请选择解析预览。");
    } catch { if (alive.current) setMessage("文本文件需为有效UTF-8；没有保存或执行任何内容。"); }
    finally { busyRef.current = false; if (alive.current) setBusy(false); }
  }
  function edit(record: NativeProxy) {
    setDeleting(undefined); setEditing(record); clearCredentials();
    setConfiguration({ name: record.name, type: record.type, host: record.host, port: record.port, country: record.country });
  }
  function save(event: FormEvent) {
    event.preventDefault(); if (!editing || !configuration) return;
    const record = editing;
    void perform(() => application.updateProxy?.({ proxyId: record.id, expectedRevision: record.revision, requestId: crypto.randomUUID(), configuration, credentials: credentialAction === "replace" ? { action: "replace", username, password } : { action: credentialAction } }), () => {
      clearCredentials(); setEditing(undefined); setConfiguration(undefined); setMessage("代理已保存，旧检查结果已失效；环境seed保持不变。");
    });
  }

  return <div className="native-proxy-manager">
    <div className="info-strip"><ShieldCheck size={19} /><div><strong>本机凭据保护，独立环境通道</strong><p>HTTP / HTTPS支持认证检查，TLS校验不能跳过。环境启动会重新建立独立认证通道并做同通道前检，不凭历史成功启动。SOCKS5暂不支持检查或启动；浏览器代理通道代码已接，实际验收与运行期全路径断线保护仍待补。</p></div></div>
    {importOpen && <section className="work-card native-proxy-form" aria-labelledby="native-proxy-import-title">
      <h2 id="native-proxy-import-title">导入代理 · 先预览再保存</h2>
      <p>每行一条：<code>http://user:password@host:port</code>、<code>https://host:port</code>或<code>host:port:user:password</code>。IPv6使用<code>socks5://[2001:db8::1]:1080</code>；凭据中的分隔符使用URI编码。预览不回显认证；HTTP到代理不加密认证，HTTPS才提供TLS保护。</p>
      <label>原始导入文本（只在本次表单保留）<textarea className={!showText ? "proxy-secret-input" : ""} autoComplete="off" aria-label="原始代理导入文本" value={text} disabled={busy} rows={5} spellCheck={false} onChange={event => { discardPreview(); setText(event.target.value); }} /></label>
      <div className="native-proxy-actions"><label className="native-proxy-inline"><input type="checkbox" checked={showText} disabled={busy} onChange={event => setShowText(event.target.checked)} />显示原始输入（可能含凭据）</label><label className="native-proxy-file">选择UTF-8文本文件<input type="file" accept=".txt,text/plain" disabled={busy} onChange={event => { void loadFile(event.target.files?.[0]); event.target.value = ""; }} /></label></div>
      <div className="native-proxy-actions"><button className="btn" disabled={busy || !text.trim()} onClick={() => void parse()}>解析预览</button><button className="btn" disabled={busy} onClick={closeImport}>取消并清除输入</button></div>
      {preview && <>
        <p role="status">有效{preview.rows.filter(row => !!row.configuration).length}行 · 错误{preview.rows.filter(row => !!row.error).length}行 · 忽略空白/注释{preview.ignoredLines}行。勾选{selected.length}行；预览15分钟后失效。</p>
        <div className="table-scroll"><table><thead><tr><th>保存</th><th>原始行号</th><th>安全配置 / 错误</th><th>认证</th><th>重复候选</th></tr></thead><tbody>{preview.rows.map(row => <tr key={row.line}>
          <td><input type="checkbox" aria-label={`保存第${row.line}行`} disabled={busy || !row.configuration || !!row.error} checked={selected.includes(row.line)} onChange={event => setSelected(previous => event.target.checked ? [...previous, row.line] : previous.filter(line => line !== row.line))} /></td>
          <td>{row.line}</td><td>{row.error ?? (row.configuration && `${row.configuration.type.toUpperCase()} · ${row.configuration.host}:${row.configuration.port}`)}</td><td>{row.configuration ? row.hasAuthentication ? "已输入认证（不回显）" : "无认证" : "—"}</td><td>{row.duplicateCount || row.existingCount ? `本批另有${row.duplicateCount}行 / 已存${row.existingCount}个节点地址相同` : "—"}</td>
        </tr>)}</tbody></table></div>
        {Object.keys(preview.duplicateGroups).length > 0 && <details><summary>查看重复组的原始行号（地址相同不代表认证相同）</summary>{Object.entries(preview.duplicateGroups).map(([groupId, group]) => <p key={groupId}>行号：{group.lines.join("、")}；已存同地址节点：{group.existingProxyIds.length}个。</p>)}</details>}
        <button className="btn primary" disabled={busy || !selected.length} onClick={() => void perform(() => application.commitProxyImport?.({ previewId: preview.previewId, selectedRows: selected, requestId: crypto.randomUUID() }), data => {
          const saved = new Set(data.importedLines), remaining = text.split(/\n/).filter((_, index) => !saved.has(index+1)).join("\n");
          previewId.current = undefined; setPreview(undefined); setSelected([]); setText(remaining); setShowText(false);
          setMessage(`已保存${data.importedIds.length}个代理；${remaining.trim() ? "未选与错误行仍留在输入框，可修正后重新解析。" : "原始凭据输入已清除。"}`);
          if (!remaining.trim()) onImportOpenChange(false);
        })}>保存所选有效行</button>
      </>}
    </section>}
    {editing && configuration && <section className="work-card native-proxy-form" aria-labelledby="native-proxy-edit-title"><h2 id="native-proxy-edit-title">编辑代理 · {editing.name}</h2><p>修订{editing.revision}；保存会使旧检查结果失效。已有认证不会回显，默认保留。</p><form onSubmit={save}>
      <div className="native-proxy-fields"><label>名称<input value={configuration.name} disabled={busy} required maxLength={256} onChange={event => setConfiguration({ ...configuration, name: event.target.value })} /></label><label>协议<select value={configuration.type} disabled={busy} onChange={event => setConfiguration({ ...configuration, type: event.target.value as ProxyConfiguration["type"] })}><option value="http">HTTP</option><option value="https">HTTPS · TLS到代理</option><option value="socks5">SOCKS5 · 暂不支持检查</option></select></label><label>主机名或IP<input value={configuration.host} disabled={busy} required onChange={event => setConfiguration({ ...configuration, host: event.target.value })} /></label><label>端口<input type="number" min={1} max={65535} required value={configuration.port} disabled={busy} onChange={event => setConfiguration({ ...configuration, port: Number(event.target.value) })} /></label><label>地区标签（自行填写，非实测）<input value={configuration.country} disabled={busy} maxLength={128} onChange={event => setConfiguration({ ...configuration, country: event.target.value })} /></label><label>认证处理<select value={credentialAction} disabled={busy} onChange={event => { setUsername(""); setPassword(""); setCredentialAction(event.target.value as typeof credentialAction); }}><option value="keep">保留现有认证（{editing.hasAuthentication ? "已设置" : "未设置"}）</option><option value="replace">明确替换用户名和密码</option><option value="clear">明确清除认证</option></select></label></div>
      {credentialAction === "replace" && <div className="native-proxy-fields"><label>新用户名（不回显旧值）<input type="password" value={username} disabled={busy} autoComplete="off" maxLength={4096} onChange={event => setUsername(event.target.value)} /></label><label>新密码<input type="password" value={password} disabled={busy} autoComplete="new-password" maxLength={4096} onChange={event => setPassword(event.target.value)} /></label></div>}
      <div className="native-proxy-actions"><button className="btn primary" disabled={busy}>确认保存</button><button type="button" className="btn" disabled={busy} onClick={() => { clearCredentials(); setEditing(undefined); setConfiguration(undefined); }}>取消并清除新凭据</button></div>
    </form></section>}
    {deleting && <section className="work-card native-proxy-form" role="group" aria-label="删除代理确认"><h2>确认删除 {deleting.name}？</h2><p>只删除本机代理配置与其受保护认证，不改变环境设备身份；被引用的节点不能直接删除。</p><div className="native-proxy-actions"><button className="btn danger" disabled={busy} onClick={() => void perform(() => application.deleteProxy?.({ proxyId: deleting.id, expectedRevision: deleting.revision, requestId: crypto.randomUUID() }), () => { setDeleting(undefined); setMessage("代理配置已删除。"); })}>确认删除</button><button className="btn" disabled={busy} onClick={() => setDeleting(undefined)}>取消</button></div></section>}
    <section className="work-card"><div className="section-toolbar"><h2>本机代理 <span>{records.length}</span></h2><button className="btn" disabled={busy} onClick={() => void application.refresh?.()}><RefreshCw size={15} />重新读取</button></div>
      <div className="table-scroll"><table><thead><tr><th>名称 / 地址</th><th>认证 / 绑定</th><th>真实检查 / 实际出口</th><th>操作</th></tr></thead><tbody>{records.map(record => {
        const task = activeByProxy.get(record.id), report = task ? task.proxyReport : record.checkReport;
        return <tr key={record.id}><td><strong>{record.name}</strong><span className="cell-secondary">{record.type.toUpperCase()} · {record.host.includes(":") ? `[${record.host}]` : record.host}:{record.port}</span><span className="cell-secondary">{record.country || "未填地区标签"} · 修订{record.revision}</span></td><td>{record.hasAuthentication ? "已设置认证（不回显）" : "无认证"}<span className="cell-secondary">绑定{record.usedBy.length}个环境</span></td><td><span role={task ? "status" : undefined}>{task ? task.persistencePending ? "结果待保存 · 修复存储后重新读取" : stageLabels[task.stage ?? "queued"] ?? "正在检查" : statusLabels[record.status]}</span>{report?.exitIp && <span className="cell-secondary">本次观测IP：{report.exitIp}</span>}{report?.finishedAt && <span className="cell-secondary">{new Date(report.finishedAt).toLocaleString()} · {report.durationMs}ms</span>}{report?.error && <span className="cell-secondary">{report.error.code}：{report.error.message}</span>}{report?.steps.length ? <details className="native-proxy-report"><summary>分阶段结果</summary><ul>{report.steps.map((step, index) => <li key={index}>{stageLabels[step.stage] ?? step.stage} · {step.status === "passed" ? "通过" : step.status === "running" ? "进行中" : step.status === "unsupported" ? "不支持" : "失败"}：{step.message}</li>)}</ul><p>目标：{report.targetOrigin || "尚未访问目标"}；不跟随重定向，不跳过TLS验证。</p></details> : null}</td><td><div className="native-proxy-actions"><button className="btn" disabled={waiting || !!task} onClick={() => void perform(() => application.checkProxy?.({ proxyId: record.id, expectedRevision: record.revision, requestId: crypto.randomUUID() }), data => setLocalOperations(previous => ({ ...previous, [data.operation.id]: data.operation })))}>{task && <LoaderCircle size={14} className="spin" />}检查</button>{task && <button className="btn" disabled={busy || task.cancelRequested || task.persistencePending} onClick={() => void perform(() => application.cancelOperation(task.id), data => setLocalOperations(previous => ({ ...previous, [data.id]: mergeOperation(previous[data.id], data) })))}>取消检查</button>}<button className="btn" disabled={waiting || !!task} onClick={() => edit(record)}>编辑</button><button className="btn danger" disabled={waiting || !!task || !!record.usedBy.length} title={record.usedBy.length ? "先修改引用环境的绑定" : "删除本机节点"} onClick={() => setDeleting(record)}><Trash2 size={14} />删除</button></div></td></tr>;
      })}</tbody></table></div>
      {!records.length && <div className="native-proxy-empty"><p>没有本机代理。演示数据不会进入这里。</p><button className="btn primary" onClick={() => onImportOpenChange(true)}>导入代理</button></div>}
    </section>
    {(busy || message) && <p className="native-proxy-message" role="status" aria-live="polite">{busy && <LoaderCircle size={16} className="spin" />}{message || "本机服务正在处理，尚未确认保存。"}</p>}
  </div>;
}
