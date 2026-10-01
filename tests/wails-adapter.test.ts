import assert from "node:assert/strict";
import test from "node:test";
import { WailsAdapter, type NativeBridge, type NativeRequest } from "../src/application/wails-adapter.ts";
import type { ApplicationResult, Operation, OperationEvent, RuntimeSession, WorkspaceView, ProxyUpdateRequest, ProxyTargetRequest } from "../src/application/contract.ts";
import type { Environment } from "../src/domain.ts";
import { applyFingerprint, demoFingerprint, demoProfile, fingerprintMatchesConfiguration } from "../src/application/fingerprint-model.ts";

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
