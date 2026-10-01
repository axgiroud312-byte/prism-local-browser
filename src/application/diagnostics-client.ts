import type { ApplicationResult, DiagnosticPreview, DiagnosticReceipt, DiagnosticState } from "./contract.ts";

type Invoke = <T>(method: string, payload: unknown) => Promise<ApplicationResult<T>>;
const refused = new Set(["VALIDATION_FAILED", "PREVIEW_EXPIRED", "STORAGE_WRITE_FAILED", "REQUEST_ID_REUSED"]);
const failure = <T>(message: string): ApplicationResult<T> => ({ ok: false, mode: "native", error: { code: "DIAGNOSTICS_RESULT_UNCONFIRMED", message, retryable: true } });
const uuid = /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/;
const hash = /^[0-9a-f]{64}$/;
function validPreview(p: DiagnosticPreview): boolean {
  const r = p?.report, w = r?.workspace;
  return !!p && uuid.test(p.reportId) && hash.test(p.sha256) && Number.isSafeInteger(p.bytes) && p.bytes > 0 && p.bytes <= 1024 * 1024 && Number.isFinite(Date.parse(p.expiresAt)) &&
    r?.format === "prism-local-diagnostics" && r.schemaVersion === 1 && r.application?.signature === "not-checked" &&
    ["available", "unavailable"].includes(r.proxyProtection) && Array.isArray(r.excluded) &&
    !!w && ["available", "partial", "unavailable"].includes(w.status) && Array.isArray(w.operations) && Array.isArray(w.sessions) && Array.isArray(w.kernels) && Array.isArray(w.maintenance) && Array.isArray(w.unavailableSections);
}

// Adapter-owned state survives route changes and the startup-error dialog.
// An unresolved save retains its exact request and never opens another chooser.
export class DiagnosticsClient {
  private state: DiagnosticState = { pending: false, busy: false };
  private request?: { reportId: string; requestId: string };
  private previewFlight?: Promise<ApplicationResult<DiagnosticPreview>>;
  private exportFlight?: Promise<ApplicationResult<DiagnosticReceipt>>;
  private invoke: Invoke;
  private publish: () => void;
  constructor(invoke: Invoke, publish: () => void) { this.invoke = invoke; this.publish = publish; }
  getState = () => this.state;
  private set(patch: Partial<DiagnosticState>) { this.state = { ...this.state, ...patch }; this.publish(); }
  preview = (): Promise<ApplicationResult<DiagnosticPreview>> => {
    if (this.previewFlight) return this.previewFlight;
    if (this.request || this.state.busy) return Promise.resolve(failure("先核实原诊断导出，再生成新报告。"));
    const work = this.read().finally(() => { this.previewFlight = undefined; });
    this.previewFlight = work; return work;
  };
  private async read(): Promise<ApplicationResult<DiagnosticPreview>> {
    this.set({ busy: true, error: undefined });
    let result: ApplicationResult<DiagnosticPreview>;
    try { result = await this.invoke<DiagnosticPreview>("Diagnostics.Preview", {}); }
    catch { result = failure("本机诊断读取未完成，可重新读取。"); }
    if (result.ok && (result.mode !== "native" || !validPreview(result.data))) result = failure("诊断格式不匹配，未显示或导出为有效报告。");
    this.set(result.ok ? { preview: result.data, receipt: undefined, busy: false } : { error: result.error, busy: false });
    return result;
  }
  export = (): Promise<ApplicationResult<DiagnosticReceipt>> => {
    if (this.exportFlight) return this.exportFlight;
    if (this.state.busy || !this.state.preview) return Promise.resolve(failure("请先生成诊断预览。"));
    this.request ??= { reportId: this.state.preview.reportId, requestId: crypto.randomUUID() };
    const work = this.save(this.request, this.state.preview.sha256).finally(() => { this.exportFlight = undefined; });
    this.exportFlight = work; return work;
  };
  private async save(request: { reportId: string; requestId: string }, sha256: string): Promise<ApplicationResult<DiagnosticReceipt>> {
    this.set({ pending: true, busy: true, error: undefined, receipt: undefined });
    let result: ApplicationResult<DiagnosticReceipt>;
    try { result = await this.invoke<DiagnosticReceipt>("Diagnostics.Export", { ...request }); }
    catch { result = failure("保存回执未收到，请核实原导出。"); }
    if (result.ok && (result.mode !== "native" || result.data?.reportId !== request.reportId || !["saved", "cancelled"].includes(result.data.status) || result.data.status === "saved" && result.data.sha256 !== sha256)) result = failure("导出回执未匹配原报告，保持原请求待核实。");
    if (result.ok || !result.ok && result.mode === "native" && refused.has(result.error.code)) this.request = undefined;
    this.set({ busy: false, pending: !!this.request, receipt: result.ok ? result.data : undefined, error: result.ok ? undefined : result.error });
    return result;
  }
  endVerification = async (): Promise<ApplicationResult<DiagnosticReceipt>> => {
    if (this.state.busy || !this.request) return failure("请等待原诊断操作结束。");
    const request = this.request;
    this.set({ busy: true });
    let result: ApplicationResult<DiagnosticReceipt>;
    try { result = await this.invoke<DiagnosticReceipt>("Diagnostics.EndVerification", { ...request }); }
    catch { result = failure("原操作是否结束仍未确认，请稍后核实。"); }
    if (result.ok && (result.mode !== "native" || result.data?.reportId !== request.reportId || !["saved", "cancelled", "unconfirmed"].includes(result.data.status) || result.data.status === "saved" && result.data.sha256 !== this.state.preview?.sha256)) result = failure("结束核实的回执不匹配，保留原请求。");
    if (result.ok) this.request = undefined;
    this.set({ busy: false, pending: !!this.request, receipt: result.ok ? result.data : undefined, error: result.ok ? undefined : result.error });
    return result;
  };
}
