import { test, expect, type Page } from "@playwright/test";
import type { Environment } from "../../src/domain.ts";
import type { DeviceProfile, EnvironmentPreview, ProfileRevision, WorkspaceView } from "../../src/application/contract.ts";
import type { NativeRequest } from "../../src/application/wails-adapter.ts";
import { nativeReferenceBridge, nativeReferenceView } from "./fixtures/native-reference-bridge.ts";

// Controlled replies at the real WailsAdapter boundary. These are UI evidence,
// never evidence of Windows processes, native persistence or kernel behaviour.
async function heldFingerprintBridge(page: Page, savedProfiles = false) {
  await nativeReferenceBridge(page);
  await page.addInitScript(savedProfiles => {
    const host = window as unknown as {
      go: { main: { DesktopApp: { Call(request: NativeRequest): Promise<unknown> } } };
      __referenceNative: { calls: NativeRequest[]; view: WorkspaceView };
      __fingerprintReplies: { pending: { method: string; previewId: string }[]; settle(index: number, outcome: "success" | "failure" | "throw"): void };
    };
    const original = host.go.main.DesktopApp.Call, previews = new Map<string, EnvironmentPreview>();
    const pending: { method: string; previewId: string }[] = [], settle: ((outcome: "success" | "failure" | "throw") => void)[] = [];
    const ok = (data: unknown) => ({ ok: true, mode: "native", data: structuredClone(data) });
    const fingerprint = (e: Environment, revision = 2): EnvironmentPreview["fingerprint"] => {
      const profile: DeviceProfile = { schemaVersion: 1, configRevision: revision, seed: e.seed, templateId: "windows-desktop-v1", templateVersion: e.fingerprintVersion, generatorVersion: "synthetic-only", platform: "windows", platformVersion: "15.0.0", brand: "Chrome", brandVersion: "148.0.7778.215", kernelId: e.coreId, coreActualVersion: "148.0.7778.215", coreExecutableSha256: "b".repeat(64), adapterVersion: "synthetic-only", capabilityVersion: "synthetic-only", language: e.language, acceptLanguages: [e.language], uiLanguage: "system", timezone: e.timezone, regionPreset: "synthetic-only", cpu: e.cpu, width: e.width, height: e.height, parameters: [], configHash: `synthetic-${e.seed}-${e.width}-${revision}` };
      return { mode: "native", action: "preview", previewProfile: profile, changes: [], capabilityReport: { kernelId: e.coreId, evidenceStatus: "synthetic-only", capabilities: [], observedFingerprint: null, canLaunchNative: false } };
    };
    host.__fingerprintReplies = { pending, settle(index, outcome) { settle[index](outcome); } };
    host.go.main.DesktopApp.Call = async request => {
      const p = request.payload as Record<string, unknown>;
      if (request.method === "Environment.Preview") {
        const result = await original(request) as { data: EnvironmentPreview };
        const draft = { ...result.data, ...(savedProfiles ? { fingerprint: fingerprint(result.data.environment) } : {}) };
        previews.set(draft.previewId, draft); return ok(draft);
      }
      if (["Fingerprint.ListRevisions", "Fingerprint.Generate", "Fingerprint.PreviewRestore"].includes(request.method)) host.__referenceNative.calls.push(structuredClone(request));
      if (request.method === "Fingerprint.ListRevisions") {
        const e = host.__referenceNative.view.state.environments.find(item => item.id === p.environmentId)!;
        const history: ProfileRevision[] = savedProfiles ? [2, 1].map(revision => ({ profile: fingerprint(e, revision)!.previewProfile, action: "edit", createdAt: "2026-10-07T00:00:00Z" })) : [];
        return ok(history);
      }
      if (["Fingerprint.Generate", "Fingerprint.PreviewRestore"].includes(request.method)) return new Promise((resolve, reject) => {
        const draft = structuredClone(previews.get(String(p.previewId))!);
        const index = pending.length;
        const environment = { ...draft.environment, ...(p.overrides as Partial<Environment>), ...(p.regenerate ? { seed: String(190000001 + index) } : {}) };
        pending.push({ method: request.method, previewId: draft.previewId });
        settle.push(outcome => {
          if (outcome === "throw") reject(new Error("synthetic delayed bridge failure"));
          else if (outcome === "failure") resolve({ ok: false, mode: "native", error: { code: "VALIDATION_FAILED", message: "合成指纹预览失败，草稿已保留", retryable: true } });
          else resolve(ok({ ...draft, environment, fingerprint: fingerprint(environment) }));
        });
      });
      return original(request);
    };
  }, savedProfiles);
}

const editor = (page: Page) => page.getByRole("dialog", { name: "编辑浏览器环境", exact: true });
const status = (page: Page) => editor(page).locator(".env34-editor-status > span");
const pendingCount = (page: Page) => page.evaluate(() => (window as unknown as { __fingerprintReplies: { pending: unknown[] } }).__fingerprintReplies.pending.length);
const settle = (page: Page, index: number, outcome: "success" | "failure" | "throw") => page.evaluate(({ index, outcome }) => (window as unknown as { __fingerprintReplies: { settle(index: number, outcome: "success" | "failure" | "throw"): void } }).__fingerprintReplies.settle(index, outcome), { index, outcome });
async function openEdit(page: Page, name: string) {
  await page.getByRole("button", { name: `${name} 更多操作`, exact: true }).click();
  await page.getByRole("button", { name: "编辑环境", exact: true }).click();
  await expect(editor(page)).toBeVisible();
}

for (const outcome of ["success", "failure", "throw"] as const) test(`closed native generation ${outcome} cannot block, unlock or overwrite a newer target`, async ({ page }) => {
  await heldFingerprintBridge(page); await page.goto("/#/environments");
  const before = (await nativeReferenceView(page)).view.state.environments;
  await openEdit(page, "工作环境 A");
  await expect.poll(() => pendingCount(page)).toBe(1);
  await expect(status(page)).toHaveText("准备指纹中…");
  await expect(editor(page).getByRole("button", { name: "保存", exact: true })).toBeDisabled();
  await editor(page).getByRole("button", { name: "取消", exact: true }).click();
  await expect(editor(page)).toHaveCount(0);
  await openEdit(page, "工作环境 B");
  await expect.poll(() => pendingCount(page)).toBe(2);
  const seed = await editor(page).getByLabel("固定指纹种子").inputValue();
  await settle(page, 0, outcome);
  await expect(status(page)).toHaveText("准备指纹中…");
  await expect(editor(page).getByRole("button", { name: "保存", exact: true })).toBeDisabled();
  await expect(editor(page).getByLabel("固定指纹种子")).toHaveValue(seed);
  await expect(editor(page).getByRole("alert")).toHaveCount(0);
  await settle(page, 1, "success");
  await expect(status(page)).toHaveText("本机保存");
  await expect(editor(page).getByRole("button", { name: "保存", exact: true })).toBeEnabled();
  await expect(editor(page).getByLabel("固定指纹种子")).toHaveValue(seed);
  await expect(editor(page).getByLabel("浏览器内核")).toHaveValue("core-148");
  await editor(page).getByText("高级设置", { exact: true }).click();
  await expect(editor(page).getByText("environments/synthetic-reference-2/user-data（本次档案修改保持不变）", { exact: true })).toBeVisible();
  const after = await nativeReferenceView(page);
  expect(after.view.state.environments).toEqual(before);
  expect(after.calls.filter(call => ["Environment.Update", "Fingerprint.CommitRevision"].includes(call.method))).toHaveLength(0);
  expect(after.calls.filter(call => call.method === "Fingerprint.Generate").map(call => (call.payload as { regenerate: boolean }).regenerate)).toEqual([false, false]);
});

test("native failed generation preserves the draft and retries the same seed without regeneration", async ({ page }) => {
  await heldFingerprintBridge(page); await page.goto("/#/environments");
  await openEdit(page, "工作环境 A"); await expect.poll(() => pendingCount(page)).toBe(1);
  const seed = await editor(page).getByLabel("固定指纹种子").inputValue();
  await editor(page).getByLabel("环境名称", { exact: true }).fill("失败后仍保留的合成草稿");
  await settle(page, 0, "failure");
  await expect(status(page)).toHaveText("指纹预览失败");
  await expect(editor(page).getByRole("button", { name: "保存", exact: true })).toBeDisabled();
  await expect(editor(page).getByLabel("环境名称", { exact: true })).toHaveValue("失败后仍保留的合成草稿");
  await editor(page).getByRole("button", { name: "重试指纹预览", exact: true }).click();
  await expect.poll(() => pendingCount(page)).toBe(2); await settle(page, 1, "success");
  await expect(status(page)).toHaveText("本机保存");
  await expect(editor(page).getByLabel("固定指纹种子")).toHaveValue(seed);
  await expect(editor(page).getByLabel("环境名称", { exact: true })).toHaveValue("失败后仍保留的合成草稿");
  const requests = (await nativeReferenceView(page)).calls.filter(call => call.method === "Fingerprint.Generate");
  expect(requests.map(call => (call.payload as { regenerate: boolean }).regenerate)).toEqual([false, false]);
  // Native seed belongs to the service preview; only supported input overrides
  // cross this boundary. Metadata edits do not change the generation request.
  expect(requests[1].payload).toEqual(requests[0].payload);
  expect(requests[1].payload).toMatchObject({ previewId: "synthetic-preview-1", kernelId: "core-148" });
});

test("closed native rollback cannot unlock a newer regeneration and reopening keeps saved identity", async ({ page }) => {
  await heldFingerprintBridge(page, true); await page.goto("/#/environments");
  const before = (await nativeReferenceView(page)).view.state.environments;
  await openEdit(page, "工作环境 A");
  await editor(page).getByText("高级设置", { exact: true }).click();
  await editor(page).getByText("已保存的档案历史（2）", { exact: true }).click();
  await editor(page).getByRole("button", { name: "预览回滚到 #1", exact: true }).click();
  await expect.poll(() => pendingCount(page)).toBe(1);
  await editor(page).getByRole("button", { name: "取消", exact: true }).click();
  await openEdit(page, "工作环境 B");
  const seed = await editor(page).getByLabel("固定指纹种子").inputValue();
  await editor(page).getByRole("button", { name: "换一套", exact: true }).click();
  await expect.poll(() => pendingCount(page)).toBe(2);
  await settle(page, 0, "success");
  await expect(status(page)).toHaveText("准备指纹中…");
  await expect(editor(page).getByRole("button", { name: "保存", exact: true })).toBeDisabled();
  await expect(editor(page).getByLabel("固定指纹种子")).toHaveValue(seed);
  await editor(page).getByRole("button", { name: "取消", exact: true }).click();
  await settle(page, 1, "success");
  await openEdit(page, "工作环境 B");
  await expect(status(page)).toHaveText("本机保存");
  await expect(editor(page).getByLabel("固定指纹种子")).toHaveValue(seed);
  await expect(editor(page).getByLabel("浏览器内核")).toHaveValue("core-148");
  await editor(page).getByText("高级设置", { exact: true }).click();
  await expect(editor(page).getByText("environments/synthetic-reference-2/user-data（本次档案修改保持不变）", { exact: true })).toBeVisible();
  expect((await nativeReferenceView(page)).view.state.environments).toEqual(before);
});
