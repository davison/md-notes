import { useEffect, useRef } from "preact/hooks";

/**
 * A flowchart at its natural size (davison/md-notes#187), opened from the
 * button a diagram in the reading view stands in. A wide diagram is shown in
 * the note at no less than a floor on its scale, which can still be smaller
 * than it was drawn; this is where the whole of it is seen as drawn.
 *
 * The image is the same URL as the one in the note, so the same palette and
 * the same cached drawing. It fills the window and scrolls both ways when it
 * is larger. Escape closes it, as the close button and a click beside the
 * image do. Escape is answered on the window in the capture phase and
 * stopped there, as the dialog does (./dialog.tsx), so the drawer and the
 * note pane's own Escape listeners do not also act on it. Focus moves to the
 * close button on open, cycles between the button and the scrolling area,
 * which can be scrolled with the arrow keys, and goes back to the diagram
 * that opened it on close. That is `opener`, not whatever had focus, because
 * a click does not focus a button in every browser.
 */
export function DiagramViewer({
  src,
  alt,
  width,
  height,
  opener,
  onClose,
}: {
  src: string;
  alt: string;
  width?: string;
  height?: string;
  opener: HTMLElement | null;
  onClose: () => void;
}) {
  const box = useRef<HTMLDivElement>(null);

  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if (e.key !== "Escape") return;
      e.preventDefault();
      e.stopPropagation();
      onClose();
    };
    window.addEventListener("keydown", onKey, true);
    return () => window.removeEventListener("keydown", onKey, true);
  }, [onClose]);

  useEffect(() => {
    box.current?.querySelector<HTMLElement>(".diagram-viewer-close")?.focus();
    return () => {
      if (opener?.isConnected) opener.focus();
    };
  }, [opener]);

  const onKeyDown = (e: KeyboardEvent) => {
    if (e.key !== "Tab") return;
    const root = box.current;
    if (!root) return;
    const items = [...root.querySelectorAll<HTMLElement>(".diagram-viewer-close, .diagram-viewer-scroll")];
    const first = items[0];
    const last = items[items.length - 1];
    const active = document.activeElement;
    if (e.shiftKey && (active === first || !root.contains(active))) {
      e.preventDefault();
      last.focus();
    } else if (!e.shiftKey && (active === last || !root.contains(active))) {
      e.preventDefault();
      first.focus();
    }
  };

  return (
    <div
      ref={box}
      class="diagram-viewer"
      role="dialog"
      aria-modal="true"
      aria-label="Diagram at natural size"
      onKeyDown={onKeyDown}
    >
      <div class="diagram-viewer-bar">
        <button type="button" class="diagram-viewer-close" onClick={onClose}>
          Close
        </button>
      </div>
      <div
        class="diagram-viewer-scroll"
        tabIndex={0}
        onClick={(e) => {
          if (e.target === e.currentTarget) onClose();
        }}
      >
        <img src={src} alt={alt} width={width} height={height} />
      </div>
    </div>
  );
}
