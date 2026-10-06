/** Shared ownership primitives; visual frames do not acquire locks or traps. */
let scrollLocks = 0;
let previousOverflow = "";
type BackgroundOwner = { count: number; previous: boolean; masks: number; previousUnderlay: boolean };
const backgrounds = new Map<HTMLElement, BackgroundOwner>();
let backgroundObserver: MutationObserver | undefined;

/** All portal owners share these counts, including out-of-order async exits. */
export function lockModalBackground(elements: HTMLElement[], transparentMasks = false): () => void {
  for (const element of elements) {
    const owner = backgrounds.get(element) ?? {
      count: 0, previous: element.inert, masks: 0,
      previousUnderlay: element.classList.contains("local-page-managed-underlay"),
    };
    owner.count++;
    if (transparentMasks && element.matches(".overlay, .pk35-overlay, .local-page-overlay")) owner.masks++;
    backgrounds.set(element, owner);
    element.inert = true;
    if (owner.masks) element.classList.add("local-page-managed-underlay");
  }
  if (!backgroundObserver) {
    backgroundObserver = new MutationObserver(() => {
      for (const element of backgrounds.keys()) if (!element.inert) element.inert = true;
    });
    backgroundObserver.observe(document.body, { subtree: true, attributes: true, attributeFilter: ["inert", "data-app-inert"] });
  }
  let released = false;
  return () => {
    if (released) return;
    released = true;
    for (const element of elements) {
      const owner = backgrounds.get(element)!;
      owner.count--;
      if (transparentMasks && element.matches(".overlay, .pk35-overlay, .local-page-overlay")) owner.masks--;
      if (!owner.masks) element.classList.toggle("local-page-managed-underlay", owner.previousUnderlay);
      if (owner.count) continue;
      backgrounds.delete(element);
      // React's desired state may change while a portal keeps a region inert.
      // Restore that live intent, not a stale true captured from a lower owner.
      const desired = element.dataset.appInert;
      element.inert = desired !== undefined ? desired === "true" : owner.previous;
    }
    if (!backgrounds.size) { backgroundObserver?.disconnect(); backgroundObserver = undefined; }
  };
}

export function lockBodyScroll(): () => void {
  if (scrollLocks++ === 0) previousOverflow = document.body.style.overflow;
  document.body.style.overflow = "hidden";
  let released = false;
  return () => {
    if (released) return;
    released = true;
    if (--scrollLocks === 0) document.body.style.overflow = previousOverflow;
  };
}

export function modalLayer(element: HTMLElement): number {
  let highest = 0;
  for (let node: HTMLElement | null = element; node; node = node.parentElement) {
    const value = Number(getComputedStyle(node).zIndex);
    if (Number.isFinite(value)) highest = Math.max(highest, value);
  }
  return highest;
}

export function topModalElement(): HTMLElement | null {
  const modals = [...document.querySelectorAll<HTMLElement>('[aria-modal="true"]')]
    .filter(element => element.getClientRects().length > 0 && !element.closest("[inert]"));
  return modals.reduce<HTMLElement | null>((top, candidate) =>
    !top || modalLayer(candidate) >= modalLayer(top) ? candidate : top, null);
}

export function ownsTopModal(element: HTMLElement | null): boolean {
  const top = topModalElement();
  return !!element && !!top && (element === top || element.contains(top));
}

export function restoreModalFocus(target: HTMLElement | null): void {
  const top = topModalElement();
  if (target?.isConnected && !target.closest("[inert]") && !target.matches(":disabled") &&
    target.getClientRects().length > 0 && (!top || top.contains(target))) target.focus();
}
