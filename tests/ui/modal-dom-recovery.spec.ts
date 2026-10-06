import { expect, test, type Locator } from "@playwright/test";
import { seedState, STORAGE_KEY } from "../../src/domain";

const ownsFocus = (dialog: Locator) => expect.poll(() => dialog.evaluate(element => element.contains(document.activeElement))).toBe(true);
test.beforeEach(async ({ page, baseURL }) => {
  const origin = new URL(baseURL!).origin;
  await page.route("**/*", route => new URL(route.request().url()).origin === origin ? route.continue() : route.abort());
  await page.routeWebSocket("**/*", socket => socket.close());
  const state = seedState(); state.environments.forEach(environment => { environment.status = "ready"; });
  await page.addInitScript(({ state, key }) => localStorage.setItem(key, JSON.stringify(state)), { state, key: STORAGE_KEY });
});

for (const viewport of [{ width: 1440, height: 900 }, { width: 1280, height: 800 }]) {
  test.describe(`${viewport.width}x${viewport.height} actual App focus recovery`, () => {
    test.use({ viewport });
    test("single proxy mode replaces its original text field without losing modal focus", async ({ page }) => {
      await page.goto("/#/proxies");
      await page.getByRole("button", { name: "添加代理", exact: true }).click();
      const dialog = page.getByRole("dialog", { name: "添加代理", exact: true });
      await expect(dialog).toBeVisible(); await ownsFocus(dialog);
      await page.keyboard.press("Escape"); await expect(dialog).toHaveCount(0);
      await expect(page.getByRole("button", { name: "添加代理", exact: true })).toBeFocused();
    });
    test("file mode replaces the clicked control and still owns focus before the next key", async ({ page }) => {
      await page.goto("/#/proxies");
      await page.getByRole("button", { name: "批量添加代理", exact: true }).click();
      await page.getByRole("button", { name: "选择文本文件", exact: true }).click();
      const dialog = page.getByRole("dialog", { name: "批量导入代理", exact: true });
      await expect(dialog).toBeVisible(); await ownsFocus(dialog);
      await page.keyboard.press("Tab"); await ownsFocus(dialog);
      await page.keyboard.press("Escape"); await expect(dialog).toHaveCount(0);
    });
    test("failed creation repairs disabled-submit blur while retaining the original draft", async ({ page }) => {
      await page.goto("/#/environments");
      await page.getByRole("button", { name: "新建环境", exact: true }).click();
      const dialog = page.getByRole("dialog", { name: "新建浏览器环境", exact: true });
      await dialog.getByLabel("环境名称", { exact: true }).fill("合成失败草稿");
      const seed = await dialog.getByLabel("固定指纹种子").inputValue();
      await page.evaluate(() => { Storage.prototype.setItem = () => { throw new DOMException("synthetic failure", "QuotaExceededError"); }; });
      await dialog.getByRole("button", { name: "创建", exact: true }).click();
      await expect(dialog.getByRole("alert")).toBeVisible(); await ownsFocus(dialog);
      await expect(dialog.getByLabel("固定指纹种子")).toHaveValue(seed);
      await expect(dialog.getByLabel("环境名称", { exact: true })).toHaveValue("合成失败草稿");
    });
    test("invalid demo backup import repairs disabled-submit blur without closing or restoring", async ({ page }) => {
      await page.goto("/#/backups");
      await page.getByRole("button", { name: "导入快照文件", exact: true }).click();
      const dialog = page.getByRole("dialog", { name: "导入演示快照", exact: true });
      await dialog.getByLabel("演示快照文件").setInputFiles({ name: "synthetic-invalid.json", mimeType: "application/json", buffer: Buffer.from("{invalid") });
      await dialog.getByRole("button", { name: "校验并预览", exact: true }).click();
      await expect(dialog.getByRole("alert")).toBeVisible(); await ownsFocus(dialog);
      await expect(dialog).toContainText("synthetic-invalid.json");
      await page.keyboard.press("Escape"); await expect(dialog).toHaveCount(0);
      await expect(page.getByRole("button", { name: "导入快照文件", exact: true })).toBeFocused();
    });
  });
}
