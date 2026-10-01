import type { RuntimeSession } from "../application/contract";
import { proxyResolutionLabel, proxyStageLabel } from "../application/proxy-network";

export function NativeRuntimeNetwork({ session }: { session: RuntimeSession }) {
  if (session.networkPolicy !== "proxy") return null;
  const report = session.proxyReport;
  // A persisted PID never proves this service still owns a live bridge.
  const current = !session.needsReconcile && ["starting", "running"].includes(session.state);
  return <div className="native-runtime-network">
    {session.networkFault && <div role="alert" className="cell-secondary">{session.networkFault.error.code === "NETWORK_PROTECTION_UNAVAILABLE" ? "系统级隔离未实现/验证，代理启动已阻止" : `网络故障：${session.networkFault.error.code} · ${session.networkFault.containment === "stopped" ? "本次故障资源已确认退出" : "停止本次会话，退出仍待确认"}`}。保持原代理策略，不自动直连或重建旧端口。</div>}
    <div className="cell-secondary">{current ? "本次会话启动前报告" : "历史启动前报告"} · 修订{session.proxyRevision}{session.needsReconcile ? " · 原会话待核对" : ""}</div>
    <div className="cell-secondary" role="status">{session.persistencePending ? "通道结果待保存，尚未确认" : report?.error ? `前检失败：${report.error.code}` : report?.finishedAt ? "同通道启动前检查通过" : "同通道前检尚未完成"}</div>
    {report?.exitIp && <div className="cell-secondary">启动前观测IP：{report.exitIp}</div>}
    {report?.resolutionPolicy && <div className="cell-secondary">{proxyResolutionLabel(report.resolutionPolicy)}</div>}
    {report && <details className="native-runtime-network-details"><summary>启动前网络阶段</summary>
      <p>{current ? "该启动前检查与本次会话使用同一桥接实例。" : session.needsReconcile ? "只记录原会话启动时的同通道前检；当前服务没有重建或接管该桥，原进程仍待核对。" : "只记录原会话启动时的同通道前检，不证明历史桥仍在运行。"}不是对每次网页请求的持续出口采样，也不代表运行期全路径断线保护已验收。</p>
      <ul>{report.steps.map((step, index) => <li key={index}>{proxyStageLabel(step.stage)} · {step.status === "passed" ? "通过" : step.status === "running" ? "进行中" : "未通过"}：{step.message}</li>)}</ul>
      <p>{report.finishedAt ? new Date(report.finishedAt).toLocaleString() : "正在检查"} · {report.durationMs ?? 0}ms</p>
      <p>安全通道标识：<code>{session.proxyChannelId}</code>；目标：{report.targetOrigin || "尚未访问目标"}</p>
    </details>}
  </div>;
}
