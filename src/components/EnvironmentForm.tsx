import type { ReactNode } from "react";
import { Fingerprint, LoaderCircle, Plus, RefreshCw, ShieldCheck, Sparkles } from "lucide-react";
import { regions, type Environment, type Kernel, type ProxyNode } from "../domain";
import type { FingerprintPreview, ProfileRevision } from "../application/contract";
import { FingerprintRevisionPanel } from "./FingerprintRevisionPanel";

function Field({ label, children, hint }: { label: string; children: ReactNode; hint?: string }) {
  return <label className="field"><span>{label}</span>{children}{hint && <small>{hint}</small>}</label>;
}

export function EnvironmentForm({ environment, kind, groups, kernels, proxies, native, busy, generating, profileBusy, kernelLocked, canGenerate, fresh, preview, history, dataRef, quantity, previewError, onChange, onQuantity, onGenerate, onRestore, onImportProxy, onKernels, canConfigure }: {
  environment: Environment; kind: "create" | "edit"; groups: string[]; kernels: Kernel[]; proxies: ProxyNode[];
  native: boolean; busy: boolean; generating: boolean; profileBusy: boolean; kernelLocked: boolean;
  canGenerate: boolean; fresh: boolean; preview?: FingerprintPreview; history?: ProfileRevision[]; dataRef?: string;
  quantity: number; previewError: string; onChange: (patch: Partial<Environment>) => void; onQuantity: (quantity: number) => void;
  onGenerate: (regenerate?: boolean) => void; onRestore: (revision: number) => void; onImportProxy: () => void; onKernels: () => void;
  canConfigure: (field: string) => boolean;
}) {
  const kernel = kernels.find(item => item.id === environment.coreId);
  const frozen = busy || profileBusy;
  return <div className="environment-form">
    <div className="field-row">
      <Field label="环境名称 *"><input aria-label="环境名称" autoComplete="off" disabled={busy} value={environment.name} placeholder="例如：北美主店" onChange={event => onChange({ name: event.target.value })} /></Field>
      <Field label="分组" hint="可选择已有分组，也可直接填写新分组。"><input aria-label="分组" list="group-options" disabled={busy} value={environment.group} placeholder="未分组" onChange={event => onChange({ group: event.target.value })} /><datalist id="group-options">{groups.map(group => <option key={group} value={group} />)}</datalist></Field>
    </div>
    <Field label="浏览器内核" hint={kernelLocked ? "已保存的内核固定不变；更换版本请到内核管理进行迁移。" : native ? "仅列出本机已安装、已核验的 fingerprint-chromium。" : "fingerprint-chromium 演示选项，网页不会安装或启动内核。"}>
      <select aria-label="浏览器内核" disabled={frozen || generating || kernelLocked || !kernels.length} value={environment.coreId} onChange={event => onChange({ coreId: event.target.value })}>
        {!kernel && <option value={environment.coreId}>{environment.coreId === "kernel-pending" ? "尚未安装可用内核" : "已保存内核当前不可用"}</option>}
        {kernels.map(item => <option key={item.id} value={item.id}>fingerprint-chromium {item.version}{native ? " · 已核验" : " · 演示"}</option>)}
      </select>
    </Field>
    {!canGenerate && <div className="form-note kernel-guidance" role="status"><ShieldCheck size={17} /><div><strong>{kind === "create" ? "没有可用的内核，暂时不能创建并打开。" : "已保存的内核当前不可用，打开前需要核验。"}</strong><p>先安装并核验 fingerprint-chromium，再返回环境配置。当前填写内容会保留。</p><button type="button" className="text-button" disabled={busy} onClick={onKernels}>打开内核管理</button></div></div>}
    <div className="form-section-title"><h3>网络连接</h3><button type="button" className="text-button" disabled={frozen} onClick={onImportProxy}><Plus size={14} />导入代理</button></div>
    <Field label="绑定代理"><select aria-label="绑定代理" disabled={frozen} value={environment.proxyId} onChange={event => onChange({ proxyId: event.target.value })}>
      <option value="">本机网络（直连）</option>
      {environment.proxyId && !proxies.some(proxy => proxy.id === environment.proxyId) && <option value={environment.proxyId}>已绑定代理不可用（不会改为直连）</option>}
      {proxies.map(proxy => <option key={proxy.id} value={proxy.id}>{proxy.name} · {proxy.type.toUpperCase()}{proxy.status === "failed" ? " · 检查失败" : proxy.status === "unchecked" ? " · 待检查" : ""}</option>)}
    </select></Field>
    <p className="connection-policy">{environment.proxyId ? "代理失败会阻止打开，不自动改为直连。修改代理不更换指纹。" : "已明确选择本机直连，网站会看到本机网络出口。"}</p>
    <section className="draft-fingerprint" aria-label="指纹摘要">
      <div className="draft-fingerprint-heading"><Fingerprint size={22} /><div><h3>固定指纹</h3><p>{generating ? "正在自动准备预览…" : fresh ? kind === "edit" ? "沿用已保存身份；普通编辑与重开不会换指纹。" : "已自动准备，创建时保存并持续沿用。" : "等待可用内核或重试预览。"}</p></div><button type="button" className="button soft-primary" disabled={frozen || generating || !canGenerate} onClick={() => onGenerate(true)}>{generating ? <LoaderCircle size={15} className="spin" /> : <Sparkles size={15} />}换一套</button></div>
      <div className="fingerprint-summary"><span>Windows 桌面</span><span>{environment.language}</span><span>{environment.timezone}</span><span>{environment.cpu === "auto" ? "CPU 由内核生成" : `${environment.cpu} 线程偏好`}</span></div>
      <p className="field-hint">换一套只修改当前草稿，保存才生效。GPU、字体及绘图等由内核生成，预览不是实测。</p>
      {previewError && <div className="form-error" role="alert"><span>{previewError}</span><button type="button" className="text-button" disabled={frozen || generating || !canGenerate} onClick={() => onGenerate(false)}><RefreshCw size={14} />重试指纹预览</button></div>}
    </section>
    {profileBusy && <div className="form-note" role="status"><ShieldCheck size={17} /><span>环境仍在运行或等待资源释放，只能修改名称、分组和备注。停止并确认空闲后，才能修改指纹、代理及浏览偏好。</span></div>}
    <details className="environment-advanced"><summary>高级设置</summary><div className="advanced-body">
      <Field label="备注"><textarea aria-label="备注" disabled={busy} rows={2} value={environment.note} onChange={event => onChange({ note: event.target.value })} placeholder="环境用途或操作提醒" /></Field>
      <h3>设备与地区</h3>
      <Field label="固定指纹种子" hint="仅显式换一套会改变草稿身份；普通编辑、代理变化和重开不改变种子。"><input aria-label="固定指纹种子" readOnly value={environment.seed} /></Field>
      <div className="field-row">
        <Field label="网站语言"><select aria-label="网站语言" disabled={busy || generating || !canConfigure("acceptLanguages")} value={environment.language} onChange={event => onChange({ language: event.target.value })}>{!Object.values(regions).some(region => region.language === environment.language) && <option>{environment.language}</option>}{Object.values(regions).map(region => <option key={region.language}>{region.language}</option>)}</select></Field>
        <Field label="时区"><select aria-label="设备时区" disabled={busy || generating || !canConfigure("timezone")} value={environment.timezone} onChange={event => onChange({ timezone: event.target.value })}>{!Object.values(regions).some(region => region.timezone === environment.timezone) && <option>{environment.timezone}</option>}{Object.values(regions).map(region => <option key={region.timezone}>{region.timezone}</option>)}</select></Field>
      </div>
      <button type="button" className="button compact" disabled={busy || generating || !canConfigure("acceptLanguages") || !canConfigure("timezone") || !regions[proxies.find(proxy => proxy.id === environment.proxyId)?.country ?? ""]} onClick={() => { const region = regions[proxies.find(proxy => proxy.id === environment.proxyId)?.country ?? ""]; if (region) onChange({ language: region.language, timezone: region.timezone }); }}>采用代理地区建议（非IP实测）</button>
      <Field label="CPU 线程偏好"><select aria-label="CPU线程偏好" disabled={busy || generating || !canConfigure("cpu")} value={environment.cpu} onChange={event => onChange({ cpu: event.target.value })}><option value="auto">由内核按种子生成</option>{["4", "8", "12", "16"].map(value => <option key={value}>{value}</option>)}</select></Field>
      <h3>浏览器偏好</h3>
      <Field label="启动网址" hint="每行一个完整 http / https 网址；可留空。"><textarea aria-label="启动网址" rows={3} disabled={frozen} value={environment.urls} onChange={event => onChange({ urls: event.target.value })} placeholder="https://example.com" /></Field>
      <div className="field-row"><Field label="窗口宽度"><input aria-label="窗口宽度" type="number" min={400} max={7680} disabled={frozen || generating} value={environment.width} onChange={event => onChange({ width: Number(event.target.value) })} /></Field><Field label="窗口高度"><input aria-label="窗口高度" type="number" min={400} max={7680} disabled={frozen || generating} value={environment.height} onChange={event => onChange({ height: Number(event.target.value) })} /></Field></div>
      <p className="field-hint">窗口大小是使用偏好，不是屏幕指纹。关闭后保留浏览数据。</p>
      <label className="toggle-row"><span>恢复上次标签页</span><input aria-label="恢复上次标签页" type="checkbox" disabled={frozen} checked={environment.restoreTabs} onChange={event => onChange({ restoreTabs: event.target.checked })} /></label>
      {kind === "create" && <Field label="创建数量" hint="每个环境拥有新身份与独立数据；数量大于 1 时使用批量流程。"><input aria-label="创建数量" type="number" min={1} step={1} disabled={busy} value={quantity} onChange={event => onQuantity(Number(event.target.value))} /></Field>}
      <details className="fingerprint-technical"><summary>技术详情与档案历史</summary><FingerprintRevisionPanel preview={preview} history={history} native={native} stale={!fresh && canGenerate} busy={busy || generating || profileBusy} canGenerate={canGenerate} dataRef={dataRef} onPreview={() => onGenerate(false)} onRestore={onRestore} /></details>
    </div></details>
  </div>;
}
