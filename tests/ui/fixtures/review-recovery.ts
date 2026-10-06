import type { Page } from "@playwright/test";
import type { ApplicationResult, ApplicationService, NativeRestorePage, NativeRestorePreview, Operation, ProxyCheckReport, WorkspaceView } from "../../../src/application/contract";
import type { NativeBridge, NativeRequest } from "../../../src/application/wails-adapter";
import { proxyKernelBridge } from "./proxy-kernel";
import { localPagesNativeBridge } from "./local-pages-native-bridge";

export type RestoreRead = "Backup.PreviewRestore" | "Backup.ReadRestorePage";
export type DiscardOutcome = "unavailable" | "refused" | "throw" | "success";
export interface ProxyReviewData { recordReport?: ProxyCheckReport; operations: Operation[] }
export interface ReviewApplication { refresh(): Promise<unknown> }
export interface ProxyReviewControl { calls: NativeRequest[]; setData(data: ProxyReviewData): void }
export interface RestoreReviewControl {
  calls: NativeRequest[];
  workspace: WorkspaceView;
  holdRead(method: RestoreRead): void;
  failNextRead(method: RestoreRead): void;
  settleRead(index?: number): void;
  settleDiscard(): void;
  setDiscardOutcome(outcome: DiscardOutcome): void;
  status(): { pendingReads: number; pendingDiscard: boolean; tokens: string[]; previews: string[] };
}

export async function openReviewHarness(page: Page, route: "proxies" | "restore") {
  await page.route("**/review-recovery-component-harness", request => request.fulfill({ contentType: "text/html", body: '<!doctype html><html lang="zh-CN"><head><meta charset="UTF-8"><title>合成审查回归</title></head><body><div id="root"></div><script type="module" src="/tests/ui/fixtures/review-recovery-harness.tsx"></script></body></html>' }));
  await page.goto(`/review-recovery-component-harness#/${route}`);
}

// Decorate the existing #35 bridge, retaining its protected binding, fixed
// seed, masked credentials and ordinary WailsAdapter request/refresh path.
export async function proxyReviewBridge(page: Page, initial: ProxyReviewData) {
  await proxyKernelBridge(page);
  await page.addInitScript(initial => {
    const desktop = (window as unknown as { go: { main: { DesktopApp: { Call: NativeBridge } } } }).go.main.DesktopApp;
    const original = desktop.Call;
    const calls = (window as unknown as { __proxyKernel: { calls: NativeRequest[] } }).__proxyKernel.calls;
    let data = structuredClone(initial);
    Object.assign(window, { __reviewRecoveryProxy: { calls, setData(next: ProxyReviewData) { data = structuredClone(next); } } });
    desktop.Call = (async (request: NativeRequest) => {
      const payload = request.payload as Record<string, unknown>;
      if (request.method === "Operation.Read") {
        const operation = data.operations.find(operation => operation.id === payload.operationId);
        if (operation) { calls.push(structuredClone(request)); return { ok: true, mode: "native", data: structuredClone(operation) }; }
      }
      const result = await original<WorkspaceView>(request);
      if (result.ok && request.method === "Workspace.Read") {
        result.data.nativeProxyRecords![0].checkReport = structuredClone(data.recordReport);
        result.data.nativeProxyRecords![0].status = data.recordReport ? data.recordReport.error ? "failed" : "connected" : "unchecked";
        // Real listProxyOperations returns rowid DESC, not Map insertion order.
        result.data.proxyOperations = structuredClone(data.operations);
      }
      if (result.ok && request.method === "Proxy.Update") data.recordReport = undefined;
      return result;
    }) as NativeBridge;
  }, initial);
}

// Reuse the #36 valid native preview/page and immutable workspace. Only these
// scoped read-only calls gain deterministic gates and exact token ownership.
export async function restoreReviewBridge(page: Page, phase?: RestoreRead, outcome: DiscardOutcome = "unavailable") {
  await localPagesNativeBridge(page);
  await page.addInitScript(({ phase, outcome }) => {
    const desktop = (window as unknown as { go: { main: { DesktopApp: { Call: NativeBridge } } } }).go.main.DesktopApp;
    const original = desktop.Call;
    const fixture = (window as unknown as { __localPagesFixture: { calls: NativeRequest[]; workspace: WorkspaceView } }).__localPagesFixture;
    const tokens = new Map<string, string>(), previews = new Map<string, string>();
    const reads: { resolve(): void; settled: boolean }[] = [];
    let serial = 0, holdNext = phase, failNext: RestoreRead | undefined, discardOutcome = outcome, holdDiscard = true, pendingDiscard: (() => void) | undefined;
    const ok = <T,>(data: T): ApplicationResult<T> => ({ ok: true, mode: "native", data: structuredClone(data) });
    const fail = (code: string): ApplicationResult<never> => ({ ok: false, mode: "native", error: { code, message: "合成取消未被确认；只读输入保留。", retryable: true } });
    function gate<T>(method: RestoreRead, result: ApplicationResult<T>) {
      if (holdNext !== method) return Promise.resolve(result);
      holdNext = undefined;
      if (!reads.length && result.ok && method === "Backup.ReadRestorePage") (result.data as NativeRestorePage).items.forEach(item => { item.name = "迟到旧清单"; });
      if (!reads.length && result.ok && method === "Backup.PreviewRestore") (result.data as NativeRestorePreview).name = "迟到旧预览.prismbackup";
      return new Promise<ApplicationResult<T>>(resolve => { reads.push({ settled: false, resolve: () => resolve(structuredClone(result)) }); });
    }
    function discard(previewId: string, sourceToken: string) {
      fixture.calls.push({ mode: "native", method: "Backup.DiscardRestore", payload: { previewId, sourceToken } });
      return new Promise<ApplicationResult<{ status: "discarded" }>>((resolve, reject) => {
        const finish = () => {
          if (sourceToken && discardOutcome !== "success") {
            if (discardOutcome === "throw") reject(new Error("synthetic rejected discard"));
            else resolve(fail(discardOutcome === "refused" ? "PROFILE_BUSY" : "NATIVE_UNAVAILABLE"));
            return;
          }
          if (sourceToken) tokens.delete(sourceToken);
          for (const [id, token] of previews) if (id === previewId || sourceToken && token === sourceToken) previews.delete(id);
          resolve(ok({ status: "discarded" as const }));
        };
        if (sourceToken && holdDiscard) { holdDiscard = false; pendingDiscard = finish; } else finish();
      });
    }
    const control = {
      calls: fixture.calls, workspace: fixture.workspace,
      holdRead(method: RestoreRead) { holdNext = method; },
      failNextRead(method: RestoreRead) { failNext = method; },
      settleRead(index = 0) { const read = reads[index]; if (!read || read.settled) throw new Error("No exact unsettled synthetic read"); read.settled = true; read.resolve(); },
      settleDiscard() { if (!pendingDiscard) throw new Error("No pending synthetic discard"); const finish = pendingDiscard; pendingDiscard = undefined; finish(); },
      setDiscardOutcome(next: DiscardOutcome) { discardOutcome = next; },
      status() { return { pendingReads: reads.filter(read => !read.settled).length, pendingDiscard: !!pendingDiscard, tokens: [...tokens.keys()], previews: [...previews.keys()] }; },
      bind(application: ApplicationService) {
        const adapterDiscard = application.discardRestore!.bind(application);
        // Wails invoke deliberately converts bridge throws into error envelopes.
        // Also exercise a rejecting service Promise at the component boundary.
        application.discardRestore = (previewId, sourceToken) => sourceToken && discardOutcome === "throw" ? discard(previewId, sourceToken) : adapterDiscard(previewId, sourceToken);
      },
    };
    Object.assign(window, { __reviewRecoveryRestore: control });
    desktop.Call = (async (request: NativeRequest) => {
      const payload = request.payload as { sourceToken?: string; previewId?: string };
      if (request.method === "Backup.DiscardRestore") return discard(payload.previewId ?? "", payload.sourceToken ?? "");
      if (request.method === failNext) { failNext = undefined; fixture.calls.push(structuredClone(request)); return fail("PREVIEW_EXPIRED"); }
      if (request.method === "Backup.PreviewRestore") {
        if (!tokens.has(payload.sourceToken!)) { fixture.calls.push(structuredClone(request)); return fail("PREVIEW_EXPIRED"); }
        const result = await original<NativeRestorePreview>({ ...request, payload: { sourceToken: "synthetic-source" } });
        // Keep the actual UI payload in the existing fixture's request ledger.
        fixture.calls[fixture.calls.length - 1] = structuredClone(request);
        if (result.ok) { result.data.name = tokens.get(payload.sourceToken!)!; previews.set(result.data.previewId, payload.sourceToken!); }
        return gate("Backup.PreviewRestore", result);
      }
      if (request.method === "Backup.ReadRestorePage") {
        if (!previews.has(payload.previewId!)) { fixture.calls.push(structuredClone(request)); return fail("PREVIEW_EXPIRED"); }
        return gate("Backup.ReadRestorePage", await original<NativeRestorePage>(request));
      }
      const result = await original<{ status: string; sourceToken?: string; name?: string }>(request);
      if (request.method === "Backup.SelectRestoreSource" && result.ok && result.data.status === "selected") {
        const token = `synthetic-review-source-${++serial}`, name = `synthetic-review-${serial}.prismbackup`;
        tokens.set(token, name); result.data.sourceToken = token; result.data.name = name;
      }
      return result;
    }) as NativeBridge;
  }, { phase, outcome });
}
