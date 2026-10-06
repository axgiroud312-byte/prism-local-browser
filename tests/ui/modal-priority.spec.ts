import { expect, test } from "@playwright/test";
import type { WorkspaceView } from "../../src/application/contract";
import { localPagesNativeBridge } from "./fixtures/local-pages-native-bridge";

test.beforeEach(async ({ page, baseURL }) => {
  const origin = new URL(baseURL!).origin;
  await page.route("**/*", route => new URL(route.request().url()).origin === origin ? route.continue() : route.abort());
  await page.routeWebSocket("**/*", socket => socket.close());
});

for (const viewport of [{ width: 1440, height: 900 }, { width: 1280, height: 800 }]) {
  test(`nested diagnostic preserves lower Cookie input, focus and scroll ownership at ${viewport.width}x${viewport.height}`, async ({ page }) => {
    await page.setViewportSize(viewport);
    await localPagesNativeBridge(page);
    await page.goto("/#/environments");
    await expect(page.getByRole("button", { name: "刷新环境列表", exact: true })).toBeEnabled();
    await page.evaluate(() => {
      const fixture = (window as unknown as { __localPagesFixture: { workspace: WorkspaceView } }).__localPagesFixture;
      const environment = fixture.workspace.state.environments[0];
      environment.status = "running";
      fixture.workspace.runtimeSessions![environment.id] = {
        mode: "native", environmentId: environment.id, sessionId: "synthetic-cookie-session",
        operationId: "synthetic-cookie-running", state: "running", revision: 1, fingerprintRevision: 1,
        kernelId: environment.coreId, userDataRef: `environments/${environment.id}/user-data`,
        networkPolicy: "proxy", proxyId: environment.proxyId, canControl: true,
        canForce: false, needsReconcile: false, persistencePending: false,
      };
    });
    await page.getByRole("button", { name: "刷新环境列表", exact: true }).click();
    await page.getByRole("button", { name: "工作环境 A 更多操作", exact: true }).click();
    await page.getByRole("button", { name: "导入 Cookie", exact: true }).click();
    const cookie = page.getByRole("dialog", { name: "导入真实环境 Cookie", exact: true, includeHidden: true });
    const input = cookie.getByLabel("Cookie 内容", { exact: true });
    const text = '[{"name":"synthetic","value":"example-only","domain":"example.test"}]';
    await input.fill(text);
    await page.evaluate(() => {
      (window as unknown as { __localPagesFixture: { workspace: WorkspaceView } }).__localPagesFixture.workspace.issue = {
        code: "NATIVE_UNAVAILABLE", message: "合成读取故障，保留下层输入", retryable: true,
      };
    });
    const blocker = page.getByRole("alertdialog", { name: "工作区需要处理", exact: true, includeHidden: true });
    await expect(blocker).toBeVisible();
    await expect(cookie.locator("..")).toHaveCSS("background-color", "rgba(0, 0, 0, 0)");
    await blocker.getByRole("button", { name: "生成诊断预览", exact: true }).click();
    const diagnostic = page.getByRole("dialog", { name: "脱敏诊断预览", exact: true });
    await expect(diagnostic).toBeVisible();
    await expect.poll(() => diagnostic.evaluate(element => {
      const rect = element.getBoundingClientRect();
      return element.contains(document.elementFromPoint(rect.x + rect.width / 2, rect.y + rect.height / 2));
    })).toBe(true);
    for (const key of ["Tab", "Shift+Tab", "Control+k"]) {
      await page.keyboard.press(key);
      await expect.poll(() => diagnostic.evaluate(element => element.contains(document.activeElement))).toBe(true);
      await expect(input).toHaveValue(text);
      expect(await page.evaluate(() => document.body.style.overflow)).toBe("hidden");
    }
    await page.keyboard.press("Escape");
    await expect(diagnostic).toHaveCount(0);
    await expect(cookie).toBeAttached();
    await expect(input).toHaveValue(text);
    await expect.poll(() => blocker.evaluate(element => element.contains(document.activeElement))).toBe(true);
    expect(await page.evaluate(() => document.body.style.overflow)).toBe("hidden");
    await page.keyboard.press("Escape");
    await expect(cookie).toBeAttached();
    await page.evaluate(() => {
      (window as unknown as { __localPagesFixture: { workspace: WorkspaceView } }).__localPagesFixture.workspace.issue = undefined;
    });
    await expect(blocker).toHaveCount(0);
    await expect(cookie.locator("..")).toHaveCSS("background-color", "rgba(0, 0, 0, 0.4)");
    await expect(input).toBeEditable();
    await input.fill(`${text} `);
    await page.keyboard.press("Escape");
    await expect(cookie).toHaveCount(0);
    await expect(page.locator(".sidebar")).toHaveJSProperty("inert", false);
    await expect(page.locator(".main-shell")).toHaveJSProperty("inert", false);
    expect(await page.evaluate(() => document.body.style.overflow)).toBe("");
    await page.getByRole("link", { name: "代理管理", exact: true }).click();
    await expect(page).toHaveURL(/#\/proxies$/);
  });
}
