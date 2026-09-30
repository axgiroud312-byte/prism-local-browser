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
  await page.getByRole("button", { name: "设备指纹", exact: true }).click();
  const seed = await page.getByLabel("固定指纹种子").inputValue();
  await page.getByRole("button", { name: "创建环境", exact: true }).click();
  await expect(page.getByRole("dialog")).toHaveCount(0);
  await expect.poll(async () => (await stored(page)).environments.some(e => e.name === "UI 合成环境")).toBe(true);
  await page.reload();
  await edit(page, "UI 合成环境");
  await page.getByLabel("环境名称", { exact: true }).fill("UI 修改环境");
  await page.getByRole("button", { name: "保存配置", exact: true }).click();
  await expect(page.getByText("环境配置已保存", { exact: true })).toBeVisible();
  await page.reload();
  const saved = (await stored(page)).environments.find(e => e.name === "UI 修改环境")!;
  expect(saved.seed).toBe(seed);
  expect(saved.proxyId).toBe("px-gb");
  expect(saved.coreId).toBe("core-148");
  expect(saved.cookies).toEqual([]);
  await expect(page.getByText("交互原型", { exact: false }).first()).toBeVisible();
  expect(errors).toEqual([]);
  await page.screenshot({ path: "output/goal/T01/created-edited.png", fullPage: true });
});

test("cancel regenerated edit leaves persistent state unchanged and restores focus", async ({ page }) => {
  const before = await stored(page);
  await edit(page, "北美主店");
  await page.getByRole("button", { name: "设备指纹", exact: true }).click();
  const seed = await page.getByLabel("固定指纹种子").inputValue();
  await page.getByRole("button", { name: "重新生成", exact: true }).click();
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

test("invalid and duplicate names, URL and quantity are rejected by the service", async ({ page }) => {
  await page.getByRole("button", { name: "新建环境", exact: true }).click();
  await page.getByRole("button", { name: "创建环境", exact: true }).click();
  await expect(page.getByRole("alert")).toContainText("名称");
  await page.getByLabel("环境名称", { exact: true }).fill("北美主店");
  await page.getByRole("button", { name: "创建环境", exact: true }).click();
  await expect(page.getByRole("alert")).toContainText("同名");
  await page.getByLabel("环境名称", { exact: true }).fill("校验样本");
  await page.getByLabel("创建数量").fill("0");
  await page.getByRole("button", { name: "创建环境", exact: true }).click();
  await expect(page.getByRole("alert")).toContainText("正整数");
  await page.getByLabel("创建数量").fill("1");
  await page.getByRole("button", { name: "浏览器偏好", exact: true }).click();
  await page.getByLabel("启动网址").fill("about:blank");
  await page.getByRole("button", { name: "创建环境", exact: true }).click();
  await expect(page.getByRole("alert")).toContainText("http");
  expect((await stored(page)).environments).toHaveLength(8);
});

test("storage write failure keeps the editor and old record; retry commits", async ({ page }) => {
  const before = await stored(page);
  await edit(page, "北美主店");
  await page.getByLabel("环境名称", { exact: true }).fill("失败后重试");
  await denyWrites(page);
  await page.getByRole("button", { name: "保存配置", exact: true }).click();
  await expect(page.getByRole("alert")).toContainText("未保存");
  await expect(page.getByRole("dialog", { name: "编辑浏览器环境" })).toBeVisible();
  expect(await stored(page)).toEqual(before);
  await expect(page.getByText("环境配置已保存", { exact: true })).toHaveCount(0);
  await restoreWrites(page);
  await page.getByRole("button", { name: "保存配置", exact: true }).click();
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
});

test("a second tab blocks the stale editor without overwriting the newer workspace", async ({ page, context }) => {
  const second = await context.newPage();
  await second.goto("/#/environments");
  await edit(second, "北美主店");
  await edit(page, "北美主店");
  await page.getByLabel("环境名称", { exact: true }).fill("跨页最新修改");
  await page.getByRole("button", { name: "保存配置", exact: true }).click();
  await expect(second.getByRole("alertdialog")).toContainText("另一个页面");
  expect((await stored(page)).environments[0].name).toBe("跨页最新修改");
  await second.close();
});

test("a large create batch can be cancelled and reopening retains only completed items", async ({ page }) => {
  await page.getByRole("button", { name: "新建环境", exact: true }).click();
  await page.getByLabel("环境名称", { exact: true }).fill("批次取消");
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
  await page.keyboard.press("Escape");
  await expect(page.getByRole("dialog")).toHaveCount(0);
});
