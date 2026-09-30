import { test, expect } from "@playwright/test";

test("a broken desktop bridge is blocked in native mode, never replaced by DemoAdapter", async ({ page }) => {
  const errors: string[] = [];
  page.on("pageerror", e => errors.push(e.message));
  await page.addInitScript(() => {
    Object.assign(window, { runtime: {} });
    Object.defineProperty(window, "localStorage", { get() { throw new Error("demo storage must not be read in native mode"); } });
  });
  await page.goto("/#/environments");
  await expect(page.getByRole("alertdialog", { name: "工作区需要处理" })).toContainText("本地服务连接失败");
  await expect(page.getByText("本机桌面", { exact: true })).toBeVisible();
  await expect(page.getByText("北美主店", { exact: true })).toHaveCount(0);
  await expect(page.getByRole("button", { name: "重置演示工作区" })).toHaveCount(0);
  await page.getByRole("button", { name: "重新读取本机工作区" }).click();
  await expect(page.getByRole("alertdialog")).toContainText("本地服务连接失败");
  expect(errors).toEqual([]);
});
