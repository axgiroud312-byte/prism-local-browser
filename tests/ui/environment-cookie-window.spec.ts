import { test, expect, type Page } from "@playwright/test";
import { seedState } from "../../src/domain.ts";
import type { CookieCommitRequest, CookieImportPreview, Operation, RuntimeSession, WorkspaceView } from "../../src/application/contract.ts";

// Narrow, in-memory bridge: only Cookie parsing/committing and safe observations.
// It never installs a kernel, starts a process, or writes a real Cookie store.
async function cookieBridge(page: Page, outcome: "partial" | "unknown-receipt" | "refused" = "partial") {
  const environment = { ...seedState().environments[7], id: "synthetic-cookie-environment", name: "合成Cookie环境", code: "001", seed: "170600001", coreId: "synthetic-cookie-build", proxyId: "", status: "running" as const, cookies: [] };
  await page.addInitScript(({ environment, outcome }) => {
    const copy = <T,>(value: T): T => JSON.parse(JSON.stringify(value));
    const calls: Array<{ method: string; payload: Record<string, unknown> }> = [];
    const session: RuntimeSession = { mode: "native", environmentId: environment.id, sessionId: "synthetic-cookie-session", operationId: "synthetic-cookie-session-owner", state: "running", revision: 1, fingerprintRevision: 1, kernelId: environment.coreId, userDataRef: `environments/${environment.id}/user-data`, networkPolicy: "direct", canControl: true, canForce: false, needsReconcile: false, persistencePending: false, resourcesPending: true };
    const workspace: WorkspaceView = { mode: "native", state: { schemaVersion: 1, environments: [environment], proxies: [], kernels: [], backups: [], activities: [] }, kernelRecords: [], runtimeSessions: { [environment.id]: session }, networkResources: { [environment.id]: "synthetic-cookie-owned" }, cookieOperations: [] };
    let preview: CookieImportPreview | undefined, serial = 0, unknownSent = false;
    const receipts = new Map<string, { request: CookieCommitRequest; operation: Operation }>();
    const ok = (data: unknown) => ({ ok: true, mode: "native", data: copy(data) });
    const fail = (code: string) => ({ ok: false, mode: "native", error: { code, message: "合成桥：未确认受理；保留原请求。", retryable: true } });
    const bridge = { calls, workspace };
    Object.defineProperty(window, "localStorage", { get() { throw new Error("Native Cookie test must not use demo storage"); } });
    Object.defineProperty(window, "cookieWindowFixture", { value: bridge });
    Object.defineProperty(window, "go", { value: { main: { DesktopApp: { Call: async (raw: { method: string; payload: Record<string, unknown> }) => {
      const request = copy(raw), p = request.payload; calls.push(request);
      if (request.method === "Workspace.Read") return ok(workspace);
      if (request.method === "Cookie.DiscardImport") return ok({ status: "discarded" });
      if (request.method === "Cookie.ParseImport") {
        if (p.environmentId !== environment.id) return fail("NOT_FOUND");
        const rows = JSON.parse(String(p.text)).map(({ value: _value, ...safe }: { value: string; name: string; domain: string; path: string }, index: number) => ({ ...safe, index: index + 1, hostOnly: true, session: true, secure: false, httpOnly: false, expired: false, conflict: false, existingConflict: false }));
        preview = { mode: "native", previewId: "synthetic-cookie-preview", environmentId: environment.id, environmentName: environment.name, expectedRevision: 1, sessionId: session.sessionId, requiresStart: false, format: "json", expiresAt: "2030-01-01T00:00:00Z", rows, total: rows.length, validCount: rows.length, errorCount: 0, expiredCount: 0, conflictCount: 0 };
        return ok(preview);
      }
      if (request.method === "Cookie.CommitImport") {
        const commit = p as unknown as CookieCommitRequest;
        const prior = receipts.get(commit.requestId);
        if (prior) return JSON.stringify(prior.request) === JSON.stringify(commit) ? ok({ status: "accepted", operation: prior.operation }) : fail("REQUEST_ID_REUSED");
        if (!preview || commit.previewId !== preview.previewId || commit.environmentId !== environment.id || commit.sessionId !== session.sessionId || commit.expectedRevision !== 1 || !commit.selectedRows.length) return fail("COOKIE_SESSION_CHANGED");
        if (outcome === "refused") return fail("REVISION_CONFLICT");
        const partial = outcome === "partial" && serial === 0;
        const items = commit.selectedRows.map((index, position) => ({ ...preview!.rows.find(row => row.index === index)!, status: partial ? position === 0 ? "verified" as const : "failed" as const : "verified" as const }));
        const verified = items.filter(item => item.status === "verified").length;
        const operation: Operation = { id: `synthetic-cookie-operation-${++serial}`, kind: "cookie-import", state: partial ? "failed" : "completed", total: items.length, completedIds: partial ? [] : [environment.id], cancelRequested: false, persistencePending: false, environmentId: environment.id, sessionId: session.sessionId, cookieReport: { mode: "native", previewId: preview.previewId, environmentId: environment.id, sessionId: session.sessionId, revision: 1, policy: commit.policy, clearState: commit.policy === "replace-all" ? "verified-empty" : "not-requested", verifiedCount: verified, writtenCount: verified, alreadyMatchedCount: 0, failedCount: items.length - verified, skippedCount: 0, unconfirmedCount: 0, items } };
        receipts.set(commit.requestId, { request: copy(commit), operation });
        // Unknown receipt deliberately publishes no observation until exact replay.
        if (outcome === "unknown-receipt" && !unknownSent) { unknownSent = true; return fail("NATIVE_UNAVAILABLE"); }
        workspace.cookieOperations = [operation, ...(workspace.cookieOperations ?? [])];
        return ok({ status: "accepted", operation });
      }
      if (request.method === "Operation.Read") {
        const operation = [...receipts.values()].find(receipt => receipt.operation.id === p.operationId)?.operation;
        return operation ? ok(operation) : fail("NOT_FOUND");
      }
      return fail("CAPABILITY_UNSUPPORTED");
    } } } } });
  }, { environment, outcome });
  await page.goto("/#/environments");
  await page.getByRole("button", { name: "合成Cookie环境 更多操作", exact: true }).click();
  await page.getByRole("button", { name: "导入 Cookie", exact: true }).click();
}

const input = JSON.stringify([{ name: "sample", value: "SYNTHETIC_ONLY", domain: "example.invalid", path: "/" }, { name: "empty", value: "", domain: "example.invalid", path: "/" }]);
async function parse(page: Page) {
  await page.getByLabel("Cookie 内容", { exact: true }).fill(input);
  await page.getByRole("button", { name: "解析预览（不写入）", exact: true }).click();
  await expect(page.getByLabel("选择第2行", { exact: true })).toBeChecked();
  await expect(page.getByLabel("Cookie 内容", { exact: true })).toHaveValue("");
  await page.getByRole("radio", { name: "先清空本环境全部Cookie", exact: true }).check();
  await page.getByLabel(/^确认删除此环境全部Cookie/).check();
}
const cookieCalls = (page: Page) => page.evaluate(() => (window as unknown as { cookieWindowFixture: { calls: Array<{ method: string; payload: CookieCommitRequest }> } }).cookieWindowFixture.calls.filter(call => call.method === "Cookie.CommitImport"));

test("environment Cookie windows: failed subset retry is merge-only for the original target", async ({ page }) => {
  await cookieBridge(page); await parse(page);
  await page.getByRole("button", { name: "确认导入 2 条并读回核对", exact: true }).click();
  await page.getByRole("button", { name: "只重试所选未核对通过项（不重清、不改其他键）", exact: true }).click();
  await expect(page.getByRole("heading", { name: /全部核对通过/ })).toBeVisible();
  const committed = await cookieCalls(page);
  expect(committed).toHaveLength(2);
  expect(committed[0].payload).toMatchObject({ policy: "replace-all", selectedRows: [1, 2] });
  expect(committed[1].payload).toMatchObject({ policy: "merge", selectedRows: [2], previewId: committed[0].payload.previewId, environmentId: committed[0].payload.environmentId, sessionId: committed[0].payload.sessionId, expectedRevision: committed[0].payload.expectedRevision });
  expect(committed[1].payload.requestId).not.toBe(committed[0].payload.requestId);
});

test("environment Cookie windows: unknown receipt locks closing and replays the exact original request", async ({ page }) => {
  await cookieBridge(page, "unknown-receipt"); await parse(page);
  await page.getByRole("button", { name: "确认导入 2 条并读回核对", exact: true }).click();
  const verify = page.getByRole("button", { name: "核实原导入请求（不改变范围或清空策略）", exact: true });
  await expect(verify).toBeEnabled();
  await expect(page.getByLabel("Cookie 内容", { exact: true })).toBeDisabled();
  await expect(page.getByLabel("选择第1行", { exact: true })).toBeDisabled();
  await expect(page.getByRole("button", { name: "关闭Cookie导入", exact: true })).toBeDisabled();
  await page.keyboard.press("Escape");
  await expect(page.getByRole("dialog", { name: "导入真实环境 Cookie" })).toBeVisible();
  await verify.click(); await expect(verify).toHaveCount(0);
  const committed = await cookieCalls(page);
  expect(committed).toHaveLength(2); expect(committed[1]).toEqual(committed[0]);
  await page.getByRole("button", { name: "关闭Cookie导入", exact: true }).click();
  await expect(page.getByRole("dialog")).toHaveCount(0);
});

test("environment Cookie windows: file picker Escape retains draft and returns focus", async ({ page }) => {
  await cookieBridge(page);
  await page.getByLabel("Cookie 内容", { exact: true }).fill(input);
  await page.getByRole("button", { name: "选择文件", exact: true }).click();
  await expect(page.getByRole("dialog", { name: "读取Cookie文件" })).toBeVisible();
  await page.keyboard.press("Escape");
  await expect(page.getByLabel("Cookie 内容", { exact: true })).toHaveValue(input);
  await expect(page.getByRole("button", { name: "选择文件", exact: true })).toBeFocused();
  expect(await cookieCalls(page)).toHaveLength(0);
});

test("environment Cookie windows: definite preflight refusal does not trap the user in unknown recovery", async ({ page }) => {
  await cookieBridge(page, "refused"); await parse(page);
  await page.getByRole("button", { name: "确认导入 2 条并读回核对", exact: true }).click();
  await expect(page.getByRole("alert")).toContainText("REVISION_CONFLICT");
  await expect(page.getByLabel("Cookie 内容", { exact: true })).toBeEnabled();
  await expect(page.getByRole("button", { name: "核实原导入请求（不改变范围或清空策略）", exact: true })).toHaveCount(0);
  await page.keyboard.press("Escape");
  await expect(page.getByRole("dialog")).toHaveCount(0);
  expect(await cookieCalls(page)).toHaveLength(1);
});
