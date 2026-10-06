import { test, expect, type Page } from "@playwright/test";
import type { Operation, ProxyCheckReport, WorkspaceView } from "../../src/application/contract";
import { openReviewHarness, proxyReviewBridge, restoreReviewBridge, type ProxyReviewControl, type RestoreReviewControl, type ReviewApplication, type RestoreRead } from "./fixtures/review-recovery";
import { proxyKernelBridge } from "./fixtures/proxy-kernel";
import { openProxyKernelHarness } from "./fixtures/proxy-kernel-harness";

type ReviewWindow = { __reviewRecoveryApplication: ReviewApplication; __reviewRecoveryProxy: ProxyReviewControl; __reviewRecoveryRestore: RestoreReviewControl };
const errors = new WeakMap<Page, string[]>();
test.beforeEach(async ({ page, baseURL }) => {
  const captured: string[] = []; errors.set(page, captured); page.on("pageerror", error => captured.push(error.message));
  const origin = new URL(baseURL!).origin;
  await page.route("**/*", route => new URL(route.request().url()).origin === origin ? route.continue() : route.abort("blockedbyclient"));
  // Source edits by the other shared-root owners must not reload this harness.
  await page.routeWebSocket("**/*", socket => socket.close());
});
test.afterEach(async ({ page }) => { expect(errors.get(page)).toEqual([]); });

const proxyId = "synthetic-proxy-a", currentIp = "203.0.113.71", oldIp = "198.51.100.19", activeIp = "192.0.2.87";
function report(exitIp: string, failed = false): ProxyCheckReport {
  return { mode: "native", adapterVersion: "synthetic-only", proxyId, revision: 1, startedAt: "2026-10-06T01:00:00Z", finishedAt: "2026-10-06T01:00:01Z", durationMs: 1000, targetOrigin: "https://example.invalid", exitIp, steps: [{ stage: "authentication", status: failed ? "failed" : "passed", time: "2026-10-06T01:00:01Z", message: failed ? "旧修订认证失败" : "当前合成认证通过" }], ...(failed ? { error: { code: "PROXY_AUTH_FAILED", message: "旧修订认证失败", retryable: true } } : {}) };
}
function task(id: string, proxyReport: ProxyCheckReport, state: Operation["state"] = "completed", persistencePending = false): Operation {
  return { id, kind: "proxy-check", proxyId, state, persistencePending, total: 1, completedIds: [], cancelRequested: false, stage: state === "running" ? "authentication" : state, proxyReport };
}
const proxyDialog = (page: Page) => page.getByRole("dialog", { name: "代理检查详情 · 合成 SOCKS", exact: true });
async function openProxyDetails(page: Page) {
  await openReviewHarness(page, "proxies");
  await page.getByRole("row").filter({ hasText: "合成 SOCKS" }).getByRole("button", { name: /尚未检查|本次检查通过|检查未通过|结果待保存|隧道与认证/, exact: true }).click();
}
async function refreshProxy(page: Page, data: { recordReport?: ProxyCheckReport; operations: Operation[] }) {
  await page.evaluate(async data => { const fixture = window as unknown as ReviewWindow; fixture.__reviewRecoveryProxy.setData(data); await fixture.__reviewRecoveryApplication.refresh(); }, data);
}

test("F1 descending native history uses the current record, never the oldest report", async ({ page }) => {
  const current = report(currentIp), old = report(oldIp, true);
  await proxyReviewBridge(page, { recordReport: current, operations: [task("new-check", current), task("old-check", old, "failed")] });
  await openProxyDetails(page);
  await expect(proxyDialog(page)).toContainText("本次检查通过");
  await expect(proxyDialog(page)).toContainText(currentIp);
  await expect(proxyDialog(page)).not.toContainText(oldIp);
  await expect(proxyDialog(page)).not.toContainText("PROXY_AUTH_FAILED");
  await page.keyboard.press("Escape");
  await expect(page.getByRole("button", { name: "删除代理 合成 SOCKS", exact: true })).toBeDisabled();
  const environment = await page.evaluate(() => (window as unknown as { __proxyKernel: { view(): WorkspaceView } }).__proxyKernel.view().state.environments[0]);
  expect(environment).toMatchObject({ proxyId, seed: "172600001" });
});

test("F1 editing the revision clears details even while both historical reports remain", async ({ page }) => {
  const current = report(currentIp), old = report(oldIp, true);
  await proxyReviewBridge(page, { recordReport: current, operations: [task("new-check", current), task("old-check", old, "failed")] });
  await openReviewHarness(page, "proxies");
  await page.getByRole("button", { name: "编辑代理 合成 SOCKS", exact: true }).click();
  await page.getByRole("dialog", { name: "修改代理", exact: true }).getByRole("button", { name: "确认保存", exact: true }).click();
  await page.getByRole("row").filter({ hasText: "合成 SOCKS" }).getByRole("button", { name: "尚未检查", exact: true }).click();
  await expect(proxyDialog(page)).toContainText("修订 2");
  await expect(proxyDialog(page)).toContainText("暂无本次安全观测报告");
  await expect(proxyDialog(page)).not.toContainText(oldIp);
  await expect(proxyDialog(page)).not.toContainText(currentIp);
  await expect(proxyDialog(page)).not.toContainText("PROXY_AUTH_FAILED");
  const requests = await page.evaluate(() => (window as unknown as ReviewWindow).__reviewRecoveryProxy.calls.filter(call => call.method === "Proxy.Update"));
  expect(requests).toHaveLength(1);
  expect(requests[0].payload).toMatchObject({ proxyId, expectedRevision: 1, credentials: { action: "keep" } });
});

for (const mismatch of ["exact", "revision", "proxy-id"] as const) test(`F1 active report ${mismatch} is matched before falling back to the current record`, async ({ page }) => {
  const current = report(currentIp), active = { ...report(activeIp), ...(mismatch === "revision" ? { revision: 0 } : mismatch === "proxy-id" ? { proxyId: "synthetic-proxy-b" } : {}) };
  await proxyReviewBridge(page, { recordReport: current, operations: [task("active-check", active, "completed", true), task("old-check", report(oldIp, true), "failed")] });
  await openProxyDetails(page);
  await expect(proxyDialog(page)).toContainText(mismatch === "exact" ? activeIp : currentIp);
  await expect(proxyDialog(page)).not.toContainText(mismatch === "exact" ? currentIp : activeIp);
  await expect(proxyDialog(page)).not.toContainText(oldIp);
  if (mismatch === "exact") await expect(proxyDialog(page)).toContainText("结果待保存，尚非持久终态");
});

test("F1 invalid current-record identity or revision cannot become a current observation", async ({ page }) => {
  await proxyReviewBridge(page, { recordReport: report(currentIp), operations: [] });
  await openProxyDetails(page);
  await expect(proxyDialog(page)).toContainText(currentIp);
  for (const invalid of [{ ...report(oldIp, true), revision: 0 }, { ...report(oldIp, true), proxyId: "synthetic-proxy-b" }]) {
    await refreshProxy(page, { recordReport: invalid, operations: [] });
    await expect(proxyDialog(page)).toContainText("暂无本次安全观测报告");
    await expect(proxyDialog(page)).not.toContainText(oldIp);
    await expect(proxyDialog(page)).not.toContainText("PROXY_AUTH_FAILED");
  }
});

const inputDialog = (page: Page) => page.getByRole("dialog", { name: "导入本机备份", exact: true });
const preflightDialog = (page: Page) => page.getByRole("dialog", { name: "恢复前只读预检", exact: true });
const restoreCalls = (page: Page) => page.evaluate(() => (window as unknown as ReviewWindow).__reviewRecoveryRestore.calls);
const restoreState = (page: Page) => page.evaluate(() => (window as unknown as ReviewWindow).__reviewRecoveryRestore.workspace.state);
const settleRead = (page: Page, index = 0) => page.evaluate(index => (window as unknown as ReviewWindow).__reviewRecoveryRestore.settleRead(index), index);
const settleDiscard = (page: Page) => page.evaluate(() => (window as unknown as ReviewWindow).__reviewRecoveryRestore.settleDiscard());
const status = (page: Page) => page.evaluate(() => (window as unknown as ReviewWindow).__reviewRecoveryRestore.status());

async function openSource(page: Page) {
  await openReviewHarness(page, "restore");
  await page.getByRole("button", { name: "导入完整备份", exact: true }).click();
  await inputDialog(page).getByRole("button", { name: "选择本机备份包", exact: true }).click();
  await expect(inputDialog(page)).toContainText("synthetic-review-1.prismbackup");
}
async function startHeldRead(page: Page, phase: "preview" | "page") {
  await inputDialog(page).getByRole("button", { name: "完整校验并预览", exact: true }).click();
  if (phase === "preview") await expect(inputDialog(page).getByRole("status")).toContainText("正在流式读取");
  else await expect(preflightDialog(page)).toContainText("正在读取影响清单");
  await expect.poll(async () => (await status(page)).pendingReads).toBe(1);
}
async function cancelHeldRead(page: Page, phase: "preview" | "page") {
  const dialog = phase === "preview" ? inputDialog(page) : preflightDialog(page);
  await dialog.getByRole("button", { name: phase === "preview" ? "取消只读预检" : "丢弃此预览", exact: true }).click();
  await expect.poll(async () => (await status(page)).pendingDiscard).toBe(true);
}
async function expectRecovered(page: Page, phase: "preview" | "page") {
  const dialog = phase === "preview" ? inputDialog(page) : preflightDialog(page);
  await expect(dialog).not.toContainText("正在流式读取");
  await expect(dialog).not.toContainText("正在读取影响清单");
  await expect(dialog.getByRole("button", { name: phase === "preview" ? "完整校验并预览" : "重新读取影响清单", exact: true })).toBeEnabled();
  await expect(dialog).toContainText("取消未确认");
  await expect(dialog).not.toContainText("迟到旧");
  await expect(dialog).toContainText("synthetic-review-1.prismbackup");
  if (phase === "page") await expect(dialog.getByRole("button", { name: "下一步：确认恢复", exact: true })).toBeDisabled();
}
async function expectReadOnly(page: Page, before: Awaited<ReturnType<typeof restoreState>>) {
  expect((await restoreCalls(page)).filter(call => ["Backup.ApplyRestore", "Runtime.Stop", "Fingerprint.Generate", "Environment.Update"].includes(call.method))).toEqual([]);
  expect(await restoreState(page)).toEqual(before);
}

for (const phase of ["preview", "page"] as const) for (const outcome of ["unavailable", "refused", "throw"] as const) for (const order of ["cancel-first", "read-first"] as const) {
  test(`F2 ${phase} ${outcome} ${order} releases invalidated loading, retains context and stays read-only`, async ({ page }) => {
    await restoreReviewBridge(page, phase === "preview" ? "Backup.PreviewRestore" : "Backup.ReadRestorePage", outcome);
    await openSource(page); const before = await restoreState(page);
    await startHeldRead(page, phase); await cancelHeldRead(page, phase);
    if (order === "cancel-first") { await settleDiscard(page); await expectRecovered(page, phase); await settleRead(page); }
    else { await settleRead(page); await settleDiscard(page); }
    await expectRecovered(page, phase);
    // Escape is now a safe UI-only exit, not another uncertain native discard.
    const callCount = (await restoreCalls(page)).filter(call => call.method === "Backup.DiscardRestore").length;
    await page.keyboard.press("Escape"); await expect(page.getByRole("dialog")).toHaveCount(0);
    expect((await restoreCalls(page)).filter(call => call.method === "Backup.DiscardRestore")).toHaveLength(callCount);
    await page.getByRole("button", { name: "导入完整备份", exact: true }).click();
    await expectRecovered(page, phase);
    const dialog = phase === "preview" ? inputDialog(page) : preflightDialog(page);
    await dialog.getByRole("button", { name: phase === "preview" ? "完整校验并预览" : "重新读取影响清单", exact: true }).click();
    await expect(preflightDialog(page).getByRole("row").filter({ hasText: "工作环境 A" })).toBeVisible();
    await expect(preflightDialog(page)).not.toContainText("取消未确认");
    await expect(preflightDialog(page).getByRole("button", { name: "下一步：确认恢复", exact: true })).toBeEnabled();
    const requests = (await restoreCalls(page)).filter(call => call.method === (phase === "preview" ? "Backup.PreviewRestore" : "Backup.ReadRestorePage"));
    expect(requests).toHaveLength(2);
    expect(requests[1].payload).toEqual(requests[0].payload);
    await expectReadOnly(page, before);
  });
}

for (const replaceSource of [false, true]) test(`F2 late preview cannot release a newer read or discard its ${replaceSource ? "new" : "retained"} source token`, async ({ page }) => {
  await restoreReviewBridge(page, "Backup.PreviewRestore"); await openSource(page); const before = await restoreState(page);
  await startHeldRead(page, "preview"); await cancelHeldRead(page, "preview"); await settleDiscard(page); await expectRecovered(page, "preview");
  if (replaceSource) {
    await inputDialog(page).getByRole("button", { name: "选择本机备份包", exact: true }).click();
    await expect(inputDialog(page)).toContainText("synthetic-review-2.prismbackup");
  }
  const currentToken = (await status(page)).tokens.at(-1)!;
  await page.evaluate(() => (window as unknown as ReviewWindow).__reviewRecoveryRestore.holdRead("Backup.PreviewRestore"));
  await inputDialog(page).getByRole("button", { name: "完整校验并预览", exact: true }).click();
  await expect.poll(async () => (await status(page)).pendingReads).toBe(2);
  await settleRead(page, 0);
  await expect(inputDialog(page).getByRole("button", { name: "完整校验并预览", exact: true })).toBeDisabled();
  await expect(inputDialog(page)).toContainText("正在流式读取");
  await expect.poll(async () => (await restoreCalls(page)).filter(call => call.method === "Backup.DiscardRestore" && (call.payload as { previewId: string }).previewId === "synthetic-preflight-1").length).toBe(1);
  const cleanup = (await restoreCalls(page)).find(call => call.method === "Backup.DiscardRestore" && (call.payload as { previewId: string }).previewId === "synthetic-preflight-1")!;
  expect(cleanup.payload).toEqual({ previewId: "synthetic-preflight-1", sourceToken: "" });
  expect((await status(page)).tokens).toContain(currentToken);
  await settleRead(page, 1);
  await expect(preflightDialog(page).getByRole("row").filter({ hasText: "工作环境 A" })).toBeVisible();
  await expect(preflightDialog(page)).not.toContainText("迟到旧预览");
  await page.evaluate(() => (window as unknown as ReviewWindow).__reviewRecoveryRestore.setDiscardOutcome("success"));
  await preflightDialog(page).getByRole("button", { name: "丢弃此预览", exact: true }).click();
  await expect(page.getByRole("dialog")).toHaveCount(0);
  const last = (await restoreCalls(page)).filter(call => call.method === "Backup.DiscardRestore").at(-1)!;
  expect(last.payload).toEqual({ previewId: "synthetic-preflight-2", sourceToken: currentToken });
  await expectReadOnly(page, before);
});

test("F2 late page cannot populate or unlock a newer same-preview read", async ({ page }) => {
  await restoreReviewBridge(page, "Backup.ReadRestorePage"); await openSource(page); const before = await restoreState(page);
  await startHeldRead(page, "page"); await cancelHeldRead(page, "page"); await settleDiscard(page); await expectRecovered(page, "page");
  await page.evaluate(() => (window as unknown as ReviewWindow).__reviewRecoveryRestore.holdRead("Backup.ReadRestorePage"));
  await preflightDialog(page).getByRole("button", { name: "重新读取影响清单", exact: true }).click();
  await expect.poll(async () => (await status(page)).pendingReads).toBe(2);
  await settleRead(page, 0);
  await expect(preflightDialog(page)).toContainText("正在读取影响清单");
  await expect(preflightDialog(page)).not.toContainText("迟到旧清单");
  await expect(preflightDialog(page).getByRole("button", { name: "下一步：确认恢复", exact: true })).toBeDisabled();
  await settleRead(page, 1);
  await expect(preflightDialog(page)).not.toContainText("迟到旧清单");
  await expect(preflightDialog(page).getByRole("button", { name: "下一步：确认恢复", exact: true })).toBeEnabled();
  await expectReadOnly(page, before);
});

for (const phase of ["preview", "page"] as const) test(`F2 successful ${phase} cancellation cannot be reopened by the invalidated read`, async ({ page }) => {
  await restoreReviewBridge(page, phase === "preview" ? "Backup.PreviewRestore" : "Backup.ReadRestorePage", "success");
  await openSource(page); const before = await restoreState(page);
  await startHeldRead(page, phase); await cancelHeldRead(page, phase); await settleDiscard(page);
  await expect(page.getByRole("dialog")).toHaveCount(0); await settleRead(page);
  await expect(page.getByRole("dialog")).toHaveCount(0);
  await page.getByRole("button", { name: "导入完整备份", exact: true }).click();
  await expect(inputDialog(page)).toContainText("尚未选择文件");
  await expect(inputDialog(page).getByRole("button", { name: "完整校验并预览", exact: true })).toBeDisabled();
  await expectReadOnly(page, before);
});

test("F2 closing without a current source never sends a native discard", async ({ page }) => {
  await restoreReviewBridge(page); await openReviewHarness(page, "restore"); const before = await restoreState(page);
  await page.getByRole("button", { name: "导入完整备份", exact: true }).click();
  await inputDialog(page).getByRole("button", { name: "取消", exact: true }).click();
  await expect(page.getByRole("dialog")).toHaveCount(0);
  expect((await restoreCalls(page)).filter(call => call.method === "Backup.DiscardRestore")).toEqual([]);
  await expectReadOnly(page, before);
});

for (const phase of ["preview", "page"] as const) test(`F2 failed ${phase} recovery keeps cancellation uncertainty and retries only the retained target`, async ({ page }) => {
  const method: RestoreRead = phase === "preview" ? "Backup.PreviewRestore" : "Backup.ReadRestorePage";
  await restoreReviewBridge(page, method); await openSource(page); const before = await restoreState(page);
  await startHeldRead(page, phase); await cancelHeldRead(page, phase); await settleDiscard(page); await expectRecovered(page, phase); await settleRead(page);
  await page.evaluate<void, RestoreRead>(method => (window as unknown as ReviewWindow).__reviewRecoveryRestore.failNextRead(method), method);
  const dialog = phase === "preview" ? inputDialog(page) : preflightDialog(page);
  await dialog.getByRole("button", { name: phase === "preview" ? "完整校验并预览" : "重新读取影响清单", exact: true }).click();
  await expect(dialog).toContainText("PREVIEW_EXPIRED");
  await expect(dialog).toContainText("取消未确认");
  await expect(dialog).toContainText("synthetic-review-1.prismbackup");
  if (phase === "page") await expect(dialog.getByRole("button", { name: "下一步：确认恢复", exact: true })).toBeDisabled();
  const original = (await restoreCalls(page)).find(call => call.method === "Backup.DiscardRestore")!;
  await page.evaluate(() => (window as unknown as ReviewWindow).__reviewRecoveryRestore.setDiscardOutcome("success"));
  await dialog.getByRole("button", { name: phase === "preview" ? "重试取消只读预检" : "重试丢弃此预览", exact: true }).click();
  await expect(page.getByRole("dialog")).toHaveCount(0);
  expect((await restoreCalls(page)).filter(call => call.method === "Backup.DiscardRestore").at(-1)!.payload).toEqual(original.payload);
  await expectReadOnly(page, before);
});

test("review recovery preservation: unknown proxy edit retains the exact request and credential mask across hide", async ({ page }) => {
  await proxyKernelBridge(page); await openProxyKernelHarness(page);
  await page.evaluate(() => (window as unknown as { __proxyKernel: { setUpdateOutcome(value: string): void } }).__proxyKernel.setUpdateOutcome("unknown"));
  await page.getByRole("button", { name: "编辑代理 合成 SOCKS", exact: true }).click();
  const edit = page.getByRole("dialog", { name: "修改代理", exact: true });
  await edit.getByLabel("认证处理").selectOption("replace");
  await edit.getByLabel("新用户名（不回显旧值）").fill("synthetic-only");
  await edit.getByLabel("新密码").fill("synthetic-not-a-real-secret");
  await edit.getByRole("button", { name: "确认保存", exact: true }).click();
  await expect(edit.getByRole("status")).toContainText("NATIVE_UNAVAILABLE");
  await page.keyboard.press("Escape"); await expect(edit).toHaveCount(0);
  await page.getByRole("link", { name: "内核管理", exact: true }).click();
  await page.getByRole("link", { name: "代理管理", exact: true }).click();
  await expect(edit.getByLabel("新密码")).toHaveAttribute("type", "password");
  await expect(edit.getByLabel("新密码")).toHaveValue("synthetic-not-a-real-secret");
  await edit.getByRole("button", { name: "核实原保存请求", exact: true }).click(); await expect(edit).toHaveCount(0);
  const requests = await page.evaluate(() => (window as unknown as { __proxyKernel: { calls: { method: string; payload: unknown }[] } }).__proxyKernel.calls.filter(call => call.method === "Proxy.Update"));
  expect(requests).toHaveLength(2); expect(requests[1].payload).toEqual(requests[0].payload);
  expect(requests[0].payload).toMatchObject({ proxyId, expectedRevision: 1 });
  await expect(page.getByRole("button", { name: "删除代理 合成 SOCKS", exact: true })).toBeDisabled();
});

test("review recovery preservation: a stale workspace task cannot revive a terminal proxy owner or change binding", async ({ page }) => {
  await proxyKernelBridge(page); await openProxyKernelHarness(page);
  const row = page.getByRole("row").filter({ hasText: "合成 SOCKS" });
  await row.getByRole("button", { name: "检查", exact: true }).click();
  await expect(row).toContainText("隧道与认证");
  await page.evaluate(() => (window as unknown as { __proxyKernel: { finish(state: string, pending: boolean): void } }).__proxyKernel.finish("completed", true));
  await expect(row).toContainText("结果待保存");
  await expect(row.getByRole("button", { name: "取消检查", exact: true })).toBeDisabled();
  await page.evaluate(() => (window as unknown as { __proxyKernel: { finish(state: string, pending: boolean): void } }).__proxyKernel.finish("failed", false));
  await expect(page.getByRole("status")).toContainText("PROXY_AUTH_FAILED");
  await page.evaluate(() => {
    const operation = (window as unknown as { __proxyKernel: { view(): WorkspaceView } }).__proxyKernel.view().proxyOperations![0];
    operation.state = "running"; operation.stage = "authentication"; operation.persistencePending = false;
  });
  await page.getByRole("button", { name: "更多操作", exact: true }).click();
  await page.getByRole("menuitem", { name: "重新读取", exact: true }).click();
  await expect(row.getByRole("button", { name: "检查", exact: true })).toBeEnabled();
  await expect(row.getByRole("button", { name: "取消检查", exact: true })).toHaveCount(0);
  const environment = await page.evaluate(() => (window as unknown as { __proxyKernel: { view(): WorkspaceView } }).__proxyKernel.view().state.environments[0]);
  expect(environment).toMatchObject({ proxyId, seed: "172600001" });
});
