// Synthetic component wiring only. MAIN owns the production App integration.
import { useEffect, useState, useSyncExternalStore } from "react";
import { createRoot } from "react-dom/client";
import { Activity, Bell, BookOpen, Box, CircleHelp, Fingerprint, Folder, Globe2, LayoutGrid, Monitor } from "lucide-react";
import { DemoAdapter } from "../../../src/application/demo-adapter";
import { WailsAdapter, type NativeBridge } from "../../../src/application/wails-adapter";
import type { ApplicationService } from "../../../src/application/contract";
import { createSnapshot, restoreSnapshot, type Snapshot } from "../../../src/domain";
import { BackupManagementPage } from "../../../src/components/BackupManagementPage";
import { DemoRestoreWindow } from "../../../src/components/DemoBackupPage";
import { ActivityPage } from "../../../src/components/ActivityPage";
import { NativeDiagnostics } from "../../../src/components/NativeDiagnostics";
import { HelpPage, type HelpDocTab, type HelpRoute } from "../../../src/components/HelpPage";
import user from "../../../docs/USER_GUIDE.md?raw";
import prd from "../../../docs/PRD.md?raw";
import development from "../../../docs/DEVELOPMENT.md?raw";
import kernel from "../../../docs/KERNEL.md?raw";
import "../../../src/styles.css";

const desktop = window as unknown as { go?: { main?: { DesktopApp?: { Call?: NativeBridge } } } };
const application: ApplicationService = desktop.go ? new WailsAdapter(request => desktop.go!.main!.DesktopApp!.Call!(request)) : new DemoAdapter(window.localStorage);
if (application.mode === "native") void application.refresh?.();
const docs = { user, prd, development, kernel };
const downloads: { name: string; content: string; type?: string }[] = [], sessionActions: { environmentId: string; sessionId: string; action: string }[] = [];
Object.assign(window, { __localPagesHarness: { application, downloads, sessionActions } });
function Harness() {
  const workspace = useSyncExternalStore(application.subscribe, application.getSnapshot), state = workspace.state;
  const [route, setRoute] = useState((location.hash.slice(2) || "backups") as HelpRoute | "groups");
  const [restore, setRestore] = useState<{ snapshot: Snapshot; name: string }>(), [error, setError] = useState("");
  const [docTab, setDocTab] = useState<HelpDocTab>(application.mode === "native" ? "user" : "prd");
  useEffect(() => { const change = () => setRoute(location.hash.slice(2) as HelpRoute); window.addEventListener("hashchange", change); return () => window.removeEventListener("hashchange", change); }, []);
  const download = (name: string, content: string, type?: string) => { downloads.push({ name, content, type }); };
  const navigate = (route: HelpRoute) => { location.hash = `/${route}`; };
  const create = () => {
    const snapshot = createSnapshot(state);
    const result = application.compatibility!.update(current => ({ ...current, backups: [{ id: crypto.randomUUID(), name: "工作区合成快照", createdAt: snapshot.createdAt, snapshot }, ...current.backups] }));
    if (!result.ok) { setError(result.error.message); return false; } setError(""); return true;
  };
  const confirmRestore = () => {
    if (!restore) return;
    try { const next = restoreSnapshot(state, restore.snapshot); const result = application.compatibility!.update(() => next); if (result.ok) { setRestore(undefined); setError(""); } else setError(result.error.message); }
    catch (error) { setError((error as Error).message); }
  };
  const selectedIds = new URLSearchParams(location.search).get("selected")?.split(",").filter(Boolean) ?? [];
  return <div className="app-shell"><aside className="sidebar"><a className="brand" href="#/environments"><div className="brand-mark"><Fingerprint size={25} /></div><div><strong>棱镜浏览器</strong><span>PRISM BROWSER</span></div></a><nav aria-label="主导航">{([{ route: "environments", text: "浏览器环境", icon: LayoutGrid }, { route: "groups", text: "分组管理", icon: Folder }, { route: "proxies", text: "代理管理", icon: Globe2 }, { route: "kernels", text: "内核管理", icon: Box }, { route: "backups", text: "备份恢复", icon: Folder }, { route: "activity", text: "操作记录", icon: Activity }, { route: "guide", text: "使用说明", icon: BookOpen }] as const).map(({ route: itemRoute, text, icon: Icon }) => <a key={itemRoute} className={`nav-item ${route === itemRoute ? "active" : ""}`} href={`#/${itemRoute}`}><Icon size={15} /><span>{text}</span></a>)}</nav></aside><div className="main-shell"><header className="topbar"><span className="topbar-local"><Monitor size={15} />本机工作区</span><div className="topbar-right"><span className="prototype-label">{application.mode === "native" ? "SYNTHETIC native bridge" : "交互原型 / synthetic demo"}</span><button className="icon-button" aria-label="查看操作记录" onClick={() => navigate("activity")}><Bell size={19} /></button><button className="icon-button" aria-label="使用说明与产品文档" onClick={() => navigate("guide")}><CircleHelp size={19} /></button></div></header><main>
    {route === "backups" && <BackupManagementPage application={application} workspace={workspace} selectedIds={selectedIds} demo={{ state, onCreate: create, onRestore: (snapshot, name) => { setError(""); setRestore({ snapshot, name }); }, onDownload: backup => download(`prism-snapshot-${backup.id}.json`, JSON.stringify(backup.snapshot, null, 2)), disabled: !!workspace.issue }} />}
    {route === "activity" && <ActivityPage activities={state.activities} native={application.mode === "native"} runtimeSessions={workspace.runtimeSessions} blockedIds={new URLSearchParams(location.search).get("blocked")?.split(",")} diagnostics={<NativeDiagnostics application={application} />} onExport={() => download("prism-activity.json", JSON.stringify(state.activities))} onSessionAction={(environmentId, sessionId, action) => { sessionActions.push({ environmentId, sessionId, action }); }} />}
    {route === "guide" && <HelpPage native={application.mode === "native"} docTab={docTab} text={docs[docTab]} onDocTab={setDocTab} onNavigate={navigate} download={download} docHref={href => !href ? "#/guide" : /^(https?:|#|\/#)/.test(href) ? href : /(?:PRD|DEVELOPMENT|KERNEL|USER_GUIDE)\.md/.test(href) ? "#/guide" : `https://github.com/axgiroud312-byte/prism-local-browser/${href?.includes("README.md") ? "" : `blob/main/docs/${href}`}`} />}
    {!["backups", "activity", "guide"].includes(route) && <p>本测试 harness 不实现其他票页面；使用导航返回 #36 页面。</p>}
    {restore && <DemoRestoreWindow {...restore} error={error} runningCount={state.environments.filter(e => ["running", "starting", "stopping"].includes(e.status)).length} onClose={() => setRestore(undefined)} onConfirm={confirmRestore} />}
  </main></div></div>;
}
createRoot(document.getElementById("root")!).render(<Harness />);
