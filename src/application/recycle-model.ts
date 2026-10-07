import type { NativeRecyclePage, NativeRecyclePageRequest, NativeRecycleRequest, Operation } from "./contract.ts";

export function invalidRecycleOperation(op: Operation): boolean {
  if (op.kind !== "recycle") return false;
  const r = op.recycleReport;
  if (!r || r.mode !== "native" || !["remove", "restore", "purge"].includes(r.action) || !r.requestId || !r.previewId || typeof r.protected !== "boolean") return true;
  if (![op.total, r.sequence, r.completed, r.failed, r.notExecuted].every(n => Number.isSafeInteger(n) && n >= 0) || r.sequence < 1 || r.completed + r.failed + r.notExecuted !== op.total) return true;
  if (r.protected && !op.persistencePending) return true;
  return op.state === "completed" && (r.completed !== op.total || r.protected || !!op.persistencePending);
}
export function confirmsRecycleRequest(op: Operation, request: NativeRecycleRequest | undefined): boolean {
  return !!request && op.kind === "recycle" && !invalidRecycleOperation(op) && op.stage !== "acceptance-pending" && op.recycleReport?.requestId === request.requestId && op.recycleReport.previewId === request.previewId;
}
export function validRecyclePage(page: NativeRecyclePage, request?: NativeRecyclePageRequest): boolean {
  if (!page || typeof page !== "object" || page.mode !== "native" || ![page.offset, page.pageSize, page.total].every(n => Number.isSafeInteger(n) && n >= 0) || page.pageSize < 1 || page.pageSize > 100 || !Array.isArray(page.items) || page.offset > page.total || page.items.length !== Math.min(page.pageSize, page.total - page.offset)) return false;
  // ReadPage returns exactly one view: the live list, a preview, or a frozen
  // operation. A short page or an unrelated view must not hide unexecuted items.
  if (page.preview && page.operation) return false;
  if (request && (page.offset !== request.offset || page.pageSize !== request.pageSize || (page.preview?.previewId ?? "") !== (request.previewId ?? "") || (page.operation?.id ?? "") !== (request.operationId ?? ""))) return false;
  if (new Set(page.items.map(item => item?.id)).size !== page.items.length || page.items.some(item => !item || !item.id || !item.environmentId || typeof item.name !== "string" || typeof item.seed !== "string" || typeof item.kernelId !== "string" || !item.kernelId || typeof item.dataPresent !== "boolean" || typeof item.backupRecorded !== "boolean" || !Number.isSafeInteger(item.revision) || item.revision < 1 || !["pending", "recycled", "restored", "purged", "failed", "protected"].includes(item.state))) return false;
  if (page.preview && (page.preview.mode !== "native" || !page.preview.previewId || !["remove", "restore", "purge"].includes(page.preview.action) || page.preview.total !== page.total || ![page.preview.dataCount, page.preview.backupCount].every(n => Number.isSafeInteger(n) && n >= 0 && n <= page.total))) return false;
  return !page.operation || page.operation.kind === "recycle" && page.operation.total === page.total && !invalidRecycleOperation(page.operation);
}
