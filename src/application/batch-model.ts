import type { NativeBatchPage, NativeBatchReport } from "./contract.ts";

export function validBatchReport(report: NativeBatchReport | undefined): boolean {
  return !!report && report.mode === "native" && !!report.planId && ["create", "clone", "assign"].includes(report.kind)
    && [report.total, report.completedCount, report.failedCount, report.notExecutedCount, report.attemptCompletedCount, report.sequence, report.directAssignments, report.sharedProxyAssignments].every(value => Number.isSafeInteger(value) && value >= 0)
    && report.total > 0 && report.completedCount + report.failedCount + report.notExecutedCount === report.total
    && report.attemptCompletedCount <= report.completedCount && report.directAssignments <= report.total && report.sharedProxyAssignments <= report.total;
}
export function mergeBatchPage(previous: NativeBatchPage | undefined, next: NativeBatchPage) {
  return previous?.planId === next.planId && previous.operationId === next.operationId && previous.offset === next.offset && previous.pageSize === next.pageSize && previous.history === next.history && previous.sequence > next.sequence ? previous : next;
}

export function validBatchPage(page: NativeBatchPage): boolean {
  return validBatchReport(page) && Number.isSafeInteger(page.offset) && page.offset >= 0
    && Number.isSafeInteger(page.pageSize) && page.pageSize >= 1 && page.pageSize <= 100
    && Array.isArray(page.items) && page.items.length === Math.max(0, Math.min(page.pageSize, page.total - page.offset))
    && page.items.every((item, index) => item.index === page.offset + index && item.index < page.total && ["not-executed", "completed", "failed"].includes(item.state) && typeof item.name === "string" && typeof item.proxyId === "string" && typeof item.proxyName === "string" && typeof item.newIdentity === "boolean");
}
