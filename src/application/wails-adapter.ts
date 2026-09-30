import type {
  ApplicationResult, ApplicationService, EnvironmentConfiguration, EnvironmentPreview,
  CreateBatchRequest, UpdateEnvironmentRequest, SavedEnvironment, WorkspaceView, Operation, OperationEvent,
} from "./contract.ts";

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
    } catch { return { ok: false, mode: "native", error: { code: "NATIVE_UNAVAILABLE", message: "本地服务连接失败，未确认保存成功。请重新打开应用核对记录。", retryable: true } }; }
  }
  async refresh(): Promise<ApplicationResult<WorkspaceView>> {
    const sequence = ++this.refreshSequence;
    let response = await this.invoke<WorkspaceView>("Workspace.Read", {});
    if (response.ok && response.data.mode !== "native") response = { ok: false, mode: "native", error: { code: "CAPABILITY_UNSUPPORTED", message: "工作区不是原生记录，已拒绝接入；未回退到演示数据。", retryable: false } };
    if (sequence !== this.refreshSequence) return response;
    if (response.ok) this.view = response.data;
    else this.view = { ...this.view, issue: response.error };
    this.publish();
    return response;
  }
  previewEnvironment(request: { kind: "create" | "edit"; sourceId?: string }) { return this.invoke<EnvironmentPreview>("Environment.Preview", request); }
  regeneratePreview(previewId: string) { return this.invoke<EnvironmentPreview>("Preview.Regenerate", { previewId }); }
  discardPreview(previewId: string) { return this.invoke<{ status: "discarded" }>("Preview.Discard", { previewId }); }
  async createBatch(request: CreateBatchRequest) {
    const response = await this.invoke<{ status: "accepted"; operation: Operation }>("Environment.Create", { previewId: request.previewId, requestId: request.requestId, count: request.count, configuration: projectConfiguration(request.configuration) });
    if (response.ok) { await this.refresh(); this.emit(response.data.operation); }
    return response;
  }
  async updateEnvironment(request: UpdateEnvironmentRequest) {
    const response = await this.invoke<{ status: "completed"; environment: SavedEnvironment }>("Environment.Update", { previewId: request.previewId, requestId: request.requestId, expectedRevision: request.expectedRevision, configuration: projectConfiguration(request.configuration) });
    if (response.ok) await this.refresh();
    return response;
  }
  getOperation(operationId: string) { return this.invoke<Operation>("Operation.Read", { operationId }); }
  cancelOperation(operationId: string) { return this.invoke<Operation>("Operation.Cancel", { operationId }); }
  private emit(operation: Operation) {
    const event: OperationEvent = { mode: "native", type: "OperationCompleted", operationId: operation.id, sequence: ++this.sequence, time: new Date().toISOString(), operation };
    this.eventListeners.forEach(listener => listener(event));
  }
}
