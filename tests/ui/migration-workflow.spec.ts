import { test, expect, type Page } from "@playwright/test";
import { mkdir, writeFile } from "node:fs/promises";
import { join } from "node:path";
import { migrationWorkflowBridge, migrationWorkflowIds as ids, type MigrationWorkflowScenario } from "./fixtures/migration-workflow-bridge";

const dialog = (page: Page, name: string) => page.getByRole("dialog", { name, exact: true });
const snapshot = (page: Page) => page.evaluate(() => window.__migrationWorkflow.snapshot());
const calls = async (page: Page, method: string) => (await snapshot(page)).calls.filter(c => c.method === method);
const commits = async (page: Page) => (await calls(page, "Migration.Action")).filter(c => (c.payload as { action: string }).action === "commit");

test.beforeEach(async ({ page }) => {
  await page.route("**/*", route => new URL(route.request().url()).hostname === "127.0.0.1" ? route.continue() : route.abort());
});
test.afterEach(async ({ page }, info) => {
  const evidence = await snapshot(page).catch(() => undefined);
  if (!evidence) return;
  await info.attach("synthetic-migration-request-trace", { body: JSON.stringify(evidence, null, 2), contentType: "application/json" });
  if (process.env.PRISM_MIGRATION_EVIDENCE_DIR) {
    await mkdir(process.env.PRISM_MIGRATION_EVIDENCE_DIR, { recursive: true });
    await writeFile(join(process.env.PRISM_MIGRATION_EVIDENCE_DIR, `${info.title.replace(/[^a-zA-Z0-9-]/g, "-")}.json`), JSON.stringify({ title: info.title, status: info.status, viewport: page.viewportSize(), ...evidence }, null, 2));
  }
  const expectedFaults = info.annotations.filter(a => a.type === "synthetic-unsupported").map(a => `Unsupported synthetic method: ${a.description}`);
  expect(evidence.faults, "every supported synthetic boundary method is explicit").toEqual(expectedFaults);
  for (const method of ["Runtime.Start", "Runtime.ForceStop", "Kernel.Install", "Fingerprint.Generate", "Environment.Update", "Backup.SelectRestoreSource"]) {
    expect(evidence.calls.filter(c => c.method === method), `${method} must not be a migration fallback`).toHaveLength(0);
  }
});

async function open(page: Page, scenario: MigrationWorkflowScenario = "baseline") {
  await migrationWorkflowBridge(page, scenario);
  await page.goto("/#/kernels");
  await page.getByRole("button", { name: "选定环境迁移", exact: true }).click();
  await expect(dialog(page, "选定环境内核迁移")).toBeVisible();
  await expect(dialog(page, "选定环境内核迁移").getByText("第 1 页 · 共 2 个")).toBeVisible();
}
async function preview(page: Page) {
  const selection = dialog(page, "选定环境内核迁移");
  await selection.getByLabel("选定环境", { exact: true }).selectOption(ids.environment);
  await selection.getByLabel("目标精确构建", { exact: true }).selectOption(ids.newKernel);
  await selection.getByRole("button", { name: "查看迁移差异", exact: true }).click();
}
async function prepare(page: Page) {
  await preview(page);
  const selection = dialog(page, "选定环境内核迁移");
  await selection.getByRole("button", { name: "备份并试用副本", exact: true }).click();
  const confirmation = dialog(page, "确认备份并试用副本");
  await expect(confirmation.getByRole("button", { name: "确认备份并试用", exact: true })).toBeDisabled();
  await confirmation.getByRole("checkbox").check();
  await confirmation.getByRole("button", { name: "确认备份并试用", exact: true }).click();
}
async function trial(page: Page) {
  await prepare(page);
  const result = dialog(page, "迁移任务");
  await expect(result).toContainText("启动副本并读取实际参数");
  const id = (await snapshot(page)).operations.find(o => o.kind === "migration")!.id;
  await page.evaluate(id => window.__migrationWorkflow.advance(id, "trial-running"), id);
  await expect(result).toContainText("正在试用新构建副本");
  return id;
}
async function ready(page: Page) {
  const id = await trial(page);
  await dialog(page, "迁移任务").getByRole("button", { name: "正常停止试用副本", exact: true }).click();
  await expect(dialog(page, "迁移任务")).toContainText("副本已停止，等待明确确认");
  return id;
}
async function rollbackPreflight(page: Page) {
  const selection = dialog(page, "选定环境内核迁移");
  await selection.getByText("迁移记录与升级前恢复", { exact: true }).click();
  await selection.getByRole("button", { name: "预检升级前完整恢复", exact: true }).click();
  await expect(selection).toContainText("升级前完整恢复预检");
}
async function restoreConfirm(page: Page) {
  await dialog(page, "选定环境内核迁移").getByRole("button", { name: "确认完整恢复", exact: true }).click();
  const confirmation = dialog(page, "确认完整恢复");
  await expect(confirmation.getByRole("button", { name: "确认并完整恢复", exact: true })).toBeDisabled();
  await confirmation.getByLabel(/确认覆盖 1 个环境/).check();
  await confirmation.getByLabel(/代理凭据需要重新输入/).check();
  await confirmation.getByLabel(/允许正常停止受影响环境/).check();
  await confirmation.getByRole("button", { name: "确认并完整恢复", exact: true }).click();
}

for (const viewport of [{ width: 1440, height: 900 }, { width: 1280, height: 800 }]) {
  test(`actual App migration graph and exact switch owner ${viewport.width}`, async ({ page }) => {
    await page.setViewportSize(viewport); await open(page);
    const initial = (await snapshot(page)).workspace;
    const selection = dialog(page, "选定环境内核迁移");
    await selection.getByLabel("选定环境", { exact: true }).selectOption(ids.environment);
    expect(await selection.getByLabel("目标精确构建").locator("option").evaluateAll(options => options.map(o => (o as HTMLOptionElement).value))).toEqual(["", ids.newKernel]);
    const id = await ready(page);
    await expect(resultIdentity(page, id)).toBeVisible();
    expect((await snapshot(page)).workspace.state.environments).toEqual(initial.state.environments);
    await dialog(page, "迁移任务").getByRole("button", { name: "明确切换", exact: true }).click();
    const confirm = dialog(page, "确认切换选定环境");
    expect(Math.round((await confirm.boundingBox())!.width)).toBe(400);
    await expect(confirm.getByRole("button", { name: "确认切换", exact: true })).toBeDisabled();
    expect(await confirm.evaluate(e => e.contains(document.activeElement))).toBe(true);
    expect(await dialog(page, "迁移任务").evaluate(e => !!e.closest("[inert]"))).toBe(true);
    await page.keyboard.press("Escape");
    await expect(confirm).toHaveCount(0); expect(await commits(page)).toHaveLength(0);
    await expect(dialog(page, "迁移任务").getByRole("button", { name: "明确切换", exact: true })).toBeFocused();
    await dialog(page, "迁移任务").getByRole("button", { name: "明确切换", exact: true }).click();
    await confirm.getByRole("checkbox").check();
    await confirm.getByRole("button", { name: "确认切换", exact: true }).click();
    await expect(dialog(page, "迁移任务")).toContainText("配置已提交，核对目录");
    await page.evaluate(id => window.__migrationWorkflow.advance(id, "completed"), id);
    await expect(dialog(page, "迁移任务")).toContainText("迁移已完成");
    const end = await snapshot(page);
    expect(await commits(page)).toEqual([{ mode: "native", method: "Migration.Action", payload: { operationId: id, action: "commit", confirm: true } }]);
    expect((await calls(page, "Migration.Action"))[0].payload).toEqual({ operationId: id, action: "stop", confirm: true });
    expect((await calls(page, "Migration.Prepare"))[0].payload).toEqual({ previewId: "synthetic-migration-preview-1", confirm: true, requestId: "00000000-0000-4000-8000-000000000001" });
    expect((await calls(page, "Migration.Preview"))[0].payload).toEqual({ environmentId: ids.environment, kernelId: ids.newKernel });
    expect((await calls(page, "Environment.Preview"))[0].payload).toEqual({ kind: "edit", sourceId: ids.environment });
    expect(end.workspace.state.environments[0]).toEqual({ ...initial.state.environments[0], coreId: ids.newKernel });
    expect(end.workspace.state.environments[1]).toEqual(initial.state.environments[1]);
    expect(end.workspace.defaultKernel).toEqual(initial.defaultKernel);
    expect(end.workspace.dataReferences).toEqual(initial.dataReferences);
    expect(end.workspace.fingerprints![ids.environment].profile).toMatchObject({ seed: ids.seed, kernelId: ids.newKernel, configRevision: 4 });
    expect(end.savedRevision).toBe(ids.revision + 1);
    expect(end.operations[0].migrationReport).toMatchObject({ environmentId: ids.environment, oldKernelId: ids.oldKernel, newKernelId: ids.newKernel, seed: ids.seed, committed: true, trialExited: true, protected: false });
  });
}
const resultIdentity = (page: Page, id: string) => dialog(page, "迁移任务").getByText(id, { exact: true });

test("saved direct policy is explicit; trial confirmation cancel sends zero prepares", async ({ page }) => {
  await open(page, "direct"); const before = (await snapshot(page)).workspace.state;
  await preview(page);
  await expect(dialog(page, "选定环境内核迁移")).toContainText("未绑定代理，明确直连");
  await dialog(page, "选定环境内核迁移").getByRole("button", { name: "备份并试用副本", exact: true }).click();
  await expect(dialog(page, "确认备份并试用副本")).toContainText("已确认为未绑定代理");
  await dialog(page, "确认备份并试用副本").getByRole("button", { name: "取消", exact: true }).click();
  expect(await calls(page, "Migration.Prepare")).toHaveLength(0); expect((await snapshot(page)).workspace.state).toEqual(before);
});

for (const scenario of ["prepare-unknown", "prepare-unknown-no-id"] as const) {
  test(`${scenario} retries original memoized request without another task`, async ({ page }) => {
    await open(page, scenario); const before = (await snapshot(page)).workspace.state;
    await prepare(page);
    await expect(dialog(page, "确认备份并试用副本").getByRole("alert")).toContainText("NATIVE_UNAVAILABLE");
    await page.keyboard.press("Escape");
    const selection = dialog(page, "选定环境内核迁移");
    await expect(selection.getByLabel("选定环境", { exact: true })).toBeDisabled();
    await expect(selection.getByLabel("目标精确构建")).toBeDisabled();
    await selection.getByRole("button", { name: "核实原迁移请求", exact: true }).click();
    await expect(dialog(page, "迁移任务")).toContainText("启动副本并读取实际参数");
    const trace = await snapshot(page), requests = await calls(page, "Migration.Prepare");
    expect(requests).toHaveLength(2); expect(requests[1]).toEqual(requests[0]); expect(trace.receipts).toHaveLength(1); expect(trace.operations).toHaveLength(1);
    expect(trace.workspace.state).toEqual(before); expect(trace.savedRevision).toBe(ids.revision); expect(await commits(page)).toHaveLength(0);
  });
}
test("definite prepare refusal preserves original configuration and has no accepted task", async ({ page }) => {
  await open(page, "prepare-refused"); const before = (await snapshot(page)).workspace;
  await prepare(page); await expect(dialog(page, "确认备份并试用副本").getByRole("alert")).toContainText("MIGRATION_NOT_ACCEPTED");
  await page.keyboard.press("Escape");
  await expect(dialog(page, "选定环境内核迁移").getByLabel("选定环境", { exact: true })).toBeEnabled();
  expect((await snapshot(page)).workspace).toEqual(before); expect((await snapshot(page)).receipts).toHaveLength(0); expect(await commits(page)).toHaveLength(0);
});
for (const scenario of ["policy-unavailable", "policy-mismatch", "revision-conflict", "preview-identity", "needs-reconcile"] as const) {
  test(`${scenario} cannot guess direct policy or bypass exact preview checks`, async ({ page }) => {
    await open(page, scenario); const before = (await snapshot(page)).workspace.state;
    await preview(page);
    await expect(dialog(page, "选定环境内核迁移").getByRole("alert")).toBeVisible();
    await expect(dialog(page, "选定环境内核迁移").getByRole("button", { name: "备份并试用副本", exact: true })).toHaveCount(0);
    expect(await calls(page, "Migration.Prepare")).toHaveLength(0); expect((await snapshot(page)).workspace.state).toEqual(before);
  });
}
test("unconfirmed normal stop is protected, never ready; recovery is original task only", async ({ page }) => {
  await open(page, "stop-unconfirmed"); const before = (await snapshot(page)).workspace.state;
  const id = await trial(page), result = dialog(page, "迁移任务");
  await result.getByRole("button", { name: "正常停止试用副本", exact: true }).click();
  await expect(result).toContainText("结果未确认，保持保护");
  await expect(result).toContainText("试用全树退出：尚未确认"); await expect(result).not.toContainText("副本已停止，等待明确确认");
  await expect(result.getByRole("button", { name: "明确切换", exact: true })).toHaveCount(0);
  expect(await commits(page)).toHaveLength(0); expect((await snapshot(page)).workspace.state).toEqual(before);
  await result.getByRole("button", { name: "核对原任务", exact: true }).click();
  await expect(result).toContainText("原状态已保留");
  expect((await calls(page, "Migration.Action")).map(c => c.payload)).toEqual([{ operationId: id, action: "stop", confirm: true }, { operationId: id, action: "recover", confirm: true }]);
  expect((await snapshot(page)).workspace.state).toEqual(before);
});
test("trial cancellation is exact and retains old build seed proxy and data reference", async ({ page }) => {
  await open(page); const before = (await snapshot(page)).workspace; const id = await trial(page);
  await dialog(page, "迁移任务").getByRole("button", { name: "取消并保留原状态", exact: true }).click();
  await expect(dialog(page, "迁移任务")).toContainText("原状态已保留");
  expect(await calls(page, "Operation.Cancel")).toEqual([{ mode: "native", method: "Operation.Cancel", payload: { operationId: id } }]);
  const end = (await snapshot(page)).workspace; expect(end.state).toEqual(before.state); expect(end.dataReferences).toEqual(before.dataReferences); expect(end.fingerprints).toEqual(before.fingerprints); expect((await snapshot(page)).savedRevision).toBe(ids.revision); expect(await commits(page)).toHaveLength(0);
});

for (const change of ["protected", "restore", "new-owner"] as const) {
  test(`open switch confirmation fails closed after ${change}`, async ({ page }) => {
    await open(page); const id = await ready(page);
    await dialog(page, "迁移任务").getByRole("button", { name: "明确切换", exact: true }).click();
    const confirm = dialog(page, "确认切换选定环境"); await confirm.getByRole("checkbox").check();
    if (change === "protected") await page.evaluate(id => window.__migrationWorkflow.advance(id, "protected"), id);
    else if (change === "restore") await page.evaluate(() => window.__migrationWorkflow.protectWorkspace("restore"));
    else await page.evaluate(id => window.__migrationWorkflow.supersede(id), id);
    await expect.poll(async () => (await calls(page, "Workspace.Read")).length).toBeGreaterThan(3);
    // Wait for the existing App/operation polling to observe the new owner.
    if (change === "protected") await expect(dialog(page, "迁移任务").locator("h3")).toContainText("结果未确认");
    else if (change === "new-owner") await expect(dialog(page, "迁移任务")).toContainText("synthetic-migration-task-2");
    else await expect(page.getByText("完整恢复正在维护保护中", { exact: false }).first()).toBeVisible();
    const button = confirm.getByRole("button", { name: "确认切换", exact: true });
    if (await confirm.count()) await expect(button).toBeDisabled();
    expect(await commits(page)).toHaveLength(0);
  });
}

for (const method of ["Migration.Action", "Operation.Cancel", "Operation.Read"] as const) {
  test(`late ${method} response cannot publish over a new task`, async ({ page }) => {
    await open(page); const id = await trial(page);
    await page.evaluate(({ method, id }) => window.__migrationWorkflow.hold(method, "synthetic-old-reply", { operationId: id, ...(method === "Migration.Action" ? { action: "stop" } : {}) }), { method, id });
    if (method === "Migration.Action") await dialog(page, "迁移任务").getByRole("button", { name: "正常停止试用副本", exact: true }).click();
    if (method === "Operation.Cancel") await dialog(page, "迁移任务").getByRole("button", { name: "取消并保留原状态", exact: true }).click();
    await expect.poll(async () => (await snapshot(page)).held.length).toBe(1);
    const next = await page.evaluate(id => window.__migrationWorkflow.supersede(id), id);
    await expect(dialog(page, "迁移任务")).toContainText(next);
    await page.evaluate(() => window.__migrationWorkflow.reply("synthetic-old-reply"));
    await expect(dialog(page, "迁移任务")).toContainText(next);
    await expect(resultIdentity(page, id)).toHaveCount(0);
    await expect(dialog(page, "迁移任务")).toContainText("正在试用新构建副本");
    expect(await commits(page)).toHaveLength(0);
  });
}
test("workspace maintenance disables selecting previewing and preparing another migration", async ({ page }) => {
  await open(page, "workspace-maintenance"); const selection = dialog(page, "选定环境内核迁移");
  for (const label of ["选定环境", "目标精确构建", "查找迁移环境"]) await expect(selection.getByLabel(label, { exact: true })).toBeDisabled();
  await expect(selection.getByRole("button", { name: "查看迁移差异", exact: true })).toBeDisabled();
  expect(await calls(page, "Migration.Preview")).toHaveLength(0); expect(await calls(page, "Migration.Prepare")).toHaveLength(0);
});
test("foreign restore lease disables conflicting trial stop and cancellation commands", async ({ page }) => {
  await open(page); await trial(page);
  await page.evaluate(() => window.__migrationWorkflow.protectWorkspace("restore"));
  await expect(page.getByText("完整恢复正在维护保护中", { exact: false }).first()).toBeVisible();
  const result = dialog(page, "迁移任务");
  await expect(result.getByRole("button", { name: "正常停止试用副本", exact: true })).toBeDisabled();
  await expect(result.getByRole("button", { name: "取消并保留原状态", exact: true })).toBeDisabled();
  expect(await calls(page, "Migration.Action")).toHaveLength(0); expect(await calls(page, "Operation.Cancel")).toHaveLength(0);
});

for (const scenario of ["rollback-success", "rollback-failed", "rollback-protected"] as const) {
  test(`${scenario} actual App upgrade-before-restore preflight confirmation and result`, async ({ page }) => {
    await open(page, scenario); const before = (await snapshot(page)).workspace;
    await rollbackPreflight(page); expect(await calls(page, "Backup.ApplyRestore")).toHaveLength(0);
    expect((await snapshot(page)).workspace).toEqual(before);
    await dialog(page, "选定环境内核迁移").getByRole("button", { name: "确认完整恢复", exact: true }).click();
    const confirm = dialog(page, "确认完整恢复");
    expect(Math.round((await confirm.boundingBox())!.width)).toBe(400);
    await page.keyboard.press("Escape"); expect(await calls(page, "Backup.ApplyRestore")).toHaveLength(0);
    await restoreConfirm(page);
    const result = dialog(page, "完整恢复结果"); await expect(result).toBeVisible();
    expect((await calls(page, "Migration.SelectRollback"))[0].payload).toEqual({ operationId: "synthetic-migration-task-1" });
    expect((await calls(page, "Backup.PreviewRestore"))[0].payload).toEqual({ sourceToken: "synthetic-upgrade-source" });
    expect((await calls(page, "Backup.ApplyRestore"))[0].payload).toEqual({ previewId: "synthetic-upgrade-restore-preview", archiveSha256: ids.archiveSha256, confirmOverwrite: true, acknowledgeCredentials: true, stopRunning: true, requestId: "00000000-0000-4000-8000-000000000001" });
    // onConsumed removes the exact accepted rollback preview; onLockChange
    // keeps its lower selection protected while the restore remains uncertain.
    await expect(dialog(page, "选定环境内核迁移").getByRole("button", { name: "确认完整恢复", exact: true })).toHaveCount(0);
    const after = (await snapshot(page)).workspace;
    expect(after.state.environments[1]).toEqual(before.state.environments[1]); expect(after.dataReferences).toEqual(before.dataReferences); expect(after.defaultKernel).toEqual(before.defaultKernel);
    if (scenario === "rollback-success") {
      await expect(result).toContainText("完整新状态已提交并核对");
      expect(after.state.environments[0]).toMatchObject({ id: ids.environment, seed: ids.seed, proxyId: ids.proxy, coreId: ids.oldKernel });
    } else {
      await expect(result).not.toContainText("完整新状态已提交并核对"); expect(after.state).toEqual(before.state);
      await expect(result).toContainText(scenario === "rollback-failed" ? "恢复未完成，原状态已回滚" : "工作区保持维护保护");
    }
    await page.keyboard.press("Escape");
    const selection = dialog(page, "选定环境内核迁移");
    if (scenario === "rollback-protected") await expect(selection.getByLabel("选定环境", { exact: true })).toBeDisabled();
    else await expect(selection.getByLabel("选定环境", { exact: true })).toBeEnabled();
    await selection.getByRole("button", { name: "关闭", exact: true }).click();
    expect(await calls(page, "Backup.DiscardRestore")).toHaveLength(0);
  });
}
test("upgrade rollback token digest mismatch fails closed and discards only its source", async ({ page }) => {
  await open(page, "rollback-hash-mismatch"); const selection = dialog(page, "选定环境内核迁移");
  await selection.getByText("迁移记录与升级前恢复", { exact: true }).click();
  await selection.getByRole("button", { name: "预检升级前完整恢复", exact: true }).click();
  await expect(selection.getByRole("alert")).toContainText("MIGRATION_RESULT_UNCONFIRMED");
  expect(await calls(page, "Backup.PreviewRestore")).toHaveLength(0); expect(await calls(page, "Backup.ApplyRestore")).toHaveLength(0);
  expect((await calls(page, "Backup.DiscardRestore"))[0].payload).toEqual({ previewId: "", sourceToken: "synthetic-upgrade-source" });
});
test("unsubmitted upgrade rollback preview is discarded exactly on route leave", async ({ page }) => {
  await open(page, "rollback-failed"); await rollbackPreflight(page);
  await dialog(page, "选定环境内核迁移").getByRole("button", { name: "关闭", exact: true }).click();
  await page.getByRole("link", { name: "操作记录", exact: true }).click();
  await expect.poll(async () => (await calls(page, "Backup.DiscardRestore")).length).toBe(1);
  expect((await calls(page, "Backup.DiscardRestore"))[0].payload).toEqual({ previewId: "synthetic-upgrade-restore-preview", sourceToken: "synthetic-upgrade-source" });
  expect((await snapshot(page)).sourceActive).toBe(false); expect(await calls(page, "Backup.ApplyRestore")).toHaveLength(0);
});
test("matching restore in flight locks migration via onLockChange and consumes only accepted preview", async ({ page }) => {
  await open(page, "rollback-failed"); await rollbackPreflight(page);
  await page.evaluate(() => window.__migrationWorkflow.hold("Backup.ApplyRestore", "synthetic-matching-restore"));
  await restoreConfirm(page);
  await expect.poll(async () => (await snapshot(page)).held.length).toBe(1);
  const selection = dialog(page, "选定环境内核迁移");
  await expect(selection.getByLabel("选定环境", { exact: true })).toBeDisabled();
  await expect(selection.getByRole("button", { name: "预检升级前完整恢复", exact: true })).toBeDisabled();
  await expect(dialog(page, "确认完整恢复").getByRole("button", { name: "确认并完整恢复", exact: true })).toBeDisabled();
  expect(await calls(page, "Backup.DiscardRestore")).toHaveLength(0);
  await page.evaluate(() => window.__migrationWorkflow.reply("synthetic-matching-restore"));
  await expect(dialog(page, "完整恢复结果")).toContainText("恢复未完成，原状态已回滚");
  await expect(selection.getByRole("button", { name: "确认完整恢复", exact: true })).toHaveCount(0);
  await page.keyboard.press("Escape"); await expect(selection.getByLabel("选定环境", { exact: true })).toBeEnabled();
  expect(await calls(page, "Backup.ApplyRestore")).toHaveLength(1); expect(await calls(page, "Migration.Prepare")).toHaveLength(0);
});
test("unknown upgrade restore preserves props request source and lock across actual App routes", async ({ page }) => {
  await open(page, "rollback-unknown"); await rollbackPreflight(page); const before = (await snapshot(page)).workspace.state;
  await restoreConfirm(page); const result = dialog(page, "完整恢复结果");
  await expect(result).toContainText("原受理尚未核实");
  await page.keyboard.press("Escape"); await dialog(page, "选定环境内核迁移").getByRole("button", { name: "关闭", exact: true }).click();
  await page.getByRole("link", { name: "操作记录", exact: true }).click(); await page.getByRole("link", { name: "内核管理", exact: true }).click();
  await page.getByRole("button", { name: "选定环境迁移", exact: true }).click();
  const selection = dialog(page, "选定环境内核迁移"); await expect(selection.getByLabel("选定环境", { exact: true })).toBeDisabled();
  await selection.getByRole("button", { name: "继续核实原恢复", exact: true }).click();
  await result.getByRole("button", { name: "核实原恢复请求", exact: true }).click();
  const requests = await calls(page, "Backup.ApplyRestore"); expect(requests).toHaveLength(2); expect(requests[1]).toEqual(requests[0]);
  expect(await calls(page, "Migration.SelectRollback")).toHaveLength(1); expect(await calls(page, "Backup.DiscardRestore")).toHaveLength(0);
  expect((await snapshot(page)).workspace.state).toEqual(before); await expect(result).not.toContainText("完整新状态已提交并核对");
});

test("synthetic boundary rejects unsupported methods and changed memoized requests", async ({ page }, info) => {
  await open(page); const id = await trial(page), before = (await snapshot(page)).workspace;
  info.annotations.push({ type: "synthetic-unsupported", description: "Synthetic.Unsupported" });
  const responses = await page.evaluate(async () => {
    const call = (window as unknown as { go: { main: { DesktopApp: { Call(request: { mode: string; method: string; payload: unknown }): Promise<{ ok: boolean; error?: { code: string } }> } } } }).go.main.DesktopApp.Call;
    const original = window.__migrationWorkflow.snapshot().calls.find(c => c.method === "Migration.Prepare")!;
    return [await call({ mode: "native", method: "Synthetic.Unsupported", payload: {} }), await call({ ...original, payload: { ...original.payload as Record<string, unknown>, previewId: "synthetic-wrong-preview" } })];
  });
  expect(responses).toMatchObject([{ ok: false, error: { code: "CAPABILITY_UNSUPPORTED" } }, { ok: false, error: { code: "REQUEST_ID_REUSED" } }]);
  expect((await snapshot(page)).workspace).toEqual(before); expect((await snapshot(page)).receipts).toHaveLength(1);
  await expect(resultIdentity(page, id)).toBeVisible(); expect(await commits(page)).toHaveLength(0);
});
