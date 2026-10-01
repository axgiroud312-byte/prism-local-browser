// Read-only, independent checks of the synthetic T04 UI fixture. No RPC calls.
import assert from "node:assert/strict";
import { createHash } from "node:crypto";
import { createReadStream } from "node:fs";
import { readdir, lstat } from "node:fs/promises";
import { resolve, join } from "node:path";
import { DatabaseSync } from "node:sqlite";

const root = resolve(process.argv[2]);
const db = new DatabaseSync(join(root, "app.db"), { readOnly: true });
let view;
try {
  assert.equal(db.prepare("PRAGMA user_version").get().user_version, 3);
  assert.deepEqual(db.prepare("PRAGMA foreign_key_check").all(), []);
  const records = db.prepare("SELECT k.status,e.record_json FROM kernel_evidence e JOIN kernels k ON k.id=e.kernel_id ORDER BY k.rowid DESC").all().map(row => ({ ...JSON.parse(row.record_json), status: row.status }));
  const environments = db.prepare("SELECT e.id,e.kernel_id,e.revision,e.fingerprint_id,e.user_data_ref,f.kernel_id AS fingerprint_kernel_id,f.seed,f.config_json,f.config_revision,r.profile_json FROM environments e JOIN fingerprints f ON f.id=e.fingerprint_id LEFT JOIN fingerprint_revisions r ON r.fingerprint_id=f.id AND r.revision=f.config_revision ORDER BY e.code").all().map(row => {
    const config = JSON.parse(row.config_json);
    const profile = JSON.parse(row.profile_json);
    assert.equal(row.kernel_id, row.fingerprint_kernel_id);
    assert.equal(config.coreId, row.kernel_id);
    assert.equal(config.seed, String(row.seed));
    assert.equal(profile.configRevision, row.config_revision);
    assert.equal(profile.seed, config.seed);
    assert.equal(profile.kernelId, row.kernel_id);
    assert.equal(row.user_data_ref, `environments/${row.id}/user-data`);
    return { id: row.id, kernelId: row.kernel_id, revision: row.revision, seed: String(row.seed), configuration: config, fingerprint: profile, userDataRef: row.user_data_ref };
  });
  const operations = db.prepare("SELECT result_json FROM operations WHERE json_extract(result_json,'$.kind') LIKE 'kernel-%' ORDER BY rowid DESC").all().map(row => JSON.parse(row.result_json));
  assert.equal(db.prepare("SELECT COUNT(*) AS count FROM proxies").get().count, 0);
  for (const record of records) {
    assert.equal(record.installPath, `kernels/${record.id}`);
    assert.equal(record.source.tag, record.version);
    assert.equal(record.architecture, "amd64");
    assert.equal(record.files[record.executableRelativePath], record.executableSha256);
    assert.ok(record.report.observations.length >= 3);
    assert.ok(record.report.observations.every(sample => sample.normalExit && sample.browserVersion === record.version));
  }
  view = { schemaVersion: 3, records, environments, operations };
} finally { db.close(); }

if (process.argv.includes("--hash")) {
  for (const record of view.records) {
    if (record.status !== "verified") continue;
    const directory = join(root, "kernels", record.id);
    const actualFiles = [];
    async function walk(path, prefix = "") {
      for (const entry of await readdir(path, { withFileTypes: true })) {
        const relative = prefix + entry.name;
        const child = join(path, entry.name);
        assert.equal((await lstat(child)).isSymbolicLink(), false);
        if (entry.isDirectory()) await walk(child, `${relative}/`);
        else actualFiles.push(relative);
      }
    }
    await walk(directory);
    assert.deepEqual(actualFiles.sort(), Object.keys(record.files).sort());
    for (const file of actualFiles) {
      const digest = createHash("sha256");
      for await (const chunk of createReadStream(join(directory, file))) digest.update(chunk);
      assert.equal(digest.digest("hex"), record.files[file], `Installed file changed: ${file}`);
    }
  }
  const staging = await readdir(join(root, "staging"));
  assert.equal(staging.length, 0, "Completed kernel tasks left temporary probe/archive resources");
  view.independentFilesVerified = true;
}
console.log(JSON.stringify(view));
