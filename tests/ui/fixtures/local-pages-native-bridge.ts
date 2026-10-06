import { createHash } from "node:crypto";
import type { Page } from "@playwright/test";
import type { DiagnosticPreview, NativeRestorePreview, Operation, WorkspaceView } from "../../../src/application/contract";
import type { NativeRequest } from "../../../src/application/wails-adapter";
import { referenceWorkspace } from "./reference-workspace.ts";

export type LocalPageScenario = "baseline" | "backup-cancel" | "backup-unknown" | "backup-unknown-no-id" | "backup-failed" | "backup-publication-pending" | "restore-rollback" | "restore-protected" | "restore-unknown" | "restore-progress" | "restore-success" | "restore-conflict" | "diagnostics-cancel" | "diagnostics-unknown" | "source-cancel" | "preflight-failed" | "diagnostics-unavailable" | "activity-sessions";

// Independent, synthetic #36 fixture. Native I/O is never called. Unknown
// methods fail closed, requests are memoized, native demo storage throws.
export async function localPagesNativeBridge(page: Page, scenario: LocalPageScenario = "baseline") {
  const state = referenceWorkspace();
  state.activities = [{ id: "synthetic-activity", time: "2026-10-06T01:00:00Z", action: "创建合成环境", target: "工作环境 A", result: "info", detail: "合成记录，未执行真实桌面操作。" }];
  const initial: WorkspaceView = { mode: "native", state, runtimeSessions: {}, backupOperations: [], restoreOperations: [], nativeBackups: [], networkResources: {} };
  if (scenario === "activity-sessions") {
    for (const [index, environment] of state.environments.slice(0, 2).entries()) {
      const sessionId = `synthetic-current-session-${index + 1}`;
      environment.status = index ? "error" : "running";
      initial.runtimeSessions![environment.id] = { mode: "native", environmentId: environment.id, sessionId, operationId: `synthetic-session-operation-${index + 1}`, state: environment.status, revision: 1, fingerprintRevision: 1, kernelId: environment.coreId, userDataRef: `environments/${environment.id}/user-data`, networkPolicy: environment.proxyId ? "proxy" : "direct", ...(environment.proxyId ? { proxyId: environment.proxyId } : {}), canControl: false, canForce: !index, needsReconcile: !!index, persistencePending: false };
      state.activities.push({ id: `synthetic-session-record-${index + 1}`, time: "2026-10-06T01:00:00Z", action: index ? "待核对合成会话" : "当前受控合成会话", target: environment.name, environmentId: environment.id, sessionId, result: "error", detail: "仅用于准确会话动作边界测试。", errorCode: "SYNTHETIC_SESSION_ERROR", nextAction: "核对原会话，不控制后来会话。" });
    }
    state.activities.push({ ...state.activities[1], id: "synthetic-stale-session-record", action: "旧合成会话", sessionId: "synthetic-old-session" });
  }
  const report: DiagnosticPreview["report"] = {
    format: "prism-local-diagnostics", schemaVersion: 1, generatedAt: "2026-10-06T01:00:00Z",
    application: { version: "0.0.0-synthetic-only", platform: "windows", architecture: "amd64", goVersion: "synthetic-not-run", signature: "not-checked" },
    proxyProtection: "unavailable", excluded: ["names", "private-paths", "credentials", "cookie-values", "browser-content", "seeds", "raw-ids", "raw-logs"],
    workspace: { status: scenario === "diagnostics-unavailable" ? "unavailable" : "partial", counts: { environments: scenario === "diagnostics-unavailable" ? 0 : 12, proxies: scenario === "diagnostics-unavailable" ? 0 : 2, kernels: scenario === "diagnostics-unavailable" ? 0 : 2 }, ...(scenario === "diagnostics-unavailable" ? { startupCode: "STORAGE_READ_FAILED" } : {}), maintenance: [], operations: [], sessions: [], kernels: [], unavailableSections: ["live-observations"], omittedRecords: 0, observationSource: "saved-only", operationLimit: 100, sessionLimit: 100, kernelLimit: 20 },
  };
  const diagnostic: DiagnosticPreview = { reportId: "00000000-0000-4000-8000-000000000036", sha256: createHash("sha256").update(JSON.stringify(report)).digest("hex"), bytes: Buffer.byteLength(JSON.stringify(report)), expiresAt: "2030-01-01T00:00:00Z", report };
  await page.addInitScript(({ initial, scenario, diagnostic }) => {
    const view = structuredClone(initial), calls: NativeRequest[] = [], operations: Record<string, Operation> = {}, receipts = new Map<string, { signature: string; id: string }>();
    const previews = new Map<string, NativeRestorePreview>(), diagnosticRequests = new Map<string, string>();
    let serial = 0, source = false, destination = false;
    const copy = <T,>(value: T) => structuredClone(value);
    const ok = (data: unknown) => ({ ok: true, mode: "native", data: copy(data) });
    const fail = (code: string, operationId?: string) => ({ ok: false, mode: "native", error: { code, message: "合成错误；原数据与请求保留。", retryable: true }, ...(operationId ? { operationId } : {}) });
    const store = (op: Operation) => {
      operations[op.id] = op;
      if (op.kind === "backup-export") view.backupOperations = Object.values(operations).filter(item => item.kind === "backup-export");
      else { view.restoreOperations = Object.values(operations).filter(item => item.kind === "backup-restore"); view.maintenance = op.restoreReport?.protected || op.persistencePending ? op : undefined; }
      return op;
    };
    const rollback = (op: Operation) => { op.state = "failed"; op.stage = "rolled-back"; op.persistencePending = false; op.restoreReport = { ...op.restoreReport!, sequence: op.restoreReport!.sequence + 1, protected: false, rolledBack: true }; op.error = { code: "RESTORE_ROLLED_BACK", message: "合成失败，旧状态已核对回滚。", retryable: true }; return store(op); };
    const publish = (op: Operation) => {
      op.state = "completed"; op.stage = "completed"; op.persistencePending = false; op.backupReport = { ...op.backupReport!, sequence: op.backupReport!.sequence + 1, copiedEnvironmentCount: op.total, fileCount: 2, byteCount: 4096, published: true, archiveSha256: "a".repeat(64), manifestSha256: "c".repeat(64) };
      view.nativeBackups = [{ id: "synthetic-backup", operationId: op.id, name: "synthetic.prismbackup", createdAt: "2026-10-06T01:00:00Z", scope: op.backupReport.scope, environmentCount: op.total, archiveSha256: "a".repeat(64), manifestSha256: "c".repeat(64) }]; return store(op);
    };
    Object.assign(window, { __localPagesFixture: {
      syntheticOnly: true, calls, operations, workspace: view,
      publish(id: string) { const op = operations[id]; if (!op || op.kind !== "backup-export" || op.state === "completed") throw new Error("Only an unsettled exact backup may advance"); publish(op); },
      rollback(id: string) { const op = operations[id]; if (!op || op.kind !== "backup-restore" || op.state === "failed" && !op.persistencePending) throw new Error("Only an unsettled exact restore may advance"); rollback(op); },
    } });
    Object.defineProperty(window, "localStorage", { get() { throw new Error("synthetic native must never use demo storage"); } });
    Object.assign(window, { go: { main: { DesktopApp: { Call: async (request: NativeRequest) => {
      calls.push(copy(request)); const p = request.payload as Record<string, unknown>;
      if (request.method === "Workspace.Read") { const all = view.state.environments, query = p.environmentQuery as { page?: number; pageSize?: number } | undefined; const page = query?.page ?? 1, pageSize = query?.pageSize ?? 10; return ok({ ...view, state: { ...view.state, environments: all.slice((page - 1) * pageSize, page * pageSize) }, environmentPage: { page, pageSize, total: all.length, filteredTotal: all.length, groups: ["工作环境", "测试环境"], runningCount: 0, errorCount: 0 } }); }
      if (request.method === "Backup.SelectDestination") { if (scenario === "backup-cancel") return ok({ status: "cancelled" }); destination = true; return ok({ status: "selected", destinationToken: "synthetic-destination", name: "synthetic.prismbackup" }); }
      if (request.method === "Backup.SelectRestoreSource") { if (scenario === "source-cancel") return ok({ status: "cancelled" }); source = true; return ok({ status: "selected", sourceToken: "synthetic-source", name: "synthetic.prismbackup" }); }
      if (request.method === "Backup.PreviewRestore") {
        if (!source || p.sourceToken !== "synthetic-source") return fail("PREVIEW_EXPIRED");
        if (scenario === "preflight-failed") return fail("BACKUP_INVALID");
        const conflict = scenario === "restore-conflict";
        const preview: NativeRestorePreview = { mode: "native", previewId: `synthetic-preflight-${++serial}`, format: "prism-local-backup", name: "synthetic.prismbackup", archiveSha256: "a".repeat(64), manifestSha256: "c".repeat(64), scope: "selected", createdAt: "2026-10-06T01:00:00Z", expiresAt: "2030-01-01T00:00:00Z", environmentCount: 2, addCount: 1, overwriteCount: 1, conflictCount: conflict ? 1 : 0, missingKernelCount: conflict ? 1 : 0, credentialReentryCount: 1, bytes: 4096, canRestore: !conflict, kernels: [{ id: "core-148", version: "148.0.7778.215", archiveSha256: "a".repeat(64), executableSha256: "b".repeat(64), localId: "core-148", state: conflict ? "missing" : "verified-bytes", required: true }], credentials: [{ proxyId: "synthetic-socks", state: "reentry-required" }] };
        previews.set(preview.previewId, preview); return ok(preview);
      }
      if (request.method === "Backup.ReadRestorePage") { if (!previews.has(String(p.previewId))) return fail("PREVIEW_EXPIRED"); return ok({ mode: "native", previewId: p.previewId, offset: p.offset, total: 2, items: [{ id: "synthetic-reference-1", name: "工作环境 A", seed: "180000001", action: "overwrite", currentRevision: 2, backupRevision: 1, dataState: "present", busy: false, conflicts: scenario === "restore-conflict" ? ["name-conflict"] : [] }, { id: "synthetic-restored", name: "测试环境 C 99", seed: "180000099", action: "add", currentRevision: 0, backupRevision: 1, dataState: "never-initialized", busy: false, conflicts: [] }].slice(Number(p.offset), Number(p.offset) + Number(p.pageSize)) }); }
      if (request.method === "Backup.DiscardRestore") { previews.delete(String(p.previewId)); source = false; return ok({ status: "discarded" }); }
      if (request.method === "Backup.Export" || request.method === "Backup.ApplyRestore") {
        const signature = JSON.stringify(request), receipt = receipts.get(String(p.requestId));
        if (receipt && receipt.signature !== signature) return fail("REQUEST_ID_REUSED");
        if (receipt) { const op = operations[receipt.id]; return op.stage === "acceptance-pending" ? fail("NATIVE_UNAVAILABLE", op.id) : ok({ status: "accepted", operation: op }); }
        if (request.method === "Backup.Export") {
          const ids = p.environmentIds as string[];
          if (!destination || p.destinationToken !== "synthetic-destination" || p.stopRunning !== true || p.scope === "all" && ids.length || p.scope === "selected" && (!ids.length || ids.some(id => !view.state.environments.some(e => e.id === id)))) return fail("VALIDATION_FAILED");
          const total = p.scope === "all" ? view.state.environments.length : ids.length;
          const op: Operation = { id: `synthetic-export-${++serial}`, kind: "backup-export", state: "running", stage: "copying-browser-data", total, completedIds: [], cancelRequested: false, backupReport: { mode: "native", format: "prism-local-backup", schemaVersion: 1, requestId: String(p.requestId), scope: p.scope as "all" | "selected", environmentCount: total, copiedEnvironmentCount: 0, fileCount: 0, byteCount: 0, sequence: 1, published: false, name: "synthetic.prismbackup", credentials: "windows-current-user-dpapi", browserData: "sensitive-same-user-not-portable", kernelBinariesIncluded: false } };
          if (scenario === "backup-unknown" || scenario === "backup-unknown-no-id") { op.state = "accepted"; op.stage = "acceptance-pending"; op.persistencePending = true; }
          if (scenario === "backup-failed") { op.state = "failed"; op.stage = "failed"; op.error = { code: "DISK_FULL", message: "合成空间不足，未发布。", retryable: true }; }
          if (scenario === "backup-publication-pending") { publish(op); view.nativeBackups = []; op.persistencePending = true; }
          store(op); receipts.set(String(p.requestId), { id: op.id, signature }); return scenario === "backup-unknown-no-id" ? fail("NATIVE_UNAVAILABLE") : scenario === "backup-unknown" ? fail("NATIVE_UNAVAILABLE", op.id) : ok({ status: "accepted", operation: op });
        }
        const preview = previews.get(String(p.previewId));
        if (!preview || !preview.canRestore || p.archiveSha256 !== preview.archiveSha256 || p.confirmOverwrite !== true || p.acknowledgeCredentials !== true || p.stopRunning !== true) return fail("BACKUP_INVALID");
        const op: Operation = { id: `synthetic-restore-${++serial}`, kind: "backup-restore", state: "running", stage: "prepared", total: 2, completedIds: [], cancelRequested: false, restoreReport: { mode: "native", requestId: String(p.requestId), previewId: preview.previewId, archiveSha256: preview.archiveSha256, sequence: 1, environmentCount: 2, switchedCount: 0, credentialReentryCount: 1, committed: false, rolledBack: false, protected: false } };
        if (scenario === "restore-unknown") { op.state = "accepted"; op.stage = "acceptance-pending"; op.persistencePending = true; op.restoreReport!.protected = true; }
        else if (scenario === "restore-protected") { op.state = "failed"; op.stage = "protected"; op.persistencePending = true; op.restoreReport!.protected = true; op.restoreReport!.recoveredAfterRestart = true; op.restoreReport!.interruptedStage = "swapping"; op.error = { code: "RESTORE_INCOMPLETE", message: "合成目录占用，维护保护保持。", retryable: true }; }
        else if (scenario === "restore-success") { op.state = "completed"; op.stage = "finalized"; op.restoreReport!.committed = true; op.restoreReport!.switchedCount = 2; }
        else if (scenario !== "restore-progress") rollback(op);
        store(op); receipts.set(String(p.requestId), { id: op.id, signature }); return scenario === "restore-unknown" ? fail("NATIVE_UNAVAILABLE", op.id) : ok({ status: "accepted", operation: op });
      }
      if (request.method === "Backup.RecoverRestore") { const op = operations[String(p.operationId)]; return op?.kind === "backup-restore" ? ok(rollback(op)) : fail("NOT_FOUND"); }
      if (request.method === "Operation.Read") return operations[String(p.operationId)] ? ok(operations[String(p.operationId)]) : fail("NOT_FOUND");
      if (request.method === "Operation.Cancel") { const op = operations[String(p.operationId)]; if (!op) return fail("NOT_FOUND"); if (op.persistencePending) return fail("NATIVE_UNAVAILABLE", op.id); if (op.kind === "backup-restore") { op.cancelRequested = true; return ok(rollback(op)); } op.state = "cancelled"; op.stage = "cancelled"; op.cancelRequested = true; op.backupReport!.sequence++; return ok(store(op)); }
      if (request.method === "Diagnostics.Preview") return ok(diagnostic);
      if (request.method === "Diagnostics.Export" || request.method === "Diagnostics.EndVerification") {
        if (p.reportId !== diagnostic.reportId || !p.requestId) return fail("VALIDATION_FAILED");
        const id = String(p.requestId), signature = JSON.stringify(p), original = diagnosticRequests.get(id);
        if (original && original !== signature) return fail("REQUEST_ID_REUSED"); diagnosticRequests.set(id, signature);
        return scenario === "diagnostics-unknown" ? request.method === "Diagnostics.EndVerification" ? ok({ status: "unconfirmed", reportId: p.reportId }) : fail("DIAGNOSTICS_RESULT_UNCONFIRMED") : ok({ status: scenario === "diagnostics-cancel" ? "cancelled" : "saved", reportId: p.reportId, ...(scenario === "diagnostics-cancel" ? {} : { sha256: diagnostic.sha256 }) });
      }
      return fail("CAPABILITY_UNSUPPORTED");
    } } } } });
  }, { initial, scenario, diagnostic });
}
