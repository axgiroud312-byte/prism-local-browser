import { mergeOperation, operationIsTerminal, type ApplicationResult, type ApplicationService, type KernelInstallRequest, type Operation } from "../application/contract";
import { proxyFailureIsKnown } from "./proxy-import-session";

export type KernelTaskRequest = { kind: "install"; request: KernelInstallRequest } | { kind: "verify" | "delete"; kernelId: string; requestId: string };
interface State { busy: boolean; unknown: boolean; pending?: KernelTaskRequest; lastRequest?: KernelTaskRequest; operation?: Operation; message: string }
const owners = new WeakMap<ApplicationService, KernelTaskOwner>();
export function getKernelTaskOwner(application: ApplicationService) { let owner = owners.get(application); if (!owner) { owner = new KernelTaskOwner(application); owners.set(application, owner); } return owner; }
class KernelTaskOwner {
  private state: State = { busy: false, unknown: false, message: "" };
  private listeners = new Set<() => void>();
  constructor(private application: ApplicationService) {}
  getSnapshot = () => this.state;
  subscribe = (listener: () => void) => { this.listeners.add(listener); return () => { this.listeners.delete(listener); }; };
  private patch(change: Partial<State>) { this.state = { ...this.state, ...change }; this.listeners.forEach(listener => listener()); }
  message = (message: string) => this.patch({ message });
  observe = (operation: Operation, targetId: string) => {
    if (operation.id !== targetId || this.state.operation && this.state.operation.id !== targetId) return;
    this.patch({ operation: mergeOperation(this.state.operation, operation) });
  };
  select = (operation: Operation) => {
    if (!this.state.busy && !this.state.unknown) this.patch({ operation, ...(operation.id !== this.state.operation?.id ? { lastRequest: undefined } : {}) });
  };
  async begin(request: KernelTaskRequest): Promise<boolean> {
    if (this.state.busy || this.application.getSnapshot().issue || this.state.operation && !operationIsTerminal(this.state.operation)) return false;
    const owner = this.state.unknown ? this.state.pending! : request;
    this.patch({ busy: true, pending: owner, message: "" });
    try {
      let result: ApplicationResult<{ status: "accepted"; operation: Operation }> | undefined;
      if (owner.kind === "install") result = await this.application.installKernel?.(owner.request);
      else if (owner.kind === "verify") result = await this.application.verifyKernel?.(owner.kernelId, owner.requestId);
      else result = await this.application.deleteKernel?.(owner.kernelId, owner.requestId);
      if (!result?.ok) {
        const known = !result || proxyFailureIsKnown(result.error.code) || ["KERNEL_MISSING", "KERNEL_INTEGRITY_FAILED", "KERNEL_BUSY"].includes(result.error.code);
        this.patch({ unknown: !known, pending: known ? undefined : owner, message: result ? `${result.error.code}：${result.error.message}${!known ? " 原请求是否受理未知，只能核实同一请求。" : " 没有报告任务成功，可修正后重试。"}` : "当前服务不支持内核操作，没有模拟执行。" }); return false;
      }
      const operation = result.data.operation;
      if (!operation?.id || operation.kind !== `kernel-${owner.kind}` || owner.kind !== "install" && operation.kernelId && operation.kernelId !== owner.kernelId) { this.patch({ unknown: true, message: "KERNEL_RESULT_UNCONFIRMED：内核回执身份不符，保留原请求核实。" }); return false; }
      this.patch({ pending: undefined, lastRequest: owner, unknown: false, operation, message: "内核任务已受理，尚不等于完成或可用。" });
      // Admission is already confirmed. A later read failure must not erase
      // that receipt or turn it into an unknown request with no replay owner.
      try { await this.application.refresh?.(); }
      catch { this.message("内核任务已受理；工作区刷新未确认，请重新读取，不重复提交。"); }
      return true;
    } catch { this.patch({ unknown: true, message: "原内核提交结果未知，保留原请求核实，不另建任务。" }); return false; }
    finally { this.patch({ busy: false }); }
  }
  retry = () => this.state.pending ? this.begin(this.state.pending) : Promise.resolve(false);
  repeat = () => {
    const previous = this.state.lastRequest;
    if (!previous || !this.state.operation || !operationIsTerminal(this.state.operation) || this.state.operation.state === "completed") return Promise.resolve(false);
    return this.begin(previous.kind === "install" ? { ...previous, request: { ...previous.request, requestId: crypto.randomUUID() } } : { ...previous, requestId: crypto.randomUUID() });
  };
}
