// Read-only inspection of the synthetic database made by verify-desktop-ui.ps1.
import assert from "node:assert/strict";
import { DatabaseSync } from "node:sqlite";

const db = new DatabaseSync(process.argv[2], { readOnly: true });
try {
  assert.deepEqual(db.prepare("PRAGMA foreign_key_check").all(), []);
  const schemaVersion = db.prepare("PRAGMA user_version").get().user_version;
  assert.equal(schemaVersion, 5);
  const records = db.prepare("SELECT e.id,e.name,e.revision,e.kernel_id,e.user_data_ref,f.seed,f.config_json,f.config_revision,r.profile_json FROM environments e JOIN fingerprints f ON f.id=e.fingerprint_id LEFT JOIN fingerprint_revisions r ON r.fingerprint_id=f.id AND r.revision=f.config_revision").all();
  assert.ok(records.length <= 1, "Native initialized with demo environments or created duplicates");
  for (const record of records) {
    const profile = JSON.parse(record.profile_json);
    assert.equal(profile.configRevision, record.config_revision);
    assert.equal(profile.seed, String(record.seed));
    assert.equal(profile.kernelId, record.kernel_id);
    assert.equal(record.user_data_ref, `environments/${record.id}/user-data`);
  }
  const kernels = db.prepare("SELECT status FROM kernels").all();
  assert.equal(kernels.length, 1); assert.equal(kernels[0].status, "missing");
  assert.equal(db.prepare("SELECT COUNT(*) AS count FROM proxies").get().count, 0);
  console.log(JSON.stringify(records.length ? { schemaVersion, id: records[0].id, name: records[0].name, revision: records[0].revision, kernelId: records[0].kernel_id, seed: String(records[0].seed), configuration: JSON.parse(records[0].config_json), fingerprint: JSON.parse(records[0].profile_json), userDataRef: records[0].user_data_ref } : { schemaVersion, empty: true }));
} finally { db.close(); }
