import { expect, test, type Page } from "@playwright/test";
import { seedState, STORAGE_KEY, type State } from "../../src/domain.ts";
import type { DeviceProfile, EnvironmentPreview, WorkspaceView } from "../../src/application/contract.ts";
import type { NativeRequest } from "../../src/application/wails-adapter.ts";
import { nativeReferenceBridge, nativeReferenceView } from "./fixtures/native-reference-bridge.ts";

async function demo(page: Page) {
  const state = seedState();
  state.environments.forEach(environment => { environment.status = "ready"; });
  await page.goto("/#/environments");
  await page.evaluate(({ key, state }) => localStorage.setItem(key, JSON.stringify(state)), { key: STORAGE_KEY, state });
  await page.reload();
}
const stored = (page: Page): Promise<State> => page.evaluate(key => JSON.parse(localStorage.getItem(key)!), STORAGE_KEY);
const warning = (page: Page) => page.getByRole("alertdialog", { name: "放弃未保存修改？", exact: true });
const forceWarning = (page: Page) => page.getByRole("alertdialog", { name: "强制结束指定会话？", exact: true });
const forceButton = (page: Page) => page.getByRole("row").filter({ hasText: "测试环境 C 6" }).getByRole("button", { name: "强制结束", exact: true });

async function guardedCreationBridge(page: Page, mode: "pending" | "accepted" | "unknown") {
  await nativeReferenceBridge(page);
  await page.addInitScript(mode => {
    const host = window as unknown as { go: { main: { DesktopApp: { Call: (request: NativeRequest) => Promise<unknown> } } }; __referenceNative: { calls: NativeRequest[]; view: WorkspaceView }; __rejectCreate: () => void };
    const original = host.go.main.DesktopApp.Call;
    const e = { ...host.__referenceNative.view.state.environments[0], id: "", name: "", seed: "172601034" };
    const profile: DeviceProfile = { schemaVersion: 1, configRevision: 1, seed: e.seed, templateId: "windows-desktop-v1", templateVersion: e.fingerprintVersion, generatorVersion: "synthetic-only", platform: "windows", platformVersion: "15.0.0", brand: "Chrome", brandVersion: "148.0.7778.215", kernelId: e.coreId, coreActualVersion: "148.0.7778.215", coreExecutableSha256: "b".repeat(64), adapterVersion: "synthetic-only", capabilityVersion: "synthetic-only", language: e.language, acceptLanguages: [e.language], uiLanguage: "system", timezone: e.timezone, regionPreset: "synthetic-only", cpu: e.cpu, width: e.width, height: e.height, parameters: [], configHash: "synthetic-guard-profile" };
    const fail = (code: string) => ({ ok: false, mode: "native", error: { code, message: "合成提交待核实，原草稿不得丢失", retryable: true } });
    host.go.main.DesktopApp.Call = async request => {
      if (["Environment.Preview", "Environment.Create", "Operation.Read"].includes(request.method)) host.__referenceNative.calls.push(structuredClone(request));
      if (request.method === "Environment.Preview") return { ok: true, mode: "native", data: { previewId: "synthetic-guard-preview", environment: e, fingerprint: { mode: "native", action: "preview", previewProfile: profile, changes: [], capabilityReport: { kernelId: e.coreId, evidenceStatus: "synthetic-only", capabilities: [], observedFingerprint: null, canLaunchNative: false } } } };
      if (request.method === "Environment.Create") {
        if (mode === "pending") return new Promise(resolve => { host.__rejectCreate = () => resolve(fail("VALIDATION_FAILED")); });
        if (mode === "unknown") return fail("NATIVE_UNAVAILABLE");
        return { ok: true, mode: "native", data: { status: "accepted", operation: { id: "synthetic-guard-operation", kind: "create", state: "accepted", total: 1, completedIds: [], cancelRequested: false } } };
      }
      if (request.method === "Operation.Read") return fail("NATIVE_UNAVAILABLE");
      return original(request);
    };
  }, mode);
}

test("dirty confirmation freezes the form, cancel/Escape keep all input, and discard returns to the root trigger", async ({ page }) => {
  await demo(page);
  const before = await stored(page);
  const root = page.getByRole("button", { name: "新建环境", exact: true });
  await root.click();
  const editor = page.getByRole("dialog", { name: "新建浏览器环境", includeHidden: true });
  await editor.getByLabel("环境名称", { exact: true }).fill("未保存的合成草稿");
  await editor.getByLabel("分组", { exact: true }).fill("保留的分组");
  const seed = await editor.getByLabel("固定指纹种子").inputValue();
  await editor.getByRole("button", { name: "取消", exact: true }).click();
  await expect(warning(page)).toBeVisible();
  await expect(warning(page)).toHaveCSS("width", "400px");
  await expect(editor).toHaveJSProperty("inert", false); // inert belongs to its overlay, not a second frame.
  expect(await editor.evaluate(element => !!element.closest("[inert]"))).toBe(true);
  for (const key of ["Tab", "Shift+Tab", "Control+k"]) {
    await page.keyboard.press(key);
    expect(await warning(page).evaluate(element => element.contains(document.activeElement))).toBe(true);
  }
  await warning(page).getByRole("button", { name: "取消", exact: true }).click();
  await expect(editor.getByRole("button", { name: "取消", exact: true })).toBeFocused();
  await expect(editor.getByLabel("环境名称", { exact: true })).toHaveValue("未保存的合成草稿");
  await expect(editor.getByLabel("分组", { exact: true })).toHaveValue("保留的分组");
  await expect(editor.getByLabel("固定指纹种子")).toHaveValue(seed);
  await page.keyboard.press("Escape");
  await expect(warning(page)).toBeVisible();
  await page.keyboard.press("Escape");
  await expect(warning(page)).toHaveCount(0);
  await expect(editor).toBeVisible();
  expect(await stored(page)).toEqual(before);
  await editor.getByRole("button", { name: "取消", exact: true }).click();
  await warning(page).getByRole("button", { name: "放弃修改", exact: true }).click();
  await expect(editor).toHaveCount(0);
  await expect(root).toBeFocused();
  expect(await stored(page)).toEqual(before);
  await root.click();
  await expect(page.getByLabel("环境名称", { exact: true })).toHaveValue("");
  await page.keyboard.press("Escape");
  await expect(root).toBeFocused();
  await expect(page.locator("body")).not.toHaveCSS("overflow", "hidden");
});

test("a late fingerprint changes the frozen draft: stale discard never discards that preview or its replacement", async ({ page }) => {
  await nativeReferenceBridge(page);
  await page.addInitScript(() => {
    const host = window as unknown as { go: { main: { DesktopApp: { Call: (request: NativeRequest) => Promise<unknown> } } }; __finishDraft: () => void };
    const original = host.go.main.DesktopApp.Call;
    let draft: EnvironmentPreview;
    host.go.main.DesktopApp.Call = async request => {
      if (request.method === "Environment.Preview") {
        const result = await original(request) as { data: EnvironmentPreview };
        draft = result.data; return result;
      }
      if (request.method === "Fingerprint.ListRevisions") return { ok: true, mode: "native", data: [] };
      if (request.method === "Fingerprint.Generate") return new Promise(resolve => {
        const e = draft.environment;
        const profile: DeviceProfile = { schemaVersion: 1, configRevision: 1, seed: e.seed, templateId: "windows-desktop-v1", templateVersion: e.fingerprintVersion, generatorVersion: "synthetic-only", platform: "windows", platformVersion: "15.0.0", brand: "Chrome", brandVersion: "148.0.7778.215", kernelId: e.coreId, coreActualVersion: "148.0.7778.215", coreExecutableSha256: "b".repeat(64), adapterVersion: "synthetic-only", capabilityVersion: "synthetic-only", language: e.language, acceptLanguages: [e.language], uiLanguage: "system", timezone: e.timezone, regionPreset: "synthetic-only", cpu: e.cpu, width: e.width, height: e.height, parameters: [], configHash: "synthetic-late-profile" };
        host.__finishDraft = () => resolve({ ok: true, mode: "native", data: { ...draft, fingerprint: { mode: "native", action: "preview", previewProfile: profile, changes: [], capabilityReport: { kernelId: e.coreId, evidenceStatus: "synthetic-only", capabilities: [], observedFingerprint: null, canLaunchNative: false } } } });
      });
      return original(request);
    };
  });
  await page.goto("/#/environments");
  const root = page.getByRole("button", { name: "工作环境 A 更多操作", exact: true });
  await root.click(); await page.getByRole("button", { name: "编辑环境", exact: true }).click();
  const editor = page.getByRole("dialog", { name: "编辑浏览器环境", includeHidden: true });
  // This fixture has no compiled profile; release an actual pending generated profile below.
  await expect.poll(() => page.evaluate(() => typeof (window as unknown as { __finishDraft?: unknown }).__finishDraft)).toBe("function");
  await editor.getByLabel("环境名称", { exact: true }).fill("仍须保留的草稿");
  await editor.getByRole("button", { name: "取消", exact: true }).click();
  await expect(warning(page)).toBeVisible();
  // A valid profile response is installed by the test host, not by touching React state.
  await page.evaluate(() => (window as unknown as { __finishDraft: () => void }).__finishDraft());
  await expect(editor.getByText("沿用已保存身份", { exact: true })).toBeVisible();
  await warning(page).getByRole("button", { name: "放弃修改", exact: true }).click();
  await expect(editor).toBeVisible();
  expect((await nativeReferenceView(page)).calls.filter(call => call.method === "Preview.Discard")).toHaveLength(0);
  await expect(editor.getByLabel("环境名称", { exact: true })).toHaveValue("仍须保留的草稿");
  await expect(page.locator(".toast-error")).toContainText("未放弃任何预览");
  await editor.getByRole("button", { name: "取消", exact: true }).click();
  await warning(page).getByRole("button", { name: "放弃修改", exact: true }).click();
  const calls = (await nativeReferenceView(page)).calls;
  const originalPreview = calls.find(call => call.method === "Environment.Preview")!;
  expect(originalPreview.payload).toMatchObject({ sourceId: "synthetic-reference-1" });
  const discards = calls.filter(call => call.method === "Preview.Discard");
  expect(discards).toHaveLength(1);
  expect(discards[0].payload).toEqual({ previewId: "synthetic-preview-1" });
  await expect(root).toBeFocused();
  await root.click(); await page.getByRole("button", { name: "编辑环境", exact: true }).click();
  await expect(editor).toBeVisible();
  await expect(editor.getByLabel("环境名称", { exact: true })).toHaveValue("工作环境 A");
  expect((await nativeReferenceView(page)).calls.filter(call => call.method === "Preview.Discard")).toHaveLength(1);
});

test("force warning cancels without a request and confirms only the frozen exact session once", async ({ page }) => {
  await nativeReferenceBridge(page, true); await page.goto("/#/environments");
  await forceButton(page).click();
  await expect(forceWarning(page)).toBeVisible();
  await expect(forceWarning(page)).toHaveCSS("width", "400px");
  await expect(forceWarning(page)).toContainText("synthetic-reference-6");
  await expect(forceWarning(page)).toContainText("synthetic-session-6");
  await page.keyboard.press("Escape");
  await expect(forceWarning(page)).toHaveCount(0);
  await expect(forceButton(page)).toBeFocused();
  expect((await nativeReferenceView(page)).calls.filter(call => call.method === "Runtime.ForceStop")).toHaveLength(0);
  await forceButton(page).click();
  await forceWarning(page).getByRole("button", { name: "确认强制结束", exact: true }).click();
  const requests = (await nativeReferenceView(page)).calls.filter(call => call.method === "Runtime.ForceStop");
  expect(requests).toHaveLength(1);
  expect(requests[0].payload).toMatchObject({ environmentId: "synthetic-reference-6", sessionId: "synthetic-session-6", requestId: expect.any(String) });
  expect(Object.keys(requests[0].payload as object).sort()).toEqual(["environmentId", "requestId", "sessionId"]);
});

for (const change of ["session", "control", "force", "reconcile"] as const) {
  test(`force confirmation rechecks fresh ${change} state before any request`, async ({ page }) => {
    await nativeReferenceBridge(page, true); await page.goto("/#/environments");
    await forceButton(page).click(); await expect(forceWarning(page)).toBeVisible();
    const before = (await nativeReferenceView(page)).calls.filter(call => call.method === "Workspace.Read").length;
    await page.evaluate(change => {
      const view = (window as unknown as { __referenceNative: { view: WorkspaceView } }).__referenceNative.view;
      const session = view.runtimeSessions!["synthetic-reference-6"];
      if (change === "session") session.sessionId = "synthetic-new-session-must-not-stop";
      if (change === "control") session.canControl = false;
      if (change === "force") session.canForce = false;
      if (change === "reconcile") session.needsReconcile = true;
    }, change);
    await expect.poll(async () => (await nativeReferenceView(page)).calls.filter(call => call.method === "Workspace.Read").length).toBeGreaterThan(before);
    await forceWarning(page).getByRole("button", { name: "确认强制结束", exact: true }).click();
    await expect(forceWarning(page)).toHaveCount(0);
    expect((await nativeReferenceView(page)).calls.filter(call => call.method === "Runtime.ForceStop")).toHaveLength(0);
    expect((await nativeReferenceView(page)).view.runtimeSessions!["synthetic-reference-6"].pid).toBe(31006);
    await expect(page.locator(".toast-error")).toBeVisible();
  });
}

for (const mode of ["pending", "accepted", "unknown"] as const) {
  test(`dirty close cannot discard a ${mode} creation; recovery retains the original request`, async ({ page }) => {
    await guardedCreationBridge(page, mode); await page.goto("/#/environments");
    await page.getByRole("button", { name: "新建环境", exact: true }).click();
    const editor = page.getByRole("dialog", { name: "新建浏览器环境" });
    await editor.getByLabel("环境名称", { exact: true }).fill("受保护的创建草稿");
    await editor.getByRole("button", { name: "创建", exact: true }).click();
    await expect.poll(async () => (await nativeReferenceView(page)).calls.filter(call => call.method === "Environment.Create").length).toBe(1);
    if (mode !== "pending") await expect(editor.getByRole("alert")).toContainText("核实");
    await expect(editor.getByRole("button", { name: "取消", exact: true })).toBeDisabled();
    await expect(editor.getByLabel("创建数量")).toBeDisabled();
    await page.keyboard.press("Escape");
    await expect(editor).toBeVisible(); await expect(warning(page)).toHaveCount(0);
    expect((await nativeReferenceView(page)).calls.filter(call => call.method === "Preview.Discard")).toHaveLength(0);
    const first = (await nativeReferenceView(page)).calls.find(call => call.method === "Environment.Create")!.payload;
    if (mode === "pending") {
      await page.evaluate(() => (window as unknown as { __rejectCreate: () => void }).__rejectCreate());
      await expect(editor.getByLabel("环境名称", { exact: true })).toBeEnabled();
      await editor.getByRole("button", { name: "取消", exact: true }).click();
      await expect(warning(page)).toBeVisible();
    } else {
      await editor.getByRole("button", { name: "重试核实创建结果", exact: true }).click();
      await expect(editor.getByRole("alert")).toContainText("核实");
      const calls = (await nativeReferenceView(page)).calls;
      const creates = calls.filter(call => call.method === "Environment.Create");
      expect(creates).toHaveLength(mode === "unknown" ? 2 : 1);
      expect(creates.every(call => JSON.stringify(call.payload) === JSON.stringify(first))).toBe(true);
      if (mode === "accepted") expect(calls.filter(call => call.method === "Operation.Read").map(call => call.payload)).toEqual([{ operationId: "synthetic-guard-operation" }, { operationId: "synthetic-guard-operation" }]);
      await expect(editor.getByLabel("环境名称", { exact: true })).toHaveValue("受保护的创建草稿");
      expect(calls.filter(call => call.method === "Preview.Discard")).toHaveLength(0);
    }
  });
}

test("workspace faults outrank a force warning without consuming it; recovery restores the warning focus", async ({ page }) => {
  await nativeReferenceBridge(page, true); await page.goto("/#/environments");
  await forceButton(page).click(); await expect(forceWarning(page)).toBeVisible();
  await page.evaluate(() => {
    (window as unknown as { __referenceNative: { view: WorkspaceView } }).__referenceNative.view.issue = { code: "NATIVE_UNAVAILABLE", message: "合成工作区故障，警告保留", retryable: true };
  });
  const blocker = page.getByRole("alertdialog", { name: "工作区需要处理", exact: true });
  await expect(blocker).toBeVisible();
  for (const key of ["Escape", "Tab", "Shift+Tab", "Control+k"]) {
    await page.keyboard.press(key);
    expect(await blocker.evaluate(element => element.contains(document.activeElement))).toBe(true);
  }
  await expect(page.getByRole("alertdialog", { name: "强制结束指定会话？", includeHidden: true })).toBeAttached();
  expect((await nativeReferenceView(page)).calls.filter(call => call.method === "Runtime.ForceStop")).toHaveLength(0);
  await page.evaluate(() => { delete (window as unknown as { __referenceNative: { view: WorkspaceView } }).__referenceNative.view.issue; });
  await expect(blocker).toHaveCount(0);
  await expect(forceWarning(page)).toBeVisible();
  await expect.poll(() => forceWarning(page).evaluate(element => element.contains(document.activeElement))).toBe(true);
  await page.keyboard.press("Escape"); await expect(forceButton(page)).toBeFocused();
  await expect(page.locator("body")).not.toHaveCSS("overflow", "hidden");
});

test("quantity-only changes require discard confirmation and demo removal preserves exact scope through write failure", async ({ page }) => {
  await demo(page); const before = await stored(page);
  await page.getByRole("button", { name: "新建环境", exact: true }).click();
  await page.getByLabel("创建数量").fill("3");
  await page.keyboard.press("Escape"); await expect(warning(page)).toBeVisible();
  await warning(page).getByRole("button", { name: "放弃修改", exact: true }).click();
  await page.getByLabel("选择 北美主店", { exact: true }).check();
  await page.getByLabel("选择 欧洲家居店", { exact: true }).check();
  await page.getByRole("button", { name: "更多操作", exact: true }).click();
  await page.getByRole("button", { name: "移除所选环境", exact: true }).click();
  const remove = page.getByRole("alertdialog", { name: "移除浏览器环境", exact: true });
  await expect(remove).toHaveCSS("width", "400px");
  await expect(remove).toContainText("2 个环境");
  await expect(remove.getByRole("checkbox")).toHaveCount(0);
  await page.evaluate(() => {
    const original = Storage.prototype.setItem;
    Object.assign(window, { __restoreWrites: () => { Storage.prototype.setItem = original; } });
    Storage.prototype.setItem = () => { throw new DOMException("synthetic failure", "QuotaExceededError"); };
  });
  await remove.getByRole("button", { name: "确认移除", exact: true }).click();
  await expect(page.locator(".toast-error")).toContainText("未保存");
  await expect(remove).toBeVisible(); expect(await stored(page)).toEqual(before);
  await page.evaluate(() => (window as unknown as { __restoreWrites: () => void }).__restoreWrites());
  await remove.getByRole("button", { name: "确认移除", exact: true }).click();
  const after = await stored(page);
  expect(after.environments).toEqual(before.environments.filter(environment => !["env-1", "env-3"].includes(environment.id)));
  expect(after.activities[0].detail).toBe("仅删除示例记录及示例 Cookie；不操作真实文件。");
});

test("failed normal stop never escalates into a force request or confirmation", async ({ page }) => {
  await nativeReferenceBridge(page, true);
  await page.addInitScript(() => {
    const host = window as unknown as { go: { main: { DesktopApp: { Call: (request: NativeRequest) => Promise<unknown> } } }; __referenceNative: { calls: NativeRequest[] } };
    const original = host.go.main.DesktopApp.Call;
    host.go.main.DesktopApp.Call = request => {
      if (request.method !== "Runtime.Stop") return original(request);
      host.__referenceNative.calls.push(structuredClone(request));
      return Promise.resolve({ ok: false, mode: "native", error: { code: "CONTROL_CHANNEL_LOST", message: "合成正常关闭失败，不自动强制结束", retryable: false } });
    };
  });
  await page.goto("/#/environments");
  const before = (await nativeReferenceView(page)).view.state.environments;
  await page.getByRole("row").filter({ hasText: "测试环境 C 6" }).getByRole("button", { name: "重试关闭", exact: true }).click();
  await expect(page.getByRole("region", { name: "逐项操作结果" })).toContainText("正常关闭失败");
  const data = await nativeReferenceView(page);
  expect(data.calls.filter(call => call.method === "Runtime.Stop").map(call => call.payload)).toMatchObject([{ environmentId: "synthetic-reference-6", requestId: expect.any(String) }]);
  expect(data.calls.filter(call => call.method === "Runtime.ForceStop")).toHaveLength(0);
  await expect(forceWarning(page)).toHaveCount(0);
  expect(data.view.state.environments).toEqual(before);
});

test("demo removal refuses running exact IDs and Cookie parse errors cannot save the valid subset", async ({ page }) => {
  await demo(page);
  const state = await stored(page); state.environments[0].status = "running";
  await page.evaluate(({ key, state }) => localStorage.setItem(key, JSON.stringify(state)), { key: STORAGE_KEY, state }); await page.reload();
  await page.getByRole("button", { name: "北美主店 更多操作", exact: true }).click();
  await page.getByRole("button", { name: "移除环境", exact: true }).click();
  const remove = page.getByRole("alertdialog", { name: "移除浏览器环境", exact: true });
  await remove.getByRole("button", { name: "确认移除", exact: true }).click();
  await expect(remove.getByRole("alert")).toContainText("先关闭");
  expect(await stored(page)).toEqual(state);
  await remove.getByRole("button", { name: "取消", exact: true }).click();
  await page.getByRole("button", { name: "北美主店 更多操作", exact: true }).click();
  await page.getByRole("button", { name: "导入 Cookie", exact: true }).click();
  const cookie = page.getByRole("dialog", { name: "导入 Cookie", exact: true });
  await cookie.getByLabel("Cookie 内容", { exact: true }).fill(JSON.stringify([{ name: "valid", value: "SYNTHETIC", domain: "example.invalid", path: "/" }, { name: "missing-domain", value: "SYNTHETIC" }]));
  await cookie.getByRole("button", { name: "校验并预览", exact: true }).click();
  await expect(cookie).toContainText("1 条有效 / 1 条错误");
  await expect(cookie.getByRole("button", { name: "导入到原型记录", exact: true })).toBeDisabled();
  expect(await stored(page)).toEqual(state);
});

test("drawer discard opened from a portal returns to its actual menu trigger", async ({ page }) => {
  await demo(page);
  const trigger = page.getByRole("button", { name: "创建菜单", exact: true });
  await trigger.click();
  await page.getByRole("button", { name: "新建 / 批量创建", exact: true }).click();
  await page.getByLabel("环境名称", { exact: true }).fill("从菜单打开的草稿");
  await page.keyboard.press("Escape");
  await warning(page).getByRole("button", { name: "放弃修改", exact: true }).click();
  await expect(trigger).toBeFocused();
});
