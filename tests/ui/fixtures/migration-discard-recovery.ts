import type { Page } from "@playwright/test";
import type { ApplicationResult } from "../../../src/application/contract";
import type { NativeRequest } from "../../../src/application/wails-adapter";
import { migrationWorkflowBridge, type MigrationWorkflowScenario } from "./migration-workflow-bridge";

export type MigrationDiscardReply = "known" | "unknown" | "invalid" | "demo" | "refused" | "throw";
interface MigrationDiscardHooks {
  snapshot(): { syntheticOnly: true; calls: NativeRequest[]; held: { label: string; request: NativeRequest }[] };
  setDiscardReply(reply: MigrationDiscardReply): void;
  hold(method: string, label: string): void;
  reply(label: string, reply?: MigrationDiscardReply): Promise<void>;
}
declare global { interface Window { __migrationDiscard: MigrationDiscardHooks } }

/** Own wrapper only: failed cleanup does NOT dispatch a successful base discard.
 * The existing workflow fixture and its original data/assertions stay unchanged.
 */
export async function migrationDiscardRecoveryBridge(page: Page, scenario: MigrationWorkflowScenario = "rollback-failed") {
  await migrationWorkflowBridge(page, scenario);
  await page.addInitScript(() => {
    type Call = (request: NativeRequest) => Promise<ApplicationResult<unknown>>;
    type Go = { main: { DesktopApp: { Call: Call } } };
    const calls: NativeRequest[] = [], holds: { method: string; label: string }[] = [];
    const held = new Map<string, { request: NativeRequest; original: Call; resolve(result: ApplicationResult<unknown>): void; reject(error: Error): void }>();
    let discardReply: MigrationDiscardReply = "known", go: Go | undefined;
    const copy = <T,>(value: T): T => structuredClone(value);
    const response = async (request: NativeRequest, original: Call, reply: MigrationDiscardReply) => {
      if (reply === "known") return original(request);
      if (reply === "throw") throw new Error("SYNTHETIC_PRIVATE_DISCARD_TRANSPORT");
      if (reply === "invalid" || reply === "demo") return { ok: true as const, mode: reply === "demo" ? "demo" as const : "native" as const, data: { status: reply === "demo" ? "discarded" : "accepted" } };
      return { ok: false as const, mode: "native" as const, error: { code: reply === "refused" ? "RESTORE_NOT_ACCEPTED" : "NATIVE_UNAVAILABLE", message: "合成原响应尚未确认；未执行真实桌面操作。", retryable: true } };
    };
    window.__migrationDiscard = {
      snapshot: () => copy({ syntheticOnly: true, calls, held: [...held.entries()].map(([label, value]) => ({ label, request: value.request })) }),
      setDiscardReply(value) { discardReply = value; },
      hold(method, label) { if (held.has(label) || holds.some(h => h.label === label)) throw new Error("Duplicate synthetic hold"); holds.push({ method, label }); },
      async reply(label, value = "known") {
        const item = held.get(label); if (!item) throw new Error("Unknown synthetic hold"); held.delete(label);
        try { item.resolve(await response(item.request, item.original, value)); } catch (error) { item.reject(error as Error); }
      },
    };
    function install(value: Go) {
      go = value; const original = value.main.DesktopApp.Call;
      value.main.DesktopApp.Call = async request => {
        calls.push(copy(request));
        const index = holds.findIndex(h => h.method === request.method);
        if (index >= 0) {
          const [hold] = holds.splice(index, 1);
          return new Promise((resolve, reject) => held.set(hold.label, { request: copy(request), original, resolve, reject }));
        }
        return request.method === "Backup.DiscardRestore" ? response(request, original, discardReply) : original(request);
      };
    }
    // addInitScript order is unspecified: intercept a future assignment or an
    // already-installed bridge, without touching the production App/adapter.
    const existing = (window as unknown as { go?: Go }).go;
    Object.defineProperty(window, "go", { configurable: true, get: () => go, set: install });
    if (existing) install(existing);
  });
}

/** MAIN may reuse this actual-App entry for independently owned bounded images. */
export async function migrationDiscardRecoveryEntry(page: Page, reply: MigrationDiscardReply, url: string) {
  await migrationDiscardRecoveryBridge(page);
  await page.goto(url);
  await page.getByRole("button", { name: "选定环境迁移", exact: true }).click();
  const selection = page.getByRole("dialog", { name: "选定环境内核迁移", exact: true });
  await selection.getByText("第 1 页 · 共 2 个").waitFor();
  await selection.getByText("迁移记录与升级前恢复", { exact: true }).click();
  await selection.getByRole("button", { name: "预检升级前完整恢复", exact: true }).click();
  await selection.getByText("升级前完整恢复预检", { exact: true }).waitFor();
  await page.evaluate(reply => window.__migrationDiscard.setDiscardReply(reply), reply);
  await selection.getByRole("button", { name: "预检升级前完整恢复", exact: true }).click();
  await page.getByRole("dialog", { name: "升级前预检清理待核实", exact: true }).waitFor();
}
