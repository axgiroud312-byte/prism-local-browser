import { useEffect, useRef, useState } from "react";
import { Download, Plus } from "lucide-react";
import { parseSnapshot, type Backup, type Snapshot, type State } from "../domain";
import { ReferenceButton } from "./ReferenceUi";
import { LocalPageTable, LocalPageWindow, LocalPagination, RestoreConfirmWindow, localTime } from "./LocalPageUi";

export interface DemoBackupPageProps {
  state: State;
  onCreate(): boolean | Promise<boolean>;
  onRestore(snapshot: Snapshot, name: string): void;
  onDownload(backup: Backup): void;
  disabled?: boolean;
}

export function DemoBackupPage({ state, onCreate, onRestore, onDownload, disabled = false }: DemoBackupPageProps) {
  const [window, setWindow] = useState<"create" | "import">(), [file, setFile] = useState<File>();
  const [busy, setBusy] = useState(false), [error, setError] = useState(""), [search, setSearch] = useState(""), [listPage, setListPage] = useState(1);
  const fileInput = useRef<HTMLInputElement>(null), flight = useRef(false);
  useEffect(() => {
    const input = fileInput.current;
    const cancelled = () => setError("已取消文件选择；未校验或恢复，原输入保留。");
    input?.addEventListener("cancel", cancelled);
    return () => input?.removeEventListener("cancel", cancelled);
  }, [window]);
  const records = state.backups.filter(backup => backup.name.includes(search));
  const page = Math.min(listPage, Math.max(1, Math.ceil(records.length / 10)));
  const create = async () => {
    if (flight.current || disabled) return; flight.current = true; setBusy(true); setError("");
    try { if (await onCreate()) setWindow(undefined); else setError("本次快照未保存；原工作区不变，恢复存储后可重试。"); }
    catch { setError("本次快照未保存；原工作区不变。"); }
    finally { flight.current = false; setBusy(false); }
  };
  const inspect = async () => {
    if (!file || flight.current || disabled) return; flight.current = true; setBusy(true); setError("");
    try {
      if (file.size > 10 * 1024 * 1024) throw new Error("原型文件请控制在 10 MB 内；这不是产品环境数量限制。");
      const snapshot = parseSnapshot(await file.text());
      setWindow(undefined); onRestore(snapshot, file.name);
    } catch (error) { setError((error as Error).message); }
    finally { flight.current = false; setBusy(false); }
  };
  return <section className="local-page local-page-backups" aria-label="演示快照管理">
    <div className="local-page-tabs"><span className="selected">演示 JSON 快照</span></div>
    <div className="local-page-panel"><div className="local-page-toolbar">
      <ReferenceButton className="primary" disabled={disabled} onClick={() => { setWindow("create"); setError(""); }}><Plus size={15} />创建快照</ReferenceButton>
      <ReferenceButton disabled={disabled} onClick={() => { setWindow("import"); setFile(undefined); setError(""); }}>导入快照文件</ReferenceButton>
      <input className="local-page-search" aria-label="搜索快照名称" placeholder="输入快照名称搜索" value={search} onChange={event => { setSearch(event.target.value); setListPage(1); }} /><span className="local-page-tail subtle-text">当前浏览器 · 演示模式</span>
    </div><LocalPageTable headings={["快照名称", "环境数量", "创建时间", "格式", "操作"]} empty={!records.length ? "尚无匹配快照；创建演示快照或导入兼容 JSON。" : undefined}>{records.slice((page - 1) * 10, page * 10).map(backup => <tr key={backup.id}><td>{backup.name}</td><td>{backup.snapshot.environments.length} 个</td><td>{localTime(backup.createdAt)}</td><td>prism-prototype v1</td><td><div className="local-page-row-actions"><button className="local-page-link" disabled={disabled} onClick={() => onDownload(backup)}><Download size={12} /> 导出</button><button className="local-page-link" disabled={disabled} onClick={() => onRestore(backup.snapshot, backup.name)}>恢复</button></div></td></tr>)}</LocalPageTable><LocalPagination total={records.length} page={page} onPage={setListPage} />
      <p className="local-page-note">只保存全工作区演示配置与示例 Cookie，排除代理密码；不含真实浏览目录，不可导入 native。请勿填写真实 Cookie。</p>
    </div>
    {window === "create" && <LocalPageWindow title="创建演示快照" height={340} busy={busy} onClose={() => setWindow(undefined)} footer={<><ReferenceButton disabled={busy} onClick={() => setWindow(undefined)}>取消</ReferenceButton><ReferenceButton className="primary" disabled={busy || disabled} onClick={() => void create()}>创建快照</ReferenceButton></>}>
      <label className="reference-field"><span>快照范围</span><input readOnly value={`全部 ${state.environments.length} 个演示环境`} /></label><label className="reference-field"><span>保存位置</span><input readOnly value="当前浏览器的演示存储" /></label>
      <p>prism-prototype / schemaVersion 1。首次写入成功才加入快照列表；不会停止真实环境，也不会创建 .prismbackup。</p><p className="subtle-text">包含示例 Cookie，排除代理密码、活动列表和快照历史。建议另导出文件留存。</p>{error && <p role="alert">{error}</p>}
    </LocalPageWindow>}
    {window === "import" && <LocalPageWindow title="导入演示快照" height={359} busy={busy} onClose={() => setWindow(undefined)} footer={<><ReferenceButton disabled={busy} onClick={() => setWindow(undefined)}>取消</ReferenceButton><ReferenceButton className="primary" disabled={busy || disabled || !file} onClick={() => void inspect()}>校验并预览</ReferenceButton></>}>
      <p>只接收 prism-prototype v1 JSON；完整 .prismbackup 和诊断 JSON 不能用于此入口。</p><div className="local-page-upload"><div className="local-page-upload-head"><span>演示 JSON 文件<br />本地读取，不上传</span><ReferenceButton disabled={busy} onClick={() => fileInput.current?.click()}>选择快照文件</ReferenceButton></div><p>{file?.name ?? "尚未选择文件"}</p></div><input hidden ref={fileInput} type="file" accept=".json" aria-label="演示快照文件" onChange={event => { const selected = event.target.files?.[0]; event.target.value = ""; if (selected) { setFile(selected); setError(""); } }} />
      <p className="subtle-text">演示文件 10 MB 解析保护，不是环境数量配额。校验通过仍须单独确认替换。</p>{error && <p role="alert">{error}</p>}
    </LocalPageWindow>}
  </section>;
}

export function DemoRestoreConfirmWindow({ snapshot, name, runningCount, error, busy, onClose, onConfirm }: {
  snapshot: Snapshot; name: string; runningCount: number; error?: string; busy?: boolean; onClose(): void; onConfirm(): void;
}) {
  return <RestoreConfirmWindow title="确认恢复演示快照" confirmText="确认恢复" busy={busy} disabled={runningCount > 0} onClose={onClose} onConfirm={onConfirm}>
    <p>{name}：替换当前全部演示环境、代理和内核配置（{snapshot.environments.length} 个环境 / {snapshot.proxies.length} 个代理）。</p><p>保留原 seed；环境重置为已停止，代理密码清空且需重新检查。快照历史与活动不被文件覆盖；不操作真实目录。</p>
    {runningCount > 0 && <p role="alert">请先停止 {runningCount} 个模拟运行/启动/停止中的环境；本次未恢复。</p>}{error && <p role="alert">{error}</p>}
  </RestoreConfirmWindow>;
}

export function DemoRestoreWindow(props: { snapshot: Snapshot; name: string; runningCount: number; error?: string; busy?: boolean; onClose(): void; onConfirm(): void }) {
  const [confirmOpen, setConfirmOpen] = useState(false);
  return <><LocalPageWindow title="演示快照恢复预览" width={1040} height={592} className="local-page-preflight" onClose={props.onClose} busy={props.busy} footer={<><ReferenceButton disabled={props.busy} onClick={props.onClose}>取消</ReferenceButton><ReferenceButton className="primary" disabled={props.busy} onClick={() => setConfirmOpen(true)}>下一步：确认恢复</ReferenceButton></>}>
    <p>格式校验通过 · 尚未恢复 · {props.name}</p><div className="local-page-scope"><span>环境 {props.snapshot.environments.length}</span><span>代理 {props.snapshot.proxies.length}</span><span>演示内核 {props.snapshot.kernels.length}</span></div>
    <LocalPageTable headings={["演示环境", "分组", "恢复影响", "身份 / 网络"]} empty={!props.snapshot.environments.length ? "快照没有环境；仍会替换演示配置。" : undefined}>{props.snapshot.environments.map(environment => <tr key={environment.id}><td>{environment.name}</td><td>{environment.group || "未分组"}</td><td>替换为快照配置 · 停止状态</td><td>原 seed 保留 · 代理凭据重填</td></tr>)}</LocalPageTable>
    <p className="local-page-note">不是完整本机恢复。继续确认才调用既有演示兼容入口，写入失败不会发布新状态。</p>{props.error && <p role="alert">{props.error}</p>}
  </LocalPageWindow>{confirmOpen && <DemoRestoreConfirmWindow {...props} onClose={() => setConfirmOpen(false)} />}</>;
}
