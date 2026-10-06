import { expect, test, type Locator, type Page, type TestInfo } from "@playwright/test";
import { STORAGE_KEY } from "../../src/domain";
import type { KernelInstallRequest, NativeEnvironmentQuery, Operation, WorkspaceView } from "../../src/application/contract";
import type { NativeRequest } from "../../src/application/wails-adapter";
import { proxyKernelBridge } from "./fixtures/proxy-kernel";
import { referenceWorkspace } from "./fixtures/reference-workspace";

const VERSION = "151.0.9000.11", CHECKSUM = "a".repeat(64), TASK_ID = "synthetic-kernel-visual-task";
type Scenario = "dense" | "proxy-dense" | "official-unknown" | "malformed" | "local-unknown" | "pending-completed" | "pending-storage" | "workspace-fault" | "loading";
interface Fixture {
  calls: NativeRequest[]; unexpected: string[]; timeline: unknown[];
  recoverWorkspace(): void; confirmPersistence(): void; releaseLoading(): void;
}
type TestWindow = Window & { __kernelVisualFixture: Fixture; __proxyKernel: { view(): WorkspaceView } };
const audits = new WeakMap<Page, { errors: string[]; deniedRequests: string[]; observations: unknown[] }>();

// Actual production main.tsx/App, with finite in-memory service replies only.
// The observer below only reads DOM ownership; it never supplies inert/focus or UI state.
async function native(page: Page, scenario: Scenario) {
  await proxyKernelBridge(page);
  await page.addInitScript(({ scenario, version, checksum, taskId }) => {
    const host = (window as unknown as { go: { main: { DesktopApp: { Call(request: NativeRequest): Promise<unknown> } } } }).go.main.DesktopApp;
    const original = host.Call, stock = (window as unknown as TestWindow).__proxyKernel;
    const calls: NativeRequest[] = [], unexpected: string[] = [], timeline: unknown[] = [], releases: (() => void)[] = [];
    let admitted = false, fault = false, loading = scenario === "loading", durable = false, firstRequest: KernelInstallRequest | undefined;
    const pending = scenario.startsWith("pending-");
    const operation = (): Operation => ({ id: taskId, kind: "kernel-install", state: pending ? "completed" : "running", stage: pending ? scenario === "pending-storage" && !durable ? "storage-pending" : "completed" : "probing", total: 1, completedIds: pending && durable ? ["synthetic-build-b"] : [], cancelRequested: false, persistencePending: pending && !durable });
    const ok = (data: unknown) => ({ ok: true, mode: "native", data: structuredClone(data) });
    const fail = (code: string, message: string) => ({ ok: false, mode: "native", error: { code, message, retryable: true } });
    Object.assign(window, { __kernelVisualFixture: { calls, unexpected, timeline, recoverWorkspace() { fault = false; }, confirmPersistence() { durable = true; }, releaseLoading() { loading = false; releases.splice(0).forEach(release => release()); } } });
    host.Call = async request => {
      calls.push(structuredClone(request));
      const p = request.payload as KernelInstallRequest;
      if (request.mode !== "native") { unexpected.push("non-native mode"); return fail("CAPABILITY_UNSUPPORTED", "Unexpected mode"); }
      const query = (request.payload as { environmentQuery?: NativeEnvironmentQuery }).environmentQuery;
      const knownWorkspaceRead = Object.keys(p).length === 0 || Object.keys(p).length === 1 && query && Object.keys(query).sort().join(",") === "group,page,pageSize,search,status" && query.page === 1 && [10, 25].includes(query.pageSize) && query.search === "" && query.group === "" && query.status === "all";
      if (request.method === "Workspace.Read" && knownWorkspaceRead) {
        if (loading) await new Promise<void>(resolve => releases.push(resolve));
        if (fault) return fail("STORAGE_READ_FAILED", "合成受理后工作区读取失败，保留原任务。");
        const response = await original(request) as { ok: boolean; data: WorkspaceView };
        if (response.ok) {
          if (scenario === "dense") {
            const templates = response.data.kernelRecords!;
            response.data.kernelRecords = Array.from({ length: 12 }, (_, index) => ({ ...structuredClone(templates[index % 2]), id: index < 2 ? templates[index].id : `synthetic-dense-build-${index + 1}`, version: index < 2 ? templates[index].version : `151.0.9000.${index + 11}`, usedBy: index ? [] : templates[0].usedBy }));
            response.data.state.kernels = response.data.kernelRecords.map(record => ({ id: record.id, version: record.version, available: true, source: "fingerprint-chromium", note: "合成分页记录，不是真实安装。" }));
          }
          if (scenario === "proxy-dense") response.data.nativeProxyRecords = Array.from({ length: 14 }, (_, index) => ({ ...structuredClone(response.data.nativeProxyRecords![index % 2]), id: `synthetic-dense-proxy-${index + 1}`, name: `合成分页代理 ${index + 1}`, hasAuthentication: false, status: "unchecked", usedBy: [], usedCount: 0 }));
          response.data.kernelOperations = admitted ? [operation()] : [];
        }
        return response;
      }
      if (request.method === "Kernel.SelectArchive" && scenario === "local-unknown" && Object.keys(p).length === 0) return ok({ status: "selected", archiveToken: "synthetic-visual-archive", name: "synthetic-visual-kernel.zip" });
      if (request.method === "Kernel.Install" && !["dense", "proxy-dense", "loading"].includes(scenario)) {
        const local = scenario === "local-unknown";
        const expected = { source: local ? "local" : "official", version, expectedChecksum: checksum, trusted: local, requestId: p.requestId, ...(local ? { archiveToken: "synthetic-visual-archive" } : {}) };
        if (!/^[a-f0-9-]{36}$/.test(p.requestId) || JSON.stringify(p) !== JSON.stringify({ source: expected.source, version, expectedChecksum: checksum, ...(local ? { archiveToken: expected.archiveToken } : {}), trusted: local, requestId: p.requestId }) || firstRequest && JSON.stringify(firstRequest) !== JSON.stringify(p)) {
          unexpected.push("Kernel.Install payload/request identity"); return fail("REQUEST_ID_REUSED", "Unexpected request identity");
        }
        const first = !firstRequest; firstRequest = structuredClone(p);
        if (first && ["official-unknown", "local-unknown"].includes(scenario)) return fail("NATIVE_UNAVAILABLE", "合成回执未知，没有推断安装成功。");
        if (first && scenario === "malformed") return ok({ status: "accepted", operation: { ...operation(), kind: "kernel-verify" } });
        admitted = true; fault = scenario === "workspace-fault";
        return ok({ status: "accepted", operation: operation() });
      }
      if (request.method === "Operation.Read" && admitted && Object.keys(p).length === 1 && (request.payload as { operationId: string }).operationId === taskId) return ok(operation());
      unexpected.push(request.method); return fail("CAPABILITY_UNSUPPORTED", "Unexpected synthetic call");
    };
    document.addEventListener("DOMContentLoaded", () => {
      let previous = "";
      const observer = new MutationObserver(records => {
        const frames = [...document.querySelectorAll<HTMLElement>('[aria-modal="true"]')].map(frame => {
          const chain = [];
          for (let node: HTMLElement | null = frame; node; node = node.parentElement) chain.push({ tag: node.tagName, className: node.className, inert: node.inert, zIndex: getComputedStyle(node).zIndex });
          return { title: frame.querySelector("h2")?.textContent, frameInert: frame.inert, closestInert: frame.closest("[inert]")?.className ?? null, chain };
        });
        const key = JSON.stringify(frames);
        if (key !== previous && timeline.length < 80) { previous = key; timeline.push({ at: performance.now(), mutations: records.map(record => ({ type: record.type, attribute: record.attributeName, target: (record.target as HTMLElement).className ?? record.target.nodeName })), frames }); }
      });
      observer.observe(document.body, { subtree: true, childList: true, attributes: true, attributeFilter: ["inert", "data-app-inert"] });
    }, { once: true });
  }, { scenario, version: VERSION, checksum: CHECKSUM, taskId: TASK_ID });
  await page.goto(scenario === "proxy-dense" ? "/#/proxies" : "/#/kernels");
}

test.beforeEach(async ({ page, baseURL }) => {
  const audit = { errors: [] as string[], deniedRequests: [] as string[], observations: [] as unknown[] }; audits.set(page, audit);
  page.on("pageerror", error => audit.errors.push(error.message));
  const origin = new URL(baseURL!).origin;
  await page.route("**/*", route => { if (new URL(route.request().url()).origin === origin) return route.continue(); audit.deniedRequests.push(route.request().url()); return route.abort("blockedbyclient"); });
  await page.routeWebSocket("**/*", socket => socket.close());
});
test.afterEach(async ({ page }, testInfo) => {
  const bridge = await page.evaluate(() => {
    const fixture = (window as unknown as TestWindow).__kernelVisualFixture;
    return { calls: fixture?.calls ?? [], unexpected: fixture?.unexpected ?? [], timeline: fixture?.timeline ?? [], demoStorageGuard: fixture ? "read-only reused native fixture throws on demo storage access" : "isolated synthetic demo storage", nativeIO: false };
  });
  await testInfo.attach("actual-App-kernel-visual-audit", { body: JSON.stringify({ ...bridge, ...audits.get(page) }, null, 2), contentType: "application/json" });
  expect(bridge.unexpected).toEqual([]);
  expect(bridge.calls.filter(call => call.method === "Operation.Cancel")).toHaveLength(0);
  expect(audits.get(page)?.errors).toEqual([]); expect(audits.get(page)?.deniedRequests).toEqual([]);
});

async function snapshot(page: Page, testInfo: TestInfo, label: string) {
  const state = await page.evaluate(() => {
    const rect = (element: Element) => { const r = element.getBoundingClientRect(), s = getComputedStyle(element); return { x: r.x, y: r.y, width: r.width, height: r.height, bottom: r.bottom, clientHeight: element.clientHeight, scrollHeight: element.scrollHeight, scrollTop: element.scrollTop, maxScroll: element.scrollHeight - element.clientHeight, fontSize: s.fontSize, background: s.backgroundColor, color: s.color }; };
    const frames = [...document.querySelectorAll<HTMLElement>('[aria-modal="true"]')].map(frame => {
      const chain = [];
      for (let node: HTMLElement | null = frame; node; node = node.parentElement) chain.push({ tag: node.tagName, className: node.className, inert: node.inert, zIndex: getComputedStyle(node).zIndex });
      return { title: frame.querySelector("h2")?.textContent, ...rect(frame), text: frame.innerText, closestInert: frame.closest("[inert]")?.className ?? null, chain };
    });
    return { frames, tableScroll: [...document.querySelectorAll(":is(.kernel35-page, .proxy35-page) > .pk35-list-panel > .pk35-table-scroll")].map(rect), pagination: [...document.querySelectorAll(":is(.kernel35-page, .proxy35-page) > .pk35-list-panel > .pk35-pagination")].map(rect), notice: [...document.querySelectorAll(":is(.kernel35-page, .proxy35-page) > .pk35-list-panel > .pk35-page-boundary")].map(rect), activeTag: document.activeElement?.tagName, activeFrame: document.activeElement?.closest('[aria-modal="true"]')?.querySelector("h2")?.textContent, documentScroll: window.scrollY, viewportHeight: innerHeight };
  });
  audits.get(page)!.observations.push({ label, ...state });
  const path = testInfo.outputPath(`${label}.png`); await page.screenshot({ path });
  await testInfo.attach(label, { path, contentType: "image/png" });
}
async function installForm(page: Page, local = false) {
  await page.getByRole("button", { name: "准备精确内核", exact: true }).click();
  const form = page.getByRole("dialog", { name: "安装精确内核", exact: true });
  if (local) { await form.getByLabel("归档来源", { exact: true }).selectOption("local"); await form.getByRole("button", { name: "选择可信 ZIP", exact: true }).click(); await form.getByRole("checkbox").check(); }
  await form.getByLabel("精确发行版本", { exact: true }).fill(VERSION); await form.getByLabel("预期归档 SHA-256", { exact: true }).fill(CHECKSUM);
  return form;
}
async function calls(page: Page, method: string) { return page.evaluate(method => (window as unknown as TestWindow).__kernelVisualFixture.calls.filter(call => call.method === method), method); }
async function focusedWithin(frame: Locator) { await expect.poll(() => frame.evaluate(element => element.contains(document.activeElement) && !document.activeElement?.closest("[inert]") && !(document.activeElement as HTMLButtonElement).disabled)).toBe(true); }

for (const viewport of [{ width: 1440, height: 900 }, { width: 1280, height: 800 }]) {
test.describe(`${viewport.width}x${viewport.height} actual App kernel visual recovery`, () => {
test.use({ viewport });

for (const mode of ["demo", "native"] as const) {
test(`${mode} dense ten-row page keeps pagination and ability notice in the viewport`, async ({ page }, testInfo) => {
  if (mode === "native") await native(page, "dense");
  else {
    const state = referenceWorkspace(); state.kernels = Array.from({ length: 12 }, (_, index) => ({ ...state.kernels[index % 2], id: index < 2 ? state.kernels[index].id : `synthetic-dense-kernel-${index + 1}`, version: `151.0.9000.${index + 11}` }));
    await page.addInitScript(({ key, state }) => localStorage.setItem(key, JSON.stringify(state)), { key: STORAGE_KEY, state }); await page.goto("/#/kernels");
  }
  const list = page.getByRole("region", { name: "内核列表", exact: true }), rows = list.locator("tbody tr"), pagination = list.locator(".pk35-pagination"), notice = list.locator(".pk35-page-boundary"), scroll = list.locator(".pk35-table-scroll");
  await expect(rows).toHaveCount(10); await expect(pagination).toContainText("共 12 条");
  await snapshot(page, testInfo, `${mode}-dense-first-page`);
  const geometry = await rows.evaluateAll(rows => rows.map((row, index) => ({ height: row.getBoundingClientRect().height, gap: index ? row.getBoundingClientRect().top - rows[index - 1].getBoundingClientRect().bottom : 10, font: getComputedStyle(row.querySelector("td")!).fontSize })));
  expect(geometry).toEqual(Array.from({ length: 10 }, () => ({ height: 48, gap: 10, font: "12px" })));
  expect(await pagination.evaluate(element => element.getBoundingClientRect().bottom)).toBeLessThanOrEqual(viewport.height - 10);
  expect(await notice.evaluate(element => element.getBoundingClientRect().bottom)).toBeLessThanOrEqual(viewport.height - 10);
  expect(await page.evaluate(() => window.scrollY)).toBe(0);
  const box = await scroll.boundingBox(); await page.mouse.move(box!.x + box!.width / 2, box!.y + box!.height / 2); await page.mouse.wheel(0, 1600);
  await expect.poll(() => scroll.evaluate(element => Math.abs(element.scrollTop + element.clientHeight - element.scrollHeight) < 2)).toBe(true);
  expect(await rows.last().evaluate(element => element.getBoundingClientRect().bottom)).toBeLessThanOrEqual((await scroll.boundingBox())!.y + (await scroll.boundingBox())!.height);
  await snapshot(page, testInfo, `${mode}-dense-table-bottom`);
  await pagination.getByRole("button", { name: "›", exact: true }).click(); await expect(rows).toHaveCount(2); await expect(rows.nth(0)).toContainText(mode === "native" ? "synthetic-dense-build-11" : "synthetic-dense-kernel-11"); await expect(rows.nth(1)).toContainText(mode === "native" ? "synthetic-dense-build-12" : "synthetic-dense-kernel-12");
  await snapshot(page, testInfo, `${mode}-dense-page-two`);
  await rows.last().getByRole("button", { name: "能力详情", exact: true }).click();
  const detail = page.getByRole("dialog", { name: "内核能力与接入", exact: true }); await expect(detail).toBeVisible(); await focusedWithin(detail);
  await page.keyboard.press("Escape"); await expect(detail).toHaveCount(0); await expect(notice).toBeVisible();
  await pagination.getByRole("button", { name: "‹", exact: true }).click(); await expect(rows).toHaveCount(10); await expect(pagination.locator(".pk35-current-page")).toHaveText("1");
});
}

for (const mode of ["demo", "native"] as const) {
test(`${mode} dense proxy page keeps ten rows, four final rows and exactly two cross-page selections`, async ({ page }, testInfo) => {
  if (mode === "native") await native(page, "proxy-dense");
  else {
    const state = referenceWorkspace(); state.proxies = Array.from({ length: 14 }, (_, index) => ({ ...state.proxies[index % 2], id: index < 2 ? state.proxies[index].id : `synthetic-dense-proxy-${index + 1}`, name: `合成分页代理 ${index + 1}`, username: "", password: "", status: "unchecked" }));
    await page.addInitScript(({ key, state }) => localStorage.setItem(key, JSON.stringify(state)), { key: STORAGE_KEY, state }); await page.goto("/#/proxies");
  }
  const list = page.getByRole("region", { name: "代理列表", exact: true }), rows = list.locator("tbody tr"), pagination = list.locator(".pk35-pagination"), notice = list.locator(".pk35-page-boundary"), scroll = list.locator(".pk35-table-scroll");
  await expect(rows).toHaveCount(10); await expect(pagination).toContainText("共 14 条"); await snapshot(page, testInfo, `${mode}-proxy-dense-first-page`);
  expect(await rows.evaluateAll(rows => rows.map((row, index) => ({ height: row.getBoundingClientRect().height, gap: index ? row.getBoundingClientRect().top - rows[index - 1].getBoundingClientRect().bottom : 10, font: getComputedStyle(row.querySelector("td")!).fontSize })))).toEqual(Array.from({ length: 10 }, () => ({ height: 48, gap: 10, font: "12px" })));
  expect(await pagination.evaluate(element => element.getBoundingClientRect().bottom)).toBeLessThanOrEqual(viewport.height - 10); expect(await notice.evaluate(element => element.getBoundingClientRect().bottom)).toBeLessThanOrEqual(viewport.height - 10);
  await list.getByLabel("选择代理 合成分页代理 1", { exact: true }).check();
  const box = await scroll.boundingBox(); await page.mouse.move(box!.x + box!.width / 2, box!.y + box!.height / 2); await page.mouse.wheel(0, 1600);
  await expect.poll(() => scroll.evaluate(element => Math.abs(element.scrollTop + element.clientHeight - element.scrollHeight) < 2)).toBe(true); await snapshot(page, testInfo, `${mode}-proxy-dense-table-bottom`);
  expect(await rows.last().evaluate(element => element.getBoundingClientRect().bottom)).toBeLessThanOrEqual((await scroll.boundingBox())!.y + (await scroll.boundingBox())!.height);
  await pagination.getByRole("button", { name: "下一页代理", exact: true }).click(); await expect(rows).toHaveCount(4); await expect(rows.first()).toContainText("合成分页代理 11"); await expect(rows.last()).toContainText("合成分页代理 14");
  await list.getByLabel("选择代理 合成分页代理 14", { exact: true }).check(); await expect(pagination.getByRole("button", { name: "取消已选 2 项", exact: true })).toBeVisible(); await expect(rows.locator('input[type="checkbox"]:checked')).toHaveCount(1); await expect(list.getByLabel("选择当前页全部代理", { exact: true })).not.toBeChecked();
  await snapshot(page, testInfo, `${mode}-proxy-dense-page-two`);
  await rows.last().getByRole("button", { name: "合成分页代理 14 已绑定环境", exact: true }).click(); const usage = page.getByRole("dialog", { name: "已绑定环境 · 合成分页代理 14", exact: true }); await expect(usage).toBeVisible();
  expect(await usage.evaluate(element => ({ width: element.getBoundingClientRect().width, height: element.getBoundingClientRect().height }))).toEqual({ width: 1040, height: 592 }); await expect(usage).toContainText("已返回 0 / 服务共 0 项"); await page.keyboard.press("Escape"); await expect(usage).toHaveCount(0);
  await pagination.getByRole("button", { name: "上一页代理", exact: true }).click(); await expect(rows).toHaveCount(10); await expect(list.getByLabel("选择代理 合成分页代理 1", { exact: true })).toBeChecked(); await expect(rows.locator('input[type="checkbox"]:checked')).toHaveCount(1); await expect(pagination.getByRole("button", { name: "取消已选 2 项", exact: true })).toBeVisible();
  expect(await notice.evaluate(element => element.getBoundingClientRect().bottom)).toBeLessThanOrEqual(viewport.height - 10); await expect(notice).toContainText("失败不自动改为直连");
  if (mode === "native") expect(await calls(page, "Proxy.Check")).toHaveLength(0);
});
}

for (const scenario of ["official-unknown", "malformed", "local-unknown"] as const) {
test(`${scenario} keeps the entire short request hint and replays only the original request`, async ({ page }, testInfo) => {
  await native(page, scenario); const form = await installForm(page, scenario === "local-unknown");
  await form.getByRole("button", { name: "安装并核验", exact: true }).click();
  const retry = form.getByRole("button", { name: "核实原内核请求", exact: true }); await expect(retry).toBeEnabled();
  await expect(form.getByLabel("精确发行版本", { exact: true })).toHaveValue(VERSION); await expect(form.getByLabel("预期归档 SHA-256", { exact: true })).toHaveValue(CHECKSUM); await expect(form.getByLabel("归档来源", { exact: true })).toBeDisabled();
  await expect(form.getByLabel("精确发行版本", { exact: true })).toBeDisabled(); await expect(form.getByLabel("预期归档 SHA-256", { exact: true })).toBeDisabled();
  await expect(form).toContainText("归档摘要、程序身份和参数读回通过后才登记"); await expect(form).toContainText(scenario === "malformed" ? "内核回执身份不符，保留原请求核实" : "原请求是否受理未知，只能核实同一请求");
  if (scenario === "local-unknown") { await expect(form).toContainText("synthetic-visual-kernel.zip"); await expect(form.getByRole("checkbox")).toBeChecked(); await expect(form.getByRole("checkbox")).toBeDisabled(); }
  const body = form.locator(".reference-modal-body"); await snapshot(page, testInfo, scenario);
  const capacity = await body.evaluate(element => ({ client: element.clientHeight, scroll: element.scrollHeight, top: element.scrollTop, lastLine: element.querySelector('[role="status"]')!.getBoundingClientRect().bottom, bottom: element.getBoundingClientRect().bottom }));
  audits.get(page)!.observations.push({ label: `${scenario}-body-capacity`, ...capacity });
  expect(capacity.top).toBe(0); expect(capacity.scroll).toBeLessThanOrEqual(capacity.client); expect(capacity.lastLine).toBeLessThanOrEqual(capacity.bottom);
  const requests = await calls(page, "Kernel.Install"); expect(requests).toHaveLength(1); await expect(page.getByRole("dialog", { name: "内核任务", exact: true })).toHaveCount(0);
  await retry.click(); const task = page.getByRole("dialog", { name: "内核任务", exact: true }); await expect(task).toContainText(TASK_ID); await expect(task).toContainText("已完成 0");
  const replay = await calls(page, "Kernel.Install"); expect(replay).toHaveLength(2); expect(replay[1]).toEqual(replay[0]);
  await page.keyboard.press("Escape"); await expect(task).toHaveCount(0);
});
}

for (const scenario of ["pending-completed", "pending-storage"] as const) {
test(`${scenario} projects protected persistence before completion on every task surface`, async ({ page }, testInfo) => {
  await native(page, scenario); const form = await installForm(page); await form.getByRole("button", { name: "安装并核验", exact: true }).click();
  const task = page.getByRole("dialog", { name: "内核任务", exact: true }), opener = page.getByRole("button", { name: "查看内核任务 · 结果待保存", exact: true });
  await expect(task).toContainText(TASK_ID); await snapshot(page, testInfo, scenario);
  await expect(task.locator(".reference-modal-body > p").first()).toHaveText("任务：结果待保存"); await expect(opener).toBeVisible();
  await expect(task).toContainText("已完成 0"); await expect(task).toContainText("尚非持久终态"); await expect(task.getByRole("button", { name: "取消此任务", exact: true })).toBeDisabled();
  await page.keyboard.press("Escape"); await expect(task).toHaveCount(0); await expect(page.getByRole("button", { name: "准备精确内核", exact: true })).toBeDisabled();
  await page.getByRole("button", { name: "最近内核任务", exact: true }).click(); const history = page.getByRole("dialog", { name: "最近内核任务（持久记录）", exact: true });
  await expect(history.locator("tbody tr").filter({ hasText: TASK_ID }).locator("td").nth(1)).toHaveText("结果待保存"); await expect(history.getByRole("button", { name: "查看任务", exact: true })).toBeDisabled(); await page.keyboard.press("Escape");
  await opener.click(); await expect(task).toContainText(TASK_ID);
  await page.evaluate(() => (window as unknown as TestWindow).__kernelVisualFixture.confirmPersistence());
  await expect(task.locator(".reference-modal-body > p").first()).toHaveText("任务：已完成"); await expect(task).toContainText("已完成 1"); await expect(task.getByRole("button", { name: "取消此任务", exact: true })).toHaveCount(0);
  expect(await calls(page, "Kernel.Install")).toHaveLength(1); expect((await calls(page, "Operation.Read")).every(call => JSON.stringify(call.payload) === JSON.stringify({ operationId: TASK_ID }))).toBe(true);
  await page.keyboard.press("Escape"); await expect(page.getByRole("button", { name: "准备精确内核", exact: true })).toBeEnabled();
});
}

test("accepted task portal is actually inert under a later workspace blocker and survives exact recovery", async ({ page }, testInfo) => {
  await native(page, "workspace-fault"); const before = await page.evaluate(() => (window as unknown as TestWindow).__proxyKernel.view().state.environments[0]);
  const form = await installForm(page); await form.getByRole("button", { name: "安装并核验", exact: true }).click();
  const blocker = page.getByRole("alertdialog", { name: "工作区需要处理", exact: true }), task = page.getByRole("dialog", { name: "内核任务", exact: true });
  await expect(blocker).toContainText("受理后工作区读取失败"); await expect(task).toContainText(TASK_ID); await focusedWithin(blocker);
  await snapshot(page, testInfo, "accepted-workspace-fault");
  const ownership = await task.evaluate(frame => {
    const overlay = frame.closest<HTMLElement>(".pk35-overlay")!, blocker = document.querySelector('[role="alertdialog"]')!;
    const masks = [...document.querySelectorAll<HTMLElement>(".overlay, .pk35-overlay, .local-page-overlay")].filter(element => element.getClientRects().length && !["transparent", "rgba(0, 0, 0, 0)"].includes(getComputedStyle(element).backgroundColor));
    return { frameClosestInert: frame.closest("[inert]") === overlay, overlayInert: overlay.inert, overlayAlpha: getComputedStyle(overlay).backgroundColor, lowerLayer: getComputedStyle(overlay).zIndex, highestLayer: getComputedStyle(blocker.closest(".workspace-blocker")!).zIndex, maskCount: masks.length, shellInert: document.querySelector<HTMLElement>(".main-shell")!.inert, blockerInert: blocker.closest("[inert]") !== null };
  });
  audits.get(page)!.observations.push({ label: "blocked-owner-guard", ...ownership });
  expect(ownership).toEqual({ frameClosestInert: true, overlayInert: true, overlayAlpha: "rgba(0, 0, 0, 0)", lowerLayer: "92", highestLayer: "160", maskCount: 1, shellInert: true, blockerInert: false });
  const reload = blocker.getByRole("button", { name: "重新读取本机工作区", exact: true }); await expect(reload).toBeEnabled();
  expect(await reload.evaluate(element => ({ background: getComputedStyle(element).backgroundColor, color: getComputedStyle(element).color }))).toEqual({ background: "rgb(47, 84, 235)", color: "rgb(255, 255, 255)" });
  for (const key of ["Tab", "Tab", "Shift+Tab", "Escape", "Control+k"]) { await page.keyboard.press(key); await focusedWithin(blocker); await expect(blocker).toBeVisible(); await expect(task).toContainText(TASK_ID); }
  await page.mouse.click(viewport.width - 10, viewport.height - 10); await focusedWithin(blocker); expect(await calls(page, "Operation.Cancel")).toHaveLength(0); expect(await calls(page, "Kernel.Install")).toHaveLength(1);
  await page.evaluate(() => (window as unknown as TestWindow).__kernelVisualFixture.recoverWorkspace()); await reload.click();
  await expect(blocker).toHaveCount(0); await expect(task).toContainText(TASK_ID); await focusedWithin(task); await expect(task.getByRole("button", { name: "取消此任务", exact: true })).toBeEnabled();
  expect(await task.evaluate(frame => frame.closest("[inert]") !== null)).toBe(false); await snapshot(page, testInfo, "accepted-workspace-recovered");
  expect(await page.evaluate(() => (window as unknown as TestWindow).__proxyKernel.view().state.environments[0])).toEqual(before);
  expect(await calls(page, "Kernel.Install")).toHaveLength(1); expect(await calls(page, "Operation.Cancel")).toHaveLength(0);
  await page.keyboard.press("Escape"); await expect(task).toHaveCount(0); const opener = page.getByRole("button", { name: "查看内核任务 · 隔离探测实际身份和参数", exact: true }); await opener.click(); await expect(task).toContainText(TASK_ID); await page.keyboard.press("Escape"); await expect(opener).toBeFocused();
});

test("initial workspace loading keeps recovery disabled and ignores Escape until its reply arrives", async ({ page }, testInfo) => {
  await native(page, "loading"); const blocker = page.getByRole("alertdialog", { name: "工作区需要处理", exact: true }), reload = blocker.getByRole("button", { name: "重新读取本机工作区", exact: true });
  await expect(blocker).toContainText("正在读取本机工作区"); await expect(reload).toBeDisabled(); await focusedWithin(blocker); await snapshot(page, testInfo, "kernel-workspace-loading");
  expect(await reload.evaluate(element => getComputedStyle(element).opacity)).toBe("0.46"); await page.keyboard.press("Escape"); await expect(blocker).toBeVisible(); await expect(reload).toBeDisabled();
  await page.evaluate(() => (window as unknown as TestWindow).__kernelVisualFixture.releaseLoading()); await expect(blocker).toHaveCount(0); await expect(page.getByRole("button", { name: "准备精确内核", exact: true })).toBeEnabled();
});
});
}
