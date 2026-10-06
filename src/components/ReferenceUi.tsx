import { useLayoutEffect, useRef, type CSSProperties, type ReactNode } from "react";
import { createPortal } from "react-dom";
import { X } from "lucide-react";
import "./reference-ui.css";

export function ReferenceButton({ className = "", ...props }: React.ButtonHTMLAttributes<HTMLButtonElement>) {
  return <button className={`button ${className}`} type="button" {...props} />;
}

/** One non-modal surface. Anchor stays in the page; the menu escapes scroll clipping. */
export function ReferencePopover({ anchor, onClose, children, className = "", width = 176, align = "start", label, role = "menu" }: {
  anchor: HTMLElement; onClose: () => void; children: ReactNode; className?: string;
  width?: number; align?: "start" | "end"; label: string; role?: "menu" | "dialog";
}) {
  const surface = useRef<HTMLDivElement>(null);
  const close = useRef(onClose); close.current = onClose;
  useLayoutEffect(() => {
    const element = surface.current;
    if (!element) return;
    const place = () => {
      const rect = anchor.getBoundingClientRect();
      const effectiveWidth = Math.min(width, innerWidth - 16);
      element.style.left = `${Math.max(8, Math.min(align === "end" ? rect.right - effectiveWidth : rect.left, innerWidth - effectiveWidth - 8))}px`;
      element.style.top = `${Math.max(8, Math.min(rect.bottom, innerHeight - element.offsetHeight - 8))}px`;
    };
    place();
    element.querySelector<HTMLElement>("input, button:not(:disabled), select")?.focus();
    const outside = (event: PointerEvent) => {
      if (!element.contains(event.target as Node) && !anchor.contains(event.target as Node)) close.current();
    };
    const key = (event: KeyboardEvent) => {
      if (event.key === "Escape") { event.preventDefault(); event.stopImmediatePropagation(); close.current(); if (anchor.isConnected) anchor.focus(); }
      if (role !== "menu" || !["ArrowDown", "ArrowUp", "Home", "End"].includes(event.key)) return;
      const buttons = [...element.querySelectorAll<HTMLButtonElement>("button:not(:disabled)")];
      const index = buttons.indexOf(document.activeElement as HTMLButtonElement);
      if (!buttons.length) return;
      event.preventDefault();
      buttons[event.key === "Home" ? 0 : event.key === "End" ? buttons.length - 1 : (index + (event.key === "ArrowUp" ? -1 : 1) + buttons.length) % buttons.length]?.focus();
    };
    const scroll = (event: Event) => { if (!element.contains(event.target as Node)) place(); };
    document.addEventListener("pointerdown", outside, true);
    document.addEventListener("keydown", key, true);
    document.addEventListener("scroll", scroll, true); window.addEventListener("resize", place);
    return () => {
      document.removeEventListener("pointerdown", outside, true); document.removeEventListener("keydown", key, true);
      document.removeEventListener("scroll", scroll, true); window.removeEventListener("resize", place);
    };
  }, [anchor, width, align, role]);
  return createPortal(<div ref={surface} className={`reference-popover ${className}`} style={{ width } as CSSProperties} role={role} aria-label={label}>{children}</div>, document.body);
}

/** Visual frame only: caller owns Escape, focus, inert background and draft protection. */
export function ReferenceModalFrame({ title, titleId, onClose, busy, children, footer, width = 500, height, variant = "dialog", className = "" }: {
  title: string; titleId: string; onClose: () => void; busy?: boolean; children: ReactNode;
  footer?: ReactNode; width?: number; height?: number; variant?: "dialog" | "drawer"; className?: string;
}) {
  return <section className={`reference-modal reference-${variant} ${className}`} tabIndex={-1} role="dialog" aria-modal="true" aria-labelledby={titleId} style={{ width, height }}>
    <header className="reference-modal-header"><h2 id={titleId}>{title}</h2><button className="icon-button" aria-label={`关闭${title}`} disabled={busy} onClick={onClose}><X size={16} /></button></header>
    <div className="reference-modal-body">{children}</div>
    {footer && <footer className="reference-modal-footer">{footer}</footer>}
  </section>;
}
