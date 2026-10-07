import type { NativeBackupExportRequest, NativeBackupReport, Operation } from "./contract.ts";

export function validBackupReport(report: NativeBackupReport | undefined): report is NativeBackupReport {
  if (!report || !report.requestId || report.mode !== "native" || report.format !== "prism-local-backup" || report.schemaVersion !== 1 || !["all", "selected"].includes(report.scope) || report.credentials !== "windows-current-user-dpapi" || report.browserData !== "sensitive-same-user-not-portable" || report.kernelBinariesIncluded !== false || typeof report.name !== "string" || /[\\/]/.test(report.name)) return false;
  if (![report.environmentCount, report.copiedEnvironmentCount, report.fileCount, report.byteCount, report.sequence].every(value => Number.isSafeInteger(value) && value >= 0) || report.copiedEnvironmentCount > report.environmentCount) return false;
  return typeof report.published === "boolean" && (!report.published || report.copiedEnvironmentCount === report.environmentCount && /^[a-f0-9]{64}$/.test(report.archiveSha256 ?? "") && /^[a-f0-9]{64}$/.test(report.manifestSha256 ?? ""));
}
export const invalidBackupOperation = (operation: Operation) => operation.kind === "backup-export" && (!validBackupReport(operation.backupReport) || operation.total !== operation.backupReport?.environmentCount || operation.state === "completed" && !operation.backupReport?.published);

// Only a verified observation of this exact request consumes its output grant.
// A refusal, unrelated history or disappearance of pending state does not.
export function confirmsBackupRequest(operation: Operation | undefined, request: NativeBackupExportRequest | undefined): boolean {
  return !!request && !!operation && !!operation.id && operation.kind === "backup-export" && !operation.persistencePending && !invalidBackupOperation(operation) && operation.backupReport?.requestId === request.requestId && operation.backupReport.scope === request.scope;
}
