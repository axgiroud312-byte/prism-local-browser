import assert from "node:assert/strict";
import test from "node:test";
import { WailsAdapter, type NativeRequest, type NativeBridge } from "../src/application/wails-adapter.ts";
import type { ApplicationResult, NativeRestoreSourceState, NativeRestorePreview } from "../src/application/contract.ts";

const ok = <T>(data: T): ApplicationResult<T> => ({ ok: true, mode: "native", data });
const fail: ApplicationResult<never> = { ok: false, mode: "native", error: { code: "BACKUP_PREFLIGHT_FAILED", message: "SYNTHETIC_CLEANUP_BUSY", retryable: true } };
const preview = (): NativeRestorePreview => ({ mode: "native", previewId: "original-preview", name: "SYNTHETIC.prismbackup", format: "prism-local-backup", archiveSha256: "a".repeat(64), manifestSha256: "b".repeat(64), scope: "selected", createdAt: "2026-10-07", expiresAt: "2030-10-07", environmentCount: 0, addCount: 0, overwriteCount: 0, conflictCount: 0, missingKernelCount: 0, credentialReentryCount: 0, bytes: 1, canRestore: true, kernels: [], credentials: [] });
function fixture(handler: (r: NativeRequest) => ApplicationResult<unknown> | Promise<ApplicationResult<unknown>>) {
  const calls: NativeRequest[] = [];
  const bridge: NativeBridge = async <T>(r: NativeRequest) => { calls.push(r); return await handler(r) as ApplicationResult<T>; };
  return { app: new WailsAdapter(bridge), calls };
}
const selected = (r: NativeRequest): NativeRestoreSourceState => ({ mode: "native", requestId: (r.payload as { requestId: string }).requestId, status: "selected", sourceToken: "original-host-token", name: "SYNTHETIC.prismbackup", preflightRunning: false, cleanupPending: false });

test("ordinary restore lost token and IPC failure recover only the original service request", async () => {
  for (const loss of ["missing-token", "transport"] as const) {
    let state!: NativeRestoreSourceState;
    const { app, calls } = fixture(r => {
      if (r.method === "Backup.SelectRestoreSource") { state = selected(r); if (loss === "transport") throw new Error("SYNTHETIC_PRIVATE_IPC"); return ok({ ...state, sourceToken: undefined }); }
      if (r.method === "Backup.ReadRestoreSource") return ok(state);
      return ok({ status: "discarded" });
    });
    assert.equal((await app.selectRestoreSource()).ok, false);
    const original = app.getPendingRestoreSource(); assert.ok(original?.requestId); assert.equal(original.sourceToken, undefined);
    assert.equal((await app.selectRestoreSource()).ok, false); assert.equal(calls.length, 1, "new chooser must remain blocked");
    const recovered = await app.recoverRestoreSource(); assert.ok(recovered.ok); assert.equal(app.getPendingRestoreSource()?.sourceToken, state.sourceToken);
    assert.deepEqual(calls[1].payload, { requestId: original.requestId });
    assert.ok((await app.discardPendingRestoreSource()).ok); assert.equal(app.getPendingRestoreSource(), undefined);
    assert.deepEqual(calls.at(-1)?.payload, { requestId: original.requestId, previewId: "", sourceToken: state.sourceToken });
    assert.equal(calls.some(r => r.method === "Workspace.Read"), false);
  }
});

test("ordinary restore refuses foreign ownership, retains failed cleanup across subscribers and retries exact request", async () => {
  let state!: NativeRestoreSourceState, foreign = true, busy = true;
  const { app, calls } = fixture(r => {
    if (r.method === "Backup.SelectRestoreSource") { state = selected(r); return ok(state); }
    if (r.method === "Backup.ReadRestoreSource") return ok({ ...state, requestId: foreign ? "foreign" : state.requestId, cleanupPending: true });
    return busy ? fail : ok({ status: "discarded" });
  });
  assert.ok((await app.selectRestoreSource()).ok);
  assert.equal((await app.recoverRestoreSource()).ok, false); assert.equal(app.getPendingRestoreSource()?.sourceToken, state.sourceToken);
  const unsubscribe = app.subscribe(() => {}); assert.equal((await app.discardPendingRestoreSource()).ok, false); unsubscribe();
  foreign = false; assert.ok((await app.recoverRestoreSource()).ok); assert.equal(app.getPendingRestoreSource()?.cleanupRequested, true);
  assert.equal((await app.previewRestore(state.sourceToken!)).ok, false); assert.equal((await app.selectRestoreSource()).ok, false);
  busy = false; assert.ok((await app.discardPendingRestoreSource()).ok);
  const attempts = calls.filter(r => r.method === "Backup.DiscardRestore"); assert.equal(attempts.length, 2); assert.deepEqual(attempts[0], attempts[1]);
});

test("ordinary restore pending selector and late preflight retain cleanup ownership until original worker is confirmed", async () => {
  let state!: NativeRestoreSourceState, deliver!: (r: ApplicationResult<unknown>) => void, selecting = true, preparing = false;
  const { app, calls } = fixture(r => {
    if (r.method === "Backup.SelectRestoreSource") { state = selected(r); return new Promise(resolve => { deliver = resolve; }); }
    if (r.method === "Backup.PreviewRestore") { preparing = true; return new Promise(resolve => { deliver = resolve; }); }
    if (r.method === "Backup.ReadRestoreSource") return ok({ ...state, preview: preparing ? undefined : preview(), preflightRunning: preparing });
    if (r.method === "Backup.DiscardRestore") return selecting || preparing ? fail : ok({ status: "discarded" });
    return fail;
  });
  const selection = app.selectRestoreSource(); const twin = app.selectRestoreSource(); assert.equal(selection, twin);
  assert.equal((await app.discardPendingRestoreSource()).ok, false);
  selecting = false; deliver(ok(state)); assert.ok((await selection).ok);
  assert.equal(app.getPendingRestoreSource()?.cleanupRequested, true); assert.equal((await app.previewRestore(state.sourceToken!)).ok, false);
  assert.ok((await app.discardPendingRestoreSource()).ok);
  const next = app.selectRestoreSource(); deliver(ok(state)); assert.ok((await next).ok);
  const preflight = app.previewRestore(state.sourceToken!); assert.equal((await app.discardPendingRestoreSource()).ok, false);
  preparing = false; deliver(ok(preview())); assert.ok((await preflight).ok);
  assert.ok((await app.discardPendingRestoreSource()).ok); assert.equal(app.getPendingRestoreSource(), undefined);
  assert.equal(calls.filter(r => r.method === "Backup.SelectRestoreSource").length, 2);
});

test("ordinary restore discarded selector's delayed response cannot become a new page's selected target", async () => {
  let state!: NativeRestoreSourceState, deliver!: (r: ApplicationResult<unknown>) => void;
  const { app, calls } = fixture(r => {
    if (r.method === "Backup.SelectRestoreSource") { state = selected(r); return new Promise(resolve => { deliver = resolve; }); }
    return ok({ status: "discarded" });
  });
  const old = app.selectRestoreSource(); assert.ok((await app.discardPendingRestoreSource()).ok);
  assert.equal(app.getPendingRestoreSource(), undefined);
  const newPageAttempt = app.selectRestoreSource(); assert.equal((await newPageAttempt).ok, false);
  deliver(ok(state)); assert.equal((await old).ok, false); assert.equal(app.getPendingRestoreSource(), undefined);
  assert.equal(calls.filter(r => r.method === "Backup.SelectRestoreSource").length, 1);
});

test("ordinary restore ownership read arriving after confirmed discard cannot restore the old source", async () => {
  let state!: NativeRestoreSourceState, deliver!: (r: ApplicationResult<unknown>) => void;
  const { app } = fixture(r => {
    if (r.method === "Backup.SelectRestoreSource") { state = selected(r); return ok(state); }
    if (r.method === "Backup.ReadRestoreSource") return new Promise(resolve => { deliver = resolve; });
    return ok({ status: "discarded" });
  });
  assert.ok((await app.selectRestoreSource()).ok); const read = app.recoverRestoreSource();
  assert.ok((await app.discardPendingRestoreSource()).ok); deliver(ok(state));
  assert.equal((await read).ok, false); assert.equal(app.getPendingRestoreSource(), undefined);
});
