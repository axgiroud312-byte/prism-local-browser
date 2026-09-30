import { createRoot } from "react-dom/client";
import App from "./App";
import { DemoAdapter } from "./application/demo-adapter";
import { WailsAdapter, type NativeBridge } from "./application/wails-adapter";
import type { ApplicationService } from "./application/contract";
import "./styles.css";

const desktop = window as unknown as { go?: { main?: { DesktopApp?: { Call?: NativeBridge } } }; runtime?: unknown };
let application: ApplicationService;
if (import.meta.env.MODE === "desktop" || desktop.go || desktop.runtime) {
  const native = new WailsAdapter(request => {
    const call = desktop.go?.main?.DesktopApp?.Call;
    if (!call) throw new Error("native bridge unavailable");
    return call(request);
  });
  application = native;
  void native.refresh();
} else {
  application = new DemoAdapter({
    getItem: key => window.localStorage.getItem(key),
    setItem: (key, value) => window.localStorage.setItem(key, value),
    removeItem: key => window.localStorage.removeItem(key),
  });
}
createRoot(document.getElementById("root")!).render(<App application={application} />);
