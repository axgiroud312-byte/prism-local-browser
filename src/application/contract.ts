import type { Environment, State } from "../domain.ts";

export type ApplicationMode = "demo" | "native";
export interface ApplicationError {
  code: string;
  message: string;
  retryable: boolean;
  field?: string;
  itemIndex?: number;
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
}
export interface SavedEnvironment { record: Environment; revision: number }
export interface WorkspaceView {
  mode: ApplicationMode;
  state: State;
  issue?: ApplicationError;
  damagedRecord?: string;
}
export interface Operation {
  id: string;
  kind: "create" | "edit";
  state: "accepted" | "running" | "completed" | "cancelled" | "failed";
  total: number;
  completedIds: string[];
  cancelRequested: boolean;
  error?: ApplicationError;
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
  ["completed", "cancelled", "failed"].includes(operation.state);

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
}
export interface UpdateEnvironmentRequest {
  previewId: string;
  configuration: EnvironmentConfiguration;
  expectedRevision: number;
  requestId: string;
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
  createBatch(request: CreateBatchRequest): Promise<ApplicationResult<{ status: "accepted"; operation: Operation }>>;
  updateEnvironment(request: UpdateEnvironmentRequest): Promise<ApplicationResult<{ status: "completed"; environment: SavedEnvironment }>>;
  getOperation(operationId: string): Promise<ApplicationResult<Operation>>;
  cancelOperation(operationId: string): Promise<ApplicationResult<Operation>>;
}
