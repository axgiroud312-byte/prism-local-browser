import { expect, test, type Page } from "@playwright/test";
import { seedState, STORAGE_KEY } from "../../src/domain";
import type { WorkspaceView } from "../../src/application/contract";
import type { NativeRequest } from "../../src/application/wails-adapter";
import { nativeReferenceBridge, nativeReferenceView } from "./fixtures/native-reference-bridge";
import { localPagesNativeBridge } from "./fixtures/local-pages-native-bridge";

// Actual main.tsx/App, synthetic boundary only. Block product network and HMR;
// these checks cannot start/stop a native process or open a real file picker.
const errors = new WeakMap<Page, string[]>();
test.beforeEach(async ({ page, baseURL }) => {
  const messages: string[] = []; errors.set(page, messages);
  page.on("pageerror", error => messages.push(error.message));
  const origin = new URL(baseURL!).origin;
  await page.route("**/*", route => new URL(route.request().url()).origin === origin ? route.continue() : route.abort("blockedbyclient"));
  await page.routeWebSocket("**/*", socket => socket.close());
});
test.afterEach(async ({ page }) => { expect(errors.get(page)).toEqual([]); });

const warning = (page: Page, includeHidden = false) => page.getByRole("alertdialog", { name: "强制结束指定会话？", exact: true, includeHidden });
const calls = async (page: Page) => (await nativeReferenceView(page)).calls.filter(call => call.method === "Runtime.ForceStop");

async function ownedControlLost(page: Page, pending = false) {
  await nativeReferenceBridge(page, true);
  await page.addInitScript(pending => {
    const host = window as unknown as {
      __referenceNative: { view: WorkspaceView; calls: NativeRequest[] };
      go: { main: { DesktopApp: { Call(request: NativeRequest): Promise<unknown> } } };
      __continuationRecovery: { syntheticOnly: true; releaseForce(): void };
    };
    const view = host.__referenceNative.view, session = view.runtimeSessions!["synthetic-reference-6"];
    session.canControl = false;
    view.state.activities = [{ id: "synthetic-recovery-record", action: "合成正常关闭失败", target: "测试环境 C 6", detail: "合成控制通道丢失，仍持有准确会话；未执行真实进程操作。", time: "2026-10-06T01:00:00Z", result: "error", environmentId: session.environmentId, sessionId: session.sessionId }];
    let release: (() => void) | undefined;
    host.__continuationRecovery = { syntheticOnly: true, releaseForce() { release?.(); } };
    if (!pending) return;
    const original = host.go.main.DesktopApp.Call;
    host.go.main.DesktopApp.Call = request => {
      if (request.method !== "Runtime.ForceStop") return original(request);
      host.__referenceNative.calls.push(structuredClone(request));
      return new Promise(resolve => { release = () => resolve({ ok: false, mode: "native", error: { code: "SYNTHETIC_FORCE_NOT_EXECUTED", message: "合成请求已释放，未执行实际强制结束。", retryable: true } }); });
    };
  }, pending);
}

async function entry(page: Page, route: "environments" | "activity") {
  await page.goto(`/#/${route}`);
  if (route === "activity") {
    await page.getByRole("button", { name: "合成正常关闭失败 测试环境 C 6 详情", exact: true }).click();
    return page.getByRole("dialog", { name: "操作记录详情", exact: true }).getByRole("button", { name: "强制结束此会话", exact: true });
  }
  return page.getByRole("row").filter({ hasText: "测试环境 C 6" }).getByRole("button", { name: "强制结束", exact: true });
}

for (const route of ["environments", "activity"] as const) {
  test(`channel-lost ${route} force pending blocks duplicate actions and preserves exact request`, async ({ page }) => {
    await ownedControlLost(page, true);
    const action = await entry(page, route);
    await action.click(); await expect(warning(page)).toBeVisible();
    expect(await calls(page)).toHaveLength(0);
    // Two immediate DOM clicks also exercise the synchronous confirmation ref,
    // not just the later disabled paint or Playwright's actionability checks.
    await warning(page).getByRole("button", { name: "确认强制结束", exact: true }).evaluate(button => {
      (button as HTMLButtonElement).click(); (button as HTMLButtonElement).click();
    });
    await expect.poll(async () => (await calls(page)).length).toBe(1);
    await expect(warning(page)).toHaveCount(0); await expect(action).toBeDisabled();
    await action.evaluate(button => (button as HTMLButtonElement).click());
    await expect(warning(page)).toHaveCount(0);
    const requests = await calls(page);
    expect(requests).toHaveLength(1);
    expect(requests[0].payload).toEqual({ environmentId: "synthetic-reference-6", sessionId: "synthetic-session-6", requestId: expect.any(String) });
    expect((await nativeReferenceView(page)).view.runtimeSessions!["synthetic-reference-6"].pid).toBe(31006);
    await page.evaluate(() => (window as unknown as { __continuationRecovery: { releaseForce(): void } }).__continuationRecovery.releaseForce());
    await expect(action).toBeEnabled(); expect(await calls(page)).toEqual(requests);
  });

  test(`channel-lost ${route} force warning sends zero requests under a workspace fault and restores cancel focus`, async ({ page }) => {
    await ownedControlLost(page);
    const action = await entry(page, route);
    await action.click(); await expect(warning(page)).toBeVisible();
    await page.evaluate(() => {
      (window as unknown as { __referenceNative: { view: WorkspaceView } }).__referenceNative.view.issue = { code: "NATIVE_UNAVAILABLE", message: "合成工作区故障，不得消费会话警告。", retryable: true };
    });
    const blocker = page.getByRole("alertdialog", { name: "工作区需要处理", exact: true });
    await expect(blocker).toBeVisible(); await expect(warning(page, true)).toBeAttached();
    await warning(page, true).getByRole("button", { name: "确认强制结束", exact: true, includeHidden: true }).evaluate(button => (button as HTMLButtonElement).click());
    expect(await calls(page)).toHaveLength(0);
    for (const key of ["Escape", "Tab", "Shift+Tab", "Control+k"]) {
      await page.keyboard.press(key);
      expect(await blocker.evaluate(element => element.contains(document.activeElement))).toBe(true);
    }
    await expect(warning(page, true)).toBeAttached();
    await page.evaluate(() => { delete (window as unknown as { __referenceNative: { view: WorkspaceView } }).__referenceNative.view.issue; });
    await expect(blocker).toHaveCount(0); await expect(warning(page)).toBeVisible();
    await expect.poll(() => warning(page).evaluate(element => element.contains(document.activeElement))).toBe(true);
    await warning(page).getByRole("button", { name: "取消", exact: true }).click();
    await expect(warning(page)).toHaveCount(0); await expect(action).toBeFocused();
    expect(await calls(page)).toHaveLength(0);
  });
}

for (const viewport of [{ width: 1440, height: 900 }, { width: 1280, height: 800 }]) {
  test.describe(`${viewport.width}x${viewport.height} actual App upload`, () => {
    test.use({ viewport });
    for (const mode of ["demo", "native"] as const) {
      test(`${mode} upload chooser hover keeps white text and unchanged border on the blue header`, async ({ page }, testInfo) => {
        if (mode === "native") await localPagesNativeBridge(page);
        else await page.addInitScript(({ state, key }) => localStorage.setItem(key, JSON.stringify(state)), { state: seedState(), key: STORAGE_KEY });
        await page.goto("/#/backups");
        await page.getByRole("button", { name: mode === "demo" ? "导入快照文件" : "导入完整备份", exact: true }).click();
        const frame = page.getByRole("dialog", { name: mode === "demo" ? "导入演示快照" : "导入本机备份", exact: true });
        const header = frame.locator(".local-page-upload-head"), chooser = header.getByRole("button", { name: mode === "demo" ? "选择快照文件" : "选择本机备份包", exact: true });
        await page.mouse.move(0, 0);
        await expect(header).toHaveCSS("background-color", "rgb(47, 84, 235)");
        await expect(chooser).toHaveCSS("color", "rgb(255, 255, 255)");
        const measure = () => chooser.evaluate(element => {
          const style = getComputedStyle(element);
          return { color: style.color, background: style.backgroundColor, borderWidth: style.borderTopWidth, borderStyle: style.borderTopStyle, borderColor: style.borderTopColor };
        });
        const before = await measure(); await chooser.hover(); const hovered = await measure();
        await testInfo.attach("upload-hover-styles", { body: JSON.stringify({ syntheticOnly: true, mode, viewport, before, hovered, headerBackground: await header.evaluate(element => getComputedStyle(element).backgroundColor) }, null, 2), contentType: "application/json" });
        await testInfo.attach("upload-hover", { body: await frame.screenshot(), contentType: "image/png" });
        expect(hovered.color).toBe("rgb(255, 255, 255)");
        expect({ width: hovered.borderWidth, style: hovered.borderStyle, color: hovered.borderColor }).toEqual({ width: before.borderWidth, style: before.borderStyle, color: before.borderColor });
        await expect(chooser).toBeEnabled(); await expect(frame).toContainText("尚未选择文件");
        await expect(frame.getByRole("button", { name: mode === "demo" ? "校验并预览" : "完整校验并预览", exact: true })).toBeDisabled();
        if (mode === "demo") {
          await expect(frame.getByLabel("演示快照文件", { exact: true })).toHaveAttribute("type", "file");
          await expect(frame.getByLabel("演示快照文件", { exact: true })).toHaveAttribute("accept", ".json");
          await expect(frame.getByLabel("演示快照文件", { exact: true })).toBeHidden();
        } else {
          const requests = await page.evaluate(() => (window as unknown as { __localPagesFixture: { syntheticOnly: boolean; calls: NativeRequest[] } }).__localPagesFixture);
          expect(requests.syntheticOnly).toBe(true);
          expect(requests.calls.filter(call => ["Backup.SelectRestoreSource", "Backup.PreviewRestore", "Backup.ApplyRestore"].includes(call.method))).toHaveLength(0);
        }
      });
    }
  });
}
