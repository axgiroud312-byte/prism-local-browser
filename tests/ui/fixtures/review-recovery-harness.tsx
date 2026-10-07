// Synthetic component seam only: intentionally never imports the active App.
import { useState, useSyncExternalStore } from "react";
import { createRoot } from "react-dom/client";
import type { ApplicationService } from "../../../src/application/contract";
import { WailsAdapter, type NativeBridge } from "../../../src/application/wails-adapter";
import { NativeProxyManager } from "../../../src/components/NativeProxyManager";
import { NativeRestoreManager } from "../../../src/components/NativeRestoreManager";
import "../../../src/styles.css";

const desktop = window as unknown as {
  go: { main: { DesktopApp: { Call: NativeBridge } } };
  __reviewRecoveryRestore?: { bind(application: ApplicationService): void };
};
const application = new WailsAdapter(request => desktop.go.main.DesktopApp.Call(request));
desktop.__reviewRecoveryRestore?.bind(application);
Object.assign(window, { __reviewRecoveryApplication: application });
void application.refresh();

function Harness() {
  const workspace = useSyncExternalStore(application.subscribe, application.getSnapshot);
  const [importOpen, setImportOpen] = useState(false);
  return <div className="app-shell"><aside className="sidebar" data-app-inert="false">合成组件检查</aside><div className="main-shell" data-app-inert="false"><main>
    {location.hash === "#/restore"
      ? <NativeRestoreManager application={application} workspace={workspace} />
      : <NativeProxyManager application={application} workspace={workspace} importOpen={importOpen} onImportOpenChange={setImportOpen} />}
  </main></div></div>;
}
createRoot(document.getElementById("root")!).render(<Harness />);
