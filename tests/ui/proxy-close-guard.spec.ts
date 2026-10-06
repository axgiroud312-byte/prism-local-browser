import { expect, test } from "@playwright/test";
import { proxyKernelBridge } from "./fixtures/proxy-kernel";
import { openProxyKernelHarness } from "./fixtures/proxy-kernel-harness";

test("closing a standalone proxy portal returns focus to its connected opener after removal", async ({ page }) => {
  await page.routeWebSocket("**/*", socket => socket.close());
  await proxyKernelBridge(page);
  await openProxyKernelHarness(page);
  const opener = page.getByRole("button", { name: "批量添加代理", exact: true });
  await opener.click();
  const dialog = page.getByRole("dialog", { name: "批量添加代理", exact: true });
  await expect(dialog).toBeVisible();
  await page.keyboard.press("Escape");
  await expect(dialog).toHaveCount(0);
  await expect(opener).toBeFocused();
  await opener.click();
  await dialog.getByRole("button", { name: "取消", exact: true }).click();
  await expect(dialog).toHaveCount(0);
  await expect(opener).toBeFocused();
});

for (const method of ["Proxy.ParseImport", "Proxy.CommitImport"]) {
  test(`an in-flight ${method} cannot hide the only import owner`, async ({ page }) => {
    await page.routeWebSocket("**/*", socket => socket.close());
    await proxyKernelBridge(page);
    await openProxyKernelHarness(page);
    await page.getByRole("button", { name: "批量添加代理", exact: true }).click();
    const dialog = page.getByRole("dialog", { name: "批量添加代理", exact: true });
    await dialog.getByLabel("原始代理导入文本").fill("http://203.0.113.71:8080");
    if (method === "Proxy.CommitImport") await dialog.getByRole("button", { name: "解析预览", exact: true }).click();
    await page.evaluate(method => {
      const host = (window as unknown as { go: { main: { DesktopApp: { Call(request: { method: string }): Promise<unknown> } } } }).go.main.DesktopApp;
      const original = host.Call;
      host.Call = request => request.method !== method ? original(request) : new Promise(resolve => {
        Object.assign(window, { __proxyBusyHeld: true, __releaseProxyBusy: () => resolve(original(request)) });
      });
    }, method);
    await dialog.getByRole("button", { name: method === "Proxy.ParseImport" ? "解析预览" : "保存所选有效行", exact: true }).click();
    await expect.poll(() => page.evaluate(() => (window as unknown as { __proxyBusyHeld: boolean }).__proxyBusyHeld)).toBe(true);
    await expect(dialog.getByRole("button", { name: "取消", exact: true })).toBeDisabled();
    await expect(dialog.getByRole("button", { name: "关闭批量添加代理", exact: true })).toBeDisabled();
    await page.keyboard.press("Escape");
    await expect(dialog).toBeVisible();
    await page.mouse.click(2, 2);
    await expect(dialog).toBeVisible();
    expect(await page.evaluate(() => document.body.style.overflow)).toBe("hidden");
    await page.evaluate(() => (window as unknown as { __releaseProxyBusy(): void }).__releaseProxyBusy());
    if (method === "Proxy.ParseImport") {
      await expect(dialog.getByRole("button", { name: "取消", exact: true })).toBeEnabled();
      await dialog.getByRole("button", { name: "取消", exact: true }).click();
    }
    await expect(dialog).toHaveCount(0);
    expect(await page.evaluate(() => document.body.style.overflow)).toBe("");
  });
}
