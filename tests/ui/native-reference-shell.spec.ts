import { test, expect } from "@playwright/test";
import { nativeReferenceBridge, nativeReferenceView } from "./fixtures/native-reference-bridge.ts";

test("native 10-row paging keeps off-page exact IDs, saved revisions and network policies", async ({ page }) => {
  await nativeReferenceBridge(page); await page.goto("/#/environments");
  await expect(page.getByLabel("选择 工作环境 B", { exact: true })).toBeEnabled();
  await page.getByLabel("选择 工作环境 B", { exact: true }).check();
  await page.getByRole("button", { name: "下一页", exact: true }).click();
  await expect(page.getByLabel("选择 工作环境 B 11", { exact: true })).toBeEnabled();
  await page.getByLabel("选择 工作环境 B 11", { exact: true }).check();
  await page.getByLabel("搜索环境").fill("测试环境 C");
  await page.getByRole("button", { name: "批量打开", exact: true }).click();
  const results = page.getByRole("region", { name: "逐项操作结果" });
  await expect(results.locator(".outcome-error")).toHaveCount(1); await expect(results.locator(".outcome-success")).toHaveCount(1);
  const data = await nativeReferenceView(page);
  expect(data.calls.some(call => call.method === "Workspace.Read" && (call.payload as { environmentQuery?: { pageSize: number } }).environmentQuery?.pageSize === 10)).toBe(true);
  expect(data.calls.filter(call => call.method === "Runtime.Start").map(call => call.payload)).toMatchObject([
    { environmentId: "synthetic-reference-2", expectedRevision: 1, networkPolicy: "proxy" },
    { environmentId: "synthetic-reference-11", expectedRevision: 1, networkPolicy: "direct" },
  ]);
  expect(data.view.state.environments.filter(environment => environment.status === "running").map(environment => environment.id)).toEqual(["synthetic-reference-11"]);
  await page.evaluate(() => (window as unknown as { __referenceNative: { allowProxy: () => void } }).__referenceNative.allowProxy());
  await results.getByRole("button", { name: "工作环境 B 重试打开", exact: true }).click();
  await expect(results.locator(".outcome-success")).toHaveCount(2);
  expect((await nativeReferenceView(page)).calls.filter(call => call.method === "Runtime.Start").map(call => (call.payload as { networkPolicy: string }).networkPolicy)).toEqual(["proxy", "direct", "proxy"]);
  await page.getByRole("button", { name: "批量关闭", exact: true }).click();
  expect((await nativeReferenceView(page)).calls.filter(call => call.method === "Runtime.Stop").map(call => (call.payload as { environmentId: string }).environmentId)).toEqual(["synthetic-reference-2", "synthetic-reference-11"]);
});

test("native group partial failure reports real items and retries only the failed exact ID", async ({ page }) => {
  await nativeReferenceBridge(page); await page.goto("/#/environments");
  await expect(page.getByLabel("选择 工作环境 A", { exact: true })).toBeEnabled();
  await page.getByLabel("选择 工作环境 A", { exact: true }).check();
  await page.getByRole("button", { name: "下一页", exact: true }).click();
  await expect(page.getByLabel("选择 工作环境 B 11", { exact: true })).toBeEnabled();
  await page.getByLabel("选择 工作环境 B 11", { exact: true }).check();
  const before = (await nativeReferenceView(page)).view.state.environments;
  await page.getByRole("button", { name: "调整分组", exact: true }).click();
  await page.getByLabel("分组名称", { exact: true }).fill("native 精确分组");
  await page.getByRole("button", { name: "应用到所选环境", exact: true }).click();
  const results = page.getByRole("region", { name: "逐项操作结果" });
  await expect(results.locator(".outcome-success")).toHaveCount(1); await expect(results.locator(".outcome-error")).toHaveCount(1);
  expect((await nativeReferenceView(page)).view.state.environments.filter(environment => environment.group === "native 精确分组").map(environment => environment.id)).toEqual(["synthetic-reference-1"]);
  await page.evaluate(() => (window as unknown as { __referenceNative: { allowGroup: () => void } }).__referenceNative.allowGroup());
  await results.getByRole("button", { name: "工作环境 B 11 重试分组", exact: true }).click();
  await expect(results.locator(".outcome-success")).toHaveCount(2);
  const data = await nativeReferenceView(page);
  expect(data.updates).toEqual([{ id: "synthetic-reference-1", revision: 1 }, { id: "synthetic-reference-11", revision: 1 }, { id: "synthetic-reference-11", revision: 1 }]);
  expect(data.view.state.environments.map(environment => [environment.id, environment.seed, environment.coreId, environment.proxyId])).toEqual(before.map(environment => [environment.id, environment.seed, environment.coreId, environment.proxyId]));
});

test("native busy rows retain reconcile, cleanup, pending, force confirmation and FIFO controls", async ({ page }) => {
  await nativeReferenceBridge(page, true); await page.goto("/#/environments");
  const row = (name: string) => page.getByRole("row").filter({ hasText: name });
  await expect(row("测试环境 C").getByRole("button", { name: "核对会话", exact: true })).toBeVisible();
  await expect(row("工作环境 A 4").getByRole("button", { name: "重试资源清理", exact: true })).toBeVisible();
  await expect(row("工作环境 B 5")).toContainText("结果待保存");
  await expect(row("工作环境 B 5").getByRole("button", { name: "打开", exact: true })).toHaveCount(0);
  const force = row("测试环境 C 6").getByRole("button", { name: "强制结束", exact: true });
  await force.click();
  const confirmation = page.getByRole("alertdialog", { name: "强制结束指定会话？", exact: true });
  await expect(confirmation).toHaveCSS("width", "400px");
  await expect(confirmation).toContainText("这份已确认会话");
  await confirmation.getByRole("button", { name: "取消", exact: true }).click();
  expect((await nativeReferenceView(page)).calls.filter(call => call.method === "Runtime.ForceStop")).toHaveLength(0);
  await force.click(); await confirmation.getByRole("button", { name: "确认强制结束", exact: true }).click();
  await row("测试环境 C").getByRole("button", { name: "核对会话", exact: true }).click();
  await row("工作环境 A 4").getByRole("button", { name: "重试资源清理", exact: true }).click();
  await page.getByRole("button", { name: "取消排队启动", exact: true }).click();
  const calls = (await nativeReferenceView(page)).calls;
  expect(calls.filter(call => call.method === "Runtime.ForceStop").map(call => call.payload)).toMatchObject([{ environmentId: "synthetic-reference-6", sessionId: "synthetic-session-6" }]);
  expect(calls.filter(call => call.method === "Runtime.Reconcile").map(call => call.payload)).toMatchObject([{ environmentId: "synthetic-reference-3", sessionId: "synthetic-session-3" }, { environmentId: "synthetic-reference-4", sessionId: "synthetic-session-4" }]);
  expect(calls.filter(call => call.method === "Runtime.Start")).toHaveLength(0);
  expect(calls.filter(call => call.method === "Operation.Cancel")).toHaveLength(1);
});
