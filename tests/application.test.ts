import assert from "node:assert/strict";
import test from "node:test";
import { DemoAdapter } from "../src/application/demo-adapter.ts";
import { newerOperationEvent, type ApplicationResult, type OperationEvent } from "../src/application/contract.ts";
import { STORAGE_KEY, seedState, createSnapshot, restoreSnapshot } from "../src/domain.ts";
import type { Environment } from "../src/domain.ts";

class MemoryStorage {
  value: string | null = null;
  failWrite = false;
  failRead = false;
  getItem() {
    if (this.failRead) throw new Error("unavailable");
    return this.value;
  }
  setItem(_key: string, value: string) {
    if (this.failWrite) throw new Error("disk full / SYNTHETIC_SECRET");
    this.value = value;
  }
  removeItem() { this.value = null; }
}
function data<T>(result: ApplicationResult<T>): T {
  if (!result.ok) assert.fail(JSON.stringify(result));
  assert.equal(result.mode, "demo");
  return result.data;
}
function fixture(yieldControl?: () => Promise<void>) {
  const storage = new MemoryStorage();
  const initial = seedState();
  initial.environments.forEach(e => { e.status = "ready"; });
  storage.value = JSON.stringify(initial);
  const app = new DemoAdapter(storage, { yieldControl });
  return { app, storage };
}
async function preview(app: DemoAdapter, sourceId?: string) {
  return data(await app.previewEnvironment({ kind: "create", sourceId }));
}
async function waitFor(app: DemoAdapter, operationId: string) {
  for (let i = 0; i < 100; i++) {
    const op = data(await app.getOperation(operationId));
    if (["completed", "failed", "cancelled"].includes(op.state)) return op;
    await new Promise(resolve => setTimeout(resolve, 2));
  }
  assert.fail("operation did not terminate");
}
async function create(app: DemoAdapter, name = "契约样本") {
  const p = await preview(app);
  const accepted = data(await app.createBatch({ previewId: p.previewId, configuration: { ...p.environment, name }, count: 1, requestId: crypto.randomUUID() }));
  assert.equal(accepted.status, "accepted");
  const op = await waitFor(app, accepted.operation.id);
  assert.equal(op.state, "completed");
  return app.getSnapshot().state.environments.find(e => e.id === op.completedIds[0])!;
}

test("preview and discard do not write storage; clone previews have new seeds and empty cookies", async () => {
  const { app, storage } = fixture();
  const source = app.getSnapshot().state.environments[0];
  const before = storage.value;
  const p = await preview(app, source.id);
  assert.notEqual(p.environment.seed, source.seed);
  assert.equal(p.environment.proxyId, source.proxyId);
  assert.deepEqual(p.environment.cookies, []);
  data(await app.discardPreview(p.previewId));
  assert.equal(storage.value, before);
  const result = await app.createBatch({ previewId: p.previewId, configuration: { ...p.environment, name: "discarded" }, count: 1, requestId: "discarded" });
  assert.equal(result.ok, false);
});

test("create commits an explicit seed and exact references; reopening reads the committed record", async () => {
  const { app, storage } = fixture();
  const p = await preview(app, "env-1");
  const accepted = data(await app.createBatch({ previewId: p.previewId, configuration: { ...p.environment, name: "合成新环境" }, count: 1, requestId: "create" }));
  const op = await waitFor(app, accepted.operation.id);
  assert.equal(op.completedIds.length, 1);
  const saved = new DemoAdapter(storage).getSnapshot().state.environments.find(e => e.id === op.completedIds[0])!;
  assert.equal(saved.seed, p.environment.seed);
  assert.equal(saved.proxyId, p.environment.proxyId);
  assert.equal(saved.coreId, p.environment.coreId);
  assert.deepEqual(saved.cookies, []);
  assert.equal(createSnapshot(app.getSnapshot().state).format, "prism-prototype");
});

test("edit persists only configuration and leaves identity, seed and cookie data unchanged", async () => {
  const { app, storage } = fixture();
  const saved = await create(app);
  const cookie = { name: "synthetic-session", value: "", domain: "example.test", path: "/" };
  data(app.compatibility.update(s => ({ ...s, environments: s.environments.map(e => e.id === saved.id ? { ...e, cookies: [cookie] } : e) })));
  const p = data(await app.previewEnvironment({ kind: "edit", sourceId: saved.id }));
  const result = data(await app.updateEnvironment({ previewId: p.previewId, configuration: { ...p.environment, name: "改名样本", cookies: [{ name: "untrusted", value: "", domain: "example.test", path: "/" }] } as Environment, expectedRevision: p.expectedRevision!, requestId: "edit" }));
  assert.equal(result.status, "completed");
  assert.equal(result.environment.record.seed, saved.seed);
  assert.equal(result.environment.record.id, saved.id);
  assert.deepEqual(result.environment.record.cookies, [cookie]);
  assert.equal(new DemoAdapter(storage).getSnapshot().state.environments.find(e => e.id === saved.id)!.name, "改名样本");
});

test("regeneration is preview-only; cancellation keeps the stored seed", async () => {
  const { app, storage } = fixture();
  const before = storage.value;
  const p = data(await app.previewEnvironment({ kind: "edit", sourceId: "env-1" }));
  const regenerated = data(await app.regeneratePreview(p.previewId));
  assert.notEqual(regenerated.environment.seed, p.environment.seed);
  data(await app.discardPreview(p.previewId));
  assert.equal(storage.value, before);
});

test("duplicate requestId is idempotent and cannot be reused with a different payload", async () => {
  const { app } = fixture();
  const p = await preview(app);
  const request = { previewId: p.previewId, configuration: { ...p.environment, name: "幂等样本" }, count: 1, requestId: "same" };
  const first = data(await app.createBatch(request));
  const second = data(await app.createBatch(request));
  assert.equal(first.operation.id, second.operation.id);
  await waitFor(app, first.operation.id);
  assert.equal(app.getSnapshot().state.environments.filter(e => e.name === "幂等样本").length, 1);
  const reused = await app.createBatch({ ...request, count: 2 });
  assert.equal(reused.ok, false);
  if (!reused.ok) assert.equal(reused.error.code, "REQUEST_ID_REUSED");
});

test("stale expectedRevision never overwrites a newer edit, including after reopening", async () => {
  const { app, storage } = fixture();
  const p = data(await app.previewEnvironment({ kind: "edit", sourceId: "env-1" }));
  data(await app.updateEnvironment({ previewId: p.previewId, configuration: { ...p.environment, name: "较新修改" }, expectedRevision: p.expectedRevision!, requestId: "new" }));
  const reopened = new DemoAdapter(storage);
  const latest = data(await reopened.previewEnvironment({ kind: "edit", sourceId: "env-1" }));
  assert.ok(latest.expectedRevision! > p.expectedRevision!);
  const stale = await reopened.updateEnvironment({ previewId: latest.previewId, configuration: { ...latest.environment, name: "过期修改" }, expectedRevision: p.expectedRevision!, requestId: "stale" });
  assert.equal(stale.ok, false);
  if (!stale.ok) assert.equal(stale.error.code, "REVISION_CONFLICT");
  assert.equal(reopened.getSnapshot().state.environments[0].name, "较新修改");
});

test("storage failure produces no committed edit, successful event, or leaked exception", async () => {
  const { app, storage } = fixture();
  const before = storage.value;
  const p = data(await app.previewEnvironment({ kind: "edit", sourceId: "env-1" }));
  const events: OperationEvent[] = [];
  app.subscribeEvents(e => events.push(e));
  storage.failWrite = true;
  const failed = await app.updateEnvironment({ previewId: p.previewId, configuration: { ...p.environment, name: "不应保存" }, expectedRevision: p.expectedRevision!, requestId: "failure" });
  assert.equal(failed.ok, false);
  if (!failed.ok) assert.equal(failed.error.code, "STORAGE_WRITE_FAILED");
  assert.equal(storage.value, before);
  assert.equal(app.getSnapshot().state.environments[0].name, p.environment.name);
  assert.equal(events.length, 0);
  assert.ok(!JSON.stringify(failed).includes("SYNTHETIC_SECRET"));
  storage.failWrite = false;
  data(await app.updateEnvironment({ previewId: p.previewId, configuration: { ...p.environment, name: "重试保存" }, expectedRevision: p.expectedRevision!, requestId: "failure" }));
});

test("corrupt storage is retained and all writes are blocked until explicit reset", async () => {
  const storage = new MemoryStorage();
  storage.value = "{ broken synthetic record";
  const app = new DemoAdapter(storage);
  assert.equal(app.getSnapshot().damagedRecord, storage.value);
  assert.equal((await app.previewEnvironment({ kind: "create" })).ok, false);
  assert.equal(app.compatibility.update(s => ({ ...s, environments: [] })).ok, false);
  assert.equal(storage.value, "{ broken synthetic record");
  data(await app.resetDamagedWorkspace());
  assert.equal(app.getSnapshot().issue, undefined);
  assert.ok(JSON.parse(storage.value!).environments.length);
});

test("storage access failure blocks rather than overwriting an unread workspace", async () => {
  const storage = new MemoryStorage();
  storage.failRead = true;
  const app = new DemoAdapter(storage);
  assert.equal(app.getSnapshot().issue?.code, "STORAGE_READ_FAILED");
  assert.equal((await app.previewEnvironment({ kind: "create" })).ok, false);
  assert.equal(storage.value, null);
});

test("external changes, removal and storage.clear block stale writes", async () => {
  for (const replacement of ["{ foreign data }", null]) {
    const { app, storage } = fixture();
    const p = data(await app.previewEnvironment({ kind: "edit", sourceId: "env-1" }));
    storage.value = replacement;
    app.handleStorageChange({ key: replacement === null ? null : STORAGE_KEY });
    const result = await app.updateEnvironment({ previewId: p.previewId, configuration: { ...p.environment, name: "旧页修改" }, expectedRevision: p.expectedRevision!, requestId: "external" });
    assert.equal(result.ok, false);
    assert.equal(storage.value, replacement);
    assert.equal(app.getSnapshot().issue?.code, "WORKSPACE_CHANGED");
  }
});

test("validation, references and busy state are checked at commit, not just in the page", async () => {
  const { app } = fixture();
  const p = data(await app.previewEnvironment({ kind: "edit", sourceId: "env-1" }));
  for (const patch of [{ name: "" }, { name: "英国精品店" }, { urls: "about:blank" }, { proxyId: "missing" }, { coreId: "missing" }]) {
    const result = await app.updateEnvironment({ previewId: p.previewId, configuration: { ...p.environment, ...patch }, expectedRevision: p.expectedRevision!, requestId: crypto.randomUUID() });
    assert.equal(result.ok, false);
  }
  data(app.compatibility.update(s => ({ ...s, environments: s.environments.map(e => e.id === "env-1" ? { ...e, status: "running" } : e) })));
  const busy = await app.previewEnvironment({ kind: "edit", sourceId: "env-1" });
  assert.equal(busy.ok, false);
  if (!busy.ok) assert.equal(busy.error.code, "PROFILE_BUSY");
});

test("cancel affects only its operation and retains the persisted completed chunk", async () => {
  let resume!: () => void;
  const { app, storage } = fixture(() => new Promise<void>(resolve => { resume = resolve; }));
  const events: OperationEvent[] = [];
  app.subscribeEvents(e => events.push(e));
  const p = await preview(app);
  const accepted = data(await app.createBatch({ previewId: p.previewId, configuration: { ...p.environment, name: "取消样本" }, count: 1000, requestId: "cancel" }));
  assert.equal(data(await app.getOperation(accepted.operation.id)).completedIds.length, 25);
  assert.equal((await app.cancelOperation("wrong-operation")).ok, false);
  data(await app.cancelOperation(accepted.operation.id));
  resume();
  const terminal = await waitFor(app, accepted.operation.id);
  assert.equal(terminal.state, "cancelled");
  assert.equal(terminal.completedIds.length, 25);
  assert.equal(terminal.total, 1000);
  assert.equal(new DemoAdapter(storage).getSnapshot().state.environments.filter(e => e.name.startsWith("取消样本")).length, 25);
  assert.ok(events.every((e, i) => i === 0 || e.sequence > events[i - 1].sequence));
  assert.equal(events.at(-1)!.type, "OperationCompleted");
});

test("a write failure after acceptance reports an actual failed operation with no false created items", async () => {
  let resume!: () => void;
  const { app, storage } = fixture(() => new Promise<void>(resolve => { resume = resolve; }));
  const p = await preview(app);
  const accepted = data(await app.createBatch({ previewId: p.previewId, configuration: { ...p.environment, name: "空间不足" }, count: 50, requestId: "resource" }));
  storage.failWrite = true;
  resume();
  const op = await waitFor(app, accepted.operation.id);
  assert.equal(op.state, "failed");
  assert.equal(op.completedIds.length, 25);
  assert.equal(op.error?.code, "STORAGE_WRITE_FAILED");
  assert.equal(new DemoAdapter(storage).getSnapshot().state.environments.filter(e => e.name.startsWith("空间不足")).length, 25);
});

test("late or old progress never regresses a completed operation", () => {
  const terminal: OperationEvent = { mode: "demo", type: "OperationCompleted", operationId: "op", sequence: 5, time: new Date().toISOString(), operation: { id: "op", kind: "create", state: "completed", total: 1, completedIds: ["env"], cancelRequested: false } };
  const progress: OperationEvent = { ...terminal, type: "OperationProgress", sequence: 6, operation: { ...terminal.operation, state: "running" } };
  assert.equal(newerOperationEvent(terminal, progress), terminal);
  assert.equal(newerOperationEvent(terminal, { ...terminal, sequence: 4 }), terminal);
  assert.equal(newerOperationEvent(undefined, terminal), terminal);
});

test("a running creation blocks destructive compatibility changes and keeps every completed reference valid", async () => {
  let resume!: () => void;
  const { app, storage } = fixture(() => new Promise<void>(resolve => { resume = resolve; }));
  const snapshot = createSnapshot(app.getSnapshot().state);
  data(app.compatibility.update(s => ({ ...s, proxies: [...s.proxies, { ...s.proxies[0], id: "px-new" }] })));
  const p = await preview(app);
  const accepted = data(await app.createBatch({ previewId: p.previewId, configuration: { ...p.environment, name: "工作区锁", proxyId: "px-new" }, count: 50, requestId: "workspace-lock" }));
  const before = storage.value;
  const restore = app.compatibility.update(s => restoreSnapshot(s, snapshot));
  assert.equal(restore.ok, false);
  if (!restore.ok) assert.equal(restore.error.code, "PROFILE_BUSY");
  assert.equal(storage.value, before);
  data(await app.cancelOperation(accepted.operation.id));
  resume();
  const op = await waitFor(app, accepted.operation.id);
  assert.equal(op.completedIds.length, 25);
  const reopened = new DemoAdapter(storage).getSnapshot();
  assert.equal(reopened.issue, undefined);
  assert.ok(op.completedIds.every(id => reopened.state.environments.some(e => e.id === id && e.proxyId === "px-new")));
});

test("malformed revision metadata is blocked and retained, never an uncaught initialization exception", () => {
  for (const revisions of [1, true, "bad", [], null, { "env-1": -1 }]) {
    const storage = new MemoryStorage();
    const original = JSON.stringify({ ...seedState(), _application: { version: 1, revisions } });
    storage.value = original;
    const view = new DemoAdapter(storage).getSnapshot();
    assert.equal(view.issue?.code, "STORAGE_DAMAGED");
    assert.equal(view.damagedRecord, original);
    assert.equal(storage.value, original);
  }
});

test("batch names are checked as generated names, not the unsuffixed prefix", async () => {
  const { app } = fixture();
  const p = await preview(app);
  const accepted = data(await app.createBatch({ previewId: p.previewId, configuration: { ...p.environment, name: "北美主店" }, count: 2, requestId: "prefix" }));
  const op = await waitFor(app, accepted.operation.id);
  assert.equal(op.state, "completed");
  assert.equal(op.completedIds.length, 2);
  assert.ok(app.getSnapshot().state.environments.some(e => e.name === "北美主店 01"));
});

test("creation does not interrupt an in-flight stop and can be retried after it finishes", async () => {
  const { app } = fixture();
  const p = await preview(app);
  data(app.compatibility.update(s => ({ ...s, environments: s.environments.map(e => e.id === "env-1" ? { ...e, status: "stopping" } : e) })));
  const request = { previewId: p.previewId, configuration: { ...p.environment, name: "停止期间创建" }, count: 1, requestId: "stop-race" };
  const blocked = await app.createBatch(request);
  assert.equal(blocked.ok, false);
  if (!blocked.ok) assert.equal(blocked.error.code, "PROFILE_BUSY");
  data(app.compatibility.update(s => ({ ...s, environments: s.environments.map(e => e.id === "env-1" ? { ...e, status: "ready" } : e) })));
  const accepted = data(await app.createBatch(request));
  assert.equal((await waitFor(app, accepted.operation.id)).state, "completed");
  assert.equal(app.getSnapshot().state.environments.find(e => e.id === "env-1")!.status, "ready");
});

test("fingerprint generation is demo-only, read-only and a forged seed cannot bypass the preview", async () => {
  const { app, storage } = fixture();
  const p = data(await app.previewEnvironment({ kind: "edit", sourceId: "env-1" }));
  const before = storage.value;
  const generated = data(await app.generateFingerprint({ previewId: p.previewId, kernelId: p.environment.coreId, templateId: "windows-desktop-v1", overrides: { ...p.environment, cpu: "8" }, regenerate: true }));
  assert.equal(generated.fingerprint?.mode, "demo");
  assert.equal(generated.fingerprint?.capabilityReport.observedFingerprint, null);
  assert.equal(generated.fingerprint?.previewProfile.coreExecutableSha256, "");
  assert.deepEqual(generated.fingerprint?.previewProfile.parameters, []);
  assert.equal(storage.value, before);
  assert.notEqual(generated.environment.seed, p.environment.seed);
  const forged = await app.updateEnvironment({ previewId: p.previewId, configuration: { ...generated.environment, seed: "123" }, expectedRevision: p.expectedRevision!, requestId: "forged-fingerprint" });
  assert.equal(forged.ok, false);
  data(await app.discardPreview(p.previewId));
  assert.equal(storage.value, before);
});

test("frozen demo profile history survives reopen and restore keeps names and cookies", async () => {
  const { app, storage } = fixture();
  const original = app.getSnapshot().state.environments[0];
  const p = data(await app.previewEnvironment({ kind: "edit", sourceId: original.id }));
  const regenerated = data(await app.generateFingerprint({ previewId: p.previewId, kernelId: original.coreId, templateId: "windows-desktop-v1", overrides: original, regenerate: true }));
  data(await app.commitFingerprintRevision({ previewId: p.previewId, environmentId: original.id, profileHash: regenerated.fingerprint!.previewProfile.configHash, configuration: { ...regenerated.environment, name: "合成历史新名称" }, expectedRevision: p.expectedRevision!, requestId: "demo-profile-commit" }));
  const reopened = new DemoAdapter(storage);
  assert.equal(data(await reopened.listFingerprintRevisions(original.id)).length, 2);
  const next = data(await reopened.previewEnvironment({ kind: "edit", sourceId: original.id }));
  const restore = data(await reopened.previewFingerprintRestore(next.previewId, 1));
  assert.equal(restore.environment.name, "合成历史新名称");
  assert.equal(restore.environment.seed, original.seed);
  assert.ok(restore.fingerprint!.changes.some(change => change.field === "seed"));
  data(await reopened.commitFingerprintRevision({ previewId: next.previewId, environmentId: original.id, profileHash: restore.fingerprint!.previewProfile.configHash, configuration: restore.environment, expectedRevision: next.expectedRevision!, requestId: "demo-profile-restore" }));
  const saved = reopened.getSnapshot().state.environments.find(e => e.id === original.id)!;
  assert.equal(saved.seed, original.seed);
  assert.equal(saved.name, "合成历史新名称");
  assert.deepEqual(saved.cookies, original.cookies);
  assert.equal(data(await new DemoAdapter(storage).listFingerprintRevisions(original.id))[0].profile.configRevision, 3);
  assert.equal(JSON.parse(storage.value!).schemaVersion, 1);
});

test("frozen demo profile write failure keeps current and historical records intact", async () => {
  const { app, storage } = fixture();
  const p = data(await app.previewEnvironment({ kind: "edit", sourceId: "env-1" }));
  const next = data(await app.generateFingerprint({ previewId: p.previewId, kernelId: p.environment.coreId, templateId: "windows-desktop-v1", overrides: p.environment, regenerate: true }));
  const request = { previewId: p.previewId, environmentId: p.environment.id, profileHash: next.fingerprint!.previewProfile.configHash, configuration: next.environment, expectedRevision: p.expectedRevision!, requestId: "history-failure" };
  const before = storage.value;
  storage.failWrite = true;
  const failed = await app.commitFingerprintRevision(request);
  assert.equal(failed.ok, false);
  assert.equal(storage.value, before);
  assert.equal(data(await app.listFingerprintRevisions("env-1")).length, 1);
  storage.failWrite = false;
  data(await app.commitFingerprintRevision(request));
  data(await app.commitFingerprintRevision(request));
  assert.equal(data(await app.listFingerprintRevisions("env-1")).length, 2);
});

test("a stale fingerprint preview cannot be promoted by supplying the latest revision", async () => {
  const { app, storage } = fixture();
  const first = data(await app.previewEnvironment({ kind: "edit", sourceId: "env-1" }));
  const stale = data(await app.previewEnvironment({ kind: "edit", sourceId: "env-1" }));
  const newer = data(await app.regeneratePreview(first.previewId));
  const older = data(await app.regeneratePreview(stale.previewId));
  const saved = data(await app.commitFingerprintRevision({ previewId: newer.previewId, environmentId: "env-1", profileHash: newer.fingerprint!.previewProfile.configHash, configuration: newer.environment, expectedRevision: newer.expectedRevision!, requestId: "newer-profile" }));
  const before = storage.value;
  const result = await app.commitFingerprintRevision({ previewId: older.previewId, environmentId: "env-1", profileHash: older.fingerprint!.previewProfile.configHash, configuration: older.environment, expectedRevision: saved.newRevision, requestId: "promoted-old-profile" });
  assert.equal(result.ok, false);
  if (!result.ok) assert.equal(result.error.code, "REVISION_CONFLICT");
  assert.equal(storage.value, before);
  const reopened = new DemoAdapter(storage);
  assert.equal(reopened.getSnapshot().issue, undefined);
  assert.deepEqual(data(await reopened.listFingerprintRevisions("env-1")).map(item => item.profile.configRevision), [2, 1]);
});

test("simultaneous identical fingerprint commits reuse one result and never cross-cache methods", async () => {
  const { app } = fixture();
  const p = data(await app.previewEnvironment({ kind: "edit", sourceId: "env-1" }));
  const next = data(await app.regeneratePreview(p.previewId));
  const request = { previewId: p.previewId, environmentId: "env-1", profileHash: next.fingerprint!.previewProfile.configHash, configuration: next.environment, expectedRevision: p.expectedRevision!, requestId: "concurrent-profile" };
  const results = await Promise.all([app.commitFingerprintRevision(request), app.commitFingerprintRevision(request)]);
  assert.deepEqual(data(results[0]), data(results[1]));
  assert.equal(data(await app.listFingerprintRevisions("env-1")).length, 2);
  const reused = await app.updateEnvironment(request);
  assert.equal(reused.ok, false);
  if (!reused.ok) assert.equal(reused.error.code, "REQUEST_ID_REUSED");
});
