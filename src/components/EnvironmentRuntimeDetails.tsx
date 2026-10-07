import { useEffect, useState } from "react";
import type { Environment } from "../domain";
import type { RuntimeSession } from "../application/contract";
import { ReferencePopover } from "./ReferenceUi";
import { NativeRuntimeNetwork } from "./NativeRuntimeNetwork";

export function EnvironmentRuntimeDetails({ environment, session, blocked }: { environment: Environment; session?: RuntimeSession; blocked: boolean }) {
  const [anchor, setAnchor] = useState<HTMLElement | null>(null);
  useEffect(() => { if (blocked) setAnchor(null); }, [blocked]);
  if (!session && !environment.error) return null;
  return <>
    <button className="text-button runtime-details-trigger" aria-label={`${environment.name} 网络与处理详情`} aria-expanded={!!anchor} onClick={event => setAnchor(anchor ? null : event.currentTarget)}>详情</button>
    {anchor && <ReferencePopover anchor={anchor} label={`${environment.name} 网络与处理详情`} role="dialog" width={360} onClose={() => setAnchor(null)} className="environment-runtime-popup">
      <h3>{environment.name}</h3>
      {environment.error && <p role="status">{environment.error}</p>}
      {session?.nextAction && <p>{session.error?.code === "NETWORK_PROTECTION_UNAVAILABLE" ? "原报告记录启动条件未通过；请使用当前程序重试原环境，保持原代理、指纹与浏览数据。" : session.nextAction}</p>}
      {session?.lastExitCode !== undefined && <p>上次退出码：{session.lastExitCode}</p>}
      {session?.reconciledAt && <p>已核对：{new Date(session.reconciledAt).toLocaleString()}</p>}
      {session && <><p>{session.persistencePending ? "结果待保存，尚未确认" : session.needsReconcile ? "原会话待核对" : "当前保存的会话状态"}{session.resourcesPending && " · 资源待释放"}</p><p>网络策略：{session.networkPolicy === "proxy" ? "必须代理，不自动直连" : "明确直连"}</p><NativeRuntimeNetwork session={session} /></>}
      {environment.lastOpened && <p>最近打开：{new Date(environment.lastOpened).toLocaleString()}</p>}
    </ReferencePopover>}
  </>;
}
