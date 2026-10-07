import { expect, test, type Locator, type Page, type TestInfo } from "@playwright/test";
import { seedState, STORAGE_KEY, type Environment, type State } from "../../src/domain";
import type { DeviceProfile, EnvironmentPreview, WorkspaceView } from "../../src/application/contract";
import type { NativeRequest } from "../../src/application/wails-adapter";
import { nativeReferenceBridge, nativeReferenceView } from "./fixtures/native-reference-bridge";

// Actual App except the explicitly labelled, synchronous props matrix below.
// Native cases reuse the existing synthetic bridge, never desktop resources.
const errors = new WeakMap<Page, string[]>();
test.beforeEach(async ({ page, baseURL }) => {
  const messages: string[] = []; errors.set(page, messages);
  page.on("pageerror", error => messages.push(error.message));
  const origin = new URL(baseURL!).origin;
  await page.route("**/*", route => new URL(route.request().url()).origin === origin ? route.continue() : route.abort("blockedbyclient"));
  await page.routeWebSocket("**/*", socket => socket.close());
});
test.afterEach(async ({ page }) => { expect(errors.get(page)).toEqual([]); });

const status = (editor: Locator) => editor.locator(".env34-editor-status > span");
const stored = (page: Page): Promise<State> => page.evaluate(key => JSON.parse(localStorage.getItem(key)!), STORAGE_KEY);
async function demo(page: Page, noKernels = false) {
  const initial = seedState();
  initial.environments.forEach(environment => { environment.status = "ready"; });
  if (noKernels) initial.kernels.forEach(kernel => { kernel.available = false; });
  await page.addInitScript(({ key, initial }) => localStorage.setItem(key, JSON.stringify(initial)), { key: STORAGE_KEY, initial });
  await page.goto("/#/environments");
  await expect(page.getByRole("button", { name: "新建环境", exact: true })).toBeEnabled();
  await page.evaluate(key => {
    const original = Storage.prototype.setItem, writes: string[] = [];
    Object.assign(window, { __editorStorageWrites: writes });
    Storage.prototype.setItem = function (name, value) { if (name === key) writes.push(value); original.call(this, name, value); };
  }, STORAGE_KEY);
}
async function unchanged(page: Page, before: State) {
  expect(await stored(page)).toEqual(before);
  expect(await page.evaluate(() => (window as unknown as { __editorStorageWrites: string[] }).__editorStorageWrites)).toEqual([]);
}
async function geometry(editor: Locator, viewport: { width: number; height: number }) {
  expect(await editor.boundingBox()).toMatchObject({ x: viewport.width - 660, y: 40, width: 660, height: viewport.height - 48 });
  expect(await editor.locator(".drawer-header").boundingBox()).toMatchObject({ y: 40, height: 40 });
  expect(await editor.locator(".drawer-footer").boundingBox()).toMatchObject({ y: viewport.height - 79, height: 71 });
  expect(await editor.locator(".drawer-body").boundingBox()).toMatchObject({ y: 80, height: viewport.height - 159 });
  await expect(status(editor)).toBeInViewport();
}
async function evidence(page: Page, editor: Locator, info: TestInfo, label: string) {
  await info.attach(label, { body: await page.screenshot(), contentType: "image/png" });
  await info.attach(`${label}-footer`, { body: JSON.stringify({ syntheticOnly: true, text: await status(editor).textContent(), title: await status(editor).getAttribute("title") }), contentType: "application/json" });
}

// Existing guarded-creation/held-preview seams from environment-confirmation,
// expressed here without editing shared fixtures or adding demo async states.
async function nativeEditorBridge(page: Page) {
  await nativeReferenceBridge(page, true);
  await page.addInitScript(() => {
    const host = window as unknown as {
      go: { main: { DesktopApp: { Call(request: NativeRequest): Promise<unknown> } } };
      __referenceNative: { view: WorkspaceView; calls: NativeRequest[] };
      __editorNative: { releasePreview(): void; loseCreateReceipt(): void };
    };
    const original = host.go.main.DesktopApp.Call;
    let draft: EnvironmentPreview, releasePreview = () => {}, loseCreateReceipt = () => {}, unknown = false;
    const ok = (data: unknown) => ({ ok: true, mode: "native", data: structuredClone(data) });
    const fail = () => ({ ok: false, mode: "native", error: { code: "NATIVE_UNAVAILABLE", message: "合成创建回执待核实，保留原请求", retryable: true } });
    const fingerprint = (e: Environment): EnvironmentPreview["fingerprint"] => {
      const profile: DeviceProfile = { schemaVersion: 1, configRevision: 1, seed: e.seed, templateId: "windows-desktop-v1", templateVersion: e.fingerprintVersion, generatorVersion: "synthetic-only", platform: "windows", platformVersion: "15.0.0", brand: "Chrome", brandVersion: "148.0.7778.215", kernelId: e.coreId, coreActualVersion: "148.0.7778.215", coreExecutableSha256: "b".repeat(64), adapterVersion: "synthetic-only", capabilityVersion: "synthetic-only", language: e.language, acceptLanguages: [e.language], uiLanguage: "system", timezone: e.timezone, regionPreset: "synthetic-only", cpu: e.cpu, width: e.width, height: e.height, parameters: [], configHash: "synthetic-editor-profile" };
      return { mode: "native", action: "preview", previewProfile: profile, changes: [], capabilityReport: { kernelId: e.coreId, evidenceStatus: "synthetic-only", capabilities: [], observedFingerprint: null, canLaunchNative: false } };
    };
    host.__editorNative = { releasePreview() { releasePreview(); }, loseCreateReceipt() { loseCreateReceipt(); } };
    host.go.main.DesktopApp.Call = async request => {
      const p = request.payload as Record<string, unknown>;
      if (request.method === "Environment.Preview") {
        if (p.kind === "edit") {
          const result = await original(request) as { data: EnvironmentPreview };
          return ok({ ...result.data, fingerprint: fingerprint(result.data.environment) });
        }
        host.__referenceNative.calls.push(structuredClone(request));
        draft = { previewId: "synthetic-editor-create-preview", environment: { ...host.__referenceNative.view.state.environments[0], id: "", name: "", seed: "172601034" } };
        return ok(draft);
      }
      if (["Fingerprint.ListRevisions", "Fingerprint.Generate", "Environment.Create"].includes(request.method)) host.__referenceNative.calls.push(structuredClone(request));
      if (request.method === "Fingerprint.ListRevisions") return ok([]);
      if (request.method === "Fingerprint.Generate") return new Promise(resolve => {
        const e = { ...draft.environment, ...(p.overrides as Partial<Environment>) };
        releasePreview = () => resolve(ok({ ...draft, environment: e, fingerprint: fingerprint(e) }));
      });
      if (request.method === "Environment.Create") {
        if (unknown) return fail();
        return new Promise(resolve => { loseCreateReceipt = () => { unknown = true; resolve(fail()); }; });
      }
      return original(request);
    };
  });
}

for (const viewport of [{ width: 1440, height: 900 }, { width: 1280, height: 800 }]) {
  test.describe(`${viewport.width}x${viewport.height} editor blocker`, () => {
    test.use({ viewport });

    test("actual App invalid width reports failed preview, then retry restores the same unsaved draft", async ({ page }, info) => {
      await demo(page); const before = await stored(page);
      await page.clock.install({ time: new Date("2026-10-07T00:00:00Z") });
      await page.clock.pauseAt(new Date("2026-10-07T01:00:00Z"));
      await page.getByRole("button", { name: "新建环境", exact: true }).click();
      const editor = page.getByRole("dialog", { name: "新建浏览器环境", exact: true });
      await expect(editor.getByRole("button", { name: "创建", exact: true })).toBeEnabled();
      await editor.getByLabel("环境名称", { exact: true }).fill("预览恢复合成草稿");
      await editor.getByLabel("分组", { exact: true }).fill("保留的合成分组");
      await editor.getByLabel("绑定代理").selectOption("px-gb");
      const seed = await editor.getByLabel("固定指纹种子").inputValue(), core = await editor.getByLabel("浏览器内核").inputValue();
      expect(before.kernels.find(kernel => kernel.id === core)?.available).toBe(true);
      await editor.getByLabel("窗口宽度").fill("0");
      await expect(editor.getByRole("button", { name: "创建", exact: true })).toBeDisabled();
      await expect.soft(status(editor)).toHaveText("指纹预览待更新");
      await page.clock.runFor(150); // Run the real App's automatic preview validation.
      await expect(editor.getByRole("alert")).toContainText("窗口");
      await expect(editor.getByRole("button", { name: "创建并打开", exact: true })).toBeDisabled();
      await expect(editor.getByRole("button", { name: "换一套", exact: true })).toBeEnabled();
      await evidence(page, editor, info, "invalid-width-actual-App");
      await expect.soft(status(editor)).toHaveText("指纹预览失败");
      await expect.soft(status(editor)).toHaveAttribute("title", /修正.*重试指纹预览/);
      await geometry(editor, viewport); await unchanged(page, before);
      await editor.getByLabel("窗口宽度").fill("1400");
      await editor.getByRole("button", { name: "重试指纹预览", exact: true }).click();
      await expect(editor.getByRole("alert")).toHaveCount(0);
      await expect(status(editor)).toHaveText("仅演示");
      await expect(status(editor)).toHaveAttribute("title", "演示模式，不启动真实浏览器");
      await expect(editor.getByRole("button", { name: "创建", exact: true })).toBeEnabled();
      await expect(editor.getByRole("button", { name: "创建并打开", exact: true })).toBeEnabled();
      await expect(editor.getByLabel("固定指纹种子")).toHaveValue(seed);
      await expect(editor.getByLabel("环境名称", { exact: true })).toHaveValue("预览恢复合成草稿");
      await expect(editor.getByLabel("分组", { exact: true })).toHaveValue("保留的合成分组");
      await expect(editor.getByLabel("绑定代理")).toHaveValue("px-gb");
      await expect(editor.getByLabel("浏览器内核")).toHaveValue(core);
      await expect(editor.getByLabel("窗口宽度")).toHaveValue("1400");
      await geometry(editor, viewport); await unchanged(page, before);
      await evidence(page, editor, info, "recovered-draft-actual-App");
    });

    test("actual App missing kernel does not claim a failed or stale preview", async ({ page }, info) => {
      await demo(page, true); const before = await stored(page);
      await page.getByRole("button", { name: "新建环境", exact: true }).click();
      const editor = page.getByRole("dialog", { name: "新建浏览器环境", exact: true });
      await editor.getByLabel("环境名称", { exact: true }).fill("缺内核合成草稿");
      const seed = await editor.getByLabel("固定指纹种子").inputValue();
      await expect(status(editor)).toHaveText("缺少可用内核");
      await expect(status(editor)).not.toHaveText(/预览/);
      await expect.soft(status(editor)).toHaveAttribute("title", /内核管理.*草稿.*保留/);
      for (const name of ["创建", "创建并打开", "换一套"]) await expect(editor.getByRole("button", { name, exact: true })).toBeDisabled();
      await expect(editor.getByRole("button", { name: "打开内核管理", exact: true })).toBeEnabled();
      await expect(editor.getByLabel("固定指纹种子")).toHaveValue(seed);
      await geometry(editor, viewport); await unchanged(page, before);
      await evidence(page, editor, info, "missing-kernel-actual-App");
    });

    test("native App busy, generating, saving and unknown creation keep their existing gates and original request", async ({ page }, info) => {
      await nativeEditorBridge(page); await page.goto("/#/environments");
      const before = (await nativeReferenceView(page)).view.state.environments;
      await page.getByRole("button", { name: "测试环境 C 更多操作", exact: true }).click();
      await page.getByRole("button", { name: "编辑环境", exact: true }).click();
      const busy = page.getByRole("dialog", { name: "编辑浏览器环境", exact: true });
      await expect.soft(status(busy)).toHaveText("关键字段已锁定");
      await expect.soft(status(busy)).toHaveAttribute("title", /名称、分组和备注.*停止.*空闲/);
      for (const label of ["环境名称", "分组", "备注"]) await expect(busy.getByLabel(label, { exact: true })).toBeEnabled();
      for (const label of ["绑定代理", "窗口宽度", "浏览器内核"]) await expect(busy.getByLabel(label, { exact: true })).toBeDisabled();
      await expect(busy.getByRole("button", { name: "保存", exact: true })).toBeEnabled(); // Safe native metadata remains savable.
      await expect(busy.getByRole("button", { name: "换一套", exact: true })).toBeDisabled();
      await geometry(busy, viewport); await evidence(page, busy, info, "native-busy-actual-App");
      await busy.getByRole("button", { name: "取消", exact: true }).click();
      await expect(busy).toHaveCount(0);
      await page.getByRole("button", { name: "新建环境", exact: true }).click();
      const editor = page.getByRole("dialog", { name: "新建浏览器环境", exact: true });
      await editor.getByLabel("环境名称", { exact: true }).fill("原请求合成草稿");
      await editor.getByLabel("分组", { exact: true }).fill("原请求合成分组");
      await editor.getByLabel("绑定代理").selectOption("synthetic-socks");
      await expect(status(editor)).toHaveText("准备指纹中…");
      for (const name of ["创建", "创建并打开", "换一套"]) await expect(editor.getByRole("button", { name, exact: true })).toBeDisabled();
      await page.evaluate(() => (window as unknown as { __editorNative: { releasePreview(): void } }).__editorNative.releasePreview());
      await expect(status(editor)).toHaveText("本机保存");
      await expect(status(editor)).toHaveAttribute("title", "保存到本机，关闭后身份不变");
      await editor.getByRole("button", { name: "创建", exact: true }).click();
      await expect.soft(status(editor)).toHaveText("保存中…");
      for (const name of ["创建", "创建并打开", "换一套", "取消", "关闭环境配置"]) await expect(editor.getByRole("button", { name, exact: true })).toBeDisabled();
      const first = (await nativeReferenceView(page)).calls.find(call => call.method === "Environment.Create")!;
      expect(first.payload).toMatchObject({ previewId: "synthetic-editor-create-preview", configuration: { name: "原请求合成草稿", group: "原请求合成分组", proxyId: "synthetic-socks", seed: "172601034", coreId: "core-148" } });
      await page.evaluate(() => (window as unknown as { __editorNative: { loseCreateReceipt(): void } }).__editorNative.loseCreateReceipt());
      await expect(editor.getByRole("alert")).toContainText("核实");
      // Existing active-session refresh now observes an unavailable selected kernel.
      // Recovery must still describe the original request, not that lower blocker.
      await page.evaluate(() => { (window as unknown as { __referenceNative: { view: WorkspaceView } }).__referenceNative.view.state.kernels.forEach(kernel => { kernel.available = false; }); });
      await expect(editor.getByRole("button", { name: "打开内核管理", exact: true })).toBeAttached();
      await expect(status(editor)).toHaveText("原请求待核实");
      await expect(status(editor)).toHaveAttribute("title", "保留原创建请求，重试只核实结果");
      await expect(editor.getByRole("button", { name: "重试核实创建结果", exact: true })).toBeEnabled();
      await expect(editor.getByRole("button", { name: "创建并打开", exact: true })).toHaveCount(0);
      await expect(editor.getByLabel("环境名称", { exact: true })).toBeDisabled();
      await editor.getByRole("button", { name: "重试核实创建结果", exact: true }).click();
      const data = await nativeReferenceView(page), creates = data.calls.filter(call => call.method === "Environment.Create");
      expect(creates).toHaveLength(2); expect(creates[1].payload).toEqual(first.payload);
      expect(data.calls.filter(call => ["Environment.Update", "Fingerprint.CommitRevision", "Runtime.Start"].includes(call.method))).toHaveLength(0);
      expect(data.calls.filter(call => call.method === "Fingerprint.Generate").map(call => (call.payload as { regenerate: boolean }).regenerate)).toEqual([false]);
      expect(data.view.state.environments).toEqual(before);
      await expect(editor.getByLabel("固定指纹种子")).toHaveValue("172601034");
      await expect(editor.getByLabel("绑定代理")).toHaveValue("synthetic-socks");
      await expect(editor.getByLabel("浏览器内核")).toHaveValue("core-148");
      await geometry(editor, viewport); await evidence(page, editor, info, "native-unknown-original-request");
    });
  });
}

test("synchronous component props matrix preserves status priority, safe titles and every original footer gate at both sizes", async ({ page }) => {
  // A props seam, NOT an invented reachable demo async workflow or App state.
  await page.route("**/editor-status-seam", route => route.fulfill({ contentType: "text/html", body: `<!doctype html><html lang="zh-CN"><head><meta charset="UTF-8"></head><body><div id="root"></div><script type="module">
    import React from '/node_modules/.vite/deps/react.js';
    import ReactDOMClient from '/node_modules/.vite/deps/react-dom_client.js';
    import ReactDOM from '/node_modules/.vite/deps/react-dom.js';
    import { EnvironmentEditorWindow } from '/src/components/EnvironmentEditorWindow.tsx';
    import { seedState } from '/src/domain.ts';
    import '/src/styles.css';
    const state = seedState(), root = ReactDOMClient.createRoot(document.getElementById('root')), noop = () => {};
    const form = { environment: state.environments[7], kind: 'create', groups: [], kernels: state.kernels, proxies: state.proxies, native: true, busy: false, generating: false, profileBusy: false, kernelLocked: false, canGenerate: true, fresh: true, quantity: 1, previewError: '', onChange: noop, onQuantity: noop, onGenerate: noop, onRestore: noop, onImportProxy: noop, onKernels: noop, canConfigure: () => true };
    window.__editorStatusSeam = patch => ReactDOM.flushSync(() => root.render(React.createElement('div', { className: 'overlay environment-overlay' }, React.createElement(EnvironmentEditorWindow, { error: '', saving: false, canSave: true, recoveringCreation: false, onSave: noop, onClose: noop, ...patch, form: { ...form, ...patch.form } }))));
    window.__editorStatusSeam({});
  </script></body></html>` }));
  await page.goto("/editor-status-seam");
  const editor = page.getByRole("dialog");
  await expect(editor).toBeVisible();
  const cases = [
    { text: "原请求待核实", title: /原创建请求/, saving: true, recoveringCreation: true, canSave: false, form: { busy: true, profileBusy: true, generating: true, canGenerate: false, previewError: "synthetic-error" } },
    { text: "保存中…", title: /保存.*等待/, saving: true, form: { busy: true, generating: true, canGenerate: false } },
    { text: "处理中…", title: /等待.*草稿.*保留/, form: { busy: true, profileBusy: true, generating: true, canGenerate: false } },
    { text: "关键字段已锁定", title: /名称、分组和备注/, form: { kind: "edit", profileBusy: true, canGenerate: false, previewError: "synthetic-error" } },
    { text: "准备指纹中…", title: /预览.*保存/, canSave: false, form: { generating: true, canGenerate: false, previewError: "synthetic-error" } },
    { text: "缺少可用内核", title: /内核管理/, canSave: false, form: { canGenerate: false, previewError: "synthetic-error" } },
    { text: "指纹预览失败", title: /修正.*重试/, canSave: false, form: { previewError: "SYNTHETIC_RAW_PAYLOAD " + "W".repeat(200) } },
    { text: "指纹预览失败", title: /修正.*重试/, form: { previewError: "SYNTHETIC_RAW_PAYLOAD" } }, // A failed regeneration must not add a save gate.
    { text: "指纹预览待更新", title: /当前输入.*重试/, canSave: false, form: { fresh: false } },
    { text: "本机保存", title: /保存到本机/, form: {} },
    { text: "仅演示", title: /演示模式/, form: { native: false } },
  ];
  for (const viewport of [{ width: 1440, height: 900 }, { width: 1280, height: 800 }]) {
    await page.setViewportSize(viewport);
    for (const entry of cases) {
      const { text, title, ...patch } = entry;
      await page.evaluate(patch => (window as unknown as { __editorStatusSeam(patch: unknown): void }).__editorStatusSeam(patch), patch);
      expect.soft(await status(editor).textContent()).toBe(text);
      expect.soft(await status(editor).getAttribute("title")).toMatch(title);
      expect(await status(editor).getAttribute("title")).not.toContain("SYNTHETIC_RAW_PAYLOAD");
      const saving = "saving" in patch && patch.saving, recovering = "recoveringCreation" in patch && patch.recoveringCreation;
      const canSave = !("canSave" in patch) || patch.canSave;
      const busy = "busy" in patch.form && patch.form.busy, profileBusy = "profileBusy" in patch.form && patch.form.profileBusy;
      const generating = "generating" in patch.form && patch.form.generating, canGenerate = !("canGenerate" in patch.form) || patch.form.canGenerate;
      await expect(editor.getByRole("button", { name: "换一套", exact: true })).toBeEnabled({ enabled: !(saving || recovering || busy || profileBusy || generating || !canGenerate) });
      await expect(editor.getByRole("button", { name: recovering ? "重试核实创建结果" : "kind" in patch.form ? "保存" : "创建", exact: true })).toBeEnabled({ enabled: !(saving || !recovering && (generating || !canSave)) });
      await expect(editor.getByRole("button", { name: "取消", exact: true })).toBeEnabled({ enabled: !(saving || recovering) });
      await expect(editor.getByRole("button", { name: "关闭环境配置", exact: true })).toBeEnabled({ enabled: !(saving || recovering) });
      if (!recovering && !("kind" in patch.form)) await expect(editor.getByRole("button", { name: "创建并打开", exact: true })).toBeEnabled({ enabled: !(generating || saving || !canSave) });
      await geometry(editor, viewport);
    }
  }
});
