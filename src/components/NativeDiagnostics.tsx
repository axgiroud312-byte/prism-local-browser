import { useSyncExternalStore } from "react";
import type { ApplicationService, DiagnosticState } from "../application/contract";
import "./native-proxy.css";

const empty: DiagnosticState = { pending: false, busy: false };
const unavailable = () => empty;
export function NativeDiagnostics({ application }: { application: ApplicationService }) {
  const state = useSyncExternalStore(application.subscribe, application.getDiagnosticState ?? unavailable);
  const { preview, busy, pending, error, receipt } = state;
  return <section className="native-proxy-panel native-diagnostics" aria-label="脱敏诊断">
    <h2>脱敏诊断</h2>
    <p>收集版本、已保存状态、错误码和数量。排除代理地址、密码、Cookie、浏览内容、名称、路径、seed 和原始日志。</p>
    <p>只读快照最多列出最近 100 条任务、100 条会话和 20 个内核；它不是实时网络检测。文件只保存到你选择的本机位置。</p>
    <div className="native-proxy-actions">
      <button className="button" disabled={busy || pending || !application.previewDiagnostics} onClick={() => { void application.previewDiagnostics?.(); }}>生成诊断预览</button>
      <button className="button primary" disabled={busy || !preview || !application.exportDiagnostics} onClick={() => { void application.exportDiagnostics?.(); }}>{pending ? "核实原导出" : "保存这份诊断 JSON"}</button>
      {pending && <button className="button" disabled={busy || !application.endDiagnosticVerification} onClick={() => { void application.endDiagnosticVerification?.(); }}>结束此次核实（保留未确认事实）</button>}
    </div>
    {busy && <p role="status">正在处理本机诊断…</p>}
    {pending && <p role="status">原保存请求待核实。返回此页可继续，重试沿用原请求。</p>}
    {error && <p role="alert">{error.code}：{error.message}</p>}
    {receipt && <p role="status">{receipt.status === "saved" ? "诊断文件已保存，SHA-256 与预览一致。" : receipt.status === "cancelled" ? "已取消保存。" : "已结束此次核实；旧文件可能已发布，请自行查看原保存位置。可以生成新的诊断。"}</p>}
    {preview && <>
      <p>工作区：{({ available: "可读取", partial: "部分可读取", unavailable: "不可读取（仅最小报告）" })[preview.report.workspace.status]} · {preview.bytes.toLocaleString()} 字节 · {preview.report.generatedAt}</p>
      <p>签名状态：本次未检查；以安装包的 release.json 为准。</p>
      <p>SHA-256：<code>{preview.sha256}</code></p>
      <details><summary>查看将保存的字段（预览有效期 15 分钟）</summary><pre tabIndex={0}>{JSON.stringify(preview.report, null, 2)}</pre></details>
    </>}
  </section>;
}
