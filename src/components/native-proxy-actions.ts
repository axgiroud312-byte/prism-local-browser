import { mergeOperation, type ApplicationResult, type ApplicationService, type Operation, type ProxyTargetRequest, type ProxyUpdateRequest } from "../application/contract";
import { proxyFailureIsKnown } from "./proxy-import-session";

type ProxyMutation = { kind: "update"; request: ProxyUpdateRequest } | { kind: "delete" | "check"; request: ProxyTargetRequest };
interface State { busy: boolean; unknown: boolean; pending?: ProxyMutation; message: string; operations: Record<string, Operation> }
const owners = new WeakMap<ApplicationService, NativeProxyActions>();
export function getNativeProxyActions(application: ApplicationService) {
  let owner = owners.get(application); if (!owner) { owner = new NativeProxyActions(application); owners.set(application, owner); } return owner;
}
class NativeProxyActions {
  private state: State = { busy: false, unknown: false, message: "", operations: {} };
  private listeners = new Set<() => void>();
  constructor(private application: ApplicationService) {}
  getSnapshot = () => this.state;
  subscribe = (listener: () => void) => { this.listeners.add(listener); return () => { this.listeners.delete(listener); }; };
  private patch(value: Partial<State>) { this.state = { ...this.state, ...value }; this.listeners.forEach(listener => listener()); }
  observe = (operation: Operation) => this.patch({ operations: { ...this.state.operations, [operation.id]: mergeOperation(this.state.operations[operation.id], operation) } });
  message = (message: string) => this.patch({ message });
  async run(mutation: ProxyMutation): Promise<boolean> {
    if (this.state.busy || this.application.getSnapshot().issue) return false;
    const owner = this.state.unknown ? this.state.pending! : mutation;
    this.patch({ busy: true, pending: owner, message: "" });
    try {
      let result: ApplicationResult<unknown> | undefined;
      if (owner.kind === "update") result = await this.application.updateProxy?.(owner.request);
      else if (owner.kind === "delete") result = await this.application.deleteProxy?.(owner.request);
      else result = await this.application.checkProxy?.(owner.request);
      if (!result?.ok) {
        const unknown = !!result && !proxyFailureIsKnown(result.error.code);
        this.patch({ unknown, pending: unknown ? owner : undefined, message: result ? `${result.error.code}：${result.error.message}${unknown ? " 保留原请求、节点和修订核实，不能另建操作。" : " 原配置未被此失败响应改写。"}` : "服务不支持此操作，没有模拟执行。" }); return false;
      }
      const data = result.data as { operation?: Operation; record?: { id: string }; deletedId?: string };
      const valid = owner.kind === "check" ? data.operation?.kind === "proxy-check" && data.operation.proxyId === owner.request.proxyId : owner.kind === "update" ? data.record?.id === owner.request.proxyId : data.deletedId === owner.request.proxyId;
      if (!valid) { this.patch({ unknown: true, message: "PROXY_RESULT_UNCONFIRMED：回执与原节点不符；只能核实原请求。" }); return false; }
      if (data.operation) this.observe(data.operation);
      this.patch({ unknown: false, pending: undefined, message: owner.kind === "update" ? "代理已保存，旧检查失效；环境 seed 不变。" : owner.kind === "delete" ? "代理配置已删除。" : "已受理本次检查，等待服务终态；历史成功不授权下一次启动。" });
      return true;
    } catch { this.patch({ unknown: true, message: "原代理请求结果未知，保留原请求核实。" }); return false; }
    finally { this.patch({ busy: false }); }
  }
  retry = () => this.state.pending ? this.run(this.state.pending) : Promise.resolve(false);
}
