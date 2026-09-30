// Read-only inspection of the synthetic database made by verify-desktop-ui.ps1.
import assert from "node:assert/strict";
import { DatabaseSync } from "node:sqlite";

const db = new DatabaseSync(process.argv[2], { readOnly: true });
try {
  assert.deepEqual(db.prepare("PRAGMA foreign_key_check").all(), []);
  const schemaVersion = db.prepare("PRAGMA user_version").get().user_version;
  assert.equal(schemaVersion, 1);
  const records = db.prepare("SELECT e.id,e.name,e.revision,e.kernel_id,f.seed,f.config_json FROM environments e JOIN fingerprints f ON f.id=e.fingerprint_id").all();
  assert.ok(records.length <= 1, "Native initialized with demo environments or created duplicates");
  const kernels = db.prepare("SELECT status FROM kernels").all();
  assert.equal(kernels.length, 1); assert.equal(kernels[0].status, "missing");
  assert.equal(db.prepare("SELECT COUNT(*) AS count FROM proxies").get().count, 0);
  console.log(JSON.stringify(records.length ? { schemaVersion, id: records[0].id, name: records[0].name, revision: records[0].revision, kernelId: records[0].kernel_id, seed: String(records[0].seed), configuration: JSON.parse(records[0].config_json) } : { schemaVersion, empty: true }));
} finally { db.close(); }
