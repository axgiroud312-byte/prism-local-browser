import { lazy, Suspense, useEffect, useRef } from "react";
import remarkGfm from "remark-gfm";
import { ReferenceButton } from "./ReferenceUi";
import "./local-pages.css";

const Markdown = lazy(() => import("react-markdown"));
export type HelpDocTab = "user" | "prd" | "development" | "kernel";
const documents = [{ id: "user", title: "本机使用指南", filename: "USER_GUIDE.md" }, { id: "prd", title: "产品需求 PRD", filename: "PRD.md" }, { id: "development", title: "开发与验收", filename: "DEVELOPMENT.md" }, { id: "kernel", title: "内核适配合同", filename: "KERNEL.md" }] as const;
export type HelpRoute = "environments" | "proxies" | "kernels" | "backups" | "activity" | "guide";

export function HelpPage({ native, docTab, text, onDocTab, onNavigate, download, docHref }: {
  native: boolean; docTab: HelpDocTab; text: string; onDocTab(tab: HelpDocTab): void; onNavigate(route: HelpRoute): void;
  download(name: string, content: string, type?: string): void; docHref(href?: string): string;
}) {
  const body = useRef<HTMLDivElement>(null);
  useEffect(() => { body.current?.scrollTo(0, 0); }, [docTab]);
  const selected = documents.find(doc => doc.id === docTab)!;
  return <section className="local-page local-page-help" aria-label="使用说明">
    <div className="local-page-tabs" role="tablist" aria-label="嵌入文档">{documents.map(doc => <button key={doc.id} role="tab" aria-selected={docTab === doc.id} className={docTab === doc.id ? "selected" : ""} onClick={() => onDocTab(doc.id)}>{doc.title}</button>)}</div>
    <div className="local-page-toolbar"><span>{native ? "本机使用指南与排错" : "演示模式 · 产品需求与操作说明"}</span><span className="subtle-text">源码 UI 预览，不代表新候选 exe 或真实桌面验收</span><ReferenceButton className="local-page-tail" onClick={() => download(selected.filename, text, "text/markdown;charset=utf-8")}>下载文档</ReferenceButton></div>
    <div className="local-page-help-frame"><aside className="local-page-help-links" aria-label="帮助导航"><h2>功能入口</h2>{([{ route: "environments", title: "环境与固定指纹", ids: "ENV-001 / ENV-002 / FP-001" }, { route: "proxies", title: "代理与故障阻断", ids: "PRX-001 / ENV-003" }, { route: "kernels", title: "内核能力与版本", ids: "CORE-001 / FP-002" }, { route: "backups", title: "备份与恢复", ids: "BKP-001 / DATA-001" }, { route: "activity", title: "记录与脱敏诊断", ids: "UX-001 / DOC-001" }] as const).map(item => <button key={item.route} onClick={() => onNavigate(item.route)}>{item.title}<small>{item.ids}</small></button>)}<h2>文档与依据</h2><a href={docHref("../README.md")} target="_blank" rel="noreferrer">项目首页</a>{documents.map(doc => <button key={doc.id} onClick={() => onDocTab(doc.id)}>{doc.title}</button>)}<p className="local-page-note">原帮助页无完整直接参考；使用冻结 shell、标题与正文滚动容器映射。</p></aside>
      <div className="document-body" ref={body} role="tabpanel" aria-label={selected.title} tabIndex={0}><Suspense fallback={<p>正在载入文档…</p>}><Markdown remarkPlugins={[remarkGfm]} components={{ a: ({ href, children }) => <a href={docHref(href)} target={docHref(href).startsWith("https://") ? "_blank" : undefined} rel="noreferrer" onClick={event => {
        const doc = documents.find(item => href?.includes(item.filename));
        if (doc) { event.preventDefault(); onDocTab(doc.id); body.current?.scrollTo(0, 0); }
      }}>{children}</a> }}>{text}</Markdown></Suspense></div>
    </div>
  </section>;
}
