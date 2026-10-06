import { parseProxyText, uid, type ProxyNode } from "../domain";
import type { ApplicationService, ProxyImportPreview } from "../application/contract";

export type ProxyImportView = Omit<ProxyImportPreview, "mode">;
type ImportRequest = { previewId: string; selectedRows: number[]; requestId: string };
export interface ProxyImportState {
  text: string; showText: boolean; preview?: ProxyImportView; selected: number[]; busy: boolean;
  unknown: boolean; message: string; request?: ImportRequest; fileName?: string;
  singleDraft?: { type: "http" | "https" | "socks5"; host: string; port: string; username: string; password: string };
}
export const proxyFailureIsKnown = (code: string) => ["VALIDATION_FAILED", "PREVIEW_EXPIRED", "PROXY_INVALID", "NOT_FOUND", "REVISION_CONFLICT", "REQUEST_ID_REUSED", "PROFILE_BUSY", "DISK_FULL", "STORAGE_WRITE_FAILED", "STORAGE_READ_FAILED", "CREDENTIALS_UNAVAILABLE", "CAPABILITY_UNSUPPORTED"].includes(code);
const sessions = new WeakMap<ApplicationService, ProxyImportSession>();

/** One volatile import owner per existing application. Hide/unmount is NOT discard. */
export function getProxyImportSession(application: ApplicationService) {
  let owner = sessions.get(application);
  if (!owner) { owner = new ProxyImportSession(application); sessions.set(application, owner); }
  return owner;
}

export class ProxyImportSession {
  private state: ProxyImportState = { text: "", showText: false, selected: [], busy: false, unknown: false, message: "" };
  private listeners = new Set<() => void>();
  private demoNodes = new Map<number, ProxyNode>();
  private generation = 0;
  constructor(readonly application: ApplicationService) {}
  getSnapshot = () => this.state;
  subscribe = (listener: () => void) => { this.listeners.add(listener); return () => { this.listeners.delete(listener); }; };
  private patch(change: Partial<ProxyImportState>) { this.state = { ...this.state, ...change }; this.listeners.forEach(listener => listener()); }
  private discard() {
    const id = this.state.preview?.previewId;
    if (id && this.application.mode === "native") void this.application.discardProxyImport?.(id).catch(() => {});
    this.demoNodes.clear(); this.generation++;
  }
  setText = (text: string) => {
    if (this.state.busy || this.state.unknown) return;
    this.discard(); this.patch({ text, preview: undefined, selected: [], request: undefined, message: "", fileName: undefined, singleDraft: undefined });
  };
  setSingle = (change: Partial<NonNullable<ProxyImportState["singleDraft"]>>) => {
    if (this.state.busy || this.state.unknown) return;
    const singleDraft = { type: "socks5" as const, host: "", port: "1080", username: "", password: "", ...this.state.singleDraft, ...change };
    const host = singleDraft.host.includes(":") && !singleDraft.host.startsWith("[") ? `[${singleDraft.host}]` : singleDraft.host;
    const authentication = singleDraft.username || singleDraft.password ? `${encodeURIComponent(singleDraft.username)}:${encodeURIComponent(singleDraft.password)}@` : "";
    this.setText(`${singleDraft.type}://${authentication}${host}:${singleDraft.port}`);
    this.patch({ singleDraft });
  };
  setShowText = (showText: boolean) => this.patch({ showText });
  select = (line: number, checked: boolean) => {
    if (this.state.busy || this.state.unknown) return;
    const row = this.state.preview?.rows.find(row => row.line === line);
    if (!row?.configuration || row.error) return;
    this.patch({ selected: checked ? [...new Set([...this.state.selected, line])] : this.state.selected.filter(item => item !== line), request: undefined });
  };
  clear = () => {
    if (this.state.busy || this.state.unknown) return;
    this.discard(); this.patch({ text: "", showText: false, preview: undefined, selected: [], request: undefined, fileName: undefined, singleDraft: undefined, message: "输入与预览已明确清除；已经保存的代理不受影响。" });
  };
  async loadFile(file: File) {
    if (this.state.busy || this.state.unknown) return;
    if (file.size > 2 * 1024 * 1024) { this.patch({ message: "文本文件不能超过 2MiB；可分文件导入，这不是代理数量限制。" }); return; }
    const generation = this.generation; this.patch({ busy: true, message: "" });
    try {
      const text = new TextDecoder("utf-8", { fatal: true }).decode(await file.arrayBuffer());
      if (this.generation !== generation) return;
      this.discard(); this.patch({ text, preview: undefined, selected: [], request: undefined, fileName: file.name, singleDraft: undefined, message: "文件已读取，尚未保存；请解析预览。" });
    } catch { this.patch({ message: "文本文件需为有效 UTF-8；没有保存或执行内容。" }); }
    finally { this.patch({ busy: false }); }
  }
  async parse() {
    if (this.state.busy || this.state.unknown || this.application.getSnapshot().issue) return;
    this.discard(); const generation = this.generation;
    this.patch({ busy: true, preview: undefined, selected: [], request: undefined, message: "" });
    try {
      let preview: ProxyImportView;
      if (this.application.mode === "native") {
        const result = await this.application.parseProxyImport?.(this.state.text);
        if (!result?.ok) { this.patch({ message: result ? `${result.error.code}：${result.error.message}` : "当前桌面服务不支持导入，没有模拟保存。" }); return; }
        if (generation !== this.generation) { void this.application.discardProxyImport?.(result.data.previewId); return; }
        preview = result.data;
      } else {
        const rows = parseProxyText(this.state.text);
        const address = (p: ProxyNode) => `${p.type}|${p.host}|${p.port}`;
        const saved = this.application.getSnapshot().state.proxies;
        const groups: ProxyImportView["duplicateGroups"] = {};
        const safeRows = rows.map(row => {
          if (row.node) this.demoNodes.set(row.line, row.node);
          const matching = row.node ? rows.filter(other => other.node && address(other.node) === address(row.node!)).map(other => other.line) : [];
          const existing = row.node ? saved.filter(other => address(other) === address(row.node!)).map(other => other.id) : [];
          const key = `demo-group-${matching[0]}`;
          if (matching.length > 1 || existing.length) groups[key] = { lines: matching, existingProxyIds: existing };
          return { line: row.line, configuration: row.node ? { name: row.node.name, type: row.node.type, host: row.node.host, port: row.node.port, country: row.node.country } : undefined, hasAuthentication: !!(row.node?.username || row.node?.password), error: row.error, duplicateGroupId: matching.length > 1 || existing.length ? key : undefined, duplicateCount: Math.max(0, matching.length - 1), existingCount: existing.length };
        });
        preview = { previewId: uid("demo-proxy-preview"), expiresAt: "", rows: safeRows, ignoredLines: this.state.text.split(/\r?\n/).length - rows.length, duplicateGroups: groups };
      }
      this.patch({ preview, selected: preview.rows.filter(row => row.configuration && !row.error && !row.duplicateCount && !row.existingCount).map(row => row.line), message: preview.rows.length ? "已解析，尚未保存。重复候选默认不选；地址相同不代表认证相同。" : "请先输入至少一条代理。" });
    } catch { this.patch({ message: "解析未确认，请重试；原始输入保留。" }); }
    finally { this.patch({ busy: false }); }
  }
  async commit(): Promise<string[] | undefined> {
    if (this.state.busy || !this.state.preview || !this.state.selected.length || this.application.getSnapshot().issue) return;
    const request = this.state.request ?? { previewId: this.state.preview.previewId, selectedRows: [...this.state.selected].sort((a, b) => a - b), requestId: crypto.randomUUID() };
    this.patch({ busy: true, request, message: "" });
    try {
      let ids: string[], lines: number[];
      if (this.application.mode === "native") {
        const result = await this.application.commitProxyImport?.(request);
        if (!result?.ok) {
          const unknown = !!result && !proxyFailureIsKnown(result.error.code);
          this.patch({ unknown, message: result ? `${result.error.code}：${result.error.message}${unknown ? " 原请求结果未知，输入与所选行已冻结；只能核实原请求，关闭不取消提交。" : " 输入与预览已保留，可修正或重试。"}` : "导入服务不可用，没有模拟保存。" }); return;
        }
        const data = result.data;
        if (data.status !== "completed" || !Array.isArray(data.importedIds) || !Array.isArray(data.importedLines) || data.importedIds.length !== data.importedLines.length || data.importedLines.length !== request.selectedRows.length || new Set(data.importedIds).size !== data.importedIds.length || data.importedIds.some(id => typeof id !== "string" || !id) || [...data.importedLines].sort((a,b) => a-b).some((line, index) => line !== request.selectedRows[index])) {
          this.patch({ unknown: true, message: "PROXY_RESULT_UNCONFIRMED：导入回执与原所选行不符，保留原请求核实；没有重新导入。" }); return;
        }
        ids = data.importedIds; lines = data.importedLines;
      } else {
        const nodes = request.selectedRows.flatMap(line => { const node = this.demoNodes.get(line); return node ? [node] : []; });
        const result = this.application.compatibility?.update(state => ({ ...state, proxies: [...state.proxies, ...nodes], activities: [{ id: uid("log"), action: "导入代理", target: `${nodes.length} 条`, detail: "有效所选行已保存到演示工作区；没有真实检测。", result: "success" as const, time: new Date().toISOString() }, ...state.activities].slice(0, 300) }));
        if (!result?.ok) { this.patch({ message: result ? `${result.error.code}：${result.error.message}` : "演示保存服务不可用。" }); return; }
        ids = nodes.map(node => node.id); lines = request.selectedRows;
      }
      const saved = new Set(lines), remaining = this.state.text.split(/\r?\n/).filter((_, index) => !saved.has(index + 1)).join("\n");
      this.demoNodes.clear(); this.generation++;
      this.patch({ text: remaining, preview: undefined, selected: [], request: undefined, singleDraft: undefined, unknown: false, showText: false, message: `已保存 ${ids.length} 个代理；${remaining.trim() ? "未选与错误行仍保留，可修正后重新解析。" : "原始凭据输入已清除。"}` });
      return ids;
    } catch { this.patch({ unknown: true, message: "原导入结果未知；保留原 requestId、预览与所选行，请核实原请求。" }); }
    finally { this.patch({ busy: false }); }
  }
}
