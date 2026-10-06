import React, { useState, useSyncExternalStore } from "react";
import { createRoot } from "react-dom/client";
import { DemoAdapter } from "../../../src/application/demo-adapter";
import { WailsAdapter } from "../../../src/application/wails-adapter";
import { DemoProxyManager } from "../../../src/components/DemoProxyManager";
import { DemoKernelManager } from "../../../src/components/DemoKernelManager";
import { NativeProxyManager } from "../../../src/components/NativeProxyManager";
import { NativeKernelManager } from "../../../src/components/NativeKernelManager";
import { NativeMigrationManager } from "../../../src/components/NativeMigrationManager";
import { ProxyImportWindow } from "../../../src/components/ProxyImportWindow";
import "../../../src/styles.css";

// Fixture entry creates ONE ordinary adapter; pages never create another one.
const native = !!window.go;
const app = native ? new WailsAdapter(request => window.go.main.DesktopApp.Call(request)) : new DemoAdapter(localStorage);
if (native) void app.refresh();
function Harness() {
  const view = useSyncExternalStore(listener => app.subscribe(listener), () => app.getSnapshot());
  const [route, setRoute] = useState(window.__issue35Route ?? "proxies");
  const [importOpen, setImport] = useState(false), [nested, setNested] = useState(false), [checking, setChecking] = useState([]);
  const [draftName, setName] = useState("合成未保存草稿"), [draftProxy, setProxy] = useState("");
  const check = ids => {
    setChecking(ids);
    setTimeout(() => {
      app.compatibility.update(state => ({ ...state, proxies: state.proxies.map(proxy => ids.includes(proxy.id) ? { ...proxy, status: proxy.simulateFailure ? "failed" : "connected", latency: 42 } : proxy) }));
      setChecking([]);
    }, 50);
  };
  return <><div className="app-shell"><aside className="sidebar"><a className="brand" href="#"><div className="brand-mark">P</div><div><strong>棱镜浏览器</strong><span>PRISM BROWSER</span></div></a><nav aria-label="主导航">{["environments", "groups", "proxies", "kernels"].map((id, i) => <a href={`#/${id}`} className={`nav-item ${route === id ? "active" : ""}`} onClick={event => { event.preventDefault(); setRoute(id); }} key={id}>{["浏览器环境", "分组管理", "代理管理", "内核管理"][i]}</a>)}</nav></aside><div className="main-shell"><header className="topbar"><span className="topbar-local">本机工作区</span><div className="topbar-right"><span className="prototype-label">{native ? "本机合成桥" : "交互原型"}</span></div></header><main>
    {route === "proxies" ? native ? <NativeProxyManager application={app} workspace={view} importOpen={importOpen} onImportOpenChange={setImport} /> : <DemoProxyManager application={app} workspace={view} checking={checking} onCheck={check} /> : route === "kernels" ? native ? <><NativeKernelManager application={app} workspace={view} /><NativeMigrationManager application={app} workspace={view} /></> : <DemoKernelManager workspace={view} /> : <section className="work-card"><label>环境名称<input aria-label="环境名称" value={draftName} onChange={event => setName(event.target.value)} /></label><p>固定 seed：172600001 · 草稿代理 {draftProxy}</p><button className="button primary" onClick={() => setNested(true)}>从环境草稿导入代理</button></section>}
  </main></div></div><ProxyImportWindow application={app} open={nested} context="environment" onClose={() => setNested(false)} onImported={ids => setProxy(ids[0] ?? "")} /></>;
}
createRoot(document.getElementById("root")).render(<Harness />);
