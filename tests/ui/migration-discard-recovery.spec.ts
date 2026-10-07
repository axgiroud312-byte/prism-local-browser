import { test, expect, type Page } from "@playwright/test";
import { migrationDiscardRecoveryBridge, migrationDiscardRecoveryEntry } from "./fixtures/migration-discard-recovery";
import { migrationWorkflowIds as ids } from "./fixtures/migration-workflow-bridge";

const dialog = (page: Page, name: string) => page.getByRole("dialog", { name, exact: true });
const snapshot = (page: Page) => page.evaluate(() => window.__migrationDiscard.snapshot());
const calls = async (page: Page, method: string) => (await snapshot(page)).calls.filter(c => c.method === method);
const errors = new WeakMap<Page, string[]>();

test.beforeEach(async ({ page }) => {
  errors.set(page, []); page.on("pageerror", error => errors.get(page)!.push(error.message));
  await page.route("**/*", route => new URL(route.request().url()).hostname === "127.0.0.1" ? route.continue() : route.abort());
});
test.afterEach(async ({ page }, info) => {
  const trace = await snapshot(page).catch(() => undefined);
  if (trace) {
    await info.attach("M22-synthetic-original-cleanup-trace", { body: JSON.stringify(trace, null, 2), contentType: "application/json" });
    expect(await page.evaluate(() => window.__migrationWorkflow.snapshot().faults)).toEqual([]);
    for (const method of ["Runtime.Start", "Runtime.ForceStop", "Kernel.Install", "Fingerprint.Generate", "Preview.Regenerate", "Environment.Create", "Environment.Update", "Migration.Prepare"]) {
      expect(trace.calls.filter(c => c.method === method), `${method} is never a cleanup fallback`).toHaveLength(0);
    }
    // Every existing case still requires zero ordinary-source calls. Only the
    // new annotated scenario explicitly selects one supported source by UI.
    expect(trace.calls.filter(c => c.method === "Backup.SelectRestoreSource")).toHaveLength(info.annotations.some(a => a.type === "M22-explicit-ordinary-source") ? 1 : 0);
  }
  expect(errors.get(page), "no unhandled rejection on cancellation/hide/unmount").toEqual([]);
});

async function open(page: Page, scenario: "rollback-failed" | "rollback-hash-mismatch" = "rollback-failed") {
  await migrationDiscardRecoveryBridge(page, scenario); await page.goto("/#/kernels");
  await page.getByRole("button", { name: "选定环境迁移", exact: true }).click();
  const selection = dialog(page, "选定环境内核迁移");
  await expect(selection.getByText("第 1 页 · 共 2 个")).toBeVisible();
  await selection.getByText("迁移记录与升级前恢复", { exact: true }).click();
  return selection;
}
async function leaveAndRemount(page: Page) {
  await page.getByRole("link", { name: "操作记录", exact: true }).click();
  await page.getByRole("link", { name: "内核管理", exact: true }).click();
  await expect(page.getByRole("button", { name: "查看原预检清理", exact: true })).toBeVisible();
  await page.getByRole("button", { name: "查看原预检清理", exact: true }).click();
  await expect(dialog(page, "升级前预检清理待核实")).toBeVisible();
}
async function verifyBlocked(page: Page, selects = 1, previews = 1, restores = 0) {
  expect(await calls(page, "Migration.SelectRollback")).toHaveLength(selects);
  expect(await calls(page, "Backup.PreviewRestore")).toHaveLength(previews);
  expect(await calls(page, "Backup.ApplyRestore")).toHaveLength(restores);
  await expect(page.getByRole("button", { name: "确认完整恢复", exact: true })).toHaveCount(0);
  await expect(dialog(page, "升级前预检清理待核实")).not.toContainText("清理成功");
  await expect(dialog(page, "升级前预检清理待核实")).not.toContainText("synthetic-upgrade-source");
}

for (const viewport of [{ width: 1440, height: 900 }, { width: 1280, height: 800 }]) {
  test(`M22 definite SelectRollback PROFILE_BUSY allows only later explicit ordinary source and rollback ${viewport.width}`, async ({ page }, info) => {
    info.annotations.push({ type: "M22-explicit-ordinary-source", description: "one user-selected supported source after definite resource-free refusal; never a fallback" });
    await page.setViewportSize(viewport); const selection = await open(page);
    const before = await page.evaluate(() => window.__migrationWorkflow.snapshot().workspace);
    await page.evaluate(() => { window.__migrationDiscard.setSelectionBusy(true); window.__migrationDiscard.enableOrdinarySource(); });
    await selection.getByRole("button", { name: "预检升级前完整恢复", exact: true }).click();
    await expect(selection.getByRole("alert")).toContainText("PROFILE_BUSY");
    await expect(dialog(page, "升级前预检清理待核实")).toHaveCount(0);
    await expect(selection.getByRole("button", { name: "预检升级前完整恢复", exact: true })).toBeEnabled();
    expect(await page.evaluate(() => window.__migrationWorkflow.snapshot().sourceActive)).toBe(false);
    expect(await calls(page, "Backup.DiscardRestore")).toHaveLength(0); expect(await calls(page, "Backup.PreviewRestore")).toHaveLength(0);
    await page.evaluate(() => window.__migrationDiscard.setSelectionBusy(false));
    expect(await calls(page, "Migration.SelectRollback")).toHaveLength(1); expect(await calls(page, "Backup.SelectRestoreSource")).toHaveLength(0);
    await selection.getByRole("button", { name: "关闭", exact: true }).click();
    await page.getByRole("link", { name: "备份与恢复", exact: true }).click();
    await expect(page.getByRole("button", { name: "查看原预检清理", exact: true })).toHaveCount(0);
    await page.getByRole("button", { name: "导入完整备份", exact: true }).click();
    const ordinary = dialog(page, "导入本机备份");
    await ordinary.getByRole("button", { name: "选择本机备份包", exact: true }).click();
    await expect(ordinary).toContainText("synthetic-ordinary.prismbackup");
    await expect(ordinary.getByRole("button", { name: "完整校验并预览", exact: true })).toBeEnabled();
    const ordinarySourceRequestId = "00000000-0000-4000-8000-000000000001";
    expect(await calls(page, "Backup.SelectRestoreSource")).toEqual([{ mode: "native", method: "Backup.SelectRestoreSource", payload: { requestId: ordinarySourceRequestId } }]);
    expect(await calls(page, "Backup.PreviewRestore")).toHaveLength(0); expect(await calls(page, "Backup.ApplyRestore")).toHaveLength(0);
    await ordinary.getByRole("button", { name: "取消", exact: true }).click(); await expect(ordinary).toHaveCount(0);
    expect(await calls(page, "Backup.DiscardRestore")).toEqual([{ mode: "native", method: "Backup.DiscardRestore", payload: { requestId: ordinarySourceRequestId, previewId: "", sourceToken: "synthetic-ordinary-supported-source" } }]);
    expect((await snapshot(page)).ordinarySourceActive).toBe(false);
    await page.getByRole("link", { name: "内核管理", exact: true }).click();
    await page.getByRole("button", { name: "选定环境迁移", exact: true }).click();
    await selection.getByText("迁移记录与升级前恢复", { exact: true }).click();
    await selection.getByRole("button", { name: "预检升级前完整恢复", exact: true }).click();
    await expect(selection).toContainText("升级前完整恢复预检");
    const selections = await calls(page, "Migration.SelectRollback"); expect(selections).toHaveLength(2); expect(selections[0]).toEqual(selections[1]);
    expect((await calls(page, "Backup.PreviewRestore"))[0].payload).toEqual({ sourceToken: "synthetic-upgrade-source" });
    expect(await calls(page, "Backup.ApplyRestore")).toHaveLength(0);
    expect(await page.evaluate(() => window.__migrationWorkflow.snapshot().workspace)).toEqual(before);
    await expect(dialog(page, "升级前预检清理待核实")).toHaveCount(0);
  });

  for (const reply of ["unknown", "invalid", "demo", "refused", "throw"] as const) {
    test(`M22 ${reply} cleanup only retries original owner across hide and route remount ${viewport.width}`, async ({ page, baseURL }) => {
      await page.setViewportSize(viewport);
      await migrationDiscardRecoveryEntry(page, reply, `${baseURL}/#/kernels`);
      const selection = dialog(page, "选定环境内核迁移"), recovery = dialog(page, "升级前预检清理待核实");
      const before = await page.evaluate(() => window.__migrationWorkflow.snapshot().workspace);
      expect(Math.round((await recovery.boundingBox())!.width)).toBe(400);
      await expect(recovery).toContainText("synthetic-migration-task-1");
      await expect(recovery).toContainText("synthetic-upgrade-restore-preview");
      await expect(recovery.getByRole("button", { name: "重试清理原预检", exact: true })).toBeEnabled();
      await expect(selection.getByLabel("选定环境", { exact: true })).toBeDisabled();
      await expect(selection.getByRole("button", { name: "预检升级前完整恢复", exact: true })).toBeDisabled();
      await verifyBlocked(page);
      await recovery.getByRole("button", { name: "仅隐藏，保留待核实", exact: true }).click();
      await selection.getByRole("button", { name: "关闭", exact: true }).click();
      await leaveAndRemount(page); expect(await calls(page, "Backup.DiscardRestore")).toHaveLength(1);
      await page.evaluate(() => window.__migrationDiscard.hold("Backup.DiscardRestore", "synthetic-retry-cleanup"));
      // Same-tick double activation must be guarded even before React commits.
      await recovery.getByRole("button", { name: "重试清理原预检", exact: true }).evaluate(button => { (button as HTMLButtonElement).click(); (button as HTMLButtonElement).click(); });
      await expect.poll(async () => (await snapshot(page)).held.length).toBe(1);
      await expect(recovery.getByRole("button", { name: "重试清理原预检", exact: true })).toBeDisabled();
      await verifyBlocked(page); expect(await calls(page, "Backup.DiscardRestore")).toHaveLength(2);
      await recovery.getByRole("button", { name: "仅隐藏，保留待核实", exact: true }).click();
      await leaveAndRemount(page);
      await expect(recovery.getByRole("button", { name: "重试清理原预检", exact: true })).toBeDisabled();
      await page.evaluate(() => window.__migrationDiscard.reply("synthetic-retry-cleanup"));
      await expect(recovery).toHaveCount(0);
      const discards = await calls(page, "Backup.DiscardRestore"); expect(discards[1]).toEqual(discards[0]);
      expect(discards[0].payload).toEqual({ previewId: "synthetic-upgrade-restore-preview", sourceToken: "synthetic-upgrade-source" });
      expect(await page.evaluate(() => window.__migrationWorkflow.snapshot().workspace)).toEqual(before);
      await page.evaluate(() => window.__migrationDiscard.setDiscardReply("known"));
      await page.getByRole("button", { name: "选定环境迁移", exact: true }).click();
      await selection.getByText("迁移记录与升级前恢复", { exact: true }).click();
      await selection.getByRole("button", { name: "预检升级前完整恢复", exact: true }).click();
      await expect(selection).toContainText("升级前完整恢复预检");
      expect(await calls(page, "Migration.SelectRollback")).toHaveLength(2); expect(await calls(page, "Backup.PreviewRestore")).toHaveLength(2);
      expect(await calls(page, "Backup.ApplyRestore")).toHaveLength(0);
    });
  }

  for (const method of ["Migration.SelectRollback", "Backup.PreviewRestore"] as const) {
    test(`M22 cancellation waits for late ${method} and original cleanup ${viewport.width}`, async ({ page }) => {
      await page.setViewportSize(viewport); const selection = await open(page);
      await page.evaluate(method => { window.__migrationDiscard.setDiscardReply("unknown"); window.__migrationDiscard.hold(method, "synthetic-late-preflight"); }, method);
      await selection.getByRole("button", { name: "预检升级前完整恢复", exact: true }).click();
      await expect.poll(async () => (await snapshot(page)).held.length).toBe(1);
      await selection.getByRole("button", { name: "关闭", exact: true }).click();
      await leaveAndRemount(page);
      const recovery = dialog(page, "升级前预检清理待核实");
      await expect(recovery.getByRole("button", { name: "重试清理原预检", exact: true })).toBeDisabled();
      expect(await calls(page, "Backup.DiscardRestore")).toHaveLength(0);
      await page.evaluate(() => window.__migrationDiscard.reply("synthetic-late-preflight"));
      await expect(recovery.getByRole("button", { name: "重试清理原预检", exact: true })).toBeEnabled();
      await verifyBlocked(page, 1, method === "Migration.SelectRollback" ? 0 : 1);
      expect(await calls(page, "Backup.DiscardRestore")).toHaveLength(1);
      await page.evaluate(() => window.__migrationDiscard.hold("Backup.DiscardRestore", "synthetic-late-cleanup"));
      await recovery.getByRole("button", { name: "重试清理原预检", exact: true }).click();
      await expect(recovery.getByRole("button", { name: "重试清理原预检", exact: true })).toBeDisabled();
      await page.evaluate(() => window.__migrationDiscard.reply("synthetic-late-cleanup"));
      await expect(recovery).toHaveCount(0);
      const discards = await calls(page, "Backup.DiscardRestore"); expect(discards[0]).toEqual(discards[1]);
      expect(discards[0].payload).toEqual({ previewId: method === "Migration.SelectRollback" ? "" : "synthetic-upgrade-restore-preview", sourceToken: "synthetic-upgrade-source" });
      expect(await calls(page, "Backup.ApplyRestore")).toHaveLength(0);
    });
  }

  test(`M22 submitted unknown restore keeps exact request and never becomes cleanup ${viewport.width}`, async ({ page }) => {
    await page.setViewportSize(viewport); const selection = await open(page);
    await selection.getByRole("button", { name: "预检升级前完整恢复", exact: true }).click();
    await expect(selection).toContainText("升级前完整恢复预检");
    await page.evaluate(() => window.__migrationDiscard.hold("Backup.ApplyRestore", "synthetic-original-restore"));
    await selection.getByRole("button", { name: "确认完整恢复", exact: true }).click();
    const confirmation = dialog(page, "确认完整恢复");
    for (const checkbox of await confirmation.getByRole("checkbox").all()) await checkbox.check();
    await confirmation.getByRole("button", { name: "确认并完整恢复", exact: true }).click();
    await expect.poll(async () => (await snapshot(page)).held.length).toBe(1);
    expect(await calls(page, "Backup.DiscardRestore")).toHaveLength(0);
    await page.evaluate(() => window.__migrationDiscard.reply("synthetic-original-restore", "unknown"));
    await expect(dialog(page, "完整恢复结果")).toContainText("原受理尚未核实");
    await page.keyboard.press("Escape"); await selection.getByRole("button", { name: "关闭", exact: true }).click();
    await page.getByRole("link", { name: "操作记录", exact: true }).click(); await page.getByRole("link", { name: "内核管理", exact: true }).click();
    await expect(page.getByRole("button", { name: "查看原预检清理", exact: true })).toHaveCount(0);
    await page.getByRole("button", { name: "选定环境迁移", exact: true }).click();
    await expect(selection.getByLabel("选定环境", { exact: true })).toBeDisabled();
    await selection.getByRole("button", { name: "继续核实原恢复", exact: true }).click();
    await dialog(page, "完整恢复结果").getByRole("button", { name: "核实原恢复请求", exact: true }).click();
    await expect(dialog(page, "完整恢复结果")).toContainText("恢复未完成，原状态已回滚");
    const requests = await calls(page, "Backup.ApplyRestore"); expect(requests).toHaveLength(2); expect(requests[1]).toEqual(requests[0]);
    expect(requests[0].payload).toEqual({ previewId: "synthetic-upgrade-restore-preview", archiveSha256: ids.archiveSha256, confirmOverwrite: true, acknowledgeCredentials: true, stopRunning: true, requestId: "00000000-0000-4000-8000-000000000001" });
    expect(await calls(page, "Backup.DiscardRestore")).toHaveLength(0); expect(await calls(page, "Migration.SelectRollback")).toHaveLength(1);
  });

  test(`M22 mismatched selection finally retains original cleanup and has no restore preflight ${viewport.width}`, async ({ page }) => {
    await page.setViewportSize(viewport); const selection = await open(page, "rollback-hash-mismatch");
    await page.evaluate(() => window.__migrationDiscard.setDiscardReply("unknown"));
    await selection.getByRole("button", { name: "预检升级前完整恢复", exact: true }).click();
    const recovery = dialog(page, "升级前预检清理待核实"); await expect(recovery).toBeVisible();
    await verifyBlocked(page, 1, 0);
    await page.evaluate(() => window.__migrationDiscard.setDiscardReply("known"));
    await recovery.getByRole("button", { name: "重试清理原预检", exact: true }).click();
    await expect(recovery).toHaveCount(0);
    const discards = await calls(page, "Backup.DiscardRestore"); expect(discards[0]).toEqual(discards[1]);
    expect(discards[0].payload).toEqual({ previewId: "", sourceToken: "synthetic-upgrade-source" });
    expect(await calls(page, "Backup.PreviewRestore")).toHaveLength(0); expect(await calls(page, "Backup.ApplyRestore")).toHaveLength(0);
  });

  test(`M22 tokenless selection source-limit remains honest and blocked across retry and remount ${viewport.width}`, async ({ page }) => {
    await page.setViewportSize(viewport); const selection = await open(page);
    await page.evaluate(() => window.__migrationDiscard.hold("Migration.SelectRollback", "synthetic-tokenless-selection"));
    await selection.getByRole("button", { name: "预检升级前完整恢复", exact: true }).click();
    await expect.poll(async () => (await snapshot(page)).held.length).toBe(1);
    await page.evaluate(() => window.__migrationDiscard.reply("synthetic-tokenless-selection", "unknown"));
    const recovery = dialog(page, "升级前预检清理待核实"); await expect(recovery).toBeVisible();
    await expect(recovery).toContainText("若原来源标识未返回"); await expect(recovery).toContainText("本次会话没有安全恢复入口"); await verifyBlocked(page, 1, 0);
    await recovery.getByRole("button", { name: "重试清理原预检", exact: true }).click();
    await expect(recovery).toBeVisible(); expect(await calls(page, "Backup.DiscardRestore")).toHaveLength(0);
    await recovery.getByRole("button", { name: "仅隐藏，保留待核实", exact: true }).click();
    await selection.getByRole("button", { name: "关闭", exact: true }).click();
    await leaveAndRemount(page); await verifyBlocked(page, 1, 0);
    await expect(recovery).not.toContainText("synthetic-upgrade-restore-preview");
    expect(await calls(page, "Backup.DiscardRestore")).toHaveLength(0);
  });

  test(`M22 unmount cleanup transport throw keeps recovery without an unhandled rejection ${viewport.width}`, async ({ page }) => {
    await page.setViewportSize(viewport); const selection = await open(page);
    await selection.getByRole("button", { name: "预检升级前完整恢复", exact: true }).click();
    await expect(selection).toContainText("升级前完整恢复预检");
    await page.evaluate(() => window.__migrationDiscard.setDiscardReply("throw"));
    await selection.getByRole("button", { name: "关闭", exact: true }).click();
    await leaveAndRemount(page); const recovery = dialog(page, "升级前预检清理待核实");
    await verifyBlocked(page); expect(await calls(page, "Backup.DiscardRestore")).toHaveLength(1);
    await page.evaluate(() => window.__migrationDiscard.setDiscardReply("known"));
    await recovery.getByRole("button", { name: "重试清理原预检", exact: true }).click(); await expect(recovery).toHaveCount(0);
    const discards = await calls(page, "Backup.DiscardRestore"); expect(discards[0]).toEqual(discards[1]);
  });

  test(`M22 definite restore refusal resumes only original unknown cleanup ${viewport.width}`, async ({ page }) => {
    await page.setViewportSize(viewport); const selection = await open(page);
    await selection.getByRole("button", { name: "预检升级前完整恢复", exact: true }).click();
    await expect(selection).toContainText("升级前完整恢复预检");
    await page.evaluate(() => window.__migrationDiscard.hold("Backup.ApplyRestore", "synthetic-abandoned-restore"));
    await selection.getByRole("button", { name: "确认完整恢复", exact: true }).click();
    const confirmation = dialog(page, "确认完整恢复");
    for (const checkbox of await confirmation.getByRole("checkbox").all()) await checkbox.check();
    await confirmation.getByRole("button", { name: "确认并完整恢复", exact: true }).click();
    await expect.poll(async () => (await snapshot(page)).held.length).toBe(1);
    await page.evaluate(() => window.__migrationDiscard.reply("synthetic-abandoned-restore", "unknown"));
    await expect(dialog(page, "完整恢复结果")).toContainText("原受理尚未核实");
    await page.keyboard.press("Escape"); await selection.getByRole("button", { name: "关闭", exact: true }).click();
    await page.getByRole("link", { name: "操作记录", exact: true }).click(); await page.getByRole("link", { name: "内核管理", exact: true }).click();
    await page.getByRole("button", { name: "选定环境迁移", exact: true }).click();
    await page.evaluate(() => { window.__migrationDiscard.setDiscardReply("unknown"); window.__migrationDiscard.hold("Backup.ApplyRestore", "synthetic-refused-restore"); });
    await selection.getByRole("button", { name: "继续核实原恢复", exact: true }).click();
    await dialog(page, "完整恢复结果").getByRole("button", { name: "核实原恢复请求", exact: true }).click();
    await expect.poll(async () => (await snapshot(page)).held.length).toBe(1);
    expect(await calls(page, "Backup.DiscardRestore")).toHaveLength(0);
    await page.evaluate(() => window.__migrationDiscard.reply("synthetic-refused-restore", "refused"));
    await expect(dialog(page, "完整恢复结果")).toContainText("RESTORE_NOT_ACCEPTED");
    await page.keyboard.press("Escape"); await selection.getByRole("button", { name: "查看原预检清理", exact: true }).click();
    await verifyBlocked(page, 1, 1, 2); expect(await calls(page, "Backup.DiscardRestore")).toHaveLength(1);
    await page.evaluate(() => window.__migrationDiscard.setDiscardReply("known"));
    const recovery = dialog(page, "升级前预检清理待核实");
    await recovery.getByRole("button", { name: "重试清理原预检", exact: true }).click(); await expect(recovery).toHaveCount(0);
    const discards = await calls(page, "Backup.DiscardRestore"), requests = await calls(page, "Backup.ApplyRestore");
    expect(discards[0]).toEqual(discards[1]); expect(requests[0]).toEqual(requests[1]);
    expect(await calls(page, "Migration.SelectRollback")).toHaveLength(1); expect(await calls(page, "Backup.PreviewRestore")).toHaveLength(1);
  });
}
