import { useEffect, useId, useRef, useState, type ReactNode } from "react";
import { createPortal } from "react-dom";
import { Fingerprint, LoaderCircle, Monitor, Network, Plus, RefreshCw, Settings2, ShieldCheck, Sparkles } from "lucide-react";
import { regions, type Environment, type Kernel, type ProxyNode } from "../domain";
import type { FingerprintPreview, ProfileRevision } from "../application/contract";
import { FingerprintRevisionPanel } from "./FingerprintRevisionPanel";
import { EnvironmentKernelSelect } from "./EnvironmentKernelSelect";
import "./environment-windows.css";

function Field({ label, children, hint }: { label: string; children: ReactNode; hint?: string }) {
  return <label className="env34-field"><span>{label}</span><div>{children}{hint && <small>{hint}</small>}</div></label>;
}

export interface EnvironmentFormProps {
  environment: Environment; kind: "create" | "edit"; groups: string[]; kernels: Kernel[]; proxies: ProxyNode[];
  native: boolean; busy: boolean; generating: boolean; profileBusy: boolean; kernelLocked: boolean;
  canGenerate: boolean; fresh: boolean; preview?: FingerprintPreview; history?: ProfileRevision[]; dataRef?: string;
  quantity: number; previewError: string; onChange: (patch: Partial<Environment>) => void; onQuantity: (quantity: number) => void;
  onGenerate: (regenerate?: boolean) => void; onRestore: (revision: number) => void; onImportProxy: () => void; onKernels: () => void;
  canConfigure: (field: string) => boolean; showGenerateAction?: boolean;
}

const sections = [
  { id: "basic", label: "基础设置", icon: Monitor },
  { id: "proxy", label: "代理设置", icon: Network },
  { id: "preferences", label: "常用设置", icon: Settings2 },
  { id: "fingerprint", label: "指纹设置", icon: Fingerprint },
] as const;

export function EnvironmentForm({ environment, kind, groups, kernels, proxies, native, busy, generating, profileBusy, kernelLocked, canGenerate, fresh, preview, history, dataRef, quantity, previewError, onChange, onQuantity, onGenerate, onRestore, onImportProxy, onKernels, canConfigure, showGenerateAction = true }: EnvironmentFormProps) {
  const form = useRef<HTMLDivElement>(null);
  const groupId = useId();
  const [railHost, setRailHost] = useState<HTMLElement | null>(null);
  const [activeSection, setActiveSection] = useState<string>("basic");
  useEffect(() => {
    const host = form.current?.closest<HTMLElement>('[role="dialog"]') ?? null;
    setRailHost(host);
    const scroller = form.current?.closest<HTMLElement>(".environment-dialog-body, .drawer-body");
    if (!scroller) return;
    const track = () => {
      const top = scroller.getBoundingClientRect().top;
      const visible = [...(form.current?.querySelectorAll<HTMLElement>("[data-environment-section]") ?? [])];
      setActiveSection(visible.filter(section => section.getBoundingClientRect().top <= top + 50).at(-1)?.dataset.environmentSection ?? "basic");
    };
    scroller.addEventListener("scroll", track, { passive: true });
    return () => scroller.removeEventListener("scroll", track);
  }, []);
  function jump(id: string) {
    const section = form.current?.querySelector<HTMLElement>(`[data-environment-section="${id}"]`);
    const scroller = form.current?.closest<HTMLElement>(".environment-dialog-body, .drawer-body");
    if (!section || !scroller) return;
    scroller.scrollTo({ top: section.getBoundingClientRect().top - scroller.getBoundingClientRect().top + scroller.scrollTop - 20 });
    setActiveSection(id);
  }
  const frozen = busy || profileBusy;
  const region = regions[proxies.find(proxy => proxy.id === environment.proxyId)?.country ?? ""];
  return <div className="environment-form environment-form-reference" ref={form}>
    {railHost && createPortal(<nav className="env34-rail" aria-label="环境配置分区">{sections.map(({ id, label, icon: Icon }) => <button key={id} type="button" aria-label={label} title={label} aria-current={activeSection === id ? "location" : undefined} onClick={() => jump(id)}><Icon size={16} /></button>)}</nav>, railHost)}
    <section className="env34-section" data-environment-section="basic" aria-label="基础设置">
      <h3>基础设置</h3>
      {kind === "create" && <Field label="创建数量" hint="正整数；每个环境独立身份和数据，没有实例数量配额。"><input aria-label="创建数量" type="number" min={1} step={1} disabled={busy} value={quantity} onChange={event => onQuantity(Number(event.target.value))} /></Field>}
       <Field label="环境名称 *"><input aria-label="环境名称" autoComplete="off" disabled={busy} value={environment.name} placeholder="例如：北美主店" onChange={event => onChange({ name: event.target.value })} /></Field>
      <Field label="分组" hint="选择已有分组，或直接填写新分组。"><input aria-label="分组" list={groupId} disabled={busy} value={environment.group} placeholder="未分组" onChange={event => onChange({ group: event.target.value })} /><datalist id={groupId}>{groups.map(group => <option key={group} value={group} />)}</datalist></Field>
      <Field label="备注"><textarea aria-label="备注" disabled={busy} rows={1} value={environment.note} onChange={event => onChange({ note: event.target.value })} placeholder="环境用途或操作提醒" /></Field>
      {profileBusy && <p className="env34-notice" role="status"><ShieldCheck size={15} />环境仍在运行或等待资源释放，只能修改名称、分组和备注。关键字段需停止并确认空闲。</p>}
    </section>
    <section className="env34-section" data-environment-section="proxy" aria-label="代理设置">
      <h3>代理设置</h3>
      <Field label="绑定代理"><select aria-label="绑定代理" disabled={frozen} value={environment.proxyId} onChange={event => onChange({ proxyId: event.target.value })}>
        <option value="">本机网络（直连）</option>
        {environment.proxyId && !proxies.some(proxy => proxy.id === environment.proxyId) && <option value={environment.proxyId}>已绑定代理不可用（不会改为直连）</option>}
        {proxies.map(proxy => <option key={proxy.id} value={proxy.id}>{proxy.name} · {proxy.type.toUpperCase()}{proxy.status === "failed" ? " · 检查失败" : proxy.status === "unchecked" ? " · 待检查" : ""}</option>)}
      </select></Field>
      <div className="env34-field"><span>代理管理</span><div><button type="button" className="text-button" disabled={frozen} onClick={onImportProxy}><Plus size={14} />导入代理</button><small>返回时保留本张表单。代理导入和环境保存是独立操作。</small></div></div>
      <p className="env34-policy">{environment.proxyId ? "代理失败会阻止打开，不自动改为直连。修改代理不更换指纹。" : "已明确选择本机直连，网站会看到本机网络出口。"}</p>
    </section>
    <section className="env34-section" data-environment-section="preferences" aria-label="常用设置">
      <h3>常用设置</h3>
      <Field label="启动网址" hint="每行一个完整 http / https 网址；可留空。"><textarea aria-label="启动网址" rows={3} disabled={frozen} value={environment.urls} onChange={event => onChange({ urls: event.target.value })} placeholder="https://example.com" /></Field>
      <div className="env34-field"><span>窗口大小</span><div className="env34-window-size"><label>宽度<input aria-label="窗口宽度" type="number" min={400} max={7680} disabled={frozen || generating} value={environment.width} onChange={event => onChange({ width: Number(event.target.value) })} /></label><span>×</span><label>高度<input aria-label="窗口高度" type="number" min={400} max={7680} disabled={frozen || generating} value={environment.height} onChange={event => onChange({ height: Number(event.target.value) })} /></label><small>窗口偏好，不是屏幕指纹。关闭后保留浏览数据。</small></div></div>
      <Field label="恢复标签页"><span className="env34-switch-line"><input className="env34-switch" aria-label="恢复上次标签页" type="checkbox" disabled={frozen} checked={environment.restoreTabs} onChange={event => onChange({ restoreTabs: event.target.checked })} />恢复上次标签页</span></Field>
    </section>
    <section className="env34-section" data-environment-section="fingerprint" aria-label="指纹设置">
      <h3>指纹设置</h3>
      <Field label="浏览器内核" hint={kernelLocked ? `已保存的精确内核固定；更换版本请到内核管理迁移。${native ? "内核系列：fingerprint-chromium。" : ""}` : native ? "本机已安装、已核验的 fingerprint-chromium。" : "演示选项，网页不会安装或启动内核。"}>
      <EnvironmentKernelSelect kernels={kernels} value={environment.coreId} disabled={frozen || generating || kernelLocked || !kernels.length} native={native} onChange={id => onChange({ coreId: id })} />
    </Field>
      {!canGenerate && <div className="env34-notice" role="status"><ShieldCheck size={17} /><div><strong>{kind === "create" ? "没有可用的内核，暂时不能创建并打开。" : "已保存的内核当前不可用，打开前需要核验。"}</strong><p>先安装并核验 fingerprint-chromium，再返回环境配置。当前填写内容会保留。</p><button type="button" className="text-button" disabled={busy} onClick={onKernels}>打开内核管理</button></div></div>}
      <div className="env34-field" aria-label="指纹摘要"><span>固定指纹</span><div><div className="env34-fingerprint-state"><span>{generating ? "正在自动准备预览…" : fresh ? kind === "edit" ? "沿用已保存身份" : "已自动准备" : "等待可用内核或重试预览"}</span>{showGenerateAction && <button type="button" className="text-button" disabled={frozen || generating || !canGenerate} onClick={() => onGenerate(true)}>{generating ? <LoaderCircle size={15} className="spin" /> : <Sparkles size={15} />}换一套</button>}</div><small>Windows 桌面；换一套只改草稿，保存才生效。普通编辑和重开不换身份。</small></div></div>
      {previewError && <div className="env34-error" role="alert"><span>{previewError}</span><button type="button" className="text-button" disabled={frozen || generating || !canGenerate} onClick={() => onGenerate(false)}><RefreshCw size={14} />重试指纹预览</button></div>}
      <Field label="固定指纹种子"><input aria-label="固定指纹种子" readOnly value={environment.seed} /></Field>
      <Field label="网站语言"><select aria-label="网站语言" disabled={frozen || generating || !canConfigure("acceptLanguages")} value={environment.language} onChange={event => onChange({ language: event.target.value })}>{!Object.values(regions).some(item => item.language === environment.language) && <option>{environment.language}</option>}{Object.values(regions).map(item => <option key={item.language}>{item.language}</option>)}</select></Field>
      <Field label="时区"><select aria-label="设备时区" disabled={frozen || generating || !canConfigure("timezone")} value={environment.timezone} onChange={event => onChange({ timezone: event.target.value })}>{!Object.values(regions).some(item => item.timezone === environment.timezone) && <option>{environment.timezone}</option>}{Object.values(regions).map(item => <option key={item.timezone}>{item.timezone}</option>)}</select></Field>
      <div className="env34-field"><span>地区建议</span><div><button type="button" className="text-button" disabled={frozen || generating || !canConfigure("acceptLanguages") || !canConfigure("timezone") || !region} onClick={() => { if (region) onChange({ language: region.language, timezone: region.timezone }); }}>采用代理地区建议（非IP实测）</button></div></div>
      <Field label="CPU线程偏好"><select aria-label="CPU线程偏好" disabled={frozen || generating || !canConfigure("cpu")} value={environment.cpu} onChange={event => onChange({ cpu: event.target.value })}><option value="auto">由内核按种子生成</option>{["4", "8", "12", "16"].map(value => <option key={value}>{value}</option>)}</select></Field>
      <p className="env34-policy">GPU、内存、字体及绘图由固定内核生成，不提供假编辑项。预览不是本环境实测。</p>
      <details className="env34-technical"><summary>高级设置</summary><FingerprintRevisionPanel preview={preview} history={history} native={native} stale={!fresh && canGenerate} busy={frozen || generating} canGenerate={canGenerate} dataRef={dataRef} onPreview={() => onGenerate(false)} onRestore={onRestore} /></details>
    </section>
  </div>;
}
