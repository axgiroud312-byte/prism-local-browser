import { test, expect } from "@playwright/test";

test("a broken desktop bridge is blocked in native mode, never replaced by DemoAdapter", async ({ page }) => {
  const errors: string[] = [];
  page.on("pageerror", e => errors.push(e.message));
  await page.addInitScript(() => {
    Object.assign(window, { runtime: {} });
    Object.defineProperty(window, "localStorage", { get() { throw new Error("demo storage must not be read in native mode"); } });
  });
  await page.goto("/#/environments");
  await expect(page.getByRole("alertdialog", { name: "工作区需要处理" })).toContainText("本地服务连接失败");
  await expect(page.getByText("本机桌面", { exact: true })).toBeVisible();
  await expect(page.getByText("北美主店", { exact: true })).toHaveCount(0);
  await expect(page.getByRole("button", { name: "重置演示工作区" })).toHaveCount(0);
  await page.getByRole("button", { name: "重新读取本机工作区" }).click();
  await expect(page.getByRole("alertdialog")).toContainText("本地服务连接失败");
  expect(errors).toEqual([]);
});

test("late cancellation of an old kernel task cannot overwrite the next task", async ({ page }) => {
  await page.addInitScript(() => {
    let count = 0; let terminal = false;
    const task = (id: string) => ({ id, kind: "kernel-install", state: "running", stage: "probing", total: 1, completedIds: [], cancelRequested: false });
    Object.assign(window, { __endOldKernel: () => { terminal = true; }, go: { main: { DesktopApp: { Call: async (request: { method: string; payload: { operationId: string } }) => {
      if (request.method === "Workspace.Read") return { ok: true, mode: "native", data: { mode: "native", state: { schemaVersion: 1, environments: [], proxies: [], kernels: [], backups: [], activities: [] }, kernelRecords: [], kernelOperations: [] } };
      if (request.method === "Kernel.Install") return { ok: true, mode: "native", data: { status: "accepted", operation: task(`synthetic-task-${++count}`) } };
      if (request.method === "Operation.Read") return { ok: true, mode: "native", data: terminal && request.payload.operationId === "synthetic-task-1" ? { ...task(request.payload.operationId), state: "cancelled", stage: "cancelled", cancelRequested: true } : task(request.payload.operationId) };
      if (request.method === "Operation.Cancel") return new Promise(resolve => { Object.assign(window, { __releaseOldCancel: () => resolve({ ok: true, mode: "native", data: { ...task("synthetic-task-1"), cancelRequested: true } }) }); });
      throw new Error("unexpected fixture request");
    } } } } });
  });
  await page.goto("/#/kernels");
  await page.getByRole("button", { name: "安装并核验", exact: true }).click();
  await expect(page.getByText("synthetic-task-1", { exact: true })).toBeVisible();
  await page.getByRole("button", { name: "取消此任务" }).click();
  await page.evaluate(() => (window as unknown as { __endOldKernel: () => void }).__endOldKernel());
  await expect(page.getByRole("button", { name: "取消此任务" })).toHaveCount(0);
  await page.getByRole("button", { name: "安装并核验", exact: true }).click();
  await expect(page.getByText("synthetic-task-2", { exact: true })).toBeVisible();
  await page.evaluate(() => (window as unknown as { __releaseOldCancel: () => void }).__releaseOldCancel());
  await expect(page.getByText("synthetic-task-1", { exact: true })).toHaveCount(0);
  await expect(page.getByText("synthetic-task-2", { exact: true })).toBeVisible();
  await expect(page.getByRole("button", { name: "取消此任务" })).toBeEnabled();
});

test("native kernel page waits for terminal service status and presents exact evidence and unsupported fields", async ({ page }) => {
  const errors: string[] = []; page.on("pageerror", error => errors.push(error.message));
  await page.addInitScript(() => {
    Object.defineProperty(window, "localStorage", { get() { throw new Error("demo storage forbidden"); } });
    let installed = false;
    const operation = { id: "synthetic-kernel-task", kind: "kernel-install", state: "running", stage: "probing", total: 1, completedIds: [], cancelRequested: false };
    const record = { id: "synthetic-exact-id", version: "148.0.7778.215", architecture: "amd64", source: { kind: "official", location: "https://example.test/synthetic.zip", tag: "148.0.7778.215", commit: null }, archiveSha256: "a".repeat(64), executableSha256: "b".repeat(64), installPath: "kernels/synthetic-exact-id", status: "verified", usedBy: ["synthetic-env"], report: { adapterVersion: "synthetic-adapter", version: "synthetic-capabilities", sampledAt: "2026-09-30T00:00:00Z", transport: "synthetic-test-only", sandbox: true, capabilities: [{ field: "screen/location/webgpu/tls/mac", status: "unverified", source: "not-probed", note: "尚未探测，不开放编辑。" }], observations: [{ seed: 123, cpu: 8, language: "en-US", timezone: "America/New_York", browserVersion: "148.0.7778.215", normalExit: true }] } };
    const state = { schemaVersion: 1, environments: [], proxies: [], kernels: [], backups: [], activities: [] };
    Object.assign(window, { __finishKernelTask: () => { installed = true; }, go: { main: { DesktopApp: { Call: async (request: { method: string }) => {
      if (request.method === "Workspace.Read") return { ok: true, mode: "native", data: { mode: "native", state, kernelRecords: installed ? [record] : [], kernelOperations: [] } };
      if (request.method === "Kernel.Install") return { ok: true, mode: "native", data: { status: "accepted", operation } };
      if (request.method === "Operation.Read") return { ok: true, mode: "native", data: { ...operation, state: installed ? "completed" : "running", stage: installed ? "completed" : "probing" } };
      return { ok: false, mode: "native", error: { code: "VALIDATION_FAILED", message: "synthetic", retryable: false } };
    } } } } });
  });
  await page.goto("/#/kernels");
  await expect(page.getByRole("heading", { name: "还没有已登记的真实内核" })).toBeVisible();
  await page.getByRole("button", { name: "安装并核验", exact: true }).click();
  await expect(page.getByText("任务：隔离探测实际身份和参数")).toBeVisible();
  await expect(page.getByText("真实诊断已核验", { exact: true })).toHaveCount(0);
  await page.evaluate(() => (window as unknown as { __finishKernelTask: () => void }).__finishKernelTask());
  await expect(page.getByText("真实诊断已核验", { exact: true })).toBeVisible();
  await expect(page.getByText("synthetic-exact-id", { exact: true })).toBeVisible();
  await expect(page.getByRole("button", { name: "移除此构建" })).toBeDisabled();
  await page.getByText("查看该版本能力与实测读值").click();
  await expect(page.getByText("未验证 · 不开放编辑", { exact: true })).toBeVisible();
  await page.setViewportSize({ width: 820, height: 900 });
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true);
  expect(errors).toEqual([]);
});
