import { createRoot } from "react-dom/client";
import App from "./App";
import { DemoAdapter } from "./application/demo-adapter";
import "./styles.css";

const application = new DemoAdapter({
  getItem: key => window.localStorage.getItem(key),
  setItem: (key, value) => window.localStorage.setItem(key, value),
  removeItem: key => window.localStorage.removeItem(key),
});
createRoot(document.getElementById("root")!).render(<App application={application} />);
