import { mergeOperation, operationIsTerminal, type Operation } from "./contract.ts";

// Workspace records arrive newest first. Keep an in-flight observation across
// stale snapshots, but adopt a newly accepted task after closing/reopening.
export function currentCookieOperation(previous: Operation | undefined, operations: Operation[], environmentId: string, previewId?: string): Operation | undefined {
  const scoped = operations.filter(operation => operation.kind === "cookie-import" && operation.environmentId === environmentId);
  const related = (operation: Operation) => !previewId || !operationIsTerminal(operation) || operation.cookieReport?.previewId === previewId;
  let current = previous?.environmentId === environmentId && related(previous) ? previous : undefined;
  if (current) {
    const same = scoped.find(operation => operation.id === current?.id);
    if (same) current = mergeOperation(current, same);
    if (!operationIsTerminal(current)) return current;
  }
  const latest = scoped.find(operation => !operationIsTerminal(operation)) ?? scoped.find(related);
  return latest ? mergeOperation(current?.id === latest.id ? current : undefined, latest) : current;
}
