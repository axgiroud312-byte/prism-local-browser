import {
  now, uid, uniqueSeed, seedState, parseSnapshot, validateEnvironment, STORAGE_KEY,
  type Environment, type State,
} from "../domain.ts";
import {
  operationIsTerminal,
  type ApplicationService, type ApplicationError, type ApplicationResult,
  type WorkspaceView, type EnvironmentPreview, type EnvironmentConfiguration,
  type Operation, type OperationEvent, type CreateBatchRequest, type UpdateEnvironmentRequest,
} from "./contract.ts";

type StoragePort = Pick<Storage, "getItem" | "setItem" | "removeItem">;
type Draft = { kind: "create" | "edit"; preview: EnvironmentPreview };
const error = (code: string, message: string, retryable = false): ApplicationError => ({ code, message, retryable });
const active = (record: Environment) => ["running", "starting", "stopping"].includes(record.status);
const freeze = <T>(value: T): T => {
  if (value && typeof value === "object" && !Object.isFrozen(value)) {
    Object.values(value).forEach(freeze);
    Object.freeze(value);
  }
  return value;
};
function configuration(input: EnvironmentConfiguration): EnvironmentConfiguration {
  // Never accept identity, runtime state or Cookie writes from the editor.
  return {
    name: input.name.trim(), group: input.group, note: input.note, proxyId: input.proxyId,
    coreId: input.coreId, seed: input.seed, language: input.language, timezone: input.timezone,
    cpu: input.cpu, width: input.width, height: input.height, urls: input.urls,
    restoreTabs: input.restoreTabs, fingerprintVersion: input.fingerprintVersion,
  };
}
function readState(raw: string): State {
  const parsed = JSON.parse(raw);
  const snapshot = parseSnapshot(JSON.stringify({ ...parsed, format: "prism-prototype" }));
  if (!Array.isArray(parsed.backups) || !Array.isArray(parsed.activities)) throw new Error("invalid workspace");
  for (const b of parsed.backups) {
    parseSnapshot(JSON.stringify(b.snapshot));
    if (typeof b.name !== "string" || typeof b.id !== "string" || !Number.isFinite(Date.parse(b.createdAt))) throw new Error("invalid backup");
  }
  for (const a of parsed.activities) {
    if (!a || !["id", "action", "target", "detail"].every(k => typeof a[k] === "string") ||
      !["success", "error", "info"].includes(a.result) || !Number.isFinite(Date.parse(a.time))) throw new Error("invalid activity");
  }
  return { schemaVersion: 1, environments: snapshot.environments, proxies: snapshot.proxies,
    kernels: snapshot.kernels, backups: parsed.backups, activities: parsed.activities };
}

export class DemoAdapter implements ApplicationService {
  readonly mode = "demo" as const;
  private storage: StoragePort;
  private view: WorkspaceView;
  private savedRaw: string | null = null;
  private revisions: Record<string, number> = Object.create(null);
  private drafts = new Map<string, Draft>();
  private operations = new Map<string, Operation>();
  private requests = new Map<string, { signature: string; result: ApplicationResult<unknown> }>();
  private listeners = new Set<() => void>();
  private eventListeners = new Set<(event: OperationEvent) => void>();
  private sequence = 0;
  private activeOperation?: string;
  private yieldControl: () => Promise<void>;

  constructor(storage: StoragePort, options: { yieldControl?: () => Promise<void> } = {}) {
    this.storage = storage;
    this.yieldControl = options.yieldControl ?? (() => new Promise(resolve => setTimeout(resolve, 40)));
    let state = seedState();
    let issue: ApplicationError | undefined;
    let damagedRecord: string | undefined;
    try { this.savedRaw = storage.getItem(STORAGE_KEY); }
    catch { issue = error("STORAGE_READ_FAILED", "无法读取本地演示记录。请恢复浏览器存储权限后重新载入。", true); }
    if (!issue && this.savedRaw !== null) {
      try {
        state = readState(this.savedRaw);
        const metadata = JSON.parse(this.savedRaw)._application;
        if (metadata !== undefined) {
          if (!metadata || metadata.version !== 1 || !metadata.revisions ||
            typeof metadata.revisions !== "object" || Array.isArray(metadata.revisions) ||
            Object.values(metadata.revisions).some(n => !Number.isSafeInteger(n) || Number(n) < 1)) throw new Error("invalid metadata");
          this.revisions = metadata.revisions;
        }
        state.environments = state.environments.map(e => ({ ...e, status: ["starting", "stopping"].includes(e.status) ? "ready" : e.status }));
      } catch {
        damagedRecord = this.savedRaw;
        issue = error("STORAGE_DAMAGED", "本地演示记录损坏，原始内容已保留。可先导出原始记录，再重置示例工作区。");
      }
    }
    for (const e of state.environments) if (!Object.hasOwn(this.revisions, e.id)) this.revisions[e.id] = 1;
    this.view = freeze({ mode: this.mode, state, issue, damagedRecord });
  }

  getSnapshot = () => this.view;
  subscribe = (listener: () => void) => { this.listeners.add(listener); return () => { this.listeners.delete(listener); }; };
  subscribeEvents = (listener: (event: OperationEvent) => void) => { this.eventListeners.add(listener); return () => { this.eventListeners.delete(listener); }; };
  private success<T>(data: T, operationId?: string): ApplicationResult<T> { return { ok: true, mode: this.mode, data: structuredClone(data), ...(operationId ? { operationId } : {}) }; }
  private failure<T>(failure: ApplicationError, operationId?: string): ApplicationResult<T> { return { ok: false, mode: this.mode, error: failure, ...(operationId ? { operationId } : {}) }; }
  private notify() { for (const listener of this.listeners) { try { listener(); } catch { /* Observers cannot undo a persisted commit. */ } } }
  private checkStorage(): ApplicationError | undefined {
    if (this.view.issue) return this.view.issue;
    try {
      if (this.storage.getItem(STORAGE_KEY) === this.savedRaw) return;
      this.view = freeze({ ...this.view, issue: error("WORKSPACE_CHANGED", "另一个页面已更新工作区。请重新载入最新记录，避免覆盖其他页面的修改。", true) });
    } catch { this.view = freeze({ ...this.view, issue: error("STORAGE_READ_FAILED", "无法读取本地演示记录，请恢复存储权限后重新载入。", true) }); }
    this.notify();
    return this.view.issue;
  }
  private commit(state: State, revisions: Record<string, number>, reset = false): ApplicationResult<WorkspaceView> {
    const blocked = reset ? undefined : this.checkStorage();
    if (blocked) return this.failure(blocked);
    try { readState(JSON.stringify(state)); }
    catch { return this.failure(error("VALIDATION_FAILED", "工作区引用或记录无效，本次修改未保存，原记录仍在。")); }
    try {
      const serialized = JSON.stringify({ ...state, _application: { version: 1, revisions } });
      this.storage.setItem(STORAGE_KEY, serialized);
      this.savedRaw = serialized;
    } catch { return this.failure(error("STORAGE_WRITE_FAILED", "浏览器存储写入失败，本次修改未保存，原记录仍在。请释放空间后重试。", true)); }
    this.revisions = revisions;
    this.view = freeze({ mode: this.mode, state });
    this.notify();
    return this.success(this.view);
  }

  readonly compatibility = {
    update: (change: (state: State) => State): ApplicationResult<WorkspaceView> => {
      const blocked = this.checkStorage();
      if (blocked) return this.failure(blocked);
      if (this.activeOperation) return this.failure(error("PROFILE_BUSY", "创建任务仍在处理，请等待完成或取消余下任务后再修改工作区。", true));
      let next: State;
      try { next = change(structuredClone(this.view.state)); readState(JSON.stringify(next)); }
      catch { return this.failure(error("VALIDATION_FAILED", "演示数据不完整，本次修改未保存。")); }
      const revisions: Record<string, number> = {};
      for (const e of next.environments) {
        const old = this.view.state.environments.find(i => i.id === e.id);
        revisions[e.id] = old ? (this.revisions[e.id] ?? 1) + (JSON.stringify(old) === JSON.stringify(e) ? 0 : 1) : 1;
      }
      return this.commit(next, revisions);
    },
    handleStorageChange: (event: { key: string | null }) => this.handleStorageChange(event),
    resetDamagedWorkspace: () => this.resetDamagedWorkspace(),
  };
  handleStorageChange(event: { key: string | null }) { if (event.key === null || event.key === STORAGE_KEY) this.checkStorage(); }
  async resetDamagedWorkspace(): Promise<ApplicationResult<WorkspaceView>> {
    if (this.view.damagedRecord === undefined) return this.failure(error("VALIDATION_FAILED", "没有可重置的损坏演示记录。"));
    try {
      if (this.storage.getItem(STORAGE_KEY) !== this.savedRaw) return this.failure(error("WORKSPACE_CHANGED", "记录已经被其他页面修改，请重新载入。", true));
    } catch { return this.failure(error("STORAGE_READ_FAILED", "无法读取原记录，请恢复存储权限后重试。", true)); }
    const state = seedState();
    const revisions = Object.fromEntries(state.environments.map(e => [e.id, 1]));
    return this.commit(state, revisions, true);
  }

  async previewEnvironment(request: { kind: "create" | "edit"; sourceId?: string }): Promise<ApplicationResult<EnvironmentPreview>> {
    const blocked = this.checkStorage();
    if (blocked) return this.failure(blocked);
    const source = this.view.state.environments.find(e => e.id === request.sourceId);
    if ((request.sourceId || request.kind === "edit") && !source) return this.failure(error("NOT_FOUND", "环境不存在，请刷新列表。", true));
    if (request.kind === "edit" && active(source!)) return this.failure(error("PROFILE_BUSY", "请先关闭该环境，再修改配置。", true));
    const environment: Environment = request.kind === "edit" ? structuredClone(source!) : {
      id: uid("draft"), code: "", name: source ? `${source.name} 副本` : "",
      group: source?.group || "日常运营", note: "", proxyId: source?.proxyId || "",
      coreId: source?.coreId || this.view.state.kernels[0]?.id || "",
      seed: uniqueSeed(this.view.state.environments.map(e => e.seed)),
      language: source?.language || "en-US", timezone: source?.timezone || "America/New_York",
      cpu: source?.cpu || "auto", width: source?.width || 1280, height: source?.height || 800,
      urls: source?.urls || "", restoreTabs: true, status: "ready", cookies: [],
      createdAt: now(), fingerprintVersion: "windows-desktop-v1",
    };
    const preview: EnvironmentPreview = { previewId: uid("preview"), environment,
      ...(request.kind === "edit" ? { expectedRevision: this.revisions[source!.id] } : {}) };
    this.drafts.set(preview.previewId, { kind: request.kind, preview: structuredClone(preview) });
    return this.success(preview);
  }
  async regeneratePreview(previewId: string): Promise<ApplicationResult<EnvironmentPreview>> {
    const blocked = this.checkStorage();
    if (blocked) return this.failure(blocked);
    const draft = this.drafts.get(previewId);
    if (!draft) return this.failure(error("PREVIEW_EXPIRED", "预览已失效，请重新打开配置。", true));
    if (draft.kind === "edit") {
      const current = this.view.state.environments.find(e => e.id === draft.preview.environment.id);
      if (!current) return this.failure(error("PREVIEW_EXPIRED", "环境已移除，请关闭预览。", true));
      if (active(current)) return this.failure(error("PROFILE_BUSY", "环境正在运行，请停止后重新打开配置。", true));
    }
    draft.preview.environment.seed = uniqueSeed([...this.view.state.environments.map(e => e.seed), draft.preview.environment.seed]);
    return this.success(draft.preview);
  }
  async discardPreview(previewId: string): Promise<ApplicationResult<{ status: "discarded" }>> {
    this.drafts.delete(previewId);
    return this.success({ status: "discarded" });
  }
  private memo<T>(method: string, request: { requestId: string }): ApplicationResult<T> | undefined {
    if (!request.requestId?.trim()) return this.failure(error("VALIDATION_FAILED", "缺少操作请求标识。"));
    const stored = this.requests.get(request.requestId);
    if (!stored) return;
    if (stored.signature !== `${method}:${JSON.stringify(request)}`) return this.failure(error("REQUEST_ID_REUSED", "同一请求标识不能用于不同操作，请重新提交。"));
    return structuredClone(stored.result) as ApplicationResult<T>;
  }
  private remember<T>(method: string, request: { requestId: string }, result: ApplicationResult<T>) {
    this.requests.set(request.requestId, { signature: `${method}:${JSON.stringify(request)}`, result: structuredClone(result) });
    return result;
  }
  private validate(config: EnvironmentConfiguration, base: Environment, excludeId?: string, checkName = true): ApplicationError | undefined {
    try {
      const candidate = { ...base, ...configuration(config), code: base.code || "preview" };
      const message = validateEnvironment(candidate, this.view.state);
      if (message) return error("VALIDATION_FAILED", message);
      parseSnapshot(JSON.stringify({ ...this.view.state, format: "prism-prototype", environments: [candidate] }));
      if (checkName && this.view.state.environments.some(e => e.id !== excludeId && e.name === candidate.name)) return error("VALIDATION_FAILED", "已有同名环境，请换一个名称。");
      if (this.view.state.environments.some(e => e.id !== excludeId && e.seed === candidate.seed)) return error("VALIDATION_FAILED", "种子与另一环境重复，请显式生成新的档案。");
    } catch { return error("VALIDATION_FAILED", "环境配置字段无效，请检查后重试。"); }
  }
  private publishOperation(operation: Operation) {
    const snapshot = structuredClone(operation);
    this.operations.set(operation.id, operation);
    const event: OperationEvent = freeze({ mode: this.mode, type: operationIsTerminal(operation) ? "OperationCompleted" : "OperationProgress",
      operationId: operation.id, sequence: ++this.sequence, time: now(), operation: snapshot });
    for (const listener of this.eventListeners) { try { listener(event); } catch { /* Isolate view observers. */ } }
  }

  async updateEnvironment(request: UpdateEnvironmentRequest) {
    const cached = this.memo<{ status: "completed"; environment: { record: Environment; revision: number } }>("update", request);
    if (cached) return cached;
    const blocked = this.checkStorage();
    if (blocked) return this.failure<{ status: "completed"; environment: { record: Environment; revision: number } }>(blocked);
    const draft = this.drafts.get(request.previewId);
    const old = draft && this.view.state.environments.find(e => e.id === draft.preview.environment.id);
    if (!draft || draft.kind !== "edit" || !old) return this.failure<{ status: "completed"; environment: { record: Environment; revision: number } }>(error("PREVIEW_EXPIRED", "预览已失效，请重新打开配置。", true));
    if (active(old)) return this.failure<{ status: "completed"; environment: { record: Environment; revision: number } }>(error("PROFILE_BUSY", "请先关闭该环境，再修改配置。", true));
    if (request.expectedRevision !== this.revisions[old.id]) return this.failure<{ status: "completed"; environment: { record: Environment; revision: number } }>(error("REVISION_CONFLICT", "配置已被更新，请重新打开最新记录后重试。", true));
    const invalid = this.validate(request.configuration, old, old.id);
    if (invalid) return this.failure<{ status: "completed"; environment: { record: Environment; revision: number } }>(invalid);
    const record = { ...old, ...configuration(request.configuration), status: "ready" as const, error: undefined };
    const revision = this.revisions[old.id] + 1;
    const next = { ...this.view.state, environments: this.view.state.environments.map(e => e.id === old.id ? record : e),
      activities: [{ id: uid("log"), time: now(), action: "修改环境", target: record.name, detail: "配置已持久保存；仅显式编辑改变档案。", result: "success" as const }, ...this.view.state.activities] };
    const committed = this.commit(next, { ...this.revisions, [old.id]: revision });
    if (!committed.ok) return committed;
    const operationId = uid("op");
    this.publishOperation({ id: operationId, kind: "edit", state: "completed", total: 1, completedIds: [old.id], cancelRequested: false });
    this.drafts.delete(request.previewId);
    return this.remember("update", request, this.success({ status: "completed" as const, environment: { record, revision } }, operationId));
  }
  async createBatch(request: CreateBatchRequest): Promise<ApplicationResult<{ status: "accepted"; operation: Operation }>> {
    const cached = this.memo<{ status: "accepted"; operation: Operation }>("create", request);
    if (cached) return cached;
    const blocked = this.checkStorage();
    if (blocked) return this.failure(blocked);
    const draft = this.drafts.get(request.previewId);
    if (!draft || draft.kind !== "create") return this.failure(error("PREVIEW_EXPIRED", "预览已失效，请重新打开配置。", true));
    if (!Number.isSafeInteger(request.count) || request.count < 1) return this.failure(error("VALIDATION_FAILED", "创建数量应为正整数。"));
    if (this.activeOperation) return this.failure(error("PROFILE_BUSY", "请等待当前创建批次完成，或取消余下任务。", true));
    if (this.view.state.environments.some(e => ["starting", "stopping"].includes(e.status)))
      return this.failure(error("PROFILE_BUSY", "环境正在启动或关闭，请等待当前操作结束后重试创建。", true));
    const invalid = this.validate(request.configuration, draft.preview.environment, undefined, request.count === 1);
    if (invalid) return this.failure(invalid);
    const operation: Operation = { id: uid("op"), kind: "create", state: "accepted", total: request.count, completedIds: [], cancelRequested: false };
    this.operations.set(operation.id, operation);
    this.activeOperation = operation.id;
    const result = this.remember("create", request, this.success({ status: "accepted" as const, operation }, operation.id));
    this.drafts.delete(request.previewId);
    void this.runCreation(operation, draft.preview.environment, configuration(request.configuration));
    return result;
  }
  private async runCreation(operation: Operation, base: Environment, config: EnvironmentConfiguration) {
    let pendingError: ApplicationError | undefined;
    try {
      for (let offset = 0; offset < operation.total && !operation.cancelRequested; offset += 25) {
        const state = this.view.state;
        const invalidReference = validateEnvironment({ ...base, ...config }, state);
        if (invalidReference) { pendingError = error("VALIDATION_FAILED", invalidReference); break; }
        const names = new Set(state.environments.map(e => e.name));
        const seeds = state.environments.map(e => e.seed);
        const maxCode = state.environments.reduce((n, e) => Math.max(n, Number(e.code) || 0), 0);
        const added: Environment[] = [];
        for (let i = offset; i < Math.min(offset + 25, operation.total); i++) {
          const name = operation.total === 1 ? config.name : `${config.name} ${String(i + 1).padStart(2, "0")}`;
          if (names.has(name)) { pendingError = error("VALIDATION_FAILED", "批次中遇到同名环境，后续创建已停止；已保存条目保留。", false); break; }
          const seed = i === 0 ? config.seed : uniqueSeed(seeds);
          seeds.push(seed); names.add(name);
          added.push({ ...base, ...config, id: uid("env"), name, seed, code: String(maxCode + added.length + 1).padStart(3, "0"), cookies: [], status: "ready", createdAt: now() });
        }
        if (added.length) {
          const committed = this.commit({ ...state, environments: [...added, ...state.environments],
            activities: [{ id: uid("log"), time: now(), action: "创建环境", target: `${added.length} 个环境`, detail: "演示配置已持久保存；未创建真实浏览器目录。", result: "success" as const }, ...state.activities] },
            { ...this.revisions, ...Object.fromEntries(added.map(e => [e.id, 1])) });
          if (!committed.ok) { pendingError = committed.error; break; }
          operation.completedIds.push(...added.map(e => e.id));
        }
        operation.state = "running";
        this.publishOperation(operation);
        if (pendingError) break;
        await this.yieldControl();
      }
    } catch { pendingError = error("RESOURCE_EXHAUSTED", "创建任务因资源错误中断，已保存条目仍在。请释放资源后重试。", true); }
    operation.state = pendingError ? "failed" : operation.cancelRequested ? "cancelled" : "completed";
    operation.error = pendingError;
    this.activeOperation = undefined;
    this.publishOperation(operation);
  }
  async getOperation(operationId: string): Promise<ApplicationResult<Operation>> {
    const operation = this.operations.get(operationId);
    return operation ? this.success(operation, operationId) : this.failure(error("NOT_FOUND", "任务不存在或属于已关闭的演示会话。", true));
  }
  async cancelOperation(operationId: string): Promise<ApplicationResult<Operation>> {
    const operation = this.operations.get(operationId);
    if (!operation) return this.failure(error("NOT_FOUND", "任务不存在，未取消其他任务。"));
    if (!operationIsTerminal(operation)) {
      operation.cancelRequested = true;
      this.publishOperation(operation);
    }
    return this.success(operation, operationId);
  }
}
