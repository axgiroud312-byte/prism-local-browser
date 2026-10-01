import type { NativeMigrationObservation, NativeMigrationPreview, NativeMigrationRequest, Operation } from "./contract.ts";
const hash = /^[a-f0-9]{64}$/;
const nonempty = (v: unknown): v is string => typeof v === "string" && v.length > 0;
const strings = (v: unknown): v is string[] => Array.isArray(v) && v.every(x => typeof x === "string");
function profile(p: NativeMigrationPreview["before"] | undefined) {
  return !!p && nonempty(p.kernelId) && nonempty(p.seed) && nonempty(p.coreActualVersion) && hash.test(p.coreExecutableSha256) && hash.test(p.configHash) && strings(p.parameters) && Number.isSafeInteger(p.configRevision) && p.configRevision > 0;
}
export function validMigrationPreview(p: NativeMigrationPreview | undefined, environmentId: string, kernelId: string) {
  if (!p || p.mode !== "native" || !nonempty(p.previewId) || p.environmentId !== environmentId || !Number.isSafeInteger(p.expectedRevision) || p.expectedRevision < 1 || !profile(p.before) || !profile(p.after)) return false;
  const capabilities = (v: NativeMigrationPreview["beforeCapabilities"]) => Array.isArray(v) && v.every(c => c && nonempty(c.field) && nonempty(c.status) && nonempty(c.source) && typeof c.note === "string");
  return p.after.kernelId === kernelId && p.before.kernelId !== kernelId && p.before.seed === p.after.seed && p.after.configRevision === p.before.configRevision + 1 && Array.isArray(p.changes) && p.changes.every(c => c && nonempty(c.field) && typeof c.before === "string" && typeof c.after === "string") && capabilities(p.beforeCapabilities) && capabilities(p.afterCapabilities);
}
function observation(v: NativeMigrationObservation | undefined) {
  if (!v) return v === undefined;
  const f = v.fingerprint;
  return !!f && nonempty(f.browserVersion) && nonempty(f.language) && nonempty(f.timezone) && Number.isFinite(f.cpu) && Number.isSafeInteger(f.seed) && typeof f.normalExit === "boolean" && typeof v.cookie === "boolean" && typeof v.localStorage === "boolean" && typeof v.indexedDB === "boolean" && nonempty(v.sampledAt);
}
export function invalidMigrationOperation(op: Operation | null | undefined) {
  if (!op || typeof op.kind !== "string") return true;
  if (op.kind !== "migration") return false;
  const r = op.migrationReport;
  return !r || r.mode !== "native" || !nonempty(r.requestId) || !nonempty(r.previewId) || !nonempty(r.environmentId) || r.environmentId !== op.environmentId || !nonempty(r.seed) || !nonempty(r.oldKernelId) || !nonempty(r.newKernelId) || r.oldKernelId === r.newKernelId || !Number.isSafeInteger(r.sequence) || r.sequence < 1 || op.total !== 1 || typeof r.protected !== "boolean" || typeof r.trialExited !== "boolean" || typeof r.committed !== "boolean" || typeof r.backupVerified !== "boolean" || r.backupVerified && !hash.test(r.archiveSha256) || !observation(r.before) || !observation(r.after) || op.state === "completed" && (!r.committed || !r.backupVerified || !r.trialExited || !op.persistencePending && r.protected) || !op.persistencePending && ["failed", "cancelled"].includes(op.state) && r.protected;
}
export function confirmsMigrationRequest(op: Operation | null | undefined, request?: NativeMigrationRequest) {
  return !!request && !!op && op.kind === "migration" && !invalidMigrationOperation(op) && !(op.persistencePending && op.stage === "acceptance-pending") && op.migrationReport?.requestId === request.requestId && op.migrationReport.previewId === request.previewId;
}
