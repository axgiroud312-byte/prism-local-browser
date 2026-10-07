import { test, expect, type Page } from "@playwright/test";
import type { NativeRequest } from "../../src/application/wails-adapter";
type Control = { calls: NativeRequest[]; release(): void; hold(): void; settle(): void };
const control = (page: Page, action: "release" | "hold" | "settle") => page.evaluate(action => (window as unknown as { __restoreOwner: Control }).__restoreOwner[action](), action);
const calls = (page: Page) => page.evaluate(() => (window as unknown as { __restoreOwner: Control }).__restoreOwner.calls);
async function enter(page: Page) { await page.getByRole("button", { name: "导入完整备份", exact: true }).click(); return page.getByRole("dialog", { name: "导入本机备份", exact: true }); }

test("ordinary restore recovers lost token and failed cleanup across hidden window and page remount", async ({ page }) => {
  await page.goto("/tests/ui/fixtures/restore-source-owner.html");
  let dialog = await enter(page); await dialog.getByRole("button", { name: "选择本机备份包", exact: true }).click();
  await expect(dialog).toContainText("原文件选择待核实"); await expect(dialog.getByRole("button", { name: "选择本机备份包", exact: true })).toBeDisabled();
  await dialog.getByRole("button", { name: "核实原来源", exact: true }).click(); await expect(dialog).toContainText("合成原包-1");
  await dialog.getByRole("button", { name: "完整校验并预览", exact: true }).click();
  dialog = page.getByRole("dialog", { name: "恢复前只读预检", exact: true }); await expect(dialog).toBeVisible();
  await dialog.getByRole("button", { name: "丢弃此预览", exact: true }).click(); await expect(dialog).toContainText("取消未确认");
  await dialog.getByRole("button", { name: "仅关闭窗口", exact: true }).click();
  await page.getByRole("button", { name: "离开恢复页", exact: true }).click(); await page.getByRole("button", { name: "返回恢复页", exact: true }).click();
  dialog = await enter(page); await expect(dialog).toContainText("合成原包-1"); await expect(dialog).toContainText("清理仍须确认");
  await expect(dialog.getByRole("button", { name: "完整校验并预览", exact: true })).toBeDisabled(); await expect(dialog.getByRole("button", { name: "选择本机备份包", exact: true })).toBeDisabled();
  await control(page, "release"); await dialog.getByRole("button", { name: "重试取消只读预检", exact: true }).click(); await expect(dialog).toHaveCount(0);
  dialog = await enter(page); await dialog.getByRole("button", { name: "选择本机备份包", exact: true }).click(); await expect(dialog).toContainText("合成原包-2");
  const requests = await calls(page), selections = requests.filter(r => r.method === "Backup.SelectRestoreSource"), discards = requests.filter(r => r.method === "Backup.DiscardRestore");
  expect(selections).toHaveLength(2); expect(new Set(discards.map(r => (r.payload as { requestId: string }).requestId)).size).toBe(1);
  expect((requests.find(r => r.method === "Backup.ReadRestoreSource")!.payload as { requestId: string }).requestId).toBe((selections[0].payload as { requestId: string }).requestId);
  expect(requests.some(r => r.method === "Workspace.Read" || r.method === "Backup.ApplyRestore")).toBe(false);
});

test("ordinary restore late preflight cannot reopen old target after cancellation and remount", async ({ page }) => {
  await page.goto("/tests/ui/fixtures/restore-source-owner.html"); await control(page, "hold");
  let dialog = await enter(page); await dialog.getByRole("button", { name: "选择本机备份包", exact: true }).click(); await dialog.getByRole("button", { name: "完整校验并预览", exact: true }).click();
  await dialog.getByRole("button", { name: "取消只读预检", exact: true }).click(); await expect(dialog).toContainText("取消未确认");
  await dialog.getByRole("button", { name: "仅关闭窗口", exact: true }).click(); await page.getByRole("button", { name: "离开恢复页", exact: true }).click();
  await control(page, "settle"); await page.getByRole("button", { name: "返回恢复页", exact: true }).click();
  dialog = await enter(page); await expect(dialog).toContainText("合成原包-1"); await expect(page.getByRole("dialog", { name: "恢复前只读预检", exact: true })).toHaveCount(0);
  await control(page, "release"); await dialog.getByRole("button", { name: "重试取消只读预检", exact: true }).click();
  dialog = await enter(page); await dialog.getByRole("button", { name: "选择本机备份包", exact: true }).click(); await dialog.getByRole("button", { name: "完整校验并预览", exact: true }).click();
  dialog = page.getByRole("dialog", { name: "恢复前只读预检", exact: true }); await expect(dialog).toContainText("合成原包-2"); await expect(dialog).not.toContainText("合成原包-1");
});

test("ordinary restore selecting a replacement first confirms discard of the exact original source", async ({ page }) => {
  await page.goto("/tests/ui/fixtures/restore-source-owner.html"); await control(page, "release");
  const dialog = await enter(page); await dialog.getByRole("button", { name: "选择本机备份包", exact: true }).click();
  await expect(dialog).toContainText("合成原包-1"); await dialog.getByRole("button", { name: "选择本机备份包", exact: true }).click();
  await expect(dialog).toContainText("合成原包-2");
  const requests = await calls(page);
  expect(requests.map(r => r.method)).toEqual(["Backup.SelectRestoreSource", "Backup.DiscardRestore", "Backup.SelectRestoreSource"]);
  const original = requests[0].payload as { requestId: string };
  expect(requests[1].payload).toEqual({ requestId: original.requestId, sourceToken: "host-token-1", previewId: "" });
  expect((requests[2].payload as { requestId: string }).requestId).not.toBe(original.requestId);
});
