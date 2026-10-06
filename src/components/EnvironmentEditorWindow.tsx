import type { Ref } from "react";
import { Check, LoaderCircle, Monitor, Sparkles, X } from "lucide-react";
import { EnvironmentForm, type EnvironmentFormProps } from "./EnvironmentForm";

/** Visual composition only. App retains preview/request ownership and modal lifecycle. */
export function EnvironmentEditorWindow({ form, error, saving, canSave, recoveringCreation, onSave, onClose, dialogRef, title }: {
  form: EnvironmentFormProps; error: string; saving: boolean; canSave: boolean; recoveringCreation: boolean;
  onSave: (openAfterCreate?: boolean) => void; onClose: () => void; dialogRef?: Ref<HTMLDivElement>; title?: string;
}) {
  const locked = saving || recoveringCreation;
  return <div className="environment-dialog environment-drawer" ref={dialogRef} tabIndex={-1} role="dialog" aria-modal="true" aria-labelledby="drawer-title">
    <header className="drawer-header"><h2 id="drawer-title">{title ?? (form.kind === "create" ? "新建浏览器环境" : "编辑浏览器环境")}</h2><button type="button" className="icon-button" aria-label="关闭环境配置" disabled={locked} onClick={onClose}><X size={16} /></button></header>
    <div className="environment-dialog-body drawer-body"><EnvironmentForm {...form} showGenerateAction={false} />{error && <p className="env34-error" role="alert">{error}</p>}</div>
    <footer className="environment-dialog-footer drawer-footer">
      <div className="env34-editor-status"><button type="button" className="button primary" disabled={locked || form.busy || form.profileBusy || form.generating || !form.canGenerate} onClick={() => form.onGenerate(true)} title="只更换未保存的指纹草稿，保存才生效">{form.generating ? <LoaderCircle size={14} className="spin" /> : <Sparkles size={14} />}换一套</button><span title={recoveringCreation ? "保留原创建请求，重试只核实结果" : form.native ? "保存到本机，关闭后身份不变" : "演示模式，不启动真实浏览器"}>{recoveringCreation ? "原请求待核实" : form.generating ? "准备指纹中…" : !canSave ? "缺少可用内核" : form.native ? "本机保存" : "仅演示"}</span></div>
      <div><button type="button" className="button" disabled={locked} onClick={onClose}>取消</button><button type="button" className={`button ${form.kind === "edit" || form.quantity > 1 || recoveringCreation ? "primary" : ""}`} disabled={saving || !recoveringCreation && (form.generating || !canSave)} onClick={() => onSave()}><Check size={14} />{recoveringCreation ? "重试核实创建结果" : form.kind === "edit" ? "保存" : form.quantity > 1 ? form.native ? `查看 ${form.quantity} 项创建计划` : `创建 ${form.quantity} 个环境` : "创建"}</button>{form.kind === "create" && !recoveringCreation && form.quantity === 1 && <button type="button" className="button primary" disabled={form.generating || saving || !canSave} onClick={() => onSave(true)}><Monitor size={14} />创建并打开</button>}</div>
    </footer>
  </div>;
}
