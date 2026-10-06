import type { Page } from "@playwright/test";
import type { Environment } from "../../../src/domain.ts";
import type { EnvironmentPreview, Operation, RuntimeSession, WorkspaceView } from "../../../src/application/contract.ts";
import type { NativeRequest } from "../../../src/application/wails-adapter.ts";
import { referenceWorkspace } from "./reference-workspace.ts";

// Synthetic WailsAdapter boundary only. No native processes/files/network.
export async function nativeReferenceBridge(page: Page, busy = false) {
  await page.addInitScript(({ initial, busy }) => {
    Object.defineProperty(window, "localStorage", { get() { throw new Error("native reference fixture must never use demo storage"); } });
    const view: WorkspaceView = { mode: "native", state: initial, runtimeSessions: {}, networkResources: {}, kernelRecords: [{
      id: "core-148", version: "148.0.7778.215", architecture: "amd64", source: { kind: "official", location: "https://example.invalid/synthetic.zip", tag: "synthetic", commit: null }, archiveSha256: "a".repeat(64), executableSha256: "b".repeat(64), executableRelativePath: "chrome.exe", installPath: "kernels/synthetic", installedAt: "2026-10-06T01:00:00Z", status: "verified", usedBy: [], report: { adapterVersion: "synthetic-only", version: "synthetic-only", sampledAt: "2026-10-06T01:00:00Z", transport: "synthetic-only", sandbox: true, observations: [], capabilities: [] },
    }] };
    const calls: NativeRequest[] = [], revisions = Object.fromEntries(initial.environments.map(environment => [environment.id, 1]));
    const previews = new Map<string, EnvironmentPreview>(), operations = new Map<string, Operation>();
    const updates: { id: string; revision: unknown }[] = [];
    let serial = 0, failProxy = true, failGroup = true;
    const ok = (data: unknown) => ({ ok: true, mode: "native", data: structuredClone(data) });
    const error = (message: string) => ({ ok: false, mode: "native", error: { code: "VALIDATION_FAILED", message, retryable: true } });
    const task = (kind: Operation["kind"], id: string): Operation => {
      const operation: Operation = { id: `synthetic-task-${++serial}`, kind, state: "completed", total: 1, completedIds: [id], cancelRequested: false, environmentId: id };
      operations.set(operation.id, operation); return operation;
    };
    const sessionFor = (environment: Environment): RuntimeSession => ({
      mode: "native", environmentId: environment.id, sessionId: `synthetic-session-${environment.code}`, operationId: `synthetic-original-${environment.code}`, state: environment.status, revision: 1, fingerprintRevision: 1, kernelId: environment.coreId, userDataRef: `environments/${environment.id}/user-data`, networkPolicy: environment.proxyId ? "proxy" : "direct", canControl: true, canForce: false, needsReconcile: false, persistencePending: false, resourcesPending: false, ...(environment.proxyId ? { proxyId: environment.proxyId, proxyRevision: 1 } : {}),
    });
    if (busy) {
      for (let index = 2; index <= 7; index++) {
        const environment = view.state.environments[index]; environment.status = index === 7 ? "starting" : "error";
        const session = sessionFor(environment); view.runtimeSessions![environment.id] = session;
        if (index === 2) { session.needsReconcile = true; session.pid = 31003; session.nextAction = "先核对原会话，不会按PID结束进程"; }
        if (index === 3) { view.networkResources![environment.id] = session.sessionId; session.resourcesPending = true; }
        if (index === 4) session.persistencePending = true;
        if (index === 5) { session.pid = 31006; session.canForce = true; }
        if (index === 6) { session.pid = 31007; session.resourcesPending = true; }
        if (index === 7) { session.launchStage = "queued"; operations.set(session.operationId, { id: session.operationId, kind: "runtime-start", state: "accepted", total: 1, completedIds: [], cancelRequested: false }); }
      }
    }
    Object.assign(window, { __referenceNative: { calls, view, revisions, updates, allowProxy: () => { failProxy = false; }, allowGroup: () => { failGroup = false; } }, go: { main: { DesktopApp: { Call: async (request: NativeRequest) => {
      calls.push(structuredClone(request));
      const payload = request.payload as Record<string, unknown>;
      if (request.method === "Workspace.Read") {
        const query = payload.environmentQuery as { page: number; pageSize: number; search: string; group: string; status: string } | undefined;
        const all = view.state.environments, filtered = all.filter(environment => !query || (!query.search || `${environment.name} ${environment.code} ${environment.note}`.includes(query.search)) && (!query.group || environment.group === query.group) && (query.status === "all" || environment.status === query.status));
        const page = query?.page ?? 1, pageSize = query?.pageSize ?? 10;
        return ok({ ...view, state: { ...view.state, environments: filtered.slice((page - 1) * pageSize, page * pageSize) }, environmentPage: { page, pageSize, total: all.length, filteredTotal: filtered.length, groups: [...new Set(all.map(environment => environment.group))], runningCount: all.filter(environment => environment.status === "running").length, errorCount: all.filter(environment => environment.status === "error").length } });
      }
      if (request.method === "Environment.Preview") {
        const environment = view.state.environments.find(environment => environment.id === payload.sourceId);
        if (!environment) return error("指定环境不存在");
        const preview: EnvironmentPreview = { previewId: `synthetic-preview-${++serial}`, environment: structuredClone(environment), expectedRevision: revisions[environment.id], userDataRef: `environments/${environment.id}/user-data` };
        previews.set(preview.previewId, preview); return ok(preview);
      }
      if (request.method === "Preview.Discard") { previews.delete(String(payload.previewId)); return ok({ status: "discarded" }); }
      if (request.method === "Environment.Update") {
        const preview = previews.get(String(payload.previewId))!, configuration = payload.configuration as Environment;
        updates.push({ id: preview.environment.id, revision: payload.expectedRevision });
        if (preview.expectedRevision !== payload.expectedRevision) return error("错误目标或修订");
        if (failGroup && preview.environment.id === "synthetic-reference-11") return error("合成分组保存失败，旧标签保留");
        const environment = view.state.environments.find(environment => environment.id === preview.environment.id)!;
        environment.group = configuration.group; revisions[environment.id]++;
        return ok({ status: "completed", environment: { record: environment, revision: revisions[environment.id] }, newRevision: revisions[environment.id] });
      }
      if (request.method === "Runtime.Start" || request.method === "Runtime.Stop") {
        const environment = view.state.environments.find(environment => environment.id === payload.environmentId)!;
        const start = request.method === "Runtime.Start", operation = task(start ? "runtime-start" : "runtime-stop", environment.id);
        if (start && failProxy && environment.proxyId) { operation.state = "failed"; operation.completedIds = []; operation.error = { code: "PROXY_AUTH_FAILED", message: "合成认证失败；不回退直连", retryable: true }; environment.status = "error"; environment.error = operation.error.message; }
        else { environment.status = start ? "running" : "ready"; environment.error = undefined; }
        view.runtimeSessions![environment.id] = { ...sessionFor(environment), operationId: operation.id };
        return ok({ status: "accepted", operation });
      }
      if (request.method === "Runtime.Reconcile" || request.method === "Runtime.ForceStop") {
        const environment = view.state.environments.find(environment => environment.id === payload.environmentId)!, session = view.runtimeSessions![environment.id];
        if (payload.sessionId !== session.sessionId) return error("旧会话不能控制新会话");
        environment.status = "ready"; session.state = "ready"; session.needsReconcile = false; session.resourcesPending = false; session.canForce = false; session.pid = undefined;
        delete view.networkResources![environment.id];
        return ok({ status: "accepted", operation: task("runtime-stop", environment.id) });
      }
      if (request.method === "Operation.Read") return ok(operations.get(String(payload.operationId)));
      if (request.method === "Operation.Cancel") { const operation = operations.get(String(payload.operationId))!; operation.state = "cancelled"; operation.cancelRequested = true; const session = Object.values(view.runtimeSessions!).find(session => session.operationId === operation.id); if (session) { session.state = "ready"; view.state.environments.find(environment => environment.id === session.environmentId)!.status = "ready"; } return ok(operation); }
      return error(`合成桥未实现：${request.method}`);
    } } } } });
  }, { initial: referenceWorkspace(), busy });
}

export const nativeReferenceView = (page: Page) => page.evaluate(() => (window as unknown as { __referenceNative: { calls: NativeRequest[]; view: WorkspaceView; revisions: Record<string, number>; updates: { id: string; revision: number }[] } }).__referenceNative);
