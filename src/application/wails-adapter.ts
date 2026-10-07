import type {
  ApplicationResult, ApplicationService, EnvironmentConfiguration, EnvironmentPreview,
  CreateBatchRequest, UpdateEnvironmentRequest, SavedEnvironment, WorkspaceView, Operation, OperationEvent,
  KernelInstallRequest, KernelDefault, KernelDefaultRequest,
  GenerateFingerprintRequest, CommitFingerprintRequest, ProfileRevision, RuntimeSession,
  ProxyConfiguration, ProxyImportPreview, ProxyUpdateRequest, ProxyTargetRequest, NativeProxy,
  CookieCommitRequest, CookieImportPreview, RuntimeStartRequest,
  NativeBatchPreviewRequest, NativeBatchPage, NativeEnvironmentQuery, NativeBackupExportRequest, NativeBackupPending, NativeRestorePreview, NativeRestorePage, NativeRestoreRequest,
  NativeRecycleAction, NativeRecyclePage, NativeRecyclePageRequest, NativeRecycleRequest,
  NativeMigrationPreview, NativeMigrationRequest,
  NativeRestoreSourceState, NativeRestoreSourcePending,
} from "./contract.ts";
import { mergeOperation, operationIsTerminal } from "./contract.ts";
import { validBatchPage, validBatchReport } from "./batch-model.ts";
import { confirmsBackupRequest, invalidBackupOperation } from "./backup-model.ts";
import { confirmsRestoreRequest, invalidRestoreOperation, validRestoreSourceState, validRestorePreview } from "./restore-model.ts";
import { confirmsRecycleRequest, invalidRecycleOperation, validRecyclePage } from "./recycle-model.ts";
import { confirmsMigrationRequest, invalidMigrationOperation, validMigrationPreview } from "./migration-model.ts";
import { DiagnosticsClient } from "./diagnostics-client.ts";

export interface NativeRequest { mode: "native"; method: string; payload: unknown }
export type NativeBridge = <T>(request: NativeRequest) => Promise<ApplicationResult<T>>;
interface MigrationRollbackOwner {
  operationId: string; sourceToken?: string; previewId?: string; cancelled: boolean;
  preparing: boolean; selectionAttempted: boolean; discardFlight?: Promise<void>; discardAfterRefusal?: boolean;
}
const rollbackCleanupUnconfirmed = <T,>(): ApplicationResult<T> => ({ ok: false, mode: "native", error: { code: "MIGRATION_RESULT_UNCONFIRMED", message: "原升级前预检清理尚未确认；只可重试清理原来源，不另选来源、重新预检或执行恢复。", retryable: true } });
const projectConfiguration = (c: EnvironmentConfiguration): EnvironmentConfiguration => ({
  name: c.name, group: c.group, note: c.note, proxyId: c.proxyId, coreId: c.coreId, seed: c.seed,
  language: c.language, timezone: c.timezone, cpu: c.cpu, width: c.width, height: c.height,
  urls: c.urls, restoreTabs: c.restoreTabs, fingerprintVersion: c.fingerprintVersion,
});
const projectProxy = (c: ProxyConfiguration): ProxyConfiguration => ({ name: c.name, type: c.type, host: c.host, port: c.port, country: c.country });
const projectProxyTarget = (r: ProxyTargetRequest) => ({ proxyId: r.proxyId, expectedRevision: r.expectedRevision, requestId: r.requestId });
const invalidCookieReport = (operation: Operation) => operation.cookieReport && (operation.cookieReport.mode !== "native" || !operation.cookieReport.previewId || operation.cookieReport.environmentId !== operation.environmentId || operation.cookieReport.sessionId !== operation.sessionId);
const invalidBatchReport = (operation: Operation) => operation.kind.startsWith("batch-") && (!validBatchReport(operation.batchReport) || operation.kind !== `batch-${operation.batchReport?.kind}` || operation.total !== operation.batchReport?.total);
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
  private environmentQuery?: NativeEnvironmentQuery;
  private pendingBackup?: NativeBackupPending;
  private pendingRestore?: { request: NativeRestoreRequest; operationId?: string };
  private restoreSourceOwner?: NativeRestoreSourcePending;
  private restoreSelectionFlight?: Promise<ApplicationResult<{ status: "selected" | "cancelled"; sourceToken?: string; name?: string }>>;
  private restoreSourceDiscardFlight?: Promise<ApplicationResult<{ status: "discarded" }>>;
  private restoreFlights = new Map<string, Promise<ApplicationResult<{ status: "accepted"; operation: Operation }>>>();
  private restoreRefusal?: string;
  private pendingRecycle?: { request: NativeRecycleRequest; operationId?: string };
  private recycleRefusal?: string;
  private pendingMigration?: { request: NativeMigrationRequest; operationId?: string };
  private migrationRefusal?: string;
  private pendingKernelDefault?: KernelDefaultRequest;
  private rollbackOwner?: MigrationRollbackOwner;
  private diagnostics = new DiagnosticsClient((method, payload) => this.invoke(method, payload), () => this.publish());
  getDiagnosticState = () => this.diagnostics.getState();
  previewDiagnostics = () => this.diagnostics.preview();
  exportDiagnostics = () => this.diagnostics.export();
  endDiagnosticVerification = () => this.diagnostics.endVerification();
  constructor(bridge: NativeBridge) { this.bridge = bridge; }
  getSnapshot = () => this.view;
  subscribe = (listener: () => void) => { this.listeners.add(listener); return () => { this.listeners.delete(listener); }; };
  subscribeEvents = (listener: (event: OperationEvent) => void) => { this.eventListeners.add(listener); return () => { this.eventListeners.delete(listener); }; };
  private publish() { this.listeners.forEach(listener => listener()); }
  private async invoke<T>(method: string, payload: unknown): Promise<ApplicationResult<T>> {
    try {
      const response = await this.bridge<T>({ mode: "native", method, payload });
      if (!response || typeof response.ok !== "boolean" || response.ok && response.data == null || !response.ok && !response.error) throw new Error("Invalid native envelope");
      if (response.mode !== "native") {
        if (method === "Migration.Prepare") return { ok: false, mode: "native", operationId: response.operationId, error: { code: "MIGRATION_RESULT_UNCONFIRMED", message: "迁移响应模式不符，原请求保持待核实。", retryable: true } };
        if (method === "Recycle.Commit") return { ok: false, mode: "native", operationId: response.operationId, error: { code: "RECYCLE_RESULT_UNCONFIRMED", message: "回收响应模式不符，原请求仍待核实。", retryable: true } };
        if (method === "Backup.ApplyRestore") return { ok: false, mode: "native", operationId: response.operationId, error: { code: "RESTORE_RESULT_UNCONFIRMED", message: "恢复响应模式不符；原请求保持待核实，不能另建恢复。", retryable: true } };
        if (method === "Backup.Export") return { ok: false, mode: "native", operationId: response.operationId, error: { code: "BACKUP_RESULT_UNCONFIRMED", message: "返回模式不符，已拒绝演示结果；这不证明后台未受理，原备份请求保留，请核实。", retryable: true } };
        return { ok: false, mode: "native", error: { code: "CAPABILITY_UNSUPPORTED", message: "服务返回了演示记录，已拒绝接入真实工作区。", retryable: false } };
      }
      return response;
    } catch { return { ok: false, mode: "native", error: { code: "NATIVE_UNAVAILABLE", message: "本地服务连接失败，本次操作结果尚未确认。请重新读取工作区核对；不会改为演示成功。", retryable: true } }; }
  }
  async refresh(): Promise<ApplicationResult<WorkspaceView>> {
    const sequence = ++this.refreshSequence;
    let response = await this.invoke<WorkspaceView>("Workspace.Read", this.environmentQuery ? { environmentQuery: { ...this.environmentQuery } } : {});
    if (response.ok && ((response.data.migrationOperations ?? []).some(invalidMigrationOperation) || response.data.migrationMaintenance && (response.data.migrationMaintenance.kind !== "migration" || invalidMigrationOperation(response.data.migrationMaintenance)))) response = { ok: false, mode: "native", error: { code: "MIGRATION_RESULT_UNCONFIRMED", message: "迁移状态尚未核实，保留原请求。", retryable: true } };
    if (response.ok && ((response.data.recycleOperations ?? []).some(invalidRecycleOperation) || response.data.recycleMaintenance && (response.data.recycleMaintenance.kind !== "recycle" || invalidRecycleOperation(response.data.recycleMaintenance)))) response = { ok: false, mode: "native", error: { code: "RECYCLE_RESULT_UNCONFIRMED", message: "回收任务状态尚未核实，未接入为完成。", retryable: true } };
    if (response.ok && ((response.data.restoreOperations ?? []).some(invalidRestoreOperation) || response.data.maintenance && (response.data.maintenance.kind !== "backup-restore" || invalidRestoreOperation(response.data.maintenance)))) response = { ok: false, mode: "native", error: { code: "RESTORE_RESULT_UNCONFIRMED", message: "恢复报告身份或完整性未核实，未接入为成功。", retryable: true } };
    if (response.ok && (response.data.mode !== "native" || Object.values(response.data.runtimeSessions ?? {}).some(session => session.mode !== "native" || (session.proxyReport && (session.proxyReport.mode !== "native" || session.proxyReport.channelId !== session.proxyChannelId || session.proxyReport.proxyId !== session.proxyId || session.proxyReport.revision !== session.proxyRevision))) || (response.data.nativeProxyRecords ?? []).some(record => record.checkReport && record.checkReport.mode !== "native") || (response.data.proxyOperations ?? []).some(operation => operation.proxyReport && operation.proxyReport.mode !== "native") || (response.data.cookieOperations ?? []).some(invalidCookieReport) || (response.data.batchOperations ?? []).some(invalidBatchReport) || (response.data.backupOperations ?? []).some(invalidBackupOperation))) response = { ok: false, mode: "native", error: { code: "CAPABILITY_UNSUPPORTED", message: "工作区、会话或报告身份不匹配，已拒绝接入；未回退演示数据。", retryable: false } };
    if (sequence !== this.refreshSequence) return response;
    if (response.ok) {
      const batchOperations = response.data.batchOperations?.map(operation => {
        const merged = mergeOperation(this.operations.get(operation.id), operation);
        this.operations.set(merged.id, merged); return merged;
      });
      const backupOperations = response.data.backupOperations?.map(operation => {
        const merged = mergeOperation(this.operations.get(operation.id), operation);
        this.operations.set(merged.id, merged); this.confirmPendingBackup(merged); return merged;
      });
      const restoreOperations = response.data.restoreOperations?.map(operation => {
        const merged = mergeOperation(this.operations.get(operation.id), operation);
        this.operations.set(merged.id, merged); this.confirmPendingRestore(merged); return merged;
      });
      const recycleOperations = response.data.recycleOperations?.map(operation => {
        const merged = mergeOperation(this.operations.get(operation.id), operation);
        this.operations.set(merged.id, merged); this.confirmPendingRecycle(merged); return merged;
      });
      const migrationOperations = response.data.migrationOperations?.map(op => {
        const merged = mergeOperation(this.operations.get(op.id), op); this.operations.set(op.id, merged);
        if (confirmsMigrationRequest(merged, this.pendingMigration?.request)) this.pendingMigration = undefined;
        return merged;
      });
      this.view = { ...response.data, ...(batchOperations ? { batchOperations } : {}), ...(backupOperations ? { backupOperations } : {}), ...(restoreOperations ? { restoreOperations } : {}), ...(recycleOperations ? { recycleOperations } : {}), ...(migrationOperations ? { migrationOperations } : {}) };
    }
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
    if (response.ok && (response.data.id !== operationId || typeof response.data.kind !== "string")) return { ok: false as const, mode: "native" as const, error: { code: "CAPABILITY_UNSUPPORTED", message: "任务响应不是指定ID，未接管其他任务。", retryable: false } };
    return this.confirmOperation(response);
  }
  async cancelOperation(operationId: string) {
    const response = await this.invoke<Operation>("Operation.Cancel", { operationId });
    if (response.ok && (response.data.id !== operationId || typeof response.data.kind !== "string")) return { ok: false as const, mode: "native" as const, error: { code: "CAPABILITY_UNSUPPORTED", message: "取消响应不是指定ID，不作为其他任务的取消结果。", retryable: false } };
    return this.confirmOperation(response);
  }
  private confirmOperation(response: ApplicationResult<Operation>): ApplicationResult<Operation> {
    if (!response.ok || (!response.data.kind.startsWith("kernel-") && !response.data.kind.startsWith("runtime-") && !response.data.kind.startsWith("batch-") && response.data.kind !== "proxy-check" && response.data.kind !== "cookie-import" && response.data.kind !== "backup-export" && response.data.kind !== "backup-restore" && response.data.kind !== "recycle" && response.data.kind !== "migration")) return response;
    if (invalidMigrationOperation(response.data)) return { ok: false, mode: "native", operationId: response.data.id, error: { code: "MIGRATION_RESULT_UNCONFIRMED", message: "迁移报告尚未核实，保持原任务保护。", retryable: true } };
    if (invalidRecycleOperation(response.data)) return { ok: false, mode: "native", operationId: response.data.id, error: { code: "RECYCLE_RESULT_UNCONFIRMED", message: "回收任务报告尚未核实，保留原请求和目录保护。", retryable: true } };
    if (invalidRestoreOperation(response.data)) return { ok: false, mode: "native", operationId: response.data.id, error: { code: "RESTORE_RESULT_UNCONFIRMED", message: "恢复报告未核实；保留原任务ID，不重复切换。", retryable: true } };
    if (invalidBackupOperation(response.data)) return { ok: false, mode: "native", operationId: response.data.id, error: { code: "BACKUP_RESULT_UNCONFIRMED", message: "备份格式、统计或发布状态未核实；已知任务ID保留，不证明未受理，不重新导出。", retryable: true } };
    if (invalidBatchReport(response.data)) return { ok: false, mode: "native", error: { code: "CAPABILITY_UNSUPPORTED", message: "批次类型或计划身份不匹配，不能计为持久任务结果。", retryable: false } };
    if (invalidCookieReport(response.data)) return { ok: false, mode: "native", error: { code: "CAPABILITY_UNSUPPORTED", message: "Cookie读回报告的环境/会话身份不匹配，不能计为真实写入。", retryable: false } };
    if (response.data.proxyReport && response.data.proxyReport.mode !== "native") return { ok: false, mode: "native", error: { code: "CAPABILITY_UNSUPPORTED", message: "演示代理报告不能作为真实检查结果。", retryable: false } };
    const previous = this.operations.get(response.data.id);
    const operation = mergeOperation(previous, response.data);
    this.operations.set(operation.id, operation);
    this.confirmPendingBackup(operation);
    this.confirmPendingRestore(operation);
    this.confirmPendingRecycle(operation);
    if (confirmsMigrationRequest(operation, this.pendingMigration?.request)) this.pendingMigration = undefined;
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
  async setDefaultKernel(request: KernelDefaultRequest) {
    const projected = { kernelId: request.kernelId, expectedRevision: request.expectedRevision, requestId: request.requestId };
    if (this.pendingKernelDefault && JSON.stringify(this.pendingKernelDefault) !== JSON.stringify(projected)) return { ok: false as const, mode: "native" as const, error: { code: "DEFAULT_RESULT_UNCONFIRMED", message: "请先核实原默认选择请求。", retryable: true } };
    const owner = this.pendingKernelDefault ??= projected;
    let result = await this.invoke<KernelDefault>("Kernel.SetDefault", projected);
    if (result.ok && (!result.data || result.data.kernelId !== projected.kernelId || result.data.revision !== projected.expectedRevision + 1 || !Number.isSafeInteger(result.data.revision))) result = { ok: false, mode: "native", error: { code: "DEFAULT_RESULT_UNCONFIRMED", message: "默认选择回执与原请求不符，请核实原请求。", retryable: true } };
    if (this.pendingKernelDefault === owner && (result.ok || ["VALIDATION_FAILED", "REVISION_CONFLICT", "REQUEST_ID_REUSED", "PROFILE_BUSY", "KERNEL_MISSING", "KERNEL_INTEGRITY_FAILED"].includes(result.error.code))) this.pendingKernelDefault = undefined;
    await this.refresh();
    return result;
  }
  getPendingKernelDefault() { return this.pendingKernelDefault ? { ...this.pendingKernelDefault } : undefined; }
  async previewMigration(environmentId: string, kernelId: string): Promise<ApplicationResult<NativeMigrationPreview>> {
    const result = await this.invoke<NativeMigrationPreview>("Migration.Preview", { environmentId, kernelId });
    if (result.ok && !validMigrationPreview(result.data, environmentId, kernelId)) return { ok: false, mode: "native", error: { code: "MIGRATION_RESULT_UNCONFIRMED", message: "迁移预览身份或参数不符。", retryable: true } };
    return result;
  }
  async lookupMigrationEnvironments(page: number, search: string) {
    const result = await this.invoke<WorkspaceView>("Workspace.Read", { environmentQuery: { page, pageSize: 25, search, group: "", status: "all" } });
    if (!result.ok) return result;
    if (result.data.mode !== "native" || !result.data.environmentPage || result.data.environmentPage.page !== page || !Array.isArray(result.data.state?.environments)) return { ok: false as const, mode: "native" as const, error: { code: "MIGRATION_RESULT_UNCONFIRMED", message: "环境查找分页未核实。", retryable: true } };
    return { ok: true as const, mode: "native" as const, data: { items: result.data.state.environments.map(e => ({ id: e.id, name: e.name, coreId: e.coreId, status: e.status })), page, total: result.data.environmentPage.filteredTotal } };
  }
  getPendingMigration() { return this.pendingMigration ? { ...this.pendingMigration, request: { ...this.pendingMigration.request } } : undefined; }
  wasMigrationNotAccepted(requestId: string) { return this.migrationRefusal === requestId; }
  async prepareMigration(request: NativeMigrationRequest) {
    const projected = { previewId: request.previewId, confirm: request.confirm, requestId: request.requestId };
    if (this.pendingMigration && JSON.stringify(this.pendingMigration.request) !== JSON.stringify(projected)) return { ok: false as const, mode: "native" as const, operationId: this.pendingMigration.operationId, error: { code: "MIGRATION_RESULT_UNCONFIRMED", message: "先核实原迁移请求，不另建副本。", retryable: true } };
    const owner = this.pendingMigration ??= { request: projected };
    let result = await this.invoke<{ status: "accepted"; operation: Operation }>("Migration.Prepare", projected);
    if (result.ok) {
      const operationId = result.data.operation?.id || result.operationId;
      if (this.pendingMigration === owner && operationId) owner.operationId = operationId;
      if (result.data.status !== "accepted" || !result.data.operation || !confirmsMigrationRequest(result.data.operation, projected)) result = { ok: false, mode: "native", operationId, error: { code: "MIGRATION_RESULT_UNCONFIRMED", message: "原迁移受理尚未核实，请保留原请求。", retryable: true } };
      else { const checked = this.confirmOperation({ ok: true, mode: "native", data: result.data.operation }); result = checked.ok ? { ...result, data: { ...result.data, operation: checked.data } } : checked; }
    } else if (this.pendingMigration === owner) {
      if (result.operationId) owner.operationId = result.operationId;
      else if (["VALIDATION_FAILED", "PREVIEW_EXPIRED", "NOT_FOUND", "REVISION_CONFLICT", "PROFILE_BUSY", "MIGRATION_NOT_ACCEPTED", "REQUEST_ID_REUSED", "NETWORK_PROTECTION_UNAVAILABLE", "KERNEL_MISSING", "KERNEL_INTEGRITY_FAILED"].includes(result.error.code)) { this.migrationRefusal = projected.requestId; this.pendingMigration = undefined; if (result.error.code === "MIGRATION_NOT_ACCEPTED" && owner.operationId) this.operations.delete(owner.operationId); }
    }
    await this.refresh();
    const known = [...this.operations.values()].find(op => confirmsMigrationRequest(op, projected));
    return known ? { ok: true as const, mode: "native" as const, operationId: known.id, data: { status: "accepted" as const, operation: known } } : result;
  }
  async migrationAction(operationId: string, action: "stop" | "commit" | "recover") {
    const owner = this.pendingMigration;
    const result = await this.invoke<Operation>("Migration.Action", { operationId, action, confirm: true });
    if (!result.ok && result.error.code === "MIGRATION_NOT_ACCEPTED" && this.pendingMigration === owner && owner?.operationId === operationId) { this.migrationRefusal = owner.request.requestId; this.pendingMigration = undefined; this.operations.delete(operationId); }
    if (result.ok && (result.data.id !== operationId || result.data.kind !== "migration")) return { ok: false as const, mode: "native" as const, error: { code: "MIGRATION_RESULT_UNCONFIRMED", message: "不是原迁移任务的结果。", retryable: true } };
    const checked = this.confirmOperation(result); await this.refresh(); return checked;
  }
  async previewMigrationRollback(operationId: string): Promise<ApplicationResult<NativeRestorePreview>> {
    const unconfirmed = (): ApplicationResult<NativeRestorePreview> => ({ ok: false, mode: "native", error: { code: "MIGRATION_RESULT_UNCONFIRMED", message: "升级前恢复预检尚未核实或已取消，未接入确认。", retryable: true } });
    if (this.rollbackOwner) return this.rollbackOwner.cancelled ? rollbackCleanupUnconfirmed() : unconfirmed();
    const owner: MigrationRollbackOwner = { operationId, cancelled: false, preparing: true, selectionAttempted: false }; this.rollbackOwner = owner;
    let retained = false;
    try {
      const operation = await this.getOperation(operationId);
      if (!operation.ok || invalidMigrationOperation(operation.data) || operation.data.kind !== "migration" || !operationIsTerminal(operation.data) || !operation.data.migrationReport?.backupVerified || owner.cancelled || this.rollbackOwner !== owner) return unconfirmed();
      const expected = operation.data.migrationReport.archiveSha256;
      owner.selectionAttempted = true;
      const selected = await this.invoke<{ sourceToken: string; archiveSha256: string }>("Migration.SelectRollback", { operationId });
      if (!selected.ok) {
        // Go SelectRollback's native PROFILE_BUSY branch returns before token
        // allocation. This is a no-source refusal, not a successful discard.
        const error = selected.error;
        if (this.rollbackOwner === owner && !owner.sourceToken && !owner.previewId && selected.mode === "native" && Object.keys(selected).length === 3 && error && typeof error === "object" && !Array.isArray(error) && Object.keys(error).length === 3 && error.code === "PROFILE_BUSY" && typeof error.message === "string" && error.message.length > 0 && error.retryable === true) owner.selectionAttempted = false;
        return selected;
      }
      if (typeof selected.data.sourceToken === "string" && selected.data.sourceToken) owner.sourceToken = selected.data.sourceToken;
      if (!owner.sourceToken || selected.data.archiveSha256 !== expected || owner.cancelled || this.rollbackOwner !== owner) return unconfirmed();
      const preview = await this.previewRestore(owner.sourceToken);
      if (!preview.ok) return preview;
      owner.previewId = preview.data.previewId;
      if (preview.data.archiveSha256 !== expected || preview.data.environmentCount !== 1 || owner.cancelled || this.rollbackOwner !== owner) return unconfirmed();
      retained = true; return preview;
    } catch { return unconfirmed();
    } finally {
      owner.preparing = false;
      if (!retained) { owner.cancelled = true; await this.cleanMigrationRollback(owner); }
    }
  }
  getMigrationRollbackCleanup() {
    const owner = this.rollbackOwner;
    return owner?.cancelled ? { operationId: owner.operationId, ...(owner.previewId ? { previewId: owner.previewId } : {}), pending: owner.preparing || !!owner.discardFlight } : undefined;
  }
  discardMigrationRollback(): Promise<void> {
    const owner = this.rollbackOwner; if (!owner) return Promise.resolve();
    // A submitted restore owns the preview across navigation and transport loss.
    // Only a definite refusal permits deferred cleanup to invalidate its token.
    if (owner.previewId && this.pendingRestore?.request.previewId === owner.previewId) { owner.discardAfterRefusal = true; return Promise.resolve(); }
    owner.cancelled = true;
    // Don't race a late selected token/preview. The original preparation's
    // finally owns its one cleanup, including failures and late cancellation.
    if (owner.preparing) { this.publish(); return Promise.resolve(); }
    return this.cleanMigrationRollback(owner);
  }
  private cleanMigrationRollback(owner: MigrationRollbackOwner): Promise<void> {
    if (this.rollbackOwner !== owner) return Promise.resolve();
    if (owner.discardFlight) return owner.discardFlight;
    if (!owner.sourceToken) {
      // Before selection there was no native source. Once selection was sent,
      // a lost token cannot be reconstructed by selecting a replacement.
      if (!owner.selectionAttempted) this.rollbackOwner = undefined;
      this.publish(); return Promise.resolve();
    }
    const previewId = owner.previewId ?? "", sourceToken = owner.sourceToken;
    const promise = Promise.resolve().then(() => this.discardRestore(previewId, sourceToken)).then(result => {
      // The existing Go RPC returns exactly this status object, not an echoed
      // requestId or receipt. Every other envelope keeps the original owner.
      if (result.ok && result.mode === "native" && typeof result.data === "object" && result.data !== null && !Array.isArray(result.data) && result.data.status === "discarded" && Object.keys(result.data).length === 1 && this.rollbackOwner === owner) this.rollbackOwner = undefined;
    }).catch(() => { /* Unknown cleanup remains retryable, including on unmount. */ }).finally(() => {
      if (owner.discardFlight === promise) owner.discardFlight = undefined;
      this.publish();
    });
    owner.discardFlight = promise; this.publish(); return promise;
  }
  consumeMigrationRollback(previewId: string) {
    if (this.rollbackOwner?.previewId === previewId && !this.rollbackOwner.cancelled) { this.rollbackOwner = undefined; this.publish(); }
  }
  private async runtimeMutation(method: string, payload: unknown) {
    const response = await this.invoke<{ status: "accepted"; operation: Operation }>(method, payload);
    if (response.ok && response.data.operation.proxyReport && response.data.operation.proxyReport.mode !== "native") return { ok: false as const, mode: "native" as const, error: { code: "CAPABILITY_UNSUPPORTED", message: "演示代理报告不能作为原生启动受理结果。", retryable: false } };
    await this.refresh();
    return response;
  }
  startRuntime(request: RuntimeStartRequest) {
    return this.runtimeMutation("Runtime.Start", { environmentId: request.environmentId, requestId: request.requestId, networkPolicy: request.networkPolicy, ...(request.purpose ? { purpose: request.purpose } : {}), ...(request.expectedRevision !== undefined ? { expectedRevision: request.expectedRevision } : {}) });
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
  async previewBatch(request: NativeBatchPreviewRequest): Promise<ApplicationResult<NativeBatchPage>> {
    const payload = request.kind === "create" ? { kind: request.kind, create: { previewId: request.create.previewId, configuration: projectConfiguration(request.create.configuration), count: request.create.count, profileHash: request.create.profileHash, requestId: request.create.requestId } }
      : request.kind === "clone" ? { kind: request.kind, sourceIds: [...request.sourceIds] }
      : { kind: request.kind, mappings: request.mappings.map(mapping => ({ environmentId: mapping.environmentId, proxyId: mapping.proxyId })) };
    const response = await this.invoke<NativeBatchPage>("Batch.Preview", payload);
    if (response.ok && (!validBatchPage(response.data) || response.data.kind !== request.kind)) return { ok: false, mode: "native", error: { code: "CAPABILITY_UNSUPPORTED", message: "批次预览类型、统计或模式不匹配，未提交。", retryable: false } };
    return response;
  }
  async readBatchPage(request: { planId: string; operationId?: string; offset: number; pageSize: number }): Promise<ApplicationResult<NativeBatchPage>> {
    const response = await this.invoke<NativeBatchPage>("Batch.ReadPage", { planId: request.planId, offset: request.offset, pageSize: request.pageSize, ...(request.operationId ? { operationId: request.operationId } : {}) });
    if (response.ok && (!validBatchPage(response.data) || response.data.planId !== request.planId || request.operationId && response.data.operationId !== request.operationId || response.data.offset !== request.offset || response.data.pageSize !== request.pageSize)) return { ok: false, mode: "native", error: { code: "CAPABILITY_UNSUPPORTED", message: "批次分页结果不属于指定计划/尝试或页位置，或统计无效。", retryable: false } };
    return response;
  }
  private async batchMutation(method: string, payload: unknown, planId?: string) {
    let response = await this.invoke<{ status: "accepted"; operation: Operation }>(method, payload);
    if (response.ok) {
      if (invalidBatchReport(response.data.operation) || planId && response.data.operation.batchReport?.planId !== planId) return { ok: false as const, mode: "native" as const, error: { code: "CAPABILITY_UNSUPPORTED", message: "批次受理计划身份不匹配，不能作为真实结果。", retryable: false } };
      const checked = this.confirmOperation({ ok: true, mode: "native", data: response.data.operation }); if (!checked.ok) return checked;
      response = { ...response, data: { ...response.data, operation: checked.data } };
    }
    await this.refresh();
    if (response.ok) {
      const latest = this.operations.get(response.data.operation.id);
      if (latest) response = { ...response, data: { ...response.data, operation: latest } };
    }
    return response;
  }
  commitBatch(request: { planId: string; requestId: string }) { return this.batchMutation("Batch.Commit", { planId: request.planId, requestId: request.requestId }, request.planId); }
  retryBatch(request: { operationId: string; requestId: string }) { return this.batchMutation("Batch.Retry", { operationId: request.operationId, requestId: request.requestId }); }
  selectBackupDestination() { return this.invoke<{ status: "selected" | "cancelled"; destinationToken?: string; name?: string }>("Backup.SelectDestination", {}); }
  getPendingRestoreSource() { return this.restoreSourceOwner ? { ...this.restoreSourceOwner } : undefined; }
  private restoreSourceUnconfirmed<T>(): ApplicationResult<T> { return { ok: false, mode: "native", error: { code: "RESTORE_SOURCE_UNCONFIRMED", message: "原来源或清理尚未确认，请核实原来源或重试清理；未另选来源。", retryable: true } }; }
  selectRestoreSource(): Promise<ApplicationResult<{ status: "selected" | "cancelled"; sourceToken?: string; name?: string }>> {
    if (this.rollbackOwner?.cancelled) return Promise.resolve(rollbackCleanupUnconfirmed());
    if (this.restoreSelectionFlight) return this.restoreSourceOwner ? this.restoreSelectionFlight : Promise.resolve(this.restoreSourceUnconfirmed());
    if (this.restoreSourceOwner || this.pendingRestore) return Promise.resolve(this.restoreSourceUnconfirmed());
    const owner: NativeRestoreSourcePending = { requestId: globalThis.crypto.randomUUID(), selectionPending: true, preflightPending: false, cleanupRequested: false };
    this.restoreSourceOwner = owner;
    const promise = this.invoke<NativeRestoreSourceState>("Backup.SelectRestoreSource", { requestId: owner.requestId }).then((result: ApplicationResult<NativeRestoreSourceState>): ApplicationResult<{ status: "selected" | "cancelled"; sourceToken?: string; name?: string }> => {
      owner.selectionPending = false;
      if (this.restoreSourceOwner !== owner) return this.restoreSourceUnconfirmed();
      if (this.restoreSourceOwner === owner && result.ok) {
        if (!validRestoreSourceState(result.data, owner.requestId)) return this.restoreSourceUnconfirmed();
        if (result.data.status === "selected" && typeof result.data.sourceToken === "string" && result.data.sourceToken) { owner.sourceToken = result.data.sourceToken; owner.name = result.data.name; }
        else if (result.data.status === "cancelled") this.restoreSourceOwner = undefined;
        else return this.restoreSourceUnconfirmed<{ status: "selected" | "cancelled"; sourceToken?: string; name?: string }>();
      }
      return result.ok ? { ok: true, mode: "native", data: { status: result.data.status as "selected" | "cancelled", sourceToken: result.data.sourceToken, name: result.data.name } } : result;
    }).finally(() => { if (this.restoreSelectionFlight === promise) this.restoreSelectionFlight = undefined; this.publish(); });
    this.restoreSelectionFlight = promise; return promise;
  }
  async recoverRestoreSource(): Promise<ApplicationResult<NativeRestoreSourceState>> {
    const owner = this.restoreSourceOwner;
    if (!owner || this.pendingRestore) return this.restoreSourceUnconfirmed();
    const result = await this.invoke<NativeRestoreSourceState>("Backup.ReadRestoreSource", { requestId: owner.requestId });
    if (!result.ok) return result;
    if (!validRestoreSourceState(result.data, owner.requestId)) return this.restoreSourceUnconfirmed();
    if (this.restoreSourceOwner !== owner) return this.restoreSourceUnconfirmed();
    if (this.restoreSourceOwner === owner) {
      const state = result.data;
      if (["cancelled", "failed", "discarded"].includes(state.status)) this.restoreSourceOwner = undefined;
      else { owner.sourceToken = state.sourceToken; owner.name = state.name; owner.previewId = state.preview?.previewId; owner.selectionPending = state.status === "selecting"; owner.preflightPending = state.preflightRunning; }
      this.publish();
    }
    return result;
  }
  discardPendingRestoreSource(): Promise<ApplicationResult<{ status: "discarded" }>> {
    if (this.restoreSourceDiscardFlight) return this.restoreSourceDiscardFlight;
    const owner = this.restoreSourceOwner;
    if (!owner || this.pendingRestore) return Promise.resolve(this.restoreSourceUnconfirmed());
    owner.cleanupRequested = true;
    const promise = this.invoke<{ status: "discarded" }>("Backup.DiscardRestore", { requestId: owner.requestId, previewId: owner.previewId ?? "", sourceToken: owner.sourceToken ?? "" }).then(result => {
      if (result.ok && result.data.status === "discarded") { if (this.restoreSourceOwner === owner) this.restoreSourceOwner = undefined; }
      else if (result.ok) return this.restoreSourceUnconfirmed<{ status: "discarded" }>();
      return result;
    }).finally(() => { if (this.restoreSourceDiscardFlight === promise) this.restoreSourceDiscardFlight = undefined; this.publish(); });
    this.restoreSourceDiscardFlight = promise; this.publish(); return promise;
  }
  discardRestore(previewId: string, sourceToken: string) {
    if (sourceToken && this.restoreSourceOwner?.sourceToken === sourceToken) return this.discardPendingRestoreSource();
    return this.invoke<{ status: "discarded" }>("Backup.DiscardRestore", { previewId, sourceToken });
  }
  async previewRestore(sourceToken: string): Promise<ApplicationResult<NativeRestorePreview>> {
    if (this.rollbackOwner?.cancelled) return rollbackCleanupUnconfirmed();
    const owner = this.restoreSourceOwner?.sourceToken === sourceToken ? this.restoreSourceOwner : undefined;
    if (owner?.cleanupRequested || this.restoreSourceOwner && !owner) return this.restoreSourceUnconfirmed();
    if (owner) owner.preflightPending = true;
    const result = await this.invoke<NativeRestorePreview>("Backup.PreviewRestore", { sourceToken });
    if (owner) { owner.preflightPending = false; if (result.ok && this.restoreSourceOwner === owner) owner.previewId = result.data.previewId; this.publish(); }
    if (result.ok && !validRestorePreview(result.data)) return { ok: false, mode: "native", error: { code: "BACKUP_INVALID", message: "恢复预览格式或统计无法核对；没有接入演示数据。", retryable: false } };
    return result; // No refresh: read-only preflight must not flush pending writes.
  }
  async readRestorePage(request: { previewId: string; offset: number; pageSize: number }): Promise<ApplicationResult<NativeRestorePage>> {
    const result = await this.invoke<NativeRestorePage>("Backup.ReadRestorePage", { previewId: request.previewId, offset: request.offset, pageSize: request.pageSize });
    if (result.ok && (result.data.mode !== "native" || result.data.previewId !== request.previewId || result.data.offset !== request.offset || !Number.isSafeInteger(result.data.total) || result.data.total < 0 || !Array.isArray(result.data.items) || result.data.items.length > request.pageSize || result.data.offset + result.data.items.length > result.data.total)) return { ok: false, mode: "native", error: { code: "BACKUP_INVALID", message: "分页结果不属于当前恢复预览，已拒绝使用。", retryable: false } };
    return result;
  }
  getPendingBackupExport(): NativeBackupPending | undefined { return this.pendingBackup ? { ...this.pendingBackup, request: { ...this.pendingBackup.request, environmentIds: [...this.pendingBackup.request.environmentIds] } } : undefined; }
  getPendingRestore() { return this.pendingRestore ? { ...this.pendingRestore, request: { ...this.pendingRestore.request } } : undefined; }
  wasRestoreNotAccepted(requestId: string) { return this.restoreRefusal === requestId; }
  async retryRestore(operationId: string) {
    const result = await this.invoke<Operation>("Backup.RecoverRestore", { operationId });
    if (result.ok && (result.data.id !== operationId || result.data.kind !== "backup-restore")) return { ok: false as const, mode: "native" as const, error: { code: "RESTORE_RESULT_UNCONFIRMED", message: "恢复核对响应不是原任务。", retryable: true } };
    const checked = this.confirmOperation(result); await this.refresh(); return checked;
  }
  private confirmPendingRestore(operation: Operation) {
    if (confirmsRestoreRequest(operation, this.pendingRestore?.request)) { const previewId = this.pendingRestore!.request.previewId; this.consumeMigrationRollback(previewId); if (this.restoreSourceOwner?.previewId === previewId) this.restoreSourceOwner = undefined; this.pendingRestore = undefined; }
  }
  applyRestore(request: NativeRestoreRequest): Promise<ApplicationResult<{ status: "accepted"; operation: Operation }>> {
    const projected = { previewId: request.previewId, archiveSha256: request.archiveSha256, confirmOverwrite: request.confirmOverwrite, acknowledgeCredentials: request.acknowledgeCredentials, stopRunning: request.stopRunning, requestId: request.requestId };
    const signature = JSON.stringify(projected);
    const prior = this.restoreFlights.get(signature);
    if (prior) return prior;
    const promise = this.performRestore(projected).finally(() => { if (this.restoreFlights.get(signature) === promise) this.restoreFlights.delete(signature); });
    this.restoreFlights.set(signature, promise);
    return promise;
  }
  private async performRestore(request: NativeRestoreRequest) {
    const projected = { previewId: request.previewId, archiveSha256: request.archiveSha256, confirmOverwrite: request.confirmOverwrite, acknowledgeCredentials: request.acknowledgeCredentials, stopRunning: request.stopRunning, requestId: request.requestId };
    if (this.rollbackOwner?.cancelled) return rollbackCleanupUnconfirmed<{ status: "accepted"; operation: Operation }>();
    if (this.pendingRestore && JSON.stringify(this.pendingRestore.request) !== JSON.stringify(projected)) return { ok: false as const, mode: "native" as const, operationId: this.pendingRestore.operationId, error: { code: "RESTORE_RESULT_UNCONFIRMED", message: "原恢复受理尚待核实；只可查询或重发原请求。", retryable: true } };
    const owner = this.pendingRestore ??= { request: projected };
    if (this.restoreRefusal === request.requestId) this.restoreRefusal = undefined;
    let response = await this.invoke<{ status: "accepted"; operation: Operation }>("Backup.ApplyRestore", projected);
    if (response.ok) {
      const operationId = response.data.operation?.id || response.operationId;
      if (this.pendingRestore === owner && operationId) owner.operationId = operationId;
      if (response.data.status !== "accepted" || !response.data.operation || !confirmsRestoreRequest(response.data.operation, projected)) response = { ok: false, mode: "native", operationId, error: { code: "RESTORE_RESULT_UNCONFIRMED", message: "恢复受理报告与原请求未一致核实；保持原请求，不创建新任务。", retryable: true } };
      else {
        const checked = this.confirmOperation({ ok: true, mode: "native", data: response.data.operation });
        response = checked.ok ? { ...response, data: { ...response.data, operation: checked.data } } : checked;
      }
    } else if (this.pendingRestore === owner) {
      if (response.operationId) owner.operationId = response.operationId;
      else if (["VALIDATION_FAILED", "PREVIEW_EXPIRED", "BACKUP_INVALID", "REVISION_CONFLICT", "PROFILE_BUSY", "SESSION_IDENTITY_UNCONFIRMED", "REQUEST_ID_REUSED", "RESTORE_NOT_ACCEPTED"].includes(response.error.code)) {
        if (response.error.code === "RESTORE_NOT_ACCEPTED" && owner.operationId) this.operations.delete(owner.operationId);
        this.restoreRefusal = projected.requestId;
        this.pendingRestore = undefined;
        if (this.rollbackOwner?.previewId === projected.previewId && this.rollbackOwner.discardAfterRefusal) await this.discardMigrationRollback();
      }
    }
    await this.refresh();
    const known = [...this.operations.values()].find(op => confirmsRestoreRequest(op, projected));
    if (known) return { ok: true as const, mode: "native" as const, operationId: known.id, data: { status: "accepted" as const, operation: known } };
    return response;
  }
  private confirmPendingBackup(operation: Operation) {
    if (confirmsBackupRequest(operation, this.pendingBackup?.request)) this.pendingBackup = undefined;
  }
  async exportBackup(request: NativeBackupExportRequest) {
    const projected = { scope: request.scope, environmentIds: [...request.environmentIds], destinationToken: request.destinationToken, stopRunning: request.stopRunning, requestId: request.requestId };
    if (this.pendingBackup && JSON.stringify(this.pendingBackup.request) !== JSON.stringify(projected)) return { ok: false as const, mode: "native" as const, operationId: this.pendingBackup.operationId, error: { code: "BACKUP_ACCEPTANCE_UNCONFIRMED", message: "原导出受理尚未核实，不能更换请求、范围或输出；请只核实原请求。", retryable: true } };
    const owner = this.pendingBackup ??= { request: projected };
    const ownsPending = () => this.pendingBackup === owner;
    let response = await this.invoke<{ status: "accepted"; operation: Operation }>("Backup.Export", projected);
    if (response.ok) {
      const operationId = response.data.operation.id || response.operationId;
      if (operationId && ownsPending()) owner.operationId = operationId;
      if (response.data.status !== "accepted" || response.data.operation.kind !== "backup-export" || invalidBackupOperation(response.data.operation) || response.data.operation.backupReport?.scope !== request.scope || response.data.operation.backupReport?.requestId !== request.requestId) response = { ok: false, mode: "native", operationId, error: { code: "BACKUP_RESULT_UNCONFIRMED", message: "备份已返回受理但报告未核实；原请求和已知ID保留，不计为成功也不当作未受理。", retryable: true } };
      else {
        const checked = this.confirmOperation({ ok: true, mode: "native", data: response.data.operation });
        response = checked.ok ? { ...response, data: { ...response.data, operation: checked.data } } : checked;
      }
    } else if (response.operationId) { if (ownsPending()) owner.operationId = response.operationId; }
    else if (["VALIDATION_FAILED", "PREVIEW_EXPIRED", "NOT_FOUND", "PROFILE_BUSY", "SESSION_IDENTITY_UNCONFIRMED", "PATH_OUTSIDE_ROOT", "CAPABILITY_UNSUPPORTED", "REQUEST_ID_REUSED"].includes(response.error.code)) {
      if (ownsPending()) this.pendingBackup = undefined;
    }
    await this.refresh();
    if (response.ok) response = { ...response, data: { ...response.data, operation: this.operations.get(response.data.operation.id) ?? response.data.operation } };
    else {
      const known = [...this.operations.values()].find(operation => confirmsBackupRequest(operation, projected));
      if (known) return { ok: true as const, mode: "native" as const, operationId: known.id, data: { status: "accepted" as const, operation: known } };
    }
    return response;
  }
  queryEnvironments(request: NativeEnvironmentQuery) {
    this.environmentQuery = { page: request.page, pageSize: request.pageSize, search: request.search, group: request.group, status: request.status };
    return this.refresh();
  }
  async previewRecycle(action: NativeRecycleAction, ids: string[]): Promise<ApplicationResult<NativeRecyclePage>> {
    const result = await this.invoke<NativeRecyclePage>("Recycle.Preview", { action, ids: [...ids] });
    if (result.ok && (!validRecyclePage(result.data) || result.data.preview?.action !== action || result.data.total !== new Set(ids).size)) return { ok: false, mode: "native", error: { code: "RECYCLE_RESULT_UNCONFIRMED", message: "影响预览与选中范围不符，没有执行。", retryable: true } };
    return result;
  }
  async readRecyclePage(request: NativeRecyclePageRequest): Promise<ApplicationResult<NativeRecyclePage>> {
    const result = await this.invoke<NativeRecyclePage>("Recycle.ReadPage", { offset: request.offset, pageSize: request.pageSize, ...(request.previewId ? { previewId: request.previewId } : {}), ...(request.operationId ? { operationId: request.operationId } : {}) });
    if (result.ok && !validRecyclePage(result.data, request)) return { ok: false, mode: "native", error: { code: "RECYCLE_RESULT_UNCONFIRMED", message: "回收分页身份或范围不符，未替换当前列表。", retryable: true } };
    if (result.ok && result.data.operation) this.confirmOperation({ ok: true, mode: "native", data: result.data.operation });
    return result;
  }
  getPendingRecycle() { return this.pendingRecycle ? { ...this.pendingRecycle, request: { ...this.pendingRecycle.request } } : undefined; }
  wasRecycleNotAccepted(requestId: string) { return this.recycleRefusal === requestId; }
  private confirmPendingRecycle(operation: Operation) {
    if (confirmsRecycleRequest(operation, this.pendingRecycle?.request)) this.pendingRecycle = undefined;
  }
  async commitRecycle(request: NativeRecycleRequest) {
    const projected = { previewId: request.previewId, confirm: request.confirm, requestId: request.requestId };
    if (this.pendingRecycle && JSON.stringify(this.pendingRecycle.request) !== JSON.stringify(projected)) return { ok: false as const, mode: "native" as const, operationId: this.pendingRecycle.operationId, error: { code: "RECYCLE_RESULT_UNCONFIRMED", message: "原回收请求尚未核实；请查询或重发原请求。", retryable: true } };
    const owner = this.pendingRecycle ??= { request: projected };
    let result = await this.invoke<{ status: "accepted"; operation: Operation }>("Recycle.Commit", projected);
    if (result.ok) {
      const operationId = result.data.operation.id || result.operationId;
      if (this.pendingRecycle === owner && operationId) owner.operationId = operationId;
      if (result.data.status !== "accepted" || !confirmsRecycleRequest(result.data.operation, projected)) result = { ok: false, mode: "native", operationId, error: { code: "RECYCLE_RESULT_UNCONFIRMED", message: "原请求受理结果尚未核实，不重新执行回收或删除。", retryable: true } };
      else { const checked = this.confirmOperation({ ok: true, mode: "native", data: result.data.operation }); result = checked.ok ? { ...result, data: { ...result.data, operation: checked.data } } : checked; }
    } else if (this.pendingRecycle === owner) {
      if (result.operationId) owner.operationId = result.operationId;
      else if (["VALIDATION_FAILED", "PREVIEW_EXPIRED", "NOT_FOUND", "REVISION_CONFLICT", "PROFILE_BUSY", "RECYCLE_NOT_ACCEPTED", "REQUEST_ID_REUSED"].includes(result.error.code)) {
        this.recycleRefusal = projected.requestId; this.pendingRecycle = undefined;
        if (result.error.code === "RECYCLE_NOT_ACCEPTED" && owner.operationId) this.operations.delete(owner.operationId);
      }
    }
    await this.refresh();
    const known = [...this.operations.values()].find(op => confirmsRecycleRequest(op, projected));
    return known ? { ok: true as const, mode: "native" as const, operationId: known.id, data: { status: "accepted" as const, operation: known } } : result;
  }
  async recoverRecycle(operationId: string) {
    const owner = this.pendingRecycle;
    const result = await this.invoke<Operation>("Recycle.Recover", { operationId });
    if (!result.ok && result.error.code === "RECYCLE_NOT_ACCEPTED" && owner && owner.operationId === operationId && this.pendingRecycle === owner) {
      this.recycleRefusal = owner.request.requestId; this.pendingRecycle = undefined; this.operations.delete(operationId);
    }
    if (result.ok && (result.data.id !== operationId || result.data.kind !== "recycle")) return { ok: false as const, mode: "native" as const, error: { code: "RECYCLE_RESULT_UNCONFIRMED", message: "不是原回收任务的结果。", retryable: true } };
    const checked = this.confirmOperation(result); await this.refresh(); return checked;
  }
}
