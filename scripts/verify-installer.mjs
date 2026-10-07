// Decode the UTF-8 PowerShell script explicitly for Windows PowerShell 5 on all locales.
import assert from "node:assert/strict";
import { spawn } from "node:child_process";
import { resolve } from "node:path";
if (process.platform !== "win32") throw new Error("Installer verification requires Windows.");
const script = resolve(import.meta.dirname, "verify-installer.ps1");
const flags = process.argv.slice(2);
const known = new Set(["--silent-wizard", "--no-clicks", "--candidate"]);
for (const flag of flags) if (!known.has(flag) && !/^--(?:first|second)-revision=[1-9][0-9]{0,4}$/.test(flag)) throw new Error(`Unknown verification option: ${flag}`);
const revisions = ["first", "second"].map(key => {
  const value = flags.find(flag => flag.startsWith(`--${key}-revision=`))?.split("=")[1];
  if (value && Number(value) > 65535) throw new Error("Revision must be <= 65535.");
  return value ? ` -${key === "first" ? "First" : "Second"}Revision ${value}` : "";
}).join("");
const command = '& ([scriptblock]::Create([IO.File]::ReadAllText($env:PRISM_INSTALLER_VERIFY)))' + (flags.includes("--silent-wizard") ? ' -SilentWizard' : '') + (flags.includes("--no-clicks") ? ' -NoClicks' : '') + (flags.includes("--candidate") ? ' -Candidate' : '') + revisions;
const child = spawn("powershell.exe", ["-NoProfile", "-NonInteractive", "-OutputFormat", "Text", "-ExecutionPolicy", "Bypass", "-EncodedCommand", Buffer.from(command, "utf16le").toString("base64")], { stdio: "inherit", env: { ...process.env, PRISM_INSTALLER_VERIFY: script } });
const code = await new Promise((resolve, reject) => { child.once("error", reject); child.once("exit", resolve); });
assert.equal(code, 0, "Actual installer verification failed; synthetic fixture retained. See output/goal/T03/ or V1-final/install-no-clicks/.");
