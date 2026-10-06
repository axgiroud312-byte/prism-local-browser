import { useEffect, useRef, useState } from "react";
import type { Kernel } from "../domain";

/** Native-compatible select API plus a DOM popup whose rows only come from the usable service records. */
export function EnvironmentKernelSelect({ kernels, value, disabled, native, onChange }: { kernels: Kernel[]; value: string; disabled: boolean; native: boolean; onChange: (id: string) => void }) {
  const [open, setOpen] = useState(false);
  const container = useRef<HTMLDivElement>(null);
  const select = useRef<HTMLSelectElement>(null);
  const found = kernels.some(kernel => kernel.id === value);
  useEffect(() => {
    if (!open) return;
    const outside = (event: PointerEvent) => { if (!container.current?.contains(event.target as Node)) setOpen(false); };
    document.addEventListener("pointerdown", outside);
    container.current?.querySelector<HTMLElement>('[aria-selected="true"], [role="option"]')?.focus();
    return () => document.removeEventListener("pointerdown", outside);
  }, [open]);
  useEffect(() => { if (disabled) setOpen(false); }, [disabled]);
  return <div className="env34-kernel-picker" ref={container}>
    <select ref={select} aria-label="浏览器内核" aria-expanded={open} disabled={disabled} value={value} onChange={event => { onChange(event.target.value); setOpen(false); }} onMouseDown={event => { if (!disabled) { event.preventDefault(); setOpen(previous => !previous); } }} onKeyDown={event => { if (["ArrowDown", "Enter", " "].includes(event.key) && !disabled) { event.preventDefault(); setOpen(true); } }}>
      {!found && <option value={value}>{value === "kernel-pending" ? "尚未安装可用内核" : "已保存内核当前不可用"}</option>}
      {kernels.map(kernel => <option key={kernel.id} value={kernel.id}>fingerprint-chromium {kernel.version}{native ? " · 已核验" : " · 演示"}</option>)}
    </select>
    {open && <div className="env34-kernel-menu" role="listbox" aria-label="可用服务内核" onKeyDown={event => {
      if (event.key === "Escape") { event.preventDefault(); event.stopPropagation(); setOpen(false); select.current?.focus(); }
      if (event.key === "Tab") {
        event.preventDefault(); event.stopPropagation(); setOpen(false);
        const dialog = container.current?.closest('[role="dialog"]');
        const fields = [...(dialog?.querySelectorAll<HTMLElement>('button:not(:disabled), input:not(:disabled), textarea:not(:disabled), select:not(:disabled), summary, a[href]') ?? [])].filter(element => element.offsetParent !== null && !element.closest(".env34-kernel-menu"));
        const index = fields.indexOf(select.current!);
        fields[(index + (event.shiftKey ? -1 : 1) + fields.length) % fields.length]?.focus();
      }
      if (["ArrowDown", "ArrowUp", "Home", "End"].includes(event.key)) { event.preventDefault(); const rows = [...(container.current?.querySelectorAll<HTMLElement>('[role="option"]') ?? [])]; const index = rows.indexOf(document.activeElement as HTMLElement); rows[event.key === "Home" ? 0 : event.key === "End" ? rows.length - 1 : (index + (event.key === "ArrowUp" ? -1 : 1) + rows.length) % rows.length]?.focus(); }
    }}>{kernels.map(kernel => <button type="button" role="option" aria-selected={kernel.id === value} key={kernel.id} onClick={() => { onChange(kernel.id); setOpen(false); select.current?.focus(); }}>{kernel.version} · {native ? "已核验" : "演示"}</button>)}</div>}
  </div>;
}
