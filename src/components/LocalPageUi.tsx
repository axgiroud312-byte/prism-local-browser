import { useId, useLayoutEffect, useRef, type ReactNode } from "react";
import { createPortal } from "react-dom";
import { ChevronLeft, ChevronRight, CircleAlert } from "lucide-react";
import { ReferenceButton, ReferenceModalFrame } from "./ReferenceUi";
import { lockBodyScroll, lockModalBackground, modalLayer, ownsTopModal, restoreModalFocus } from "./modal-lifecycle";
import "./local-pages.css";

// #36 owns these windows. The frame is visual only; one capture listener per
// top window prevents App's lower Escape/Tab handlers from also firing.
type WindowOwner = { element: HTMLElement; returnFocus: HTMLElement | null };
const windows: WindowOwner[] = [];
let handoffFocus: HTMLElement | null = null;
const visibleModals = () => [...document.querySelectorAll<HTMLElement>('[aria-modal="true"]')].filter(modal => modal.getClientRects().length && !modal.closest("[inert]"));

export function LocalPageWindow({ title, children, footer, onClose, busy = false, width = 500, height, className = "" }: {
  title: string; children: ReactNode; footer: ReactNode; onClose(): void;
  busy?: boolean; width?: number; height?: number; className?: string;
}) {
  const titleId = useId(), root = useRef<HTMLDivElement>(null);
  const close = useRef(onClose), locked = useRef(busy); close.current = onClose; locked.current = busy;
  useLayoutEffect(() => {
    const element = root.current!;
    const owner: WindowOwner = { element, returnFocus: handoffFocus ?? document.activeElement as HTMLElement | null };
    handoffFocus = null;
    const releaseScroll = lockBodyScroll();
    windows.push(owner);
    // An existing fault blocker remains above an older task window; a child
    // opened from that blocker must, however, paint above it as well as focus.
    const previousLayer = visibleModals().filter(modal => !element.contains(modal)).reduce((highest, modal) => Math.max(highest, modalLayer(modal)), 120);
    element.style.zIndex = String(previousLayer + 2);
    const background = [...document.body.children].flatMap(child => {
      if (!(child instanceof HTMLElement) || child === element || ["SCRIPT", "STYLE"].includes(child.tagName)) return [];
      // Do not inert the whole React root: a new, higher-priority workspace
      // blocker must remain reachable if the host faults while this is open.
      const shell = child.querySelector<HTMLElement>(":scope > .app-shell");
      return shell ? [...shell.children].filter((region): region is HTMLElement => region instanceof HTMLElement) : [child];
    });
    const releaseBackground = lockModalBackground(background, true);
    const tabbables = () => [...element.querySelectorAll<HTMLElement>('button:not(:disabled), input:not(:disabled), select:not(:disabled), textarea:not(:disabled), a[href], summary, [tabindex="0"]')].filter(item => item.getClientRects().length && !item.closest("[inert]"));
    const focusFirst = () => (tabbables()[0] ?? element).focus();
    const isTop = () => {
      if (windows.at(-1) !== owner) return false;
      return ownsTopModal(element);
    };
    // React owns inert too, and host faults may appear after this mount. Keep
    // our regions protected and only the visually effective top mask opaque.
    const refreshBackground = () => {
      element.toggleAttribute("data-local-page-underlay", !isTop());
    };
    const backgroundChanges = new MutationObserver(refreshBackground);
    backgroundChanges.observe(document.body, { childList: true, subtree: true, attributes: true, attributeFilter: ["inert", "style", "class"] });
    refreshBackground();
    focusFirst();
    const key = (event: KeyboardEvent) => {
      if (event.defaultPrevented || !isTop()) return;
      if (event.key === "Escape") { event.preventDefault(); event.stopImmediatePropagation(); if (!locked.current) close.current(); }
      if ((event.ctrlKey || event.metaKey) && event.key.toLowerCase() === "k") { event.preventDefault(); event.stopImmediatePropagation(); }
      if (event.key !== "Tab") return;
      event.stopImmediatePropagation();
      const items = tabbables(), first = items[0], last = items.at(-1);
      if (!first) { event.preventDefault(); element.focus(); }
      else if (!element.contains(document.activeElement) || event.shiftKey && document.activeElement === first || !event.shiftKey && document.activeElement === last) { event.preventDefault(); (event.shiftKey ? last : first)?.focus(); }
    };
    const focus = (event: FocusEvent) => { if (isTop() && !element.contains(event.target as Node)) focusFirst(); };
    window.addEventListener("keydown", key, true); document.addEventListener("focusin", focus);
    return () => {
      window.removeEventListener("keydown", key, true); document.removeEventListener("focusin", focus);
      backgroundChanges.disconnect();
      windows.splice(windows.indexOf(owner), 1);
      // A flow can replace a window, or remove both a parent and its nested
      // confirmation in one commit. Keep the original connected return target.
      for (const remaining of windows) if (remaining.returnFocus && element.contains(remaining.returnFocus)) remaining.returnFocus = owner.returnFocus;
      releaseBackground();
      const top = windows.at(-1);
      releaseScroll();
      if (!top) handoffFocus = owner.returnFocus;
      // Layout cleanup precedes React's disabled/DOM updates. Wait until that
      // commit finishes, and never steal focus from a newly opened top window.
      queueMicrotask(() => {
        if (windows.at(-1) !== top) return;
        const previous = owner.returnFocus;
        if (previous?.isConnected && !previous.closest("[inert]") && !previous.matches(":disabled")) restoreModalFocus(previous);
        else top?.element.querySelector<HTMLElement>("button:not(:disabled)")?.focus();
        if (!top && handoffFocus === previous) handoffFocus = null;
      });
    };
  }, []);
  return createPortal(<div className="local-page-overlay" data-local-page-window ref={root} tabIndex={-1}>
    <ReferenceModalFrame title={title} titleId={titleId} onClose={onClose} busy={busy} width={width} height={height} className={`local-page-window ${className}`} footer={footer}>{children}</ReferenceModalFrame>
  </div>, document.body);
}

export function LocalPageTable({ headings, children, empty, className = "" }: { headings: string[]; children: ReactNode; empty?: string; className?: string }) {
  return <div className={`local-page-table-scroll ${className}`}><table className="local-page-table"><thead><tr>{headings.map(heading => <th key={heading}>{heading}</th>)}</tr></thead><tbody>{empty ? <tr><td className="local-page-empty" colSpan={headings.length}>{empty}</td></tr> : children}</tbody></table></div>;
}

export function LocalPagination({ total, page, pageSize = 10, onPage, disabled = false }: { total: number; page: number; pageSize?: number; onPage(page: number): void; disabled?: boolean }) {
  const last = Math.max(1, Math.ceil(total / pageSize));
  return <div className="local-page-pagination"><span>共 {total} 条</span><span>{pageSize}条/页</span><button className="icon-button" aria-label="上一页" disabled={disabled || page <= 1} onClick={() => onPage(page - 1)}><ChevronLeft size={14} /></button><span className="local-page-number">{page}</span><button className="icon-button" aria-label="下一页" disabled={disabled || page >= last} onClick={() => onPage(page + 1)}><ChevronRight size={14} /></button></div>;
}

export function RestoreConfirmWindow({ title = "确认完整恢复", children, onClose, onConfirm, busy, disabled, confirmText = "确认并完整恢复" }: {
  title?: string; children: ReactNode; onClose(): void; onConfirm(): void;
  busy?: boolean; disabled?: boolean; confirmText?: string;
}) {
  return <LocalPageWindow title={title} width={400} onClose={onClose} busy={busy} className="local-page-confirm" footer={<><ReferenceButton disabled={busy} onClick={onClose}>取消</ReferenceButton><ReferenceButton className="danger" disabled={busy || disabled} onClick={onConfirm}>{confirmText}</ReferenceButton></>}>
    <div className="local-page-danger-note"><CircleAlert size={18} /><div>{children}</div></div>
  </LocalPageWindow>;
}

export function LocalResultStats({ items }: { items: { label: string; value: number | string; tone?: "success" | "error" }[] }) {
  return <div className="local-page-result-stats">{items.map(item => <div key={item.label} className={item.tone ?? ""}>{item.label}：<strong>{item.value}</strong></div>)}</div>;
}

const timeFormat = new Intl.DateTimeFormat("zh-CN", { year: "numeric", month: "2-digit", day: "2-digit", hour: "2-digit", minute: "2-digit", second: "2-digit", hour12: false });
export const localTime = (value: string) => Number.isFinite(Date.parse(value)) ? timeFormat.format(new Date(value)).replaceAll("/", "-") : "时间未记录";
