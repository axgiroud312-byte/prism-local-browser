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
  defaultKernel?: KernelDefault;
  fingerprints?: Record<string, ProfileRevision>;
  dataReferences?: Record<string, string>;
  runtimeSessions?: Record<string, RuntimeSession>;
  nativeProxyRecords?: NativeProxy[];
  proxyOperations?: Operation[];
  cookieOperations?: Operation[];
  batchOperations?: Operation[];
  backupOperations?: Operation[];
  restoreOperations?: Operation[];
  recycleOperations?: Operation[];
  recycleMaintenance?: Operation;
  migrationOperations?: Operation[];
  migrationMaintenance?: Operation;
  maintenance?: Operation;
  nativeBackups?: NativeBackup[];
  environmentPage?: NativeEnvironmentPage;
}
export interface Operation {
  id: string;
  kind: "create" | "edit" | "kernel-install" | "kernel-verify" | "kernel-delete" | "runtime-start" | "runtime-stop" | "runtime-force-stop" | "runtime-reconcile" | "proxy-check" | "cookie-import" | "batch-create" | "batch-clone" | "batch-assign" | "backup-export" | "backup-restore" | "recycle" | "migration";
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
  cookieReport?: CookieImportReport;
  batchReport?: NativeBatchReport;
  backupReport?: NativeBackupReport;
  restoreReport?: NativeRestoreReport;
  recycleReport?: NativeRecycleReport;
  migrationReport?: NativeMigrationReport;
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
  if (previous?.batchReport && next.batchReport && previous.batchReport.planId === next.batchReport.planId && next.batchReport.sequence < previous.batchReport.sequence) return previous;
  if (!previous || previous.id !== next.id) return next;
  if (previous.migrationReport && next.migrationReport) {
    if (next.migrationReport.sequence < previous.migrationReport.sequence) return previous;
    if (previous.stage === "acceptance-pending" && previous.persistencePending && !next.persistencePending) return next;
    if (previous.persistencePending && next.migrationReport.sequence > previous.migrationReport.sequence) return next;
  }
  if (previous.recycleReport && next.recycleReport) {
    if (next.recycleReport.sequence < previous.recycleReport.sequence) return previous;
    if (previous.persistencePending && next.recycleReport.sequence > previous.recycleReport.sequence) return next;
  }
  if (previous.restoreReport && next.restoreReport) {
    if (next.restoreReport.sequence < previous.restoreReport.sequence) return previous;
    if (previous.stage === "acceptance-pending" && previous.persistencePending && !next.persistencePending) return next;
    if (previous.persistencePending && next.restoreReport.sequence > previous.restoreReport.sequence) return next;
  }
  if (previous.backupReport && next.backupReport && next.backupReport.sequence < previous.backupReport.sequence) return previous;
  if (previous.kind === "backup-export" && previous.stage === "acceptance-pending" && previous.persistencePending && !next.persistencePending) return next;
  if (previous.batchReport && next.batchReport && next.batchReport.sequence < previous.batchReport.sequence) return previous;
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
  getDiagnosticState?(): DiagnosticState;
  previewDiagnostics?(): Promise<ApplicationResult<DiagnosticPreview>>;
  exportDiagnostics?(): Promise<ApplicationResult<DiagnosticReceipt>>;
  endDiagnosticVerification?(): Promise<ApplicationResult<DiagnosticReceipt>>;
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
  previewRecycle?(action: NativeRecycleAction, ids: string[]): Promise<ApplicationResult<NativeRecyclePage>>;
  readRecyclePage?(request: NativeRecyclePageRequest): Promise<ApplicationResult<NativeRecyclePage>>;
  commitRecycle?(request: NativeRecycleRequest): Promise<ApplicationResult<{ status: "accepted"; operation: Operation }>>;
  recoverRecycle?(operationId: string): Promise<ApplicationResult<Operation>>;
  getPendingRecycle?(): { request: NativeRecycleRequest; operationId?: string } | undefined;
  wasRecycleNotAccepted?(requestId: string): boolean;
  selectKernelArchive?(): Promise<ApplicationResult<{ status: "selected" | "cancelled"; archiveToken?: string; name?: string }>>;
  installKernel?(request: KernelInstallRequest): Promise<ApplicationResult<{ status: "accepted"; operation: Operation }>>;
  verifyKernel?(kernelId: string, requestId: string): Promise<ApplicationResult<{ status: "accepted"; operation: Operation }>>;
  deleteKernel?(kernelId: string, requestId: string): Promise<ApplicationResult<{ status: "accepted"; operation: Operation }>>;
  setDefaultKernel?(request: KernelDefaultRequest): Promise<ApplicationResult<KernelDefault>>;
  getPendingKernelDefault?(): KernelDefaultRequest | undefined;
  previewMigration?(environmentId: string, kernelId: string): Promise<ApplicationResult<NativeMigrationPreview>>;
  lookupMigrationEnvironments?(page: number, search: string): Promise<ApplicationResult<{ items: Pick<Environment, "id" | "name" | "coreId" | "status">[]; page: number; total: number }>>;
  prepareMigration?(request: NativeMigrationRequest): Promise<ApplicationResult<{ status: "accepted"; operation: Operation }>>;
  migrationAction?(operationId: string, action: "stop" | "commit" | "recover"): Promise<ApplicationResult<Operation>>;
  previewMigrationRollback?(operationId: string): Promise<ApplicationResult<NativeRestorePreview>>;
  discardMigrationRollback?(): Promise<void>;
  consumeMigrationRollback?(previewId: string): void;
  getPendingMigration?(): { request: NativeMigrationRequest; operationId?: string } | undefined;
  wasMigrationNotAccepted?(requestId: string): boolean;
  startRuntime?(request: RuntimeStartRequest): Promise<ApplicationResult<{ status: "accepted"; operation: Operation }>>;
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
  parseCookieImport?(environmentId: string, text: string): Promise<ApplicationResult<CookieImportPreview>>;
  discardCookieImport?(previewId: string): Promise<ApplicationResult<{ status: "discarded" }>>;
  commitCookieImport?(request: CookieCommitRequest): Promise<ApplicationResult<{ status: "accepted"; operation: Operation }>>;
  previewBatch?(request: NativeBatchPreviewRequest): Promise<ApplicationResult<NativeBatchPage>>;
  readBatchPage?(request: { planId: string; operationId?: string; offset: number; pageSize: number }): Promise<ApplicationResult<NativeBatchPage>>;
  selectBackupDestination?(): Promise<ApplicationResult<{ status: "selected" | "cancelled"; destinationToken?: string; name?: string }>>;
  exportBackup?(request: NativeBackupExportRequest): Promise<ApplicationResult<{ status: "accepted"; operation: Operation }>>;
  getPendingBackupExport?(): NativeBackupPending | undefined;
  selectRestoreSource?(): Promise<ApplicationResult<{ status: "selected" | "cancelled"; sourceToken?: string; name?: string }>>;
  previewRestore?(sourceToken: string): Promise<ApplicationResult<NativeRestorePreview>>;
  readRestorePage?(request: { previewId: string; offset: number; pageSize: number }): Promise<ApplicationResult<NativeRestorePage>>;
  discardRestore?(previewId: string, sourceToken: string): Promise<ApplicationResult<{ status: "discarded" }>>;
  applyRestore?(request: NativeRestoreRequest): Promise<ApplicationResult<{ status: "accepted"; operation: Operation }>>;
  getPendingRestore?(): { request: NativeRestoreRequest; operationId?: string } | undefined;
  wasRestoreNotAccepted?(requestId: string): boolean;
  retryRestore?(operationId: string): Promise<ApplicationResult<Operation>>;
  commitBatch?(request: { planId: string; requestId: string }): Promise<ApplicationResult<{ status: "accepted"; operation: Operation }>>;
  retryBatch?(request: { operationId: string; requestId: string }): Promise<ApplicationResult<{ status: "accepted"; operation: Operation }>>;
  queryEnvironments?(request: NativeEnvironmentQuery): Promise<ApplicationResult<WorkspaceView>>;
}

export interface NativeBackupExportRequest {
  scope: "all" | "selected"; environmentIds: string[]; destinationToken: string; stopRunning: boolean; requestId: string;
}

export interface DiagnosticReport {
  format: "prism-local-diagnostics"; schemaVersion: 1; generatedAt: string;
  application: { version: string; platform: string; architecture: string; goVersion: string; signature: "not-checked" };
  proxyProtection: "available" | "unavailable"; excluded: string[];
  workspace: {
    status: "available" | "partial" | "unavailable"; startupCode?: string; schemaVersion?: number;
    counts: Record<string, number>; maintenance: DiagnosticOperation[]; operations: DiagnosticOperation[];
    sessions: { label: string; state: string; networkPolicy: string; errorCode?: string; networkErrorCode?: string; containment: string; needsReconcile: boolean; persistencePending: boolean }[];
    kernels: { label: string; version: string; status: string; archiveSha256: string; executableSha256: string }[];
    unavailableSections: string[]; omittedRecords: number; observationSource: string;
    operationLimit: number; sessionLimit: number; kernelLimit: number;
  };
}
export interface DiagnosticOperation {
  label: string; kind: string; state: string; stage: string; errorCode?: string;
  persistencePending: boolean; cancelRequested: boolean; total: number; completed: number; progressMetric: string;
}
export interface DiagnosticPreview { reportId: string; sha256: string; bytes: number; expiresAt: string; report: DiagnosticReport }
export interface DiagnosticReceipt { status: "saved" | "cancelled" | "unconfirmed"; reportId: string; sha256?: string }
export interface DiagnosticState { preview?: DiagnosticPreview; pending: boolean; busy: boolean; receipt?: DiagnosticReceipt; error?: ApplicationError }
export interface NativeRestorePreview {
  mode: "native"; previewId: string; format: "prism-local-backup"; name: string;
  archiveSha256: string; manifestSha256: string; scope: "all" | "selected"; createdAt: string; expiresAt: string;
  environmentCount: number; addCount: number; overwriteCount: number; conflictCount: number; missingKernelCount: number; credentialReentryCount: number;
  bytes: number; canRestore: boolean;
  kernels: { id: string; version: string; archiveSha256: string; executableSha256: string; localId: string; state: "pending" | "missing" | "unavailable" | "verified-bytes"; required: boolean }[];
  credentials: { proxyId: string; state: "none" | "available-current-user" | "reentry-required" }[];
}
export interface NativeRestoreRequest {
  previewId: string; archiveSha256: string; confirmOverwrite: boolean; acknowledgeCredentials: boolean; stopRunning: boolean; requestId: string;
}
export interface NativeRestoreReport {
  mode: "native"; requestId: string; previewId: string; archiveSha256: string; sequence: number;
  environmentCount: number; switchedCount: number; credentialReentryCount: number;
  committed: boolean; rolledBack: boolean; protected: boolean;
  recoveredAfterRestart?: boolean; interruptedStage?: string;
}

export type NativeRecycleAction = "remove" | "restore" | "purge";
export interface NativeRecycleRequest { previewId: string; confirm: boolean; requestId: string }
export interface NativeRecycleReport {
  mode: "native"; action: NativeRecycleAction; requestId: string; previewId: string;
  sequence: number; completed: number; failed: number; notExecuted: number; protected: boolean;
}
export interface NativeRecycleItem {
  id: string; environmentId: string; name: string; seed: string; kernelId: string; revision: number;
  dataPresent: boolean; backupRecorded: boolean; removedAt?: string;
  state: "pending" | "recycled" | "restored" | "purged" | "failed" | "protected";
  error?: ApplicationError;
}
export interface NativeRecyclePreview {
  mode: "native"; previewId: string; action: NativeRecycleAction; expiresAt: string;
  total: number; dataCount: number; backupCount: number;
}
export interface NativeRecyclePageRequest { offset: number; pageSize: number; previewId?: string; operationId?: string }
export interface NativeRecyclePage {
  mode: "native"; offset: number; pageSize: number; total: number; items: NativeRecycleItem[];
  preview?: NativeRecyclePreview; operation?: Operation;
}
export interface NativeRestorePage {
  mode: "native"; previewId: string; offset: number; total: number;
  items: { id: string; name: string; seed: string; action: "add" | "overwrite"; currentRevision: number; backupRevision: number; dataState: "present" | "never-initialized"; busy: boolean; conflicts: string[] }[];
}
export interface NativeBackupReport {
  requestId: string;
  mode: "native"; format: "prism-local-backup"; schemaVersion: 1; scope: "all" | "selected";
  environmentCount: number; copiedEnvironmentCount: number; fileCount: number; byteCount: number; sequence: number;
  published: boolean; name: string; archiveSha256?: string; manifestSha256?: string;
  credentials: "windows-current-user-dpapi"; browserData: "sensitive-same-user-not-portable"; kernelBinariesIncluded: false;
}
export interface NativeBackupPending { request: NativeBackupExportRequest; operationId?: string }
export interface NativeBackup {
  id: string; operationId: string; name: string; createdAt: string; scope: "all" | "selected";
  environmentCount: number; archiveSha256: string; manifestSha256: string;
}

export type NativeBatchKind = "create" | "clone" | "assign";
export type NativeBatchPreviewRequest =
  | { kind: "create"; create: CreateBatchRequest }
  | { kind: "clone"; sourceIds: string[] }
  | { kind: "assign"; mappings: { environmentId: string; proxyId: string }[] };
export interface NativeBatchItem {
  index: number; name: string; environmentId?: string; sourceId?: string; sourceRevision?: number;
  expectedRevision?: number; proxyId: string; proxyName: string; newIdentity: boolean;
  state: "not-executed" | "completed" | "failed"; error?: ApplicationError;
}
export interface NativeBatchPage extends NativeBatchReport {
  offset: number; pageSize: number; expiresAt: string; operationId?: string;
  currentOperationId?: string; history?: boolean;
  items: NativeBatchItem[];
}
export interface NativeBatchReport {
  mode: "native"; planId: string; kind: NativeBatchKind; total: number;
  completedCount: number; failedCount: number; notExecutedCount: number; attemptCompletedCount: number;
  sharedProxyAssignments: number; finishedAt?: string;
  directAssignments: number;
  sequence: number;
}
export interface NativeEnvironmentQuery {
  page: number; pageSize: number; search: string; group: string; status: string;
}
export interface NativeEnvironmentPage { page: number; pageSize: number; total: number; filteredTotal: number; groups: string[]; runningCount: number; errorCount: number }

export interface RuntimeStartRequest { environmentId: string; requestId: string; networkPolicy: "direct" | "proxy"; purpose?: "cookie-import"; expectedRevision?: number }
export interface CookiePartitionKey { topLevelSite: string; hasCrossSiteAncestor: boolean }
// Safe projections deliberately do not include value, raw input or CDP data.
export interface NativeCookieRow {
  index: number; name?: string; domain?: string; hostOnly: boolean; path?: string; session: boolean;
  secure: boolean; httpOnly: boolean; sameSite?: "Strict" | "Lax" | "None";
  expires?: number; partitionKey?: CookiePartitionKey; expired: boolean;
  conflict: boolean; existingConflict: boolean; errorCode?: string; message?: string;
}
export interface CookieImportPreview {
  mode: "native"; previewId: string; environmentId: string; environmentName: string;
  expectedRevision: number; sessionId?: string; requiresStart: boolean;
  format: "json" | "netscape" | ""; expiresAt: string; rows: NativeCookieRow[];
  total: number; validCount: number; errorCount: number; expiredCount: number; conflictCount: number;
  existingConflictCount?: number; observationError?: ApplicationError;
}
export interface CookieCommitRequest {
  previewId: string; environmentId: string; expectedRevision: number; sessionId: string;
  selectedRows: number[]; policy: "merge" | "replace-all"; requestId: string;
}
export interface CookieItemResult extends NativeCookieRow { status: "pending" | "verified" | "already-matched" | "failed" | "unknown" | "expired" | "cancelled" }
export interface CookieImportReport {
  mode: "native"; previewId: string; environmentId: string; sessionId: string; revision: number; policy: "merge" | "replace-all";
  clearState: "not-requested" | "pending" | "not-attempted" | "verified-empty" | "unknown";
  verifiedCount: number; writtenCount: number; alreadyMatchedCount: number; failedCount: number;
  skippedCount: number; unconfirmedCount: number; finishedAt?: string; items: CookieItemResult[];
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
  usedBy: string[]; usedCount?: number; checkReport?: ProxyCheckReport;
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
  networkFault?: { state: "network_error"; error: ApplicationError; observedAt: string; containment: "stopping" | "stopped" | "exit-unconfirmed" };
  pid?: number;
  processCreatedAt?: string;
  startedAt?: string;
  error?: ApplicationError;
  rootPid?: number;
  canControl: boolean;
  canForce: boolean;
  needsReconcile: boolean;
  persistencePending: boolean;
  /** Current host still owns session resources, even when no browser was created. */
  resourcesPending?: boolean;
  nextAction?: string;
  reconciledAt?: string;
  lastExitCode?: number;
}

export interface KernelDefault { kernelId: string; revision: number }
export interface NativeMigrationRequest { previewId: string; confirm: boolean; requestId: string }
export interface NativeMigrationPreview {
  mode: "native"; previewId: string; environmentId: string; name: string; expectedRevision: number;
  before: DeviceProfile; after: DeviceProfile; beforeCapabilities: FingerprintCapability[]; afterCapabilities: FingerprintCapability[];
  changes: { field: string; before: string; after: string }[]; expiresAt: string;
}
export interface NativeMigrationObservation { fingerprint: KernelObservation; cookie: boolean; localStorage: boolean; indexedDB: boolean; sampledAt: string }
export interface NativeMigrationReport {
  mode: "native"; requestId: string; previewId: string; environmentId: string; oldKernelId: string; newKernelId: string; seed: string;
  sequence: number; backupVerified: boolean; archiveSha256: string; trialExited: boolean; committed: boolean; protected: boolean;
  before?: NativeMigrationObservation; after?: NativeMigrationObservation;
}
export interface KernelDefaultRequest { kernelId: string; expectedRevision: number; requestId: string }
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
  installPath: string; installedAt: string; status: "verified" | "missing"; usedBy: string[]; usedCount?: number;
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
