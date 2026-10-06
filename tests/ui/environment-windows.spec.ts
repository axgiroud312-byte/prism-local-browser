import { test, expect, type Page } from "@playwright/test";
import { seedState, STORAGE_KEY, type State } from "../../src/domain.ts";

async function stored(page: Page): Promise<State> {
  return page.evaluate(key => JSON.parse(localStorage.getItem(key)!), STORAGE_KEY);
}
test.beforeEach(async ({ page }) => {
  const state = seedState();
  state.environments.forEach(environment => { environment.status = "ready"; });
  await page.goto("/#/environments");
  await page.evaluate(({ state, key }) => localStorage.setItem(key, JSON.stringify(state)), { state, key: STORAGE_KEY });
  await page.reload();
});

for (const viewport of [{ width: 1440, height: 900 }, { width: 1280, height: 800 }]) {
  test(`environment windows: supported sections scroll within the fixed drawer ${viewport.width}`, async ({ page }) => {
    await page.setViewportSize(viewport);
    await page.getByRole("button", { name: "新建环境", exact: true }).click();
    const dialog = page.getByRole("dialog", { name: "新建浏览器环境" });
    const bounds = await dialog.boundingBox();
    expect(bounds).toMatchObject({ x: viewport.width - 660, y: 40, width: 660, height: viewport.height - 48 });
    await expect(dialog.getByRole("navigation", { name: "环境配置分区" })).toBeVisible();
    await expect(dialog.getByRole("heading", { name: "基础设置", exact: true })).toBeVisible();
    await expect(dialog.locator("[data-environment-section]")).toHaveCount(4);
    const seed = await page.getByLabel("固定指纹种子").inputValue();
    await dialog.getByRole("button", { name: "指纹设置", exact: true }).click();
    await expect(dialog.getByRole("heading", { name: "指纹设置", exact: true })).toBeInViewport();
    await expect.poll(() => dialog.locator(".drawer-body, .environment-dialog-body").evaluate(element => element.scrollTop)).toBeGreaterThan(0);
    await expect(dialog.getByRole("button", { name: "创建并打开", exact: true })).toBeInViewport();
    await page.getByLabel("绑定代理").selectOption("px-gb");
    await expect(page.getByLabel("固定指纹种子")).toHaveValue(seed);
    await dialog.getByRole("button", { name: "基础设置", exact: true }).click();
    await expect(page.getByLabel("环境名称", { exact: true })).toBeInViewport();
  });
}

test("environment windows: explicit regeneration and cancellation do not mutate saved identity", async ({ page }) => {
  const before = await stored(page);
  await page.getByRole("button", { name: "北美主店 更多操作", exact: true }).click();
  await page.getByRole("button", { name: "编辑环境", exact: true }).click();
  const seed = await page.getByLabel("固定指纹种子").inputValue();
  await page.getByLabel("环境名称", { exact: true }).fill("取消的合成草稿");
  await page.getByLabel("备注", { exact: true }).fill("应当丢弃");
  await page.getByRole("button", { name: "换一套", exact: true }).click();
  await expect(page.getByLabel("固定指纹种子")).not.toHaveValue(seed);
  expect(await stored(page)).toEqual(before);
  page.once("dialog", dialog => dialog.accept());
  await page.getByRole("button", { name: "取消", exact: true }).click();
  await expect(page.getByRole("dialog")).toHaveCount(0);
  expect(await stored(page)).toEqual(before);
});

test("environment windows: kernel popup uses available records and Escape only closes that popup", async ({ page }) => {
  const kernels = (await stored(page)).kernels.filter(kernel => kernel.available);
  await page.getByRole("button", { name: "新建环境", exact: true }).click();
  const dialog = page.getByRole("dialog", { name: "新建浏览器环境" });
  await dialog.getByRole("button", { name: "指纹设置", exact: true }).click();
  const seed = await page.getByLabel("固定指纹种子").inputValue();
  const select = dialog.getByLabel("浏览器内核", { exact: true });
  await select.press("ArrowDown");
  const list = dialog.getByRole("listbox", { name: "可用服务内核" });
  await expect(list.getByRole("option")).toHaveCount(kernels.length);
  await expect(list).toHaveCSS("width", "264px");
  await page.keyboard.press("Escape");
  await expect(list).toHaveCount(0);
  await expect(dialog).toBeVisible();
  await expect(select).toBeFocused();
  await select.press("ArrowDown");
  await page.keyboard.press("End");
  await page.keyboard.press("Enter");
  await expect(select).toHaveValue(kernels.at(-1)!.id);
  await expect(page.getByLabel("固定指纹种子")).toHaveValue(seed);
  await expect(list).toHaveCount(0);
});

test("environment windows: Tab dismisses the kernel popup without dismissing the editor", async ({ page }) => {
  await page.getByRole("button", { name: "新建环境", exact: true }).click();
  const dialog = page.getByRole("dialog", { name: "新建浏览器环境" });
  await dialog.getByRole("button", { name: "指纹设置", exact: true }).click();
  await dialog.getByLabel("浏览器内核", { exact: true }).press("ArrowDown");
  await expect(dialog.getByRole("listbox")).toBeVisible();
  await page.keyboard.press("Tab");
  await expect(dialog.getByRole("listbox")).toHaveCount(0);
  await expect(dialog).toBeVisible();
  expect(await dialog.evaluate(element => element.contains(document.activeElement))).toBe(true);
});
