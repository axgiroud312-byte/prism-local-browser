import type { ApplicationResult, ApplicationService } from "./contract.ts";

export interface RuntimeStartPlanItem {
  environmentId: string; name: string; expectedRevision: number; networkPolicy: "direct" | "proxy";
}

// A paged workspace is not a cache of hidden selections. Resolve every exact
// target through the native service; missing records never mean direct access.
export async function readRuntimeStartPlan(application: ApplicationService, ids: string[]): Promise<ApplicationResult<RuntimeStartPlanItem[]>> {
  if (application.mode !== "native") return { ok: false, mode: application.mode, error: { code: "CAPABILITY_UNSUPPORTED", message: "真实启动计划只接受本机环境。", retryable: false } };
  const items: RuntimeStartPlanItem[] = [];
  for (const environmentId of [...new Set(ids)]) {
    const result = await application.previewEnvironment({ kind: "edit", sourceId: environmentId });
    if (!result.ok) return result;
    try {
      const { environment, expectedRevision } = result.data;
      if (result.mode !== "native" || environment.id !== environmentId || !Number.isSafeInteger(expectedRevision) || !expectedRevision || expectedRevision < 1 || typeof environment.proxyId !== "string") return { ok: false, mode: "native", error: { code: "REVISION_CONFLICT", message: "所选环境的身份、修订或网络策略无法确认；未把缺失记录当作直连，请重新读取。", retryable: true } };
      items.push({ environmentId, name: environment.name, expectedRevision, networkPolicy: environment.proxyId ? "proxy" : "direct" });
    } finally { await application.discardPreview(result.data.previewId); }
  }
  return { ok: true, mode: "native", data: items };
}
