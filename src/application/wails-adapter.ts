import type {
  ApplicationResult, ApplicationService, EnvironmentConfiguration, EnvironmentPreview,
  CreateBatchRequest, UpdateEnvironmentRequest, SavedEnvironment, WorkspaceView, Operation, OperationEvent,
  KernelInstallRequest,
  GenerateFingerprintRequest, CommitFingerprintRequest, ProfileRevision, RuntimeSession,
  ProxyConfiguration, ProxyImportPreview, ProxyUpdateRequest, ProxyTargetRequest, NativeProxy,
  CookieCommitRequest, CookieImportPreview, RuntimeStartRequest,
} from "./contract.ts";
import { mergeOperation, operationIsTerminal } from "./contract.ts";

export interface NativeRequest { mode: "native"; method: string; payload: unknown }
export type NativeBridge = <T>(request: NativeRequest) => Promise<ApplicationResult<T>>;
const projectConfiguration = (c: EnvironmentConfiguration): EnvironmentConfiguration => ({
  name: c.name, group: c.group, note: c.note, proxyId: c.proxyId, coreId: c.coreId, seed: c.seed,
  language: c.language, timezone: c.timezone, cpu: c.cpu, width: c.width, height: c.height,
  urls: c.urls, restoreTabs: c.restoreTabs, fingerprintVersion: c.fingerprintVersion,
});
const projectProxy = (c: ProxyConfiguration): ProxyConfiguration => ({ name: c.name, type: c.type, host: c.host, port: c.port, country: c.country });
const projectProxyTarget = (r: ProxyTargetRequest) => ({ proxyId: r.proxyId, expectedRevision: r.expectedRevision, requestId: r.requestId });
const invalidCookieReport = (operation: Operation) => operation.cookieReport && (operation.cookieReport.mode !== "native" || !operation.cookieReport.previewId || operation.cookieReport.environmentId !== operation.environmentId || operation.cookieReport.sessionId !== operation.sessionId);
export class WailsAdapter implements ApplicationService {
  readonly mode = "native" as const;
  private bridge: NativeBridge;
  private operations = new Map<string, Operation>();
  private view: WorkspaceView = {
    mode: "native",
    state: { schemaVersion: 1, environments: [], proxies: [], kernels: [], backups: [], activities: [] },
    issue: { code: "WORKSPACE_LOADING", message: "正在读取本机工作区…", retryable: false },
  };
  private listeners = new Set<() => void>();
  private eventListeners = new Set<(event: OperationEvent) => void>();
  private sequence = 0;
  private refreshSequence = 0;
  constructor(bridge: NativeBridge) { this.bridge = bridge; }
  getSnapshot = () => this.view;
  subscribe = (listener: () => void) => { this.listeners.add(listener); return () => { this.listeners.delete(listener); }; };
  subscribeEvents = (listener: (event: OperationEvent) => void) => { this.eventListeners.add(listener); return () => { this.eventListeners.delete(listener); }; };
  private publish() { this.listeners.forEach(listener => listener()); }
  private async invoke<T>(method: string, payload: unknown): Promise<ApplicationResult<T>> {
    try {
      const response = await this.bridge<T>({ mode: "native", method, payload });
      if (response.mode !== "native") return { ok: false, mode: "native", error: { code: "CAPABILITY_UNSUPPORTED", message: "服务返回了演示记录，已拒绝接入真实工作区。", retryable: false } };
      return response;
    } catch { return { ok: false, mode: "native", error: { code: "NATIVE_UNAVAILABLE", message: "本地服务连接失败，本次操作结果尚未确认。请重新读取工作区核对；不会改为演示成功。", retryable: true } }; }
  }
  async refresh(): Promise<ApplicationResult<WorkspaceView>> {
    const sequence = ++this.refreshSequence;
    let response = await this.invoke<WorkspaceView>("Workspace.Read", {});
    if (response.ok && (response.data.mode !== "native" || Object.values(response.data.runtimeSessions ?? {}).some(session => session.mode !== "native" || (session.proxyReport && (session.proxyReport.mode !== "native" || session.proxyReport.channelId !== session.proxyChannelId || session.proxyReport.proxyId !== session.proxyId || session.proxyReport.revision !== session.proxyRevision))) || (response.data.nativeProxyRecords ?? []).some(record => record.checkReport && record.checkReport.mode !== "native") || (response.data.proxyOperations ?? []).some(operation => operation.proxyReport && operation.proxyReport.mode !== "native") || (response.data.cookieOperations ?? []).some(invalidCookieReport))) response = { ok: false, mode: "native", error: { code: "CAPABILITY_UNSUPPORTED", message: "工作区、会话或报告身份不匹配，已拒绝接入；未回退演示数据。", retryable: false } };
    if (sequence !== this.refreshSequence) return response;
    if (response.ok) this.view = response.data;
    else this.view = { ...this.view, issue: response.error };
    this.publish();
    return response;
  }
  private async previewCall(method: string, payload: unknown): Promise<ApplicationResult<EnvironmentPreview>> {
    const response = await this.invoke<EnvironmentPreview>(method, payload);
    if (response.ok && response.data.fingerprint && response.data.fingerprint.mode !== "native") return { ok: false, mode: "native", error: { code: "CAPABILITY_UNSUPPORTED", message: "演示档案不能进入真实工作区。", retryable: false } };
    return response;
  }
  previewEnvironment(request: { kind: "create" | "edit"; sourceId?: string }) { return this.previewCall("Environment.Preview", request); }
  regeneratePreview(previewId: string) { return this.previewCall("Preview.Regenerate", { previewId }); }
  discardPreview(previewId: string) { return this.invoke<{ status: "discarded" }>("Preview.Discard", { previewId }); }
  generateFingerprint(request: GenerateFingerprintRequest) {
    const o = request.overrides;
    return this.previewCall("Fingerprint.Generate", { previewId: request.previewId, kernelId: request.kernelId, templateId: request.templateId, regenerate: !!request.regenerate, overrides: { language: o.language, timezone: o.timezone, cpu: o.cpu, width: o.width, height: o.height } });
  }
  listFingerprintRevisions(environmentId: string) { return this.invoke<ProfileRevision[]>("Fingerprint.ListRevisions", { environmentId }); }
  previewFingerprintRestore(previewId: string, revision: number) { return this.previewCall("Fingerprint.PreviewRestore", { previewId, revision }); }
  async commitFingerprintRevision(request: CommitFingerprintRequest) {
    const response = await this.invoke<{ status: "completed"; environment: SavedEnvironment; newRevision: number; fingerprintRevision: number }>("Fingerprint.CommitRevision", { previewId: request.previewId, environmentId: request.environmentId, profileHash: request.profileHash, expectedRevision: request.expectedRevision, requestId: request.requestId, configuration: projectConfiguration(request.configuration) });
    if (response.ok) await this.refresh();
    return response;
  }
  async createBatch(request: CreateBatchRequest) {
    const response = await this.invoke<{ status: "accepted"; operation: Operation }>("Environment.Create", { previewId: request.previewId, requestId: request.requestId, count: request.count, profileHash: request.profileHash, configuration: projectConfiguration(request.configuration) });
    if (response.ok) { await this.refresh(); this.emit(response.data.operation); }
    return response;
  }
  async updateEnvironment(request: UpdateEnvironmentRequest) {
    const response = await this.invoke<{ status: "completed"; environment: SavedEnvironment }>("Environment.Update", { previewId: request.previewId, requestId: request.requestId, expectedRevision: request.expectedRevision, profileHash: request.profileHash, configuration: projectConfiguration(request.configuration) });
    if (response.ok) await this.refresh();
    return response;
  }
  async getOperation(operationId: string) {
    const response = await this.invoke<Operation>("Operation.Read", { operationId });
    return this.confirmOperation(response);
  }
  async cancelOperation(operationId: string) { return this.confirmOperation(await this.invoke<Operation>("Operation.Cancel", { operationId })); }
  private confirmOperation(response: ApplicationResult<Operation>): ApplicationResult<Operation> {
    if (!response.ok || (!response.data.kind.startsWith("kernel-") && !response.data.kind.startsWith("runtime-") && response.data.kind !== "proxy-check" && response.data.kind !== "cookie-import")) return response;
    if (invalidCookieReport(response.data)) return { ok: false, mode: "native", error: { code: "CAPABILITY_UNSUPPORTED", message: "Cookie读回报告的环境/会话身份不匹配，不能计为真实写入。", retryable: false } };
    if (response.data.proxyReport && response.data.proxyReport.mode !== "native") return { ok: false, mode: "native", error: { code: "CAPABILITY_UNSUPPORTED", message: "演示代理报告不能作为真实检查结果。", retryable: false } };
    const previous = this.operations.get(response.data.id);
    const operation = mergeOperation(previous, response.data);
    this.operations.set(operation.id, operation);
    if (operation !== previous) this.emit(operation);
    return { ...response, data: operation };
  }
  private emit(operation: Operation) {
    const event: OperationEvent = { mode: "native", type: operationIsTerminal(operation) ? "OperationCompleted" : "OperationProgress", operationId: operation.id, sequence: ++this.sequence, time: new Date().toISOString(), operation };
    this.eventListeners.forEach(listener => listener(event));
  }
  selectKernelArchive() { return this.invoke<{ status: "selected" | "cancelled"; archiveToken?: string; name?: string }>("Kernel.SelectArchive", {}); }
  installKernel(request: KernelInstallRequest) {
    return this.invoke<{ status: "accepted"; operation: Operation }>("Kernel.Install", { source: request.source, version: request.version, expectedChecksum: request.expectedChecksum, archiveToken: request.archiveToken, trusted: request.trusted, requestId: request.requestId });
  }
  verifyKernel(kernelId: string, requestId: string) { return this.invoke<{ status: "accepted"; operation: Operation }>("Kernel.Verify", { kernelId, requestId }); }
  deleteKernel(kernelId: string, requestId: string) { return this.invoke<{ status: "accepted"; operation: Operation }>("Kernel.Delete", { kernelId, requestId }); }
  private async runtimeMutation(method: string, payload: unknown) {
    const response = await this.invoke<{ status: "accepted"; operation: Operation }>(method, payload);
    if (response.ok && response.data.operation.proxyReport && response.data.operation.proxyReport.mode !== "native") return { ok: false as const, mode: "native" as const, error: { code: "CAPABILITY_UNSUPPORTED", message: "演示代理报告不能作为原生启动受理结果。", retryable: false } };
    await this.refresh();
    return response;
  }
  startRuntime(request: RuntimeStartRequest) {
    return this.runtimeMutation("Runtime.Start", { environmentId: request.environmentId, requestId: request.requestId, networkPolicy: request.networkPolicy, ...(request.purpose ? { purpose: request.purpose, expectedRevision: request.expectedRevision } : {}) });
  }
  stopRuntime(request: { environmentId: string; requestId: string }) {
    return this.runtimeMutation("Runtime.Stop", { environmentId: request.environmentId, requestId: request.requestId });
  }
  forceStopRuntime(request: { environmentId: string; sessionId: string; requestId: string }) {
    return this.runtimeMutation("Runtime.ForceStop", { environmentId: request.environmentId, sessionId: request.sessionId, requestId: request.requestId });
  }
  reconcileRuntime(request: { environmentId: string; sessionId: string; requestId: string }) {
    return this.runtimeMutation("Runtime.Reconcile", { environmentId: request.environmentId, sessionId: request.sessionId, requestId: request.requestId });
  }
  async inspectRuntime(ids: string[]): Promise<ApplicationResult<RuntimeSession[]>> {
    const response = await this.invoke<RuntimeSession[]>("Runtime.Inspect", { ids });
    if (response.ok && response.data.some(session => session.mode !== "native" || (session.proxyReport && (session.proxyReport.mode !== "native" || session.proxyReport.channelId !== session.proxyChannelId || session.proxyReport.proxyId !== session.proxyId || session.proxyReport.revision !== session.proxyRevision)))) return { ok: false, mode: "native", error: { code: "CAPABILITY_UNSUPPORTED", message: "会话和代理通道报告身份不匹配，不能作为真实运行结果。", retryable: false } };
    return response;
  }
  async parseProxyImport(text: string): Promise<ApplicationResult<ProxyImportPreview>> {
    const response = await this.invoke<ProxyImportPreview>("Proxy.ParseImport", { text });
    if (response.ok && response.data.mode !== "native") return { ok: false, mode: "native", error: { code: "CAPABILITY_UNSUPPORTED", message: "演示代理预览不能保存到本机工作区。", retryable: false } };
    return response;
  }
  discardProxyImport(previewId: string) { return this.invoke<{ status: "discarded" }>("Proxy.DiscardImport", { previewId }); }
  async commitProxyImport(request: { previewId: string; selectedRows: number[]; requestId: string }) {
    const response = await this.invoke<{ status: "completed"; importedIds: string[]; importedLines: number[] }>("Proxy.CommitImport", { previewId: request.previewId, selectedRows: [...request.selectedRows], requestId: request.requestId });
    if (response.ok) await this.refresh(); return response;
  }
  async updateProxy(request: ProxyUpdateRequest) {
    const credentials = request.credentials.action === "replace" ? { action: "replace", username: request.credentials.username, password: request.credentials.password } : { action: request.credentials.action };
    const response = await this.invoke<{ status: "completed"; record: NativeProxy }>("Proxy.Update", { ...projectProxyTarget(request), configuration: projectProxy(request.configuration), credentials });
    if (response.ok) await this.refresh(); return response;
  }
  async deleteProxy(request: ProxyTargetRequest) {
    const response = await this.invoke<{ status: "completed"; deletedId: string }>("Proxy.Delete", projectProxyTarget(request));
    if (response.ok) await this.refresh(); return response;
  }
  async checkProxy(request: ProxyTargetRequest) {
    const response = await this.invoke<{ status: "accepted"; operation: Operation }>("Proxy.Check", projectProxyTarget(request));
    if (response.ok) { const confirmed = this.confirmOperation({ ok: true, mode: "native", data: response.data.operation }); if (!confirmed.ok) return confirmed; }
    await this.refresh(); return response;
  }
  async parseCookieImport(environmentId: string, text: string): Promise<ApplicationResult<CookieImportPreview>> {
    const response = await this.invoke<CookieImportPreview>("Cookie.ParseImport", { environmentId, text });
    if (response.ok && (response.data.mode !== "native" || response.data.environmentId !== environmentId)) return { ok: false, mode: "native", error: { code: "CAPABILITY_UNSUPPORTED", message: "Cookie预览不是指定原生环境，已拒绝写入。", retryable: false } };
    return response;
  }
  discardCookieImport(previewId: string) { return this.invoke<{ status: "discarded" }>("Cookie.DiscardImport", { previewId }); }
  async commitCookieImport(request: CookieCommitRequest) {
    const response = await this.invoke<{ status: "accepted"; operation: Operation }>("Cookie.CommitImport", { previewId: request.previewId, environmentId: request.environmentId, expectedRevision: request.expectedRevision, sessionId: request.sessionId, selectedRows: [...request.selectedRows], policy: request.policy, requestId: request.requestId });
    if (response.ok) {
      if (response.data.operation.environmentId !== request.environmentId || response.data.operation.sessionId !== request.sessionId || response.data.operation.cookieReport?.previewId !== request.previewId) return { ok: false as const, mode: "native" as const, error: { code: "CAPABILITY_UNSUPPORTED", message: "导入受理目标或预览不匹配，不能作为指定会话结果。", retryable: false } };
      const confirmed = this.confirmOperation({ ok: true, mode: "native", data: response.data.operation }); if (!confirmed.ok) return confirmed;
    }
    await this.refresh(); return response;
  }
}
