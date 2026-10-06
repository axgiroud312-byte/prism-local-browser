import { useState, type ReactNode } from "react";
import type { Activity } from "../domain";
import type { RuntimeSession } from "../application/contract";
import { ReferenceButton } from "./ReferenceUi";
import { LocalPageTable, LocalPageWindow, LocalPagination, localTime } from "./LocalPageUi";

export interface ActivityPageProps {
  activities: Activity[]; native: boolean; runtimeSessions?: Record<string, RuntimeSession>; blockedIds?: string[];
  diagnostics?: ReactNode; onExport?: () => void;
  onSessionAction?(environmentId: string, sessionId: string, action: "reconcile" | "force"): void;
}
const resultText = { success: "已完成", error: "失败", info: "记录 / 未确认完成" };

function SessionActions({ activity, runtimeSessions, blockedIds = [], onSessionAction }: Pick<ActivityPageProps, "runtimeSessions" | "blockedIds" | "onSessionAction"> & { activity: Activity }) {
  const session = activity.environmentId ? runtimeSessions?.[activity.environmentId] : undefined;
  if (!session || session.environmentId !== activity.environmentId || !activity.sessionId || session.sessionId !== activity.sessionId || !onSessionAction) return null;
  const disabled = blockedIds.includes(session.environmentId);
  return <div className="local-page-row-actions">{session.needsReconcile && <ReferenceButton disabled={disabled} onClick={() => onSessionAction(session.environmentId, session.sessionId, "reconcile")}>核对会话</ReferenceButton>}{session.canForce && !session.needsReconcile && <ReferenceButton className="danger" disabled={disabled} onClick={() => onSessionAction(session.environmentId, session.sessionId, "force")}>强制结束此会话</ReferenceButton>}</div>;
}

export function ActivityDetailWindow({ activity, native, runtimeSessions, blockedIds, onSessionAction, onClose }: Omit<ActivityPageProps, "activities" | "diagnostics" | "onExport"> & { activity: Activity; onClose(): void }) {
  return <LocalPageWindow title="操作记录详情" width={620} height={440} onClose={onClose} footer={<ReferenceButton onClick={onClose}>返回记录</ReferenceButton>}>
    <dl className="local-page-diagnostic-list"><dt>操作对象</dt><dd>{activity.target}</dd><dt>操作</dt><dd>{activity.action}</dd><dt>结果</dt><dd>{resultText[activity.result]}</dd><dt>时间</dt><dd>{localTime(activity.time)}</dd><dt>详情</dt><dd>{activity.detail}</dd>{activity.errorCode && <><dt>错误码</dt><dd>{activity.errorCode}</dd></>}{activity.nextAction && <><dt>下一步</dt><dd>{activity.nextAction}</dd></>}</dl>
    {native && <><p className="local-page-note">历史记录不是当前会话控制权；只有准确匹配仍受控的会话才提供动作。</p><SessionActions activity={activity} runtimeSessions={runtimeSessions} blockedIds={blockedIds} onSessionAction={onSessionAction} /></>}
  </LocalPageWindow>;
}

export function ActivityPage({ activities, native, diagnostics, onExport, ...sessionProps }: ActivityPageProps) {
  const [search, setSearch] = useState(""), [result, setResult] = useState("all"), [listPage, setListPage] = useState(1), [detail, setDetail] = useState<Activity>();
  const records = activities.filter(activity => (!search || `${activity.action} ${activity.target} ${activity.detail} ${activity.errorCode ?? ""}`.includes(search)) && (result === "all" || activity.result === result));
  const page = Math.min(listPage, Math.max(1, Math.ceil(records.length / 10)));
  return <section className="local-page local-page-activity" aria-label="操作记录">
    <div className="local-page-tabs"><span className="selected">操作记录</span></div>
    <div className="local-page-toolbar"><input className="local-page-search" aria-label="搜索操作记录" placeholder="输入对象或操作内容搜索" value={search} onChange={event => { setSearch(event.target.value); setListPage(1); }} /><select aria-label="记录结果" value={result} onChange={event => { setResult(event.target.value); setListPage(1); }}><option value="all">全部结果</option><option value="success">已完成</option><option value="error">失败</option><option value="info">记录 / 未确认完成</option></select><ReferenceButton onClick={() => { setSearch(""); setResult("all"); setListPage(1); }}>重置</ReferenceButton><div className="local-page-tail">{native ? diagnostics : onExport && <ReferenceButton onClick={onExport}>导出记录</ReferenceButton>}</div></div>
    <div className="local-page-panel"><LocalPageTable headings={["操作对象", "操作内容", "结果", "操作时间", "详情"]} empty={!records.length ? "没有匹配的操作记录。" : undefined}>{records.slice((page - 1) * 10, page * 10).map(activity => <tr key={activity.id}><td title={activity.target}>{activity.target}</td><td title={activity.action}>{activity.action}</td><td><span className={`local-page-status ${activity.result}`}>{resultText[activity.result]}</span></td><td>{localTime(activity.time)}</td><td><button className="local-page-link" aria-label={`${activity.action} ${activity.target} 详情`} onClick={() => setDetail(activity)}>详情</button></td></tr>)}</LocalPageTable><LocalPagination total={records.length} page={page} onPage={setListPage} /></div>
    {detail && <ActivityDetailWindow activity={detail} native={native} {...sessionProps} onClose={() => setDetail(undefined)} />}
  </section>;
}
