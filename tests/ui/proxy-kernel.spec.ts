import { test, expect } from "@playwright/test";
import { proxyKernelBridge } from "./fixtures/proxy-kernel";
import { openProxyKernelHarness } from "./fixtures/proxy-kernel-harness";

test.beforeEach(async ({ page }) => {
  await page.route("**/*", route => {
    const url = new URL(route.request().url());
    return url.hostname === "127.0.0.1" ? route.continue() : route.abort();
  });
});

test("native proxy import hides without clearing preview, errors or input across routes", async ({ page }) => {
  await proxyKernelBridge(page, "failure");
  await page.goto("/#/proxies");
  await page.getByRole("button", { name: "批量添加代理", exact: true }).click();
  const dialog = page.getByRole("dialog", { name: "批量添加代理", exact: true });
  const input = "socks5://synthetic:only@203.0.113.55:1080\nnot-a-proxy\nhttp://198.51.100.90:8080";
  await dialog.getByLabel("原始代理导入文本").fill(input);
  await dialog.getByRole("button", { name: "解析预览", exact: true }).click();
  await expect(dialog.getByText("代理格式错误", { exact: true })).toBeVisible();
  await dialog.getByRole("button", { name: "保存所选有效行", exact: true }).click();
  await expect(dialog.getByRole("status")).toContainText("STORAGE_WRITE_FAILED");
  await expect(dialog.locator("footer")).toContainText("STORAGE_WRITE_FAILED");
  await dialog.getByRole("button", { name: "取消", exact: true }).click();
  await page.getByRole("link", { name: "内核管理", exact: true }).click();
  await page.getByRole("link", { name: "代理管理", exact: true }).click();
  await page.getByRole("button", { name: "批量添加代理", exact: true }).click();
  await expect(dialog.getByLabel("原始代理导入文本")).toHaveValue(input);
  await expect(dialog.getByText("代理格式错误", { exact: true })).toBeVisible();
  await expect(dialog.getByLabel("保存第1行")).toBeChecked();
  await dialog.getByRole("button", { name: "清除输入与预览", exact: true }).click();
  await expect(dialog.getByLabel("原始代理导入文本")).toHaveValue("");
});

test("unknown proxy commit freezes original receipt; success cannot replay committed rows", async ({ page }) => {
  await proxyKernelBridge(page, "unknown");
  await page.goto("/#/proxies");
  await page.getByRole("button", { name: "批量添加代理", exact: true }).click();
  const dialog = page.getByRole("dialog", { name: "批量添加代理", exact: true });
  await dialog.getByLabel("原始代理导入文本").fill("http://203.0.113.55:8080\ninvalid-line");
  await dialog.getByRole("button", { name: "解析预览", exact: true }).click();
  await dialog.getByRole("button", { name: "保存所选有效行", exact: true }).click();
  await expect(dialog.locator("footer")).toContainText("原请求结果未知");
  await expect(dialog.getByLabel("原始代理导入文本")).toBeDisabled();
  await expect(dialog.getByRole("button", { name: "清除输入与预览" })).toBeDisabled();
  await dialog.getByRole("button", { name: "取消", exact: true }).click();
  await page.getByRole("button", { name: "批量添加代理", exact: true }).click();
  await page.evaluate(() => (window as unknown as { __proxyKernel: { setOutcome(value: string): void } }).__proxyKernel.setOutcome("completed"));
  await dialog.getByRole("button", { name: "核实原导入请求", exact: true }).click();
  await expect(dialog.getByLabel("原始代理导入文本")).toHaveValue("invalid-line");
  const calls = await page.evaluate(() => (window as unknown as { __proxyKernel: { calls: { method: string; payload: Record<string, unknown> }[] } }).__proxyKernel.calls.filter(c => c.method === "Proxy.CommitImport"));
  expect(calls).toHaveLength(2);
  expect(calls[1].payload).toEqual(calls[0].payload);
});

test("native proxy edit masks saved credentials and bound deletion is protected", async ({ page }) => {
  await proxyKernelBridge(page);
  await page.goto("/#/proxies");
  await expect(page.getByRole("button", { name: "删除代理 合成 SOCKS", exact: true })).toBeDisabled();
  await page.getByRole("button", { name: "编辑代理 合成 SOCKS", exact: true }).click();
  const dialog = page.getByRole("dialog", { name: "修改代理", exact: true });
  await dialog.getByLabel("认证处理").selectOption("replace");
  await expect(dialog.getByLabel("新密码")).toHaveValue("");
  await dialog.getByLabel("新用户名（不回显旧值）").fill("synthetic-only");
  await dialog.getByLabel("新密码").fill("synthetic-secret");
  await dialog.getByLabel("认证处理").selectOption("keep");
  await dialog.getByRole("button", { name: "确认保存", exact: true }).click();
  const requests = await page.evaluate(() => (window as unknown as { __proxyKernel: { calls: { method: string; payload: Record<string, unknown> }[] } }).__proxyKernel.calls.filter(c => c.method === "Proxy.Update"));
  expect(requests[0].payload.credentials).toEqual({ action: "keep" });
  expect(requests[0].payload.expectedRevision).toBe(1);
});

test("kernel prepare uses explicit trusted source; persistence-pending isn't terminal and task survives hide", async ({ page }) => {
  await proxyKernelBridge(page);
  await page.goto("/#/kernels");
  await page.getByRole("button", { name: "准备精确内核", exact: true }).click();
  const prepare = page.getByRole("dialog", { name: "安装精确内核", exact: true });
  await expect(prepare.getByLabel("精确发行版本")).toHaveValue("");
  await prepare.getByLabel("归档来源").selectOption("local");
  await prepare.getByLabel("精确发行版本").fill("151.0.9000.11");
  await prepare.getByLabel("预期归档 SHA-256").fill("a".repeat(64));
  await prepare.getByRole("button", { name: "选择可信 ZIP", exact: true }).click();
  await expect(prepare.getByRole("button", { name: "安装并核验", exact: true })).toBeDisabled();
  await prepare.getByLabel("我已核对来源和摘要", { exact: false }).check();
  await prepare.getByRole("button", { name: "安装并核验", exact: true }).click();
  const progress = page.getByRole("dialog", { name: "内核任务", exact: true });
  await expect(progress).toContainText("隔离探测实际身份和参数");
  await page.evaluate(() => (window as unknown as { __proxyKernel: { finish(state: string, pending: boolean): void } }).__proxyKernel.finish("completed", true));
  await expect(progress).toContainText("结果待保存");
  await progress.getByRole("button", { name: "关闭", exact: true }).click();
  await expect(page.getByRole("button", { name: "准备精确内核", exact: true })).toBeDisabled();
  await page.getByRole("button", { name: "查看内核任务", exact: false }).click();
  await page.evaluate(() => (window as unknown as { __proxyKernel: { finish(state: string, pending: boolean): void } }).__proxyKernel.finish("failed", false));
  await expect(progress).toContainText("KERNEL_INTEGRITY_FAILED");
  await progress.getByRole("button", { name: "关闭", exact: true }).click();
  await expect(page.getByRole("button", { name: "重试同一精确构建任务", exact: true })).toBeEnabled();
});

test("kernel prepare accepted receipt survives a rejected workspace refresh without replay", async ({ page }) => {
  await proxyKernelBridge(page);
  await openProxyKernelHarness(page, "kernels");
  await page.getByRole("button", { name: "准备精确内核", exact: true }).click();
  const prepare = page.getByRole("dialog", { name: "安装精确内核", exact: true });
  await prepare.getByLabel("精确发行版本").fill("151.0.9000.11");
  await prepare.getByLabel("预期归档 SHA-256").fill("a".repeat(64));
  await page.evaluate(async () => {
    const modulePath = "/src/application/wails-adapter.ts";
    const { WailsAdapter } = await import(modulePath);
    const refresh = WailsAdapter.prototype.refresh;
    WailsAdapter.prototype.refresh = async function () {
      WailsAdapter.prototype.refresh = refresh;
      throw new Error("synthetic rejected refresh after acceptance");
    };
  });
  await prepare.getByRole("button", { name: "安装并核验", exact: true }).click();
  const progress = page.getByRole("dialog", { name: "内核任务", exact: true });
  await expect(progress).toContainText("synthetic-task-1");
  await expect(progress).toContainText("任务已受理；工作区刷新未确认");
  await progress.getByRole("button", { name: "关闭", exact: true }).click();
  await expect(page.getByRole("button", { name: "核实原内核请求", exact: true })).toHaveCount(0);
  await page.getByRole("link", { name: "代理管理", exact: true }).click();
  await page.getByRole("link", { name: "内核管理", exact: true }).click();
  await page.getByRole("button", { name: "查看内核任务", exact: false }).click();
  await expect(progress).toContainText("synthetic-task-1");
  const installs = await page.evaluate(() => (window as unknown as { __proxyKernel: { calls: { method: string }[] } }).__proxyKernel.calls.filter(call => call.method === "Kernel.Install"));
  expect(installs).toHaveLength(1);
});

test("demo exported import shares controller with environment return, preserves draft and masks preview", async ({ page }) => {
  await openProxyKernelHarness(page, "environments");
  await page.getByLabel("环境名称").fill("合成未保存名称");
  await page.getByRole("button", { name: "从环境草稿导入代理", exact: true }).click();
  const dialog = page.getByRole("dialog", { name: "批量添加代理", exact: true });
  await dialog.getByLabel("原始代理导入文本").fill("socks5://synthetic:only@203.0.113.55:1080\nftp://203.0.113.1:21");
  await dialog.getByRole("button", { name: "解析预览", exact: true }).click();
  await expect(dialog.getByLabel("保存第2行")).toBeDisabled();
  await expect(dialog.locator("tbody")).not.toContainText("synthetic:only");
  await page.keyboard.press("Escape");
  await expect(page.getByLabel("环境名称")).toHaveValue("合成未保存名称");
  await expect(page.getByRole("button", { name: "从环境草稿导入代理", exact: true })).toBeFocused();
  await page.getByRole("button", { name: "从环境草稿导入代理", exact: true }).click();
  await expect(dialog.getByLabel("原始代理导入文本")).toHaveValue("socks5://synthetic:only@203.0.113.55:1080\nftp://203.0.113.1:21");
  await dialog.getByRole("button", { name: "保存所选有效行", exact: true }).click();
  await expect(dialog.getByLabel("原始代理导入文本")).toHaveValue("ftp://203.0.113.1:21");
  await dialog.getByRole("button", { name: "返回环境配置", exact: true }).click();
  await page.getByRole("link", { name: "代理管理", exact: true }).click();
  await page.getByRole("button", { name: "批量添加代理", exact: true }).click();
  await expect(dialog.getByLabel("原始代理导入文本")).toHaveValue("ftp://203.0.113.1:21");
});

test("file import retains UTF-8 input and filename across hide, traps Tab, and Clear is explicit", async ({ page }) => {
  await proxyKernelBridge(page);
  await openProxyKernelHarness(page);
  await page.getByRole("button", { name: "代理添加方式", exact: true }).click();
  await page.getByRole("menuitem", { name: "导入文本文件", exact: true }).click();
  const file = page.getByRole("dialog", { name: "批量导入代理", exact: true });
  const text = "socks5://203.0.113.55:1080\ninvalid-line";
  await file.getByLabel("选择UTF-8文本文件").setInputFiles({ name: "synthetic-proxies.txt", mimeType: "text/plain", buffer: Buffer.from(text) });
  await expect(file).toContainText("synthetic-proxies.txt");
  await file.getByLabel("选择UTF-8文本文件").setInputFiles({ name: "synthetic-invalid.txt", mimeType: "text/plain", buffer: Buffer.from([0xff, 0x80]) });
  await expect(file.locator("footer")).toContainText("文本文件需为有效 UTF-8");
  await expect(file).toContainText("synthetic-proxies.txt");
  await page.keyboard.press("Tab");
  expect(await file.evaluate(element => element.contains(document.activeElement))).toBe(true);
  expect(await page.locator(".main-shell").evaluate(element => element.closest("[inert]") !== null)).toBe(true);
  await page.keyboard.press("Escape");
  await page.getByRole("link", { name: "内核管理", exact: true }).click();
  await page.getByRole("link", { name: "代理管理", exact: true }).click();
  await page.getByRole("button", { name: "批量添加代理", exact: true }).click();
  const preview = page.getByRole("dialog", { name: "批量添加代理", exact: true });
  await expect(preview.getByLabel("原始代理导入文本")).toHaveValue(text);
  await preview.getByRole("button", { name: "选择文本文件", exact: true }).click();
  await expect(file).toContainText("synthetic-proxies.txt");
  await file.getByRole("button", { name: "清除输入与预览", exact: true }).click();
  await expect(file).toContainText("尚未选择文件");
  await expect(file.getByRole("button", { name: "继续解析预览", exact: true })).toBeDisabled();
});

test("unknown native edit replays the original revision and new credentials after route return", async ({ page }) => {
  await proxyKernelBridge(page);
  await openProxyKernelHarness(page);
  await page.evaluate(() => (window as unknown as { __proxyKernel: { setUpdateOutcome(value: string): void } }).__proxyKernel.setUpdateOutcome("unknown"));
  await page.getByRole("button", { name: "编辑代理 合成 SOCKS", exact: true }).click();
  const edit = page.getByRole("dialog", { name: "修改代理", exact: true });
  await edit.getByLabel("认证处理").selectOption("replace");
  await edit.getByLabel("新用户名（不回显旧值）").fill("synthetic-only");
  await edit.getByLabel("新密码").fill("synthetic-secret");
  await edit.getByRole("button", { name: "确认保存", exact: true }).click();
  await expect(edit.getByRole("status")).toContainText("NATIVE_UNAVAILABLE");
  await expect(edit.getByLabel("主机名或IP")).toBeDisabled();
  await page.keyboard.press("Escape");
  await expect(edit).toHaveCount(0);
  await page.getByRole("link", { name: "内核管理", exact: true }).click();
  await page.getByRole("link", { name: "代理管理", exact: true }).click();
  await expect(edit.getByLabel("新密码")).toHaveValue("synthetic-secret");
  await expect(edit.getByLabel("新密码")).toHaveAttribute("type", "password");
  await edit.getByRole("button", { name: "核实原保存请求", exact: true }).click();
  await expect(edit).toHaveCount(0);
  const calls = await page.evaluate(() => (window as unknown as { __proxyKernel: { calls: { method: string; payload: Record<string, unknown> }[] } }).__proxyKernel.calls.filter(call => call.method === "Proxy.Update"));
  expect(calls).toHaveLength(2);
  expect(calls[1].payload).toEqual(calls[0].payload);
  expect(calls[0].payload.expectedRevision).toBe(1);
});

test("native proxy phases and pending persistence remain owned across routes, failure keeps binding", async ({ page }) => {
  await proxyKernelBridge(page);
  await openProxyKernelHarness(page);
  const row = page.getByRole("row").filter({ hasText: "合成 SOCKS" });
  await row.getByRole("button", { name: "检查", exact: true }).click();
  await expect(row).toContainText("隧道与认证");
  await page.getByRole("link", { name: "内核管理", exact: true }).click();
  await page.getByRole("link", { name: "代理管理", exact: true }).click();
  await expect(row).toContainText("隧道与认证");
  await page.evaluate(() => (window as unknown as { __proxyKernel: { finish(state: string, pending: boolean): void } }).__proxyKernel.finish("completed", true));
  await expect(row).toContainText("结果待保存");
  await expect(row.getByRole("button", { name: "取消检查", exact: true })).toBeDisabled();
  await page.evaluate(() => (window as unknown as { __proxyKernel: { finish(state: string, pending: boolean): void } }).__proxyKernel.finish("failed", false));
  await expect(page.getByRole("status")).toContainText("PROXY_AUTH_FAILED");
  await expect(row.getByRole("button", { name: "检查", exact: true })).toBeEnabled();
  const environment = await page.evaluate(() => (window as unknown as { __proxyKernel: { view(): { state: { environments: { proxyId: string; seed: string }[] } } } }).__proxyKernel.view().state.environments[0]);
  expect(environment).toMatchObject({ proxyId: "synthetic-proxy-a", seed: "172600001" });
});

test("selecting an older failed kernel task cannot retry a different task's request", async ({ page }) => {
  await proxyKernelBridge(page);
  await openProxyKernelHarness(page, "kernels");
  await page.getByRole("button", { name: "准备精确内核", exact: true }).click();
  await page.getByLabel("精确发行版本").fill("151.0.9000.11");
  await page.getByLabel("预期归档 SHA-256").fill("a".repeat(64));
  await page.getByRole("button", { name: "安装并核验", exact: true }).click();
  const progress = page.getByRole("dialog", { name: "内核任务", exact: true });
  await page.evaluate(() => (window as unknown as { __proxyKernel: { finish(state: string, pending: boolean): void } }).__proxyKernel.finish("failed", false));
  await expect(progress).toContainText("KERNEL_INTEGRITY_FAILED");
  await progress.getByRole("button", { name: "关闭", exact: true }).click();
  await page.getByRole("button", { name: "重试同一精确构建任务", exact: true }).click();
  await expect(progress).toContainText("synthetic-task-2");
  await page.evaluate(() => (window as unknown as { __proxyKernel: { finish(state: string, pending: boolean): void } }).__proxyKernel.finish("failed", false));
  await expect(progress).toContainText("KERNEL_INTEGRITY_FAILED");
  await progress.getByRole("button", { name: "关闭", exact: true }).click();
  await page.getByRole("button", { name: "最近内核任务", exact: true }).click();
  const history = page.getByRole("dialog", { name: "最近内核任务（持久记录）", exact: true });
  await history.getByRole("row").filter({ hasText: "synthetic-task-1" }).getByRole("button", { name: "查看任务", exact: true }).click();
  await expect(progress).toContainText("synthetic-task-1");
  await expect(page.getByRole("button", { name: "重试同一精确构建任务", exact: true })).toHaveCount(0);
});

test("a workspace fault stays above the kernel modal and recovery preserves the accepted task", async ({ page }) => {
  await proxyKernelBridge(page);
  await page.goto("/#/kernels");
  await page.getByRole("button", { name: "准备精确内核", exact: true }).click();
  await page.getByLabel("精确发行版本").fill("151.0.9000.11");
  await page.getByLabel("预期归档 SHA-256").fill("a".repeat(64));
  await page.evaluate(() => (window as unknown as { __proxyKernel: { setWorkspaceFailure(value: boolean): void } }).__proxyKernel.setWorkspaceFailure(true));
  await page.getByRole("button", { name: "安装并核验", exact: true }).click();
  const blocker = page.getByRole("alertdialog", { name: "工作区需要处理", exact: true });
  await expect(blocker).toBeVisible();
  expect(await blocker.evaluate(element => element.closest("[inert]") === null)).toBe(true);
  await expect(blocker.getByRole("button").first()).toBeFocused();
  await page.keyboard.press("Tab");
  expect(await blocker.evaluate(element => element.contains(document.activeElement))).toBe(true);
  await page.keyboard.press("Escape");
  await expect(blocker).toBeVisible();
  const recover = blocker.getByRole("button", { name: "重新读取本机工作区", exact: true });
  // Release the synthetic read fault on this exact user click, not in a gap
  // where another already-pending refresh could remove the recovery target.
  await recover.evaluate(element => element.addEventListener("click", () => (window as unknown as { __proxyKernel: { setWorkspaceFailure(value: boolean): void } }).__proxyKernel.setWorkspaceFailure(false), { once: true, capture: true }));
  await recover.click();
  await expect(blocker).toHaveCount(0);
  await expect(page.getByRole("dialog", { name: "内核任务", exact: true })).toContainText("synthetic-task-1");
  const installs = await page.evaluate(() => (window as unknown as { __proxyKernel: { calls: { method: string }[] } }).__proxyKernel.calls.filter(call => call.method === "Kernel.Install"));
  expect(installs).toHaveLength(1);
});
