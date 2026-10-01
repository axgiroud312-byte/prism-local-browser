import { useEffect, useRef, useState } from "react";
import { Box, LoaderCircle, RefreshCw, ShieldCheck, Trash2 } from "lucide-react";
import { mergeOperation, operationIsTerminal, type ApplicationResult, type ApplicationService, type Operation, type WorkspaceView } from "../application/contract";
import "./native-kernel.css";

const stageLabels: Record<string, string> = {
  queued: "已受理，等待执行", "acquiring-archive": "取得精确归档并校验 SHA-256", extracting: "在安全暂存目录解包",
  probing: "隔离探测实际身份和参数", "verifying-files": "核验既有文件摘要", publishing: "发布不可替换的新内核 ID",
  "verifying-extracted-files": "锁定解包文件并复核全部摘要",
  completed: "已完成", failed: "失败，查看具体原因", cancelled: "已取消", interrupted: "上次任务被中断",
};
const fieldLabels: Record<string, string> = { identity: "品牌和真实版本", seed: "固定种子", cpu: "CPU", acceptLanguages: "网页/请求语言", timezone: "时区", uiLanguage: "浏览器菜单语言", memory: "内存", gpu: "GPU/WebGL", "font/canvas/audio/clientrects": "字体 / 绘图 / 音频", "screen/location/webgpu/tls/mac": "屏幕 / 定位 / 其他", "proxy/webrtc": "代理 / WebRTC" };
const statusLabels = { configurable: "可配置 · 已实测", "seed-generated": "由固定内核按 seed 生成", system: "跟随真实环境", unverified: "未验证 · 不开放编辑" };

export function NativeKernelManager({ application, workspace }: { application: ApplicationService; workspace: WorkspaceView }) {
  const [source, setSource] = useState<"official" | "local">("official");
  const [version, setVersion] = useState("148.0.7778.215");
  const [checksum, setChecksum] = useState("9ef3f471b7a6641b4224532522b29141ce3746e27d55788d88e2fd951f362579");
  const [archive, setArchive] = useState<{ token: string; name: string }>();
  const [trusted, setTrusted] = useState(false);
  const [busy, setBusy] = useState(false);
  const [operation, setOperation] = useState<Operation>();
  const [message, setMessage] = useState("");
  const [confirmDelete, setConfirmDelete] = useState("");
  const ongoing = workspace.kernelOperations?.find(item => !operationIsTerminal(item));
  const currentId = useRef(ongoing?.id);
  const alive = useRef(true);
  const generation = useRef(0);
  const activeId = operation && !operationIsTerminal(operation) ? operation.id : ongoing?.id;
  const waiting = busy || !!activeId;
  const records = workspace.kernelRecords ?? [];
  useEffect(() => { alive.current = true; return () => { alive.current = false; generation.current++; }; }, []);
  function applyOperation(next: Operation, expectedId: string) {
    if (!alive.current || next.id !== expectedId || currentId.current !== expectedId) return;
    setOperation(previous => mergeOperation(previous, next));
  }

  useEffect(() => {
    if (!activeId) return;
    currentId.current = activeId;
    let disposed = false;
    let timer: ReturnType<typeof setTimeout>;
    async function read() {
      const response = await application.getOperation(activeId!);
      if (disposed || currentId.current !== activeId) return;
      if (!response.ok) {
        setMessage(`${response.error.code}：${response.error.message} 状态未确认，继续读取。`);
        timer = setTimeout(read, 1000);
        return;
      }
      if (operationIsTerminal(response.data)) {
        await application.refresh?.();
        if (disposed || currentId.current !== activeId) return;
        applyOperation(response.data, activeId!);
        setMessage(response.data.state === "completed" ? "内核任务已实际完成；环境正常启停仍待接入。" : `${response.data.error?.code ?? response.data.state}：${response.data.error?.message ?? "任务未完成"}${response.data.error?.details?.reason ? `（${response.data.error.details.reason}）` : ""}`);
      } else { applyOperation(response.data, activeId!); timer = setTimeout(read, 500); }
    }
    void read();
    return () => { disposed = true; clearTimeout(timer); };
  }, [application, activeId]);

  async function begin(request: () => Promise<ApplicationResult<{ status: "accepted"; operation: Operation }>> | undefined) {
    const requestGeneration = ++generation.current;
    currentId.current = undefined;
    setBusy(true); setMessage("");
    try {
      const result = await request();
      if (!alive.current || generation.current !== requestGeneration) return;
      if (!result) { setMessage("此桌面服务尚不支持内核操作，没有执行模拟操作。"); return; }
      if (!result.ok) { setMessage(`${result.error.code}：${result.error.message}`); return; }
      currentId.current = result.data.operation.id;
      setOperation(result.data.operation);
      await application.refresh?.();
    } finally { if (alive.current && generation.current === requestGeneration) setBusy(false); }
  }

  return <>
    <div className="info-strip"><ShieldCheck size={19} /><div><strong>真实精确构建，不跟随默认版本</strong><p>安装会执行所选内核的隔离诊断。SHA-256、程序版本和参数读回通过后，才登记新的 kernelId；不会替换旧构建或改写旧环境。诊断不代表正常环境已可启动。</p></div></div>
    <section className="work-card native-kernel-install" aria-label="安装精确内核">
      <h2>安装并核验 fingerprint-chromium</h2>
      <form onSubmit={event => { event.preventDefault(); void begin(() => application.installKernel?.({ source, version, expectedChecksum: checksum.trim().toLowerCase(), archiveToken: source === "local" ? archive?.token : undefined, trusted, requestId: crypto.randomUUID() })); }}>
        <div className="native-kernel-fields">
          <label>归档来源<select aria-label="归档来源" value={source} disabled={waiting} onChange={event => { setSource(event.target.value as "official" | "local"); setTrusted(false); }}><option value="official">官方发行 ZIP</option><option value="local">本地可信 ZIP</option></select></label>
          <label>精确发行版本<input aria-label="精确发行版本" required pattern="[0-9]+\.[0-9]+\.[0-9]+\.[0-9]+" value={version} disabled={waiting} onChange={event => { setVersion(event.target.value); setChecksum(""); }} /></label>
        </div>
        <label className="native-kernel-digest">预期归档 SHA-256<input aria-label="预期归档 SHA-256" className="mono" required pattern="[a-fA-F0-9]{64}" value={checksum} disabled={waiting} onChange={event => setChecksum(event.target.value)} /></label>
        {source === "official" ? <p className="field-hint">填写所选版本官方发行资产的摘要。148 是本项目实测候选，不是生产推荐；上游 README 链接不保证资产可获取，失败不会自动换版本。</p> : <div className="native-kernel-local">
          <button className="button" type="button" disabled={waiting} onClick={async () => { const result = await application.selectKernelArchive?.(); if (!result) return; if (!result.ok) setMessage(result.error.message); else if (result.data.status === "selected") { setArchive({ token: result.data.archiveToken!, name: result.data.name! }); setTrusted(false); } }}>选择可信 ZIP</button>
          <span>{archive?.name ?? "尚未选择文件"}</span>
          <label className="native-kernel-trust"><input type="checkbox" checked={trusted} disabled={waiting} onChange={event => setTrusted(event.target.checked)} />我已核对来源和摘要，确认此 ZIP 可信，允许执行其中的内核进行诊断。</label>
        </div>}
        <div className="native-kernel-actions"><button className="button primary" disabled={waiting || (source === "local" && (!archive || !trusted))}>{waiting ? <LoaderCircle size={16} className="spin" /> : <Box size={16} />}安装并核验</button><button type="button" className="button" onClick={() => void application.refresh?.()}><RefreshCw size={16} />重新读取本机记录</button></div>
      </form>
      {(operation || ongoing) && <div className="native-kernel-operation" aria-live="polite">任务：{stageLabels[(operation ?? ongoing)?.stage ?? "queued"] ?? (operation ?? ongoing)?.stage}<span className="mono">{(operation ?? ongoing)?.id}</span>{activeId && <button className="button" disabled={!!operation?.cancelRequested} onClick={async () => { const target = activeId; const response = await application.cancelOperation(target); if (!alive.current || currentId.current !== target) return; if (response.ok) applyOperation(response.data, target); else setMessage(response.error.message); }}>取消此任务</button>}</div>}
      {message && <p role="status" className="native-kernel-message">{message}</p>}
    </section>
    {!records.length && <div className="empty-state"><Box size={32} /><h3>还没有已登记的真实内核</h3><p>选择精确构建并完成核验；旧的“未安装”档案不会被自动重绑定。</p></div>}
    <div className="kernel-grid">{records.map(record => <section className="kernel-card" key={record.id}>
      <div className="kernel-card-top"><div className="kernel-symbol"><Box size={28} /></div><span className={`tag ${record.status === "verified" ? "blue-tag" : "warning"}`}>{record.status === "verified" ? "真实诊断已核验" : "核验失败 · 不可用"}</span></div>
      <h2>Chromium {record.version.split(".")[0]}</h2><div className="kernel-version mono">{record.version}</div><p>{record.architecture} · {record.source.kind === "official" ? "官方发行" : "用户确认可信的本地归档"}</p>
      <dl className="native-kernel-evidence"><dt>精确 ID</dt><dd className="mono">{record.id}</dd><dt>发行 tag</dt><dd>{record.source.tag}</dd><dt>来源</dt><dd>{record.source.location}</dd><dt>源码 commit</dt><dd className="mono">{record.source.commit ?? "未知（不从 main 推断）"}</dd><dt>归档 SHA-256</dt><dd className="mono">{record.archiveSha256}</dd><dt>主程序 SHA-256</dt><dd className="mono">{record.executableSha256}</dd><dt>内部安装位置</dt><dd className="mono">{record.installPath}</dd><dt>关联环境</dt><dd>{record.usedCount ?? record.usedBy.length} 个（下方最多展示100个ID）{record.usedBy.length > 0 && <ul>{record.usedBy.map(id => <li key={id}>{workspace.state.environments.find(environment => environment.id === id)?.name ?? id}</li>)}</ul>}</dd></dl>
      <details className="native-kernel-report"><summary>查看该版本能力与实测读值</summary><p>{record.report.adapterVersion} · {record.report.version}<br />安装时采样 {record.report.sampledAt} · 私有 pipe · 沙箱保留</p>{record.report.capabilities.map(capability => <div className="native-kernel-capability" key={capability.field}><strong>{fieldLabels[capability.field] ?? capability.field}</strong><span>{statusLabels[capability.status]}</span><small>{capability.source === "observed" ? "真实观测" : capability.source === "source-derived" ? "源码推导，具体读值另列" : "未探测"}</small><p>{capability.note}</p></div>)}<div className="table-wrap"><table><thead><tr><th>诊断 seed</th><th>CPU</th><th>语言</th><th>时区</th><th>实际版本 / 正常退出</th></tr></thead><tbody>{record.report.observations.map((sample, index) => <tr key={index}><td>{sample.seed}</td><td>{sample.cpu}</td><td>{sample.language}</td><td>{sample.timezone}</td><td>{sample.browserVersion} / {sample.normalExit ? "是" : "否"}</td></tr>)}</tbody></table></div><p>浏览器菜单、字体/Canvas/音频等未验证字段不开放编辑；代理和正常环境运行仍属后续验收。</p></details>
      <div className="native-kernel-actions"><button className="button" disabled={waiting} onClick={() => void begin(() => application.verifyKernel?.(record.id, crypto.randomUUID()))}><RefreshCw size={16} />重新核验</button><button className="button" disabled={waiting || !!(record.usedCount ?? record.usedBy.length)} onClick={() => setConfirmDelete(record.id)}><Trash2 size={16} />移除此构建</button></div>
      {confirmDelete === record.id && <div className="native-kernel-confirm"><p>仅移除此精确构建，不能删除被引用内核，也不删除浏览数据。确认移除？</p><button className="button" onClick={() => setConfirmDelete("")}>保留构建</button><button className="button danger" disabled={waiting} onClick={() => { setConfirmDelete(""); void begin(() => application.deleteKernel?.(record.id, crypto.randomUUID())); }}>确认移除</button></div>}
    </section>)}</div>
    {!!workspace.kernelOperations?.length && <details className="work-card native-kernel-history"><summary>最近内核任务（持久记录）</summary>{workspace.kernelOperations.map(item => <div key={item.id}><p><span className="mono">{item.id}</span> · {stageLabels[item.stage ?? item.state] ?? item.state}{item.error && ` · ${item.error.code}：${item.error.message}`}</p>{item.report && <details><summary>本次复验采样 {item.report.sampledAt}</summary><p>精确内核 ID：{item.kernelId} · {item.report.adapterVersion} · {item.report.version}</p>{item.report.observations.map((sample, index) => <p key={index}>seed {sample.seed} · CPU {sample.cpu} · {sample.language} · {sample.timezone} · 实际版本 {sample.browserVersion} · 正常退出 {sample.normalExit ? "是" : "否"}</p>)}</details>}</div>)}</details>}
  </>;
}
