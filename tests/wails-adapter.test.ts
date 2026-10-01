import assert from "node:assert/strict";
import test from "node:test";
import { WailsAdapter, type NativeBridge, type NativeRequest } from "../src/application/wails-adapter.ts";
import { mergeOperation, type ApplicationResult, type Operation, type OperationEvent, type RuntimeSession, type WorkspaceView, type ProxyUpdateRequest, type ProxyTargetRequest, type CookieCommitRequest, type CookieImportPreview, type CookieImportReport, type NativeBatchReport, type NativeBatchPage, type NativeBatchPreviewRequest } from "../src/application/contract.ts";
import type { Environment } from "../src/domain.ts";
import { applyFingerprint, demoFingerprint, demoProfile, fingerprintMatchesConfiguration } from "../src/application/fingerprint-model.ts";
import { proxyResolutionLabel, proxyStageLabel } from "../src/application/proxy-network.ts";
import { currentCookieOperation } from "../src/application/cookie-import.ts";
import { mergeBatchPage, validBatchPage, validBatchReport } from "../src/application/batch-model.ts";
import { readRuntimeStartPlan } from "../src/application/runtime-start-plan.ts";
import { confirmsBackupRequest, validBackupReport } from "../src/application/backup-model.ts";
import type { NativeBackupReport, NativeBackupExportRequest, NativeRestorePreview } from "../src/application/contract.ts";

const empty = (): WorkspaceView => ({ mode: "native", state: { schemaVersion: 1, environments: [], proxies: [], kernels: [], backups: [], activities: [] } });
const ok = <T>(data: T): ApplicationResult<T> => ({ ok: true, mode: "native", data });
const rejected: ApplicationResult<never> = { ok: false, mode: "native", error: { code: "STORAGE_WRITE_FAILED", message: "本次未保存", retryable: true } };
function fixture(handler: (request: NativeRequest) => ApplicationResult<unknown> | Promise<ApplicationResult<unknown>>) {
  const calls: NativeRequest[] = [];
  const bridge: NativeBridge = async <T>(request: NativeRequest) => { calls.push(request); return await handler(request) as ApplicationResult<T>; };
  return { app: new WailsAdapter(bridge), calls };
}
const environment: Environment = {
  id: "synthetic-id", code: "001", name: "合成原生样本", group: "测试", note: "", proxyId: "", coreId: "kernel-pending", seed: "123",
  language: "en-US", timezone: "America/New_York", cpu: "auto", width: 1280, height: 800, urls: "", restoreTabs: true,
  fingerprintVersion: "windows-desktop-v1", status: "ready", cookies: [], createdAt: "2026-09-30T00:00:00Z",
};
const operation: Operation = { id: "synthetic-operation", kind: "create", state: "completed", total: 1, completedIds: [environment.id], cancelRequested: false };

const syntheticBatchReport = (): NativeBatchReport => ({ mode: "native", planId: "synthetic-batch-plan", kind: "create", total: 3, completedCount: 1, failedCount: 0, notExecutedCount: 2, attemptCompletedCount: 1, sharedProxyAssignments: 0, directAssignments: 3, sequence: 4 });
const syntheticBackupReport = (): NativeBackupReport => ({ requestId: "synthetic-backup-request", mode: "native", format: "prism-local-backup", schemaVersion: 1, scope: "selected", environmentCount: 2, copiedEnvironmentCount: 0, fileCount: 0, byteCount: 0, sequence: 1, published: false, name: "synthetic.prismbackup", credentials: "windows-current-user-dpapi", browserData: "sensitive-same-user-not-portable", kernelBinariesIncluded: false });

const restorePreview = (): NativeRestorePreview => ({ mode: "native", previewId: "synthetic-preview", format: "prism-local-backup", name: "synthetic.prismbackup", archiveSha256: "a".repeat(64), manifestSha256: "b".repeat(64), scope: "selected", createdAt: "2026-10-01T00:00:00Z", expiresAt: "2026-10-01T00:30:00Z", environmentCount: 1, addCount: 1, overwriteCount: 0, conflictCount: 0, missingKernelCount: 0, credentialReentryCount: 0, bytes: 100, canRestore: true, kernels: [], credentials: [] });

test("restore preflight never refreshes/flushed workspace and sends only host source token", async () => {
  const { app, calls } = fixture(() => ok(restorePreview()));
  const result = await app.previewRestore("synthetic-source-token");
  assert.equal(result.ok, true);
  assert.deepEqual(calls, [{ mode: "native", method: "Backup.PreviewRestore", payload: { sourceToken: "synthetic-source-token" } }]);
});
test("restore refuses demo previews and inconsistent eligibility without claiming restore success", async () => {
  for (const preview of [{ ...restorePreview(), mode: "demo" }, { ...restorePreview(), archiveSha256: "" }, { ...restorePreview(), conflictCount: 1 }, { ...restorePreview(), overwriteCount: 2 }]) {
    const { app } = fixture(() => ok(preview));
    assert.equal((await app.previewRestore("synthetic-token")).ok, false);
  }
});
test("restore page projects pagination and rejects another preview identity", async () => {
  const { app, calls } = fixture(() => ok({ mode: "native", previewId: "wrong-preview", offset: 0, total: 0, items: [] }));
  const result = await app.readRestorePage({ previewId: "synthetic-preview", offset: 0, pageSize: 25, path: "C:/SYNTHETIC_PRIVATE" } as unknown as Parameters<WailsAdapter["readRestorePage"]>[0]);
  assert.equal(result.ok, false);
  assert.deepEqual(calls[0].payload, { previewId: "synthetic-preview", offset: 0, pageSize: 25 });
});

test("backup export only projects frozen IDs and host token, never paths or Cookie bytes", async () => {
  const { app, calls } = fixture(request => request.method === "Workspace.Read" ? ok(empty()) : rejected);
  await app.exportBackup({ scope: "selected", environmentIds: ["selected-A", "selected-B"], destinationToken: "synthetic-host-token", requestId: "synthetic-backup-request", stopRunning: true, path: "C:/SYNTHETIC_PRIVATE", cookies: [{ value: "SYNTHETIC_BACKUP_SECRET" }] } as unknown as NativeBackupExportRequest);
  const payload = calls.find(request => request.method === "Backup.Export")?.payload;
  assert.deepEqual(payload, { scope: "selected", environmentIds: ["selected-A", "selected-B"], destinationToken: "synthetic-host-token", requestId: "synthetic-backup-request", stopRunning: true });
  assert.equal(JSON.stringify(payload).includes("SYNTHETIC_BACKUP_SECRET"), false);
});

test("backup operation cannot declare completed without confirmed publication and exact hashes", async () => {
  const report = syntheticBackupReport();
  assert.equal(validBackupReport({ ...report, published: true }), false);
  assert.equal(validBackupReport({ ...report, name: "C:/synthetic.prismbackup" }), false);
  const { app } = fixture(() => ok({ ...operation, id: "synthetic-backup", kind: "backup-export", total: 2, backupReport: report }));
  assert.equal((await app.getOperation("synthetic-backup")).ok, false);
});

test("backup refresh and late cancellation keep monotonic progress and original published result", async () => {
  const report = { ...syntheticBackupReport(), sequence: 9, copiedEnvironmentCount: 2, published: true, archiveSha256: "a".repeat(64), manifestSha256: "b".repeat(64) };
  const completed: Operation = { ...operation, id: "synthetic-backup", kind: "backup-export", total: 2, completedIds: [], backupReport: report };
  const stale: Operation = { ...completed, state: "running", backupReport: { ...syntheticBackupReport(), sequence: 5 } };
  const { app } = fixture(request => request.method === "Workspace.Read" ? ok({ ...empty(), backupOperations: [completed] }) : ok(stale));
  await app.refresh();
  const response = await app.cancelOperation(completed.id);
  assert.equal(response.ok && response.data.state, "completed");
  assert.equal(response.ok && response.data.backupReport?.published, true);
  assert.equal(mergeOperation(completed, stale), completed);
});

test("uncertain backup request survives unrelated history reads and exposes immutable retry input", async () => {
  const request: NativeBackupExportRequest = { scope: "selected", environmentIds: ["selected-A"], destinationToken: "synthetic-token", stopRunning: true, requestId: "synthetic-original-request" };
  const other: Operation = { ...operation, id: "synthetic-history", kind: "backup-export", state: "failed", total: 2, backupReport: syntheticBackupReport() };
  const { app, calls } = fixture(call => call.method === "Workspace.Read" ? ok(empty()) : call.method === "Operation.Read" ? ok(other) : rejected);
  await app.exportBackup(request);
  const recovered = app.getPendingBackupExport();
  assert.deepEqual(recovered?.request, request);
  recovered?.request.environmentIds.push("mutated-ui-copy");
  assert.deepEqual(app.getPendingBackupExport()?.request.environmentIds, ["selected-A"]);
  await app.getOperation(other.id);
  assert.equal(app.getPendingBackupExport()?.request.requestId, request.requestId);
  const before = calls.filter(call => call.method === "Backup.Export").length;
  assert.equal((await app.exportBackup({ ...request, requestId: "new-request" })).ok, false);
  assert.equal(calls.filter(call => call.method === "Backup.Export").length, before);
});

test("backup accepted-but-unverified response retains original request and known operation ID", async () => {
  const request: NativeBackupExportRequest = { scope: "selected", environmentIds: ["selected-A"], destinationToken: "synthetic-token", stopRunning: true, requestId: "synthetic-backup-request" };
  const invalid: Operation = { ...operation, id: "synthetic-known-backup", kind: "backup-export", state: "accepted", total: 2, backupReport: { ...syntheticBackupReport(), format: "prism-prototype" } as unknown as NativeBackupReport };
  const { app } = fixture(call => call.method === "Workspace.Read" ? ok(empty()) : ok({ status: "accepted", operation: invalid }));
  const result = await app.exportBackup(request);
  assert.equal(result.ok, false);
  assert.equal(result.operationId, invalid.id);
  assert.equal(app.getPendingBackupExport()?.operationId, invalid.id);
  assert.equal(app.getPendingBackupExport()?.request.requestId, request.requestId);
});

test("backup refresh can recover actual original acceptance after a lost IPC response", async () => {
  const request: NativeBackupExportRequest = { scope: "selected", environmentIds: ["selected-A"], destinationToken: "synthetic-token", stopRunning: true, requestId: "synthetic-backup-request" };
  const actual: Operation = { ...operation, id: "synthetic-confirmed-backup", kind: "backup-export", state: "running", total: 2, backupReport: syntheticBackupReport() };
  const { app } = fixture(call => call.method === "Workspace.Read" ? ok({ ...empty(), backupOperations: [actual] }) : rejected);
  const result = await app.exportBackup(request);
  assert.equal(result.ok && result.data.operation.id, actual.id);
  assert.equal(app.getPendingBackupExport(), undefined);
});
test("late accepted and refused replies for an older export never edit the newer pending request", async () => {
  const a: NativeBackupExportRequest = { scope: "selected", environmentIds: ["A1", "A2"], destinationToken: "token-A", stopRunning: true, requestId: "request-A" };
  const b: NativeBackupExportRequest = { ...a, environmentIds: ["B1", "B2"], destinationToken: "token-B", requestId: "request-B" };
  const accepted: Operation = { ...operation, id: "operation-A", kind: "backup-export", state: "accepted", total: 2, backupReport: { ...syntheticBackupReport(), requestId: a.requestId } };
  const replies: ((response: ApplicationResult<unknown>) => void)[] = [];
  const { app } = fixture(call => call.method === "Backup.Export" ? new Promise(resolve => replies.push(resolve)) : call.method === "Operation.Read" ? ok(accepted) : ok(empty()));
  const old = app.exportBackup(a), retry = app.exportBackup(a);
  await app.getOperation(accepted.id);
  const next = app.exportBackup(b);
  replies[0](ok({ status: "accepted", operation: accepted })); await old;
  assert.deepEqual(app.getPendingBackupExport()?.request, b);
  assert.equal(app.getPendingBackupExport()?.operationId, undefined);
  replies[1]({ ok: false, mode: "native", error: { code: "PROFILE_BUSY", message: "合成旧拒绝", retryable: true } }); await retry;
  assert.deepEqual(app.getPendingBackupExport()?.request, b);
  replies[2](rejected); await next;
  assert.equal(app.getPendingBackupExport()?.request.requestId, b.requestId);
});

test("a mode-invalid export is unknown, and a native refresh can still confirm its actual acceptance", async () => {
  const request: NativeBackupExportRequest = { scope: "selected", environmentIds: ["A1", "A2"], destinationToken: "synthetic-token", stopRunning: true, requestId: "synthetic-backup-request" };
  const actual: Operation = { ...operation, id: "operation-native", kind: "backup-export", state: "running", total: 2, backupReport: syntheticBackupReport() };
  let known = false;
  const { app } = fixture(call => call.method === "Workspace.Read" ? ok(known ? { ...empty(), backupOperations: [actual] } : empty()) : { ok: true, mode: "demo", data: { status: "accepted", operation: actual } });
  const unknown = await app.exportBackup(request);
  assert.ok(!unknown.ok && unknown.error.code === "BACKUP_RESULT_UNCONFIRMED");
  assert.deepEqual(app.getPendingBackupExport()?.request, request);
  known = true;
  const confirmed = await app.exportBackup(request);
  assert.equal(confirmed.ok && confirmed.data.operation.id, actual.id);
  assert.equal(app.getPendingBackupExport(), undefined);
});

test("only an exact verified backup request observation consumes its old output authorization", () => {
  const request: NativeBackupExportRequest = { scope: "selected", environmentIds: ["A1", "A2"], destinationToken: "synthetic-token", stopRunning: true, requestId: "synthetic-backup-request" };
  const observed: Operation = { ...operation, id: "operation-native", kind: "backup-export", state: "running", total: 2, backupReport: syntheticBackupReport() };
  assert.equal(confirmsBackupRequest(observed, request), true);
  assert.equal(confirmsBackupRequest({ ...observed, persistencePending: true }, request), false);
  assert.equal(confirmsBackupRequest({ ...observed, backupReport: { ...syntheticBackupReport(), requestId: "unrelated-history" } }, request), false);
  assert.equal(confirmsBackupRequest(undefined, request), false);
});
test("native batch projections exclude identity/credentials and keep an explicit full mapping", async () => {
  const { app, calls } = fixture(request => request.method === "Workspace.Read" ? ok(empty()) : rejected);
  await app.previewBatch({ kind: "create", create: { previewId: "synthetic-draft", count: 1000003, requestId: "synthetic-preview-request", configuration: { ...environment, cookies: [{ name: "synthetic-login", value: "SYNTHETIC_BATCH_SECRET" }], parameters: ["--foreign"], userDataRef: "foreign/profile" } } } as NativeBatchPreviewRequest);
  await app.previewBatch({ kind: "assign", mappings: [{ environmentId: "selected-A", proxyId: "proxy-B", username: "SYNTHETIC_BATCH_SECRET" }, { environmentId: "selected-B", proxyId: "", password: "SYNTHETIC_BATCH_SECRET" }], autoRoundRobin: true } as unknown as NativeBatchPreviewRequest);
  const payload = calls.filter(request => request.method === "Batch.Preview");
  assert.equal(JSON.stringify(payload).includes("SYNTHETIC_BATCH_SECRET"), false);
  assert.deepEqual(payload[1].payload, { kind: "assign", mappings: [{ environmentId: "selected-A", proxyId: "proxy-B" }, { environmentId: "selected-B", proxyId: "" }] });
  assert.equal((payload[0].payload as { create: { count: number } }).create.count, 1000003);
});
test("batch counts/modes are checked and delayed progress/pages never regress another attempt", async () => {
  const report = syntheticBatchReport(); assert.equal(validBatchReport(report), true);
  assert.equal(validBatchReport({ ...report, completedCount: 5 }), false);
  assert.equal(validBatchReport({ ...report, total: Number.MAX_SAFE_INTEGER + 1 }), false);
  const current: Operation = { ...operation, id: "new-batch-attempt", kind: "batch-create", total: report.total, completedIds: [], state: "running", batchReport: report };
  const old: Operation = { ...current, id: "old-batch-attempt", state: "failed", batchReport: { ...report, sequence: 2 } };
  assert.equal(mergeOperation(current, old), current);
  const page: NativeBatchPage = { ...report, expiresAt: "2026-10-01T02:00:00Z", offset: 0, pageSize: 25, items: [] };
  assert.equal(mergeBatchPage(page, { ...page, sequence: 2 }), page);
  const { app } = fixture(request => request.method === "Operation.Read" ? ok({ ...current, batchReport: { ...report, completedCount: 5 } }) : ok({ ...empty(), batchOperations: [{ ...current, batchReport: { ...report, mode: "demo" } }] }));
  assert.equal((await app.getOperation(current.id)).ok, false); assert.equal((await app.refresh()).ok, false);
});
test("server-side environment query persists across refreshes without carrying client records", async () => {
  const { app, calls } = fixture(() => ok(empty()));
  await app.queryEnvironments({ page: 7, pageSize: 8, search: "合成", group: "测试", status: "ready", environments: [environment], paths: ["foreign/profile"] } as Parameters<WailsAdapter["queryEnvironments"]>[0]);
  await app.refresh();
  for (const request of calls) assert.deepEqual(request.payload, { environmentQuery: { page: 7, pageSize: 8, search: "合成", group: "测试", status: "ready" } });
});

test("batch page navigation keeps the requested offset and immutable historical attempt", async () => {
  const report = syntheticBatchReport();
  const page: NativeBatchPage = { ...report, total: 60, notExecutedCount: 59, expiresAt: "2026-10-01T02:00:00Z", offset: 0, pageSize: 25, items: [], operationId: "synthetic-attempt-A", sequence: 7 };
  const next = { ...page, offset: 25, sequence: 6 };
  assert.equal(mergeBatchPage(page, next), next);
  const historical = { ...page, operationId: "synthetic-old-attempt", history: true, sequence: 2 };
  assert.equal(mergeBatchPage(page, historical), historical);
  assert.equal(validBatchPage(page), false, "an empty payload cannot stand in for a full result page");
  const { app, calls } = fixture(() => ok(page));
  assert.equal((await app.readBatchPage({ planId: page.planId, operationId: "synthetic-attempt-B", offset: 0, pageSize: 25 })).ok, false);
  assert.deepEqual(calls[0].payload, { planId: page.planId, operationId: "synthetic-attempt-B", offset: 0, pageSize: 25 });
});

test("batch acceptance returns merged refresh progress instead of a stale accepted snapshot", async () => {
  const report = syntheticBatchReport();
  const accepted: Operation = { ...operation, id: "synthetic-batch-attempt", kind: "batch-create", total: report.total, state: "accepted", completedIds: [], batchReport: { ...report, sequence: 3 } };
  const running: Operation = { ...accepted, state: "running", batchReport: report };
  const { app } = fixture(request => request.method === "Workspace.Read" ? ok({ ...empty(), batchOperations: [running] }) : ok({ status: "accepted", operation: accepted }));
  const response = await app.commitBatch({ planId: report.planId, requestId: "synthetic-acceptance" });
  assert.equal(response.ok && response.data.operation.state, "running");
  assert.equal(response.ok && response.data.operation.batchReport?.sequence, 4);
});

test("cross-page runtime policies are read by exact IDs and missing selections never become direct", async () => {
  const { app, calls } = fixture(request => request.method === "Preview.Discard" ? ok({ status: "discarded" }) : request.method === "Environment.Preview" ? ok({ previewId: "synthetic-start-preview", environment: { ...environment, id: (request.payload as { sourceId: string }).sourceId, proxyId: "synthetic-hidden-proxy" }, expectedRevision: 9 }) : request.method === "Runtime.Start" ? rejected : ok(empty()));
  const plan = await readRuntimeStartPlan(app, ["hidden-selected-A", "hidden-selected-B"]);
  assert.equal(plan.ok && plan.data.length, 2);
  assert.equal(plan.ok && plan.data.every(item => item.networkPolicy === "proxy" && item.expectedRevision === 9), true);
  assert.equal(calls.filter(request => request.method === "Preview.Discard").length, 2);
  assert.equal(calls.some(request => request.method === "Runtime.Start"), false);
  const unavailable = fixture(() => rejected).app;
  assert.equal((await readRuntimeStartPlan(unavailable, ["missing-selected"])).ok, false);
  const foreign = fixture(() => ok({ previewId: "synthetic-wrong-target", environment: { ...environment, id: "other-unselected" }, expectedRevision: 9 })).app;
  assert.equal((await readRuntimeStartPlan(foreign, ["hidden-selected-A"])).ok, false);
  await app.startRuntime({ environmentId: "hidden-selected-A", networkPolicy: "proxy", expectedRevision: 9, requestId: "synthetic-start" });
  assert.deepEqual(calls.find(request => request.method === "Runtime.Start")?.payload, { environmentId: "hidden-selected-A", networkPolicy: "proxy", expectedRevision: 9, requestId: "synthetic-start" });
});

test("native Cookie commit and explicit blank launch project only safe target metadata", async () => {
  const { app, calls } = fixture(request => request.method === "Workspace.Read" ? ok(empty()) : rejected);
  await app.commitCookieImport({ previewId: "synthetic-preview", environmentId: environment.id, expectedRevision: 3, sessionId: "synthetic-session", selectedRows: [1], policy: "merge", requestId: "synthetic-cookie-write", value: "SYNTHETIC_PRIVATE_COOKIE_VALUE", cdpEndpoint: "http://127.0.0.1:9999", cookies: [], networkPolicy: "direct" } as CookieCommitRequest);
  await app.startRuntime({ environmentId: environment.id, expectedRevision: 3, purpose: "cookie-import", networkPolicy: "proxy", requestId: "synthetic-blank-start", allowUnsafeProxy: true, urls: "https://other.test", restoreTabs: true } as Parameters<WailsAdapter["startRuntime"]>[0]);
  assert.deepEqual(calls.find(call => call.method === "Cookie.CommitImport")?.payload, { previewId: "synthetic-preview", environmentId: environment.id, expectedRevision: 3, sessionId: "synthetic-session", selectedRows: [1], policy: "merge", requestId: "synthetic-cookie-write" });
  assert.deepEqual(calls.find(call => call.method === "Runtime.Start")?.payload, { environmentId: environment.id, expectedRevision: 3, purpose: "cookie-import", networkPolicy: "proxy", requestId: "synthetic-blank-start" });
  assert.ok(!JSON.stringify(calls).includes("SYNTHETIC_PRIVATE_COOKIE_VALUE")); assert.ok(!JSON.stringify(calls).includes("allowUnsafeProxy"));
});

test("Cookie previews and readback reports reject demo or foreign environment/session", async () => {
  const preview: CookieImportPreview = { mode: "native", previewId: "synthetic-preview", environmentId: "foreign-environment", environmentName: "合成另一个环境", expectedRevision: 1, requiresStart: true, format: "json", expiresAt: "2026-10-01T01:00:00Z", rows: [], total: 0, validCount: 0, errorCount: 0, expiredCount: 0, conflictCount: 0 };
  const report: CookieImportReport = { mode: "native", previewId: "synthetic-preview", environmentId: environment.id, sessionId: "foreign-session", revision: 1, policy: "merge", clearState: "not-requested", verifiedCount: 1, writtenCount: 1, alreadyMatchedCount: 0, failedCount: 0, skippedCount: 0, unconfirmedCount: 0, items: [] };
  const cookieOperation: Operation = { ...operation, kind: "cookie-import", environmentId: environment.id, sessionId: "synthetic-session", cookieReport: report };
  const { app } = fixture(request => request.method === "Cookie.ParseImport" ? ok(preview) : request.method === "Operation.Read" ? ok(cookieOperation) : ok({ ...empty(), cookieOperations: [cookieOperation] }));
  assert.equal((await app.parseCookieImport(environment.id, "[]")).ok, false);
  assert.equal((await app.getOperation(cookieOperation.id)).ok, false);
  assert.equal((await app.refresh()).ok, false);
});

test("reopened Cookie views adopt later accepted tasks while keeping in-flight observations across stale snapshots", () => {
  const older: Operation = { ...operation, id: "synthetic-old-cookie", kind: "cookie-import", environmentId: environment.id };
  const active: Operation = { ...older, id: "synthetic-new-cookie", state: "accepted", completedIds: [] };
  const foreign: Operation = { ...active, id: "synthetic-foreign-cookie", environmentId: "other-environment" };
  assert.equal(currentCookieOperation(undefined, [foreign, active, older], environment.id)?.id, active.id);
  assert.equal(currentCookieOperation(older, [active, older], environment.id)?.id, active.id);
  assert.equal(currentCookieOperation(active, [older], environment.id)?.id, active.id);
  assert.equal(currentCookieOperation(older, [{ ...active, state: "completed" }, older], environment.id)?.id, active.id);
  assert.equal(currentCookieOperation({ ...active, persistencePending: true, state: "failed" }, [active, older], environment.id)?.persistencePending, true);
  assert.equal(currentCookieOperation(undefined, [older], environment.id, "synthetic-new-preview"), undefined);
  assert.equal(currentCookieOperation(older, [older], environment.id, "synthetic-new-preview"), undefined);
  assert.equal(currentCookieOperation(undefined, [active, older], environment.id, "synthetic-new-preview")?.id, active.id);
});

test("SOCKS5 library checks and runtime starts cannot send local-DNS/auth-downgrade overrides", async () => {
  const { app, calls } = fixture(request => request.method === "Workspace.Read" ? ok(empty()) : rejected);
  await app.checkProxy({ proxyId: "synthetic-socks", expectedRevision: 2, requestId: "synthetic-check", resolutionPolicy: "local", resolveLocally: true, fallback: "no-auth" } as ProxyTargetRequest);
  await app.startRuntime({ environmentId: "synthetic-environment", requestId: "synthetic-start", networkPolicy: "proxy", resolutionPolicy: "local", fallback: "DIRECT" } as Parameters<WailsAdapter["startRuntime"]>[0]);
  assert.deepEqual(calls.find(call => call.method === "Proxy.Check")?.payload, { proxyId: "synthetic-socks", expectedRevision: 2, requestId: "synthetic-check" });
  assert.deepEqual(calls.find(call => call.method === "Runtime.Start")?.payload, { environmentId: "synthetic-environment", requestId: "synthetic-start", networkPolicy: "proxy" });
  assert.ok(!JSON.stringify(calls).includes("resolveLocally")); assert.ok(!JSON.stringify(calls).includes("fallback"));
});

test("SOCKS5 safe reports keep fixed remote-DNS policy and share stage vocabulary", async () => {
  const report = { mode: "native" as const, adapterVersion: "synthetic-host-only", proxyId: "synthetic-socks", revision: 2, channelId: "synthetic-check-channel", resolutionPolicy: "remote-target-dns" as const, startedAt: "2026-10-01T00:00:00Z", finishedAt: "2026-10-01T00:00:01Z", durationMs: 1000, targetOrigin: "https://synthetic.invalid", steps: [{ stage: "socks-negotiation", status: "passed", time: "2026-10-01T00:00:00Z", message: "仅合成报告，不是网络验收。" }], exitIp: "203.0.113.101" };
  const workspace = { ...empty(), nativeProxyRecords: [{ id: report.proxyId, name: "合成SOCKS5", type: "socks5" as const, host: "127.0.0.1", port: 1080, country: "", revision: 2, hasAuthentication: false, status: "connected" as const, usedBy: [], checkReport: report }] };
  const { app } = fixture(() => ok(workspace)); assert.ok((await app.refresh()).ok);
  assert.equal(app.getSnapshot().nativeProxyRecords?.[0].checkReport?.resolutionPolicy, "remote-target-dns");
  assert.equal(proxyStageLabel("socks-negotiation"), "SOCKS5方法协商"); assert.equal(proxyStageLabel("ipv6-target"), "IPv6目标连接"); assert.equal(proxyStageLabel("future-stage"), "future-stage");
  assert.match(proxyResolutionLabel(report.resolutionPolicy), /上游解析/); assert.equal(proxyResolutionLabel(undefined), "未记录目标解析策略");
});

test("network fault observations stay separate from historic preflight and cannot be request overrides", async () => {
  const session: RuntimeSession = { mode: "native", environmentId: "synthetic-environment", sessionId: "synthetic-session", operationId: "synthetic-operation", state: "error", revision: 1, fingerprintRevision: 1, kernelId: "synthetic-kernel", userDataRef: "synthetic-ref", networkPolicy: "proxy", proxyId: "synthetic-proxy", proxyRevision: 1, proxyChannelId: "synthetic-channel", pid: 0, canControl: false, canForce: false, needsReconcile: false, persistencePending: false, networkFault: { state: "network_error", error: { code: "NETWORK_PROTECTION_UNAVAILABLE", message: "合成门禁缺失，不是已安装隔离组件。", retryable: false }, observedAt: "2026-10-01T00:00:00Z", containment: "stopped" } };
  const { app, calls } = fixture(request => request.method === "Workspace.Read" ? ok({ ...empty(), runtimeSessions: { [session.environmentId]: session } }) : rejected);
  assert.ok((await app.refresh()).ok); assert.equal(app.getSnapshot().runtimeSessions?.[session.environmentId].networkFault?.state, "network_error"); assert.equal(app.getSnapshot().runtimeSessions?.[session.environmentId].proxyReport, undefined);
  await app.startRuntime({ environmentId: session.environmentId, requestId: "synthetic-bypass-attempt", networkPolicy: "proxy", networkProtectionReady: true, networkFault: undefined, allowUnsafeProxy: true } as Parameters<WailsAdapter["startRuntime"]>[0]);
  assert.deepEqual(calls.find(call => call.method === "Runtime.Start")?.payload, { environmentId: session.environmentId, requestId: "synthetic-bypass-attempt", networkPolicy: "proxy" });
});

test("native starts empty/loading, has no demo compatibility and only publishes native reads", async () => {
  const workspace = empty();
  const { app, calls } = fixture(() => ok(workspace));
  assert.equal(app.mode, "native");
  assert.equal("compatibility" in app, false);
  assert.deepEqual(app.getSnapshot().state.environments, []);
  assert.equal(app.getSnapshot().issue?.code, "WORKSPACE_LOADING");
  let changes = 0; const unsubscribe = app.subscribe(() => changes++);
  assert.equal((await app.refresh()).ok, true);
  assert.equal(changes, 1); assert.equal(app.getSnapshot(), workspace);
  assert.deepEqual(calls[0], { mode: "native", method: "Workspace.Read", payload: {} });
  unsubscribe(); await app.refresh(); assert.equal(changes, 1);
});

test("mutations send only configuration fields, never client identity, cookies, state or paths", async () => {
  const { app, calls } = fixture(r => r.method === "Workspace.Read" ? ok(empty()) : r.method === "Environment.Create" ? ok({ status: "accepted", operation }) : ok({ status: "completed", environment: { record: environment, revision: 2 } }));
  const untrusted = { ...environment, cookies: [{ name: "do-not-send", value: "SYNTHETIC_SECRET", domain: "example.test" }], userDataDir: "untrusted-path" };
  await app.createBatch({ previewId: "p", configuration: untrusted, count: 1, requestId: "r" });
  await app.updateEnvironment({ previewId: "p2", configuration: untrusted, expectedRevision: 1, requestId: "r2" });
  const mutations = calls.filter(r => r.method.startsWith("Environment."));
  assert.equal(mutations.length, 2);
  for (const request of mutations) {
    assert.equal(request.mode, "native");
    const config = (request.payload as { configuration: Record<string, unknown> }).configuration;
    assert.equal(config.seed, environment.seed);
    for (const field of ["id", "code", "status", "cookies", "createdAt", "userDataDir"]) assert.equal(field in config, false);
    assert.equal(JSON.stringify(request).includes("SYNTHETIC_SECRET"), false);
  }
});

test("failed writes leave old snapshot intact and publish no completion event", async () => {
  const { app, calls } = fixture(r => r.method === "Workspace.Read" ? ok(empty()) : rejected);
  await app.refresh(); const before = app.getSnapshot(); const events: OperationEvent[] = [];
  app.subscribeEvents(e => events.push(e));
  const result = await app.createBatch({ previewId: "p", configuration: environment, count: 1, requestId: "r" });
  assert.equal(result, rejected); assert.equal(app.getSnapshot(), before); assert.deepEqual(events, []);
  assert.equal(calls.filter(r => r.method === "Workspace.Read").length, 1);
});

test("committed creation refreshes the service view before a native terminal event", async () => {
  const workspace = empty(); workspace.state.environments = [environment];
  const { app } = fixture(r => r.method === "Workspace.Read" ? ok(workspace) : ok({ status: "accepted", operation }));
  const events: OperationEvent[] = [];
  const unsubscribe = app.subscribeEvents(e => { assert.equal(app.getSnapshot(), workspace); events.push(e); });
  const request = { previewId: "p", configuration: environment, count: 1, requestId: "r" };
  await app.createBatch(request); await app.createBatch(request);
  assert.equal(events.length, 2); assert.equal(events[0].mode, "native"); assert.equal(events[0].type, "OperationCompleted");
  assert.equal(events[0].operationId, operation.id); assert.ok(events[1].sequence > events[0].sequence);
  unsubscribe(); await app.createBatch(request); assert.equal(events.length, 2);
});

test("demo top-level and nested workspace responses are rejected without fallback", async () => {
  for (const response of [{ ok: true, mode: "demo", data: empty() }, ok({ ...empty(), mode: "demo" })]) {
    const { app } = fixture(() => response as ApplicationResult<unknown>);
    const result = await app.refresh(); assert.equal(result.ok, false);
    assert.equal(app.getSnapshot().mode, "native"); assert.deepEqual(app.getSnapshot().state.environments, []);
    assert.equal(app.getSnapshot().issue?.code, "CAPABILITY_UNSUPPORTED");
  }
});

test("bridge errors are safe, remain native and can recover with an explicit reread", async () => {
  let unavailable = true;
  const { app } = fixture(() => { if (unavailable) throw new Error("SYNTHETIC_SECRET / private-path"); return ok(empty()); });
  const result = await app.refresh(); assert.equal(result.ok, false);
  assert.equal(app.getSnapshot().issue?.code, "NATIVE_UNAVAILABLE");
  assert.equal(JSON.stringify(result).includes("SYNTHETIC_SECRET"), false);
  assert.equal(app.getSnapshot().mode, "native");
  unavailable = false; assert.equal((await app.refresh()).ok, true); assert.equal(app.getSnapshot().issue, undefined);
});

test("a delayed older read cannot overwrite a newer snapshot", async () => {
  const replies: ((value: ApplicationResult<unknown>) => void)[] = [];
  const { app } = fixture(() => new Promise(resolve => replies.push(resolve)));
  const older = app.refresh(); const newer = app.refresh();
  const latest = empty(); latest.state.environments = [environment];
  replies[1](ok(latest)); await newer;
  replies[0](ok(empty())); await older;
  assert.equal(app.getSnapshot(), latest);
});

test("a committed mutation remains confirmed when the following read fails, with a visible read issue", async () => {
  const { app } = fixture(r => r.method === "Workspace.Read" ? { ok: false, mode: "native", error: { code: "STORAGE_READ_FAILED", message: "重新读取失败", retryable: true } } : ok({ status: "completed", environment: { record: environment, revision: 2 } }));
  const result = await app.updateEnvironment({ previewId: "p", configuration: environment, expectedRevision: 1, requestId: "r" });
  assert.equal(result.ok, true); assert.equal(app.getSnapshot().issue?.code, "STORAGE_READ_FAILED");
});

test("kernel acceptance is not completion, progress uses the real operation and path fields are excluded", async () => {
  let state: Operation["state"] = "accepted";
  const task: Operation = { id: "synthetic-kernel-op", kind: "kernel-install", state, total: 1, completedIds: [], cancelRequested: false };
  const { app, calls } = fixture(request => request.method === "Operation.Read" ? ok({ ...task, state }) : request.method === "Workspace.Read" ? ok(empty()) : ok({ status: "accepted", operation: task }));
  const events: OperationEvent[] = []; app.subscribeEvents(event => events.push(event));
  const request = { source: "local" as const, version: "148.0.7778.215", expectedChecksum: "a".repeat(64), archiveToken: "dialog-token", trusted: true, requestId: "synthetic-request", executablePath: "SYNTHETIC_PRIVATE_PATH", args: ["--no-sandbox"] };
  await app.installKernel(request);
  assert.equal(events.length, 0); assert.equal(calls.length, 1);
  assert.equal(JSON.stringify(calls[0]).includes("SYNTHETIC_PRIVATE_PATH"), false);
  assert.equal(JSON.stringify(calls[0]).includes("--no-sandbox"), false);
  await app.getOperation(task.id); assert.equal(events[0].type, "OperationProgress");
  state = "completed"; await app.getOperation(task.id); assert.equal(events[1].type, "OperationCompleted");
  assert.ok(events[1].sequence > events[0].sequence);
});

test("kernel selection, verification and deletion only send picker tokens or exact IDs", async () => {
  const { app, calls } = fixture(() => rejected);
  assert.equal((await app.selectKernelArchive()).ok, false);
  assert.equal((await app.verifyKernel("exact-kernel-id", "verify-request")).ok, false);
  assert.equal((await app.deleteKernel("exact-kernel-id", "delete-request")).ok, false);
  assert.deepEqual(calls.map(request => request.method), ["Kernel.SelectArchive", "Kernel.Verify", "Kernel.Delete"]);
  assert.deepEqual(calls[1].payload, { kernelId: "exact-kernel-id", requestId: "verify-request" });
});

test("a late running cancel response cannot regress a confirmed kernel terminal state", async () => {
  const task: Operation = { id: "synthetic-cancel-race", kind: "kernel-verify", state: "running", total: 1, completedIds: [], cancelRequested: false };
  let finishCancel: ((result: ApplicationResult<Operation>) => void) | undefined;
  const { app } = fixture(request => request.method === "Operation.Cancel" ? new Promise(resolve => { finishCancel = resolve; }) : ok({ ...task, state: "cancelled", cancelRequested: true }));
  const events: OperationEvent[] = []; app.subscribeEvents(event => events.push(event));
  const pending = app.cancelOperation(task.id);
  await app.getOperation(task.id);
  finishCancel!(ok({ ...task, cancelRequested: true }));
  const late = await pending;
  assert.ok(late.ok && late.data.state === "cancelled");
  assert.equal(events.length, 1); assert.equal(events[0].type, "OperationCompleted");
});

test("fingerprint calls whitelist preferences and IDs, not client hardware, paths or commands", async () => {
  const { app, calls } = fixture(() => rejected);
  const untrusted = { ...environment, language: "de-DE", timezone: "Europe/Berlin", cpu: "8", gpuVendor: "SYNTHETIC_GPU", arguments: ["--no-sandbox"], userDataDir: "SYNTHETIC_PATH" };
  await app.generateFingerprint({ previewId: "preview", kernelId: "exact-kernel", templateId: "windows-desktop-v1", overrides: untrusted, regenerate: true });
  await app.previewFingerprintRestore("preview", 1);
  await app.listFingerprintRevisions(environment.id);
  await app.commitFingerprintRevision({ previewId: "preview", environmentId: environment.id, profileHash: "canonical-hash", configuration: untrusted, expectedRevision: 2, requestId: "fingerprint-request" });
  assert.deepEqual(calls.map(call => call.method), ["Fingerprint.Generate", "Fingerprint.PreviewRestore", "Fingerprint.ListRevisions", "Fingerprint.CommitRevision"]);
  assert.deepEqual((calls[0].payload as { overrides: unknown }).overrides, { language: "de-DE", timezone: "Europe/Berlin", cpu: "8", width: 1280, height: 800 });
  assert.ok(!JSON.stringify(calls).includes("SYNTHETIC_GPU"));
  assert.ok(!JSON.stringify(calls).includes("SYNTHETIC_PATH"));
  assert.ok(!JSON.stringify(calls).includes("--no-sandbox"));
  assert.equal((calls[3].payload as { profileHash: string }).profileHash, "canonical-hash");
});

test("native rejects a nested demo fingerprint preview and previews publish no saved snapshot", async () => {
  const p = { previewId: "synthetic-preview", environment, expectedRevision: 1, fingerprint: demoFingerprint(demoProfile(environment, 1)) };
  const { app } = fixture(() => ok(p));
  const before = app.getSnapshot(); let publishes = 0; app.subscribe(() => publishes++);
  const result = await app.generateFingerprint({ previewId: p.previewId, kernelId: environment.coreId, templateId: "windows-desktop-v1", overrides: environment });
  assert.equal(result.ok, false);
  if (!result.ok) assert.equal(result.error.code, "CAPABILITY_UNSUPPORTED");
  assert.equal(app.getSnapshot(), before);
  assert.equal(publishes, 0);
});

test("applying a delayed device preview keeps unsaved names/proxy/URLs and detects stale preferences", () => {
  const draft = { ...environment, name: "合成未保存名称", proxyId: "synthetic-proxy", urls: "https://example.test/", cookies: [{ name: "synthetic", value: "", domain: "example.test", path: "/" }] };
  const profile = demoProfile({ ...environment, seed: "456", cpu: "8" }, 2);
  const next = applyFingerprint(draft, profile);
  assert.equal(next.name, draft.name);
  assert.equal(next.proxyId, draft.proxyId);
  assert.equal(next.urls, draft.urls);
  assert.deepEqual(next.cookies, draft.cookies);
  assert.equal(next.seed, "456");
  assert.equal(fingerprintMatchesConfiguration(profile, next), true);
  assert.equal(fingerprintMatchesConfiguration(profile, { ...next, height: 900 }), false);
});

test("runtime requests only select stored IDs and explicit direct policy, never client launch overrides", async () => {
  const { app, calls } = fixture(() => rejected);
  const untrusted = { environmentId: environment.id, requestId: "runtime-request", networkPolicy: "direct" as const, executable: "SYNTHETIC_PATH", userDataDir: "SYNTHETIC_OTHER_PROFILE", arguments: ["--no-sandbox"], state: "running" };
  await app.startRuntime(untrusted);
  await app.stopRuntime(untrusted);
  await app.inspectRuntime([environment.id]);
  assert.deepEqual(calls.map(call => call.method), ["Runtime.Start", "Workspace.Read", "Runtime.Stop", "Workspace.Read", "Runtime.Inspect"]);
  assert.deepEqual(calls[0].payload, { environmentId: environment.id, requestId: untrusted.requestId, networkPolicy: "direct" });
  assert.deepEqual(calls[2].payload, { environmentId: environment.id, requestId: untrusted.requestId });
  assert.ok(!JSON.stringify(calls).includes("SYNTHETIC_PATH"));
  assert.ok(!JSON.stringify(calls).includes("--no-sandbox"));
});

test("accepted runtime starts refresh real state but never emit a false completion", async () => {
  const task: Operation = { id: "synthetic-runtime-op", kind: "runtime-start", state: "accepted", total: 1, completedIds: [], cancelRequested: false, environmentId: environment.id };
  const workspace = empty(); workspace.state.environments = [{ ...environment, status: "starting" }];
  const { app } = fixture(request => request.method === "Workspace.Read" ? ok(workspace) : ok({ status: "accepted", operation: task }));
  const events: OperationEvent[] = []; app.subscribeEvents(event => events.push(event));
  await app.startRuntime({ environmentId: environment.id, requestId: "runtime-accept", networkPolicy: "direct" });
  assert.equal(app.getSnapshot().state.environments[0].status, "starting");
  assert.equal(events.length, 0);
});

test("native refuses a nested demo runtime session without publishing running", async () => {
  const session = { mode: "demo", environmentId: environment.id, state: "running" };
  const workspace = { ...empty(), runtimeSessions: { [environment.id]: session } };
  const { app } = fixture(request => request.method === "Workspace.Read" ? ok(workspace) : ok([session]));
  assert.equal((await app.refresh()).ok, false);
  assert.deepEqual(app.getSnapshot().state.environments, []);
  assert.equal((await app.inspectRuntime([environment.id])).ok, false);
});

test("recovery commands only send the exact environment/session and request IDs, never PID or commands", async () => {
  const { app, calls } = fixture(request => request.method === "Workspace.Read" ? ok(empty()) : rejected);
  const untrusted = { environmentId: environment.id, sessionId: "synthetic-saved-session", requestId: "synthetic-recovery-request", pid: 4242, executable: "SYNTHETIC_PATH", arguments: ["--no-sandbox"], networkPolicy: "direct" };
  await app.forceStopRuntime(untrusted);
  await app.reconcileRuntime(untrusted);
  assert.deepEqual(calls.map(call => call.method), ["Runtime.ForceStop", "Workspace.Read", "Runtime.Reconcile", "Workspace.Read"]);
  const expected = { environmentId: environment.id, sessionId: untrusted.sessionId, requestId: untrusted.requestId };
  assert.deepEqual(calls[0].payload, expected); assert.deepEqual(calls[2].payload, expected);
  assert.ok(!JSON.stringify(calls).includes("SYNTHETIC_PATH"));
  assert.ok(!JSON.stringify(calls).includes("4242"));
});

test("a failed normal close still refreshes the native next step without claiming a forced stop", async () => {
  const session: RuntimeSession = { mode: "native", environmentId: environment.id, sessionId: "synthetic-session", operationId: "synthetic-start", state: "error", revision: 1, fingerprintRevision: 1, kernelId: environment.coreId, userDataRef: "environments/synthetic/user-data", networkPolicy: "direct", pid: 4242, canControl: false, canForce: true, needsReconcile: false, persistencePending: false, error: { code: "CONTROL_CHANNEL_LOST", message: "合成控制通道失败", retryable: false }, nextAction: "仅可明确结束本次受控会话" };
  const workspace = { ...empty(), runtimeSessions: { [environment.id]: session } };
  const { app } = fixture(request => request.method === "Workspace.Read" ? ok(workspace) : rejected);
  const response = await app.stopRuntime({ environmentId: environment.id, requestId: "synthetic-close-request" });
  assert.equal(response.ok, false);
  assert.equal(app.getSnapshot().runtimeSessions?.[environment.id].canForce, true);
  assert.equal(app.getSnapshot().runtimeSessions?.[environment.id].state, "error");
});

test("a temporary storage-pending failure can complete after persistence recovery but a real terminal stays final", async () => {
  const desired: Operation = { id: "synthetic-pending-storage", kind: "runtime-stop", state: "completed", total: 1, completedIds: [environment.id], cancelRequested: false, environmentId: environment.id };
  let recovering = false;
  const { app } = fixture(() => ok(recovering ? desired : { ...desired, state: "failed", completedIds: [], persistencePending: true, stage: "storage-pending", error: { code: "STORAGE_WRITE_FAILED", message: "合成结果待保存", retryable: true } }));
  const events: OperationEvent[] = []; app.subscribeEvents(event => events.push(event));
  const failed = await app.getOperation(desired.id);
  assert.ok(failed.ok && failed.data.persistencePending);
  assert.equal(events[0].type, "OperationProgress");
  recovering = true;
  const completed = await app.getOperation(desired.id);
  assert.ok(completed.ok && completed.data.state === "completed" && !completed.data.persistencePending);
  assert.equal(events[1].type, "OperationCompleted");
  recovering = false;
  const late = await app.getOperation(desired.id);
  assert.ok(late.ok && late.data.state === "completed");
  assert.equal(events.length, 2);
});

test("native proxy edits explicitly keep, replace or clear secrets without spreading legacy projection fields", async () => {
  const { app, calls } = fixture(() => rejected);
  const target = { proxyId: "synthetic-proxy-id", expectedRevision: 3, requestId: "synthetic-edit-request" };
  const configuration = { name: "合成节点", type: "http" as const, host: "localhost", port: 8080, country: "合成标签", username: "DO_NOT_SPREAD_USERNAME", password: "DO_NOT_SPREAD_PASSWORD", simulateFailure: true, credentialRef: "DO_NOT_SPREAD_REFERENCE" };
  const untrusted = { ...target, configuration, credentials: { action: "keep", username: "DO_NOT_SPREAD_USERNAME", password: "DO_NOT_SPREAD_PASSWORD" }, mode: "demo", checkReport: { exitIp: "203.0.113.45" } } as unknown as ProxyUpdateRequest;
  await app.updateProxy(untrusted);
  assert.deepEqual(calls[0].payload, { ...target, configuration: { name: configuration.name, type: "http", host: "localhost", port: 8080, country: "合成标签" }, credentials: { action: "keep" } });
  assert.ok(!JSON.stringify(calls).includes("DO_NOT_SPREAD"));
  await app.updateProxy({ ...target, configuration, credentials: { action: "replace", username: "synthetic-user", password: "synthetic-replacement" } });
  assert.deepEqual((calls[1].payload as ProxyUpdateRequest).credentials, { action: "replace", username: "synthetic-user", password: "synthetic-replacement" });
  await app.updateProxy({ ...target, configuration, credentials: { action: "clear" } });
  assert.deepEqual((calls[2].payload as ProxyUpdateRequest).credentials, { action: "clear" });
});

test("proxy commit and check do not forward raw input, TLS overrides, target URLs or process credentials", async () => {
  const { app, calls } = fixture(request => request.method === "Workspace.Read" ? ok(empty()) : rejected);
  const commit = { previewId: "synthetic-import-preview", selectedRows: [2, 4], requestId: "synthetic-import-request", text: "DO_NOT_FORWARD_RAW_CREDENTIALS", password: "DO_NOT_FORWARD_RAW_CREDENTIALS" };
  await app.commitProxyImport(commit);
  const target = { proxyId: "synthetic-proxy", expectedRevision: 1, requestId: "synthetic-check-request", targetUrl: "https://DO_NOT_FORWARD_TARGET.invalid", skipTlsVerify: true, credentialRef: "DO_NOT_FORWARD_REF", pid: 4242 } as ProxyTargetRequest;
  await app.checkProxy(target); await app.deleteProxy(target);
  assert.deepEqual(calls[0].payload, { previewId: commit.previewId, selectedRows: [2, 4], requestId: commit.requestId });
  assert.deepEqual(calls[1].payload, { proxyId: target.proxyId, expectedRevision: 1, requestId: target.requestId });
  assert.ok(!JSON.stringify(calls).includes("DO_NOT_FORWARD"));
  assert.ok(!JSON.stringify(calls).includes("4242"));
});

test("native proxy events recover temporary persistence failure but never accept a nested demo report", async () => {
  let state: "pending" | "completed" | "demo" = "pending";
  const operation: Operation = { id: "synthetic-proxy-operation", kind: "proxy-check", proxyId: "synthetic-proxy", state: "completed", total: 1, completedIds: ["synthetic-proxy"], cancelRequested: false };
  const { app } = fixture(() => ok(state === "pending" ? { ...operation, state: "failed", persistencePending: true, stage: "storage-pending" } : state === "completed" ? operation : { ...operation, proxyReport: { mode: "demo" } }));
  const events: OperationEvent[] = []; app.subscribeEvents(event => events.push(event));
  await app.getOperation(operation.id); assert.equal(events[0].type, "OperationProgress");
  state = "completed"; const completed = await app.getOperation(operation.id);
  assert.ok(completed.ok && completed.data.state === "completed"); assert.equal(events[1].type, "OperationCompleted");
  state = "demo"; const rejected = await app.getOperation(operation.id);
  assert.ok(!rejected.ok && rejected.error.code === "CAPABILITY_UNSUPPORTED"); assert.equal(events.length, 2);
});

test("native runtime proxy starts forward only explicit policy and never private endpoint or credentials", async () => {
  const { app, calls } = fixture(request => request.method === "Workspace.Read" ? ok(empty()) : rejected);
  const request = { environmentId: "synthetic-environment", requestId: "synthetic-proxy-start", networkPolicy: "proxy" as const, proxyEndpoint: "DO_NOT_FORWARD_LOOPBACK_ENDPOINT", proxyPassword: "DO_NOT_FORWARD_SECRET", bypass: "DIRECT", skipTlsVerify: true };
  await app.startRuntime(request);
  assert.deepEqual(calls[0].payload, { environmentId: request.environmentId, requestId: request.requestId, networkPolicy: "proxy" });
  assert.ok(!JSON.stringify(calls).includes("DO_NOT_FORWARD")); assert.ok(!JSON.stringify(calls).includes("DIRECT"));
});

test("runtime session proxy reports must match native channel identity and saved revision", async () => {
  const session: RuntimeSession = { mode: "native", environmentId: "synthetic-environment", sessionId: "synthetic-session", operationId: "synthetic-operation", state: "running", revision: 1, fingerprintRevision: 1, kernelId: "synthetic-kernel", userDataRef: "synthetic-reference", networkPolicy: "proxy", proxyId: "synthetic-proxy", proxyRevision: 3, proxyChannelId: "synthetic-channel", canControl: true, canForce: false, needsReconcile: false, persistencePending: false, proxyReport: { mode: "native", adapterVersion: "synthetic-host-only", channelId: "synthetic-channel", proxyId: "synthetic-proxy", revision: 3, startedAt: "2026-10-01T00:00:00Z", finishedAt: "2026-10-01T00:00:01Z", durationMs: 1000, targetOrigin: "https://synthetic-target.invalid", steps: [], exitIp: "203.0.113.81" } };
  const { app } = fixture(request => request.method === "Workspace.Read" ? ok({ ...empty(), runtimeSessions: { [session.environmentId]: session } }) : ok([session]));
  assert.ok((await app.refresh()).ok);
  assert.ok((await app.inspectRuntime([session.environmentId])).ok);
  session.proxyReport!.channelId = "unrelated-channel";
  const refresh = await app.refresh(); const inspect = await app.inspectRuntime([session.environmentId]);
  assert.ok(!refresh.ok && refresh.error.code === "CAPABILITY_UNSUPPORTED"); assert.ok(!inspect.ok && inspect.error.code === "CAPABILITY_UNSUPPORTED");
  session.proxyReport!.channelId = session.proxyChannelId; session.proxyReport!.revision = 2;
  assert.ok(!(await app.inspectRuntime([session.environmentId])).ok);
});
