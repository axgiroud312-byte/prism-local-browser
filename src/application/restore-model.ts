import type { NativeRestoreRequest, Operation } from "./contract.ts";

export function invalidRestoreOperation(op: Operation): boolean {
  if (op.kind !== "backup-restore") return false;
  const r = op.restoreReport;
  if (!r || r.mode !== "native" || !r.requestId || !r.previewId || !/^[0-9a-f]{64}$/.test(r.archiveSha256)) return true;
  if (![r.sequence, r.environmentCount, r.switchedCount, r.credentialReentryCount].every(n => Number.isSafeInteger(n) && n >= 0) || r.switchedCount > r.environmentCount || op.total !== r.environmentCount) return true;
  if (![r.committed, r.rolledBack, r.protected].every(b => typeof b === "boolean") || r.committed && r.rolledBack) return true;
  if (op.state === "completed" && (!r.committed || r.protected || r.switchedCount !== r.environmentCount || op.persistencePending)) return true;
  return ["failed", "cancelled"].includes(op.state) && !op.persistencePending && r.protected;
}

export function confirmsRestoreRequest(op: Operation | undefined, request: NativeRestoreRequest | undefined): boolean {
  return !!request && !!op && !!op.id && op.kind === "backup-restore" && !invalidRestoreOperation(op) && op.stage !== "acceptance-pending" && op.restoreReport?.requestId === request.requestId && op.restoreReport.previewId === request.previewId && op.restoreReport.archiveSha256 === request.archiveSha256;
}
