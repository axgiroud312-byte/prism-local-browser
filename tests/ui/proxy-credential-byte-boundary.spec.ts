import { expect, test, type Page } from "@playwright/test";
import { seedState, STORAGE_KEY, type State } from "../../src/domain";
import { proxyKernelBridge } from "./fixtures/proxy-kernel";

const proxyId = "synthetic-proxy-byte-boundary";
const proxyName = "合成字节边界";
const editorName = "新用户名（不回显旧值）";
const byteError = /SOCKS5.*1–255.*UTF-8/;
const boundaryPairs = [
  { name: "ASCII", valid: "s".repeat(255), invalid: "s".repeat(256) },
  { name: "Unicode", valid: "界".repeat(85), invalid: "界".repeat(84) + "🧪" },
];

test.beforeEach(async ({ page }) => {
  await page.route("**/*", route => new URL(route.request().url()).hostname === "127.0.0.1" ? route.continue() : route.abort());
  await page.routeWebSocket("**/*", socket => socket.close());
});

async function openDemo(page: Page) {
  const state = seedState();
  state.proxies = [{ id: proxyId, name: proxyName, type: "socks5", host: "192.0.2.35", port: 1080, country: "US", username: "synthetic-old-user", password: "synthetic-old-password", status: "connected", latency: 42 }];
  state.environments = state.environments.map(environment => ({ ...environment, proxyId, status: "ready" }));
  await page.addInitScript(({ key, state }) => {
    localStorage.setItem(key, JSON.stringify(state));
    const setItem = Storage.prototype.setItem;
    Storage.prototype.setItem = function (name, value) {
      if ((window as unknown as { __proxyByteWriteBlocked?: boolean }).__proxyByteWriteBlocked && name === key) throw new DOMException("synthetic blocked write", "QuotaExceededError");
      return setItem.call(this, name, value);
    };
  }, { key: STORAGE_KEY, state });
  await page.goto("/#/proxies");
  await expect(page.getByRole("button", { name: `编辑代理 ${proxyName}`, exact: true })).toBeVisible();
}

const savedRaw = (page: Page) => page.evaluate(key => localStorage.getItem(key), STORAGE_KEY);
async function savedState(page: Page): Promise<State> { return JSON.parse((await savedRaw(page))!); }
async function openEditor(page: Page) {
  await page.getByRole("button", { name: `编辑代理 ${proxyName}`, exact: true }).click();
  return page.getByRole("dialog", { name: "修改代理", exact: true });
}
const proxyURI = (type: string, username: string, password: string, suffix: number) => `${type}://${encodeURIComponent(username)}:${encodeURIComponent(password)}@192.0.2.${suffix}:1080`;

for (const pair of boundaryPairs) {
  for (const field of ["username", "password"] as const) {
    test(`demo SOCKS5 editor rejects 256 and accepts 255 decoded UTF-8 bytes: ${pair.name} ${field}`, async ({ page }) => {
      expect(Buffer.byteLength(pair.valid)).toBe(255);
      expect(Buffer.byteLength(pair.invalid)).toBe(256);
      await openDemo(page);
      const before = await savedRaw(page), identity = (await savedState(page)).environments;
      const edit = await openEditor(page);
      await edit.getByLabel("名称", { exact: true }).fill("合成未保存草稿");
      await edit.getByLabel("认证处理").selectOption("replace");
      await edit.getByLabel(editorName).fill(field === "username" ? pair.invalid : "synthetic:user");
      await edit.getByLabel("新密码", { exact: true }).fill(field === "password" ? pair.invalid : "synthetic-pass");
      if (field === "username") await edit.getByRole("button", { name: "确认保存", exact: true }).click();
      else await edit.getByLabel("新密码", { exact: true }).press("Enter");
      await expect(edit.getByRole("alert")).toContainText(byteError);
      await expect(edit.getByLabel("名称", { exact: true })).toHaveValue("合成未保存草稿");
      await expect(edit.getByLabel(field === "username" ? editorName : "新密码", { exact: true })).toHaveValue(pair.invalid);
      expect(await savedRaw(page)).toBe(before);
      await edit.getByLabel(field === "username" ? editorName : "新密码", { exact: true }).fill(pair.valid);
      await edit.getByRole("button", { name: "确认保存", exact: true }).click();
      await expect(edit).toHaveCount(0);
      const after = await savedState(page);
      expect(after.proxies[0][field]).toBe(pair.valid);
      expect(after.proxies[0]).toMatchObject({ id: proxyId, name: "合成未保存草稿", status: "unchecked" });
      expect(after.environments).toEqual(identity);
    });
  }
}

for (const missing of ["username", "password", "both"] as const) {
  test(`demo SOCKS5 replace requires both nonempty credentials: missing ${missing}`, async ({ page }) => {
    await openDemo(page);
    const before = await savedRaw(page), edit = await openEditor(page);
    await edit.getByLabel("认证处理").selectOption("replace");
    if (missing === "password") await edit.getByLabel(editorName).fill("synthetic-user");
    if (missing === "username") await edit.getByLabel("新密码", { exact: true }).fill("synthetic-pass");
    await edit.getByRole("button", { name: "确认保存", exact: true }).click();
    await expect(edit.getByRole("alert")).toContainText(byteError);
    expect(await savedRaw(page)).toBe(before);
    await edit.getByRole("button", { name: "取消", exact: true }).click();
    await expect(edit).toHaveCount(0);
    expect(await savedRaw(page)).toBe(before);
  });
}

for (const action of ["keep", "clear"] as const) {
  test(`demo SOCKS5 ${action} does not validate discarded replacement fields`, async ({ page }) => {
    await openDemo(page);
    const edit = await openEditor(page);
    await edit.getByLabel("认证处理").selectOption("replace");
    await edit.getByLabel(editorName).fill(boundaryPairs[0].invalid);
    await edit.getByLabel("新密码", { exact: true }).fill("");
    await edit.getByLabel("认证处理").selectOption(action);
    await edit.getByRole("button", { name: "确认保存", exact: true }).click();
    await expect(edit).toHaveCount(0);
    expect((await savedState(page)).proxies[0]).toMatchObject(action === "keep" ? { username: "synthetic-old-user", password: "synthetic-old-password" } : { username: "", password: "" });
  });
}

for (const protocol of ["http", "https"] as const) {
  test(`demo ${protocol} replacements do not inherit the SOCKS5 255-byte bound`, async ({ page }) => {
    await openDemo(page);
    const edit = await openEditor(page);
    await edit.getByLabel("协议", { exact: true }).selectOption(protocol);
    await edit.getByLabel("认证处理").selectOption("replace");
    await edit.getByLabel(editorName).fill(boundaryPairs[0].invalid);
    await edit.getByLabel("新密码", { exact: true }).fill(boundaryPairs[1].invalid);
    await edit.getByRole("button", { name: "确认保存", exact: true }).click();
    await expect(edit).toHaveCount(0);
    expect((await savedState(page)).proxies[0]).toMatchObject({ type: protocol, username: boundaryPairs[0].invalid, password: boundaryPairs[1].invalid });
  });
}

test("demo invalid draft can cancel; a corrected draft survives failed persistence and retries", async ({ page }) => {
  await openDemo(page);
  const before = await savedRaw(page), edit = await openEditor(page);
  await edit.getByLabel("认证处理").selectOption("replace");
  await edit.getByLabel(editorName).fill(boundaryPairs[1].invalid);
  await edit.getByLabel("新密码", { exact: true }).fill("synthetic-pass");
  await edit.getByRole("button", { name: "确认保存", exact: true }).click();
  await expect(edit.getByRole("alert")).toContainText(byteError);
  await edit.getByRole("button", { name: "取消", exact: true }).click();
  expect(await savedRaw(page)).toBe(before);
  await openEditor(page);
  await edit.getByLabel("名称", { exact: true }).fill("合成重试草稿");
  await edit.getByLabel("认证处理").selectOption("replace");
  await edit.getByLabel(editorName).fill(boundaryPairs[1].valid);
  await edit.getByLabel("新密码", { exact: true }).fill("synthetic-pass");
  await page.evaluate(() => { (window as unknown as { __proxyByteWriteBlocked: boolean }).__proxyByteWriteBlocked = true; });
  await edit.getByRole("button", { name: "确认保存", exact: true }).click();
  await expect(edit.getByRole("status")).toContainText("STORAGE_WRITE_FAILED");
  await expect(edit.getByLabel("名称", { exact: true })).toHaveValue("合成重试草稿");
  await expect(edit.getByLabel(editorName)).toHaveValue(boundaryPairs[1].valid);
  expect(await savedRaw(page)).toBe(before);
  await page.evaluate(() => { (window as unknown as { __proxyByteWriteBlocked: boolean }).__proxyByteWriteBlocked = false; });
  await edit.getByRole("button", { name: "确认保存", exact: true }).click();
  await expect(edit).toHaveCount(0);
  expect((await savedState(page)).proxies[0]).toMatchObject({ name: "合成重试草稿", username: boundaryPairs[1].valid });
});

test("demo decoded import boundaries keep invalid original lines unselected across hide, failed save and valid-only commit", async ({ page }) => {
  await openDemo(page);
  const before = await savedRaw(page);
  const rows = [
    { raw: proxyURI("socks5", boundaryPairs[0].valid, "synthetic-pass", 41), valid: true },
    { raw: `  ${proxyURI("socks5", boundaryPairs[0].invalid, "synthetic-pass", 41)}  `, valid: false },
    { raw: proxyURI("socks5", "synthetic:user", boundaryPairs[1].valid, 43), valid: true },
    { raw: proxyURI("socks5", "synthetic:user", boundaryPairs[1].invalid, 44), valid: false },
    { raw: proxyURI("socks5", boundaryPairs[1].valid, boundaryPairs[0].valid, 45), valid: true },
    { raw: proxyURI("socks5", boundaryPairs[1].invalid, "synthetic-pass", 46), valid: false },
    { raw: proxyURI("socks5", "synthetic-user", boundaryPairs[0].invalid, 47), valid: false },
    { raw: proxyURI("socks5", "", "synthetic-pass", 48), valid: false },
    { raw: proxyURI("socks5", "synthetic-user", "", 49), valid: false },
    { raw: "socks5://192.0.2.50:1080", valid: true },
    { raw: proxyURI("http", boundaryPairs[0].invalid, boundaryPairs[1].invalid, 51), valid: true },
    { raw: proxyURI("https", boundaryPairs[1].invalid, boundaryPairs[0].invalid, 52), valid: true },
    { raw: proxyURI("socks5", "u", "p", 53), valid: true },
    { raw: "socks5://:@192.0.2.54:1080", valid: false },
    { raw: "socks5://@192.0.2.55:1080", valid: false },
  ];
  const text = ["# synthetic byte boundary", "", ...rows.map(row => row.raw)].join("\n");
  await page.getByRole("button", { name: "批量添加代理", exact: true }).click();
  const dialog = page.getByRole("dialog", { name: "批量添加代理", exact: true });
  await dialog.getByLabel("原始代理导入文本").fill(text);
  await dialog.getByRole("button", { name: "解析预览", exact: true }).click();
  for (const [index, row] of rows.entries()) {
    const select = dialog.getByLabel(`保存第${index + 3}行`, { exact: true });
    if (row.valid) await expect(select).toBeChecked();
    else {
      await expect(select).toBeDisabled();
      await expect(select).not.toBeChecked();
      await expect(dialog.getByRole("row").filter({ has: page.getByLabel(`保存第${index + 3}行`, { exact: true }) })).toContainText(byteError);
    }
  }
  await expect(dialog.getByLabel("原始代理导入文本")).toHaveValue(text);
  await expect(dialog.locator("tbody")).not.toContainText("synthetic:user");
  await dialog.getByRole("button", { name: "取消", exact: true }).click();
  await page.getByRole("link", { name: "内核管理", exact: true }).click();
  await page.getByRole("link", { name: "代理管理", exact: true }).click();
  await page.getByRole("button", { name: "批量添加代理", exact: true }).click();
  await expect(dialog.getByLabel("原始代理导入文本")).toHaveValue(text);
  await expect(dialog.getByLabel("保存第4行", { exact: true })).toBeDisabled();
  await page.evaluate(() => { (window as unknown as { __proxyByteWriteBlocked: boolean }).__proxyByteWriteBlocked = true; });
  await dialog.getByRole("button", { name: "保存所选有效行", exact: true }).click();
  await expect(dialog.locator("footer")).toContainText("STORAGE_WRITE_FAILED");
  await expect(dialog.getByLabel("原始代理导入文本")).toHaveValue(text);
  await expect(dialog.getByLabel("保存第3行", { exact: true })).toBeChecked();
  expect(await savedRaw(page)).toBe(before);
  await page.evaluate(() => { (window as unknown as { __proxyByteWriteBlocked: boolean }).__proxyByteWriteBlocked = false; });
  await dialog.getByRole("button", { name: "保存所选有效行", exact: true }).click();
  await expect(dialog.getByLabel("原始代理导入文本")).toHaveValue(["# synthetic byte boundary", "", ...rows.filter(row => !row.valid).map(row => row.raw)].join("\n"));
  const after = await savedState(page);
  expect(after.proxies).toHaveLength(1 + rows.filter(row => row.valid).length);
  expect(after.proxies.slice(1).map(proxy => proxy.host)).toEqual(["192.0.2.41", "192.0.2.43", "192.0.2.45", "192.0.2.50", "192.0.2.51", "192.0.2.52", "192.0.2.53"]);
  expect(after.proxies[2]).toMatchObject({ username: "synthetic:user", password: boundaryPairs[1].valid });
  expect(after.environments).toEqual(JSON.parse(before!).environments);
});

test("demo single-node import uses the same decoded byte gate and can correct its draft", async ({ page }) => {
  await openDemo(page);
  const errors: string[] = [];
  page.on("pageerror", error => errors.push(error.message));
  const [windowSource, mainSource] = await Promise.all([
    page.request.get("/src/components/ProxyImportWindow.tsx").then(response => response.text()),
    page.request.get("/src/main.tsx").then(response => response.text()),
  ]);
  const reactURL = windowSource.match(/from\s*["']([^"']*\/react\.js[^"']*)["']/)?.[1];
  const rootURL = mainSource.match(/from\s*["']([^"']*\/react-dom_client\.js[^"']*)["']/)?.[1];
  expect(reactURL).toBeTruthy();
  expect(rootURL).toBeTruthy();
  // Standalone single-mode export: the current demo menu maps to batch text.
  await page.route("**/proxy-byte-single", route => route.fulfill({ contentType: "text/html", body: '<!doctype html><html lang="zh-CN"><head><meta charset="UTF-8"></head><body><div id="root"></div><script type="module" src="/proxy-byte-single.js"></script></body></html>' }));
  await page.route("**/proxy-byte-single.js", route => route.fulfill({ contentType: "text/javascript", body: `
    import React from ${JSON.stringify(reactURL)};
    import * as ReactDOM from ${JSON.stringify(rootURL)};
    import { DemoAdapter } from "/src/application/demo-adapter.ts";
    import { ProxyImportWindow } from "/src/components/ProxyImportWindow.tsx";
    import "/src/styles.css";
    const createRoot = ReactDOM.createRoot ?? ReactDOM.default.createRoot;
    const app = new DemoAdapter(localStorage);
    function Harness() {
      const [open, setOpen] = React.useState(true);
      return React.createElement(ProxyImportWindow, { application: app, open, initialMode: "single", onClose: () => setOpen(false) });
    }
    createRoot(document.getElementById("root")).render(React.createElement(Harness));
  ` }));
  await page.goto("/proxy-byte-single");
  const dialog = page.getByRole("dialog", { name: "添加代理", exact: true });
  await expect.poll(async () => errors.length ? errors.join("; ") : await dialog.count() ? "mounted" : "pending").toBe("mounted");
  await dialog.getByLabel("添加代理主机").fill("192.0.2.60");
  await dialog.getByLabel("添加代理账号").fill(boundaryPairs[1].invalid);
  await dialog.getByLabel("添加代理密码").fill("synthetic-pass");
  await dialog.getByRole("button", { name: "解析预览", exact: true }).click();
  await expect(dialog).toContainText(byteError);
  await expect(dialog.getByRole("button", { name: "保存所选有效行", exact: true })).toBeDisabled();
  await expect(dialog.getByLabel("添加代理账号")).toHaveValue(boundaryPairs[1].invalid);
  expect((await savedState(page)).proxies).toHaveLength(1);
  await dialog.getByLabel("添加代理账号").fill(boundaryPairs[1].valid);
  await dialog.getByRole("button", { name: "解析预览", exact: true }).click();
  await expect(dialog.getByLabel("保存第1行")).toBeChecked();
  await dialog.getByRole("button", { name: "保存所选有效行", exact: true }).click();
  await expect(dialog).toHaveCount(0);
  expect((await savedState(page)).proxies[1].username).toBe(boundaryPairs[1].valid);
});

test("native 256-byte replacement still reaches the authoritative service rejection unchanged", async ({ page }) => {
  await proxyKernelBridge(page);
  await page.goto("/#/proxies");
  await page.evaluate(() => {
    const host = (window as unknown as { go: { main: { DesktopApp: { Call(request: { method: string; payload: Record<string, unknown> }): Promise<unknown> } } }; __proxyKernel: { calls: unknown[] } });
    const original = host.go.main.DesktopApp.Call;
    host.go.main.DesktopApp.Call = request => {
      if (request.method !== "Proxy.Update") return original(request);
      host.__proxyKernel.calls.push(structuredClone(request));
      return Promise.resolve({ ok: false, mode: "native", error: { code: "VALIDATION_FAILED", message: "合成服务拒绝：SOCKS5各需1–255个UTF-8字节", retryable: false } });
    };
  });
  await page.getByRole("button", { name: "编辑代理 合成 SOCKS", exact: true }).click();
  const edit = page.getByRole("dialog", { name: "修改代理", exact: true });
  await edit.getByLabel("认证处理").selectOption("replace");
  await edit.getByLabel(editorName).fill(boundaryPairs[0].invalid);
  await edit.getByLabel("新密码", { exact: true }).fill("synthetic-pass");
  await edit.getByRole("button", { name: "确认保存", exact: true }).click();
  await expect(edit.getByRole("status")).toContainText("VALIDATION_FAILED");
  await expect(edit.getByLabel(editorName)).toHaveValue(boundaryPairs[0].invalid);
  await expect(edit.getByRole("alert")).toHaveCount(0);
  const calls = await nativeUpdates(page);
  expect(calls).toHaveLength(1);
  expect(calls[0].payload).toMatchObject({ proxyId: "synthetic-proxy-a", expectedRevision: 1, credentials: { action: "replace", username: boundaryPairs[0].invalid, password: "synthetic-pass" } });
});

async function nativeUpdates(page: Page) {
  return page.evaluate(() => (window as unknown as { __proxyKernel: { calls: { method: string; payload: Record<string, unknown> }[] } }).__proxyKernel.calls.filter(call => call.method === "Proxy.Update"));
}

test("native unknown original 256-byte request can still reconcile across route return", async ({ page }) => {
  await proxyKernelBridge(page);
  await page.goto("/#/proxies");
  await page.evaluate(() => (window as unknown as { __proxyKernel: { setUpdateOutcome(value: string): void } }).__proxyKernel.setUpdateOutcome("unknown"));
  await page.getByRole("button", { name: "编辑代理 合成 SOCKS", exact: true }).click();
  const edit = page.getByRole("dialog", { name: "修改代理", exact: true });
  await edit.getByLabel("认证处理").selectOption("replace");
  await edit.getByLabel(editorName).fill(boundaryPairs[1].invalid);
  await edit.getByLabel("新密码", { exact: true }).fill("synthetic-pass");
  await edit.getByRole("button", { name: "确认保存", exact: true }).click();
  await expect(edit.getByRole("status")).toContainText("NATIVE_UNAVAILABLE");
  await expect(edit.getByLabel(editorName)).toBeDisabled();
  await edit.getByRole("button", { name: "取消", exact: true }).click();
  await page.getByRole("link", { name: "内核管理", exact: true }).click();
  await page.getByRole("link", { name: "代理管理", exact: true }).click();
  await expect(edit.getByLabel(editorName)).toHaveValue(boundaryPairs[1].invalid);
  await edit.getByRole("button", { name: "核实原保存请求", exact: true }).click();
  await expect(edit).toHaveCount(0);
  const calls = await nativeUpdates(page);
  expect(calls).toHaveLength(2);
  expect(calls[1].payload).toEqual(calls[0].payload);
  expect(calls[0].payload).toMatchObject({ expectedRevision: 1, credentials: { action: "replace", username: boundaryPairs[1].invalid, password: "synthetic-pass" } });
});
