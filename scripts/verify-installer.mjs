// Decode the UTF-8 PowerShell script explicitly for Windows PowerShell 5 on all locales.
import assert from "node:assert/strict";
import { spawn } from "node:child_process";
import { resolve } from "node:path";
if (process.platform !== "win32") throw new Error("Installer verification requires Windows.");
const script = resolve(import.meta.dirname, "verify-installer.ps1");
const command = '& ([scriptblock]::Create([IO.File]::ReadAllText($env:PRISM_INSTALLER_VERIFY)))' + (process.argv.includes("--silent-wizard") ? ' -SilentWizard' : '');
const child = spawn("powershell.exe", ["-NoProfile", "-NonInteractive", "-OutputFormat", "Text", "-ExecutionPolicy", "Bypass", "-EncodedCommand", Buffer.from(command, "utf16le").toString("base64")], { stdio: "inherit", env: { ...process.env, PRISM_INSTALLER_VERIFY: script } });
const code = await new Promise((resolve, reject) => { child.once("error", reject); child.once("exit", resolve); });
assert.equal(code, 0, "Actual installer verification failed; synthetic fixture retained. See output/goal/T03/.");
