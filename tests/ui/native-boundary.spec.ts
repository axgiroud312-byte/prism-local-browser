import { test, expect, type Page } from "@playwright/test";
import { seedState, type Environment } from "../../src/domain.ts";
import type { DeviceProfile, EnvironmentPreview, Operation, RuntimeSession, WorkspaceView } from "../../src/application/contract.ts";
import type { NativeRequest } from "../../src/application/wails-adapter.ts";

// This bridge is deliberately synthetic. It checks UI -> existing WailsAdapter
// requests, not Windows processes, SQLite, installed kernels or proxy traffic.
async function environmentBridge(page: Page, options: { failStart?: boolean; noKernels?: boolean } = {}) {
  const template = { ...seedState().environments[7], id: "", name: "", group: "", coreId: "fixture-build-a", seed: "172600001", status: "ready" as const };
  await page.addInitScript(({ template, options }) => {
    Object.defineProperty(window, "localStorage", { get() { throw new Error("native fixture must not read demo storage"); } });
    const key = "prism-native-synthetic-ui";
    const kernels = options.noKernels ? [] : [
      { id: "fixture-build-a", version: "148.0.7778.215" },
      { id: "fixture-build-b", version: "151.0.9000.11" },
    ];
    const restored = sessionStorage.getItem(key);
    const view: WorkspaceView = restored ? JSON.parse(restored) : {
      mode: "native", state: { schemaVersion: 1, environments: [], proxies: [{ id: "fixture-proxy", name: "合成代理", type: "socks5", host: "192.0.2.88", port: 1080, country: "US", username: "", password: "", status: "connected" }], kernels: kernels.map(k => ({ ...k, source: "fingerprint-chromium", available: true, note: "合成桥，不是安装证据" })), backups: [], activities: [] },
      kernelRecords: kernels.map(k => ({ ...k, architecture: "amd64", source: { kind: "official", location: "https://example.test/fixture.zip", tag: k.version, commit: null }, archiveSha256: "a".repeat(64), executableSha256: "b".repeat(64), executableRelativePath: "chrome.exe", installPath: `kernels/${k.id}`, installedAt: "2026-10-06T00:00:00Z", status: "verified", usedBy: [], report: { adapterVersion: "synthetic-only", version: "synthetic-only", sampledAt: "2026-10-06T00:00:00Z", transport: "synthetic-only", sandbox: true, observations: [], capabilities: [] } })),
      nativeProxyRecords: [{ id: "fixture-proxy", name: "合成代理", type: "socks5", host: "192.0.2.88", port: 1080, country: "US", status: "connected", hasAuthentication: false, revision: 1, usedBy: [] }],
      fingerprints: {}, runtimeSessions: {},
    };
    const revisions: Record<string, number> = JSON.parse(sessionStorage.getItem(`${key}-revisions`) ?? "null") ?? Object.fromEntries(view.state.environments.map(e => [e.id, 1]));
    const previews = new Map<string, EnvironmentPreview>();
    const operations = new Map<string, Operation>();
    const calls: NativeRequest[] = [];
    let serial = 0, failStart = !!options.failStart;
    let finishFailedStart: (() => void) | undefined;
    const save = () => { sessionStorage.setItem(key, JSON.stringify(view)); sessionStorage.setItem(`${key}-revisions`, JSON.stringify(revisions)); };
    const ok = (data: unknown) => ({ ok: true, mode: "native", data: structuredClone(data) });
    function profile(e: Environment): DeviceProfile {
      const version = kernels.find(k => k.id === e.coreId)?.version ?? "";
      return { schemaVersion: 1, configRevision: view.fingerprints?.[e.id]?.profile.configRevision ?? 1, seed: e.seed, templateId: "windows-desktop-v1", templateVersion: e.fingerprintVersion, generatorVersion: "synthetic-only", platform: "windows", platformVersion: "15.0.0", brand: "Chrome", brandVersion: version, kernelId: e.coreId, coreActualVersion: version, coreExecutableSha256: "b".repeat(64), adapterVersion: "synthetic-only", capabilityVersion: "synthetic-only", language: e.language, acceptLanguages: [e.language], uiLanguage: "system", timezone: e.timezone, regionPreset: "synthetic-only", cpu: e.cpu, width: e.width, height: e.height, parameters: [], configHash: `${e.seed}-${e.coreId}-${e.width}` };
    }
    function preview(e: Environment, previewId: string): EnvironmentPreview {
      return { previewId, environment: { ...e }, expectedRevision: e.id ? revisions[e.id] : undefined, fingerprint: kernels.length ? { mode: "native", previewProfile: profile(e), action: "preview", changes: [], capabilityReport: { kernelId: e.coreId, evidenceStatus: "synthetic-only", capabilities: [], observedFingerprint: null, canLaunchNative: false } } : undefined, userDataRef: e.id ? `environments/${e.id}/user-data` : undefined };
    }
    function operation(kind: Operation["kind"], id: string): Operation {
      const task: Operation = { id: `fixture-operation-${++serial}`, kind, state: "completed", total: 1, completedIds: [id], cancelRequested: false, environmentId: id };
      operations.set(task.id, task); return task;
    }
    Object.assign(window, { __nativeUI: { calls, view, failNextProxyStart: () => { failStart = true; }, finishFailedStart: () => { finishFailedStart?.(); finishFailedStart = undefined; } }, go: { main: { DesktopApp: { Call: async (request: NativeRequest) => {
      calls.push(structuredClone(request));
      const p = request.payload as Record<string, unknown>;
      if (request.method === "Workspace.Read") {
        const query = p.environmentQuery as { page: number; pageSize: number; group: string; status: string; search: string } | undefined;
        const all = view.state.environments;
        const filtered = all.filter(e => !query || (!query.search || `${e.name} ${e.code} ${e.note}`.includes(query.search)) && (!query.group || e.group === query.group) && (query.status === "all" || e.status === query.status));
        const page = query?.page ?? 1, pageSize = query?.pageSize ?? 8;
        return ok({ ...view, state: { ...view.state, environments: filtered.slice((page-1)*pageSize, page*pageSize) }, environmentPage: { page, pageSize, total: all.length, filteredTotal: filtered.length, groups: [...new Set(all.map(e => e.group).filter(Boolean))], runningCount: all.filter(e => e.status === "running").length, errorCount: all.filter(e => e.status === "error").length } });
      }
      if (request.method === "Environment.Preview") {
        const e = p.kind === "edit" ? view.state.environments.find(e => e.id === p.sourceId)! : { ...template, seed: String(172600001 + ++serial), coreId: kernels[0]?.id ?? "kernel-pending" };
        const draft = preview(e, `fixture-preview-${++serial}`); previews.set(draft.previewId, draft); return ok(draft);
      }
      if (request.method === "Preview.Discard") { previews.delete(String(p.previewId)); return ok({ status: "discarded" }); }
      if (request.method === "Fingerprint.ListRevisions") return ok(view.fingerprints?.[String(p.environmentId)] ? [view.fingerprints[String(p.environmentId)]] : []);
      if (request.method === "Fingerprint.Generate") {
        const old = previews.get(String(p.previewId))!;
        const e = { ...old.environment, ...(p.overrides as Partial<Environment>), coreId: String(p.kernelId), seed: p.regenerate ? String(172600001 + ++serial) : old.environment.seed };
        const next = preview(e, old.previewId); previews.set(next.previewId, next); return ok(next);
      }
      if (request.method === "Environment.Create") {
        const id = `fixture-environment-${view.state.environments.length+1}`;
        const e = { ...template, ...(p.configuration as Partial<Environment>), id, code: String(view.state.environments.length+1).padStart(3, "0") };
        view.state.environments.push(e); revisions[id] = 1;
        view.fingerprints![id] = { profile: profile(e), action: "create", createdAt: "2026-10-06T00:00:00Z" }; save();
        return ok({ status: "accepted", operation: operation("create", id) });
      }
      if (request.method === "Environment.Update" || request.method === "Fingerprint.CommitRevision") {
        const draft = previews.get(String(p.previewId))!;
        const e = view.state.environments.find(e => e.id === draft.environment.id)!;
        Object.assign(e, p.configuration); revisions[e.id]++;
        view.fingerprints![e.id] = { profile: profile(e), action: "edit", createdAt: "2026-10-06T00:00:00Z" }; save();
        return ok({ status: "completed", environment: { record: e, revision: revisions[e.id] }, newRevision: revisions[e.id], fingerprintRevision: revisions[e.id] });
      }
      if (request.method === "Operation.Read") return ok(operations.get(String(p.operationId)));
      if (request.method === "Runtime.Inspect") return ok(Object.values(view.runtimeSessions!));
      if (request.method === "Runtime.Start" || request.method === "Runtime.Stop") {
        const e = view.state.environments.find(e => e.id === p.environmentId)!;
        const start = request.method === "Runtime.Start";
        const failed = start && !!e.proxyId && failStart; if (failed) failStart = false;
        e.status = failed ? "starting" : start ? "running" : "ready";
        e.error = undefined;
        const task = operation(start ? "runtime-start" : "runtime-stop", e.id);
        const session: RuntimeSession = { mode: "native", environmentId: e.id, sessionId: `fixture-session-${e.id}`, operationId: task.id, state: e.status, revision: revisions[e.id], fingerprintRevision: revisions[e.id], kernelId: e.coreId, userDataRef: `environments/${e.id}/user-data`, networkPolicy: e.proxyId ? "proxy" : "direct", canControl: !failed && start, canForce: false, needsReconcile: false, persistencePending: false, resourcesPending: false, ...(e.proxyId ? { proxyId: e.proxyId, proxyRevision: 1 } : {}) };
        if (failed) {
          task.state = "accepted"; task.completedIds = [];
          finishFailedStart = () => {
            const error = { code: "PROXY_AUTH_FAILED", message: "合成代理认证失败；已保存环境，可以修复后重试打开。", retryable: true };
            e.status = "error"; e.error = error.message; session.state = "error"; session.error = error;
            task.state = "failed"; task.error = error; save();
          };
        }
        view.runtimeSessions![e.id] = session; save();
        return ok({ status: "accepted", operation: task });
      }
      return { ok: false, mode: "native", error: { code: "CAPABILITY_UNSUPPORTED", message: `Unimplemented synthetic fixture: ${request.method}`, retryable: false } };
    } } } } });
  }, { template, options });
}

const nativeView = (page: Page) => page.evaluate(() => (window as unknown as { __nativeUI: { calls: NativeRequest[]; view: WorkspaceView } }).__nativeUI);

test("injected native bridge creates once, retries failed opening and preserves identity through save and reload", async ({ page }) => {
  await environmentBridge(page, { failStart: true });
  const confirmations: string[] = [];
  page.on("dialog", async d => { confirmations.push(d.message()); await d.accept(); });
  await page.goto("/#/environments");
  await page.getByRole("button", { name: "新建环境", exact: true }).click();
  await page.getByLabel("环境名称", { exact: true }).fill("桥接合成环境");
  await page.getByLabel("分组", { exact: true }).fill("桥接合成分组");
  await page.getByLabel("浏览器内核").selectOption("fixture-build-b");
  await page.getByLabel("绑定代理").selectOption("fixture-proxy");
  await page.getByRole("button", { name: "换一套", exact: true }).click();
  const seed = await page.getByLabel("固定指纹种子").inputValue();
  await page.getByRole("button", { name: "创建并打开", exact: true }).click();
  await expect.poll(async () => (await nativeView(page)).calls.filter(c => c.method === "Runtime.Start").length).toBe(1);
  await page.evaluate(() => (window as unknown as { __nativeUI: { finishFailedStart: () => void } }).__nativeUI.finishFailedStart());
  await expect(page.getByRole("dialog")).toHaveCount(0);
  const row = page.getByRole("row").filter({ hasText: "桥接合成环境" });
  await expect(row).toContainText("需处理");
  await expect(page.getByText(/合成代理认证失败/).first()).toBeVisible();
  let data = await nativeView(page);
  expect(data.view.state.environments).toHaveLength(1);
  expect(data.calls.filter(c => c.method === "Environment.Create")).toHaveLength(1);
  expect(data.calls.some(c => c.method === "Batch.Preview")).toBe(false);
  expect(data.calls.filter(c => c.method === "Runtime.Start")[0].payload).toMatchObject({ networkPolicy: "proxy", expectedRevision: 1 });
  await row.getByRole("button", { name: /打开|重试/, exact: true }).click();
  await expect(row).toContainText("运行中");
  data = await nativeView(page);
  expect(data.calls.filter(c => c.method === "Environment.Create")).toHaveLength(1);
  expect(data.calls.filter(c => c.method === "Runtime.Start").map(c => (c.payload as { networkPolicy: string }).networkPolicy)).toEqual(["proxy", "proxy"]);
  expect(data.view.state.environments[0].seed).toBe(seed);
  await row.getByRole("button", { name: "关闭", exact: true }).click();
  await expect(row).toContainText("待启动");
  await page.getByRole("button", { name: "桥接合成环境 更多操作", exact: true }).click();
  await page.getByRole("button", { name: "编辑环境", exact: true }).click();
  await page.getByLabel("环境名称", { exact: true }).fill("桥接保存合成环境");
  await page.getByRole("button", { name: "保存", exact: true }).click();
  await expect(page.getByRole("dialog")).toHaveCount(0);
  await page.reload();
  data = await nativeView(page);
  expect(data.view.state.environments[0]).toMatchObject({ name: "桥接保存合成环境", seed, coreId: "fixture-build-b", proxyId: "fixture-proxy", group: "桥接合成分组" });
  expect(confirmations).toEqual([]);
  await expect(page.getByText("北美主店", { exact: true })).toHaveCount(0);
  await page.screenshot({ path: "output/playwright/native-synthetic-flow.png", fullPage: true });
});

test("injected native bridge directly creates and opens explicit direct policy without technical confirmation", async ({ page }) => {
  await environmentBridge(page);
  const confirmations: string[] = [];
  page.on("dialog", async d => { confirmations.push(d.message()); await d.dismiss(); });
  await page.goto("/#/environments");
  await page.getByRole("button", { name: "新建环境", exact: true }).click();
  await page.getByLabel("环境名称", { exact: true }).fill("直连桥接样本");
  await page.getByRole("button", { name: "创建并打开", exact: true }).click();
  const row = page.getByRole("row").filter({ hasText: "直连桥接样本" });
  await expect(row).toContainText("运行中");
  await expect(row).toContainText("直连");
  const data = await nativeView(page);
  expect(data.calls.filter(c => c.method === "Runtime.Start")[0].payload).toMatchObject({ networkPolicy: "direct", expectedRevision: 1 });
  expect(data.calls.filter(c => c.method === "Environment.Create")).toHaveLength(1);
  expect(confirmations).toEqual([]);
  await page.getByRole("button", { name: "直连桥接样本 更多操作", exact: true }).click();
  await page.getByRole("button", { name: "编辑环境", exact: true }).click();
  await expect(page.getByLabel("环境名称", { exact: true })).toBeEnabled();
  await expect(page.getByLabel("绑定代理")).toBeDisabled();
  await expect(page.getByRole("button", { name: "换一套", exact: true })).toBeDisabled();
  await page.keyboard.press("Escape");
  await expect(page.getByRole("dialog")).toHaveCount(0);
  await row.getByRole("button", { name: "关闭", exact: true }).click();
  await expect(row).toContainText("待启动");
  await page.getByRole("button", { name: "新建环境", exact: true }).click();
  await page.getByLabel("环境名称", { exact: true }).fill("批量桥接代理样本");
  await page.getByLabel("绑定代理").selectOption("fixture-proxy");
  await page.getByRole("button", { name: "创建", exact: true }).click();
  await expect(page.getByRole("dialog")).toHaveCount(0);
  await page.getByLabel("选择 直连桥接样本", { exact: true }).check();
  await page.getByLabel("选择 批量桥接代理样本", { exact: true }).check();
  await page.evaluate(() => (window as unknown as { __nativeUI: { failNextProxyStart: () => void } }).__nativeUI.failNextProxyStart());
  await page.getByRole("button", { name: "批量打开", exact: true }).click();
  await expect.poll(async () => (await nativeView(page)).calls.filter(c => c.method === "Runtime.Start").length).toBe(3);
  await page.evaluate(() => (window as unknown as { __nativeUI: { finishFailedStart: () => void } }).__nativeUI.finishFailedStart());
  await expect(row).toContainText("运行中");
  await expect(page.getByRole("row").filter({ hasText: "批量桥接代理样本" })).toContainText("需处理");
  const batch = await nativeView(page);
  expect(batch.calls.filter(c => c.method === "Runtime.Start").map(c => (c.payload as { networkPolicy: string }).networkPolicy)).toEqual(["direct", "direct", "proxy"]);
  expect(batch.view.state.environments).toHaveLength(2);
  expect(confirmations).toEqual([]);
  await page.getByRole("button", { name: "批量关闭", exact: true }).click();
  await expect(row).toContainText("待启动");
});

test("injected native bridge with no usable kernel guides preparation instead of simulated creation", async ({ page }) => {
  await environmentBridge(page, { noKernels: true });
  await page.goto("/#/environments");
  await page.getByRole("button", { name: "新建环境", exact: true }).click();
  await expect(page.getByRole("dialog")).toContainText(/没有.*内核|尚无.*内核|还没有.*内核/);
  await expect(page.getByRole("button", { name: "创建并打开", exact: true })).toBeDisabled();
  expect((await nativeView(page)).calls.filter(c => c.method === "Environment.Create")).toHaveLength(0);
});

test("injected native invalid quantities keep the draft editable without submitting an unknown create", async ({ page }) => {
  await environmentBridge(page);
  await page.goto("/#/environments");
  await page.getByRole("button", { name: "新建环境", exact: true }).click();
  await page.getByLabel("环境名称", { exact: true }).fill("数量恢复合成样本");
  const editor = page.getByRole("dialog", { name: "新建浏览器环境" });
  await editor.locator("details").first().locator(":scope > summary").click();
  for (const invalid of ["0", "-1", "", "0.5", "1.5", "9007199254740992"]) {
    await page.getByLabel("创建数量").fill(invalid);
    await editor.getByRole("button", { name: Number(invalid) > 1 ? `查看 ${Number(invalid)} 项创建计划` : "创建", exact: true }).click();
    await expect(editor.getByRole("alert")).toContainText("正整数");
    await expect(page.getByLabel("创建数量")).toBeEnabled();
    await expect(page.getByLabel("环境名称", { exact: true })).toHaveValue("数量恢复合成样本");
    expect((await nativeView(page)).calls.filter(c => ["Environment.Create", "Batch.Preview"].includes(c.method))).toHaveLength(0);
  }
  await page.getByLabel("创建数量").fill("1");
  await page.getByRole("button", { name: "创建", exact: true }).click();
  await expect(editor).toHaveCount(0);
  expect((await nativeView(page)).view.state.environments).toHaveLength(1);
});

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
