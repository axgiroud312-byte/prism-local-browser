// Operate the production Windows executable via UIA; do not enable debugging.
import assert from "node:assert/strict";
import { spawn } from "node:child_process";
import { createHash, randomUUID } from "node:crypto";
import { mkdir, readFile, writeFile } from "node:fs/promises";
import { resolve } from "node:path";

if (process.platform !== "win32") throw new Error("Kernel desktop verification requires Windows.");
const root = resolve(import.meta.dirname, "..");
const evidenceRoot = resolve(root, "output/goal/T04");
const resumeAfterOfficial = process.argv.includes("--resume-after-official");
const resumeAfterLocal = process.argv.includes("--resume-after-local");
const workspaceRoot = process.env.PRISM_VERIFY_KERNEL_ROOT || (resumeAfterOfficial || resumeAfterLocal ? await readFile(resolve(evidenceRoot, "verification-root.txt"), "utf8") : resolve(root, ".appdata/verification", `T04-${randomUUID()}`));
const archive = resolve(process.env.PRISM_KERNEL_ARCHIVE || resolve(root, ".tools/fingerprint-chromium/148.0.7778.215/ungoogled-chromium_148.0.7778.215-1.1_windows_x64.zip"));
const actualArchiveDigest = createHash("sha256").update(await readFile(archive)).digest("hex");
assert.equal(actualArchiveDigest, "9ef3f471b7a6641b4224532522b29141ce3746e27d55788d88e2fd951f362579", "Explicitly selected real ZIP changed; do not execute");
const exe = resolve(root, "build/bin/prism-browser.exe");
const executableSha256 = createHash("sha256").update(await readFile(exe)).digest("hex");
await mkdir(evidenceRoot, { recursive: true });
await writeFile(resolve(evidenceRoot, "verification-root.txt"), workspaceRoot); // ignored, private location for failure recovery only
const command = '& ([scriptblock]::Create([IO.File]::ReadAllText($env:PRISM_VERIFY_SCRIPT)))';
const child = spawn("powershell.exe", ["-NoProfile", "-NonInteractive", "-OutputFormat", "Text", "-ExecutionPolicy", "Bypass", "-EncodedCommand", Buffer.from(command, "utf16le").toString("base64")], {
  cwd: root, stdio: "inherit", env: { ...process.env, PRISM_VERIFY_SCRIPT: resolve(root, "scripts/verify-desktop-ui.ps1"), PRISM_VERIFY_EXE: exe, PRISM_WORKSPACE_ROOT: workspaceRoot, PRISM_VERIFY_EVIDENCE: evidenceRoot, PRISM_VERIFY_KERNEL_MODE: "1", PRISM_KERNEL_ARCHIVE: archive, PRISM_KERNEL_RESUME: resumeAfterLocal ? "after-local" : resumeAfterOfficial ? "after-official" : "" },
});
const code = await new Promise((resolve, reject) => { child.once("error", reject); child.once("exit", resolve); });
assert.equal(code, 0, "Actual kernel UI flow failed; inspect output/goal/T04/ and retain fixture for diagnosis.");
const evidence = JSON.parse(await readFile(resolve(evidenceRoot, "kernel-uia-verification.json"), "utf8"));
assert.ok(evidence.processes.every(process => process.normalExit));
assert.ok(Object.values(evidence.checks).every(Boolean));
await writeFile(resolve(evidenceRoot, "kernel-desktop-verification.json"), JSON.stringify({ ...evidence, executableSha256, actualArchiveDigest }, null, 2));
console.log("PASS: production UIA exact official/local installs, failure isolation, explicit binding, normal reopen, file hashes and reference/deletion protection.");
