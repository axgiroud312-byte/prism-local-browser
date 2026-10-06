import { useId, useLayoutEffect, useRef, type ReactNode } from "react";
import { createPortal } from "react-dom";
import { ReferenceModalFrame } from "./ReferenceUi";
import { lockBodyScroll, lockModalBackground, ownsTopModal, restoreModalFocus } from "./modal-lifecycle";
import "./proxy-kernel35.css";

const layers: HTMLElement[] = [];
const tabbables = (element: HTMLElement) => [...element.querySelectorAll<HTMLElement>("button:not(:disabled), input:not(:disabled), select:not(:disabled), textarea:not(:disabled), a[href], summary, [tabindex='0']")].filter(item => !item.closest("[inert], [hidden]") && item.getClientRects().length > 0);

/** Ticket-scoped lifecycle. Parent-managed mode is available to the App layer. */
export function ProxyKernelModal({ title, onClose, children, footer, width = 620, height, busy = false, className = "", lifecycle = "self", variant = "dialog" }: {
  title: string; onClose: () => void; children: ReactNode; footer?: ReactNode; width?: number; height?: number;
  busy?: boolean; className?: string; lifecycle?: "self" | "parent"; variant?: "dialog" | "drawer";
}) {
  const id = useId(), layer = useRef<HTMLDivElement>(null), close = useRef(onClose), blocked = useRef(busy);
  close.current = onClose; blocked.current = busy;
  useLayoutEffect(() => {
    if (lifecycle === "parent" || !layer.current) return;
    const element = layer.current;
    const trigger = document.activeElement instanceof HTMLElement ? document.activeElement : null;
    // Never inert the root: App's higher-priority workspace blocker lives in it.
    // Only background shell regions and existing lower overlays are suspended.
    const shell = document.querySelector(".app-shell");
    const siblings = [
      ...(shell ? shell.querySelectorAll<HTMLElement>(":scope > .sidebar, :scope > .main-shell, :scope > .overlay:not(.workspace-blocker)") : []),
      ...[...document.body.children].filter((item): item is HTMLElement => item instanceof HTMLElement && item !== element && !item.contains(shell) && !["SCRIPT", "STYLE"].includes(item.tagName) && !item.matches(".workspace-blocker") && !item.querySelector("[role='alertdialog']")),
    ];
    const releaseBackground = lockModalBackground(siblings);
    const releaseScroll = lockBodyScroll();
    layers.push(element);
    if (ownsTopModal(element)) (tabbables(element).find(item => item.matches("textarea,input,select")) ?? tabbables(element)[0] ?? element).focus();
    const key = (event: KeyboardEvent) => {
      if (event.defaultPrevented || layers.at(-1) !== element || element.inert) return;
      // Higher workspace/restore windows own their own keyboard lifecycle.
      if (!ownsTopModal(element)) return;
      if (event.key === "Escape") { event.preventDefault(); event.stopImmediatePropagation(); if (!blocked.current) close.current(); }
      if ((event.ctrlKey || event.metaKey) && event.key.toLowerCase() === "k") { event.preventDefault(); event.stopImmediatePropagation(); }
      if (event.key === "Tab") {
        event.preventDefault(); event.stopImmediatePropagation();
        const items = tabbables(element), index = items.indexOf(document.activeElement as HTMLElement);
        if (!items.length) { element.focus(); return; }
        items[index < 0 ? (event.shiftKey ? items.length - 1 : 0) : (index + (event.shiftKey ? -1 : 1) + items.length) % items.length].focus();
      }
    };
    document.addEventListener("keydown", key, true);
    return () => {
      document.removeEventListener("keydown", key, true);
      const index = layers.indexOf(element); if (index >= 0) layers.splice(index, 1);
      releaseBackground();
      releaseScroll();
      // Layout cleanup precedes removal of this portal; restore only after that
      // commit, still respecting a new or remaining higher-priority owner.
      queueMicrotask(() => restoreModalFocus(trigger));
    };
  }, [lifecycle]);
  const frame = <ReferenceModalFrame title={title} titleId={id} width={width} height={height} onClose={onClose} busy={busy} footer={footer} variant={variant} className={`pk35-modal ${className}`}>{children}</ReferenceModalFrame>;
  if (lifecycle === "parent") return frame;
  return createPortal(<div className="pk35-overlay" ref={layer} tabIndex={-1} onMouseDown={event => { if (event.target === event.currentTarget && !busy) onClose(); }}>{frame}</div>, document.body);
}
