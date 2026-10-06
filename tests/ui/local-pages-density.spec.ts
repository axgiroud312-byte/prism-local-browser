import { expect, test } from "@playwright/test";
import { createSnapshot, STORAGE_KEY } from "../../src/domain";
import type { WorkspaceView } from "../../src/application/contract";
import { referenceWorkspace } from "./fixtures/reference-workspace";
import { localPagesNativeBridge } from "./fixtures/local-pages-native-bridge";

for (const mode of ["demo", "native"] as const) for (const viewport of [{ width: 1440, height: 900 }, { width: 1280, height: 800 }]) {
  test(`${mode} dense backup table keeps pagination reachable at ${viewport.width}x${viewport.height}`, async ({ page, baseURL }) => {
    await page.setViewportSize(viewport);
    const origin = new URL(baseURL!).origin;
    await page.route("**/*", route => new URL(route.request().url()).origin === origin ? route.continue() : route.abort());
    await page.routeWebSocket("**/*", socket => socket.close());
    if (mode === "native") await localPagesNativeBridge(page);
    else {
      const state = referenceWorkspace(), snapshot = createSnapshot(state);
      state.backups = Array.from({ length: 12 }, (_, index) => ({ id: `synthetic-dense-${index + 1}`, name: `合成备份 ${index + 1}`, createdAt: "2026-10-06T01:00:00Z", snapshot }));
      await page.addInitScript(({ state, key }) => localStorage.setItem(key, JSON.stringify(state)), { state, key: STORAGE_KEY });
    }
    await page.goto("/#/backups");
    if (mode === "native") await page.evaluate(() => {
      const workspace = (window as unknown as { __localPagesFixture: { workspace: WorkspaceView } }).__localPagesFixture.workspace;
      workspace.nativeBackups = Array.from({ length: 12 }, (_, index) => ({ id: `synthetic-dense-${index + 1}`, operationId: `synthetic-published-${index + 1}`, name: `合成备份 ${index + 1}`, scope: "all", environmentCount: 12, createdAt: "2026-10-06T01:00:00Z", archiveSha256: "a".repeat(64), manifestSha256: "c".repeat(64) }));
    });
    const rows = page.locator(".local-page-table tbody tr"), table = page.locator(".local-page-backups .local-page-table-scroll"), pager = page.locator(".local-page-pagination");
    await expect(rows).toHaveCount(10);
    const before = await pager.boundingBox();
    await table.evaluate(element => { element.scrollTop = element.scrollHeight; });
    await expect(pager).toBeVisible();
    const after = await pager.boundingBox();
    expect(after).toEqual(before);
    expect(after!.y + after!.height).toBeLessThanOrEqual(viewport.height);
    await expect(page.getByRole("button", { name: "下一页", exact: true })).toBeEnabled();
    await page.getByRole("button", { name: "下一页", exact: true }).click();
    await expect(rows).toHaveCount(2); await expect(rows.first()).toContainText("合成备份 11");
    await page.getByRole("button", { name: "上一页", exact: true }).click();
    await expect(rows).toHaveCount(10); await expect(rows.first()).toContainText("合成备份 1");
  });
}
