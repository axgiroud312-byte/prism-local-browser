import { test, expect, type Page } from "@playwright/test";
import { createSnapshot, STORAGE_KEY, type State } from "../../src/domain";
import type { NativeRequest } from "../../src/application/wails-adapter";
import type { Operation } from "../../src/application/contract";
import { referenceWorkspace } from "./fixtures/reference-workspace";
import { localPagesNativeBridge, type LocalPageScenario } from "./fixtures/local-pages-native-bridge";

const harness = "/tests/ui/fixtures/local-pages.html";
type Fixture = { calls: NativeRequest[]; operations: Record<string, Operation>; workspace: { state: State; issue?: { code: string; message: string; retryable: boolean } }; publish(id: string): void; rollback(id: string): void };
const fixture = (page: Page) => page.evaluate(() => { const f = (window as unknown as { __localPagesFixture: Fixture }).__localPagesFixture; return { calls: f.calls, operations: f.operations, state: f.workspace.state }; });
const stored = (page: Page) => page.evaluate(key => JSON.parse(localStorage.getItem(key)!) as State, STORAGE_KEY);
const calls = async (page: Page, method: string) => (await fixture(page)).calls.filter(call => call.method === method);
const dialog = (page: Page, name: string) => page.getByRole("dialog", { name, exact: true });
const pageErrors = new WeakMap<Page, string[]>();

test.beforeEach(async ({ page, baseURL }) => {
  const errors: string[] = []; pageErrors.set(page, errors); page.on("pageerror", error => errors.push(error.message));
  // Only this test's Vite origin is reachable; no product network/host probes.
  const origin = new URL(baseURL!).origin;
  await page.route("**/*", route => new URL(route.request().url()).origin === origin ? route.continue() : route.abort("blockedbyclient"));
  // No product socket is used here. Reject Vite HMR too: a queued dev-server
  // full-reload must not restart the synthetic adapter halfway through a test.
  await page.routeWebSocket("**/*", socket => socket.close());
});
test.afterEach(async ({ page }) => { expect(pageErrors.get(page)).toEqual([]); });

async function openNative(page: Page, scenario: LocalPageScenario, route = "backups", selected = "", blocked = "") {
  await localPagesNativeBridge(page, scenario);
  const query = new URLSearchParams(); if (selected) query.set("selected", selected); if (blocked) query.set("blocked", blocked);
  await page.goto(`${harness}${query.size ? `?${query}` : ""}#/${route}`);
  await expect.poll(async () => (await calls(page, "Workspace.Read")).length).toBeGreaterThan(0);
  await expect(page.getByRole("button", { name: route === "activity" ? "生成诊断预览" : "导入完整备份", exact: true })).toBeEnabled();
}
async function openDemo(page: Page, route = "backups", running = false, recordCount = 1) {
  const state = referenceWorkspace(), snapshot = createSnapshot(state); snapshot.createdAt = "2026-10-06T01:00:00Z";
  state.backups = [{ id: "synthetic-demo-backup", name: "工作区合成快照", createdAt: snapshot.createdAt, snapshot }];
  state.activities = Array.from({ length: recordCount }, (_, index) => ({ id: `synthetic-record-${index}`, time: snapshot.createdAt, action: "创建合成环境", target: index ? `工作环境 A ${index + 1}` : "工作环境 A", detail: "合成数据，非真实桌面执行。", result: "success" as const }));
  if (running) state.environments[0].status = "running";
  await page.addInitScript(({ state, key }) => { localStorage.setItem(key, JSON.stringify(state)); }, { state, key: STORAGE_KEY });
  await page.goto(`${harness}#/${route}`);
  await expect(page.locator("main > section")).toBeVisible();
}
async function preflight(page: Page) {
  await page.getByRole("button", { name: "导入完整备份", exact: true }).click();
  await dialog(page, "导入本机备份").getByRole("button", { name: "选择本机备份包", exact: true }).click();
  await dialog(page, "导入本机备份").getByRole("button", { name: "完整校验并预览", exact: true }).click();
  await expect(dialog(page, "恢复前只读预检")).toBeVisible();
  await expect(dialog(page, "恢复前只读预检").getByRole("row").filter({ hasText: "工作环境 A" })).toBeVisible();
}
async function confirmNative(page: Page) {
  await dialog(page, "恢复前只读预检").getByRole("button", { name: "下一步：确认恢复", exact: true }).click();
  const confirmation = dialog(page, "确认完整恢复");
  await confirmation.getByLabel(/确认覆盖/).check(); await confirmation.getByLabel(/了解 .*项代理凭据/).check(); await confirmation.getByLabel(/允许正常停止/).check();
  await confirmation.getByRole("button", { name: "确认并完整恢复", exact: true }).click();
  await expect(dialog(page, "完整恢复结果")).toBeVisible();
}

for (const viewport of [{ width: 1440, height: 900 }, { width: 1280, height: 800 }]) {
  test.describe(`${viewport.width}x${viewport.height}`, () => {
    test.use({ viewport });
    test("demo backup storage failure retains window and original state, then retries", async ({ page }) => {
      await openDemo(page);
      const before = await stored(page);
      await page.evaluate(() => { const original = Storage.prototype.setItem; Object.assign(window, { restoreLocalWrites: () => { Storage.prototype.setItem = original; } }); Storage.prototype.setItem = () => { throw new DOMException("synthetic write failure", "QuotaExceededError"); }; });
      await page.getByRole("button", { name: "创建快照", exact: true }).click();
      const create = dialog(page, "创建演示快照"); await create.getByRole("button", { name: "创建快照", exact: true }).click();
      await expect(create.getByRole("alert")).toContainText("未保存"); expect(await stored(page)).toEqual(before);
      await page.evaluate(() => (window as unknown as { restoreLocalWrites(): void }).restoreLocalWrites());
      await create.getByRole("button", { name: "创建快照", exact: true }).click(); await expect(create).toHaveCount(0); expect((await stored(page)).backups).toHaveLength(2);
    });
    test("demo JSON invalid import and cancel do not mutate; restore failure preserves confirmation", async ({ page }) => {
      await openDemo(page); const before = await stored(page);
      await page.getByRole("button", { name: "导入快照文件", exact: true }).click();
      const input = page.getByLabel("演示快照文件", { exact: true });
      await input.setInputFiles({ name: "invalid.json", mimeType: "application/json", buffer: Buffer.from('{"format":"prism-local-diagnostics"}') });
      await dialog(page, "导入演示快照").getByRole("button", { name: "校验并预览", exact: true }).click();
      await expect(dialog(page, "导入演示快照").getByRole("alert")).toBeVisible(); expect(await stored(page)).toEqual(before);
      await page.keyboard.press("Escape"); await expect(page.getByRole("button", { name: "导入快照文件", exact: true })).toBeFocused();
      await page.getByRole("button", { name: "恢复", exact: true }).click(); await dialog(page, "演示快照恢复预览").getByRole("button", { name: "下一步：确认恢复", exact: true }).click();
      await page.evaluate(() => { Storage.prototype.setItem = () => { throw new DOMException("synthetic write failure", "QuotaExceededError"); }; });
      await dialog(page, "确认恢复演示快照").getByRole("button", { name: "确认恢复", exact: true }).click();
      await expect(dialog(page, "确认恢复演示快照").getByRole("alert")).toBeVisible(); expect(await stored(page)).toEqual(before);
      await page.keyboard.press("Escape"); await expect(dialog(page, "演示快照恢复预览")).toBeVisible();
      await page.keyboard.press("Escape"); await expect(page.getByRole("dialog")).toHaveCount(0);
    });
    test("demo compatible import is read-only until saved restore and keeps identity/history", async ({ page }) => {
      await openDemo(page); const before = await stored(page), snapshot = createSnapshot(referenceWorkspace());
      snapshot.environments[0].name = "已恢复合成环境"; snapshot.environments[0].status = "running"; snapshot.proxies[0].password = "synthetic-only-not-real";
      await page.getByRole("button", { name: "导入快照文件", exact: true }).click();
      await page.getByLabel("演示快照文件", { exact: true }).setInputFiles({ name: "synthetic-compatible.json", mimeType: "application/json", buffer: Buffer.from(JSON.stringify(snapshot)) });
      await dialog(page, "导入演示快照").getByRole("button", { name: "校验并预览", exact: true }).click(); await expect(dialog(page, "演示快照恢复预览")).toContainText("已恢复合成环境"); expect(await stored(page)).toEqual(before);
      await dialog(page, "演示快照恢复预览").getByRole("button", { name: "下一步：确认恢复", exact: true }).click(); await dialog(page, "确认恢复演示快照").getByRole("button", { name: "确认恢复", exact: true }).click();
      await expect(page.getByRole("dialog")).toHaveCount(0); await expect(page.getByRole("button", { name: "导入快照文件", exact: true })).toBeFocused();
      const after = await stored(page); expect(after.environments.map(e => [e.id, e.seed, e.coreId, e.proxyId])).toEqual(snapshot.environments.map(e => [e.id, e.seed, e.coreId, e.proxyId])); expect(after.environments.every(e => e.status === "ready")).toBe(true); expect(after.environments[0].name).toBe("已恢复合成环境"); expect(after.proxies.every(p => p.password === "" && p.status === "unchecked")).toBe(true); expect(after.backups).toEqual(before.backups); expect(after.activities).toEqual(before.activities);
    });
    test("demo busy restore blocks submission and snapshot export retains compatible format", async ({ page }) => {
      await openDemo(page, "backups", true); const before = await stored(page);
      await page.getByRole("button", { name: "导出", exact: true }).click(); const downloads = await page.evaluate(() => (window as unknown as { __localPagesHarness: { downloads: { content: string }[] } }).__localPagesHarness.downloads); expect(JSON.parse(downloads[0].content)).toMatchObject({ format: "prism-prototype", schemaVersion: 1 });
      await page.getByRole("button", { name: "恢复", exact: true }).click(); await dialog(page, "演示快照恢复预览").getByRole("button", { name: "下一步：确认恢复", exact: true }).click();
      await expect(dialog(page, "确认恢复演示快照").getByRole("button", { name: "确认恢复", exact: true })).toBeDisabled(); await expect(dialog(page, "确认恢复演示快照")).toContainText("请先停止 1 个"); expect(await stored(page)).toEqual(before);
    });
    test("native preflight and discard are read-only, scoped and have one top focus owner", async ({ page }) => {
      await openNative(page, "baseline"); const before = await fixture(page); await preflight(page);
      expect((await calls(page, "Workspace.Read")).length).toBe(before.calls.filter(c => c.method === "Workspace.Read").length);
      expect(await calls(page, "Backup.ApplyRestore")).toHaveLength(0); expect((await fixture(page)).state).toEqual(before.state);
      const frame = dialog(page, "恢复前只读预检"); expect(Math.round((await frame.boundingBox())!.width)).toBe(1040);
      await frame.getByRole("button", { name: "下一步：确认恢复", exact: true }).click();
      await expect(dialog(page, "确认完整恢复").getByRole("button", { name: "关闭确认完整恢复", exact: true })).toBeFocused();
      await expect(page.locator(".sidebar")).toHaveJSProperty("inert", true); await expect(page.locator(".main-shell")).toHaveJSProperty("inert", true); expect(await page.locator(".local-page-overlay[inert]").evaluate(e => getComputedStyle(e).backgroundColor)).toBe("rgba(0, 0, 0, 0)");
      await page.evaluate(() => { Object.assign(window, { lowerModalKeys: 0 }); window.addEventListener("keydown", e => { if (e.key === "Escape" || e.key === "Tab" || e.ctrlKey && e.key.toLowerCase() === "k") (window as unknown as { lowerModalKeys: number }).lowerModalKeys++; }); });
      await page.keyboard.press("Control+k"); await expect(dialog(page, "确认完整恢复")).toBeVisible();
      await page.keyboard.press("Shift+Tab"); await expect(dialog(page, "确认完整恢复").getByRole("button", { name: "取消", exact: true })).toBeFocused();
      await page.keyboard.press("Escape"); await expect(frame).toBeVisible(); await expect(frame.getByRole("button", { name: "下一步：确认恢复", exact: true })).toBeFocused();
      await page.keyboard.press("Escape"); await expect(page.getByRole("dialog")).toHaveCount(0); expect(await calls(page, "Backup.ApplyRestore")).toHaveLength(0); expect((await fixture(page)).state).toEqual(before.state);
      expect((await calls(page, "Workspace.Read")).length).toBe(before.calls.filter(c => c.method === "Workspace.Read").length); expect(await calls(page, "Backup.DiscardRestore")).toHaveLength(1);
      await expect(page.getByRole("button", { name: "导入完整备份", exact: true })).toBeFocused();
      await expect(page.locator(".sidebar")).toHaveJSProperty("inert", false); await expect(page.locator(".main-shell")).toHaveJSProperty("inert", false); expect(await page.evaluate(() => (window as unknown as { lowerModalKeys: number }).lowerModalKeys)).toBe(0); expect(await page.evaluate(() => document.body.style.overflow)).toBe("");
    });
    test("native chooser cancellation and preflight failure preserve inputs without apply", async ({ page }) => {
      await openNative(page, "backup-cancel"); await page.getByRole("button", { name: "创建完整备份", exact: true }).click();
      const create = dialog(page, "创建完整本机备份"); await create.getByRole("button", { name: "选择新备份文件位置", exact: true }).click();
      await expect(create.getByRole("alert")).toContainText("已取消"); expect(await calls(page, "Backup.Export")).toHaveLength(0); await page.keyboard.press("Escape");
      await page.getByRole("button", { name: "导入完整备份", exact: true }).click(); await expect(dialog(page, "导入本机备份")).toBeVisible(); await page.keyboard.press("Escape");
      expect(await calls(page, "Backup.ApplyRestore")).toHaveLength(0);
    });
    for (const scenario of ["source-cancel", "preflight-failed"] as const) {
      test(`${scenario} remains read-only and retains the source until explicit discard`, async ({ page }) => {
        await openNative(page, scenario); const before = (await fixture(page)).state;
        await page.getByRole("button", { name: "导入完整备份", exact: true }).click(); const input = dialog(page, "导入本机备份"); await input.getByRole("button", { name: "选择本机备份包", exact: true }).click();
        if (scenario === "source-cancel") { await expect(input).toContainText("已取消"); await expect(input.getByRole("button", { name: "完整校验并预览", exact: true })).toBeDisabled(); expect(await calls(page, "Backup.PreviewRestore")).toHaveLength(0); }
        else { await input.getByRole("button", { name: "完整校验并预览", exact: true }).click(); await expect(input.getByRole("alert")).toContainText("BACKUP_INVALID"); await expect(input).toContainText("synthetic.prismbackup"); await expect(input.getByRole("button", { name: "完整校验并预览", exact: true })).toBeEnabled(); }
        expect(await calls(page, "Backup.ApplyRestore")).toHaveLength(0); expect((await fixture(page)).state).toEqual(before); await page.keyboard.press("Escape"); await expect(page.getByRole("dialog")).toHaveCount(0); await expect(page.getByRole("button", { name: "导入完整备份", exact: true })).toBeFocused();
      });
    }
    test("native backup exact off-page selection and unknown replay keep the original output", async ({ page }) => {
      await openNative(page, "backup-unknown", "backups", "synthetic-reference-1,synthetic-reference-12");
      await page.getByRole("button", { name: "创建完整备份", exact: true }).click(); const create = dialog(page, "创建完整本机备份");
      await expect(create.getByLabel("备份范围", { exact: true })).toHaveValue("selected");
      await create.getByRole("button", { name: "选择新备份文件位置", exact: true }).click(); await create.getByLabel(/允许正常关闭/).check(); await create.getByRole("button", { name: "正常关闭并导出", exact: true }).click();
      const result = dialog(page, "本机备份结果"); await expect(result).toBeVisible();
      const first = (await calls(page, "Backup.Export"))[0].payload as { environmentIds: string[]; scope: string };
      expect(first.environmentIds).toEqual(["synthetic-reference-1", "synthetic-reference-12"]); expect(first.scope).toBe("selected");
      await page.keyboard.press("Escape"); await page.getByRole("link", { name: "操作记录", exact: true }).click(); await page.getByRole("link", { name: "备份恢复", exact: true }).click();
      await page.getByRole("button", { name: "继续核实原导出", exact: true }).click(); await result.getByRole("button", { name: /核实原导出请求/ }).click();
      expect((await calls(page, "Backup.Export")).map(c => c.payload)).toEqual([first, first]); expect(await calls(page, "Backup.SelectDestination")).toHaveLength(1);
    });
    test("native unknown without operation ID opens usable original-request recovery", async ({ page }) => {
      await openNative(page, "backup-unknown-no-id"); await page.getByRole("button", { name: "创建完整备份", exact: true }).click(); const create = dialog(page, "创建完整本机备份"); await create.getByRole("button", { name: "选择新备份文件位置", exact: true }).click(); await create.getByLabel(/允许正常关闭/).check(); await create.getByRole("button", { name: "正常关闭并导出", exact: true }).click(); const result = dialog(page, "本机备份结果"); await expect(result).toBeVisible(); await expect(create).toHaveCount(0); const first = (await calls(page, "Backup.Export"))[0].payload; await result.getByRole("button", { name: /核实原导出请求/ }).click(); expect((await calls(page, "Backup.Export")).map(c => c.payload)).toEqual([first, first]); expect(await calls(page, "Backup.SelectDestination")).toHaveLength(1);
    });
    test("native all scope uses empty IDs, publishes only verified package, cancellation is exact", async ({ page }) => {
      await openNative(page, "baseline"); await page.getByRole("button", { name: "创建完整备份", exact: true }).click(); const create = dialog(page, "创建完整本机备份");
      await create.getByRole("button", { name: "选择新备份文件位置", exact: true }).click(); await create.getByLabel(/允许正常关闭/).check(); await create.getByRole("button", { name: "正常关闭并导出", exact: true }).click();
      const first = (await calls(page, "Backup.Export"))[0].payload as { scope: string; environmentIds: string[] }; expect(first).toMatchObject({ scope: "all", environmentIds: [] });
      const result = dialog(page, "本机备份结果"); await expect(result).not.toContainText("完整备份已发布"); await result.getByRole("button", { name: "取消此导出", exact: true }).click();
      await expect(result).toContainText("导出已取消"); expect(await calls(page, "Operation.Cancel")).toHaveLength(1); expect((await fixture(page)).state.environments).toHaveLength(12);
    });
    test("native publication appears only after exact terminal report; failed and pending cannot claim completion", async ({ page }) => {
      await openNative(page, "baseline"); await page.getByRole("button", { name: "创建完整备份", exact: true }).click(); const create = dialog(page, "创建完整本机备份");
      await create.getByRole("button", { name: "选择新备份文件位置", exact: true }).click(); await create.getByLabel(/允许正常关闭/).check(); await create.getByRole("button", { name: "正常关闭并导出", exact: true }).click(); const result = dialog(page, "本机备份结果"); await expect(result).toBeVisible(); await expect(result).not.toContainText("完整备份已发布");
      const id = Object.keys((await fixture(page)).operations)[0]; await page.evaluate(id => (window as unknown as { __localPagesFixture: Fixture }).__localPagesFixture.publish(id), id); await result.getByRole("button", { name: "重新读取此任务 ID", exact: true }).click(); await expect(result).toContainText("完整备份已发布"); await expect(result).toContainText("包 SHA-256"); await page.keyboard.press("Escape"); await expect(page.getByRole("button", { name: "创建完整备份", exact: true })).toBeFocused(); await expect(page.getByRole("row").filter({ hasText: "synthetic.prismbackup" })).toBeVisible(); expect(await calls(page, "Backup.Export")).toHaveLength(1);
    });
    for (const scenario of ["backup-failed", "backup-publication-pending"] as const) {
      test(`${scenario} shows an accurate unfinished result and never retries a new output automatically`, async ({ page }) => {
        await openNative(page, scenario); const before = (await fixture(page)).state; await page.getByRole("button", { name: "创建完整备份", exact: true }).click(); const create = dialog(page, "创建完整本机备份"); await create.getByRole("button", { name: "选择新备份文件位置", exact: true }).click(); await create.getByLabel(/允许正常关闭/).check(); await create.getByRole("button", { name: "正常关闭并导出", exact: true }).click(); const result = dialog(page, "本机备份结果"); await expect(result).toBeVisible(); await expect(result).not.toContainText("完整备份已发布"); await expect(result).toContainText(scenario === "backup-failed" ? "DISK_FULL" : "结果待保存 / 核实"); expect((await fixture(page)).state).toEqual(before); expect(await calls(page, "Backup.Export")).toHaveLength(1);
      });
    }
    for (const scenario of ["restore-rollback", "restore-protected", "restore-unknown", "restore-progress"] as const) {
      test(`${scenario} never claims successful restore; recovery targets original task`, async ({ page }) => {
        await openNative(page, scenario); const before = (await fixture(page)).state; await preflight(page); await confirmNative(page);
        const result = dialog(page, "完整恢复结果"); await expect(result).not.toContainText("完整新状态已提交并核对");
        expect((await fixture(page)).state).toEqual(before); const first = (await calls(page, "Backup.ApplyRestore"))[0].payload;
        if (scenario === "restore-rollback") { await expect(result).toContainText("恢复未完成，原状态已回滚"); await expect(result.getByRole("button", { name: "取消此恢复并回滚", exact: true })).toHaveCount(0); }
        if (scenario === "restore-protected") { await expect(result).toContainText("工作区保持维护保护"); await expect(result).toContainText("上次应用"); await result.getByRole("button", { name: "重试核对/收尾原恢复", exact: true }).click(); await expect(result).toContainText("恢复未完成，原状态已回滚"); expect(await calls(page, "Backup.RecoverRestore")).toHaveLength(1); }
        if (scenario === "restore-progress") { await result.getByRole("button", { name: "取消此恢复并回滚", exact: true }).click(); await expect(result).toContainText("恢复未完成，原状态已回滚"); expect(await calls(page, "Operation.Cancel")).toHaveLength(1); }
        if (scenario === "restore-unknown") { await page.keyboard.press("Escape"); await page.getByRole("link", { name: "操作记录", exact: true }).click(); await page.getByRole("link", { name: "备份恢复", exact: true }).click(); await page.getByRole("button", { name: "继续核实原恢复", exact: true }).click(); await result.getByRole("button", { name: "核实原恢复请求", exact: true }).click(); expect((await calls(page, "Backup.ApplyRestore")).map(c => c.payload)).toEqual([first, first]); }
      });
    }
    test("restore conflict and missing build block apply, original identity is not regenerated", async ({ page }) => {
      await openNative(page, "restore-conflict"); await preflight(page); const frame = dialog(page, "恢复前只读预检");
      await expect(frame.getByRole("button", { name: "下一步：确认恢复", exact: true })).toBeDisabled(); await frame.getByRole("button", { name: "精确内核", exact: true }).click(); await expect(frame).toContainText("缺少同版本同摘要内核");
      expect(await calls(page, "Backup.ApplyRestore")).toHaveLength(0); expect(await calls(page, "Fingerprint.Generate")).toHaveLength(0);
    });
    test("native restore success requires the matched full commit report, not preflight or accepted progress", async ({ page }) => {
      await openNative(page, "restore-success"); const before = (await fixture(page)).state; await preflight(page); await confirmNative(page); const result = dialog(page, "完整恢复结果"); await expect(result).toContainText("完整新状态已提交并核对"); await expect(result).toContainText("已切换：2"); expect(await calls(page, "Backup.ApplyRestore")).toHaveLength(1); expect((await fixture(page)).state.environments.map(e => [e.id, e.seed])).toEqual(before.environments.map(e => [e.id, e.seed])); expect(await calls(page, "Fingerprint.Generate")).toHaveLength(0); await page.keyboard.press("Escape"); await expect(page.getByRole("button", { name: "导入完整备份", exact: true })).toBeFocused();
    });
    test("diagnostics whitelist, cancellation or unknown route-return retains exact report", async ({ page }) => {
      await openNative(page, "diagnostics-unknown", "activity"); await page.getByRole("button", { name: "生成诊断预览", exact: true }).click(); const preview = dialog(page, "脱敏诊断预览");
      await expect(preview).toContainText("部分可读取"); await expect(preview).toContainText("本次未检查"); await expect(preview).not.toContainText("工作环境 A"); await expect(preview.locator("pre")).toHaveCount(0);
      await preview.getByRole("button", { name: "保存这份诊断 JSON", exact: true }).click(); const result = dialog(page, "诊断导出结果"); await expect(result).toContainText("原保存请求待核实");
      const original = (await calls(page, "Diagnostics.Export"))[0].payload;
      await page.keyboard.press("Escape"); await page.getByRole("link", { name: "备份恢复", exact: true }).click(); await page.getByRole("link", { name: "操作记录", exact: true }).click(); await page.getByRole("button", { name: "继续核实原诊断导出", exact: true }).click(); await result.getByRole("button", { name: "核实原导出", exact: true }).click();
      expect((await calls(page, "Diagnostics.Export")).map(c => c.payload)).toEqual([original, original]); expect(await calls(page, "Diagnostics.Preview")).toHaveLength(1);
      await result.getByRole("button", { name: /结束此次核实/ }).click(); const end = dialog(page, "结束诊断导出核实"); await expect(end).toContainText("不会删除或撤回"); await end.getByRole("button", { name: "结束此次核实", exact: true }).click(); await expect(result).toContainText("旧文件可能已发布");
    });
    for (const scenario of ["diagnostics-cancel", "diagnostics-unavailable"] as const) {
      test(`${scenario} retains sanitized report and only accepts the matching save receipt`, async ({ page }) => {
        await openNative(page, scenario, "activity"); await page.getByRole("button", { name: "生成诊断预览", exact: true }).click(); const preview = dialog(page, "脱敏诊断预览"); await expect(preview).toContainText(scenario === "diagnostics-cancel" ? "部分可读取" : "不可读取（仅最小报告）"); await expect(preview).not.toContainText("工作环境 A"); await preview.getByRole("button", { name: "保存这份诊断 JSON", exact: true }).click(); const result = dialog(page, "诊断导出结果"); await expect(result).toContainText(scenario === "diagnostics-cancel" ? "已取消保存；未发布" : "诊断文件已保存，SHA-256 与预览一致"); await expect(result.getByRole("button", { name: "核实原导出", exact: true })).toHaveCount(0); expect(await calls(page, "Diagnostics.Export")).toHaveLength(1); await page.keyboard.press("Escape"); await expect(page.getByRole("button", { name: "生成诊断预览", exact: true })).toBeFocused();
      });
    }
    test("activity detail is reachable and demo export uses only original callback", async ({ page }) => {
      await openDemo(page, "activity"); await expect(page.getByRole("columnheader", { name: "操作对象", exact: true })).toBeVisible(); await page.getByRole("button", { name: /创建合成环境 工作环境 A 详情/ }).click();
      await expect(dialog(page, "操作记录详情")).toContainText("非真实桌面执行"); await page.keyboard.press("Escape"); await expect(page.getByRole("button", { name: /创建合成环境 工作环境 A 详情/ })).toBeFocused();
      await page.getByRole("button", { name: "导出记录", exact: true }).click(); const download = await page.evaluate(() => (window as unknown as { __localPagesHarness: { downloads: { name: string }[] } }).__localPagesHarness.downloads); expect(download).toHaveLength(1); expect(download[0].name).toBe("prism-activity.json");
    });
    test("activity direct-reference density survives global styles; filters and pagination preserve records", async ({ page }) => {
      await openDemo(page, "activity", false, 12); const heading = page.getByRole("columnheader", { name: "操作对象", exact: true }); const rect = (await heading.boundingBox())!; expect(rect.x).toBe(220); expect(Math.abs(rect.y - 164.578)).toBeLessThan(1); expect(rect.height).toBe(40);
      expect((await page.locator("tbody tr").first().boundingBox())!.height).toBe(48); expect(await page.locator(".local-page-table").evaluate(e => getComputedStyle(e).borderCollapse)).toBe("separate"); await expect(page.getByRole("button", { name: "下一页", exact: true })).toBeInViewport();
      await page.getByRole("button", { name: "下一页", exact: true }).click(); await expect(page.getByRole("button", { name: "创建合成环境 工作环境 A 12 详情", exact: true })).toBeVisible(); await page.getByLabel("搜索操作记录", { exact: true }).fill("不存在合成对象"); await expect(page.getByRole("row").filter({ hasText: "没有匹配" })).toBeVisible(); await page.getByRole("button", { name: "重置", exact: true }).click(); await expect(page.getByRole("button", { name: "创建合成环境 工作环境 A 详情", exact: true })).toBeVisible(); expect((await stored(page)).activities).toHaveLength(12);
    });
    test("activity actions require current matching session and honor blocked IDs", async ({ page }) => {
      await openNative(page, "activity-sessions", "activity");
      await page.getByRole("button", { name: "旧合成会话 工作环境 A 详情", exact: true }).click(); await expect(dialog(page, "操作记录详情").getByRole("button", { name: "强制结束此会话", exact: true })).toHaveCount(0); await page.keyboard.press("Escape");
      await page.getByRole("button", { name: "当前受控合成会话 工作环境 A 详情", exact: true }).click(); await expect(dialog(page, "操作记录详情")).toContainText("SYNTHETIC_SESSION_ERROR"); await dialog(page, "操作记录详情").getByRole("button", { name: "强制结束此会话", exact: true }).click(); await page.keyboard.press("Escape");
      await page.getByRole("button", { name: "待核对合成会话 工作环境 B 详情", exact: true }).click(); await dialog(page, "操作记录详情").getByRole("button", { name: "核对会话", exact: true }).click();
      const actions = await page.evaluate(() => (window as unknown as { __localPagesHarness: { sessionActions: unknown[] } }).__localPagesHarness.sessionActions); expect(actions).toEqual([{ environmentId: "synthetic-reference-1", sessionId: "synthetic-current-session-1", action: "force" }, { environmentId: "synthetic-reference-2", sessionId: "synthetic-current-session-2", action: "reconcile" }]);
      await page.goto(`${harness}?blocked=synthetic-reference-1#/activity`); await page.getByRole("button", { name: "当前受控合成会话 工作环境 A 详情", exact: true }).click(); await expect(dialog(page, "操作记录详情").getByRole("button", { name: "强制结束此会话", exact: true })).toBeDisabled();
    });
    test("help preserves four embedded docs, navigation, markdown download and scroll reset", async ({ page }) => {
      await openDemo(page, "guide"); const tabs = page.getByRole("tablist", { name: "嵌入文档", exact: true }); await expect(tabs.getByRole("tab")).toHaveCount(4);
      await expect(page.getByRole("tabpanel")).toContainText("本地指纹浏览器产品需求"); await page.getByRole("tabpanel").evaluate(e => { e.scrollTop = 500; }); await tabs.getByRole("tab", { name: "内核适配合同", exact: true }).click(); await expect.poll(() => page.getByRole("tabpanel").evaluate(e => e.scrollTop)).toBe(0);
      await page.getByRole("button", { name: "下载文档", exact: true }).click(); const downloads = await page.evaluate(() => (window as unknown as { __localPagesHarness: { downloads: { name: string; content: string; type: string }[] } }).__localPagesHarness.downloads); expect(downloads[0].name).toBe("KERNEL.md"); expect(downloads[0].type).toContain("text/markdown"); expect(downloads[0].content).toContain("fingerprint-chromium");
      await page.getByRole("button", { name: "备份与恢复 BKP-001 / DATA-001", exact: true }).click(); await expect(page.getByRole("button", { name: "创建快照", exact: true })).toBeVisible();
    });
  });
}

test("narrow windows keep pinned footer reachable and return to accessible navigation", async ({ page }) => {
  await page.setViewportSize({ width: 820, height: 600 }); await openNative(page, "baseline"); await preflight(page); const frame = dialog(page, "恢复前只读预检");
  const next = frame.getByRole("button", { name: "下一步：确认恢复", exact: true }); await expect(next).toBeInViewport(); await next.click(); await expect(dialog(page, "确认完整恢复").getByRole("button", { name: "关闭确认完整恢复", exact: true })).toBeFocused(); await page.keyboard.press("Escape"); await expect(frame).toBeVisible(); await page.keyboard.press("Escape"); await expect(page.getByRole("dialog")).toHaveCount(0);
  await page.getByRole("button", { name: "查看操作记录", exact: true }).click(); await page.getByRole("button", { name: /创建合成环境 工作环境 A 详情/ }).click(); await expect(dialog(page, "操作记录详情").getByRole("button", { name: "强制结束此会话", exact: true })).toHaveCount(0);
});

test("actual App workspace fault outranks an open restore result and nested diagnostics return to the blocker", async ({ page }) => {
  await localPagesNativeBridge(page, "restore-progress"); await page.goto("/#/backups"); await expect(page.getByRole("button", { name: "导入完整备份", exact: true })).toBeEnabled(); await preflight(page); await confirmNative(page);
  const result = page.getByRole("dialog", { name: "完整恢复结果", exact: true, includeHidden: true });
  await page.evaluate(() => { (window as unknown as { __localPagesFixture: Fixture }).__localPagesFixture.workspace.issue = { code: "NATIVE_UNAVAILABLE", message: "合成工作区故障；原恢复窗口必须保留。", retryable: true }; });
  const blocker = page.getByRole("alertdialog", { name: "工作区需要处理", exact: true, includeHidden: true }); await expect(blocker).toBeVisible(); await expect.poll(() => blocker.evaluate(e => e.contains(document.activeElement))).toBe(true);
  for (const key of ["Escape", "Control+k", "Tab", "Shift+Tab"]) { await page.keyboard.press(key); await expect(blocker).toBeVisible(); await expect(result).toBeAttached(); await expect.poll(() => blocker.evaluate(e => e.contains(document.activeElement))).toBe(true); }
  await blocker.getByRole("button", { name: "生成诊断预览", exact: true }).click(); const preview = dialog(page, "脱敏诊断预览"); await expect(preview).toContainText("公开字段摘要"); await expect.poll(() => preview.evaluate(e => e.contains(document.activeElement))).toBe(true); await page.keyboard.press("Escape"); await expect(preview).toHaveCount(0); await expect(blocker).toBeVisible(); await expect.poll(() => blocker.evaluate(e => e.contains(document.activeElement))).toBe(true); await expect(result).toBeAttached(); expect(await calls(page, "Backup.ApplyRestore")).toHaveLength(1);
  await page.evaluate(() => { (window as unknown as { __localPagesFixture: Fixture }).__localPagesFixture.workspace.issue = undefined; }); await expect(blocker).toHaveCount(0); await expect.poll(() => result.evaluate(e => e.contains(document.activeElement))).toBe(true); await expect(page.locator(".sidebar")).toHaveJSProperty("inert", true); await expect(page.locator(".main-shell")).toHaveJSProperty("inert", true); await page.keyboard.press("Escape"); await expect(result).toHaveCount(0); await expect(page.locator(".sidebar")).toHaveJSProperty("inert", false); await expect(page.locator(".main-shell")).toHaveJSProperty("inert", false); expect(await page.evaluate(() => document.body.style.overflow)).toBe(""); await page.getByRole("button", { name: "查看操作记录", exact: true }).click(); await expect(page).toHaveURL(/#\/activity$/);
});
