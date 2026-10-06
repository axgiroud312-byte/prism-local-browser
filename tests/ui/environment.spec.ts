import { test, expect, type Page } from "@playwright/test";
import { seedState, STORAGE_KEY, type State } from "../../src/domain.ts";

async function stored(page: Page): Promise<State> {
  return page.evaluate(key => JSON.parse(localStorage.getItem(key)!), STORAGE_KEY);
}
async function edit(page: Page, name: string) {
  await page.getByRole("button", { name: `${name} 更多操作`, exact: true }).click();
  await page.getByRole("button", { name: "编辑环境", exact: true }).click();
  await expect(page.getByRole("dialog", { name: "编辑浏览器环境" })).toBeVisible();
}
async function denyWrites(page: Page) {
  await page.evaluate(() => {
    const original = Storage.prototype.setItem;
    (window as unknown as { restoreTestStorage: () => void }).restoreTestStorage = () => { Storage.prototype.setItem = original; };
    Storage.prototype.setItem = () => { throw new DOMException("synthetic failure", "QuotaExceededError"); };
  });
}
async function restoreWrites(page: Page) {
  await page.evaluate(() => (window as unknown as { restoreTestStorage: () => void }).restoreTestStorage());
}
async function denyWritesAfter(page: Page, allowed: number) {
  await page.evaluate(allowed => {
    const original = Storage.prototype.setItem;
    let writes = 0;
    (window as unknown as { restoreTestStorage: () => void }).restoreTestStorage = () => { Storage.prototype.setItem = original; };
    Storage.prototype.setItem = function (key, value) {
      if (writes++ >= allowed) throw new DOMException("synthetic failure after partial progress", "QuotaExceededError");
      original.call(this, key, value);
    };
  }, allowed);
}
async function advanced(page: Page) {
  const details = page.getByRole("dialog").locator("details").first();
  if (await details.getAttribute("open") === null) await details.locator(":scope > summary").click();
}
test.beforeEach(async ({ page }) => {
  const initial = seedState();
  initial.environments.forEach(e => { e.status = "ready"; });
  await page.goto("/#/environments");
  await page.evaluate(({ key, initial }) => localStorage.setItem(key, JSON.stringify(initial)), { key: STORAGE_KEY, initial });
  await page.reload();
});

test("create, edit and reopen preserve the selected seed, kernel and proxy", async ({ page }) => {
  const errors: string[] = [];
  page.on("pageerror", e => errors.push(e.message));
  await page.getByRole("button", { name: "新建环境", exact: true }).click();
  await page.getByLabel("环境名称", { exact: true }).fill("UI 合成环境");
  await page.getByLabel("绑定代理").selectOption("px-gb");
  const seed = await page.getByLabel("固定指纹种子").inputValue();
  await page.getByRole("button", { name: "创建", exact: true }).click();
  await expect(page.getByRole("dialog")).toHaveCount(0);
  await expect.poll(async () => (await stored(page)).environments.some(e => e.name === "UI 合成环境")).toBe(true);
  await page.reload();
  await edit(page, "UI 合成环境");
  await page.getByLabel("环境名称", { exact: true }).fill("UI 修改环境");
  await page.getByRole("button", { name: "保存", exact: true }).click();
  await expect(page.getByText("环境配置已保存", { exact: true })).toBeVisible();
  await page.reload();
  const saved = (await stored(page)).environments.find(e => e.name === "UI 修改环境")!;
  expect(saved.seed).toBe(seed);
  expect(saved.proxyId).toBe("px-gb");
  expect(saved.coreId).toBe("core-148");
  expect(saved.cookies).toEqual([]);
  await expect(page.getByText("交互原型", { exact: false }).first()).toBeVisible();
  expect(errors).toEqual([]);
  await page.screenshot({ path: "output/playwright/created-edited.png", fullPage: true });
});

test("cancel regenerated edit leaves persistent state unchanged and restores focus", async ({ page }) => {
  const before = await stored(page);
  await edit(page, "北美主店");
  await page.getByLabel("环境名称", { exact: true }).fill("取消后不应保存的合成名字");
  const seed = await page.getByLabel("固定指纹种子").inputValue();
  await page.getByRole("button", { name: "换一套", exact: true }).click();
  await expect(page.getByLabel("固定指纹种子")).not.toHaveValue(seed);
  page.once("dialog", dialog => dialog.accept());
  await page.keyboard.press("Escape");
  await expect(page.getByRole("dialog")).toHaveCount(0);
  expect(await stored(page)).toEqual(before);
  await page.reload();
  expect((await stored(page)).environments[0].seed).toBe(seed);
  const create = page.getByRole("button", { name: "新建环境", exact: true });
  await create.click();
  await page.keyboard.press("Escape");
  await expect(create).toBeFocused();
});

test("invalid and duplicate names, URL and quantity are rejected without saving", async ({ page }) => {
  await page.getByRole("button", { name: "新建环境", exact: true }).click();
  await page.getByRole("button", { name: "创建", exact: true }).click();
  await expect(page.getByRole("alert")).toContainText("名称");
  await page.getByLabel("环境名称", { exact: true }).fill("北美主店");
  await page.getByRole("button", { name: "创建", exact: true }).click();
  await expect(page.getByRole("alert")).toContainText("同名");
  await page.getByLabel("环境名称", { exact: true }).fill("校验样本");
  await advanced(page);
  await page.getByLabel("创建数量").fill("0");
  await page.getByRole("button", { name: "创建", exact: true }).click();
  await expect(page.getByRole("alert")).toContainText("正整数");
  await page.getByLabel("创建数量").fill("1");
  await page.getByLabel("启动网址").fill("about:blank");
  await page.getByRole("button", { name: "创建", exact: true }).click();
  await expect(page.getByRole("alert")).toContainText("http");
  expect((await stored(page)).environments).toHaveLength(8);
});

test("storage write failure keeps the editor and old record; retry commits", async ({ page }) => {
  const before = await stored(page);
  await edit(page, "北美主店");
  await page.getByLabel("环境名称", { exact: true }).fill("失败后重试");
  await denyWrites(page);
  await page.getByRole("button", { name: "保存", exact: true }).click();
  await expect(page.getByRole("alert")).toContainText("未保存");
  await expect(page.getByRole("dialog", { name: "编辑浏览器环境" })).toBeVisible();
  expect(await stored(page)).toEqual(before);
  await expect(page.getByText("环境配置已保存", { exact: true })).toHaveCount(0);
  await restoreWrites(page);
  await page.getByRole("button", { name: "保存", exact: true }).click();
  await expect(page.getByText("环境配置已保存", { exact: true })).toBeVisible();
  await page.reload();
  expect((await stored(page)).environments[0].name).toBe("失败后重试");
});

test("bad storage remains intact until explicit reset", async ({ page }) => {
  await page.evaluate(key => localStorage.setItem(key, "{ broken synthetic storage"), STORAGE_KEY);
  await page.reload();
  await expect(page.getByRole("alertdialog", { name: "工作区需要处理" })).toBeVisible();
  expect(await page.evaluate(key => localStorage.getItem(key), STORAGE_KEY)).toBe("{ broken synthetic storage");
  const downloaded = page.waitForEvent("download");
  await page.getByRole("button", { name: "导出原始记录" }).click();
  expect((await downloaded).suggestedFilename()).toBe("prism-original-record.txt");
  await page.getByRole("button", { name: "重置演示工作区" }).click();
  await expect(page.getByRole("alertdialog")).toHaveCount(0);
  expect((await stored(page)).environments).toHaveLength(8);
});

test("disabled browser storage shows a recoverable blocker instead of a blank page", async ({ page }) => {
  const errors: string[] = [];
  page.on("pageerror", e => errors.push(e.message));
  await page.addInitScript(() => {
    Object.defineProperty(window, "localStorage", { get() { throw new DOMException("synthetic denied", "SecurityError"); } });
  });
  await page.reload();
  await expect(page.getByRole("alertdialog", { name: "工作区需要处理" })).toContainText("无法读取");
  await expect(page.getByRole("button", { name: "重新载入", exact: true })).toBeVisible();
  expect(errors).toEqual([]);
});

test("compatibility snapshot and Cookie writes preserve inputs and never override a write failure with success", async ({ page }) => {
  await page.goto("/#/backups");
  await denyWrites(page);
  await page.getByRole("button", { name: "创建快照", exact: true }).first().click();
  await expect(page.locator(".toast-error")).toContainText("未保存");
  expect((await stored(page)).backups).toHaveLength(0);
  await expect(page.getByText("原型快照已保存到当前浏览器", { exact: true })).toHaveCount(0);
  await restoreWrites(page);
  await page.getByRole("button", { name: "创建快照", exact: true }).first().click();
  expect((await stored(page)).backups).toHaveLength(1);
  await page.goto("/#/environments");
  await page.getByRole("button", { name: "北美主店 更多操作", exact: true }).click();
  await page.getByRole("button", { name: "导入 Cookie", exact: true }).click();
  const sample = JSON.stringify([{ name: "synthetic-session", value: "", domain: "example.test", path: "/" }]);
  await page.getByLabel("Cookie 内容", { exact: true }).fill(sample);
  await page.getByRole("button", { name: "校验并预览", exact: true }).click();
  await denyWrites(page);
  await page.getByRole("button", { name: "导入到原型记录", exact: true }).click();
  await expect(page.locator(".toast-error")).toContainText("未保存");
  await expect(page.getByLabel("Cookie 内容", { exact: true })).toHaveValue(sample);
  expect((await stored(page)).environments[0].cookies).toEqual([]);
  await expect(page.getByText("已导入 1 条示例 Cookie", { exact: true })).toHaveCount(0);
  await restoreWrites(page);
  await page.getByRole("button", { name: "导入到原型记录", exact: true }).click();
  await expect(page.getByRole("dialog")).toHaveCount(0);
  expect((await stored(page)).environments[0].cookies[0].value).toBe("");
  const row = page.getByRole("row").filter({ hasText: "北美主店" });
  await row.getByRole("button", { name: "打开", exact: true }).click();
  await expect(row).toContainText("运行中");
  await row.getByRole("button", { name: "关闭", exact: true }).click();
  await expect(row).toContainText("待启动");
  await row.getByRole("button", { name: "打开", exact: true }).click();
  await expect(row).toContainText("运行中");
  await page.reload();
  expect((await stored(page)).environments[0].cookies[0]).toMatchObject({ name: "synthetic-session", value: "", domain: "example.test" });
});

test("a second tab blocks the stale editor without overwriting the newer workspace", async ({ page, context }) => {
  const second = await context.newPage();
  await second.goto("/#/environments");
  await edit(second, "北美主店");
  await edit(page, "北美主店");
  await page.getByLabel("环境名称", { exact: true }).fill("跨页最新修改");
  await page.getByRole("button", { name: "保存", exact: true }).click();
  await expect(second.getByRole("alertdialog")).toContainText("另一个页面");
  expect((await stored(page)).environments[0].name).toBe("跨页最新修改");
  await second.close();
});

test("a large create batch can be cancelled and reopening retains only completed items", async ({ page }) => {
  await page.getByRole("button", { name: "新建环境", exact: true }).click();
  await page.getByLabel("环境名称", { exact: true }).fill("批次取消");
  await advanced(page);
  await page.getByLabel("创建数量").fill("1000");
  await page.getByRole("button", { name: "创建 1000 个环境", exact: true }).click();
  await page.getByRole("button", { name: "取消余下任务", exact: true }).click();
  await expect(page.getByRole("button", { name: "取消余下任务", exact: true })).toHaveCount(0);
  const count = (await stored(page)).environments.filter(e => e.name.startsWith("批次取消")).length;
  expect(count).toBeGreaterThan(0);
  expect(count).toBeLessThan(1000);
  await page.reload();
  expect((await stored(page)).environments.filter(e => e.name.startsWith("批次取消")).length).toBe(count);
});

test("all six pages and narrow-window creation remain usable", async ({ page }) => {
  for (const route of ["proxies", "kernels", "backups", "activity", "guide", "environments"]) {
    await page.goto(`/#/${route}`);
    await expect(page.locator("main")).toBeVisible();
  }
  await page.setViewportSize({ width: 390, height: 844 });
  await page.getByRole("button", { name: "新建环境", exact: true }).click();
  await expect(page.getByLabel("环境名称", { exact: true })).toBeVisible();
  await page.getByLabel("环境名称", { exact: true }).fill("窄窗口合成环境");
  await expect(page.getByLabel("绑定代理")).toBeVisible();
  await expect(page.getByRole("button", { name: "创建并打开", exact: true })).toBeInViewport();
  await page.getByRole("button", { name: "创建并打开", exact: true }).click();
  await expect.poll(async () => (await stored(page)).environments.find(e => e.name === "窄窗口合成环境")?.status).toBe("running");
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true);
  await page.getByLabel("搜索环境").fill("窄窗口合成环境");
  const row = page.getByRole("row").filter({ hasText: "窄窗口合成环境" });
  await row.getByRole("button", { name: "关闭", exact: true }).click();
  await expect(row).toContainText("待启动");
  await page.getByRole("button", { name: "新建环境", exact: true }).click();
  await page.keyboard.press("Escape");
  await expect(page.getByRole("dialog")).toHaveCount(0);
});

test("one window selects service kernels and saves an automatically generated fingerprint", async ({ page }) => {
  const initial = await stored(page);
  initial.kernels.push({ id: "synthetic-other-build", version: "151.0.9000.11", available: true, source: "fingerprint-chromium", note: "合成服务版本，不是本机安装证据" });
  await page.evaluate(({ key, initial }) => localStorage.setItem(key, JSON.stringify(initial)), { key: STORAGE_KEY, initial });
  await page.reload();
  await page.getByRole("button", { name: "新建环境", exact: true }).click();
  const editor = page.getByRole("dialog", { name: "新建浏览器环境" });
  await expect(editor.getByLabel("环境名称", { exact: true })).toBeVisible();
  await expect(editor.getByLabel("绑定代理")).toBeVisible();
  await expect(editor.getByRole("button", { name: "换一套", exact: true })).toBeVisible();
  await expect(editor).not.toContainText("批次计划");
  await editor.getByLabel("环境名称", { exact: true }).fill("自动指纹合成环境");
  await editor.getByLabel("分组", { exact: true }).fill("合成分组");
  await editor.getByLabel("浏览器内核").selectOption("synthetic-other-build");
  const original = await editor.getByLabel("固定指纹种子").inputValue();
  await editor.getByRole("button", { name: "换一套", exact: true }).click();
  await expect(editor.getByLabel("固定指纹种子")).not.toHaveValue(original);
  const seed = await editor.getByLabel("固定指纹种子").inputValue();
  expect(initial.environments.map(e => e.seed)).not.toContain(seed);
  await editor.getByRole("button", { name: "创建", exact: true }).click();
  await expect(editor).toHaveCount(0);
  const saved = (await stored(page)).environments.find(e => e.name === "自动指纹合成环境")!;
  expect(saved).toMatchObject({ seed, coreId: "synthetic-other-build", proxyId: "", group: "合成分组", status: "ready" });
  await page.getByLabel("搜索环境").fill(saved.name);
  const row = page.getByRole("row").filter({ hasText: saved.name });
  await expect(row).toContainText("151.0.9000.11");
  await expect(row).toContainText("直连");
  await expect(row).toContainText("合成分组");
  await row.getByRole("button", { name: "打开", exact: true }).click();
  await expect(row).toContainText("运行中");
  await row.getByRole("button", { name: "关闭", exact: true }).click();
  await expect(row).toContainText("待启动");
  await row.getByRole("button", { name: "打开", exact: true }).click();
  await expect(row).toContainText("运行中");
  await page.reload();
  expect((await stored(page)).environments.find(e => e.id === saved.id)).toMatchObject({ seed, coreId: "synthetic-other-build", group: "合成分组" });
});

test("proxy import returns to the unchanged environment draft", async ({ page }) => {
  await page.getByRole("button", { name: "新建环境", exact: true }).click();
  await page.getByLabel("环境名称", { exact: true }).fill("代理往返草稿");
  await page.getByLabel("分组", { exact: true }).fill("合成往返分组");
  const seed = await page.getByLabel("固定指纹种子").inputValue();
  const kernel = await page.getByLabel("浏览器内核").inputValue();
  await page.getByRole("button", { name: "导入代理", exact: true }).click();
  await page.keyboard.press("Escape");
  await expect(page.getByRole("dialog", { name: "新建浏览器环境" })).toBeVisible();
  await expect(page.getByLabel("环境名称", { exact: true })).toHaveValue("代理往返草稿");
  await page.getByRole("button", { name: "导入代理", exact: true }).click();
  await page.getByLabel("代理文本").fill("socks5://192.0.2.99:1080");
  await page.getByRole("button", { name: "解析预览", exact: true }).click();
  await denyWrites(page);
  await page.getByRole("button", { name: /导入 1 个代理/ }).click();
  await expect(page.locator(".toast-error")).toContainText("未保存");
  await expect(page.getByLabel("代理文本")).toHaveValue("socks5://192.0.2.99:1080");
  expect((await stored(page)).proxies.some(p => p.host === "192.0.2.99")).toBe(false);
  await restoreWrites(page);
  await page.getByRole("button", { name: /导入 1 个代理/ }).click();
  await expect(page.getByRole("dialog", { name: "新建浏览器环境" })).toBeVisible();
  await expect(page.getByLabel("环境名称", { exact: true })).toHaveValue("代理往返草稿");
  await expect(page.getByLabel("分组", { exact: true })).toHaveValue("合成往返分组");
  await expect(page.getByLabel("固定指纹种子")).toHaveValue(seed);
  await expect(page.getByLabel("浏览器内核")).toHaveValue(kernel);
  await expect(page.getByRole("button", { name: "导入代理", exact: true })).toBeFocused();
  const imported = (await stored(page)).proxies.find(p => p.host === "192.0.2.99")!;
  await page.getByLabel("绑定代理").selectOption(imported.id);
  await page.getByRole("button", { name: "创建", exact: true }).click();
  await expect.poll(async () => (await stored(page)).environments.find(e => e.name === "代理往返草稿")?.proxyId).toBe(imported.id);
});

test("group and search keep a precise batch scope and failures remain retryable", async ({ page }) => {
  await page.getByLabel("筛选分组").selectOption("日常运营");
  await expect(page.getByRole("row").filter({ hasText: "北美选品环境" })).toHaveCount(0);
  await page.getByLabel("搜索环境").fill("没有这个合成环境");
  await expect(page.getByRole("heading", { name: /没有.*环境|未找到/ })).toBeVisible();
  await page.getByRole("button", { name: "清除筛选", exact: true }).click();
  await page.getByLabel("选择 北美主店", { exact: true }).check();
  await page.getByLabel("选择 欧洲家居店", { exact: true }).check();
  await page.getByRole("button", { name: "批量打开", exact: true }).click();
  const success = page.getByRole("row").filter({ hasText: "北美主店" });
  const failure = page.getByRole("row").filter({ hasText: "欧洲家居店" });
  await expect(success).toContainText("运行中");
  await expect(failure).toContainText("需处理");
  const after = await stored(page);
  expect(after.environments[0].status).toBe("running");
  expect(after.environments[2]).toMatchObject({ status: "error", proxyId: "px-de", seed: "162776995" });
  expect(after.environments.filter(e => e.status === "running")).toHaveLength(1);
  await page.getByRole("button", { name: "批量关闭", exact: true }).click();
  await expect(success).toContainText("待启动");
  await failure.getByRole("button", { name: /打开|重试/, exact: true }).click();
  await expect(failure).toContainText("需处理");
  expect((await stored(page)).environments[2].proxyId).toBe("px-de");
});

test("demo persistence failure finishes item feedback, preserves partial progress and allows retry", async ({ page }) => {
  await page.getByLabel("选择 北美主店", { exact: true }).check();
  await page.getByLabel("选择 英国精品店", { exact: true }).check();
  await page.getByLabel("选择 日本生活馆", { exact: true }).check();
  await denyWritesAfter(page, 2);
  await page.getByRole("button", { name: "批量打开", exact: true }).click();
  const results = page.getByRole("region", { name: "逐项操作结果" });
  await expect(results.locator(".outcome-success")).toHaveCount(1);
  await expect(results.locator(".outcome-error")).toHaveCount(1);
  await expect(results.locator(".outcome-skipped")).toHaveCount(1);
  await expect(results).toContainText("未保存");
  await expect(results.getByRole("button", { name: "收起结果" })).toBeEnabled();
  expect((await stored(page)).environments.filter(e => e.status === "running").map(e => e.id)).toEqual(["env-1"]);
  await restoreWrites(page);
  await results.getByRole("button", { name: "英国精品店 重试打开", exact: true }).click();
  await expect(page.getByRole("row").filter({ hasText: "英国精品店" })).toContainText("运行中");
  const beforeClose = await stored(page);
  await denyWrites(page);
  await page.getByRole("row").filter({ hasText: "英国精品店" }).getByRole("button", { name: "关闭", exact: true }).click();
  await expect(results.getByRole("button", { name: "英国精品店 重试关闭", exact: true })).toBeVisible();
  await expect(results.getByRole("button", { name: "收起结果" })).toBeEnabled();
  expect(await stored(page)).toEqual(beforeClose);
  await restoreWrites(page);
  await results.getByRole("button", { name: "英国精品店 重试关闭", exact: true }).click();
  await expect(page.getByRole("row").filter({ hasText: "英国精品店" })).toContainText("待启动");

  // A second-stage write failure leaves a persisted transitional record;
  // retry must finish that exact synthetic action, not skip it as active.
  const japan = page.getByRole("row").filter({ hasText: "日本生活馆" });
  await denyWritesAfter(page, 1);
  await japan.getByRole("button", { name: "打开", exact: true }).click();
  await expect(results.getByRole("button", { name: "日本生活馆 重试打开", exact: true })).toBeVisible();
  expect((await stored(page)).environments.find(e => e.id === "env-4")?.status).toBe("starting");
  await restoreWrites(page);
  await results.getByRole("button", { name: "日本生活馆 重试打开", exact: true }).click();
  await expect(japan).toContainText("运行中");
  await denyWritesAfter(page, 1);
  await japan.getByRole("button", { name: "关闭", exact: true }).click();
  await expect(results.getByRole("button", { name: "日本生活馆 重试关闭", exact: true })).toBeVisible();
  expect((await stored(page)).environments.find(e => e.id === "env-4")?.status).toBe("stopping");
  await restoreWrites(page);
  await results.getByRole("button", { name: "日本生活馆 重试关闭", exact: true }).click();
  await expect(japan).toContainText("待启动");
  await results.getByRole("button", { name: "收起结果" }).click();
  await expect(results).toHaveCount(0);
});
