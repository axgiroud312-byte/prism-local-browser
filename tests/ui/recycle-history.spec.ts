import { test, expect, type Page } from "@playwright/test";
import type { NativeRecycleItem, NativeRecyclePage, Operation, WorkspaceView } from "../../src/application/contract";
import type { NativeRequest } from "../../src/application/wails-adapter";

type RecycleHooks = { calls: NativeRequest[]; release(): void };
declare global { interface Window { __recycleCloseout: RecycleHooks } }

// Automated application/UI contract evidence only; no native files or processes.
async function bridge(page: Page, scenario: "history" | "wrong-target" | "late" = "history") {
  await page.addInitScript(({ scenario }) => {
    const operation: Operation = { id: "synthetic-original-recycle-operation", kind: "recycle", state: "cancelled", stage: "finished", total: 27, completedIds: [], cancelRequested: true, recycleReport: { mode: "native", action: "purge", requestId: "synthetic-original-request", previewId: "synthetic-original-preview", sequence: 8, completed: 1, failed: 1, notExecuted: 25, protected: false } };
    const items: NativeRecycleItem[] = Array.from({ length: 27 }, (_, index) => ({ id: `synthetic-trash-${index + 1}`, environmentId: `synthetic-environment-${index + 1}`, name: `合成回收 ${index + 1}`, seed: String(180000001 + index), kernelId: "synthetic-exact-kernel", revision: 3, dataPresent: true, backupRecorded: false, state: "recycled" }));
    const workspace: WorkspaceView = { mode: "native", state: { schemaVersion: 1, environments: [], proxies: [], kernels: [], backups: [], activities: [] }, recycleOperations: [operation] };
    const held: (() => void)[] = [];
    window.__recycleCloseout = { calls: [], release() { held.splice(0).forEach(resolve => resolve()); } };
    Object.defineProperty(window, "localStorage", { get() { throw new Error("native-only synthetic test must not access demo storage"); } });
    Object.assign(window, { go: { main: { DesktopApp: { Call: async (request: NativeRequest) => {
      window.__recycleCloseout.calls.push(structuredClone(request));
      if (request.method === "Workspace.Read") return { ok: true, mode: "native", data: structuredClone(workspace) };
      if (request.method === "Recycle.ReadPage") {
        const input = request.payload as { operationId?: string; offset: number; pageSize: number };
        const result: NativeRecyclePage = { mode: "native", offset: input.offset, pageSize: input.pageSize, total: items.length, items: structuredClone(items.slice(input.offset, input.offset + input.pageSize)) };
        if (input.operationId) {
          result.operation = structuredClone(operation);
          result.items = result.items.map((item, index) => ({ ...item, state: input.offset + index === 0 ? "purged" : input.offset + index === 1 ? "failed" : "pending" }));
          if (scenario === "wrong-target") result.operation.id = "synthetic-unrelated-operation";
          if (scenario === "late") await new Promise<void>(resolve => held.push(resolve));
        }
        return { ok: true, mode: "native", data: result };
      }
      return { ok: false, mode: "native", error: { code: "CAPABILITY_UNSUPPORTED", message: "合成测试未提供此入口", retryable: false } };
    } } } } });
  }, { scenario });
}

const calls = (page: Page) => page.evaluate(() => window.__recycleCloseout.calls);
async function open(page: Page) {
  await page.getByRole("button", { name: "打开回收区", exact: true }).click();
  const list = page.getByRole("dialog", { name: "本机回收区", exact: true });
  await expect(list.getByText("共 27 项 · 本页 25 项", { exact: true })).toBeVisible();
  return list;
}

test("Recycle.ReadPage history uses the frozen operation and exposes failed and all unexecuted targets across pages", async ({ page }) => {
  await bridge(page); await page.goto("/#/environments"); const list = await open(page);
  await list.getByLabel("最近回收任务").selectOption("synthetic-original-recycle-operation");
  const result = page.getByRole("dialog", { name: "回收任务结果", exact: true });
  await expect(result).toContainText("未执行 25");
  await result.getByText("逐项结果", { exact: true }).click();
  await expect(result).toContainText("已永久删除"); await expect(result).toContainText("未完成，原状态已核对");
  await expect(result.getByText("共 27 项 · 本页 25 项", { exact: true })).toBeVisible();
  await result.getByRole("button", { name: "下一页", exact: true }).click();
  await expect(result.getByText("共 27 项 · 本页 2 项", { exact: true })).toBeVisible();
  await expect(result).toContainText("synthetic-environment-27");
  await expect(result.getByRole("button", { name: "下一页", exact: true })).toBeDisabled();
  await result.getByRole("button", { name: "查看回收列表", exact: true }).click();
  await expect(list.getByLabel("选择回收项 合成回收 1", { exact: true })).toBeEnabled();
  const trace = await calls(page);
  expect(trace.filter(call => call.method === "Operation.Read")).toEqual([]);
  expect(trace.some(call => call.method === "Recycle.ReadPage" && JSON.stringify(call.payload) === JSON.stringify({ offset: 25, pageSize: 25, operationId: "synthetic-original-recycle-operation" }))).toBe(true);
});

test("Recycle.ReadPage wrong history identity gives an error and never shows unrelated success", async ({ page }) => {
  await bridge(page, "wrong-target"); await page.goto("/#/environments"); const list = await open(page);
  await list.getByLabel("最近回收任务").selectOption("synthetic-original-recycle-operation");
  await expect(list.getByRole("alert")).toContainText("RECYCLE_RESULT_UNCONFIRMED");
  await expect(page.getByRole("dialog", { name: "回收任务结果", exact: true })).toHaveCount(0);
  expect((await calls(page)).filter(call => call.method === "Operation.Read")).toEqual([]);
});

test("late original Recycle.ReadPage cannot replace a newly reopened live recycle list", async ({ page }) => {
  await bridge(page, "late"); await page.goto("/#/environments"); const list = await open(page);
  await list.getByLabel("最近回收任务").selectOption("synthetic-original-recycle-operation");
  await expect.poll(async () => (await calls(page)).filter(call => call.method === "Recycle.ReadPage" && (call.payload as { operationId?: string }).operationId).length).toBeGreaterThan(0);
  await page.getByRole("button", { name: "关闭回收区", exact: true }).click();
  const reopened = await open(page);
  await page.evaluate(() => window.__recycleCloseout.release());
  await expect(reopened.getByLabel("选择回收项 合成回收 1", { exact: true })).toBeEnabled();
  await expect(page.getByRole("dialog", { name: "回收任务结果", exact: true })).toHaveCount(0);
});
