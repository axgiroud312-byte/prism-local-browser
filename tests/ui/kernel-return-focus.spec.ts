import { expect, test, type Locator, type Page } from "@playwright/test";
import type { KernelInstallRequest, Operation, WorkspaceView } from "../../src/application/contract";
import type { NativeRequest } from "../../src/application/wails-adapter";
import { proxyKernelBridge } from "./fixtures/proxy-kernel";

const VERSION = "151.0.9000.11", CHECKSUM = "a".repeat(64), TASK_ID = "synthetic-return-focus-task";
type Scenario = "history-completed" | "history-failed" | "install-completed" | "install-busy-failed" | "delete-completed" | "verify-running" | "workspace-blocker" | "generic";
interface FocusFixture {
  calls: NativeRequest[]; unexpected: string[];
  releaseInstall(): void; recoverWorkspace(): void;
}
type FocusWindow = Window & { __kernelReturnFocus: FocusFixture };
const audits = new WeakMap<Page, { errors: string[]; denied: string[] }>();

// Real App/manager/lifecycle, finite in-memory bridge replies only. No native I/O.
async function native(page: Page, scenario: Scenario) {
  await proxyKernelBridge(page);
  await page.addInitScript(({ scenario, version, checksum, taskId }) => {
    const host = (window as unknown as { go: { main: { DesktopApp: { Call(request: NativeRequest): Promise<unknown> } } } }).go.main.DesktopApp;
    const stock = (window as unknown as { __proxyKernel: { view(): WorkspaceView } }).__proxyKernel;
    const calls: NativeRequest[] = [], unexpected: string[] = [];
    let fault = false, removed = false, release: (() => void) | undefined;
    const failed = scenario === "history-failed" || scenario === "install-busy-failed";
    const operation = (kind: Operation["kind"] = "kernel-install", kernelId?: string): Operation => ({ id: taskId, kind, kernelId, state: failed ? "failed" : scenario === "verify-running" ? "running" : "completed", stage: failed ? "failed" : scenario === "verify-running" ? "probing" : "completed", total: 1, completedIds: failed || scenario === "verify-running" ? [] : [kernelId ?? "synthetic-return-focus-build"], cancelRequested: false, ...(failed ? { error: { code: "KERNEL_INTEGRITY_FAILED", message: "合成失败，未执行内核", retryable: true } } : {}) });
    let current = scenario.startsWith("history-") ? operation() : undefined;
    const ok = (data: unknown) => ({ ok: true, mode: "native", data: structuredClone(data) });
    const fail = (code: string, message: string) => ({ ok: false, mode: "native", error: { code, message, retryable: true } });
    Object.assign(window, { __kernelReturnFocus: { calls, unexpected, releaseInstall() { release?.(); }, recoverWorkspace() { fault = false; } } });
    host.Call = async request => {
      calls.push(structuredClone(request));
      const p = request.payload as Record<string, unknown>;
      if (request.mode !== "native") { unexpected.push("non-native request"); return fail("CAPABILITY_UNSUPPORTED", "Unexpected mode"); }
      if (request.method === "Workspace.Read") {
        if (fault) return fail("STORAGE_READ_FAILED", "合成读取故障，保留内核返回位置");
        const view = structuredClone(stock.view());
        view.kernelOperations = current ? [current] : [];
        if (removed) { view.kernelRecords = view.kernelRecords!.filter(record => record.id !== "synthetic-build-b"); view.state.kernels = view.state.kernels.filter(record => record.id !== "synthetic-build-b"); }
        return ok(view);
      }
      if (request.method === "Kernel.Install" && ["install-completed", "install-busy-failed", "workspace-blocker"].includes(scenario)) {
        const expected: KernelInstallRequest = { source: "official", version, expectedChecksum: checksum, trusted: false, requestId: String(p.requestId) };
        if (!/^[a-f0-9-]{36}$/.test(expected.requestId) || JSON.stringify(p) !== JSON.stringify(expected)) { unexpected.push("Kernel.Install identity"); return fail("VALIDATION_FAILED", "Unexpected synthetic install"); }
        if (scenario === "install-busy-failed") await new Promise<void>(resolve => { release = resolve; });
        current = operation(); fault = scenario === "workspace-blocker";
        return ok({ status: "accepted", operation: current });
      }
      if ((request.method === "Kernel.Delete" && scenario === "delete-completed" || request.method === "Kernel.Verify" && scenario === "verify-running") && p.kernelId === "synthetic-build-b" && /^[a-f0-9-]{36}$/.test(String(p.requestId)) && Object.keys(p).sort().join(",") === "kernelId,requestId") {
        removed = request.method === "Kernel.Delete";
        current = operation(removed ? "kernel-delete" : "kernel-verify", "synthetic-build-b");
        return ok({ status: "accepted", operation: current });
      }
      if (request.method === "Operation.Read" && current && p.operationId === taskId && Object.keys(p).length === 1) return ok(current);
      unexpected.push(request.method); return fail("CAPABILITY_UNSUPPORTED", "Unexpected synthetic call");
    };
  }, { scenario, version: VERSION, checksum: CHECKSUM, taskId: TASK_ID });
  await page.goto(scenario === "generic" ? "/#/proxies" : "/#/kernels");
}

test.beforeEach(async ({ page, baseURL }) => {
  const audit = { errors: [] as string[], denied: [] as string[] }; audits.set(page, audit);
  page.on("pageerror", error => audit.errors.push(error.message));
  const origin = new URL(baseURL!).origin;
  await page.route("**/*", route => { if (new URL(route.request().url()).origin === origin) return route.continue(); audit.denied.push(route.request().url()); return route.abort("blockedbyclient"); });
  await page.routeWebSocket("**/*", socket => socket.close());
});
test.afterEach(async ({ page }, testInfo) => {
  const fixture = await page.evaluate(() => {
    const { calls, unexpected } = (window as unknown as FocusWindow).__kernelReturnFocus;
    const active = document.activeElement as HTMLElement | null;
    return { calls, unexpected, activeTag: active?.tagName, activeText: active?.textContent, nativeIO: false };
  });
  await testInfo.attach("synthetic-kernel-return-focus-audit", { body: JSON.stringify({ ...fixture, ...audits.get(page) }, null, 2), contentType: "application/json" });
  expect(fixture.unexpected).toEqual([]);
  expect(fixture.calls.filter(call => call.method === "Operation.Cancel")).toHaveLength(0);
  expect(audits.get(page)?.errors).toEqual([]); expect(audits.get(page)?.denied).toEqual([]);
});

const task = (page: Page) => page.getByRole("dialog", { name: "内核任务", exact: true });
const historyButton = (page: Page) => page.getByRole("button", { name: "最近内核任务", exact: true });
async function focusedWithin(frame: Locator) { await expect.poll(() => frame.evaluate(element => element.contains(document.activeElement) && !document.activeElement?.closest("[inert]") && !document.activeElement?.matches(":disabled"))).toBe(true); }
async function commandCounts(page: Page, expected: { install?: number; verify?: number; delete?: number } = {}) {
  const calls = await page.evaluate(() => (window as unknown as FocusWindow).__kernelReturnFocus.calls);
  expect(calls.filter(call => call.method === "Kernel.Install")).toHaveLength(expected.install ?? 0);
  expect(calls.filter(call => call.method === "Kernel.Verify")).toHaveLength(expected.verify ?? 0);
  expect(calls.filter(call => call.method === "Kernel.Delete")).toHaveLength(expected.delete ?? 0);
  expect(calls.filter(call => call.method === "Operation.Cancel")).toHaveLength(0);
}
async function historyTask(page: Page) {
  await historyButton(page).click();
  const history = page.getByRole("dialog", { name: "最近内核任务（持久记录）", exact: true });
  await focusedWithin(history);
  await history.getByRole("row").filter({ hasText: TASK_ID }).getByRole("button", { name: "查看任务", exact: true }).click();
  await expect(history).toHaveCount(0); await expect(task(page)).toContainText(TASK_ID); await focusedWithin(task(page));
}
async function prepareForm(page: Page) {
  await page.getByRole("button", { name: "准备精确内核", exact: true }).click();
  const prepare = page.getByRole("dialog", { name: "安装精确内核", exact: true });
  await prepare.getByLabel("精确发行版本", { exact: true }).fill(VERSION);
  await prepare.getByLabel("预期归档 SHA-256", { exact: true }).fill(CHECKSUM);
  return prepare;
}

test("completed history → task → Escape returns the original page history button, not BODY", async ({ page }) => {
  await native(page, "history-completed"); await historyTask(page);
  await expect(task(page)).toContainText("已完成 1");
  await page.keyboard.press("Escape"); await expect(task(page)).toHaveCount(0);
  await expect(historyButton(page)).toBeFocused();
  expect(await page.evaluate(() => document.activeElement?.tagName)).toBe("BUTTON");
  await expect(page.locator(".main-shell")).toHaveJSProperty("inert", false);
  expect(await page.evaluate(() => document.body.style.overflow)).toBe("");
  await commandCounts(page);
});

test("failed history → task → reprepare keeps the history return target without submitting", async ({ page }) => {
  await native(page, "history-failed"); await historyTask(page);
  await expect(task(page)).toContainText("KERNEL_INTEGRITY_FAILED");
  await task(page).getByRole("button", { name: "重新准备", exact: true }).click();
  const prepare = page.getByRole("dialog", { name: "安装精确内核", exact: true });
  await expect(task(page)).toHaveCount(0); await focusedWithin(prepare);
  await prepare.getByLabel("精确发行版本", { exact: true }).fill(VERSION);
  await page.keyboard.press("Escape"); await expect(prepare).toHaveCount(0);
  await expect(historyButton(page)).toBeFocused();
  await commandCounts(page);
});

test("mock install → completed task returns prepare, and hiding retains the exact draft", async ({ page }) => {
  await native(page, "install-completed"); const prepare = await prepareForm(page);
  await prepare.getByRole("button", { name: "安装并核验", exact: true }).click();
  await expect(prepare).toHaveCount(0); await expect(task(page)).toContainText(TASK_ID); await focusedWithin(task(page));
  await page.keyboard.press("Escape"); await expect(task(page)).toHaveCount(0);
  await expect(page.getByRole("button", { name: "准备精确内核", exact: true })).toBeFocused();
  await page.getByRole("button", { name: "准备精确内核", exact: true }).click();
  await expect(prepare.getByLabel("精确发行版本", { exact: true })).toHaveValue(VERSION);
  await expect(prepare.getByLabel("预期归档 SHA-256", { exact: true })).toHaveValue(CHECKSUM);
  await prepare.getByRole("button", { name: "取消", exact: true }).click();
  await expect(page.getByRole("button", { name: "准备精确内核", exact: true })).toBeFocused();
  await commandCounts(page, { install: 1 });
});

test("mock delete → task uses the named page history fallback when its row trigger is removed", async ({ page }) => {
  await native(page, "delete-completed");
  const remove = page.getByRole("button", { name: `移除此构建 ${VERSION}`, exact: true });
  await remove.click(); const confirm = page.getByRole("dialog", { name: "移除内核确认", exact: true });
  await focusedWithin(confirm); await confirm.getByRole("button", { name: "确认移除", exact: true }).click();
  await expect(confirm).toHaveCount(0); await expect(remove).toHaveCount(0);
  await expect(task(page)).toContainText(TASK_ID); await focusedWithin(task(page));
  await page.keyboard.press("Escape"); await expect(task(page)).toHaveCount(0);
  await expect(historyButton(page)).toBeFocused();
  await expect(page.getByRole("row").filter({ hasText: "synthetic-build-a" })).toBeVisible();
  await commandCounts(page, { delete: 1 });
});

test("mock running verify never returns to its disabled trigger and reopening chooses a new page target", async ({ page }) => {
  await native(page, "verify-running");
  const verify = page.getByRole("row").filter({ hasText: "synthetic-build-b" }).getByRole("button", { name: "重新核验", exact: true });
  await verify.click(); await expect(task(page)).toContainText(TASK_ID); await focusedWithin(task(page));
  await expect(verify).toBeDisabled();
  await page.keyboard.press("Escape"); await expect(task(page)).toHaveCount(0);
  await expect(historyButton(page)).toBeFocused(); await expect(verify).not.toBeFocused();
  await expect(page.getByRole("button", { name: "准备精确内核", exact: true })).toBeDisabled();
  const opener = page.getByRole("button", { name: "查看内核任务 · 隔离探测实际身份和参数", exact: true });
  await opener.click(); await expect(task(page)).toContainText(TASK_ID);
  await page.keyboard.press("Escape"); await expect(task(page)).toHaveCount(0); await expect(opener).toBeFocused();
  await commandCounts(page, { verify: 1 });
});

test("busy mock install retains its trap and locks; failed task → reprepare retains the original prepare target", async ({ page }) => {
  await native(page, "install-busy-failed"); const prepare = await prepareForm(page);
  await prepare.getByRole("button", { name: "安装并核验", exact: true }).click();
  await expect(prepare.getByRole("button", { name: "取消", exact: true })).toBeDisabled();
  for (const key of ["Escape", "Tab", "Shift+Tab", "Control+k"]) {
    await page.keyboard.press(key); await expect(prepare).toBeVisible(); await focusedWithin(prepare);
    expect(await page.evaluate(() => document.body.style.overflow)).toBe("hidden");
    await expect(page.locator(".main-shell")).toHaveJSProperty("inert", true);
  }
  await commandCounts(page, { install: 1 });
  await page.evaluate(() => (window as unknown as FocusWindow).__kernelReturnFocus.releaseInstall());
  await expect(task(page)).toContainText("KERNEL_INTEGRITY_FAILED"); await focusedWithin(task(page));
  await task(page).getByRole("button", { name: "重新准备", exact: true }).click();
  await expect(task(page)).toHaveCount(0); await focusedWithin(prepare);
  await expect(prepare.getByLabel("精确发行版本", { exact: true })).toHaveValue(VERSION);
  await expect(prepare.getByLabel("预期归档 SHA-256", { exact: true })).toHaveValue(CHECKSUM);
  await page.keyboard.press("Escape"); await expect(prepare).toHaveCount(0);
  await expect(page.getByRole("button", { name: "准备精确内核", exact: true })).toBeFocused();
  expect(await page.evaluate(() => document.body.style.overflow)).toBe("");
  await expect(page.locator(".main-shell")).toHaveJSProperty("inert", false);
  await commandCounts(page, { install: 1 });
});

test("a direct page task → reprepare chain returns that chosen task opener, not a previous history target", async ({ page }) => {
  await native(page, "history-failed"); await historyTask(page);
  await page.keyboard.press("Escape"); await expect(task(page)).toHaveCount(0); await expect(historyButton(page)).toBeFocused();
  const opener = page.getByRole("button", { name: "查看内核任务 · 失败，查看具体原因", exact: true });
  await opener.click(); await task(page).getByRole("button", { name: "重新准备", exact: true }).click();
  const prepare = page.getByRole("dialog", { name: "安装精确内核", exact: true });
  await expect(task(page)).toHaveCount(0); await focusedWithin(prepare);
  await page.keyboard.press("Escape"); await expect(prepare).toHaveCount(0); await expect(opener).toBeFocused();
  await expect(historyButton(page)).not.toBeFocused();
  await commandCounts(page);
});

test("a higher workspace blocker owns keys and focus until gone, then the task returns its original page target", async ({ page }) => {
  await native(page, "workspace-blocker"); const prepare = await prepareForm(page);
  await prepare.getByRole("button", { name: "安装并核验", exact: true }).click();
  const blocker = page.getByRole("alertdialog", { name: "工作区需要处理", exact: true });
  const lowerTask = page.getByRole("dialog", { name: "内核任务", exact: true, includeHidden: true });
  await expect(blocker).toContainText("合成读取故障"); await expect(lowerTask).toContainText(TASK_ID); await focusedWithin(blocker);
  expect(await lowerTask.evaluate(element => element.closest("[inert]") !== null)).toBe(true);
  for (const key of ["Tab", "Shift+Tab", "Escape", "Control+k"]) {
    await page.keyboard.press(key); await focusedWithin(blocker); await expect(lowerTask).toBeAttached();
    expect(await page.evaluate(() => document.body.style.overflow)).toBe("hidden");
  }
  const recover = blocker.getByRole("button", { name: "重新读取本机工作区", exact: true });
  await recover.evaluate(element => element.addEventListener("click", () => (window as unknown as FocusWindow).__kernelReturnFocus.recoverWorkspace(), { capture: true, once: true }));
  await recover.click(); await expect(blocker).toHaveCount(0); await expect(task(page)).toContainText(TASK_ID); await focusedWithin(task(page));
  expect(await task(page).evaluate(element => element.closest("[inert]") !== null)).toBe(false);
  expect(await page.evaluate(() => document.body.style.overflow)).toBe("hidden");
  await page.keyboard.press("Escape"); await expect(task(page)).toHaveCount(0);
  await expect(page.getByRole("button", { name: "准备精确内核", exact: true })).toBeFocused();
  await expect(page.locator(".main-shell")).toHaveJSProperty("inert", false);
  expect(await page.evaluate(() => document.body.style.overflow)).toBe("");
  await commandCounts(page, { install: 1 });
});

test("generic proxy modal without returnFocus still returns its own page trigger and releases its locks", async ({ page }) => {
  await native(page, "generic");
  const opener = page.getByRole("button", { name: "编辑代理 合成 SOCKS", exact: true });
  await opener.click(); const edit = page.getByRole("dialog", { name: "修改代理", exact: true });
  await focusedWithin(edit); await edit.getByLabel("名称", { exact: true }).fill("合成未提交名称");
  await page.keyboard.press("Tab"); await focusedWithin(edit);
  await page.keyboard.press("Escape"); await expect(edit).toHaveCount(0); await expect(opener).toBeFocused();
  await expect(page.locator(".sidebar")).toHaveJSProperty("inert", false);
  await expect(page.locator(".main-shell")).toHaveJSProperty("inert", false);
  expect(await page.evaluate(() => document.body.style.overflow)).toBe("");
  await commandCounts(page);
});
