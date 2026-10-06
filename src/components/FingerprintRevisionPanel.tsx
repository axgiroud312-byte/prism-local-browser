import type { FingerprintPreview, ProfileRevision } from "../application/contract";
import "./fingerprint-revision.css";

const fieldLabels: Record<string, string> = { seed: "固定种子", kernelId: "精确内核", coreActualVersion: "实际内核版本", generatorVersion: "生成器版本", identity: "品牌与版本组", cpu: "CPU偏好", acceptLanguages: "网页与请求语言", language: "网站语言", timezone: "时区", window: "窗口偏好", width: "窗口宽度", height: "窗口高度", uiLanguage: "浏览器菜单语言", screen: "屏幕与缩放", memory: "内存", gpu: "GPU / WebGL", "font/canvas/audio/clientrects": "字体 / Canvas / 音频 / ClientRects", "screen/location/webgpu/tls/mac": "定位 / 其他未验收项", "proxy/webrtc": "代理 / WebRTC" };
const statusLabels = { configurable: "直接可配置", "seed-generated": "内核按 seed 生成", system: "跟随真实环境", unverified: "未验证 · 不开放编辑" };

export function FingerprintRevisionPanel({ preview, history, native, stale, busy, canGenerate, dataRef, onPreview, onRestore }: {
  preview?: FingerprintPreview; history?: ProfileRevision[]; native: boolean; stale: boolean;
  busy: boolean; canGenerate: boolean; dataRef?: string;
  onPreview: () => void; onRestore: (revision: number) => void;
}) {
  const profile = preview?.previewProfile;
  return <section className="fingerprint-revision-panel" aria-label="设备档案预览与历史">
    <div className="fingerprint-preview-heading"><h3>技术详情与档案历史</h3><button type="button" className="text-button" disabled={busy || !canGenerate} onClick={onPreview}>生成并查看预览</button></div>
    {!canGenerate && <p className="field-hint">当前构建不可用，不能启动。先选择已安装、已核验的精确内核；旧种子不会自动改变。</p>}
    {stale && <p className="fingerprint-stale" role="status">设备字段已改变，当前预览已过时。请重新生成预览后保存；不会自动更换种子。</p>}
    {profile && <>
      <p className="field-hint">{native ? "原生档案输入；能力引用所选构建的安装证据，不是当前环境实测。" : "演示档案与演示历史；没有真实内核或浏览器读值。"}生成与回滚都只改草稿，取消保留当前档案。</p>
      <dl className="fingerprint-profile-facts">
        <dt>档案修订</dt><dd>#{profile.configRevision}{preview?.action === "restore" ? ` · 从 #${preview.restoredFrom} 回滚的新预览` : ""}</dd>
        <dt>精确内核</dt><dd>{profile.kernelId}</dd>
        <dt>实际版本</dt><dd>{profile.coreActualVersion || "未核验，不假填版本"}</dd>
        <dt>模板 / 生成器</dt><dd>{profile.templateVersion} / {profile.generatorVersion}</dd>
        <dt>能力版本</dt><dd>{profile.capabilityVersion || "未核验"}</dd>
        <dt>网站语言顺序</dt><dd>{profile.acceptLanguages.join(" → ")}</dd>
        <dt>窗口偏好</dt><dd>{profile.width} × {profile.height}（不是屏幕指纹）</dd>
        <dt>规范化摘要</dt><dd className="mono">{profile.configHash}</dd>
        {dataRef && <><dt>已有数据引用</dt><dd>{dataRef}（本次档案修改保持不变）</dd></>}
      </dl>
      {!!preview?.changes.length && <div className="fingerprint-change-preview"><strong>保存后的变更</strong><ul>{preview.changes.map(change => <li key={change.field}>{fieldLabels[change.field] ?? change.field}：<span>{change.before || "未核验"}</span> → <span>{change.after || "未核验"}</span></li>)}</ul><p>不会恢复旧名称或代理，也不会清除浏览数据。取消不提交这些变更。</p></div>}
      <details className="fingerprint-capabilities" open><summary>该构建的能力边界</summary>{preview?.capabilityReport.capabilities.map(capability => <div key={capability.field}><strong>{fieldLabels[capability.field] ?? capability.field}</strong><span>{statusLabels[capability.status]}</span><small>{capability.source === "source-derived" ? "源码推导，非本环境读值" : capability.source === "observed" ? "所选构建安装时实测，非本环境读值" : capability.source === "demo-only" ? "仅演示" : "应用策略或未探测"}</small><p>{capability.note}</p></div>)}</details>
      <p className="field-hint">预览不启动浏览器，不编造 GPU、内存或字体读值。页面/合成 bridge 不能证明真实桌面能力已验收。</p>
    </>}
    {!!history?.length && <details className="fingerprint-history"><summary>已保存的档案历史（{history.length}）</summary><p>回滚只使用同一精确内核的旧设备输入，保存后形成新的修订，不倒退修订号。</p>{history.map(item => <div key={item.profile.configRevision}><div><strong>#{item.profile.configRevision} · seed {item.profile.seed}</strong><small>{item.createdAt} · {item.action}</small></div><button type="button" className="button" disabled={busy || item.profile.configRevision === history[0].profile.configRevision || (native && (item.profile.kernelId !== profile?.kernelId || item.profile.kernelId === "kernel-pending"))} onClick={() => onRestore(item.profile.configRevision)}>预览回滚到 #{item.profile.configRevision}</button></div>)}</details>}
  </section>;
}
