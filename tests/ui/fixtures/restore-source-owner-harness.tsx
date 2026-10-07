// Synthetic bridge/component evidence only; no native files or processes.
import { useState } from "react";
import { createRoot } from "react-dom/client";
import { WailsAdapter, type NativeBridge, type NativeRequest } from "../../../src/application/wails-adapter";
import type { ApplicationResult, NativeRestorePreview, NativeRestoreSourceState, WorkspaceView } from "../../../src/application/contract";
import { NativeRestoreManager } from "../../../src/components/NativeRestoreManager";
import "../../../src/styles.css";

const workspace: WorkspaceView = { mode: "native", state: { schemaVersion: 1, environments: [], proxies: [], kernels: [], backups: [], activities: [] } };
const ok = (data: unknown) => ({ ok: true, mode: "native", data });
const fail = (code = "BACKUP_PREFLIGHT_FAILED") => ({ ok: false, mode: "native", error: { code, message: "合成原暂存被占用；原来源保留。", retryable: true } });
const calls: NativeRequest[] = [];
let state: NativeRestoreSourceState | undefined, serial = 0, cleanupBusy = true, withoutToken = true, holdPreview = false;
let pendingPreview: (() => void) | undefined;
function makePreview(): NativeRestorePreview { return { mode: "native", previewId: `preview-${serial}`, format: "prism-local-backup", name: `合成原包-${serial}.prismbackup`, archiveSha256: "a".repeat(64), manifestSha256: "b".repeat(64), scope: "selected", createdAt: "2026-10-07", expiresAt: "2030-10-07", environmentCount: 0, addCount: 0, overwriteCount: 0, conflictCount: 0, missingKernelCount: 0, credentialReentryCount: 0, bytes: 1, canRestore: true, kernels: [], credentials: [] }; }
async function handle(request: NativeRequest): Promise<unknown> {
  calls.push(structuredClone(request)); const p = request.payload as Record<string, unknown>;
  if (request.method === "Backup.SelectRestoreSource") {
    serial++; state = { mode: "native", requestId: String(p.requestId), status: "selected", sourceToken: `host-token-${serial}`, name: `合成原包-${serial}.prismbackup`, preflightRunning: false, cleanupPending: false };
    return ok(withoutToken ? { ...state, sourceToken: undefined } : state);
  }
  if (request.method === "Backup.ReadRestoreSource") return state?.requestId === p.requestId ? ok(state) : fail("RESTORE_SOURCE_UNCONFIRMED");
  if (request.method === "Backup.PreviewRestore") {
    const original = state!; original.preflightRunning = true;
    const preview = makePreview();
    const finish = () => { original.preflightRunning = false; original.preview = preview; return ok(preview); };
    if (holdPreview) return new Promise(resolve => { pendingPreview = () => { pendingPreview = undefined; resolve(finish()); }; });
    return finish();
  }
  if (request.method === "Backup.ReadRestorePage") return ok({ mode: "native", previewId: p.previewId, offset: p.offset, total: 0, items: [] });
  if (request.method === "Backup.DiscardRestore") {
    if (!state || state.requestId !== p.requestId) return fail("RESTORE_SOURCE_UNCONFIRMED");
    if (cleanupBusy || state.preflightRunning) { state.cleanupPending = !state.preflightRunning; if (state.cleanupPending) state.preview = undefined; return fail(state.preflightRunning ? "PROFILE_BUSY" : "BACKUP_PREFLIGHT_FAILED"); }
    state = { ...state, status: "discarded", sourceToken: undefined, preview: undefined, cleanupPending: false }; return ok({ status: "discarded" });
  }
  return fail("CAPABILITY_UNSUPPORTED");
}
const bridge: NativeBridge = async <T,>(request: NativeRequest) => await handle(request) as ApplicationResult<T>;
const application = new WailsAdapter(bridge);
Object.assign(window, { __restoreOwner: { calls, release() { cleanupBusy = false; withoutToken = false; holdPreview = false; }, hold() { holdPreview = true; withoutToken = false; }, settle() { pendingPreview?.(); }, status() { return structuredClone(state); } } });
function Harness() {
  const [show, setShow] = useState(true);
  return <><button onClick={() => setShow(!show)}>{show ? "离开恢复页" : "返回恢复页"}</button>{show && <NativeRestoreManager application={application} workspace={workspace} />}</>;
}
createRoot(document.getElementById("root")!).render(<Harness />);
