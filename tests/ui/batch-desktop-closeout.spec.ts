import { expect, test, type Page } from "@playwright/test";
import type { NativeBatchPage, NativeBatchReport, Operation, WorkspaceView } from "../../src/application/contract";
import type { NativeRequest } from "../../src/application/wails-adapter";
import { nativeReferenceBridge } from "./fixtures/native-reference-bridge";

// Actual App/WailsAdapter UI with explicitly synthetic RPC responses. No native
// processes, directories, kernel execution or desktop acceptance are claimed.
type Scenario = "success" | "mixed" | "cancel";
const firstID = "synthetic-closeout-attempt-1";
const retryID = "synthetic-closeout-attempt-2";
const planID = "synthetic-closeout-plan";

async function installBatch(page: Page, scenario: Scenario) {
  await nativeReferenceBridge(page);
  await page.addInitScript(({ scenario, firstID, retryID, planID }) => {
    const install = () => {
      const fixture = (window as unknown as { __referenceNative: { calls: NativeRequest[]; view: WorkspaceView } }).__referenceNative;
      const host = (window as unknown as { go: { main: { DesktopApp: { Call(request: NativeRequest): Promise<unknown> } } } }).go.main.DesktopApp;
      const original = host.Call;
      const ok = (data: unknown) => ({ ok: true, mode: "native", data: structuredClone(data) });
      const unexpected: string[] = [];
      const failed = scenario === "mixed", cancelled = scenario === "cancel";
      const states = scenario === "success" ? ["completed", "completed", "completed", "completed", "completed"] : failed ? ["completed", "failed", "completed", "not-executed", "not-executed"] : ["completed", "not-executed", "not-executed", "not-executed", "not-executed"];
      const report: NativeBatchReport = { mode: "native", planId: planID, kind: "create", total: 5, completedCount: scenario === "success" ? 5 : failed ? 2 : 1, failedCount: failed ? 1 : 0, notExecutedCount: scenario === "success" ? 0 : failed ? 2 : 4, attemptCompletedCount: scenario === "success" ? 5 : failed ? 2 : 1, directAssignments: 5, sharedProxyAssignments: 0, sequence: 6 };
      const operation: Operation = { id: firstID, kind: "batch-create", state: scenario === "success" ? "completed" : failed ? "failed" : "running", total: 5, completedIds: [], cancelRequested: false, batchReport: report, ...(failed ? { error: { code: "DISK_FULL", message: "合成资源暂停，已提交项保留。", retryable: true } } : {}) };
      const first: NativeBatchPage = { ...report, offset: 0, pageSize: 25, expiresAt: "2099-10-07T01:00:00Z", operationId: firstID, currentOperationId: firstID, history: false, items: states.map((state, index) => ({ index, name: `合成批次目标 ${index + 1}`, ...(scenario === "success" || failed && index < 4 || cancelled && index < 2 ? { environmentId: `synthetic-created-${index}` } : {}), proxyId: "", proxyName: "不绑定代理", newIdentity: true, state: state as "completed" | "failed" | "not-executed", ...(state === "failed" ? { error: { code: "PROFILE_DIRECTORY_UNSAFE", message: "合成逐项拒绝", retryable: true } } : {}) })) };
      const attempts = new Map([[firstID, { page: first, operation }]]);
      let currentID = firstID;
      fixture.view.batchOperations = [operation];
      Object.assign(window, { __batchCloseout: { unexpected } });
      host.Call = async request => {
        if (request.method === "Workspace.Read") return original(request);
        fixture.calls.push(structuredClone(request));
        const payload = request.payload as Record<string, unknown>;
        if (request.method === "Operation.Read") {
          const found = attempts.get(String(payload.operationId));
          if (found) return ok(found.operation);
        }
        if (request.method === "Batch.ReadPage") {
          const found = attempts.get(String(payload.operationId ?? currentID));
        if (found && payload.planId === planID && payload.offset === 0 && payload.pageSize === 25) {
          const history = found.operation.id !== currentID;
          return ok({ ...found.page, currentOperationId: currentID, history, items: history ? found.page.items.map(item => item.state === "not-executed" ? { ...item, environmentId: undefined } : item) : found.page.items });
        }
        }
        if (request.method === "Operation.Cancel" && cancelled && payload.operationId === firstID) {
          operation.state = "cancelled"; operation.cancelRequested = true;
          operation.error = { code: "OPERATION_CANCELLED", message: "合成取消，保留已提交1项，其余4项未执行。", retryable: true };
          report.sequence++; first.sequence = report.sequence;
          return ok(operation);
        }
        if (request.method === "Batch.Retry" && payload.operationId === firstID && currentID === firstID && operation.state !== "running") {
          const nextReport: NativeBatchReport = { ...report, sequence: 20, completedCount: 5, failedCount: 0, notExecutedCount: 0, attemptCompletedCount: 5 - report.completedCount };
          const nextOperation: Operation = { ...operation, id: retryID, state: "completed", error: undefined, cancelRequested: false, batchReport: nextReport };
          const nextPage: NativeBatchPage = { ...first, ...nextReport, operationId: retryID, currentOperationId: retryID, items: first.items.map((item, index) => ({ ...item, environmentId: item.environmentId ?? `synthetic-created-${index}`, state: "completed", error: undefined })) };
          attempts.set(retryID, { page: nextPage, operation: nextOperation }); currentID = retryID;
          fixture.view.batchOperations = [nextOperation, operation];
          return ok({ status: "accepted", operation: nextOperation });
        }
        unexpected.push(request.method);
        return { ok: false, mode: "native", error: { code: "SYNTHETIC_UNEXPECTED_RPC", message: `Unexpected synthetic request ${request.method}`, retryable: false } };
      };
    };
    // Separate init-script order is unspecified. Install after the reference
    // bridge is present and before opening the batch dialog's refresh.
    window.addEventListener("DOMContentLoaded", install, { once: true });
  }, { scenario, firstID, retryID, planID });
  await page.goto("/#/environments");
}

const calls = (page: Page, method: string) => page.evaluate(method => (window as unknown as { __referenceNative: { calls: NativeRequest[] } }).__referenceNative.calls.filter(call => call.method === method), method);
const batch = (page: Page) => page.getByRole("dialog", { name: "持久批次与逐项结果", exact: true });
async function openHistory(page: Page) {
  await page.getByRole("button", { name: "更多操作", exact: true }).click();
  await page.getByRole("button", { name: "批次与逐项结果", exact: true }).click();
  await expect(batch(page)).toContainText(planID);
  await expect(batch(page).locator("tbody tr")).toHaveCount(5);
}
async function exactRows(page: Page, expected: string[]) {
  const rows = batch(page).locator("tbody tr");
  for (let index = 0; index < expected.length; index++) {
    await expect(rows.nth(index)).toContainText(`合成批次目标 ${index + 1}`);
    await expect(rows.nth(index).locator("td").last()).toContainText(expected[index]);
  }
}
test.afterEach(async ({ page }) => {
  expect(await page.evaluate(() => (window as unknown as { __batchCloseout: { unexpected: string[] } }).__batchCloseout.unexpected)).toEqual([]);
  expect(await calls(page, "Environment.Create")).toHaveLength(0);
  expect(await calls(page, "Batch.Commit")).toHaveLength(0);
  expect(await calls(page, "Runtime.Start")).toHaveLength(0);
});

test("native batch success contains every committed target and no unfinished retry", async ({ page }) => {
  await installBatch(page, "success"); await openHistory(page);
  await expect(batch(page)).toContainText("已完成 5 · 失败 0 · 未执行 0");
  await exactRows(page, Array(5).fill("已事务提交"));
  await expect(batch(page).getByRole("button", { name: "明确继续未完成项（不重复已完成项）", exact: true })).toHaveCount(0);
});

test("native mixed batch shows exact failed/unexecuted targets, continues old operation and freezes old results", async ({ page }) => {
  await installBatch(page, "mixed"); await openHistory(page);
  await expect(batch(page)).toContainText("已完成 2 · 失败 1 · 未执行 2");
  const expected = ["已事务提交", "失败，其他项保留", "已事务提交", "未执行", "未执行"];
  await exactRows(page, expected);
  await expect(batch(page)).toContainText("PROFILE_DIRECTORY_UNSAFE");
  await batch(page).getByRole("button", { name: "查看批次结果", exact: true }).click();
  const result = page.getByRole("dialog", { name: "批次执行结果", exact: true });
  await expect(result).toContainText("未执行 2 项");
  await result.getByRole("button", { name: "返回逐项明细", exact: true }).click();
  await batch(page).getByRole("button", { name: "明确继续未完成项（不重复已完成项）", exact: true }).click();
  await expect(batch(page)).toContainText("已完成 5 · 失败 0 · 未执行 0");
  await exactRows(page, Array(5).fill("已事务提交"));
  const retries = await calls(page, "Batch.Retry"); expect(retries).toHaveLength(1);
  expect(retries[0].payload).toEqual({ operationId: firstID, requestId: expect.any(String) });
  await batch(page).getByLabel("最近30次尝试", { exact: true }).selectOption(firstID);
  await expect(batch(page)).toContainText(`这是旧尝试 ${firstID} 的冻结结果`);
  await expect(batch(page)).toContainText("已完成 2 · 失败 1 · 未执行 2"); await exactRows(page, expected);
  await expect(batch(page).locator("tbody tr").nth(3)).toContainText("执行时分配新ID");
  await expect(batch(page).getByRole("button", { name: "明确继续未完成项（不重复已完成项）", exact: true })).toHaveCount(0);
  expect(await calls(page, "Batch.Retry")).toHaveLength(1);
});

test("native cancel keeps committed row and exact unexecuted suffix across close/reopen, then resumes original attempt", async ({ page }) => {
  await installBatch(page, "cancel"); await openHistory(page);
  await batch(page).getByRole("button", { name: "只取消此批次剩余项（保留已完成）", exact: true }).click();
  await expect(batch(page)).toContainText("已完成 1 · 失败 0 · 未执行 4");
  await exactRows(page, ["已事务提交", "未执行", "未执行", "未执行", "未执行"]);
  expect((await calls(page, "Operation.Cancel"))[0].payload).toEqual({ operationId: firstID });
  await batch(page).getByRole("button", { name: "关闭", exact: true }).click();
  await openHistory(page);
  await expect(batch(page)).toContainText("已完成 1 · 失败 0 · 未执行 4");
  expect(await calls(page, "Batch.Retry")).toHaveLength(0);
  await batch(page).getByRole("button", { name: "明确继续未完成项（不重复已完成项）", exact: true }).click();
  await expect(batch(page)).toContainText("已完成 5 · 失败 0 · 未执行 0");
  await exactRows(page, Array(5).fill("已事务提交"));
  expect((await calls(page, "Batch.Retry"))[0].payload).toEqual({ operationId: firstID, requestId: expect.any(String) });
  expect(await calls(page, "Operation.Cancel")).toHaveLength(1);
});
