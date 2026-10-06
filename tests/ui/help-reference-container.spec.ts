import { readFileSync } from "node:fs";
import { expect, test, type Locator, type Page } from "@playwright/test";
import { STORAGE_KEY } from "../../src/domain";
import type { NativeRequest } from "../../src/application/wails-adapter";
import { referenceWorkspace } from "./fixtures/reference-workspace";
import { localPagesNativeBridge } from "./fixtures/local-pages-native-bridge";

const documents = [
  { title: "本机使用指南", filename: "USER_GUIDE.md" },
  { title: "产品需求 PRD", filename: "PRD.md" },
  { title: "开发与验收", filename: "DEVELOPMENT.md" },
  { title: "内核适配合同", filename: "KERNEL.md" },
].map(document => {
  const markdown = readFileSync(new URL(`../../docs/${document.filename}`, import.meta.url), "utf8");
  const link = [...markdown.matchAll(/\[([^\]]+)\]\(([^)]+)\)/g)].find(match =>
    /(?:USER_GUIDE|PRD|DEVELOPMENT|KERNEL)\.md/.test(match[2]) && !match[2].includes(document.filename))!;
  return {
    ...document,
    heading: markdown.match(/^# (.+)$/m)![1].trim(),
    linkLabel: link[1],
    nextFilename: link[2].match(/(?:USER_GUIDE|PRD|DEVELOPMENT|KERNEL)\.md/)![0],
    // Bind the real final paragraph to the current embedded document, not a
    // loading placeholder or a historic screenshot's scrollHeight.
    tail: markdown.trim().split(/\r?\n/).at(-1)!
      .replace(/\[([^\]]+)\]\([^)]+\)/g, "$1")
      .replace(/`([^`]+)`/g, "$1"),
  };
});
const guards = new WeakMap<Page, { errors: string[]; rejected: string[]; downloads: string[]; pickers: number }>();

test.beforeEach(async ({ page, baseURL }) => {
  const guard = { errors: [] as string[], rejected: [] as string[], downloads: [] as string[], pickers: 0 };
  guards.set(page, guard);
  page.on("pageerror", error => guard.errors.push(error.message));
  page.on("download", download => guard.downloads.push(download.suggestedFilename()));
  page.on("filechooser", () => guard.pickers++);
  const origin = new URL(baseURL!).origin;
  await page.context().route("**/*", route => {
    if (new URL(route.request().url()).origin === origin) return route.continue();
    guard.rejected.push(route.request().url());
    return route.abort("blockedbyclient");
  });
  // Close every socket, including Vite HMR, so a queued reload cannot reset
  // the synthetic workspace. An attempted product socket fails the test.
  await page.routeWebSocket("**/*", socket => {
    if (new URL(socket.url()).host !== new URL(origin).host) guard.rejected.push(socket.url());
    socket.close();
  });
});

test.afterEach(async ({ page }) => {
  expect(guards.get(page)).toEqual({ errors: [], rejected: [], downloads: [], pickers: 0 });
});

async function measure(page: Page) {
  return page.locator(".local-page-help-frame").evaluate(frame => {
    const index = frame.querySelector<HTMLElement>(".local-page-help-links")!;
    const body = frame.querySelector<HTMLElement>(".document-body")!;
    const tabs = document.querySelector<HTMLElement>(".local-page-help .local-page-tabs")!;
    const toolbar = document.querySelector<HTMLElement>(".local-page-help .local-page-toolbar")!;
    const rect = (element: Element) => {
      const { x, y, width, height } = element.getBoundingClientRect();
      return { x, y, width, height };
    };
    const style = getComputedStyle(frame);
    return {
      gridColumns: style.gridTemplateColumns.split(" ").map(value => parseFloat(value)),
      innerWidth: frame.clientWidth - parseFloat(style.paddingLeft) - parseFloat(style.paddingRight),
      index: rect(index), body: rect(body), tabs: rect(tabs), toolbar: rect(toolbar),
      fontSize: getComputedStyle(body).fontSize,
      clientHeight: body.clientHeight, scrollHeight: body.scrollHeight, scrollTop: body.scrollTop,
      bottomDifference: body.scrollHeight - body.clientHeight - body.scrollTop,
      horizontalOverflow: body.scrollWidth - body.clientWidth,
      documentOverflow: document.documentElement.scrollWidth - window.innerWidth,
    };
  });
}

async function reachable(locator: Locator) {
  await expect(locator).toBeEnabled();
  await expect(locator).toBeInViewport({ ratio: 1 });
  expect(await locator.evaluate(element => {
    const rect = element.getBoundingClientRect();
    return element.contains(document.elementFromPoint(rect.x + rect.width / 2, rect.y + rect.height / 2));
  })).toBe(true);
}

async function fixedControls(page: Page) {
  const tabs = page.getByRole("tablist", { name: "嵌入文档", exact: true });
  await expect(tabs.getByRole("tab")).toHaveCount(4);
  for (const document of documents) await reachable(tabs.getByRole("tab", { name: document.title, exact: true }));
  // MAIN runs the existing filename/download action checker separately.
  // Here, verify reachability without creating a duplicate download.
  await reachable(page.getByRole("button", { name: "下载文档", exact: true }));
  const navigation = page.getByRole("complementary", { name: "帮助导航", exact: true });
  await expect(navigation.getByRole("button")).toHaveCount(9);
  for (const button of await navigation.getByRole("button").all()) await reachable(button);
  const home = navigation.getByRole("link", { name: "项目首页", exact: true });
  await expect(home).toHaveAttribute("href", "https://github.com/axgiroud312-byte/prism-local-browser");
  await reachable(home);
}

for (const mode of ["demo", "native"] as const) {
  for (const viewport of [{ width: 1280, height: 800 }, { width: 1440, height: 900 }]) {
    for (const document of documents) {
      test(`${mode} ${document.filename} real App top/bottom container at ${viewport.width}x${viewport.height}`, async ({ page }, testInfo) => {
        await page.setViewportSize(viewport);
        if (mode === "native") await localPagesNativeBridge(page);
        else await page.addInitScript(({ state, key }) => localStorage.setItem(key, JSON.stringify(state)), { state: referenceWorkspace(), key: STORAGE_KEY });
        await page.goto("/#/guide");
        const tabs = page.getByRole("tablist", { name: "嵌入文档", exact: true });
        await tabs.getByRole("tab", { name: document.title, exact: true }).click();
        const body = page.getByRole("tabpanel", { name: document.title, exact: true });
        await expect(body.getByRole("heading", { level: 1, name: document.heading, exact: true })).toBeVisible();
        await expect(body).not.toContainText("正在载入文档");
        const tail = body.locator(":scope > :last-child");
        await expect(tail).toHaveText(document.tail);
        await expect.poll(() => body.evaluate(element => element.scrollTop)).toBe(0);
        await fixedControls(page);
        const top = await measure(page);

        await body.evaluate(element => { element.scrollTop = element.scrollHeight; });
        await expect.poll(() => body.evaluate(element => Math.abs(element.scrollHeight - element.clientHeight - element.scrollTop))).toBeLessThanOrEqual(1);
        await expect(tail).toBeInViewport({ ratio: 1 });
        await expect(body).not.toContainText("正在载入文档");
        await fixedControls(page);
        const bottom = await measure(page);
        await testInfo.attach("help-container-measurements", {
          body: JSON.stringify({ mode, viewport, filename: document.filename, top, bottom }), contentType: "application/json",
        });

        expect(bottom.tabs).toEqual(top.tabs);
        expect(bottom.toolbar).toEqual(top.toolbar);
        expect(bottom.index).toEqual(top.index);
        expect(bottom.body).toEqual(top.body);
        expect(bottom.scrollTop).toBeGreaterThan(0);
        for (const position of [top, bottom]) {
          expect(position.fontSize).toBe("12px");
          expect(position.tabs.y).toBe(50);
          expect(position.tabs.height).toBe(29);
          expect(position.toolbar.height).toBe(52);
          expect(position.body.y).toBe(151);
          expect(position.clientHeight).toBe(viewport.height - 170);
          expect(position.horizontalOverflow).toBeLessThanOrEqual(1);
          expect(position.documentOverflow).toBeLessThanOrEqual(1);
        }
        expect(await body.locator('a[href="#/guide"]').first().getAttribute("target")).toBeNull();
        // Follow a real embedded Markdown document link after reaching the
        // tail. It must select another document and reset the body, not leave
        // the App or request an external Markdown file.
        const next = documents.find(item => item.filename === document.nextFilename)!;
        const link = body.getByRole("link", { name: document.linkLabel, exact: true }).first();
        await expect(link).toHaveAttribute("href", "#/guide");
        await link.click();
        await expect(tabs.getByRole("tab", { name: next.title, exact: true })).toHaveAttribute("aria-selected", "true");
        const nextBody = page.getByRole("tabpanel", { name: next.title, exact: true });
        await expect(nextBody.getByRole("heading", { level: 1, name: next.heading, exact: true })).toBeVisible();
        await expect.poll(() => nextBody.evaluate(element => element.scrollTop)).toBe(0);
        await expect(page).toHaveURL(/#\/guide$/);
        if (mode === "native") {
          const calls = await page.evaluate(() => (window as unknown as { __localPagesFixture: { calls: NativeRequest[] } }).__localPagesFixture.calls);
          expect(calls.some(call => call.method === "Workspace.Read")).toBe(true);
          expect(calls.filter(call => call.method !== "Workspace.Read" && !(call.method === "Cookie.DiscardImport" && (call.payload as { previewId: string }).previewId === ""))).toEqual([]);
        }

        const expected = viewport.width === 1280 ? { inner: 1040, index: 260, body: 780, x: 480 } : { inner: 1200, index: 300, body: 900, x: 520 };
        for (const position of [top, bottom]) {
          expect(position.gridColumns, "real App computed desktop help columns").toEqual([expected.index, expected.body]);
          expect(position.innerWidth).toBe(expected.inner);
          expect(position.index.width).toBe(expected.index);
          expect(position.body.width).toBe(expected.body);
          expect(position.body.x).toBe(expected.x);
        }
      });
    }
  }
}
