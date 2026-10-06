import { test, expect, type Page } from "@playwright/test";
import { STORAGE_KEY, type State } from "../../src/domain.ts";
import { referenceWorkspace } from "./fixtures/reference-workspace.ts";

async function stored(page: Page): Promise<State> {
  return page.evaluate(key => JSON.parse(localStorage.getItem(key)!), STORAGE_KEY);
}
test.beforeEach(async ({ page }) => {
  await page.addInitScript(({ state, key }) => {
    if (!sessionStorage.getItem("reference-seeded")) {
      localStorage.setItem(key, JSON.stringify(state)); sessionStorage.setItem("reference-seeded", "yes");
    }
  }, { state: referenceWorkspace(), key: STORAGE_KEY });
  await page.goto("/#/environments");
});

test("reference shell geometry has isolated row scrolling and pinned pagination at both sizes", async ({ page }) => {
  for (const viewport of [{ width: 1440, height: 900 }, { width: 1280, height: 800 }]) {
    await page.setViewportSize(viewport);
    await expect(page.locator(".stats-grid, .workspace, .breadcrumb")).toHaveCount(0);
    expect((await page.locator(".sidebar").boundingBox())?.width).toBe(200);
    expect((await page.locator(".topbar").boundingBox())?.height).toBe(42);
    expect((await page.getByRole("link", { name: "浏览器环境", exact: true }).boundingBox())?.y).toBe(144);
    expect((await page.locator(".split-button").boundingBox())?.width).toBe(134);
    const header = await page.locator(".environment-table thead").boundingBox();
    expect(header?.x).toBe(220); expect(header?.y).toBeCloseTo(162.86, 0); expect(header?.height).toBe(40);
    const rows = page.locator(".environment-table tbody tr");
    await expect(rows).toHaveCount(10);
    const first = (await rows.nth(0).boundingBox())!, second = (await rows.nth(1).boundingBox())!;
    expect(first.height).toBe(48); expect(second.y - first.y).toBe(58);
    expect(await page.locator(".environment-table tbody").evaluate(element => element.scrollHeight)).toBe(590);
    const footer = (await page.locator(".environment-pagination").boundingBox())!;
    await page.locator(".environment-table tbody").evaluate(element => { element.scrollTop = element.scrollHeight; });
    expect((await page.locator(".environment-pagination").boundingBox())?.y).toBe(footer.y);
    await expect(rows.last()).toBeInViewport();
    await page.locator(".environment-table tbody").evaluate(element => { element.scrollTop = 0; });
  }
});

test("selection survives filtering and paging without expanding the exact IDs; group edits preserve identity", async ({ page }) => {
  const before = await stored(page);
  await page.getByLabel("选择 工作环境 A", { exact: true }).check();
  await page.getByRole("button", { name: "下一页", exact: true }).click();
  await page.getByLabel("选择 工作环境 B 11", { exact: true }).check();
  await page.getByLabel("搜索环境", { exact: true }).fill("工作环境 A");
  await page.getByRole("button", { name: "调整分组", exact: true }).click();
  await page.getByLabel("分组名称", { exact: true }).fill("精确范围分组");
  await page.getByRole("button", { name: "应用到所选环境", exact: true }).click();
  const after = await stored(page);
  expect(after.environments.filter(e => e.group === "精确范围分组").map(e => e.id)).toEqual([before.environments[0].id, before.environments[10].id]);
  expect(after.environments.map(e => [e.id, e.seed, e.coreId, e.proxyId, e.cookies])).toEqual(before.environments.map(e => [e.id, e.seed, e.coreId, e.proxyId, e.cookies]));
  await page.getByRole("button", { name: "批量打开", exact: true }).click();
  await expect.poll(async () => (await stored(page)).environments.filter(e => e.status === "running").map(e => e.id)).toEqual([before.environments[0].id, before.environments[10].id]);
  await page.getByRole("button", { name: "批量关闭", exact: true }).click();
  await expect.poll(async () => (await stored(page)).environments.filter(e => e.status === "running").length).toBe(0);
});

test("supported filter popups and portalled row menu are operable and return focus", async ({ page }) => {
  await page.getByRole("button", { name: "分组筛选", exact: true }).click();
  await page.getByRole("button", { name: "测试环境", exact: true }).click();
  await expect(page.locator(".environment-table tbody tr")).toHaveCount(6);
  await page.getByRole("button", { name: "高级搜索", exact: true }).click();
  await page.getByLabel("高级搜索关键词").fill("不存在的合成名字");
  await page.getByRole("button", { name: "开始搜索", exact: true }).click();
  await expect(page.getByRole("heading", { name: "没有符合条件的环境" })).toBeVisible();
  await page.getByRole("button", { name: "清除筛选", exact: true }).click();
  const trigger = page.getByRole("button", { name: "工作环境 A 更多操作", exact: true });
  await trigger.click();
  await expect(page.getByRole("button", { name: "导入 Cookie", exact: true })).toBeVisible();
  await page.keyboard.press("Escape"); await expect(trigger).toBeFocused();
  await trigger.click(); await page.getByLabel("搜索环境").click();
  await expect(page.locator(".environment-row-menu")).toHaveCount(0);
});

test("derived group management renames real members and does not manufacture group entities", async ({ page }) => {
  const before = await stored(page);
  await page.getByRole("link", { name: "分组管理", exact: true }).click();
  await expect(page.getByText("分组来自环境记录", { exact: false })).toBeVisible();
  await expect(page.getByRole("button", { name: "添加空分组", exact: true })).toHaveCount(0);
  await page.getByRole("button", { name: "修改 工作环境 分组", exact: true }).click();
  await page.getByLabel("分组名称", { exact: true }).fill("工作分组重命名");
  await page.getByRole("button", { name: "应用到所选环境", exact: true }).click();
  expect((await stored(page)).environments.filter(e => e.group === "工作分组重命名")).toHaveLength(6);
  expect((await stored(page)).environments.map(e => e.seed)).toEqual(before.environments.map(e => e.seed));
});
