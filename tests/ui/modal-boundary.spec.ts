import { expect, test } from "@playwright/test";

test("a broken native bridge keeps keyboard focus inside the blocking workspace dialog", async ({ page }) => {
  await page.addInitScript(() => {
    Object.defineProperty(window, "localStorage", { get() { throw new Error("native blocker must never fall back to demo storage"); } });
    (window as unknown as { go: unknown }).go = { main: { DesktopApp: { Call: async () => { throw new Error("synthetic unavailable bridge"); } } } };
  });
  await page.goto("/#/environments");
  const blocker = page.getByRole("alertdialog", { name: "工作区需要处理", exact: true });
  await expect(blocker).toBeVisible();
  await expect.poll(() => blocker.evaluate(element => element.contains(document.activeElement))).toBe(true);
  for (const key of ["Escape", "Control+k", "Tab", "Shift+Tab"]) {
    await page.keyboard.press(key);
    await expect(blocker).toBeVisible();
    await expect.poll(() => blocker.evaluate(element => element.contains(document.activeElement))).toBe(true);
  }
  await expect(page.getByRole("button", { name: "新建环境", exact: true })).toBeDisabled();
  await expect(page.getByText("北美主店", { exact: true })).toHaveCount(0);
});

test("the loading native blocker itself receives focus when no enabled action is available", async ({ page }) => {
  await page.addInitScript(() => {
    Object.defineProperty(window, "localStorage", { get() { throw new Error("loading native workspace must not use demo storage"); } });
    (window as unknown as { go: unknown }).go = { main: { DesktopApp: { Call: () => new Promise(() => {}) } } };
  });
  await page.goto("/#/environments");
  const blocker = page.getByRole("alertdialog", { name: "工作区需要处理", exact: true });
  await expect(blocker).toBeVisible();
  await expect(page.getByRole("button", { name: "重新读取本机工作区", exact: true })).toBeDisabled();
  await expect(blocker).toBeFocused();
  await page.keyboard.press("Tab");
  await expect(blocker).toBeFocused();
  await page.keyboard.press("Escape");
  await expect(blocker).toBeVisible();
  await expect(blocker).toBeFocused();
});
