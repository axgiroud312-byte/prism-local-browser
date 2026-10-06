import { useState, useSyncExternalStore } from "react";
import type { ApplicationService, DiagnosticOperation, DiagnosticState } from "../application/contract";
import { ReferenceButton } from "./ReferenceUi";
import { LocalPageWindow, RestoreConfirmWindow, localTime } from "./LocalPageUi";

const empty: DiagnosticState = { pending: false, busy: false };
const unavailable = () => empty;
const safeCode = (value?: string) => value && /^[A-Z][A-Z0-9_]{0,80}$/.test(value) ? value : "UNKNOWN";
const safeVersion = (value: string) => /^(?:\d+\.){2,3}\d+(?:-[a-z0-9.-]+)?$/i.test(value) ? value : "未提供可公开版本";
const safeHash = (value: string) => /^[a-f0-9]{64}$/.test(value) ? value : "摘要不可用";
const safeCount = (value: number) => Number.isSafeInteger(value) && value >= 0 ? value : 0;
const operationState: Record<string, string> = { accepted: "已受理", running: "进行中", completed: "已完成", failed: "失败", cancelled: "已取消" };
const operationKind: Record<string, string> = { "backup-export": "备份导出", "backup-restore": "完整恢复", "runtime-start": "启动", "runtime-stop": "停止", "batch-create": "批量创建", "batch-assign": "批量分配", "batch-clone": "批量复制", "cookie-import": "Cookie 导入", migration: "迁移", "recycle-purge": "回收清理" };
function DiagnosticTasks({ tasks }: { tasks: DiagnosticOperation[] }) {
  return <ul>{tasks.slice(0, 100).map((task, index) => <li key={index}>任务 {index + 1} · {operationKind[task.kind] ?? "其他操作"} · {operationState[task.state] ?? "未知状态"} · 已记录 {safeCount(task.completed)} / {safeCount(task.total)}{task.errorCode && ` · ${safeCode(task.errorCode)}`}{task.persistencePending && " · 待保存"}{task.cancelRequested && " · 取消已请求"}</li>)}</ul>;
}

export function NativeDiagnostics({ application }: { application: ApplicationService }) {
  const state = useSyncExternalStore(application.subscribe, application.getDiagnosticState ?? unavailable);
  const { preview, busy, pending, error, receipt } = state;
  const [window, setWindow] = useState<"preview" | "result">(), [endOpen, setEndOpen] = useState(false);
  const report = preview?.report, workspace = report?.workspace;
  // Keep the adapter's DiagnosticClient: no per-page client or raw workspace
  // export. Display an explicit field projection rather than arbitrary JSON.
  const showPreview = () => { setWindow("preview"); void application.previewDiagnostics?.(); };
  const save = () => { setWindow("result"); void application.exportDiagnostics?.(); };
  return <div className="local-page-diagnostic-entry" aria-label="脱敏诊断">
    <ReferenceButton disabled={busy || pending || !application.previewDiagnostics} onClick={showPreview}>生成诊断预览</ReferenceButton>
    {preview && <button className="local-page-link" onClick={() => setWindow(pending || receipt || error ? "result" : "preview")}>{pending ? "继续核实原诊断导出" : "查看诊断预览 / 结果"}</button>}
    {window === "preview" && <LocalPageWindow title="脱敏诊断预览" width={620} height={580} onClose={() => setWindow(undefined)} footer={<><ReferenceButton onClick={() => setWindow(undefined)}>返回</ReferenceButton><ReferenceButton className="primary" disabled={busy || !preview || !application.exportDiagnostics} onClick={save}>{pending ? "核实原导出" : "保存这份诊断 JSON"}</ReferenceButton></>}>
      <p>仅保存版本、已保存状态、错误码、数量与构建摘要。排除名称、路径、ID、seed、代理地址、凭据、Cookie、浏览内容与原始日志；不是备份或实时探测。</p>
      {busy && <p role="status">正在生成本机诊断…</p>}{error && <p role="alert">{safeCode(error.code)}：诊断未确认；保留原状态，稍后重试。</p>}
      {preview && report && workspace && <><dl className="local-page-diagnostic-list"><dt>工作区</dt><dd>{({ available: "可读取", partial: "部分可读取", unavailable: "不可读取（仅最小报告）" })[workspace.status]}</dd><dt>应用版本</dt><dd>{safeVersion(report.application.version)}</dd><dt>报告时间</dt><dd>{localTime(report.generatedAt)} · {safeCount(preview.bytes).toLocaleString()} 字节</dd><dt>签名状态</dt><dd>本次未检查；以对应安装包的 release.json 为准。</dd><dt>SHA-256</dt><dd><code>{safeHash(preview.sha256)}</code></dd></dl>
        <h3>公开字段摘要（服务白名单，不展示原始 JSON）</h3><div className="local-page-diagnostic-fields" tabIndex={0}>
          <p>已保存环境 {safeCount(workspace.counts.environments)} · 代理 {safeCount(workspace.counts.proxies)} · 内核 {safeCount(workspace.counts.kernels)}</p>
          <p>代理保护 provider：{report.proxyProtection === "available" ? "已提供（不是实时核验）" : "未提供"}；工作区 schema {workspace.schemaVersion === undefined ? "不可读取" : safeCount(workspace.schemaVersion)}</p>
          {workspace.startupCode && <p>启动错误码：{safeCode(workspace.startupCode)}</p>}
          <h3>已保存任务 / 维护</h3><DiagnosticTasks tasks={workspace.operations} /><DiagnosticTasks tasks={workspace.maintenance} />
          <h3>会话摘要</h3><ul>{workspace.sessions.slice(0, 100).map((session, index) => <li key={index}>会话 {index + 1} · {["running", "starting", "stopping", "error", "ready"].includes(session.state) ? session.state : "unknown"} · {session.networkPolicy === "proxy" ? "代理" : session.networkPolicy === "direct" ? "直连" : "策略未知"}{session.errorCode && ` · ${safeCode(session.errorCode)}`}{session.networkErrorCode && ` · ${safeCode(session.networkErrorCode)}`}{session.needsReconcile && " · 待核对"}{session.persistencePending && " · 待保存"}</li>)}</ul>
          <h3>构建摘要</h3><ul>{workspace.kernels.slice(0, 20).map((kernel, index) => <li key={index}>内核 {index + 1} · {safeVersion(kernel.version)}<br />归档 <code>{safeHash(kernel.archiveSha256)}</code><br />主程序 <code>{safeHash(kernel.executableSha256)}</code></li>)}</ul>
          <p>遗漏记录 {safeCount(workspace.omittedRecords)} · 不可读取节 {workspace.unavailableSections.length}；最多最近 100 任务 / 100 会话 / 20 内核。</p>
        </div><p className="local-page-note">预览有效期 15 分钟。本机新文件，不覆盖、不上传；报告仅反映保存的观察，不替代真实核验。</p>
      </>}
    </LocalPageWindow>}
    {window === "result" && <LocalPageWindow title="诊断导出结果" width={400} className="local-page-result" onClose={() => setWindow(undefined)} footer={<><ReferenceButton onClick={() => setWindow(undefined)}>返回</ReferenceButton>{pending ? <ReferenceButton className="primary" disabled={busy || !application.exportDiagnostics} onClick={save}>核实原导出</ReferenceButton> : <ReferenceButton disabled={!preview} onClick={() => setWindow("preview")}>查看预览</ReferenceButton>}</>}>
      {busy && <p role="status">正在核对原诊断保存…</p>}
      {pending && <p role="status">原保存请求待核实。返回此页仍保留同一报告与 requestId；核实不打开另一个选择器、不重新写文件。</p>}
      {error && <p role="alert">{safeCode(error.code)}：未收到匹配的保存回执；不能认定未写文件。</p>}
      {receipt && <p role="status">{receipt.status === "saved" ? "诊断文件已保存，SHA-256 与预览一致。" : receipt.status === "cancelled" ? "已取消保存；未发布诊断文件。" : "已结束此次核实；旧文件可能已发布，请自行查看原保存位置。可以生成新的诊断。"}</p>}
      {receipt?.status === "saved" && receipt.sha256 && <p>SHA-256：<code>{safeHash(receipt.sha256)}</code></p>}
      {pending && <ReferenceButton disabled={busy || !application.endDiagnosticVerification} onClick={() => setEndOpen(true)}>结束此次核实（保留未确认事实）</ReferenceButton>}
    </LocalPageWindow>}
    {endOpen && <RestoreConfirmWindow title="结束诊断导出核实" confirmText="结束此次核实" busy={busy} onClose={() => setEndOpen(false)} onConfirm={() => { void application.endDiagnosticVerification?.().then(() => { setEndOpen(false); setWindow("result"); }); }}><p>这不会删除或撤回原文件。旧文件可能已经发布；结束后可生成新报告，但需自行核对原保存位置。</p></RestoreConfirmWindow>}
  </div>;
}
