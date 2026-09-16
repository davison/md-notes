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
  const id = useId();

  // Escape is answered on the window in the capture phase, and stopped
  // there. The drawer and the editor listen for it on the document, which
  // capture reaches later, so the dialog on top answers first and alone —
  // cancelling a name prompt must not also close the drawer underneath it.
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if (e.key !== "Escape") return;
      e.preventDefault();
      e.stopPropagation();
      onCancel();
    };
    window.addEventListener("keydown", onKey, true);
    return () => window.removeEventListener("keydown", onKey, true);
  }, [onCancel]);

  useEffect(() => {
    const from = document.activeElement as HTMLElement | null;
    const root = box.current;
    const field = root?.querySelector<HTMLElement>("input, textarea");
    (field ?? root?.querySelector<HTMLElement>("button[type=submit]") ?? root?.querySelector<HTMLElement>(FOCUSABLE))?.focus();
    return () => from?.focus();
  }, []);

  const onKeyDown = (e: KeyboardEvent) => {
    if (e.key !== "Tab") return;
    const root = box.current;
    if (!root) return;
    const items = [...root.querySelectorAll<HTMLElement>(FOCUSABLE)];
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

  const submit = (e: Event) => {
    e.preventDefault();
    if (!busy) onConfirm();
  };

  return (
    <>
      <div class="modal-backdrop" onClick={onCancel} />
      <div ref={box} class="modal" role="dialog" aria-modal="true" aria-labelledby={id} onKeyDown={onKeyDown}>
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
