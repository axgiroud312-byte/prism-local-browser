import type { Page } from "@playwright/test";

/** Standalone exports until MAIN inserts these modules in App; no real RPC. */
export async function openProxyKernelHarness(page: Page, initialRoute = "proxies") {
  await page.route("**/issue35-component-harness", route => route.fulfill({ contentType: "text/html", body: `<!doctype html><html lang="zh-CN"><head><meta charset="UTF-8"><title>#35 合成组件检查</title></head><body><div id="root"></div><script>window.__issue35Route=${JSON.stringify(initialRoute)}</script><script type="module" src="/tests/ui/fixtures/proxy-kernel-harness.jsx"></script></body></html>` }));
  await page.goto("/issue35-component-harness");
}
