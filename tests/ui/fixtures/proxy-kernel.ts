import type { Page } from "@playwright/test";
import { seedState } from "../../../src/domain.ts";

/** Synthetic bridge only: no filesystem, real proxy, Chromium or RPC. */
export async function proxyKernelBridge(page: Page, outcome: "failure" | "unknown" | "completed" = "completed") {
  const environment = seedState().environments[0];
  await page.addInitScript(({ environment, outcome }) => {
    Object.defineProperty(window, "localStorage", { get() { throw new Error("native must not use demo storage"); } });
    const calls: { method: string; payload: Record<string, unknown> }[] = [];
    const proxies = [
      { id: "synthetic-proxy-a", name: "合成 SOCKS", type: "socks5", host: "192.0.2.11", port: 1080, country: "US", revision: 1, hasAuthentication: true, status: "unchecked", usedBy: ["synthetic-environment"], usedCount: 1 },
      { id: "synthetic-proxy-b", name: "合成 HTTP", type: "http", host: "198.51.100.21", port: 8080, country: "", revision: 1, hasAuthentication: false, status: "unchecked", usedBy: [], usedCount: 0 },
    ];
    const kernels = ["synthetic-build-a", "synthetic-build-b"].map((id, i) => ({ id, version: i ? "151.0.9000.11" : "148.0.7778.215", architecture: "amd64", status: "verified", usedBy: i ? [] : ["synthetic-environment"], source: { kind: "official", location: "https://example.invalid/synthetic.zip", tag: i ? "151.0.9000.11" : "148.0.7778.215", commit: null }, archiveSha256: "a".repeat(64), executableSha256: "b".repeat(64), installPath: `kernels/${id}`, executableRelativePath: "chrome.exe", installedAt: "2026-10-06T00:00:00Z", report: { adapterVersion: "synthetic-not-probed", version: "synthetic-capability", sampledAt: "2026-10-06T00:00:00Z", transport: "synthetic-not-probed", sandbox: true, observations: [], capabilities: [{ field: "cpu", status: "configurable", source: "observed", note: "合成字段，不是真实观测" }, { field: "gpu", status: "seed-generated", source: "source-derived", note: "合成源码推导" }, { field: "screen/location/webgpu/tls/mac", status: "unverified", source: "not-probed", note: "未探测，不开放编辑" }] } }));
    const environments = [{ ...environment, id: "synthetic-environment", name: "合成环境 A", coreId: kernels[0].id, proxyId: proxies[0].id, seed: "172600001", status: "ready", note: "合成数据" }];
    let serial = 0, currentOutcome = outcome, updateOutcome = "completed", workspaceFailure = false, workspaceFailureAfterInstall = false, taskState = "running", persistencePending = false;
    const previews = new Map<string, { rows: Record<string, unknown>[] }>();
    const receipts = new Map<string, unknown>();
    const tasks = new Map<string, Record<string, unknown>>();
    const ok = (data: unknown) => ({ ok: true, mode: "native", data: structuredClone(data) });
    const fail = (code: string, message: string) => ({ ok: false, mode: "native", error: { code, message, retryable: true } });
    const workspace = () => ({ mode: "native", state: { schemaVersion: 1, environments, proxies: proxies.map(p => ({ ...p, username: "", password: "" })), kernels: kernels.map(k => ({ id: k.id, version: k.version, available: k.status === "verified", source: "fingerprint-chromium", note: "合成元数据" })), backups: [], activities: [] }, kernelRecords: kernels, nativeProxyRecords: proxies, kernelOperations: [...tasks.values()].filter(t => String(t.kind).startsWith("kernel-")), proxyOperations: [...tasks.values()].filter(t => t.kind === "proxy-check"), defaultKernel: { kernelId: kernels[0].id, revision: 1 } });
    Object.assign(window, { __proxyKernel: { calls, setOutcome(value: typeof outcome) { currentOutcome = value; }, setUpdateOutcome(value: string) { updateOutcome = value; }, setWorkspaceFailure(value: boolean) { workspaceFailure = value; }, failWorkspaceAfterNextInstall() { workspaceFailureAfterInstall = true; }, finish(state = "completed", pending = false) { taskState = state; persistencePending = pending; }, view: workspace }, go: { main: { DesktopApp: { Call: async (request: { method: string; payload: Record<string, unknown> }) => {
      calls.push(structuredClone(request));
      const p = request.payload;
      if (request.method === "Workspace.Read") return workspaceFailure ? fail("STORAGE_READ_FAILED", "合成本机工作区读取失败") : ok(workspace());
      if (request.method === "Migration.LookupEnvironments") return ok({ items: environments, page: 1, total: 1 });
      if (request.method === "Proxy.ParseImport") {
        const lines = String(p.text).split("\n");
        const rows = lines.flatMap<Record<string, unknown>>((text, i) => {
          if (!text.trim() || text.startsWith("#")) return [];
          try {
            const u = new URL(text.includes("://") ? text : `http://${text}`);
            if (!u.port || !["http:", "https:", "socks5:"].includes(u.protocol)) throw new Error();
            return [{ line: i + 1, configuration: { name: `合成导入 ${i+1}`, type: u.protocol.slice(0, -1), host: u.hostname, port: +u.port, country: "" }, hasAuthentication: !!u.username, duplicateCount: lines.filter(other => other === text).length - 1, existingCount: proxies.filter(proxy => proxy.host === u.hostname && proxy.port === +u.port).length }];
          } catch { return [{ line: i + 1, error: "代理格式错误", hasAuthentication: false, duplicateCount: 0, existingCount: 0 }]; }
        });
        const previewId = `synthetic-import-${++serial}`; previews.set(previewId, { rows });
        return ok({ mode: "native", previewId, rows, ignoredLines: lines.length - rows.length, expiresAt: "2099-10-06T01:00:00Z", duplicateGroups: {} });
      }
      if (request.method === "Proxy.DiscardImport") { previews.delete(String(p.previewId)); return ok({ status: "discarded" }); }
      if (request.method === "Proxy.CommitImport") {
        if (receipts.has(String(p.requestId))) return ok(receipts.get(String(p.requestId)));
        if (currentOutcome === "failure") return fail("STORAGE_WRITE_FAILED", "合成保存失败，未提交");
        if (currentOutcome === "unknown") return fail("NATIVE_UNAVAILABLE", "合成回执未知");
        const preview = previews.get(String(p.previewId)); if (!preview) return fail("VALIDATION_FAILED", "预览失效");
        const rows = preview.rows.filter(r => (p.selectedRows as number[]).includes(Number(r.line)) && r.configuration);
        const ids = rows.map(r => { const id = `synthetic-imported-${++serial}`; proxies.push({ ...r.configuration as typeof proxies[0], id, revision: 1, hasAuthentication: !!r.hasAuthentication, status: "unchecked", usedBy: [], usedCount: 0 }); return id; });
        const receipt = { status: "completed", importedIds: ids, importedLines: rows.map(r => r.line) }; receipts.set(String(p.requestId), receipt); return ok(receipt);
      }
      if (request.method === "Proxy.Update") {
        if (receipts.has(String(p.requestId))) return ok(receipts.get(String(p.requestId)));
        const record = proxies.find(proxy => proxy.id === p.proxyId)!;
        if (p.expectedRevision !== record.revision) return fail("REVISION_CONFLICT", "合成修订不符，未修改");
        Object.assign(record, p.configuration, { revision: record.revision + 1, status: "unchecked" });
        if ((p.credentials as { action: string }).action !== "keep") record.hasAuthentication = (p.credentials as { action: string }).action === "replace";
        const receipt = { status: "completed", record: structuredClone(record) };
        receipts.set(String(p.requestId), receipt);
        return updateOutcome === "unknown" ? fail("NATIVE_UNAVAILABLE", "合成保存已受理，但回执未知") : ok(receipt);
      }
      if (request.method === "Proxy.Delete") { const index = proxies.findIndex(proxy => proxy.id === p.proxyId); if (proxies[index].usedCount) return fail("PROFILE_BUSY", "被引用"); proxies.splice(index, 1); return ok({ status: "completed", deletedId: p.proxyId }); }
      if (["Kernel.Install", "Kernel.Verify", "Kernel.Delete", "Proxy.Check"].includes(request.method)) {
        const kind = request.method === "Proxy.Check" ? "proxy-check" : `kernel-${request.method.split(".")[1].toLowerCase()}`;
        const id = `synthetic-task-${++serial}`;
        const operation = { id, kind, state: "running", stage: kind === "proxy-check" ? "authentication" : "probing", total: 1, completedIds: [], cancelRequested: false, kernelId: p.kernelId, proxyId: p.proxyId };
        tasks.set(id, operation); taskState = "running"; persistencePending = false;
        // Fault only after admission so the owner's normal refresh observes it.
        if (request.method === "Kernel.Install" && workspaceFailureAfterInstall) { workspaceFailureAfterInstall = false; workspaceFailure = true; }
        return ok({ status: "accepted", operation });
      }
      if (request.method === "Operation.Read") { const operation = tasks.get(String(p.operationId)); if (!operation) return fail("VALIDATION_FAILED", "未知合成任务"); Object.assign(operation, { state: taskState, persistencePending, stage: taskState === "running" ? operation.stage : taskState, ...(taskState === "failed" ? { error: { code: operation.kind === "proxy-check" ? "PROXY_AUTH_FAILED" : "KERNEL_INTEGRITY_FAILED", message: "合成核验失败", retryable: true } } : {}) }); return ok(operation); }
      if (request.method === "Operation.Cancel") { const operation = tasks.get(String(p.operationId)); if (!operation) return fail("VALIDATION_FAILED", "未知任务"); Object.assign(operation, { state: "cancelled", stage: "cancelled", cancelRequested: true }); taskState = "cancelled"; return ok(operation); }
      if (request.method === "Kernel.SelectArchive") return ok({ status: "selected", archiveToken: "synthetic-archive-token", name: "synthetic-kernel.zip" });
      return fail("CAPABILITY_UNSUPPORTED", "合成桥不支持此方法");
    } } } } });
  }, { environment, outcome });
}
