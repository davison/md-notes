import { useEffect, useId, useRef } from "preact/hooks";
import type { ComponentChildren } from "preact";

/**
 * The application's own modal dialog, for the two questions that must be
 * answered before anything is written: the name of a new note, and whether
 * a note really is to be deleted.
 *
 * Not window.prompt or window.confirm. Both are the browser's chrome rather
 * than the app's — no light override, no e-ink sizing, no tap targets — and
 * neither can show the daemon's refusal beside the name that was typed
 * without throwing that name away first. This is the same shape as the
 * drawer and the settings popover: markup the stylesheet owns, and the
 * keyboard behaviour written out.
 *
 * Enter confirms, because the content is a form and the confirm button is
 * its submit. Escape cancels. Focus moves in on open — to the first text
 * box if there is one, since that is what a prompt is for, and otherwise to
 * the confirm button — cycles inside while it is open, and is handed back
 * to whatever had it when the dialog closes.
 */
export function Dialog({
  title,
  confirmLabel,
  onConfirm,
  onCancel,
  busy = false,
  children,
}: {
  title: string;
  confirmLabel: string;
  onConfirm: () => void;
  onCancel: () => void;
  /** A request is in flight: the controls are inert until it answers. */
  busy?: boolean;
  children: ComponentChildren;
}) {
  const box = useRef<HTMLDivElement>(null);
  const parked = useRef(false);
  const id = useId();

  // Escape is answered on the window in the capture phase, and stopped
  // there. The drawer and the editor listen for it on the document, which
  // capture reaches later, so the dialog on top answers first and alone —
  // cancelling a name prompt must not also close the drawer underneath it.
  //
  // While a write is in flight the keystroke is still consumed but answers
  // nothing: `busy` disables both buttons to say "not now", and a dismissal
  // that still worked would be the one way out that cannot be seen to be
  // disabled — it would leave the prompt gone while the request it started
  // carried on to navigate or delete behind it.
  //
  // That "stopped there" is checked rather than remembered, and in two
  // places. `ui/src/dialog.test.tsx` registers a capture-phase document
  // listener and asserts it never fires; `ui/e2e/create-delete.test.mjs`
  // asks the same question of the built application in Chromium — "lets no
  // document-level listener see Escape while the prompt is open" — against
  // the note pane's own Ctrl+E listener, which is live under the prompt.
  // Delete the `stopPropagation` below and both fail. The decision on
  // davison/md-notes#78 records why the browser case that came before could
  // not hold this, and davison/md-notes#101 why the one that replaced it can.
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if (e.key !== "Escape") return;
      e.preventDefault();
      e.stopPropagation();
      if (!busy) onCancel();
    };
    window.addEventListener("keydown", onKey, true);
    return () => window.removeEventListener("keydown", onKey, true);
  }, [busy, onCancel]);

  useEffect(() => {
    const from = document.activeElement as HTMLElement | null;
    const root = box.current;
    const field = root?.querySelector<HTMLElement>("input, textarea");
    (field ?? root?.querySelector<HTMLElement>("button[type=submit]") ?? root?.querySelector<HTMLElement>(FOCUSABLE))?.focus();
    return () => from?.focus();
  }, []);

  /**
   * While busy every control is disabled, so the element that had focus is
   * no longer focusable: the browser drops focus to the body, outside the
   * dialog, where nothing holds it. The dialog itself takes focus for the
   * length of the request — it is why the box carries tabindex="-1" — and
   * hands it back when the answer arrives, to the text box when there is
   * one, since a refusal is a name to correct.
   */
  useEffect(() => {
    const root = box.current;
    if (!root) return;
    if (busy) {
      const active = document.activeElement as HTMLElement | null;
      if (!active || !root.contains(active) || active.matches("[disabled]")) {
        parked.current = true;
        root.focus();
      }
      return;
    }
    if (!parked.current) return;
    parked.current = false;
    (root.querySelector<HTMLElement>("input, textarea") ?? root.querySelector<HTMLElement>(FOCUSABLE))?.focus();
  }, [busy]);

  const onKeyDown = (e: KeyboardEvent) => {
    if (e.key !== "Tab") return;
    const root = box.current;
    if (!root) return;
    const items = [...root.querySelectorAll<HTMLElement>(FOCUSABLE)];
    if (items.length === 0) {
      // A confirmation with a write in flight has no enabled control at
      // all: there is nothing to cycle between, and the dialog holds focus
      // rather than letting Tab walk into the page behind it.
      e.preventDefault();
      root.focus();
      return;
    }
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

  const submit = (e: Event) => {
    e.preventDefault();
    if (!busy) onConfirm();
  };

  return (
    <>
      <div class="modal-backdrop" onClick={() => !busy && onCancel()} />
      <div
        ref={box}
        class="modal"
        role="dialog"
        aria-modal="true"
        aria-labelledby={id}
        tabIndex={-1}
        onKeyDown={onKeyDown}
      >
        <form onSubmit={submit}>
          <h2 id={id} class="modal-title">
            {title}
          </h2>
          {children}
          <div class="modal-actions">
            <button type="submit" class="primary" disabled={busy}>
              {confirmLabel}
            </button>
            <button type="button" onClick={onCancel} disabled={busy}>
              Cancel
            </button>
          </div>
        </form>
      </div>
    </>
  );
}

const FOCUSABLE =
  'a[href], button:not([disabled]), input:not([disabled]), select:not([disabled]), textarea:not([disabled]), [tabindex]:not([tabindex="-1"])';
