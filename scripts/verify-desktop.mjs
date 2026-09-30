// Run Windows UI Automation against the actual production executable, with no debug port.
import assert from "node:assert/strict";
import { spawn } from "node:child_process";
import { createHash, randomUUID } from "node:crypto";
import { mkdir, readFile, writeFile } from "node:fs/promises";
import { resolve } from "node:path";

if (process.platform !== "win32") throw new Error("Actual desktop verification requires Windows.");
const root = resolve(import.meta.dirname, "..");
const evidenceRoot = resolve(root, "output/goal/T02");
const workspaceRoot = resolve(root, ".appdata/verification", `T02-${randomUUID()}`);
const exe = resolve(root, "build/bin/prism-browser.exe");
await mkdir(evidenceRoot, { recursive: true });
const executableSha256 = createHash("sha256").update(await readFile(exe)).digest("hex");
// Powershell 5 reads UTF-8 scripts without a BOM incorrectly; decode explicitly, then execute.
const command = '& ([scriptblock]::Create([IO.File]::ReadAllText($env:PRISM_VERIFY_SCRIPT)))';
const child = spawn("powershell.exe", ["-NoProfile", "-NonInteractive", "-OutputFormat", "Text", "-ExecutionPolicy", "Bypass", "-EncodedCommand", Buffer.from(command, "utf16le").toString("base64")], {
  cwd: root, stdio: "inherit", env: { ...process.env, PRISM_VERIFY_SCRIPT: resolve(root, "scripts/verify-desktop-ui.ps1"), PRISM_VERIFY_EXE: exe, PRISM_WORKSPACE_ROOT: workspaceRoot, PRISM_VERIFY_EVIDENCE: evidenceRoot },
});
const code = await new Promise((resolve, reject) => { child.once("error", reject); child.once("exit", resolve); });
assert.equal(code, 0, "Actual Windows UI verification failed.");
const evidence = JSON.parse(await readFile(resolve(evidenceRoot, "uia-verification.json"), "utf8"));
assert.equal(evidence.database.name, "桌面重开验证");
assert.equal(evidence.database.revision, 2);
assert.equal(evidence.database.configuration.group, "合成持久组");
assert.equal(evidence.database.configuration.width, 1440);
assert.equal(evidence.database.configuration.height, 900);
assert.equal(evidence.database.configuration.restoreTabs, false);
assert.equal(evidence.database.configuration.urls, "https://example.test/desktop");
assert.equal(evidence.processes.length, 2);
assert.ok(evidence.processes.every(p => p.normalExit));
await writeFile(resolve(evidenceRoot, "desktop-verification.json"), JSON.stringify({ ...evidence, executableSha256 }, null, 2));
console.log("PASS: production Wails exe operated by Windows UI Automation; SQLite identity/preferences identical after two normal closes and a reopen. No debugging endpoint enabled. Evidence: output/goal/T02/");
