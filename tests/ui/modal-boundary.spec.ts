import { expect, test } from "@playwright/test";
import { nativeReferenceBridge } from "./fixtures/native-reference-bridge";

test.beforeEach(async ({ page, baseURL }) => {
  const origin = new URL(baseURL!).origin;
  await page.route("**/*", route => new URL(route.request().url()).origin === origin ? route.continue() : route.abort("blockedbyclient"));
  await page.routeWebSocket("**/*", socket => socket.close());
});

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

test("a workspace fault blocks Escape from closing a lower native Cookie dialog", async ({ page }) => {
  await nativeReferenceBridge(page, true);
  await page.goto("/#/environments");
  await page.getByRole("button", { name: "工作环境 A 更多操作", exact: true }).click();
  await page.getByRole("button", { name: "导入 Cookie", exact: true }).click();
  const cookie = page.getByRole("dialog", { name: /Cookie/, includeHidden: true });
  await expect(cookie).toBeVisible();
  await page.evaluate(() => {
    // The existing synthetic active-session refresh observes a host fault.
    // No click through an inert layer and no direct/native RPC are needed.
    (window as unknown as { __referenceNative: { view: { issue?: unknown } } }).__referenceNative.view.issue = {
      code: "NATIVE_UNAVAILABLE", message: "合成工作区读取故障；原弹窗输入保留", retryable: true,
    };
  });
  const blocker = page.getByRole("alertdialog", { name: "工作区需要处理", exact: true });
  await expect(blocker).toBeVisible();
  await page.keyboard.press("Escape");
  await expect(blocker).toBeVisible();
  await expect(cookie).toBeAttached();
  await expect.poll(() => blocker.evaluate(element => element.contains(document.activeElement))).toBe(true);
});
