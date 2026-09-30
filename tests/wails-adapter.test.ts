import assert from "node:assert/strict";
import test from "node:test";
import { WailsAdapter, type NativeBridge, type NativeRequest } from "../src/application/wails-adapter.ts";
import type { ApplicationResult, Operation, OperationEvent, WorkspaceView } from "../src/application/contract.ts";
import type { Environment } from "../src/domain.ts";

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
