import type { Environment, State } from "../domain.ts";

export type ApplicationMode = "demo" | "native";
export interface ApplicationError {
  code: string;
  message: string;
  retryable: boolean;
  field?: string;
	itemIndex?: number;
	details?: { reason?: string; kernelId?: string };
}
export type ApplicationResult<T> =
  | { ok: true; mode: ApplicationMode; data: T; operationId?: string }
  | { ok: false; mode: ApplicationMode; error: ApplicationError; operationId?: string };

// v1 prototype fields are a UI projection, not the future SQLite schema.
export type EnvironmentConfiguration = Pick<Environment,
  "name" | "group" | "note" | "proxyId" | "coreId" | "seed" | "language" |
  "timezone" | "cpu" | "width" | "height" | "urls" | "restoreTabs" | "fingerprintVersion"
>;
export interface EnvironmentPreview {
  previewId: string;
  environment: Environment;
  expectedRevision?: number;
  fingerprint?: FingerprintPreview;
  userDataRef?: string;
}
export interface SavedEnvironment { record: Environment; revision: number }
export interface WorkspaceView {
  mode: ApplicationMode;
  state: State;
  issue?: ApplicationError;
  damagedRecord?: string;
  kernelRecords?: NativeKernel[];
  kernelOperations?: Operation[];
  fingerprints?: Record<string, ProfileRevision>;
  dataReferences?: Record<string, string>;
  runtimeSessions?: Record<string, RuntimeSession>;
  nativeProxyRecords?: NativeProxy[];
  proxyOperations?: Operation[];
}
export interface Operation {
  id: string;
  kind: "create" | "edit" | "kernel-install" | "kernel-verify" | "kernel-delete" | "runtime-start" | "runtime-stop" | "runtime-force-stop" | "runtime-reconcile" | "proxy-check";
  state: "accepted" | "running" | "completed" | "cancelled" | "failed";
  total: number;
  completedIds: string[];
  cancelRequested: boolean;
  error?: ApplicationError;
  stage?: string;
  persistencePending?: boolean;
  kernelId?: string;
  report?: KernelReport;
  environmentId?: string;
  sessionId?: string;
  proxyId?: string;
  proxyReport?: ProxyCheckReport;
}
export interface OperationEvent {
  mode: ApplicationMode;
  type: "OperationProgress" | "OperationCompleted";
  operationId: string;
  sequence: number;
  time: string;
  operation: Operation;
}
export const operationIsTerminal = (operation: Operation) =>
  !operation.persistencePending && ["completed", "cancelled", "failed"].includes(operation.state);

export function mergeOperation(previous: Operation | undefined, next: Operation): Operation {
  if (!previous || previous.id !== next.id) return next;
  if (operationIsTerminal(previous) || (previous.state === "running" && next.state === "accepted")) return previous;
  if (previous.persistencePending && !next.persistencePending && !["completed", "cancelled", "failed"].includes(next.state)) return previous;
  return previous.cancelRequested && !next.cancelRequested ? { ...next, cancelRequested: true } : next;
}

export function newerOperationEvent(previous: OperationEvent | undefined, next: OperationEvent) {
  if (previous && previous.operationId === next.operationId &&
    (next.sequence <= previous.sequence || operationIsTerminal(previous.operation))) return previous;
  return next;
}

export interface CreateBatchRequest {
  previewId: string;
  configuration: EnvironmentConfiguration;
  count: number;
  requestId: string;
  profileHash?: string;
}
export interface UpdateEnvironmentRequest {
  previewId: string;
  configuration: EnvironmentConfiguration;
  expectedRevision: number;
  requestId: string;
  profileHash?: string;
}
export interface DemoCompatibility {
  update(change: (state: State) => State): ApplicationResult<WorkspaceView>;
  handleStorageChange(event: { key: string | null }): void;
  resetDamagedWorkspace(): Promise<ApplicationResult<WorkspaceView>>;
}

export interface ApplicationService {
  readonly mode: ApplicationMode;
  readonly compatibility?: DemoCompatibility;
  getSnapshot(): WorkspaceView;
  refresh?(): Promise<ApplicationResult<WorkspaceView>>;
  subscribe(listener: () => void): () => void;
  subscribeEvents(listener: (event: OperationEvent) => void): () => void;
  previewEnvironment(request: { kind: "create" | "edit"; sourceId?: string }): Promise<ApplicationResult<EnvironmentPreview>>;
  regeneratePreview(previewId: string): Promise<ApplicationResult<EnvironmentPreview>>;
  discardPreview(previewId: string): Promise<ApplicationResult<{ status: "discarded" }>>;
  generateFingerprint(request: GenerateFingerprintRequest): Promise<ApplicationResult<EnvironmentPreview>>;
  listFingerprintRevisions(environmentId: string): Promise<ApplicationResult<ProfileRevision[]>>;
  previewFingerprintRestore(previewId: string, revision: number): Promise<ApplicationResult<EnvironmentPreview>>;
  commitFingerprintRevision(request: CommitFingerprintRequest): Promise<ApplicationResult<{ status: "completed"; environment: SavedEnvironment; newRevision: number; fingerprintRevision: number }>>;
  createBatch(request: CreateBatchRequest): Promise<ApplicationResult<{ status: "accepted"; operation: Operation }>>;
  updateEnvironment(request: UpdateEnvironmentRequest): Promise<ApplicationResult<{ status: "completed"; environment: SavedEnvironment }>>;
  getOperation(operationId: string): Promise<ApplicationResult<Operation>>;
  cancelOperation(operationId: string): Promise<ApplicationResult<Operation>>;
  selectKernelArchive?(): Promise<ApplicationResult<{ status: "selected" | "cancelled"; archiveToken?: string; name?: string }>>;
  installKernel?(request: KernelInstallRequest): Promise<ApplicationResult<{ status: "accepted"; operation: Operation }>>;
  verifyKernel?(kernelId: string, requestId: string): Promise<ApplicationResult<{ status: "accepted"; operation: Operation }>>;
  deleteKernel?(kernelId: string, requestId: string): Promise<ApplicationResult<{ status: "accepted"; operation: Operation }>>;
  startRuntime?(request: { environmentId: string; requestId: string; networkPolicy: "direct" | "proxy" }): Promise<ApplicationResult<{ status: "accepted"; operation: Operation }>>;
  stopRuntime?(request: { environmentId: string; requestId: string }): Promise<ApplicationResult<{ status: "accepted"; operation: Operation }>>;
  inspectRuntime?(ids: string[]): Promise<ApplicationResult<RuntimeSession[]>>;
  forceStopRuntime?(request: { environmentId: string; sessionId: string; requestId: string }): Promise<ApplicationResult<{ status: "accepted"; operation: Operation }>>;
  reconcileRuntime?(request: { environmentId: string; sessionId: string; requestId: string }): Promise<ApplicationResult<{ status: "accepted"; operation: Operation }>>;
  parseProxyImport?(text: string): Promise<ApplicationResult<ProxyImportPreview>>;
  discardProxyImport?(previewId: string): Promise<ApplicationResult<{ status: "discarded" }>>;
  commitProxyImport?(request: { previewId: string; selectedRows: number[]; requestId: string }): Promise<ApplicationResult<{ status: "completed"; importedIds: string[]; importedLines: number[] }>>;
  updateProxy?(request: ProxyUpdateRequest): Promise<ApplicationResult<{ status: "completed"; record: NativeProxy }>>;
  deleteProxy?(request: ProxyTargetRequest): Promise<ApplicationResult<{ status: "completed"; deletedId: string }>>;
  checkProxy?(request: ProxyTargetRequest): Promise<ApplicationResult<{ status: "accepted"; operation: Operation }>>;
}

export interface ProxyConfiguration {
  name: string; type: "http" | "https" | "socks5"; host: string; port: number; country: string;
}
export type ProxyCredentialChange =
  | { action: "keep" | "clear" }
  | { action: "replace"; username: string; password: string };
export interface ProxyTargetRequest { proxyId: string; expectedRevision: number; requestId: string }
export interface ProxyUpdateRequest extends ProxyTargetRequest { configuration: ProxyConfiguration; credentials: ProxyCredentialChange }
export interface NativeProxy extends ProxyConfiguration {
  id: string; revision: number; hasAuthentication: boolean; status: "unchecked" | "connected" | "failed";
  usedBy: string[]; checkReport?: ProxyCheckReport;
}
export interface ProxyImportPreview {
  mode: "native"; previewId: string; expiresAt: string; ignoredLines: number;
  rows: { line: number; configuration?: ProxyConfiguration; hasAuthentication: boolean; duplicateGroupId?: string; duplicateCount: number; existingCount: number; error?: string }[];
  duplicateGroups: Record<string, { lines: number[]; existingProxyIds: string[] }>;
}
export interface ProxyCheckReport {
	channelId?: string;
  resolutionPolicy?: "remote-target-dns";
  mode: "native"; adapterVersion: string; proxyId: string; revision: number;
  startedAt: string; finishedAt: string; durationMs: number; targetOrigin: string;
  steps: { stage: string; status: "running" | "passed" | "failed" | "unsupported"; time: string; message: string }[];
  exitIp?: string; error?: ApplicationError;
}

export interface RuntimeSession {
  mode: "native";
  environmentId: string;
  sessionId: string;
  operationId: string;
  state: Environment["status"];
  revision: number;
  fingerprintRevision: number;
  kernelId: string;
  userDataRef: string;
  networkPolicy: "direct" | "proxy";
  proxyId?: string;
  proxyRevision?: number;
  proxyChannelId?: string;
  proxyReport?: ProxyCheckReport;
  pid?: number;
  processCreatedAt?: string;
  startedAt?: string;
  error?: ApplicationError;
  rootPid?: number;
  canControl: boolean;
  canForce: boolean;
  needsReconcile: boolean;
  persistencePending: boolean;
  nextAction?: string;
  reconciledAt?: string;
  lastExitCode?: number;
}

export interface KernelInstallRequest {
  source: "official" | "local";
  version: string;
  expectedChecksum: string;
  archiveToken?: string;
  trusted: boolean;
  requestId: string;
}
export interface KernelObservation {
  seed: number; browserVersion: string; httpUserAgent: string; userAgent: string;
  cpu: number; memory: number; language: string; timezone: string;
  gpuVendor: string; gpuRenderer: string; pid: number; processCreatedAt: string; normalExit: boolean;
}
export interface KernelReport {
  adapterVersion: string; version: string; sampledAt: string; transport: string; sandbox: boolean;
  observations: KernelObservation[];
  capabilities: FingerprintCapability[];
}
export interface NativeKernel {
  id: string; version: string; architecture: string;
  source: { kind: "official" | "local"; location: string; tag: string; commit: string | null };
  archiveSha256: string; executableSha256: string; executableRelativePath: string;
  installPath: string; installedAt: string; status: "verified" | "missing"; usedBy: string[];
  report: KernelReport;
}

export interface FingerprintCapability {
  field: string;
  status: "configurable" | "seed-generated" | "system" | "unverified";
  source: string;
  note: string;
}
export interface DeviceProfile {
  schemaVersion: number; configRevision: number; seed: string;
  templateId: string; templateVersion: string; generatorVersion: string;
  platform: string; platformVersion: string; brand: string; brandVersion: string;
  kernelId: string; coreActualVersion: string; coreExecutableSha256: string;
  adapterVersion: string; capabilityVersion: string;
  language: string; acceptLanguages: string[]; uiLanguage: string; timezone: string;
  regionPreset: string; cpu: string; width: number; height: number;
  parameters: string[]; configHash: string;
}
export interface FingerprintPreview {
  mode: ApplicationMode;
  previewProfile: DeviceProfile;
  capabilityReport: {
    kernelId: string; evidenceStatus: string; capabilities: FingerprintCapability[];
    observedFingerprint: KernelObservation | null; canLaunchNative: boolean;
  };
  changes: { field: string; before: string; after: string }[];
  action: string;
  restoredFrom?: number;
}
export interface ProfileRevision {
  profile: DeviceProfile; createdAt: string; action: string; restoredFrom?: number;
}
export interface GenerateFingerprintRequest {
  previewId: string; kernelId: string; templateId: string; regenerate?: boolean;
  overrides: Pick<EnvironmentConfiguration, "language" | "timezone" | "cpu" | "width" | "height">;
}
export interface CommitFingerprintRequest extends UpdateEnvironmentRequest {
  environmentId: string; profileHash: string;
}
