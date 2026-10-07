import { expect, test, type Locator, type Page } from "@playwright/test";
import { STORAGE_KEY, type State } from "../../src/domain";
import type { WorkspaceView } from "../../src/application/contract";
import { nativeReferenceBridge, nativeReferenceView } from "./fixtures/native-reference-bridge";
import { referenceWorkspace } from "./fixtures/reference-workspace";

// Actual App, synthetic data only. Never inject layout styles or call a real bridge.
const audits = new WeakMap<Page, { errors: string[]; blockedRequests: string[] }>();
test.beforeEach(async ({ page, baseURL }) => {
  const audit = { errors: [] as string[], blockedRequests: [] as string[] };
  audits.set(page, audit);
  page.on("pageerror", error => audit.errors.push(error.message));
  const origin = new URL(baseURL!).origin;
  await page.route("**/*", route => {
    const request = route.request();
    if (new URL(request.url()).origin === origin && request.method() === "GET") return route.continue();
    audit.blockedRequests.push(request.url());
    return route.abort("blockedbyclient");
  });
  await page.routeWebSocket("**/*", socket => socket.close());
});
test.afterEach(async ({ page }, testInfo) => {
  const audit = audits.get(page)!;
  testInfo.annotations.push({ type: "browser-boundary", description: JSON.stringify({ ...audit, realNativeCalls: 0 }) });
  expect(audit.errors).toEqual([]);
  expect(audit.blockedRequests).toEqual([]);
});

async function demo(page: Page, state = referenceWorkspace()) {
  await page.addInitScript(({ state, key }) => {
    for (const name of ["go", "runtime"]) Object.defineProperty(window, name, { value: undefined, writable: false, configurable: false });
    localStorage.setItem(key, JSON.stringify(state));
  }, { state, key: STORAGE_KEY });
  await page.goto("/#/environments");
  await expect(page.locator(".environment-pagination")).toBeVisible();
}
const stored = (page: Page): Promise<State> => page.evaluate(key => JSON.parse(localStorage.getItem(key)!), STORAGE_KEY);
const button = (page: Page, name: string) => page.getByRole("button", { name, exact: true });
const box = async (locator: Locator) => (await locator.boundingBox())!;

async function toolbarReachable(page: Page) {
  const outside = await page.locator(".environment-search-toolbar, .environment-list-toolbar").evaluateAll(toolbars =>
    toolbars.flatMap(toolbar => [...toolbar.querySelectorAll("button, input, select")].flatMap(element => {
      const r = element.getBoundingClientRect();
      return r.x < 0 || r.right > innerWidth || r.y < 42 || r.bottom > innerHeight || r.width === 0 || r.height === 0
        ? [{ label: element.getAttribute("aria-label") ?? element.textContent?.trim(), x: r.x, right: r.right, bottom: r.bottom }] : [];
    })));
  expect(outside, "Every full toolbar control must fit the viewport, not just intersect it").toEqual([]);
  expect(await page.evaluate(() => document.documentElement.scrollWidth)).toBe(page.viewportSize()!.width);
  await expect(page.locator(".count-boundary")).toBeVisible();
  const controls = page.locator(".environment-search-toolbar button, .environment-search-toolbar input, .environment-search-toolbar select, .environment-list-toolbar button");
  for (const control of await controls.all()) {
    await expect(control).toBeVisible();
    if (await control.isEnabled()) await control.click({ trial: true });
  }
  expect((await box(page.getByLabel("搜索环境", { exact: true }))).width).toBeGreaterThanOrEqual(120);
}

async function tableBudget(page: Page) {
  const rows = page.locator(".environment-table tbody tr");
  await expect(rows).toHaveCount(10);
  const body = page.locator(".environment-table tbody"), scroll = page.locator(".environment-table-scroll");
  const first = await box(rows.first()), second = await box(rows.nth(1)), footer = await box(page.locator(".environment-pagination"));
  expect(first.height).toBe(48);
  expect(second.y - first.y).toBe(58);
  expect((await box(page.locator(".environment-table thead"))).height).toBe(40);
  expect(await body.evaluate(element => element.scrollHeight)).toBe(590);
  await expect(page.locator(".environment-table")).toHaveCSS("min-width", "855px");
  expect((await box(page.locator(".environment-table"))).width).toBe(Math.max(855, (await box(scroll)).width));
  const dimensions = await body.evaluate(element => ({ height: element.clientHeight, scrollHeight: element.scrollHeight }));
  expect(dimensions.height).toBeGreaterThanOrEqual(68);
  expect((dimensions.height - 10) % 58, "Remaining table budget keeps complete 48px rows and 10px gaps").toBe(0);
  expect(dimensions.height).toBeLessThan(dimensions.scrollHeight);
  expect(footer.y + footer.height).toBeLessThanOrEqual(page.viewportSize()!.height - 9);
  expect((await box(scroll)).y + (await box(scroll)).height).toBeLessThanOrEqual(footer.y);
  expect(await scroll.evaluate(element => element.scrollWidth > element.clientWidth)).toBe((await box(scroll)).width < 855);
  await body.evaluate(element => { element.scrollTop = element.scrollHeight; });
  await scroll.evaluate(element => { element.scrollLeft = element.scrollWidth; });
  await expect(rows.last()).toBeInViewport();
  await expect(rows.last().getByRole("button", { name: "打开", exact: true })).toBeInViewport();
  expect(await box(page.locator(".environment-pagination"))).toEqual(footer);
  expect(await page.evaluate(() => ({ x: scrollX, y: scrollY }))).toEqual({ x: 0, y: 0 });
  await body.evaluate(element => { element.scrollTop = 0; });
  await scroll.evaluate(element => { element.scrollLeft = 0; });
}

for (const width of [1024, 900]) {
  test(`${width}x720 normal toolbar retains readable filters, refresh and independent table scrolling`, async ({ page }) => {
    await page.setViewportSize({ width, height: 720 });
    await demo(page);
    await toolbarReachable(page);
    await tableBudget(page);
    const advanced = button(page, "高级搜索");
    await advanced.click();
    await expect(page.getByRole("dialog", { name: "高级搜索", exact: true })).toBeInViewport();
    await page.keyboard.press("Escape");
    await expect(advanced).toBeFocused();
    const beforeRefresh = await stored(page);
    await button(page, "刷新环境列表").click();
    await expect(button(page, "刷新环境列表")).toBeFocused();
    // DemoAdapter has no refresh RPC; the layout repair must not manufacture one.
    expect(await stored(page)).toEqual(beforeRefresh);
    await button(page, "分组筛选").click();
    await button(page, "测试环境").click();
    await expect(page.getByLabel("筛选分组", { exact: true })).toHaveValue("测试环境");
    await expect(page.locator(".environment-table tbody tr")).toHaveCount(6);
    await toolbarReachable(page);
  });

  test(`${width}x720 selected toolbar keeps exact IDs, full bulk actions and outcome/footer space`, async ({ page }) => {
    await page.setViewportSize({ width, height: 720 });
    const before = referenceWorkspace();
    await demo(page, before);
    await page.getByLabel("选择 工作环境 A", { exact: true }).check();
    await page.getByLabel("选择 工作环境 B", { exact: true }).check();
    await toolbarReachable(page);
    await tableBudget(page);
    const more = button(page, "更多操作");
    await more.click();
    await expect(page.getByRole("menu", { name: "更多操作", exact: true })).toBeInViewport();
    await page.keyboard.press("Escape");
    await expect(more).toBeFocused();
    await button(page, "高级搜索").click();
    await page.getByLabel("高级搜索关键词", { exact: true }).fill("工作环境 A");
    await button(page, "开始搜索").click();
    await expect(page.locator(".environment-list-toolbar .selection-bar strong")).toHaveText("2");
    await button(page, "调整分组").click();
    await page.getByLabel("分组名称", { exact: true }).fill("窄窗口精确所选");
    await button(page, "应用到所选环境").click();
    const assigned = await stored(page);
    expect(assigned.environments.filter(environment => environment.group === "窄窗口精确所选").map(environment => environment.id)).toEqual(before.environments.slice(0, 2).map(environment => environment.id));
    expect(assigned.environments.map(environment => [environment.id, environment.seed, environment.coreId, environment.proxyId, environment.cookies])).toEqual(before.environments.map(environment => [environment.id, environment.seed, environment.coreId, environment.proxyId, environment.cookies]));
    await more.click();
    await page.getByRole("menu", { name: "更多操作", exact: true }).getByRole("button", { name: "清除筛选", exact: true }).click();
    await button(page, "批量打开").click();
    const results = page.getByRole("region", { name: "逐项操作结果", exact: true });
    await expect(results.locator(".outcome-success")).toHaveCount(2);
    await expect(results).toContainText("模拟操作，不启动真实浏览器");
    await tableBudget(page);
    await toolbarReachable(page);
    expect((await stored(page)).environments.filter(environment => environment.status === "running").map(environment => environment.id)).toEqual(before.environments.slice(0, 2).map(environment => environment.id));
    await button(page, "批量关闭").click();
    await expect(results.locator(".outcome-success")).toHaveCount(2);
    expect((await stored(page)).environments.every(environment => environment.status === "ready")).toBe(true);
    await button(page, "取消选择").click();
    await expect(button(page, "批量打开")).toHaveCount(0);
    await toolbarReachable(page);
  });
}

test("900x720 empty list stays compact and its real create action is reachable", async ({ page }) => {
  await page.setViewportSize({ width: 900, height: 720 });
  await demo(page, { ...referenceWorkspace(), environments: [] });
  await toolbarReachable(page);
  await expect(page.locator(".environment-table tbody")).toHaveCSS("height", "68px");
  await expect(page.getByRole("heading", { name: "还没有浏览器环境", exact: true })).toBeVisible();
  await expect(page.locator(".environment-pagination")).toBeInViewport();
  await page.locator(".environment-empty-row").getByRole("button", { name: "创建第一个环境", exact: true }).click();
  await expect(page.getByRole("dialog", { name: "新建浏览器环境", exact: true })).toBeVisible();
});

test("900x720 selected no-results state stays compact and clear restores the same selection", async ({ page }) => {
  await page.setViewportSize({ width: 900, height: 720 });
  await demo(page);
  await page.getByLabel("选择 工作环境 A", { exact: true }).check();
  await page.getByLabel("搜索环境", { exact: true }).fill("合成不存在的名字");
  await expect(page.getByRole("heading", { name: "没有符合条件的环境", exact: true })).toBeVisible();
  await toolbarReachable(page);
  await expect(page.locator(".environment-table tbody")).toHaveCSS("height", "68px");
  await expect(page.locator(".environment-list-toolbar .selection-bar strong")).toHaveText("1");
  await page.locator(".environment-empty-row").getByRole("button", { name: "清除筛选", exact: true }).click();
  await expect(page.getByLabel("选择 工作环境 A", { exact: true })).toBeChecked();
  await expect(page.locator(".environment-table tbody tr.row-selected")).toHaveCount(1);
  await tableBudget(page);
});

test("selected upper responsive edge and 850px breakpoint preserve long labels and exact selection", async ({ page }) => {
  const state = referenceWorkspace(), group = "跨境业务专用长分组名称".repeat(3);
  state.environments = state.environments.map((environment, index) => ({ ...environment, name: `${"超长合成环境名称".repeat(3)} ${index + 1}`, group }));
  await page.setViewportSize({ width: 1101, height: 720 });
  await demo(page, state);
  await page.getByLabel(`选择 ${state.environments[0].name}`, { exact: true }).check();
  await page.getByLabel(`选择 ${state.environments[1].name}`, { exact: true }).check();
  await page.getByLabel("筛选分组", { exact: true }).selectOption(group);
  for (const width of [1101, 1170, 1199, 1200, 1279, 1280, 1100, 1099, 851, 850, 800]) {
    await page.setViewportSize({ width, height: width === 1280 ? 800 : 720 });
    await toolbarReachable(page);
    if (width < 1280) await tableBudget(page);
    else {
      // At the frozen desktop boundary, retain its original (not rounded) table budget.
      const header = await box(page.locator(".environment-table thead")), footer = await box(page.locator(".environment-pagination"));
      expect([header.x, header.y, header.width, header.height]).toEqual([220, 162.859375, 1040, 40]);
      await expect(page.locator(".environment-table tbody")).toHaveCSS("height", "482px");
      expect([footer.x, footer.y, footer.width, footer.height]).toEqual([220, 694.859375, 1040, 30]);
      await expect(page.locator(".environment-table tbody tr")).toHaveCount(10);
    }
    expect((await box(page.locator(".sidebar"))).width).toBe(width > 850 ? 200 : 56);
    await expect(page.getByLabel("筛选分组", { exact: true })).toHaveValue(group);
    await expect(page.locator(".environment-list-toolbar .selection-bar strong")).toHaveText("2");
    await button(page, "高级搜索").click();
    await expect(page.getByLabel("高级搜索分组", { exact: true })).toHaveValue(group);
    await page.keyboard.press("Escape");
  }
  expect((await stored(page)).environments.map(environment => [environment.id, environment.seed])).toEqual(state.environments.map(environment => [environment.id, environment.seed]));
});

test("900x720 bounded native fixture keeps safe filters, full queue copy and maintenance banner unclipped", async ({ page }) => {
  await page.setViewportSize({ width: 900, height: 720 });
  await nativeReferenceBridge(page, true);
  await page.goto("/#/environments");
  await expect(page.getByLabel("选择 工作环境 A", { exact: true })).toBeEnabled();
  await page.getByLabel("选择 工作环境 A", { exact: true }).check();
  await toolbarReachable(page);
  await tableBudget(page);
  await button(page, "打开回收区").click({ trial: true });
  const queue = page.locator(".environment-table-panel > .selection-bar");
  await expect(queue).toContainText("按受理顺序启动；已运行环境不占队列名额。");
  expect(await queue.evaluate(element => element.scrollWidth <= element.clientWidth)).toBe(true);
  const queueBox = await box(queue);
  expect(queueBox.x + queueBox.width).toBeLessThanOrEqual(900);
  await button(page, "取消排队启动").click({ trial: true });
  await button(page, "高级搜索").click();
  await page.getByLabel("高级搜索状态", { exact: true }).selectOption("error");
  await button(page, "开始搜索").click();
  await expect(page.locator(".environment-table tbody tr")).toHaveCount(5);
  await expect(page.locator(".environment-list-toolbar .selection-bar strong")).toHaveText("1");
  await page.evaluate(() => {
    const view = (window as unknown as { __referenceNative: { view: WorkspaceView } }).__referenceNative.view;
    view.maintenance = { id: "synthetic-narrow-maintenance", kind: "backup-restore", state: "running", total: 1, completedIds: [], cancelRequested: false,
      restoreReport: { mode: "native", requestId: "synthetic-narrow-request", previewId: "synthetic-narrow-preview", archiveSha256: "a".repeat(64), sequence: 1,
        environmentCount: 1, switchedCount: 0, credentialReentryCount: 0, committed: false, rolledBack: false, protected: true } };
  });
  await button(page, "刷新环境列表").click();
  const banner = page.locator(".environment-workspace > .prototype-notice");
  await expect(banner).toContainText("完整恢复正在维护保护中，配置修改与新启动暂不可用。");
  await banner.getByRole("button", { name: "查看恢复任务", exact: true }).click({ trial: true });
  expect(await banner.evaluate(element => element.scrollWidth <= element.clientWidth)).toBe(true);
  await toolbarReachable(page);
  await expect(page.locator(".environment-pagination")).toBeInViewport();
  const fixture = await nativeReferenceView(page);
  expect(fixture.calls.every(call => ["Workspace.Read", "Operation.Read"].includes(call.method))).toBe(true);
  expect(fixture.calls.some(call => call.method === "Workspace.Read" && (call.payload as { environmentQuery?: { status: string } }).environmentQuery?.status === "error")).toBe(true);
});

test("frozen 1440x900 and 1280x800 normal/selected control and table geometry is unchanged", async ({ page }) => {
  await demo(page);
  for (const viewport of [{ width: 1440, height: 900 }, { width: 1280, height: 800 }]) {
    await page.setViewportSize(viewport);
    for (const selected of [false, true]) {
      await page.getByLabel("选择 工作环境 A", { exact: true }).setChecked(selected);
      const surfaces: [string, number[]][] = [
        [".environment-search-toolbar", [210, 50, viewport.width - 220, 52.859375]],
        [".environment-list-toolbar", [210, 112.859375, viewport.width - 220, 50]],
        [".environment-table-scroll", [220, 162.859375, viewport.width - 240, viewport.height - 278]],
        [".environment-table tbody", [220, 202.859375, viewport.width - 240, viewport.height - 318]],
        [".environment-pagination", [220, viewport.height - 105.140625, viewport.width - 240, 30]],
      ];
      for (const [selector, expected] of surfaces) {
        const actual = await box(page.locator(selector));
        expect([actual.x, actual.y, actual.width, actual.height], `${viewport.width} ${selected ? "selected" : "normal"} ${selector}`).toEqual(expected);
      }
      const controls: [string, number[]][] = [
        ["新建环境", [220, 60.921875, 110, 31]], ["创建菜单", [330, 60.921875, 24, 31]],
        ["导入 / 备份", [542.234375, 60.921875, 114.234375, 31]],
        ["高级搜索", [viewport.width - 86, 63.171875, 66, 26.515625]],
        ["全部", [221, 123.359375, 79.59375, 29]], ["待启动", [300.59375, 123.359375, 79.609375, 29]],
        ["已打开", [380.203125, 123.359375, 79.59375, 29]], ["需处理", [459.796875, 123.359375, 79.609375, 29]],
        ["分组筛选", [539.40625, 123.359375, 79.59375, 29]],
        ["更多操作", [viewport.width - 162, 122.359375, 100, 31]], ["刷新环境列表", [viewport.width - 52, 122.359375, 32, 31]],
      ];
      if (selected) controls.push(["取消选择", [706.40625, 128.859375, 48, 18]], ["批量打开", [viewport.width - 438, 122.359375, 82, 31]], ["批量关闭", [viewport.width - 346, 122.359375, 82, 31]], ["调整分组", [viewport.width - 254, 122.359375, 82, 31]]);
      for (const [name, expected] of controls) {
        const actual = await box(button(page, name));
        expect([actual.x, actual.y, actual.width, actual.height], `${viewport.width} ${name}`).toEqual(expected);
      }
      const group = await box(page.getByLabel("筛选分组", { exact: true })), search = await box(page.getByLabel("搜索环境", { exact: true }));
      expect([group.x, group.y, group.width, group.height]).toEqual([666.46875, 60.421875, 150, 32]);
      expect([search.x, search.y, search.width, search.height]).toEqual([942.9375, 60.921875, viewport.width - 1061.9375, 30]);
      await expect(page.locator(".environment-table tbody tr")).toHaveCount(10);
    }
    await page.getByLabel("选择 工作环境 A", { exact: true }).uncheck();
  }
});
