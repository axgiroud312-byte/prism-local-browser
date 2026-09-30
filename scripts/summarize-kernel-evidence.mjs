// Project already-recorded synthetic evidence. This command never launches a UI
// or kernel, clicks controls, or modifies a workspace/database.
import assert from "node:assert/strict";
import { createHash } from "node:crypto";
import { execFileSync } from "node:child_process";
import { readFile, writeFile } from "node:fs/promises";
import { resolve } from "node:path";

const [closureFile, fixturePointer, outputFile] = process.argv.slice(2);
assert.ok(closureFile && fixturePointer && outputFile, "Provide closure JSON, synthetic fixture pointer, and output JSON.");
const root = resolve(import.meta.dirname, "..");
const closure = JSON.parse(await readFile(resolve(closureFile), "utf8"));
const fixture = await readFile(resolve(fixturePointer), "utf8");
const exeSha256 = createHash("sha256").update(await readFile(resolve(root, "build/bin/prism-browser.exe"))).digest("hex");
assert.equal(closure.executableSha256, exeSha256, "Do not reuse UI closure evidence from another executable.");
assert.ok(Object.values(closure.checks).every(Boolean));
assert.ok(closure.processes.every(process => process.normalExit));
const fresh = JSON.parse(execFileSync(process.execPath, ["--disable-warning=ExperimentalWarning", resolve(root, "scripts/verify-kernel-db.mjs"), fixture, "--hash"], { encoding: "utf8" }));
assert.equal(fresh.records.length, 2);
assert.ok(fresh.operations.some(operation => operation.error?.details?.reason === "asset-unavailable"));
assert.ok(fresh.operations.some(operation => operation.error?.details?.reason === "hash-mismatch"));
const records = fresh.records.map(({ files, ...record }) => ({ ...record, fileCount: Object.keys(files).length, filesManifestSha256: createHash("sha256").update(JSON.stringify(Object.entries(files).sort(([a], [b]) => a.localeCompare(b, "en")))).digest("hex") }));
const reverify = closure.final.operations.find(operation => operation.kind === "kernel-verify" && operation.state === "completed");
assert.ok(reverify?.report);
const evidence = {
  recordedAt: new Date().toISOString(), platform: "windows/amd64", data: "synthetic-only",
  sourceBaseCommit: execFileSync("git", ["rev-parse", "HEAD"], { cwd: root, encoding: "utf8" }).trim(), sourceDirty: true,
  executableSha256: exeSha256,
  completeDesktopClosure: { verifiedAt: closure.verifiedAt, sameExecutableSha256: true, processes: closure.processes, checks: closure.checks, remainingKernelId: closure.final.records[0].id, removedKernelId: closure.localRecord.id, environment: closure.final.environments[0], reverify: { operationId: reverify.id, kernelId: reverify.kernelId, report: reverify.report } },
  freshFinalInstallation: { schemaVersion: fresh.schemaVersion, allInstalledFilesIndependentlyVerified: fresh.independentFilesVerified, stagingEmpty: true, records, environment: fresh.environments[0], operations: fresh.operations.map(operation => ({ id: operation.id, kind: operation.kind, state: operation.state, kernelId: operation.kernelId, completedIds: operation.completedIds, error: operation.error })) },
  supplementaryFreshUIFlow: { state: "stopped", reason: "Shared-desktop input affected the synthetic name check. User requested no further automated clicks; no retry or success claim for this supplementary flow.", completeClosureEvidence: "Use the independently recorded same-executable closure above, not the stopped attempt." },
  remainingBoundaries: ["Normal environment launch and browser-data isolation are T06, not diagnostic sessions.", "148 is an explicitly selected test candidate, not a production recommendation.", "Menu language, font/canvas/audio/compatibility and proxy/leak protection are not verified by this ticket.", "Automatic CI does not perform UI clicking; earlier UI evidence retains its actual date and scope."]
};
const text = JSON.stringify(evidence, null, 2);
assert.ok(!/C:\\\\Users\\\\|file:\/\/\//i.test(text), "Do not publish private paths.");
await writeFile(resolve(outputFile), `${text}\n`);
console.log("PASS: same-executable completed closure and fresh final installed bytes projected; stopped supplementary UI flow remains explicitly stopped.");
