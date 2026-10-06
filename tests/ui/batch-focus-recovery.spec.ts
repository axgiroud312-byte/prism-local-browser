import { expect, test, type Locator, type Page } from "@playwright/test";
import type { NativeBatchPage, NativeBatchPreviewRequest, WorkspaceView } from "../../src/application/contract";
import type { NativeRequest } from "../../src/application/wails-adapter";
import { nativeReferenceBridge } from "./fixtures/native-reference-bridge";

// Actual main.tsx/App with an in-memory bridge; no controller, native I/O or
// forced focus. Extend the existing paginated workspace fixture read-only.
type BatchFixture = { unexpected: string[]; holdPreview(): void; releasePreview(outcome: "failure" | "completed"): void };
const audits = new WeakMap<Page, { errors: string[]; deniedRequests: string[] }>();
test.beforeEach(async ({ page, baseURL }) => {
  const audit = { errors: [] as string[], deniedRequests: [] as string[] }; audits.set(page, audit);
  page.on("pageerror", error => audit.errors.push(error.message));
  const origin = new URL(baseURL!).origin;
  await page.route("**/*", route => {
    if (new URL(route.request().url()).origin === origin) return route.continue();
    audit.deniedRequests.push(route.request().url()); return route.abort("blockedbyclient");
  });
  await page.routeWebSocket("**/*", socket => socket.close());
});
test.afterEach(async ({ page }, testInfo) => {
  const proof = await page.evaluate(() => {
    const fixture = window as unknown as { __referenceNative: { calls: NativeRequest[] }; __batchFocus: BatchFixture };
    const frames = [...document.querySelectorAll<HTMLElement>('[aria-modal="true"]')].filter(element => element.getClientRects().length && !element.closest("[inert]"));
    const active = document.activeElement;
    return { methodCounts: Object.fromEntries([...new Set(fixture.__referenceNative.calls.map(call => call.method))].map(method => [method, fixture.__referenceNative.calls.filter(call => call.method === method).length])), unexpected: fixture.__batchFocus.unexpected, activeTag: active?.tagName, focusInside: frames.some(frame => frame.contains(active)), effectiveFrames: frames.length, nativeIO: false };
  });
  await testInfo.attach("actual-App-focus-audit", { body: JSON.stringify({ ...proof, ...audits.get(page) }, null, 2), contentType: "application/json" });
  expect(proof.unexpected).toEqual([]);
  expect(proof.methodCounts["Batch.Commit"] ?? 0).toBe(0);
  expect(audits.get(page)).toEqual({ errors: [], deniedRequests: [] });
});

async function native(page: Page) {
  await nativeReferenceBridge(page);
  await page.addInitScript(() => {
    const fixture = (window as unknown as { __referenceNative: { calls: NativeRequest[]; view: WorkspaceView } }).__referenceNative;
    const host = (window as unknown as { go: { main: { DesktopApp: { Call(request: NativeRequest): Promise<unknown> } } } }).go.main.DesktopApp;
    const original = host.Call, view = fixture.view, unexpected: string[] = [];
    view.nativeProxyRecords = view.state.proxies.map((proxy, index) => ({ id: proxy.id, name: proxy.name, type: proxy.type, host: proxy.host, port: proxy.port, country: proxy.country, revision: 1, hasAuthentication: false, status: "unchecked", usedBy: index ? [] : view.state.environments.map(environment => environment.id), usedCount: index ? 0 : 30 }));
    let hold = false, release: ((outcome: "failure" | "completed") => void) | undefined, serial = 0;
    const ok = (data: unknown) => ({ ok: true, mode: "native", data: structuredClone(data) });
    const fail = (message: string) => ({ ok: false, mode: "native", error: { code: "STORAGE_READ_FAILED", message, retryable: true } });
    Object.assign(window, { __batchFocus: { unexpected, holdPreview() { hold = true; }, releasePreview(outcome: "failure" | "completed") { hold = false; release?.(outcome); release = undefined; } } });
    host.Call = async request => {
      // Workspace.Read is the only delegated method. Anything else not explicitly
      // implemented fails the test, rather than falling back to a desktop bridge.
      if (request.method === "Workspace.Read") return original(request);
      fixture.calls.push(structuredClone(request));
      if (request.method !== "Batch.Preview") { unexpected.push(request.method); return fail(`Unexpected synthetic call: ${request.method}`); }
      const payload = request.payload as NativeBatchPreviewRequest;
      if (payload.kind !== "assign" || payload.mappings.length !== 2 || payload.mappings.some(mapping => !view.state.environments.some(environment => environment.id === mapping.environmentId) || !view.nativeProxyRecords!.some(proxy => proxy.id === mapping.proxyId))) {
        unexpected.push("invalid assignment scope"); return fail("Invalid synthetic scope");
      }
      const outcome = hold ? await new Promise<"failure" | "completed">(resolve => { release = resolve; }) : "completed";
      if (outcome === "failure") return fail("合成预览读取失败，映射未保存，可重试。");
      const result: NativeBatchPage = { mode: "native", planId: `synthetic-focus-plan-${++serial}`, kind: "assign", total: payload.mappings.length, completedCount: 0, failedCount: 0, notExecutedCount: payload.mappings.length, attemptCompletedCount: 0, sharedProxyAssignments: payload.mappings.length, directAssignments: 0, sequence: 1, offset: 0, pageSize: 25, expiresAt: "2099-10-06T01:00:00Z", items: payload.mappings.map((mapping, index) => ({ index, environmentId: mapping.environmentId, name: view.state.environments.find(environment => environment.id === mapping.environmentId)!.name, expectedRevision: 1, proxyId: mapping.proxyId, proxyName: view.nativeProxyRecords!.find(proxy => proxy.id === mapping.proxyId)!.name, newIdentity: false, state: "not-executed" })) };
      return ok(result);
    };
  });
  await page.goto("/#/proxies");
}
const calls = (page: Page, method: string) => page.evaluate(method => (window as unknown as { __referenceNative: { calls: NativeRequest[] } }).__referenceNative.calls.filter(call => call.method === method), method);
const assignment = (page: Page) => page.getByRole("dialog", { name: "逐项分配代理", exact: true });
const previewButton = (frame: Locator) => frame.getByRole("button", { name: "查看逐项映射预览（不保存绑定）", exact: true });
const selectedIds = ["synthetic-reference-1", "synthetic-reference-12"];
async function selectAssignment(page: Page) {
  await native(page);
  await page.getByRole("button", { name: "合成 SOCKS5 已绑定环境", exact: true }).click();
  const usage = page.getByRole("dialog", { name: "已绑定环境 · 合成 SOCKS5", exact: true });
  await expect(usage).toContainText("已返回 12 / 服务共 30 项");
  await usage.getByLabel(`选择绑定环境 ${selectedIds[0]}`, { exact: true }).check();
  await usage.getByRole("button", { name: "下一页", exact: true }).click();
  await usage.getByLabel(`选择绑定环境 ${selectedIds[1]}`, { exact: true }).check();
  await usage.getByRole("button", { name: "为所选环境重新分配代理", exact: true }).click();
  const frame = assignment(page);
  await expect(usage).toHaveCount(0);
  await expect(frame.locator("tbody tr")).toHaveCount(2);
  for (const id of selectedIds) await frame.getByLabel(`为 ${id} 分配代理`, { exact: true }).selectOption("synthetic-socks");
  return frame;
}
async function focusedEnabled(frame: Locator) {
  await expect.poll(() => frame.evaluate(element => {
    const active = document.activeElement;
    return active instanceof HTMLElement && element.contains(active) && !active.matches(":disabled") && !active.closest("[inert]") && active.getClientRects().length > 0;
  })).toBe(true);
}
async function exactPlan(page: Page, frame: Locator, previews = 1) {
  await expect(frame).toContainText("计划ID：synthetic-focus-plan-1");
  await expect(frame.locator("tbody tr")).toHaveCount(2);
  for (const id of selectedIds) await expect(frame.locator("tbody")).toContainText(id);
  await expect(frame.getByLabel("确认以上明确的节点共享，不使用自动轮询映射。", { exact: true })).not.toBeChecked();
  await expect(frame.getByRole("button", { name: "确认执行整个预览计划 2 项", exact: true })).toBeDisabled();
  const requests = await calls(page, "Batch.Preview"); expect(requests).toHaveLength(previews);
  for (const request of requests) expect(request.payload).toEqual({ kind: "assign", mappings: selectedIds.map(environmentId => ({ environmentId, proxyId: "synthetic-socks" })) });
  expect(await calls(page, "Batch.Commit")).toHaveLength(0);
}

for (const viewport of [{ width: 1440, height: 900 }, { width: 1280, height: 800 }]) {
test.describe(`${viewport.width}x${viewport.height} actual App batch focus`, () => {
test.use({ viewport });

test("selected cross-page assignment keeps focus when preview removes the clicked control", async ({ page }) => {
  const frame = await selectAssignment(page);
  await previewButton(frame).click();
  await exactPlan(page, frame);
  await expect(previewButton(frame)).toHaveCount(0);
  await focusedEnabled(frame);
  expect(Math.round((await frame.boundingBox())!.width)).toBe(1040);
  await expect(page.locator('[aria-modal="true"]')).toHaveCount(1);
  await expect(page.locator(".main-shell")).toHaveJSProperty("inert", true);
  expect(await page.evaluate(() => document.body.style.overflow)).toBe("hidden");
});

test("disabled busy preview, failed retry and return to mapping retain enabled modal focus", async ({ page }) => {
  const frame = await selectAssignment(page);
  await page.evaluate(() => (window as unknown as { __batchFocus: BatchFixture }).__batchFocus.holdPreview());
  await previewButton(frame).click();
  await expect(previewButton(frame)).toBeDisabled();
  await expect(frame.getByRole("status")).toContainText("正在读取或受理");
  await focusedEnabled(frame);
  await page.evaluate(() => (window as unknown as { __batchFocus: BatchFixture }).__batchFocus.releasePreview("failure"));
  await expect(frame.getByRole("alert")).toContainText("STORAGE_READ_FAILED");
  await expect(previewButton(frame)).toBeEnabled();
  for (const id of selectedIds) await expect(frame.getByLabel(`为 ${id} 分配代理`, { exact: true })).toHaveValue("synthetic-socks");
  await focusedEnabled(frame);
  await previewButton(frame).click();
  await exactPlan(page, frame, 2); await focusedEnabled(frame);
  await frame.getByRole("button", { name: "返回修改映射（重新预览）", exact: true }).click();
  await expect(previewButton(frame)).toBeEnabled();
  for (const id of selectedIds) await expect(frame.getByLabel(`为 ${id} 分配代理`, { exact: true })).toHaveValue("synthetic-socks");
  await focusedEnabled(frame);
  expect(await calls(page, "Batch.Commit")).toHaveLength(0);
});

test("highest task result keeps keyboard focus and Escape restores its exact surviving invoker", async ({ page }) => {
  const frame = await selectAssignment(page);
  await previewButton(frame).click(); await exactPlan(page, frame);
  const trigger = frame.getByRole("button", { name: "查看批次结果", exact: true });
  await trigger.click();
  const result = page.getByRole("dialog", { name: "批次执行结果", exact: true });
  await expect(frame).toHaveCount(0);
  await expect(page.locator('[aria-modal="true"]')).toHaveCount(1);
  expect(Math.round((await result.boundingBox())!.width)).toBe(400);
  await focusedEnabled(result);
  await page.keyboard.press("Tab");
  const back = result.getByRole("button", { name: "返回逐项明细", exact: true });
  await expect(back).toBeFocused();
  for (const key of ["Shift+Tab", "Tab", "Control+k"]) { await page.keyboard.press(key); await focusedEnabled(result); }
  await expect(back).toBeFocused();
  await back.click(); await expect(result).toHaveCount(0); await expect(trigger).toBeFocused();
  await trigger.click(); await page.keyboard.press("Escape");
  await expect(result).toHaveCount(0); await expect(trigger).toBeFocused();
  await page.keyboard.press("Escape");
  await expect(page.getByRole("dialog")).toHaveCount(0);
  await expect(page.locator(".main-shell")).toHaveJSProperty("inert", false);
  expect(await page.evaluate(() => document.body.style.overflow)).toBe("");
  // The assignment's original usage footer was removed in the handoff. Check
  // restoration where the existing contract has a surviving exact invoker.
  const usageTrigger = page.getByRole("button", { name: "合成 SOCKS5 已绑定环境", exact: true });
  await usageTrigger.click(); await page.keyboard.press("Escape");
  await expect(usageTrigger).toBeFocused();
  expect(await calls(page, "Batch.Commit")).toHaveLength(0);
});
});
}
