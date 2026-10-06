import { expect, test } from "@playwright/test";

test("shared scroll and inert owners release out of order without unlocking a live owner", async ({ page }) => {
  await page.routeWebSocket("**/*", socket => socket.close());
  await page.goto("/tests/ui/fixtures/local-pages.html#/activity");
  const result = await page.evaluate(async () => {
    const module = "/src/components/modal-lifecycle.ts";
    const { lockBodyScroll, lockModalBackground } = await import(module);
    const region = document.createElement("section"); document.body.append(region);
    const originalOverflow = document.body.style.overflow;
    const firstScroll = lockBodyScroll(), first = lockModalBackground([region]);
    const secondScroll = lockBodyScroll(), second = lockModalBackground([region]);
    first(); first(); firstScroll(); firstScroll();
    const lowerReleased = { inert: region.inert, overflow: document.body.style.overflow };
    second(); secondScroll();
    const allReleased = { inert: region.inert, overflow: document.body.style.overflow };
    region.remove();
    return { lowerReleased, allReleased, originalOverflow };
  });
  expect(result.lowerReleased).toEqual({ inert: true, overflow: "hidden" });
  expect(result.allReleased).toEqual({ inert: false, overflow: result.originalOverflow });
});

test("last portal release restores current React inert intent, not the old lower-window state", async ({ page }) => {
  await page.routeWebSocket("**/*", socket => socket.close());
  await page.goto("/tests/ui/fixtures/local-pages.html#/activity");
  const result = await page.evaluate(async () => {
    const module = "/src/components/modal-lifecycle.ts";
    const { lockModalBackground } = await import(module);
    const region = document.createElement("section"); region.inert = true; region.dataset.appInert = "true";
    document.body.append(region);
    const release = lockModalBackground([region]);
    region.dataset.appInert = "false"; region.inert = false;
    await new Promise<void>(resolve => queueMicrotask(resolve));
    const whileOwned = region.inert;
    release(); const afterRelease = region.inert;
    region.remove();
    return { whileOwned, afterRelease };
  });
  expect(result).toEqual({ whileOwned: true, afterRelease: false });
});
