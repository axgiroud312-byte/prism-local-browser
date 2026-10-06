import type { ReactNode, Ref } from "react";
import { Info, X } from "lucide-react";
import "./environment-windows.css";

/** No keyboard listeners or scroll locks: existing manager/App is the only lifecycle owner. */
export function EnvironmentWindowFrame({ title, titleId, dialogRef, onClose, closeLabel, closeDisabled, width, height, children, footer, className = "" }: {
  title: string; titleId: string; dialogRef?: Ref<HTMLDivElement>; onClose: () => void; closeLabel?: string; closeDisabled?: boolean;
  width: number; height?: number; children: ReactNode; footer?: ReactNode; className?: string;
}) {
  return <div className={`env34-window ${className}`} style={{ width, height }} ref={dialogRef} tabIndex={-1} role="dialog" aria-modal="true" aria-labelledby={titleId}>
    <header className="env34-window-header"><h2 id={titleId}>{title}</h2><button type="button" className="icon-button" aria-label={closeLabel ?? `关闭${title}`} disabled={closeDisabled} onClick={onClose}><X size={16} /></button></header>
    <div className="env34-window-body">{children}</div>
    {footer && <footer className="env34-window-footer">{footer}</footer>}
  </div>;
}

export function EnvironmentConfirmation({ title, children, onCancel, onConfirm, disabled, busy, confirmLabel = "确定", danger, dialogRef, titleId = "environment-confirm-title", footer }: {
  title: string; children: ReactNode; onCancel: () => void; onConfirm: () => void; disabled?: boolean; busy?: boolean;
  confirmLabel?: string; danger?: boolean; dialogRef?: Ref<HTMLDivElement>; titleId?: string; footer?: ReactNode;
}) {
  return <div className="env34-window env34-confirm" ref={dialogRef} tabIndex={-1} role="alertdialog" aria-modal="true" aria-labelledby={titleId}>
    <header><Info size={18} /><h2 id={titleId}>{title}</h2></header><div className="env34-confirm-body">{children}</div><footer className="env34-confirm-footer">{footer}<button type="button" className="button" disabled={busy} onClick={onCancel}>取消</button><button type="button" className={`button ${danger ? "danger" : "primary"}`} disabled={disabled || busy} onClick={onConfirm}>{confirmLabel}</button></footer>
  </div>;
}

export function EnvironmentTaskResult({ title, total, completed, failed, children }: { title: string; total: number; completed: number; failed: number; children?: ReactNode }) {
  return <section className="env34-task-result" aria-label={title} aria-live="polite"><h3>{title}</h3><div className="env34-task-counts"><span>共 {total} 项</span><span>完成 {completed} 项</span><span>失败 {failed} 项</span></div>{children}</section>;
}
