import type { Page } from "@playwright/test";
import type { ApplicationResult, DeviceProfile, EnvironmentPreview, NativeMigrationPreview, NativeRestorePreview, Operation, WorkspaceView } from "../../../src/application/contract";
import type { NativeRequest } from "../../../src/application/wails-adapter";

export type MigrationWorkflowScenario = "baseline" | "direct" | "prepare-unknown" | "prepare-unknown-no-id" | "prepare-refused" | "stop-unconfirmed" | "policy-unavailable" | "policy-mismatch" | "revision-conflict" | "preview-identity" | "needs-reconcile" | "workspace-maintenance" | "rollback-success" | "rollback-failed" | "rollback-protected" | "rollback-unknown" | "rollback-hash-mismatch";
export type MigrationFixtureStage = "trial-running" | "ready" | "committed" | "completed" | "protected" | "original-retained";
export const migrationWorkflowIds = {
  environment: "synthetic-migration-environment", unrelated: "synthetic-unrelated-environment",
  oldKernel: "synthetic-exact-build-148", newKernel: "synthetic-exact-build-150", missingKernel: "synthetic-missing-build",
  proxy: "synthetic-migration-proxy", seed: "180000035", revision: 7,
  userDataRef: "environments/synthetic-migration-environment/user-data", archiveSha256: "d".repeat(64),
};

export interface MigrationWorkflowSnapshot {
  syntheticOnly: true;
  workspace: WorkspaceView;
  calls: NativeRequest[];
  operations: Operation[];
  receipts: { requestId: string; operationId: string; signature: string }[];
  held: { label: string; request: NativeRequest }[];
  faults: string[];
  savedRevision: number;
  sourceActive: boolean;
  restorePreviewActive: boolean;
}
export interface MigrationWorkflowHooks {
  snapshot(): MigrationWorkflowSnapshot;
  hold(method: string, label: string, payload?: Record<string, unknown>): void;
  reply(label: string, response?: ApplicationResult<unknown>): void;
  advance(operationId: string, stage: MigrationFixtureStage): void;
  supersede(operationId: string): string;
  protectWorkspace(value: "restore" | "issue" | "none"): void;
}
declare global { interface Window { __migrationWorkflow: MigrationWorkflowHooks } }

/** Independently authored contract projections. No archived response or native I/O. */
export function migrationWorkflowInput(scenario: MigrationWorkflowScenario = "baseline") {
  const ids = migrationWorkflowIds, time = "2026-10-06T01:00:00Z";
  const capabilities = [{ field: "cpu", status: "configurable" as const, source: "synthetic-not-observed", note: "合成合同字段，未运行内核。" }];
  const kernels = [ids.oldKernel, ids.newKernel, ids.missingKernel].map((id, index) => ({
    id, version: ["148.0.7778.215", "150.0.7871.186", "151.0.9000.11"][index], architecture: "amd64",
    source: { kind: "official" as const, location: "https://example.invalid/synthetic.zip", tag: "synthetic-contract-only", commit: null },
    archiveSha256: String(index + 1).repeat(64), executableSha256: String(index + 4).repeat(64),
    installPath: `kernels/${id}`, executableRelativePath: "chrome.exe", installedAt: time,
    status: index === 2 ? "missing" as const : "verified" as const, usedBy: index ? [] : [ids.environment, ids.unrelated], usedCount: index ? 0 : 2,
    report: { adapterVersion: "synthetic-not-run", version: "synthetic-capabilities", sampledAt: time, transport: "synthetic-not-run", sandbox: true, observations: [], capabilities },
  }));
  const environment = {
    id: ids.environment, code: "1", name: "合成迁移环境 A", group: "合成工作组", note: "仅用于页面合同验证",
    proxyId: scenario === "direct" ? "" : ids.proxy, coreId: ids.oldKernel, seed: ids.seed,
    language: "en-US", timezone: "America/New_York", cpu: "8", width: 1366, height: 768,
    urls: "https://example.invalid/", restoreTabs: true, status: "ready" as const, cookies: [], createdAt: time, fingerprintVersion: "synthetic-generator-v1",
  };
  const profile = (index: number): DeviceProfile => ({
    schemaVersion: 1, configRevision: 3 + index, seed: ids.seed, templateId: "windows", templateVersion: "synthetic-template-v1",
    generatorVersion: "synthetic-generator-v1", platform: "windows", platformVersion: "15.0.0", brand: "Chrome", brandVersion: kernels[index].version,
    kernelId: kernels[index].id, coreActualVersion: kernels[index].version, coreExecutableSha256: kernels[index].executableSha256,
    adapterVersion: "synthetic-not-run", capabilityVersion: "synthetic-capabilities", language: "en-US", acceptLanguages: ["en-US"], uiLanguage: "system",
    timezone: "America/New_York", regionPreset: "US", cpu: "8", width: 1366, height: 768,
    parameters: [`--fingerprint=${ids.seed}`, `--fingerprint-brand-version=${kernels[index].version}`, "--fingerprint-hardware-concurrency=8"], configHash: String(index + 7).repeat(64),
  });
  const before = profile(0), after = profile(1);
  const workspace: WorkspaceView = {
    mode: "native", state: { schemaVersion: 1, environments: [environment, { ...environment, id: ids.unrelated, code: "2", name: "合成未选环境 B", seed: "180000036", proxyId: "" }],
      proxies: [{ id: ids.proxy, name: "合成 SOCKS5", type: "socks5", host: "192.0.2.35", port: 1080, country: "US", username: "", password: "", status: "unchecked" }],
      kernels: kernels.map(k => ({ id: k.id, version: k.version, available: k.status === "verified", source: "fingerprint-chromium", note: "合成精确构建记录，未运行" })), backups: [], activities: [] },
    kernelRecords: kernels, kernelOperations: [], migrationOperations: [], restoreOperations: [], runtimeSessions: {},
    defaultKernel: { kernelId: ids.oldKernel, revision: 2 }, dataReferences: { [ids.environment]: ids.userDataRef, [ids.unrelated]: `environments/${ids.unrelated}/user-data` },
    fingerprints: { [ids.environment]: { profile: before, createdAt: time, action: "synthetic-original" } },
  };
  const environmentPreview: EnvironmentPreview = { previewId: "synthetic-saved-policy", environment, expectedRevision: ids.revision, userDataRef: ids.userDataRef };
  const migrationPreview: NativeMigrationPreview = { mode: "native", previewId: "synthetic-migration-preview-1", environmentId: ids.environment, name: environment.name,
    expectedRevision: ids.revision, before, after, beforeCapabilities: capabilities, afterCapabilities: capabilities,
    changes: [{ field: "kernelId", before: before.kernelId, after: after.kernelId }, { field: "coreActualVersion", before: before.coreActualVersion, after: after.coreActualVersion }], expiresAt: "2030-01-01T00:00:00Z" };
  const restorePreview: NativeRestorePreview = { mode: "native", previewId: "synthetic-upgrade-restore-preview", format: "prism-local-backup", name: "synthetic-before-upgrade.prismbackup",
    archiveSha256: ids.archiveSha256, manifestSha256: "e".repeat(64), scope: "selected", createdAt: time, expiresAt: "2030-01-01T00:00:00Z",
    environmentCount: 1, addCount: 0, overwriteCount: 1, conflictCount: 0, missingKernelCount: 0, credentialReentryCount: 1, bytes: 4096, canRestore: true,
    kernels: [{ id: ids.oldKernel, version: kernels[0].version, archiveSha256: kernels[0].archiveSha256, executableSha256: kernels[0].executableSha256, localId: ids.oldKernel, state: "verified-bytes", required: true }],
    credentials: [{ proxyId: ids.proxy, state: "reentry-required" }] };
  return { scenario, ids, time, workspace, environmentPreview, migrationPreview, restorePreview };
}

/** Main App entry: install before page.goto('/#/kernels'). Node strip-types compatible. */
export async function migrationWorkflowBridge(page: Page, scenario: MigrationWorkflowScenario = "baseline") {
  await page.addInitScript(input => {
    const { scenario, ids, time } = input, view = structuredClone(input.workspace);
    const calls: NativeRequest[] = [], faults: string[] = [], operations = new Map<string, Operation>();
    const receipts = new Map<string, { signature: string; operationId: string }>();
    const previews = new Map<string, NativeMigrationPreview>(), before = structuredClone(view.state.environments[0]);
    const holds: { method: string; label: string; payload: Record<string, unknown> }[] = [];
    const held = new Map<string, { request: NativeRequest; response: ApplicationResult<unknown>; resolve(response: ApplicationResult<unknown>): void }>();
    let serial = 0, uuid = 0, revision = ids.revision, sourceActive = false, restorePreviewActive = false;
    const copy = <T,>(v: T): T => structuredClone(v);
    const ok = (data: unknown): ApplicationResult<unknown> => ({ ok: true, mode: "native", data: copy(data) });
    const fail = (code: string, operationId?: string): ApplicationResult<unknown> => ({ ok: false, mode: "native", error: { code, message: "合成边界响应；未执行任何真实桌面操作。", retryable: true }, ...(operationId ? { operationId } : {}) });
    const terminal = (op: Operation) => !op.persistencePending && ["completed", "failed", "cancelled"].includes(op.state);
    function store(op: Operation) {
      operations.set(op.id, copy(op));
      view.migrationOperations = [...operations.values()].filter(o => o.kind === "migration").map(copy);
      view.restoreOperations = [...operations.values()].filter(o => o.kind === "backup-restore").map(copy);
      view.migrationMaintenance = view.migrationOperations.find(o => !terminal(o));
      view.maintenance = view.restoreOperations.find(o => !terminal(o));
      return op;
    }
    function migration(requestId: string, previewId: string): Operation {
      return { id: `synthetic-migration-task-${++serial}`, kind: "migration", state: "running", stage: "trial-starting", environmentId: ids.environment,
        total: 1, completedIds: [], cancelRequested: false, migrationReport: { mode: "native", requestId, previewId, environmentId: ids.environment,
          oldKernelId: ids.oldKernel, newKernelId: ids.newKernel, seed: ids.seed, sequence: 1, backupVerified: true, archiveSha256: ids.archiveSha256, trialExited: false, committed: false, protected: true } };
    }
    function observation(index: number, normalExit: boolean) {
      return { sampledAt: time, cookie: true, localStorage: true, indexedDB: true, transport: "private-pipe-canary" as const,
        fingerprint: { seed: Number(ids.seed), browserVersion: index ? "150.0.7871.186" : "148.0.7778.215", httpUserAgent: "synthetic-only", userAgent: "synthetic-only",
          cpu: 8, memory: 8, language: "en-US", timezone: "America/New_York", gpuVendor: "synthetic-only", gpuRenderer: "synthetic-only", pid: 0, processCreatedAt: time, normalExit } };
    }
    function advance(id: string, stage: MigrationFixtureStage) {
      const op = copy(operations.get(id));
      if (!op || op.kind !== "migration") throw new Error("Unknown synthetic migration");
      const report = op.migrationReport!; report.sequence++; op.stage = stage; op.error = undefined; op.persistencePending = false;
      if (stage === "trial-running") { op.state = "running"; report.before = observation(0, true); report.after = observation(1, false); }
      if (stage === "ready") { op.state = "running"; report.trialExited = true; report.after = observation(1, true); }
      if (stage === "protected") { op.state = "failed"; op.persistencePending = true; report.protected = true; op.error = { code: "MIGRATION_INCOMPLETE", message: "合成试用全树退出尚未确认，保持保护。", retryable: true }; }
      if (stage === "original-retained") { op.state = "failed"; report.protected = false; report.committed = false; op.error = { code: "MIGRATION_INCOMPLETE", message: "合成失败，原配置已保留。", retryable: true }; }
      if (stage === "committed" || stage === "completed") {
        if (!report.trialExited) throw new Error("Synthetic commit requires confirmed trial exit");
        report.committed = true; op.state = stage === "completed" ? "completed" : "running"; report.protected = stage !== "completed";
        if (stage === "completed") op.completedIds = [ids.environment];
        view.state.environments[0].coreId = ids.newKernel; revision = ids.revision + 1;
        view.fingerprints![ids.environment] = { profile: copy(input.migrationPreview.after), createdAt: time, action: "synthetic-migration" };
      }
      store(op); return op;
    }
    function restoreOperation(requestId: string, previewId: string): Operation {
      return { id: `synthetic-upgrade-restore-task-${++serial}`, kind: "backup-restore", state: "running", stage: "prepared", environmentId: ids.environment,
        total: 1, completedIds: [], cancelRequested: false, restoreReport: { mode: "native", requestId, previewId, archiveSha256: ids.archiveSha256,
          sequence: 1, environmentCount: 1, switchedCount: 0, credentialReentryCount: 1, committed: false, rolledBack: false, protected: true } };
    }
    function protectWorkspace(value: "restore" | "issue" | "none") {
      view.issue = value === "issue" ? { code: "STORAGE_READ_FAILED", message: "合成工作区读取未确认", retryable: true } : undefined;
      if (value === "restore") store(restoreOperation("synthetic-external-restore-request", "synthetic-external-restore-preview"));
      if (value === "none") { for (const [id, op] of operations) if (op.restoreReport?.requestId === "synthetic-external-restore-request") operations.delete(id); view.restoreOperations = [...operations.values()].filter(o => o.kind === "backup-restore"); view.maintenance = undefined; }
    }
    if (scenario.startsWith("rollback-")) { const op = store(migration("synthetic-history-request", "synthetic-history-preview")); advance(op.id, "trial-running"); advance(op.id, "ready"); advance(op.id, "completed"); }
    if (scenario === "workspace-maintenance") protectWorkspace("restore");
    if (scenario === "needs-reconcile") view.runtimeSessions![ids.environment] = { mode: "native", environmentId: ids.environment, sessionId: "synthetic-unconfirmed-session", operationId: "synthetic-session-operation", state: "error",
      revision, fingerprintRevision: 3, kernelId: ids.oldKernel, userDataRef: ids.userDataRef, networkPolicy: "proxy", proxyId: ids.proxy, canControl: false, canForce: false, needsReconcile: true, persistencePending: false };
    Object.defineProperty(crypto, "randomUUID", { value: () => `00000000-0000-4000-8000-${String(++uuid).padStart(12, "0")}` });
    Object.defineProperty(window, "localStorage", { get() { throw new Error("Synthetic native migration must not read demo storage"); } });
    window.__migrationWorkflow = {
      snapshot: () => copy({ syntheticOnly: true, workspace: view, calls, operations: [...operations.values()], receipts: [...receipts.entries()].map(([requestId, r]) => ({ requestId, ...r })), held: [...held.entries()].map(([label, h]) => ({ label, request: h.request })), faults, savedRevision: revision, sourceActive, restorePreviewActive }),
      hold(method, label, payload = {}) { if (!/^synthetic-[a-z0-9-]+$/.test(label) || held.has(label) || holds.some(h => h.label === label)) throw new Error("Use a unique safe synthetic label"); holds.push({ method, label, payload }); },
      reply(label, response) { const h = held.get(label); if (!h) throw new Error("Unknown held synthetic request"); held.delete(label); h.resolve(copy(response ?? h.response)); },
      advance,
      supersede(id) { const old = operations.get(id); if (!old || old.kind !== "migration") throw new Error("Unknown superseded migration"); if (!terminal(old)) { const ended = copy(old); ended.state = "cancelled"; ended.stage = "original-retained"; ended.cancelRequested = true; ended.persistencePending = false; ended.migrationReport!.protected = false; ended.migrationReport!.sequence++; store(ended); } const next = store(migration("synthetic-new-owner-request", "synthetic-new-owner-preview")); advance(next.id, "trial-running"); return next.id; },
      protectWorkspace,
    };
    function dispatch(request: NativeRequest): ApplicationResult<unknown> {
      const p = request.payload as Record<string, unknown>, method = request.method;
      if (request.mode !== "native") return fail("CAPABILITY_UNSUPPORTED");
      if (method === "Workspace.Read") {
        const query = p.environmentQuery as { page?: number; pageSize?: number; search?: string } | undefined;
        const page = query?.page ?? 1, pageSize = query?.pageSize ?? 10;
        const filtered = view.state.environments.filter(e => !query?.search || `${e.name} ${e.id}`.includes(query.search));
        return ok({ ...view, state: { ...view.state, environments: filtered.slice((page - 1) * pageSize, page * pageSize) }, environmentPage: { page, pageSize, total: view.state.environments.length, filteredTotal: filtered.length, groups: ["合成工作组"], runningCount: 0, errorCount: 0 } });
      }
      if (method === "Cookie.DiscardImport" && p.previewId === "") return ok({ status: "discarded" });
      if (method === "Environment.Preview") {
        if (p.kind !== "edit" || p.sourceId !== ids.environment) return fail("NOT_FOUND");
        if (scenario === "policy-unavailable") return fail("STORAGE_READ_FAILED");
        const preview = { ...copy(input.environmentPreview), environment: copy(view.state.environments[0]), expectedRevision: revision };
        if (scenario === "policy-mismatch") preview.environment.id = ids.unrelated;
        return ok(preview);
      }
      if (method === "Preview.Discard" && p.previewId === input.environmentPreview.previewId) return ok({ status: "discarded" });
      if (method === "Migration.Preview") {
        if (p.environmentId !== ids.environment || p.kernelId !== ids.newKernel) return fail("VALIDATION_FAILED");
        if (view.maintenance || view.runtimeSessions?.[ids.environment]?.needsReconcile) return fail("PROFILE_BUSY");
        const preview = copy(input.migrationPreview); preview.previewId = `synthetic-migration-preview-${previews.size + 1}`;
        preview.expectedRevision = scenario === "revision-conflict" ? revision + 1 : revision;
        if (scenario === "preview-identity") preview.after.seed = "180000099";
        previews.set(preview.previewId, preview); return ok(preview);
      }
      if (method === "Migration.Prepare" || method === "Backup.ApplyRestore") {
        if (!p.requestId || typeof p.requestId !== "string") return fail("VALIDATION_FAILED");
        const signature = JSON.stringify(request), prior = receipts.get(p.requestId);
        if (prior) {
          if (prior.signature !== signature) return fail("REQUEST_ID_REUSED");
          let op = copy(operations.get(prior.operationId)!);
          if (method === "Migration.Prepare" && op.stage === "acceptance-pending") { op.stage = "trial-starting"; op.state = "running"; op.persistencePending = false; op.migrationReport!.backupVerified = true; op.migrationReport!.archiveSha256 = ids.archiveSha256; op.migrationReport!.sequence++; store(op); }
          return op.stage === "acceptance-pending" ? fail("NATIVE_UNAVAILABLE", op.id) : ok({ status: "accepted", operation: op });
        }
        if (method === "Migration.Prepare") {
          const preview = previews.get(String(p.previewId));
          if (!preview || p.confirm !== true) return fail("PREVIEW_EXPIRED");
          if (scenario === "prepare-refused") return fail("MIGRATION_NOT_ACCEPTED");
          if (preview.expectedRevision !== revision || view.runtimeSessions?.[ids.environment]?.needsReconcile || view.maintenance || view.migrationMaintenance) return fail("PROFILE_BUSY");
          const op = migration(p.requestId, preview.previewId);
          if (scenario === "prepare-unknown" || scenario === "prepare-unknown-no-id") { op.stage = "acceptance-pending"; op.state = "accepted"; op.persistencePending = true; op.migrationReport!.backupVerified = false; op.migrationReport!.archiveSha256 = ""; }
          store(op); receipts.set(p.requestId, { signature, operationId: op.id });
          return op.stage === "acceptance-pending" ? fail("NATIVE_UNAVAILABLE", scenario === "prepare-unknown" ? op.id : undefined) : ok({ status: "accepted", operation: op });
        }
        if (!restorePreviewActive || p.previewId !== input.restorePreview.previewId || p.archiveSha256 !== ids.archiveSha256 || p.confirmOverwrite !== true || p.acknowledgeCredentials !== true || p.stopRunning !== true) return fail("BACKUP_INVALID");
        const op = restoreOperation(p.requestId, String(p.previewId));
        if (scenario === "rollback-unknown") { op.state = "accepted"; op.stage = "acceptance-pending"; op.persistencePending = true; }
        else if (scenario === "rollback-success") { op.state = "completed"; op.stage = "finalized"; op.completedIds = [ids.environment]; op.restoreReport!.committed = true; op.restoreReport!.protected = false; op.restoreReport!.switchedCount = 1; view.state.environments[0] = copy(before); revision++; view.fingerprints![ids.environment] = { profile: copy(input.migrationPreview.before), createdAt: time, action: "synthetic-full-restore" }; }
        else if (scenario === "rollback-protected") { op.state = "failed"; op.stage = "protected"; op.persistencePending = true; op.error = { code: "RESTORE_INCOMPLETE", message: "合成目录核对未完成，保持保护。", retryable: true }; }
        else { op.state = "failed"; op.stage = "rolled-back"; op.restoreReport!.rolledBack = true; op.restoreReport!.protected = false; op.error = { code: "RESTORE_ROLLED_BACK", message: "合成恢复失败，恢复前配置保留。", retryable: true }; }
        store(op); receipts.set(p.requestId, { signature, operationId: op.id });
        return op.stage === "acceptance-pending" ? fail("NATIVE_UNAVAILABLE", op.id) : ok({ status: "accepted", operation: op });
      }
      if (method === "Migration.Action") {
        const op = operations.get(String(p.operationId)); if (!op || op.kind !== "migration") return fail("NOT_FOUND");
        if (p.confirm !== true || !["stop", "commit", "recover"].includes(String(p.action))) return fail("VALIDATION_FAILED");
        if (view.maintenance || view.issue) return fail("PROFILE_BUSY");
        if (p.action === "stop" && op.stage === "trial-running") return ok(advance(op.id, scenario === "stop-unconfirmed" ? "protected" : "ready"));
        if (p.action === "commit" && op.stage === "ready" && op.migrationReport!.trialExited && !op.persistencePending && !op.cancelRequested) return ok(advance(op.id, "committed"));
        if (p.action === "recover" && op.persistencePending) return ok(advance(op.id, "original-retained"));
        return fail("MIGRATION_INCOMPLETE", op.id);
      }
      if (method === "Operation.Read") return operations.has(String(p.operationId)) ? ok(operations.get(String(p.operationId))) : fail("NOT_FOUND");
      if (method === "Operation.Cancel") {
        const op = copy(operations.get(String(p.operationId))); if (!op) return fail("NOT_FOUND");
        if (op.kind !== "migration" || terminal(op) || op.migrationReport!.committed || op.persistencePending) return fail("MIGRATION_INCOMPLETE", op.id);
        op.state = "cancelled"; op.stage = "original-retained"; op.cancelRequested = true; op.migrationReport!.sequence++; op.migrationReport!.protected = false; return ok(store(op));
      }
      if (method === "Migration.SelectRollback") {
        const op = operations.get(String(p.operationId)); if (!op || op.kind !== "migration" || !terminal(op) || !op.migrationReport!.backupVerified) return fail("MIGRATION_INCOMPLETE");
        sourceActive = true; return ok({ sourceToken: "synthetic-upgrade-source", archiveSha256: scenario === "rollback-hash-mismatch" ? "f".repeat(64) : ids.archiveSha256 });
      }
      if (method === "Backup.PreviewRestore") { if (!sourceActive || p.sourceToken !== "synthetic-upgrade-source") return fail("PREVIEW_EXPIRED"); restorePreviewActive = true; return ok(input.restorePreview); }
      if (method === "Backup.DiscardRestore") { if (p.sourceToken !== "synthetic-upgrade-source" || p.previewId !== "" && p.previewId !== input.restorePreview.previewId) return fail("VALIDATION_FAILED"); sourceActive = false; restorePreviewActive = false; return ok({ status: "discarded" }); }
      faults.push(`Unsupported synthetic method: ${method}`); return fail("CAPABILITY_UNSUPPORTED");
    }
    Object.assign(window, { go: { main: { DesktopApp: { Call: async (request: NativeRequest) => {
      calls.push(copy(request)); const response = dispatch(request);
      const index = holds.findIndex(h => h.method === request.method && Object.entries(h.payload).every(([key, value]) => (request.payload as Record<string, unknown>)[key] === value));
      if (index < 0) return response;
      const [h] = holds.splice(index, 1);
      return new Promise<ApplicationResult<unknown>>(resolve => held.set(h.label, { request: copy(request), response: copy(response), resolve }));
    } } } } });
  }, migrationWorkflowInput(scenario));
}

export type MigrationWorkflowStageEntry = "select" | "diff" | "trial-confirm" | "trial-starting" | "trial-running" | "ready" | "switch-confirm" | "committed" | "completed" | "protected" | "cancelled" | "upgrade-preflight" | "upgrade-confirm" | "upgrade-result-rollback" | "upgrade-result-protected" | "upgrade-result-completed";

/** Reusable synthetic stage entry for MAIN's source-bound final App capture.
 * The caller chooses the real Vite URL/viewport and owns screenshots/metadata.
 * All windows are opened by actual App UI; phase hooks only author RPC results.
 */
export async function migrationWorkflowStageEntry(page: Page, stage: MigrationWorkflowStageEntry, url: string) {
  const scenario: MigrationWorkflowScenario = stage === "upgrade-result-protected" ? "rollback-protected" : stage === "upgrade-result-completed" ? "rollback-success" : stage.startsWith("upgrade-") ? "rollback-failed" : "baseline";
  await migrationWorkflowBridge(page, scenario);
  await page.goto(url);
  await page.getByRole("button", { name: "选定环境迁移", exact: true }).click();
  const selection = page.getByRole("dialog", { name: "选定环境内核迁移", exact: true });
  await selection.getByText("第 1 页 · 共 2 个").waitFor();
  if (stage === "select") return;
  if (stage.startsWith("upgrade-")) {
    await selection.getByText("迁移记录与升级前恢复", { exact: true }).click();
    await selection.getByRole("button", { name: "预检升级前完整恢复", exact: true }).click();
    await selection.getByText("升级前完整恢复预检", { exact: true }).waitFor();
    if (stage === "upgrade-preflight") return;
    await selection.getByRole("button", { name: "确认完整恢复", exact: true }).click();
    const confirm = page.getByRole("dialog", { name: "确认完整恢复", exact: true });
    await confirm.waitFor();
    if (stage === "upgrade-confirm") return;
    for (const checkbox of await confirm.getByRole("checkbox").all()) await checkbox.check();
    await confirm.getByRole("button", { name: "确认并完整恢复", exact: true }).click();
    await page.getByRole("dialog", { name: "完整恢复结果", exact: true }).waitFor();
    return;
  }
  await selection.getByLabel("选定环境", { exact: true }).selectOption(migrationWorkflowIds.environment);
  await selection.getByLabel("目标精确构建", { exact: true }).selectOption(migrationWorkflowIds.newKernel);
  await selection.getByRole("button", { name: "查看迁移差异", exact: true }).click();
  await selection.getByRole("button", { name: "备份并试用副本", exact: true }).waitFor();
  if (stage === "diff") return;
  await selection.getByRole("button", { name: "备份并试用副本", exact: true }).click();
  const trial = page.getByRole("dialog", { name: "确认备份并试用副本", exact: true });
  await trial.waitFor();
  if (stage === "trial-confirm") return;
  await trial.getByRole("checkbox").check(); await trial.getByRole("button", { name: "确认备份并试用", exact: true }).click();
  const result = page.getByRole("dialog", { name: "迁移任务", exact: true });
  await result.getByRole("heading", { name: "启动副本并读取实际参数", exact: true }).waitFor();
  if (stage === "trial-starting") return;
  const id = await page.evaluate(() => window.__migrationWorkflow.snapshot().operations.find(o => o.kind === "migration")!.id);
  await page.evaluate(id => window.__migrationWorkflow.advance(id, "trial-running"), id);
  await result.getByRole("heading", { name: "正在试用新构建副本", exact: true }).waitFor();
  if (stage === "trial-running") return;
  if (stage === "cancelled") { await result.getByRole("button", { name: "取消并保留原状态", exact: true }).click(); await result.getByRole("heading", { name: "原状态已保留", exact: true }).waitFor(); return; }
  if (stage === "protected") { await page.evaluate(id => window.__migrationWorkflow.advance(id, "protected"), id); await result.getByRole("heading", { name: "结果未确认，保持保护", exact: true }).waitFor(); return; }
  await result.getByRole("button", { name: "正常停止试用副本", exact: true }).click();
  await result.getByRole("heading", { name: "副本已停止，等待明确确认", exact: true }).waitFor();
  if (stage === "ready") return;
  await result.getByRole("button", { name: "明确切换", exact: true }).click();
  const confirm = page.getByRole("dialog", { name: "确认切换选定环境", exact: true });
  await confirm.waitFor();
  if (stage === "switch-confirm") return;
  await confirm.getByRole("checkbox").check(); await confirm.getByRole("button", { name: "确认切换", exact: true }).click();
  await result.getByRole("heading", { name: "配置已提交，核对目录", exact: true }).waitFor();
  if (stage === "committed") return;
  await page.evaluate(id => window.__migrationWorkflow.advance(id, "completed"), id);
  await result.getByRole("heading", { name: "迁移已完成", exact: true }).waitFor();
}
