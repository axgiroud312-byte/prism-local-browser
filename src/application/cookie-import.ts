import { mergeOperation, operationIsTerminal, type Operation, type RuntimeSession } from "./contract.ts";

// This only enables an explicit request. Actual network protection and saved
// binding are rechecked by Runtime.Start; a proxy never needs direct consent.
export function cookieStartupAllowed(proxyId: string, confirmedDirect: boolean, session?: RuntimeSession): boolean {
  return (!!proxyId || confirmedDirect) && !session?.needsReconcile && !session?.persistencePending && !session?.resourcesPending;
}

export function cookieWriteAllowed(session?: RuntimeSession): boolean {
  // A running process normally holds resources. That is not a cleanup fault.
  return session?.state === "running" && session.canControl && !session.needsReconcile && !session.persistencePending && !session.networkFault;
}

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
