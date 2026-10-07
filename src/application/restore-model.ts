import type { NativeRestoreRequest, NativeRestorePreview, NativeRestoreSourceState, Operation } from "./contract.ts";

export function validRestorePreview(p: NativeRestorePreview): boolean {
  const hash = /^[0-9a-f]{64}$/;
  return !!p && p.mode === "native" && p.format === "prism-local-backup" && typeof p.previewId === "string" && !!p.previewId && hash.test(p.archiveSha256) && hash.test(p.manifestSha256)
    && [p.environmentCount, p.addCount, p.overwriteCount, p.conflictCount, p.missingKernelCount, p.credentialReentryCount, p.bytes].every(n => Number.isSafeInteger(n) && n >= 0)
    && p.addCount + p.overwriteCount === p.environmentCount && p.canRestore === (p.conflictCount === 0 && p.missingKernelCount === 0) && Array.isArray(p.kernels) && Array.isArray(p.credentials);
}

export function validRestoreSourceState(value: NativeRestoreSourceState, requestId: string): boolean {
  return !!value && value.mode === "native" && value.requestId === requestId && ["selecting", "selected", "cancelled", "failed", "discarded"].includes(value.status)
    && typeof value.preflightRunning === "boolean" && typeof value.cleanupPending === "boolean"
    && (value.status === "selected" ? typeof value.sourceToken === "string" && !!value.sourceToken : !value.sourceToken && !value.preflightRunning && !value.cleanupPending && !value.preview)
    && (!value.preview || !value.preflightRunning && !value.cleanupPending && validRestorePreview(value.preview));
}

export function invalidRestoreOperation(op: Operation): boolean {
  if (op.kind !== "backup-restore") return false;
  const r = op.restoreReport;
  if (!r || r.mode !== "native" || !r.requestId || !r.previewId || !/^[0-9a-f]{64}$/.test(r.archiveSha256)) return true;
  if (r.recoveredAfterRestart !== undefined && typeof r.recoveredAfterRestart !== "boolean" || r.interruptedStage !== undefined && typeof r.interruptedStage !== "string" || r.recoveredAfterRestart && !r.interruptedStage) return true;
  if (![r.sequence, r.environmentCount, r.switchedCount, r.credentialReentryCount].every(n => Number.isSafeInteger(n) && n >= 0) || r.switchedCount > r.environmentCount || op.total !== r.environmentCount) return true;
  if (![r.committed, r.rolledBack, r.protected].every(b => typeof b === "boolean") || r.committed && r.rolledBack) return true;
  if (op.state === "completed" && (!r.committed || r.protected || r.switchedCount !== r.environmentCount || op.persistencePending)) return true;
  return ["failed", "cancelled"].includes(op.state) && !op.persistencePending && r.protected;
}

export function confirmsRestoreRequest(op: Operation | undefined, request: NativeRestoreRequest | undefined): boolean {
  return !!request && !!op && !!op.id && op.kind === "backup-restore" && !invalidRestoreOperation(op) && op.stage !== "acceptance-pending" && op.restoreReport?.requestId === request.requestId && op.restoreReport.previewId === request.previewId && op.restoreReport.archiveSha256 === request.archiveSha256;
}
