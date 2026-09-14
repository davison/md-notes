import { useCallback, useEffect, useRef, useState } from "preact/hooks";
import type { ComponentChildren } from "preact";

/**
 * The navigator and the search-and-tags pane share one off-canvas drawer at
 * narrow widths, with a tab apiece; at wide widths the same element is
 * `display: contents` and the two panes are grid items of the shell, so this
 * module's state is inert there. Which of the two halves is on screen — and
 * whether the drawer's own chrome exists at all — is decided in the
 * stylesheet's narrow block, not here: hidden by `display: none` means
 * unreachable by the keyboard and by assistive technology without a media
 * query in JavaScript, and the wide layout keeps every element it had.
 */

export type DrawerTab = "notes" | "find";

/** The id the top bar's buttons name in `aria-controls`. */
export const DRAWER_ID = "drawer";

export interface DrawerState {
  open: boolean;
  tab: DrawerTab;
  /** Opens the drawer at a tab, remembering the button to hand focus back to. */
  show: (tab: DrawerTab, opener: HTMLElement | null) => void;
  /** Shows a tab without changing whether the drawer is open. */
  select: (tab: DrawerTab) => void;
  close: () => void;
}

export function useDrawer(): DrawerState {
  const [open, setOpen] = useState(false);
  const [tab, setTab] = useState<DrawerTab>("notes");
  const opener = useRef<HTMLElement | null>(null);
  const was = useRef(false);

  const show = useCallback((next: DrawerTab, from: HTMLElement | null) => {
    opener.current = from;
    setTab(next);
    setOpen(true);
  }, []);

  const close = useCallback(() => setOpen(false), []);

  // Closing hands focus back to the button that opened the drawer, so a
  // keyboard or screen-reader user is not dropped at the top of the
  // document. The child's effect, which moves focus in, runs first.
  useEffect(() => {
    if (open) {
      was.current = true;
      return;
    }
    if (!was.current) return;
    was.current = false;
    const from = opener.current;
    opener.current = null;
    from?.focus();
  }, [open]);

  return { open, tab, show, select: setTab, close };
}

const FOCUSABLE =
  'a[href], button:not([disabled]), input:not([disabled]), select:not([disabled]), textarea:not([disabled]), [tabindex]:not([tabindex="-1"])';

/**
 * The wrapper around both panes. Open, it is a modal dialog: Escape and a
 * tap on the backdrop close it, Tab cycles within it, and selecting a note
 * or a search hit closes it, since on a phone the drawer covers the note
 * that selection just opened. Selecting a tag instead moves to the notes
 * tab: the filter's whole effect is on the tree, and closing the drawer
 * would hide the result of the tap.
 */
export function Drawer({ state, children }: { state: DrawerState; children: ComponentChildren }) {
  const { open, tab, select, close } = state;
  const panes = useRef<HTMLDivElement>(null);

  useEffect(() => {
    if (!open) return;
    const onKey = (e: KeyboardEvent) => {
      if (e.key !== "Escape") return;
      e.preventDefault();
      e.stopPropagation();
      close();
    };
    // Captured, so the drawer answers Escape ahead of the editor below it.
    document.addEventListener("keydown", onKey, true);
    return () => document.removeEventListener("keydown", onKey, true);
  }, [open, close]);

  // A window dragged past the breakpoint gives the panes back to the grid,
  // and a wrapper that is `display: contents` has no dialog to be: left
  // open, it would tell a screen reader the whole shell is behind a modal
  // that is not on screen. Asking the element what the stylesheet made of
  // it keeps the breakpoint in one place — the number is not repeated here.
  useEffect(() => {
    if (!open) return;
    let frame = 0;
    const onResize = () => {
      // On the next frame, not in the handler: a resize can reach script
      // before the media query behind `display: contents` has been applied,
      // and the answer would be the old one.
      cancelAnimationFrame(frame);
      frame = requestAnimationFrame(() => {
        const root = panes.current;
        if (root && getComputedStyle(root).display === "contents") close();
      });
    };
    window.addEventListener("resize", onResize);
    return () => {
      cancelAnimationFrame(frame);
      window.removeEventListener("resize", onResize);
    };
  }, [open, close]);

  // Opening moves focus inside. The search tab leads with its box, which is
  // the reason to have opened it; the notes tab leads with the first control.
  useEffect(() => {
    if (!open) return;
    const root = panes.current;
    if (!root) return;
    const box = tab === "find" ? root.querySelector<HTMLElement>('input[type="search"]') : null;
    (box ?? root.querySelector<HTMLElement>(FOCUSABLE))?.focus();
    // Only on opening: switching tabs afterwards leaves focus on the tab button.
  }, [open]);

  const onKeyDown = (e: KeyboardEvent) => {
    if (!open || e.key !== "Tab") return;
    const root = panes.current;
    if (!root) return;
    // The tab that is off screen is `display: none`, so its controls are not
    // in the cycle; scoping by class rather than by layout keeps this honest
    // in a test environment that computes no styles.
    const hidden = tab === "notes" ? ".side" : ".nav";
    const items = [...root.querySelectorAll<HTMLElement>(FOCUSABLE)].filter((n) => !n.closest(hidden));
    if (items.length === 0) return;
    const first = items[0];
    const last = items[items.length - 1];
    const active = document.activeElement;
    if (e.shiftKey && (active === first || !root.contains(active))) {
      e.preventDefault();
      last.focus();
    } else if (!e.shiftKey && active === last) {
      e.preventDefault();
      first.focus();
    }
  };

  const onClick = (e: MouseEvent) => {
    if (!open) return;
    const a = (e.target as HTMLElement | null)?.closest("a");
    if (!a || !panes.current?.contains(a)) return;
    if (a.classList.contains("tag") || a.classList.contains("tag-clear")) select("notes");
    else close();
  };

  return (
    <>
      {open && <div class="drawer-backdrop" onClick={close} />}
      <div
        ref={panes}
        id={DRAWER_ID}
        class={open ? "panes open" : "panes"}
        data-tab={tab}
        role={open ? "dialog" : undefined}
        aria-modal={open ? "true" : undefined}
        aria-label={open ? "Navigator, search and tags" : undefined}
        onKeyDown={onKeyDown}
        onClick={onClick}
      >
        <div class="drawer-head">
          {/* Two toggles rather than an ARIA tablist: with the panels as
              plain regions there is no roving focus to get wrong, and the
              pressed state says the same thing. */}
          <button type="button" class={tab === "notes" ? "drawer-tab on" : "drawer-tab"} aria-pressed={tab === "notes"} onClick={() => select("notes")}>
            Notes
          </button>
          <button type="button" class={tab === "find" ? "drawer-tab on" : "drawer-tab"} aria-pressed={tab === "find"} onClick={() => select("find")}>
            Search &amp; tags
          </button>
          <button type="button" class="drawer-close" aria-label="Close navigation" onClick={close}>
            <svg viewBox="0 0 20 20" width="18" height="18" aria-hidden="true" focusable="false">
              <path d="M5 5l10 10M15 5L5 15" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" />
            </svg>
          </button>
        </div>
        {children}
      </div>
    </>
  );
}

/** The burger: opens the drawer on the notes tab. */
export function NavToggle({ state }: { state: DrawerState }) {
  return (
    <button
      type="button"
      class="drawer-toggle nav-toggle"
      aria-label="Open navigation"
      aria-controls={DRAWER_ID}
      aria-expanded={state.open}
      onClick={(e) => state.show("notes", e.currentTarget as HTMLElement)}
    >
      <svg viewBox="0 0 20 20" width="20" height="20" aria-hidden="true" focusable="false">
        <path d="M3 5h14M3 10h14M3 15h14" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" />
      </svg>
    </button>
  );
}

/** The magnifier: opens the same drawer on the search-and-tags tab. */
export function FindToggle({ state }: { state: DrawerState }) {
  return (
    <button
      type="button"
      class="drawer-toggle find-toggle"
      aria-label="Search and tags"
      aria-controls={DRAWER_ID}
      aria-expanded={state.open}
      onClick={(e) => state.show("find", e.currentTarget as HTMLElement)}
    >
      <svg viewBox="0 0 20 20" width="20" height="20" aria-hidden="true" focusable="false">
        <circle cx="9" cy="9" r="5.5" fill="none" stroke="currentColor" stroke-width="2" />
        <path d="M13 13l4 4" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" />
      </svg>
    </button>
  );
}
