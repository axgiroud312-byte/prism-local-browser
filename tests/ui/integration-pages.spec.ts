import { expect, test, type Locator, type Page } from "@playwright/test";
import { createSnapshot, seedState, STORAGE_KEY, type Environment, type State } from "../../src/domain";
import type { DeviceProfile, EnvironmentPreview, RuntimeSession, WorkspaceView } from "../../src/application/contract";
import type { NativeRequest } from "../../src/application/wails-adapter";
import { proxyKernelBridge } from "./fixtures/proxy-kernel";
import { localPagesNativeBridge } from "./fixtures/local-pages-native-bridge";

// Production App wiring with synthetic adapters only; no desktop I/O is used.
const errors = new WeakMap<Page, string[]>();
test.beforeEach(async ({ page, baseURL }) => {
  const messages: string[] = []; errors.set(page, messages);
  page.on("pageerror", error => messages.push(error.message));
  const origin = new URL(baseURL!).origin;
  await page.route("**/*", route => new URL(route.request().url()).origin === origin ? route.continue() : route.abort("blockedbyclient"));
  // Shared source is edited concurrently. Never let Vite restart a fixture.
  await page.routeWebSocket("**/*", socket => socket.close());
});
test.afterEach(async ({ page }) => { expect(errors.get(page)).toEqual([]); });

const importer = (page: Page) => page.getByRole("dialog", { name: "批量添加代理", exact: true });
const stored = (page: Page) => page.evaluate(key => JSON.parse(localStorage.getItem(key)!) as State, STORAGE_KEY);
async function demo(page: Page, route = "environments") {
  const state = seedState(); state.environments.forEach(environment => { environment.status = "ready"; });
  await page.addInitScript(({ state, key }) => localStorage.setItem(key, JSON.stringify(state)), { state, key: STORAGE_KEY });
  await page.goto(`/#/${route}`);
}
async function denyWrites(page: Page) {
  await page.evaluate(() => {
    const original = Storage.prototype.setItem;
    Object.assign(window, { __restoreIntegrationWrites: () => { Storage.prototype.setItem = original; } });
    Storage.prototype.setItem = () => { throw new DOMException("synthetic write failure", "QuotaExceededError"); };
  });
}
const restoreWrites = (page: Page) => page.evaluate(() => (window as unknown as { __restoreIntegrationWrites(): void }).__restoreIntegrationWrites());

// Extend the existing independent proxy bridge only for this test's App-owned
// draft/runtime seams. Unimplemented operations still fail closed.
async function native(page: Page, outcome: "failure" | "unknown" | "completed" = "completed", activity = false) {
  await proxyKernelBridge(page, outcome);
  await page.addInitScript(({ activity }) => {
    type ProxyFixture = { calls: NativeRequest[]; view(): WorkspaceView; setOutcome(value: string): void };
    const fixture = (window as unknown as { __proxyKernel: ProxyFixture }).__proxyKernel;
    const host = (window as unknown as { go: { main: { DesktopApp: { Call(request: NativeRequest): Promise<unknown> } } } }).go.main.DesktopApp;
    const original = host.Call, base = fixture.view();
    const sessions: Record<string, RuntimeSession> = {};
    const previews = new Map<string, EnvironmentPreview>();
    let serial = 0, holdCommit = false, releaseCommit: (() => void) | undefined;
    const ok = (data: unknown) => ({ ok: true, mode: "native", data: structuredClone(data) });
    const record = base.nativeProxyRecords![0];
    record.usedBy = ["synthetic-environment", "synthetic-off-page"]; record.usedCount = 2;
    const activities = activity ? [{ id: "synthetic-current-record", action: "合成会话待处理", target: "合成环境 A", detail: "合成记录，不是实际桌面执行。", result: "error" as const, time: "2026-10-06T01:00:00Z", environmentId: "synthetic-environment", sessionId: "synthetic-current-session" }] : [];
    if (activity) {
      base.state.environments[0].status = "error";
      sessions["synthetic-environment"] = { mode: "native", environmentId: "synthetic-environment", sessionId: "synthetic-current-session", operationId: "synthetic-runtime-operation", state: "error", revision: 1, fingerprintRevision: 1, kernelId: "synthetic-build-a", userDataRef: "environments/synthetic-environment/user-data", networkPolicy: "proxy", proxyId: "synthetic-proxy-a", proxyRevision: 1, canControl: true, canForce: true, needsReconcile: false, persistencePending: false, pid: 32035 };
    }
    function preview(environment: Environment, previewId: string): EnvironmentPreview {
      const kernel = base.kernelRecords!.find(kernel => kernel.id === environment.coreId)!;
      const profile: DeviceProfile = { schemaVersion: 1, configRevision: 1, seed: environment.seed, templateId: "windows-desktop-v1", templateVersion: environment.fingerprintVersion, generatorVersion: "synthetic-only", platform: "windows", platformVersion: "15.0.0", brand: "Chrome", brandVersion: kernel.version, kernelId: kernel.id, coreActualVersion: kernel.version, coreExecutableSha256: kernel.executableSha256, adapterVersion: "synthetic-only", capabilityVersion: "synthetic-only", language: environment.language, acceptLanguages: [environment.language], uiLanguage: "system", timezone: environment.timezone, regionPreset: "synthetic-only", cpu: environment.cpu, width: environment.width, height: environment.height, parameters: [], configHash: `${environment.seed}-${environment.coreId}-${environment.width}` };
      return { previewId, environment, expectedRevision: environment.id ? 1 : undefined, fingerprint: { mode: "native", action: "preview", changes: [], previewProfile: profile, capabilityReport: { kernelId: kernel.id, evidenceStatus: "synthetic-only", capabilities: [], observedFingerprint: null, canLaunchNative: false } } };
    }
    Object.assign(window, { __integration: { sessions, holdCommit() { holdCommit = true; }, releaseCommit() { holdCommit = false; releaseCommit?.(); } } });
    host.Call = async request => {
      const payload = request.payload as Record<string, unknown>;
      if (request.method === "Workspace.Read") {
        const response = await original(request) as { ok: boolean; data: WorkspaceView };
        if (response.ok) { response.data.runtimeSessions = structuredClone(sessions); response.data.state.activities = structuredClone(activities); }
        return response;
      }
      if (request.method === "Proxy.CommitImport" && holdCommit) await new Promise<void>(resolve => { releaseCommit = resolve; });
      if (!["Environment.Preview", "Fingerprint.Generate", "Fingerprint.ListRevisions", "Preview.Discard", "Runtime.ForceStop"].includes(request.method)) return original(request);
      fixture.calls.push(structuredClone(request));
      if (request.method === "Environment.Preview") {
        const source = base.state.environments[0];
        const environment = payload.kind === "edit" ? { ...source } : { ...source, id: "", name: "", group: "", note: "", proxyId: "", seed: "172600035", status: "ready" as const };
        const value = preview(environment, `synthetic-app-preview-${++serial}`); previews.set(value.previewId, value); return ok(value);
      }
      if (request.method === "Fingerprint.Generate") {
        const old = previews.get(String(payload.previewId))!;
        const environment = { ...old.environment, ...(payload.overrides as Partial<Environment>), coreId: String(payload.kernelId), seed: payload.regenerate ? String(172600035 + ++serial) : old.environment.seed };
        const value = preview(environment, old.previewId); previews.set(value.previewId, value); return ok(value);
      }
      if (request.method === "Fingerprint.ListRevisions") return ok([]);
      if (request.method === "Preview.Discard") { previews.delete(String(payload.previewId)); return ok({ status: "discarded" }); }
      return ok({ status: "accepted", operation: { id: "synthetic-force-operation", kind: "runtime-stop", state: "completed", total: 1, completedIds: [payload.environmentId], cancelRequested: false, environmentId: payload.environmentId } });
    };
  }, { activity });
}
const nativeCalls = (page: Page, method: string) => page.evaluate(method => (window as unknown as { __proxyKernel: { calls: NativeRequest[] } }).__proxyKernel.calls.filter(call => call.method === method), method);

async function draft(page: Page) {
  await page.getByRole("button", { name: "新建环境", exact: true }).click();
  const editor = page.getByRole("dialog", { name: "新建浏览器环境", exact: true });
  await editor.getByLabel("环境名称", { exact: true }).fill("集成往返草稿");
  await editor.getByLabel("分组", { exact: true }).fill("合成集成分组");
  await editor.getByLabel("备注", { exact: true }).fill("保留备注");
  await editor.getByLabel("创建数量").fill("2");
  await editor.getByLabel("启动网址").fill("https://example.invalid/integration-draft");
  const seed = await editor.getByLabel("固定指纹种子").inputValue(), kernel = await editor.getByLabel("浏览器内核").inputValue();
  return { editor, seed, kernel };
}
async function retainedDraft(editor: Locator, seed: string, kernel: string) {
  await expect(editor.getByLabel("环境名称", { exact: true })).toHaveValue("集成往返草稿");
  await expect(editor.getByLabel("分组", { exact: true })).toHaveValue("合成集成分组");
  await expect(editor.getByLabel("备注", { exact: true })).toHaveValue("保留备注");
  await expect(editor.getByLabel("创建数量")).toHaveValue("2");
  await expect(editor.getByLabel("启动网址")).toHaveValue("https://example.invalid/integration-draft");
  await expect(editor.getByLabel("固定指纹种子")).toHaveValue(seed);
  await expect(editor.getByLabel("浏览器内核")).toHaveValue(kernel);
  await expect(editor.getByLabel("环境名称", { exact: true })).toBeEditable();
  await editor.getByLabel("环境名称", { exact: true }).focus();
  await expect(editor.getByLabel("环境名称", { exact: true })).toBeFocused();
  // Actually edit after returning, not just an enabled attribute under inert.
  await editor.getByLabel("环境名称", { exact: true }).fill("可编辑的返回草稿");
  await editor.getByLabel("环境名称", { exact: true }).fill("集成往返草稿");
}

for (const viewport of [{ width: 1440, height: 900 }, { width: 1280, height: 800 }]) {
test.describe(`${viewport.width}x${viewport.height} actual App`, () => {
test.use({ viewport });

for (const mode of ["demo", "native"] as const) {
  test(`${mode} App importer has one parent-owned frame and returns an editable stable draft`, async ({ page }) => {
    if (mode === "demo") await demo(page); else { await native(page); await page.goto("/#/environments"); }
    const { editor, seed, kernel } = await draft(page);
    await editor.getByRole("button", { name: "导入代理", exact: true }).click();
    const frame = importer(page);
    expect(await page.locator('[aria-modal="true"]').evaluateAll(elements => elements.filter(element => !element.closest("[inert]")).length)).toBe(1);
    await expect(page.locator(".environment-overlay")).toHaveJSProperty("inert", true);
    await expect(page.locator(".stacked-overlay [aria-modal='true']")).toHaveCount(1);
    expect(Math.round((await frame.boundingBox())!.width)).toBe(1050);
    const text = "socks5://synthetic:only@203.0.113.55:1080\nftp://203.0.113.1:21";
    await frame.getByLabel("原始代理导入文本").fill(text);
    await frame.getByRole("button", { name: "解析预览", exact: true }).click();
    await expect(frame.getByLabel("保存第2行")).toBeDisabled();
    await frame.getByRole("button", { name: "返回环境配置", exact: true }).focus();
    await page.keyboard.press("Tab"); await expect.poll(() => frame.evaluate(element => element.contains(document.activeElement))).toBe(true);
    await page.mouse.click(5, 5); await expect(frame).toBeVisible();
    await frame.getByRole("button", { name: "选择文本文件", exact: true }).click();
    const file = page.getByRole("dialog", { name: "批量导入代理", exact: true });
    expect(Math.round((await file.boundingBox())!.width)).toBe(500);
    await page.keyboard.press("Escape");
    await expect(editor.getByRole("button", { name: "导入代理", exact: true })).toBeFocused();
    await retainedDraft(editor, seed, kernel);
    expect(await page.evaluate(() => document.body.style.overflow)).toBe("hidden");
    await editor.getByRole("button", { name: "导入代理", exact: true }).click();
    await expect(frame.getByLabel("原始代理导入文本")).toHaveValue(text);
    await page.keyboard.press("Escape");
    await page.keyboard.press("Escape");
    await page.getByRole("alertdialog", { name: "放弃未保存修改？", exact: true }).getByRole("button", { name: "放弃修改", exact: true }).click();
    await page.getByRole("link", { name: "代理管理", exact: true }).click();
    await page.getByRole("button", { name: "批量添加代理", exact: true }).click();
    await expect(frame.getByLabel("原始代理导入文本")).toHaveValue(text);
    await frame.getByRole("button", { name: "清除输入与预览", exact: true }).click();
    await page.keyboard.press("Escape");
    await expect(page.getByRole("button", { name: "批量添加代理", exact: true })).toBeFocused();
    expect(await page.evaluate(() => document.body.style.overflow)).toBe("");
    await expect(page.locator(".main-shell")).toHaveJSProperty("inert", false);
    await page.getByRole("button", { name: "添加代理", exact: true }).click();
    const single = page.getByRole("dialog", { name: "添加代理", exact: true });
    expect(Math.round((await single.boundingBox())!.width)).toBe(620);
    await page.keyboard.press("Escape");
    await expect(page.getByRole("button", { name: "添加代理", exact: true })).toBeFocused();
  });
}

test("native failed nested import retries without changing draft identity or creating an environment", async ({ page }) => {
  await native(page, "failure"); await page.goto("/#/environments");
  const { editor, seed, kernel } = await draft(page);
  await editor.getByRole("button", { name: "导入代理", exact: true }).click();
  const frame = importer(page); await frame.getByLabel("原始代理导入文本").fill("socks5://203.0.113.55:1080");
  await frame.getByRole("button", { name: "解析预览", exact: true }).click();
  await frame.getByRole("button", { name: "保存所选有效行", exact: true }).click();
  await expect(frame.getByRole("status")).toContainText("STORAGE_WRITE_FAILED");
  await frame.getByRole("button", { name: "返回环境配置", exact: true }).click();
  await retainedDraft(editor, seed, kernel);
  await editor.getByRole("button", { name: "导入代理", exact: true }).click();
  await expect(frame.getByLabel("原始代理导入文本")).toHaveValue("socks5://203.0.113.55:1080");
  await page.evaluate(() => (window as unknown as { __proxyKernel: { setOutcome(value: string): void } }).__proxyKernel.setOutcome("completed"));
  await frame.getByRole("button", { name: "保存所选有效行", exact: true }).click();
  await expect(frame).toHaveCount(0); await retainedDraft(editor, seed, kernel);
  const id = await editor.getByLabel("绑定代理").inputValue(); expect(id).toMatch(/^synthetic-imported-/);
  expect(await nativeCalls(page, "Environment.Create")).toHaveLength(0);
  expect(await nativeCalls(page, "Proxy.CommitImport")).toHaveLength(2);
});

test("native unknown nested import hides and returns across routes with the exact original request", async ({ page }) => {
  await native(page, "unknown"); await page.goto("/#/environments");
  const { editor, seed, kernel } = await draft(page);
  await editor.getByRole("button", { name: "导入代理", exact: true }).click();
  const frame = importer(page), text = "http://203.0.113.55:8080\ninvalid-line";
  await frame.getByLabel("原始代理导入文本").fill(text); await frame.getByRole("button", { name: "解析预览", exact: true }).click();
  await frame.getByRole("button", { name: "保存所选有效行", exact: true }).click();
  await expect(frame.getByRole("status")).toContainText("原请求结果未知");
  await expect(frame.getByLabel("原始代理导入文本")).toBeDisabled();
  await expect(frame.getByRole("button", { name: "清除输入与预览", exact: true })).toBeDisabled();
  await page.keyboard.press("Escape"); await retainedDraft(editor, seed, kernel);
  await editor.getByRole("button", { name: "导入代理", exact: true }).click();
  await expect(frame.getByLabel("原始代理导入文本")).toHaveValue(text);
  await page.keyboard.press("Escape"); await page.keyboard.press("Escape");
  await page.getByRole("alertdialog", { name: "放弃未保存修改？", exact: true }).getByRole("button", { name: "放弃修改", exact: true }).click();
  await page.getByRole("link", { name: "代理管理", exact: true }).click();
  await page.getByRole("button", { name: "批量添加代理", exact: true }).click();
  await expect(frame.getByLabel("原始代理导入文本")).toHaveValue(text);
  await page.evaluate(() => (window as unknown as { __proxyKernel: { setOutcome(value: string): void } }).__proxyKernel.setOutcome("completed"));
  await frame.getByRole("button", { name: "核实原导入请求", exact: true }).click();
  await expect(frame.getByLabel("原始代理导入文本")).toHaveValue("invalid-line");
  await expect(frame.getByRole("button", { name: "保存所选有效行", exact: true })).toBeDisabled();
  const requests = await nativeCalls(page, "Proxy.CommitImport"); expect(requests).toHaveLength(2); expect(requests[1].payload).toEqual(requests[0].payload);
  expect(await nativeCalls(page, "Environment.Create")).toHaveLength(0);
});

test("native busy nested import cannot close from footer, Escape or backdrop", async ({ page }) => {
  await native(page); await page.goto("/#/environments"); const { editor } = await draft(page);
  await editor.getByRole("button", { name: "导入代理", exact: true }).click(); const frame = importer(page);
  await frame.getByLabel("原始代理导入文本").fill("http://203.0.113.55:8080"); await frame.getByRole("button", { name: "解析预览", exact: true }).click();
  await page.evaluate(() => (window as unknown as { __integration: { holdCommit(): void } }).__integration.holdCommit());
  await frame.getByRole("button", { name: "保存所选有效行", exact: true }).click();
  await expect(frame.getByLabel("原始代理导入文本")).toBeDisabled();
  const close = frame.getByRole("button", { name: "返回环境配置", exact: true });
  await expect(close).toBeDisabled();
  const rect = (await close.boundingBox())!; await page.mouse.click(rect.x + rect.width / 2, rect.y + rect.height / 2); await expect(frame).toBeVisible();
  await page.keyboard.press("Escape"); await page.mouse.click(5, 5); await expect(frame).toBeVisible();
  await page.evaluate(() => (window as unknown as { __integration: { releaseCommit(): void } }).__integration.releaseCommit());
  await expect(frame).toHaveCount(0); await expect(editor).toBeVisible();
  expect(await nativeCalls(page, "Proxy.CommitImport")).toHaveLength(1);
});

test("native proxy assignment never treats selected proxies or visible environments as its scope", async ({ page }) => {
  await native(page); await page.goto("/#/proxies");
  await page.getByLabel("选择代理 合成 HTTP", { exact: true }).check();
  await page.getByRole("button", { name: "更多操作", exact: true }).click();
  await page.getByRole("menuitem", { name: "批量分配代理", exact: true }).click();
  await expect(page).toHaveURL(/#\/environments$/); await expect(page.locator(".toast")).toContainText("请先明确选择");
  expect(await nativeCalls(page, "Batch.Preview")).toHaveLength(0);
  await page.getByRole("link", { name: "代理管理", exact: true }).click();
  await page.getByRole("button", { name: "合成 SOCKS 已绑定环境", exact: true }).click();
  const usage = page.getByRole("dialog", { name: "已绑定环境 · 合成 SOCKS", exact: true });
  await usage.getByLabel("选择绑定环境 synthetic-off-page", { exact: true }).check();
  await usage.getByRole("button", { name: "为所选环境重新分配代理", exact: true }).click();
  const assign = page.getByRole("dialog", { name: "逐项分配代理", exact: true });
  await expect(assign.getByLabel("为 synthetic-off-page 分配代理", { exact: true })).toBeVisible();
  await expect(assign.getByLabel("为 synthetic-environment 分配代理", { exact: true })).toHaveCount(0);
  await assign.getByLabel("为 synthetic-off-page 分配代理", { exact: true }).selectOption("synthetic-proxy-b");
  await assign.getByRole("button", { name: "查看逐项映射预览（不保存绑定）", exact: true }).click();
  const requests = await nativeCalls(page, "Batch.Preview"); expect(requests).toHaveLength(1);
  expect(requests[0].payload).toMatchObject({ kind: "assign", mappings: [{ environmentId: "synthetic-off-page", proxyId: "synthetic-proxy-b" }] });
});

test("native kernel migration uses its controlled single entry and keeps the manager mounted", async ({ page }) => {
  await native(page); await page.goto("/#/kernels");
  const entry = page.getByRole("button", { name: "选定环境迁移", exact: true }); await expect(entry).toHaveCount(1);
  await expect(page.locator(".migration35-entry")).toHaveCount(0);
  await entry.click(); const selection = page.getByRole("dialog", { name: "选定环境内核迁移", exact: true });
  await expect(selection).toBeVisible(); await page.keyboard.press("Escape"); await expect(selection).toHaveCount(0);
  await entry.click(); await expect(selection).toBeVisible();
  expect(await nativeCalls(page, "Migration.Prepare")).toHaveLength(0);
});

test("App demo restore owns its lifecycle, keeps failed writes inline and releases inert after closing", async ({ page }) => {
  await demo(page, "backups"); const before = await stored(page), snapshot = createSnapshot(before);
  snapshot.environments[0].name = "恢复后的合成环境";
  await page.getByRole("button", { name: "导入快照文件", exact: true }).click();
  await page.getByLabel("演示快照文件", { exact: true }).setInputFiles({ name: "synthetic-integration.json", mimeType: "application/json", buffer: Buffer.from(JSON.stringify(snapshot)) });
  await page.getByRole("dialog", { name: "导入演示快照", exact: true }).getByRole("button", { name: "校验并预览", exact: true }).click();
  const preview = page.getByRole("dialog", { name: "演示快照恢复预览", exact: true });
  await expect(preview).toContainText("恢复后的合成环境"); expect(await stored(page)).toEqual(before);
  await preview.getByRole("button", { name: "下一步：确认恢复", exact: true }).click();
  const confirmation = page.getByRole("dialog", { name: "确认恢复演示快照", exact: true });
  await denyWrites(page); await confirmation.getByRole("button", { name: "确认恢复", exact: true }).click();
  await expect(confirmation.getByRole("alert")).toContainText("未保存"); expect(await stored(page)).toEqual(before);
  await expect(page.getByText("快照已恢复，代理需要重新检查", { exact: true })).toHaveCount(0);
  await page.keyboard.press("Escape"); await expect(preview).toBeVisible(); await page.keyboard.press("Escape");
  await expect(page.getByRole("dialog")).toHaveCount(0); await expect(page.locator(".sidebar")).toHaveJSProperty("inert", false);
  await expect(page.locator(".main-shell")).toHaveJSProperty("inert", false); expect(await page.evaluate(() => document.body.style.overflow)).toBe("");
  await restoreWrites(page);
  await page.getByRole("button", { name: "导入快照文件", exact: true }).click();
  await page.getByLabel("演示快照文件", { exact: true }).setInputFiles({ name: "synthetic-integration.json", mimeType: "application/json", buffer: Buffer.from(JSON.stringify(snapshot)) });
  await page.getByRole("dialog", { name: "导入演示快照", exact: true }).getByRole("button", { name: "校验并预览", exact: true }).click();
  await preview.getByRole("button", { name: "下一步：确认恢复", exact: true }).click();
  await confirmation.getByRole("button", { name: "确认恢复", exact: true }).click();
  await expect(page.getByRole("dialog")).toHaveCount(0); await expect(page.getByText("快照已恢复，代理需要重新检查", { exact: true })).toBeVisible();
  const after = await stored(page);
  expect(after.environments[0].name).toBe("恢复后的合成环境");
  expect(after.environments.map(environment => [environment.id, environment.seed, environment.coreId, environment.proxyId])).toEqual(snapshot.environments.map(environment => [environment.id, environment.seed, environment.coreId, environment.proxyId]));
  expect(after.proxies.every(proxy => proxy.password === "" && proxy.status === "unchecked")).toBe(true);
  expect(after.backups).toEqual(before.backups); expect(after.activities.slice(1)).toEqual(before.activities);
  await expect(page.locator(".main-shell")).toHaveJSProperty("inert", false);
  expect(await page.evaluate(() => document.body.style.overflow)).toBe("");
  await page.getByRole("link", { name: "浏览器环境", exact: true }).click();
  await page.getByRole("button", { name: "新建环境", exact: true }).click(); await expect(page.getByLabel("环境名称", { exact: true })).toBeEditable();
});

test("App native backup freezes exact selected IDs before cross-page navigation", async ({ page }) => {
  await localPagesNativeBridge(page, "baseline"); await page.goto("/#/environments");
  await page.getByLabel("选择 工作环境 A", { exact: true }).check();
  await page.getByRole("button", { name: "下一页", exact: true }).click();
  await page.getByLabel("选择 工作环境 B 11", { exact: true }).check();
  await page.getByRole("button", { name: "更多操作", exact: true }).click();
  await page.getByRole("button", { name: "完整备份所选", exact: true }).click();
  await expect(page).toHaveURL(/#\/backups$/);
  await page.getByRole("button", { name: "创建完整备份", exact: true }).click();
  const create = page.getByRole("dialog", { name: "创建完整本机备份", exact: true });
  await expect(create.getByLabel("备份范围", { exact: true })).toHaveValue("selected");
  await create.getByRole("button", { name: "选择新备份文件位置", exact: true }).click();
  await create.getByLabel(/允许正常关闭/).check(); await create.getByRole("button", { name: "正常关闭并导出", exact: true }).click();
  const requests = await page.evaluate(() => (window as unknown as { __localPagesFixture: { calls: NativeRequest[] } }).__localPagesFixture.calls.filter(call => call.method === "Backup.Export"));
  expect(requests).toHaveLength(1); expect(requests[0].payload).toMatchObject({ scope: "selected", environmentIds: ["synthetic-reference-1", "synthetic-reference-11"] });
});

for (const change of ["none", "ownership", "session"] as const) {
  test(`App activity detail force confirmation paints above its owner and revalidates ${change}`, async ({ page }) => {
    await native(page, "completed", true); await page.goto("/#/activity");
    await page.getByRole("button", { name: "合成会话待处理 合成环境 A 详情", exact: true }).click();
    const detail = page.getByRole("dialog", { name: "操作记录详情", exact: true });
    await detail.getByRole("button", { name: "强制结束此会话", exact: true }).click();
    const confirmation = page.getByRole("alertdialog", { name: "强制结束指定会话？", exact: true });
    await expect(confirmation).toContainText("synthetic-environment"); await expect(confirmation).toContainText("synthetic-current-session");
    expect(await confirmation.evaluate(element => {
      const rect = element.getBoundingClientRect(), hit = document.elementFromPoint(rect.x + rect.width / 2, rect.y + rect.height / 2);
      return !!hit && element.contains(hit);
    })).toBe(true);
    for (const key of ["Tab", "Shift+Tab", "Control+k"]) { await page.keyboard.press(key); await expect.poll(() => confirmation.evaluate(element => element.contains(document.activeElement))).toBe(true); }
    await confirmation.getByRole("button", { name: "取消", exact: true }).click(); await expect(detail).toBeVisible();
    expect(await nativeCalls(page, "Runtime.ForceStop")).toHaveLength(0);
    await detail.getByRole("button", { name: "强制结束此会话", exact: true }).click();
    if (change !== "none") {
      const count = (await nativeCalls(page, "Workspace.Read")).length;
      await page.evaluate(change => {
        const session = (window as unknown as { __integration: { sessions: Record<string, RuntimeSession> } }).__integration.sessions["synthetic-environment"];
        if (change === "ownership") session.canControl = false; else session.sessionId = "synthetic-new-session";
      }, change);
      await expect.poll(async () => (await nativeCalls(page, "Workspace.Read")).length).toBeGreaterThan(count);
    }
    await confirmation.getByRole("button", { name: "确认强制结束", exact: true }).click();
    const requests = await nativeCalls(page, "Runtime.ForceStop");
    if (change === "none") { expect(requests).toHaveLength(1); expect(requests[0].payload).toMatchObject({ environmentId: "synthetic-environment", sessionId: "synthetic-current-session" }); }
    else expect(requests).toHaveLength(0);
  });
}

test("App extracted demo pages have no old toolbar and help retains four document downloads and navigation", async ({ page }) => {
  await demo(page, "proxies"); await expect(page.locator(".pk35-page")).toBeVisible();
  for (const route of ["kernels", "backups", "activity", "guide"]) {
    await page.getByRole("link", { name: { kernels: "内核管理", backups: "备份与恢复", activity: "操作记录", guide: "产品与开发" }[route], exact: true }).click();
    await expect(page.locator("main > .pk35-page, main > .local-page")).toBeVisible(); await expect(page.locator("main > .page-toolbar")).toHaveCount(0);
  }
  for (const [name, filename] of [["本机使用指南", "USER_GUIDE.md"], ["产品需求 PRD", "PRD.md"], ["开发与验收", "DEVELOPMENT.md"], ["内核适配合同", "KERNEL.md"]]) {
    await page.getByRole("tab", { name, exact: true }).click(); await expect(page.getByRole("tabpanel", { name, exact: true })).toBeVisible();
    const download = page.waitForEvent("download"); await page.getByRole("button", { name: "下载文档", exact: true }).click(); expect((await download).suggestedFilename()).toBe(filename);
  }
  await page.getByRole("button", { name: /^环境与固定指纹/ }).click(); await expect(page).toHaveURL(/#\/environments$/);
});
});
}
