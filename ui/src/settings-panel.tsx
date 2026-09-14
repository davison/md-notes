import { useEffect, useRef, useState } from "preact/hooks";
import { update, useSettings } from "./settings";

/**
 * The display settings, behind a gear in the top bar: the light override and
 * the no-animation switch. It is one button at every width — the drawer, which
 * is where the rest of the chrome goes at narrow widths, exists only below the
 * breakpoint, and these two settings are wanted most on a tablet that is wide
 * enough to be above it.
 *
 * A popover rather than a route or a modal: two checkboxes are not a page, and
 * a modal would take focus away from the note to say so. It closes on Escape,
 * on a click outside it, and on the gear itself.
 */
export function SettingsMenu() {
  const settings = useSettings();
  const [open, setOpen] = useState(false);
  const box = useRef<HTMLDivElement>(null);
  const button = useRef<HTMLButtonElement>(null);

  useEffect(() => {
    if (!open) return;
    const onKey = (e: KeyboardEvent) => {
      if (e.key !== "Escape") return;
      e.preventDefault();
      e.stopPropagation();
      setOpen(false);
      button.current?.focus();
    };
    const onDown = (e: Event) => {
      const target = e.target as Node | null;
      if (box.current?.contains(target) || button.current?.contains(target)) return;
      setOpen(false);
    };
    // Captured, so Escape closes this before the drawer or the editor sees it.
    document.addEventListener("keydown", onKey, true);
    document.addEventListener("pointerdown", onDown, true);
    return () => {
      document.removeEventListener("keydown", onKey, true);
      document.removeEventListener("pointerdown", onDown, true);
    };
  }, [open]);

  return (
    <div class="settings">
      <button
        ref={button}
        type="button"
        class="settings-toggle"
        aria-label="Display settings"
        aria-expanded={open}
        onClick={() => setOpen((v) => !v)}
      >
        <svg viewBox="0 0 20 20" width="18" height="18" aria-hidden="true" focusable="false">
          <circle cx="10" cy="10" r="3" fill="none" stroke="currentColor" stroke-width="2" />
          <path
            d="M10 1.5v2.2M10 16.3v2.2M1.5 10h2.2M16.3 10h2.2M4 4l1.6 1.6M14.4 14.4L16 16M16 4l-1.6 1.6M5.6 14.4L4 16"
            fill="none"
            stroke="currentColor"
            stroke-width="2"
            stroke-linecap="round"
          />
        </svg>
      </button>
      {open && (
        <div ref={box} class="settings-panel" role="dialog" aria-label="Display settings">
          <label class="setting">
            <input
              type="checkbox"
              checked={settings.light}
              onChange={(e) => update({ light: (e.currentTarget as HTMLInputElement).checked })}
            />
            <span>
              Always use the light theme
              <span class="setting-note">Ignores the device's dark preference. For an e-ink screen.</span>
            </span>
          </label>
          <label class="setting">
            <input
              type="checkbox"
              checked={settings.noMotion}
              onChange={(e) => update({ noMotion: (e.currentTarget as HTMLInputElement).checked })}
            />
            <span>
              No animation
              <span class="setting-note">
                No transitions and no flash on a search hit. Already on when the device asks for reduced motion.
              </span>
            </span>
          </label>
        </div>
      )}
    </div>
  );
}
