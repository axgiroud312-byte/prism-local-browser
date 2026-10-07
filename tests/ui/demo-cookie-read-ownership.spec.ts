import { test, expect, type Page } from "@playwright/test";
import { parseCookies, seedState, STORAGE_KEY, type State } from "../../src/domain.ts";

const fileName = "synthetic-cookie-owner.json";
const readFailure = "读取文件失败，请重新选择文件或手动粘贴 Cookie 内容。原草稿已保留。";
const sourceFailure = "SYNTHETIC_SOURCE_PAYLOAD_MUST_NOT_BE_ECHOED: synthetic-private-source.json";
const cookieText = (name: string) => JSON.stringify([{ name, value: "SYNTHETIC_ONLY", domain: "example.invalid", path: "/" }]);
const initial = seedState();
initial.environments = initial.environments.slice(0, 2).map((environment, index) => ({
  ...environment, id: `synthetic-cookie-owner-${index}`, name: `合成Cookie目标${index === 0 ? "A" : "B"}`, status: "ready", cookies: [],
}));
initial.backups = [];
initial.activities = [];

type ReadFixture = {
  reads: Array<{ name: string; resolve: (text: string) => void; reject: (error: Error) => void }>;
  unhandled: string[];
};
type HarnessState = { environment: State["environments"][number]; text: string; result: ReturnType<typeof parseCookies> | null; error: string; busy: boolean; mounted: boolean };
type HarnessFixture = { patch: (patch: Partial<HarnessState>) => void; calls: Array<{ target: string; text: string }>; saves: number };

test.beforeEach(async ({ page, baseURL }) => {
  const origin = new URL(baseURL!).origin;
  const errors: string[] = [], externalRequests: string[] = [];
  page.on("pageerror", error => errors.push(error.message));
  await page.route("**/*", route => {
    if (new URL(route.request().url()).origin === origin) return route.continue();
    externalRequests.push(route.request().url());
    return route.abort("blockedbyclient");
  });
  await page.routeWebSocket("**/*", socket => socket.close());
  await page.addInitScript(({ initial, key, fileName }) => {
    localStorage.setItem(key, JSON.stringify(initial));
    const fixture: ReadFixture = { reads: [], unhandled: [] };
    Object.defineProperty(window, "cookieReadFixture", { value: fixture });
    // Only the in-memory chooser payload is intercepted; no file paths or host bridge.
    const original = File.prototype.text;
    File.prototype.text = function () {
      if (this.name !== fileName) return original.call(this);
      return new Promise<string>((resolve, reject) => fixture.reads.push({ name: this.name, resolve, reject }));
    };
    window.addEventListener("unhandledrejection", event => fixture.unhandled.push(String(event.reason)));
  }, { initial, key: STORAGE_KEY, fileName });
  await page.goto("/#/environments");
  await expect(page.getByRole("button", { name: "合成Cookie目标A 更多操作", exact: true })).toBeVisible();
  (page as Page & { cookieAudit: { errors: string[]; externalRequests: string[] } }).cookieAudit = { errors, externalRequests };
});

test.afterEach(async ({ page }) => {
  const audit = (page as Page & { cookieAudit: { errors: string[]; externalRequests: string[] } }).cookieAudit;
  expect(audit.errors, "no uncaught file-read or detached React event errors").toEqual([]);
  expect(audit.externalRequests, "only the private Vite origin is allowed").toEqual([]);
  expect(await page.evaluate(() => (window as unknown as { cookieReadFixture: ReadFixture }).cookieReadFixture.unhandled), "no unhandled read rejection").toEqual([]);
});

const dialog = (page: Page) => page.getByRole("dialog", { name: "导入 Cookie", exact: true });
const input = (page: Page) => dialog(page).getByLabel("Cookie 内容", { exact: true });
const save = (page: Page) => dialog(page).getByRole("button", { name: "导入到原型记录", exact: true });
async function openTarget(page: Page, target: "A" | "B") {
  await page.getByRole("button", { name: `合成Cookie目标${target} 更多操作`, exact: true }).click();
  await page.getByRole("button", { name: "导入 Cookie", exact: true }).click();
  await expect(dialog(page)).toContainText(`目标：合成Cookie目标${target}`);
}
async function preview(page: Page, text: string) {
  await input(page).fill(text);
  await dialog(page).getByRole("button", { name: "校验并预览", exact: true }).click();
  await expect(dialog(page)).toContainText("校验结果 · 1 条有效 / 0 条错误");
  await expect(save(page)).toBeEnabled();
}
async function selectFile(page: Page) {
  const index = await page.evaluate(() => (window as unknown as { cookieReadFixture: ReadFixture }).cookieReadFixture.reads.length);
  const chooser = page.waitForEvent("filechooser");
  await dialog(page).getByRole("button", { name: "选择文件", exact: true }).click();
  await (await chooser).setFiles({ name: fileName, mimeType: "application/json", buffer: Buffer.from("SYNTHETIC_CONTROLLED_READ") });
  await expect.poll(() => page.evaluate(() => (window as unknown as { cookieReadFixture: ReadFixture }).cookieReadFixture.reads.length)).toBe(index + 1);
  return index;
}
async function settle(page: Page, index: number, outcome: "resolve" | "reject", text = sourceFailure) {
  await page.evaluate(async ({ index, outcome, text }) => {
    const read = (window as unknown as { cookieReadFixture: ReadFixture }).cookieReadFixture.reads[index];
    if (outcome === "resolve") read.resolve(text); else read.reject(new Error(text));
    // Cross a task and paint boundary so assertions cannot pass before React applies a late read.
    await new Promise<void>(resolve => setTimeout(resolve, 0));
    await new Promise<void>(resolve => requestAnimationFrame(() => resolve()));
  }, { index, outcome, text });
}
async function stored(page: Page): Promise<State> {
  return page.evaluate(key => JSON.parse(localStorage.getItem(key)!), STORAGE_KEY);
}
async function patchHarness(page: Page, patch: Partial<HarnessState>) {
  await page.evaluate(async patch => {
    (window as unknown as { cookieOwnerHarness: HarnessFixture }).cookieOwnerHarness.patch(patch);
    await new Promise<void>(resolve => requestAnimationFrame(() => resolve()));
  }, patch);
}
async function openHarness(page: Page) {
  const [componentSource, mainSource] = await Promise.all([
    page.request.get("/src/components/DemoEnvironmentWindows.tsx").then(response => response.text()),
    page.request.get("/src/main.tsx").then(response => response.text()),
  ]);
  const reactURL = componentSource.match(/from\s*["']([^"']*\/react\.js[^"']*)["']/)?.[1];
  const rootURL = mainSource.match(/from\s*["']([^"']*\/react-dom_client\.js[^"']*)["']/)?.[1];
  expect(reactURL).toBeTruthy();
  expect(rootURL).toBeTruthy();
  // App always unmounts on close. This small host also exercises the public props'
  // same-instance target changes and observes callbacks after child unmount.
  await page.route("**/cookie-owner-harness", route => route.fulfill({ contentType: "text/html", body: '<!doctype html><html lang="zh-CN"><head><meta charset="UTF-8"></head><body><div id="root"></div><script type="module" src="/cookie-owner-harness.js"></script></body></html>' }));
  await page.route("**/cookie-owner-harness.js", route => route.fulfill({ contentType: "text/javascript", body: `
    import React from ${JSON.stringify(reactURL)};
    import * as ReactDOM from ${JSON.stringify(rootURL)};
    import { parseCookies } from "/src/domain.ts";
    import { DemoCookieImportWindow } from "/src/components/DemoEnvironmentWindows.tsx";
    import "/src/styles.css";
    const fixture = window.cookieOwnerHarness = { calls: [], saves: 0 };
    function Harness() {
      const [state, setState] = React.useState({ environment: ${JSON.stringify(initial.environments[0])}, text: "", result: null, error: "", busy: false, mounted: true });
      fixture.patch = patch => setState(previous => ({ ...previous, ...patch }));
      return state.mounted ? React.createElement(DemoCookieImportWindow, {
        ...state, onText: text => { fixture.calls.push({ target: state.environment.id, text }); setState(previous => ({ ...previous, text, result: null })); },
        onParse: () => setState(previous => ({ ...previous, result: parseCookies(previous.text) })),
        onClose: () => setState(previous => ({ ...previous, mounted: false })), onSave: () => fixture.saves++,
      }) : null;
    }
    const createRoot = ReactDOM.createRoot ?? ReactDOM.default.createRoot;
    createRoot(document.getElementById("root")).render(React.createElement(Harness));
  ` }));
  await page.goto("/cookie-owner-harness");
  await expect(dialog(page)).toBeVisible();
}

test("late A file success cannot replace B draft or its parsed preview after close", async ({ page }) => {
  const before = await stored(page);
  await openTarget(page, "A");
  const read = await selectFile(page);
  await dialog(page).getByRole("button", { name: "关闭对话框", exact: true }).click();
  await openTarget(page, "B");
  const draft = cookieText("target_b_current");
  await preview(page, draft);
  await settle(page, read, "resolve", cookieText("target_a_late"));
  await expect(input(page)).toHaveValue(draft);
  await expect(dialog(page)).toContainText("target_b_current");
  await expect(save(page)).toBeEnabled();
  expect(await stored(page)).toEqual(before);
});

test("Escape then reopen of the same target gives the new draft its own read owner", async ({ page }) => {
  await openTarget(page, "A");
  const read = await selectFile(page);
  await page.keyboard.press("Escape");
  await expect(dialog(page)).toHaveCount(0);
  await openTarget(page, "A");
  const draft = cookieText("reopened_current");
  await preview(page, draft);
  await settle(page, read, "resolve", cookieText("previous_open_late"));
  await expect(input(page)).toHaveValue(draft);
  await expect(dialog(page)).toContainText("reopened_current");
  await expect(save(page)).toBeEnabled();
});

test("newer file wins out-of-order reads and the captured chooser resets before either await", async ({ page }) => {
  await openTarget(page, "A");
  const first = await selectFile(page);
  const firstInputValue = await dialog(page).getByLabel("读取示例Cookie文件").inputValue();
  const second = await selectFile(page); // Same in-memory filename must be selectable again while read one is pending.
  const secondInputValue = await dialog(page).getByLabel("读取示例Cookie文件").inputValue();
  const latest = cookieText("newest_file");
  await settle(page, second, "resolve", latest);
  await expect(input(page)).toHaveValue(latest);
  await dialog(page).getByRole("button", { name: "校验并预览", exact: true }).click();
  await settle(page, first, "resolve", cookieText("old_file"));
  await expect(input(page)).toHaveValue(latest);
  await expect(dialog(page)).toContainText("newest_file");
  await expect(save(page)).toBeEnabled();
  expect(firstInputValue).toBe("");
  expect(secondInputValue).toBe("");
});

test("manual text supersedes a pending file and keeps its genuine App preview", async ({ page }) => {
  await openTarget(page, "A");
  const read = await selectFile(page);
  const draft = cookieText("manual_current");
  await preview(page, draft);
  await settle(page, read, "resolve", cookieText("superseded_file"));
  await expect(input(page)).toHaveValue(draft);
  await expect(dialog(page)).toContainText("manual_current");
  await expect(save(page)).toBeEnabled();
});

test("fill example supersedes a pending file without auto parsing or saving", async ({ page }) => {
  const before = await stored(page);
  await openTarget(page, "A");
  await preview(page, cookieText("previous_preview"));
  const read = await selectFile(page);
  await expect(save(page)).toBeEnabled(); // Choosing a file alone must not discard an existing preview.
  await dialog(page).getByRole("button", { name: "填入示例", exact: true }).click();
  const example = await input(page).inputValue();
  expect(JSON.parse(example).map((cookie: { name: string }) => cookie.name)).toEqual(["sample", "empty"]);
  await expect(save(page)).toBeDisabled();
  await settle(page, read, "resolve", cookieText("superseded_file"));
  await expect(input(page)).toHaveValue(example);
  await expect(dialog(page)).toContainText("尚未解析，没有写入。");
  expect(await stored(page)).toEqual(before);
});

test("current read rejection keeps draft and preview, shows a safe error, and allows successful retry", async ({ page }) => {
  const before = await stored(page);
  await openTarget(page, "A");
  const draft = cookieText("retained_draft");
  await preview(page, draft);
  const failed = await selectFile(page);
  await settle(page, failed, "reject");
  await expect(dialog(page).getByRole("alert")).toHaveText(readFailure);
  await expect(dialog(page)).not.toContainText(sourceFailure);
  await expect(input(page)).toHaveValue(draft);
  await expect(dialog(page)).toContainText("retained_draft");
  await expect(save(page)).toBeEnabled(); // Existing save gates remain based on busy and the parsed result.
  const retry = await selectFile(page);
  const latest = cookieText("retry_file");
  await settle(page, retry, "resolve", latest);
  await expect(input(page)).toHaveValue(latest);
  await expect(dialog(page).getByRole("alert")).toHaveCount(0);
  await expect(dialog(page)).toContainText("尚未解析，没有写入。");
  await expect(save(page)).toBeDisabled();
  expect(await stored(page)).toEqual(before);
});

test("obsolete rejected read cannot replace a newer good read with an error", async ({ page }) => {
  await openTarget(page, "A");
  const old = await selectFile(page);
  const latest = await selectFile(page);
  const draft = cookieText("good_new_read");
  await settle(page, latest, "resolve", draft);
  await dialog(page).getByRole("button", { name: "校验并预览", exact: true }).click();
  await settle(page, old, "reject");
  await expect(input(page)).toHaveValue(draft);
  await expect(dialog(page).getByRole("alert")).toHaveCount(0);
  await expect(dialog(page)).toContainText("good_new_read");
  await expect(save(page)).toBeEnabled();
});

test("hidden A read rejection does not pollute B error or draft ownership", async ({ page }) => {
  await openTarget(page, "A");
  const read = await selectFile(page);
  await dialog(page).getByRole("button", { name: "取消", exact: true }).click();
  await openTarget(page, "B");
  const draft = cookieText("b_after_cancel");
  await preview(page, draft);
  await settle(page, read, "reject");
  await expect(input(page)).toHaveValue(draft);
  await expect(dialog(page).getByRole("alert")).toHaveCount(0);
  await expect(save(page)).toBeEnabled();
});

test("same-instance target changes invalidate local read and error without changing busy save gates", async ({ page }) => {
  await openHarness(page);
  const old = await selectFile(page);
  const draft = cookieText("mounted_b");
  const result = parseCookies(draft);
  await patchHarness(page, { environment: initial.environments[1], text: draft, result });
  await expect(dialog(page)).toContainText("目标：合成Cookie目标B");
  await settle(page, old, "resolve", cookieText("mounted_a_old"));
  await expect(input(page)).toHaveValue(draft);
  expect(await page.evaluate(() => (window as unknown as { cookieOwnerHarness: HarnessFixture }).cookieOwnerHarness.calls)).toEqual([]);
  const failed = await selectFile(page);
  await settle(page, failed, "reject");
  await expect(dialog(page).getByRole("alert")).toHaveText(readFailure);
  await expect(save(page)).toBeEnabled();
  await patchHarness(page, { environment: initial.environments[0], error: "合成保存失败：原记录保留", busy: true });
  await expect(dialog(page).getByRole("alert")).toHaveText("合成保存失败：原记录保留");
  await expect(input(page)).toBeDisabled();
  for (const name of ["选择文件", "填入示例", "校验并预览", "取消", "导入到原型记录"]) await expect(dialog(page).getByRole("button", { name, exact: true })).toBeDisabled();
  await patchHarness(page, { busy: false, error: "", result: parseCookies("[]") });
  await expect(save(page)).toBeDisabled();
  await patchHarness(page, { result: { ...result, errors: ["合成解析错误"] } });
  await expect(save(page)).toBeDisabled();
  await patchHarness(page, { result });
  await expect(save(page)).toBeEnabled();
  expect(await page.evaluate(() => (window as unknown as { cookieOwnerHarness: HarnessFixture }).cookieOwnerHarness.saves)).toBe(0);
});

test("unmounted component never calls onText and still handles late rejection", async ({ page }) => {
  await openHarness(page);
  const success = await selectFile(page);
  await patchHarness(page, { mounted: false });
  await expect(dialog(page)).toHaveCount(0);
  await settle(page, success, "resolve", cookieText("unmounted_success"));
  expect(await page.evaluate(() => (window as unknown as { cookieOwnerHarness: HarnessFixture }).cookieOwnerHarness.calls)).toEqual([]);
  await patchHarness(page, { mounted: true });
  await expect(dialog(page)).toBeVisible();
  const failure = await selectFile(page);
  await patchHarness(page, { mounted: false });
  await settle(page, failure, "reject");
  expect(await page.evaluate(() => (window as unknown as { cookieOwnerHarness: HarnessFixture }).cookieOwnerHarness.calls)).toEqual([]);
});
