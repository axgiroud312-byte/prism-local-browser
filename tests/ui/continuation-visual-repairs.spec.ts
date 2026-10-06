import { expect, test, type Locator, type Page } from "@playwright/test";
import { STORAGE_KEY } from "../../src/domain";
import type { Operation, ProxyCheckReport, WorkspaceView } from "../../src/application/contract";
import type { NativeRequest } from "../../src/application/wails-adapter";
import { proxyKernelBridge } from "./fixtures/proxy-kernel";
import { referenceWorkspace } from "./fixtures/reference-workspace";

// Production main.tsx/App only; fixtures author finite synthetic bridge replies,
// not application/controller state. No proxy checks, desktop I/O or source transforms.
const audits = new WeakMap<Page, { errors: string[]; deniedRequests: string[]; observations: unknown[] }>();
test.beforeEach(async ({ page, baseURL }) => {
  const audit = { errors: [] as string[], deniedRequests: [] as string[], observations: [] as unknown[] }; audits.set(page, audit);
  page.on("pageerror", error => audit.errors.push(error.message));
  const origin = new URL(baseURL!).origin;
  await page.route("**/*", route => {
    if (new URL(route.request().url()).origin === origin) return route.continue();
    audit.deniedRequests.push(route.request().url()); return route.abort("blockedbyclient");
  });
  await page.routeWebSocket("**/*", socket => socket.close());
});
test.afterEach(async ({ page }, testInfo) => {
  const bridge = await page.evaluate(() => {
    const fixture = (window as unknown as { __visualNative?: { calls: NativeRequest[]; unexpected: string[] } }).__visualNative;
    return { calls: fixture?.calls ?? [], unexpected: fixture?.unexpected ?? [], nativeIO: false };
  });
  await testInfo.attach("actual-App-visual-repair-audit", { body: JSON.stringify({ ...bridge, ...audits.get(page) }, null, 2), contentType: "application/json" });
  expect(bridge.unexpected).toEqual([]);
  expect(bridge.calls.every(call => ["Workspace.Read", "Operation.Read"].includes(call.method))).toBe(true);
  expect(audits.get(page)?.errors).toEqual([]); expect(audits.get(page)?.deniedRequests).toEqual([]);
});

const proxyCases = [
  { kind: "stale", label: "暂无当前修订有效检查结果", hiddenIp: true },
  { kind: "wrong-proxy", label: "暂无当前修订有效检查结果", hiddenIp: true },
  { kind: "missing", label: "暂无当前修订有效检查结果", hiddenIp: true },
  { kind: "empty-steps", label: "暂无当前修订有效检查结果", hiddenIp: true },
  { kind: "unfinished", label: "检查尚未完成", hiddenIp: true },
  { kind: "success", label: "本次检查通过", hiddenIp: false },
  { kind: "failed", label: "检查未通过", hiddenIp: true },
  { kind: "unchecked", label: "尚未检查", hiddenIp: true },
  { kind: "running", label: "隧道与认证", hiddenIp: true },
  { kind: "pending", label: "结果待保存，尚非持久终态", hiddenIp: false },
  { kind: "running-no-report", label: "隧道与认证", hiddenIp: true },
  { kind: "pending-no-report", label: "结果待保存，尚非持久终态", hiddenIp: true },
  { kind: "future-stage", label: "synthetic-future-check-stage", hiddenIp: true },
] as const;
type ProxyCase = typeof proxyCases[number]["kind"];
async function native(page: Page, kind: ProxyCase | "loading") {
  await proxyKernelBridge(page);
  await page.addInitScript(kind => {
    const fixture = (window as unknown as { __proxyKernel: { view(): WorkspaceView } }).__proxyKernel;
    const host = (window as unknown as { go: { main: { DesktopApp: { Call(request: NativeRequest): Promise<unknown> } } } }).go.main.DesktopApp;
    const original = host.Call, view = fixture.view(), record = view.nativeProxyRecords![0];
    const calls: NativeRequest[] = [], unexpected: string[] = [], releases: (() => void)[] = [];
    let loading = kind === "loading";
    record.status = kind === "unchecked" ? "unchecked" : kind === "failed" ? "failed" : "connected";
    const report: ProxyCheckReport = { mode: "native", adapterVersion: "synthetic-only/no-network", proxyId: record.id, revision: record.revision, targetOrigin: "https://example.invalid", startedAt: "2026-10-06T01:00:00Z", finishedAt: "2026-10-06T01:00:01Z", durationMs: 36, exitIp: "203.0.113.81", steps: [{ stage: "connection", status: "running", time: "2026-10-06T01:00:00Z", message: "合成历史进度，不是实时网络。" }, { stage: "connection", status: "passed", time: "2026-10-06T01:00:01Z", message: "合成连接结果。" }, { stage: "exit", status: "passed", time: "2026-10-06T01:00:01Z", message: "合成完成结果，不提供环境启动许可。" }] };
    if (kind === "stale") report.revision--;
    if (kind === "wrong-proxy") report.proxyId = "synthetic-unrelated-proxy";
    if (kind === "failed") { report.exitIp = undefined; report.error = { code: "PROXY_AUTH_FAILED", message: "合成认证失败，不改为直连。", retryable: true }; report.steps = [{ ...report.steps[0], status: "failed" }]; }
    if (kind === "empty-steps") { report.steps = []; report.exitIp = undefined; }
    if (kind === "unfinished" || kind === "running") { report.finishedAt = ""; report.exitIp = undefined; report.steps = [{ ...report.steps[0], stage: "authentication" }]; }
    const taskKind = ["running", "pending", "running-no-report", "pending-no-report", "future-stage"].includes(kind);
    const noReport = ["missing", "unchecked", "running-no-report", "pending-no-report", "future-stage"].includes(kind);
    record.checkReport = taskKind || noReport ? undefined : report;
    const task: Operation | undefined = taskKind ? { id: "synthetic-visual-check", kind: "proxy-check", proxyId: record.id, state: kind.startsWith("pending") ? "completed" : "running", stage: kind.startsWith("pending") ? "storage-pending" : kind === "future-stage" ? "synthetic-future-check-stage" : "authentication", total: 1, completedIds: kind.startsWith("pending") ? [record.id] : [], cancelRequested: false, persistencePending: kind.startsWith("pending"), proxyReport: noReport ? undefined : report } : undefined;
    Object.assign(window, { __visualNative: { calls, unexpected, releaseWorkspace() { loading = false; for (const release of releases.splice(0)) release(); } } });
    host.Call = async request => {
      calls.push(structuredClone(request));
      if (request.method === "Workspace.Read") {
        if (loading) await new Promise<void>(resolve => releases.push(resolve));
        const response = await original(request) as { ok: boolean; data: WorkspaceView };
        if (response.ok) response.data.proxyOperations = task ? [structuredClone(task)] : [];
        return response;
      }
      if (request.method === "Operation.Read" && task && (request.payload as { operationId: string }).operationId === task.id) return { ok: true, mode: "native", data: structuredClone(task) };
      unexpected.push(request.method); return { ok: false, mode: "native", error: { code: "CAPABILITY_UNSUPPORTED", message: "Unexpected synthetic call", retryable: false } };
    };
  }, kind);
  await page.goto("/#/proxies");
}
async function demo(page: Page, route = "environments") {
  await page.addInitScript(({ state, key }) => localStorage.setItem(key, JSON.stringify(state)), { state: referenceWorkspace(), key: STORAGE_KEY });
  await page.goto(`/#/${route}`);
}
async function colors(page: Page, control: Locator) {
  const result = await control.evaluate(element => { const style = getComputedStyle(element); return { background: style.backgroundColor, border: style.borderTopColor, color: style.color, opacity: style.opacity }; });
  audits.get(page)!.observations.push({ text: await control.innerText(), ...result }); return result;
}

for (const viewport of [{ width: 1440, height: 900 }, { width: 1280, height: 800 }]) {
test.describe(`${viewport.width}x${viewport.height} actual App visual repairs`, () => {
test.use({ viewport });

for (const scenario of proxyCases) {
test(`native proxy detail projects ${scenario.kind} without a false current success`, async ({ page }) => {
  await native(page, scenario.kind);
  await page.getByRole("row").filter({ hasText: "合成 SOCKS" }).locator("td").nth(3).getByRole("button").click();
  const detail = page.getByRole("dialog", { name: "代理检查详情 · 合成 SOCKS", exact: true });
  const status = detail.locator(".reference-modal-body > p").first();
  await expect(status).toHaveText(`修订 1 · ${scenario.label}`);
  if (scenario.kind !== "success") await expect(status).not.toContainText("本次检查通过");
  if (scenario.hiddenIp) await expect(detail).not.toContainText("203.0.113.81"); else await expect(detail).toContainText("203.0.113.81");
  if (["stale", "wrong-proxy", "missing", "unchecked", "running-no-report", "pending-no-report", "future-stage"].includes(scenario.kind)) await expect(detail.locator("tbody tr")).toHaveCount(0);
  if (scenario.kind === "failed") await expect(detail).toContainText("PROXY_AUTH_FAILED");
  await expect(detail).toContainText("不是环境启动许可");
  audits.get(page)!.observations.push({ scenario: scenario.kind, status: await status.innerText(), text: await detail.innerText() });
  const record = await page.evaluate(() => (window as unknown as { __proxyKernel: { view(): WorkspaceView } }).__proxyKernel.view().nativeProxyRecords![0]);
  expect(record.status).toBe(scenario.kind === "unchecked" ? "unchecked" : scenario.kind === "failed" ? "failed" : "connected");
  await page.keyboard.press("Escape"); await expect(detail).toHaveCount(0);
});
}

test("changed demo workspace has an enabled blue and white reload action", async ({ page }) => {
  await demo(page, "groups");
  await expect(page.getByLabel("搜索分组名称", { exact: true })).toBeVisible();
  const changed = await page.evaluate(key => {
    const oldValue = localStorage.getItem(key)!, state = JSON.parse(oldValue); state.environments[0].note = "合成其他页新记录";
    const newValue = JSON.stringify(state); localStorage.setItem(key, newValue);
    window.dispatchEvent(new StorageEvent("storage", { key, oldValue, newValue, storageArea: localStorage })); return newValue;
  }, STORAGE_KEY);
  const blocker = page.getByRole("alertdialog", { name: "工作区需要处理", exact: true });
  await expect(blocker).toContainText("另一个页面已更新工作区");
  const reload = blocker.getByRole("button", { name: "重新载入", exact: true });
  await expect(reload).toBeEnabled();
  expect(await colors(page, reload)).toEqual({ background: "rgb(47, 84, 235)", border: "rgb(47, 84, 235)", color: "rgb(255, 255, 255)", opacity: "1" });
  await page.keyboard.press("Escape"); await expect(blocker).toBeVisible();
  expect(await page.evaluate(key => localStorage.getItem(key), STORAGE_KEY)).toBe(changed);
});

test("loading native workspace retains its disabled recovery guard", async ({ page }) => {
  await native(page, "loading");
  const blocker = page.getByRole("alertdialog", { name: "工作区需要处理", exact: true });
  await expect(blocker).toContainText("正在读取本机工作区");
  const reload = blocker.getByRole("button", { name: "重新读取本机工作区", exact: true });
  await expect(reload).toBeDisabled();
  expect((await colors(page, reload)).opacity).toBe("0.46");
  await page.keyboard.press("Escape"); await expect(blocker).toBeVisible(); await expect(reload).toBeDisabled();
  await page.evaluate(() => (window as unknown as { __visualNative: { releaseWorkspace(): void } }).__visualNative.releaseWorkspace());
  await expect(blocker).toHaveCount(0); await expect(page.getByRole("button", { name: "添加代理", exact: true })).toBeEnabled();
});

test("existing neutral footer cancel stays gray and dangerous confirmation stays red", async ({ page }) => {
  await demo(page);
  await page.getByLabel("选择 工作环境 A", { exact: true }).check();
  await page.getByRole("button", { name: "调整分组", exact: true }).click();
  const group = page.getByRole("dialog", { name: "调整环境分组", exact: true });
  const cancel = group.getByRole("button", { name: "取消", exact: true });
  expect(await colors(page, cancel)).toEqual({ background: "rgb(238, 238, 238)", border: "rgb(238, 238, 238)", color: "rgb(89, 89, 89)", opacity: "1" });
  await cancel.click();
  await page.getByRole("link", { name: "代理管理", exact: true }).click();
  await page.getByRole("button", { name: "删除代理 合成 HTTP", exact: true }).click();
  const deletion = page.getByRole("dialog", { name: "删除代理确认", exact: true });
  const danger = deletion.getByRole("button", { name: "确认删除", exact: true });
  await expect(danger).toBeEnabled();
  expect(await colors(page, danger)).toEqual({ background: "rgb(245, 34, 45)", border: "rgb(245, 34, 45)", color: "rgb(255, 255, 255)", opacity: "1" });
  await page.keyboard.press("Escape"); await expect(deletion).toHaveCount(0);
  expect(await page.evaluate(key => JSON.parse(localStorage.getItem(key)!).proxies.length, STORAGE_KEY)).toBe(2);
});
});
}
