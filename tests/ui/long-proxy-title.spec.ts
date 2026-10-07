import { expect, test, type Locator } from "@playwright/test";
import { STORAGE_KEY } from "../../src/domain";
import { referenceWorkspace } from "./fixtures/reference-workspace";

// Actual main.tsx/App/DemoProxyManager, synthetic state only. Opening existing
// details/usage must not run a check, mutate the workspace or use a native bridge.
const names = ["合成 SOCKS5", "W".repeat(200)];
const variants = [
  { kind: "details", prefix: "代理检查详情", width: 620, height: 186, titleOffset: { x: 0, y: 11 } },
  { kind: "usage", prefix: "已绑定环境", width: 1040, height: 592, titleOffset: { x: -8, y: 18 } },
] as const;
type DemoAudit = { proxyCheckActions: string[] };

async function measure(dialog: Locator) {
  return dialog.evaluate(element => {
    const box = (node: Element) => {
      const rect = node.getBoundingClientRect();
      return { x: rect.x, y: rect.y, width: rect.width, height: rect.height, right: rect.right, bottom: rect.bottom };
    };
    const header = element.querySelector<HTMLElement>(".reference-modal-header")!;
    const title = header.querySelector<HTMLElement>("h2")!;
    const close = header.querySelector<HTMLButtonElement>("button")!;
    const footer = element.querySelector<HTMLElement>(".reference-modal-footer")!;
    const h = box(header), t = box(title), c = box(close);
    const titleStyle = getComputedStyle(title), footerStyle = getComputedStyle(footer);
    const hitPoints = [
      [c.x + c.width / 2, c.y + c.height / 2],
      [c.x + 1, c.y + 1], [c.right - 1, c.y + 1],
      [c.x + 1, c.bottom - 1], [c.right - 1, c.bottom - 1],
    ];
    return {
      viewport: { width: innerWidth, height: innerHeight, dpr: devicePixelRatio, scale: visualViewport!.scale },
      dialog: box(element), header: h, body: box(element.querySelector(".reference-modal-body")!),
      title: {
        ...t, text: title.textContent, clientWidth: title.clientWidth, scrollWidth: title.scrollWidth,
        fontSize: titleStyle.fontSize, fontWeight: titleStyle.fontWeight, lineHeight: titleStyle.lineHeight,
        whiteSpace: titleStyle.whiteSpace, overflow: titleStyle.overflow, textOverflow: titleStyle.textOverflow,
      },
      close: c, footer: { ...box(footer), padding: footerStyle.padding },
      titleContained: t.x >= h.x - 0.5 && t.right <= h.right + 0.5 && t.y >= h.y - 0.5 && t.bottom <= h.bottom + 0.5,
      titleIntersectsClose: t.x < c.right && t.right > c.x && t.y < c.bottom && t.bottom > c.y,
      closeHitTests: hitPoints.map(([x, y]) => document.elementFromPoint(x, y)?.closest("button") === close),
      rootHorizontalOverflow: document.documentElement.scrollWidth > innerWidth,
    };
  });
}

for (const viewport of [{ width: 1440, height: 900 }, { width: 1280, height: 800 }]) {
  for (const variant of variants) {
    test(`${viewport.width}x${viewport.height} demo proxy ${variant.kind}: long title fits and short frame stays unchanged`, async ({ browser, baseURL }, testInfo) => {
      const origin = new URL(baseURL!).origin;
      for (const name of names) {
        const long = name.length === 200, label = long ? "long" : "short";
        const state = referenceWorkspace(); state.proxies[0].name = name;
        const original = JSON.stringify(state);
        const context = await browser.newContext({ viewport, deviceScaleFactor: 1, locale: "zh-CN", timezoneId: "Asia/Shanghai", serviceWorkers: "block" });
        const blockedRequests: string[] = [], pageErrors: string[] = [];
        await context.route("**/*", route => {
          const request = route.request();
          if (new URL(request.url()).origin === origin && request.method() === "GET") return route.continue();
          blockedRequests.push(`${request.method()} ${request.url()}`); return route.abort("blockedbyclient");
        });
        await context.routeWebSocket("**/*", socket => socket.close());
        await context.addInitScript(({ state, key }) => {
          for (const name of ["go", "runtime"]) {
            if (Object.getOwnPropertyDescriptor(window, name)) throw new Error("Unexpected native bridge descriptor");
            Object.defineProperty(window, name, { value: undefined, configurable: false, writable: false });
          }
          localStorage.setItem(key, JSON.stringify(state));
          const audit: DemoAudit = { proxyCheckActions: [] };
          Object.assign(window, { __longProxyTitleAudit: audit });
          document.addEventListener("click", event => {
            const button = event.target instanceof Element ? event.target.closest("button") : null;
            const text = button?.textContent?.trim() ?? "";
            if (button?.closest(".proxy35-page") && ["检查", "批量代理检测"].includes(text)) audit.proxyCheckActions.push(text);
          }, true);
        }, { state, key: STORAGE_KEY });
        const page = await context.newPage();
        page.on("pageerror", error => pageErrors.push(error.message));
        let measurement: Awaited<ReturnType<typeof measure>> | undefined;
        try {
          await page.goto(`${origin}/#/proxies`);
          await expect(page.locator(".pk35-mode")).toHaveText("演示配置 · 检测均为模拟");
          const row = page.locator(".proxy35-table tbody tr").filter({ has: page.getByLabel(`选择代理 ${name}`, { exact: true }) });
          await expect(row).toHaveCount(1);
          expect(await page.evaluate(key => localStorage.getItem(key), STORAGE_KEY)).toBe(original);
          const trigger = variant.kind === "details" ? row.locator("td").nth(3).getByRole("button") : row.getByRole("button", { name: `${name} 已绑定环境`, exact: true });
          await trigger.click();
          const fullTitle = `${variant.prefix} · ${name}`;
          const dialog = page.getByRole("dialog", { name: fullTitle, exact: true });
          await expect(dialog).toBeVisible();
          await expect(dialog).toHaveAccessibleName(fullTitle);
          await expect(dialog.getByRole("heading", { level: 2, name: fullTitle, exact: true })).toHaveText(fullTitle);
          const close = dialog.getByRole("button", { name: `关闭${fullTitle}`, exact: true });
          const confirm = dialog.locator("footer").getByRole("button", { name: "确定", exact: true });
          await expect(confirm).toBeVisible();
          await page.evaluate(() => new Promise<void>(resolve => requestAnimationFrame(() => requestAnimationFrame(() => resolve()))));
          measurement = await measure(dialog);
          await testInfo.attach(`long-proxy-title-${label}-${variant.kind}-${viewport.width}`, { body: JSON.stringify(measurement, null, 2), contentType: "application/json" });
          await page.screenshot({ path: testInfo.outputPath(`long-proxy-title-${label}-${variant.kind}-${viewport.width}.png`), fullPage: false });

          // Keep the observed existing header, frame, typography, close slot and footer.
          const m = measurement;
          expect(m.viewport).toEqual({ ...viewport, dpr: 1, scale: 1 });
          expect(m.dialog.width).toBe(variant.width); expect(m.dialog.height).toBe(variant.height);
          expect(m.header).toEqual({ ...m.dialog, height: 40, bottom: m.dialog.y + 40 });
          expect(m.body.y).toBe(m.header.bottom);
          expect(m.footer.height).toBe(58); expect(m.footer.bottom).toBe(m.dialog.bottom); expect(m.footer.padding).toBe("10px");
          expect(m.close).toEqual({ x: m.header.right - 34, y: m.header.y + 5, width: 26, height: 30, right: m.header.right - 8, bottom: m.header.y + 35 });
          expect(m.title.fontSize).toBe("14px"); expect(m.title.fontWeight).toBe("600"); expect(m.title.lineHeight).toBe("18px");
          expect(m.title.text).toBe(fullTitle);
          expect.soft(m.titleContained, "complete visible title box stays inside the 40px header").toBe(true);
          expect.soft(m.titleIntersectsClose, "title must not occupy the close button slot").toBe(false);
          expect.soft(m.title.height, "title stays on a single line").toBe(18);
          expect.soft(m.close.x - m.title.right, "at least 8px clearance before the close slot").toBeGreaterThanOrEqual(8);
          if (long) {
            expect.soft(m.title.whiteSpace).toBe("nowrap"); expect.soft(m.title.overflow).toBe("hidden"); expect.soft(m.title.textOverflow).toBe("ellipsis");
            expect.soft(m.title.scrollWidth).toBeGreaterThan(m.title.clientWidth);
          } else {
            expect(m.title.scrollWidth).toBeLessThanOrEqual(m.title.clientWidth);
            expect(m.title.x + m.title.width / 2).toBeCloseTo(m.header.x + m.header.width / 2 + variant.titleOffset.x, 1);
            expect(m.title.y).toBe(m.header.y + variant.titleOffset.y);
          }
          expect(m.closeHitTests).toEqual([true, true, true, true, true]);
          expect(m.rootHorizontalOverflow).toBe(false);

          // Keyboard reachability and real pointer close, followed by exact return focus.
          if (variant.kind === "details") {
            await expect(close).toBeFocused(); await page.keyboard.press("Tab"); await expect(confirm).toBeFocused();
          } else await expect(dialog.getByRole("textbox", { name: "搜索已绑定环境", exact: true })).toBeFocused();
          await page.keyboard.press("Shift+Tab"); await expect(close).toBeFocused();
          await close.click(); await expect(dialog).toHaveCount(0); await expect(trigger).toBeFocused();
          expect(await page.evaluate(key => localStorage.getItem(key), STORAGE_KEY)).toBe(original);
          await expect(row.locator("td").nth(7)).toHaveText(name);
        } finally {
          const audit = await page.evaluate(() => {
            const host = window as unknown as { go?: unknown; runtime?: unknown; __longProxyTitleAudit: DemoAudit };
            return { ...host.__longProxyTitleAudit, nativeBridgePresent: host.go !== undefined || host.runtime !== undefined };
          });
          const stateUnchanged = await page.evaluate(key => localStorage.getItem(key), STORAGE_KEY) === original;
          await testInfo.attach(`long-proxy-title-${label}-safety`, { body: JSON.stringify({ ...audit, stateUnchanged, blockedRequests, pageErrors }, null, 2), contentType: "application/json" });
          await context.close();
          expect(audit.nativeBridgePresent).toBe(false); expect(audit.proxyCheckActions).toEqual([]);
          expect(stateUnchanged).toBe(true); expect(blockedRequests).toEqual([]); expect(pageErrors).toEqual([]);
        }
      }
    });
  }
}
