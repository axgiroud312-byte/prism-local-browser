import type {
  ApplicationResult, ApplicationService, EnvironmentConfiguration, EnvironmentPreview,
  CreateBatchRequest, UpdateEnvironmentRequest, SavedEnvironment, WorkspaceView, Operation, OperationEvent,
  KernelInstallRequest,
  GenerateFingerprintRequest, CommitFingerprintRequest, ProfileRevision, RuntimeSession,
} from "./contract.ts";
import { mergeOperation } from "./contract.ts";

export interface NativeRequest { mode: "native"; method: string; payload: unknown }
export type NativeBridge = <T>(request: NativeRequest) => Promise<ApplicationResult<T>>;
const projectConfiguration = (c: EnvironmentConfiguration): EnvironmentConfiguration => ({
  name: c.name, group: c.group, note: c.note, proxyId: c.proxyId, coreId: c.coreId, seed: c.seed,
  language: c.language, timezone: c.timezone, cpu: c.cpu, width: c.width, height: c.height,
  urls: c.urls, restoreTabs: c.restoreTabs, fingerprintVersion: c.fingerprintVersion,
});
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
    if (response.ok && (response.data.mode !== "native" || Object.values(response.data.runtimeSessions ?? {}).some(session => session.mode !== "native"))) response = { ok: false, mode: "native", error: { code: "CAPABILITY_UNSUPPORTED", message: "工作区或运行会话不是原生记录，已拒绝接入；未回退到演示数据。", retryable: false } };
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
    if (!response.ok || (!response.data.kind.startsWith("kernel-") && !response.data.kind.startsWith("runtime-"))) return response;
    const previous = this.operations.get(response.data.id);
    const operation = mergeOperation(previous, response.data);
    this.operations.set(operation.id, operation);
    if (operation !== previous) this.emit(operation);
    return { ...response, data: operation };
  }
  private emit(operation: Operation) {
    const event: OperationEvent = { mode: "native", type: ["completed", "cancelled", "failed"].includes(operation.state) ? "OperationCompleted" : "OperationProgress", operationId: operation.id, sequence: ++this.sequence, time: new Date().toISOString(), operation };
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
    if (response.ok) await this.refresh();
    return response;
  }
  startRuntime(request: { environmentId: string; requestId: string; networkPolicy: "direct" }) {
    return this.runtimeMutation("Runtime.Start", { environmentId: request.environmentId, requestId: request.requestId, networkPolicy: request.networkPolicy });
  }
  stopRuntime(request: { environmentId: string; requestId: string }) {
    return this.runtimeMutation("Runtime.Stop", { environmentId: request.environmentId, requestId: request.requestId });
  }
  async inspectRuntime(ids: string[]): Promise<ApplicationResult<RuntimeSession[]>> {
    const response = await this.invoke<RuntimeSession[]>("Runtime.Inspect", { ids });
    if (response.ok && response.data.some(session => session.mode !== "native")) return { ok: false, mode: "native", error: { code: "CAPABILITY_UNSUPPORTED", message: "演示会话不能作为真实运行结果。", retryable: false } };
    return response;
  }
}
