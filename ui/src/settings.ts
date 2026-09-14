import { useEffect, useState } from "preact/hooks";

/**
 * The two display settings, held in `localStorage` and expressed as
 * attributes on the root element, which is what the stylesheet switches on.
 *
 * They exist for an e-ink tablet (davison/md-notes#52). Such a screen has no
 * backlight, so the dark scheme it inherits from an Android in night mode is
 * the wrong way round — grey on black is what an e-ink panel renders worst —
 * and every animation is a visible, slow, partial repaint rather than motion.
 *
 * Both attributes are also written by an inline script in `index.html`, which
 * runs before the stylesheet paints so the page never shows a frame of the
 * scheme the reader has overridden. `BOOT` is that script's source, exported
 * so a test can hold the copy in `index.html` to it; it deliberately duplicates
 * the key and the attribute names rather than importing them, because the
 * module bundle it would have to import from is deferred and the first paint
 * is not.
 */
export interface Settings {
  /** Use the light palette whatever the device's colour-scheme preference says. */
  light: boolean;
  /** Draw no animation and no transition. */
  noMotion: boolean;
}

export const KEY = "mdn:settings";

/** The attribute the light override sets, and the value it sets it to. */
export const THEME_ATTR = "data-theme";
export const THEME_LIGHT = "light";

/** The attribute the no-animation setting sets, and its value. */
export const MOTION_ATTR = "data-motion";
export const MOTION_NONE = "none";

export const DEFAULTS: Settings = { light: false, noMotion: false };

/**
 * The inline boot script, as it stands in `index.html`. Kept here so the
 * duplication is checked rather than merely commented: `settings.test.ts`
 * fails when the two drift apart.
 */
export const BOOT = `var s={};try{s=JSON.parse(localStorage.getItem("${KEY}"))||{}}catch(e){}\
if(s.light)document.documentElement.setAttribute("${THEME_ATTR}","${THEME_LIGHT}");\
if(s.noMotion)document.documentElement.setAttribute("${MOTION_ATTR}","${MOTION_NONE}");`;

/** Reads the stored settings, falling back to the defaults for anything missing. */
export function read(): Settings {
  try {
    const raw = localStorage.getItem(KEY);
    if (raw) {
      const stored = JSON.parse(raw) as Partial<Settings>;
      return {
        light: stored.light === true,
        noMotion: stored.noMotion === true,
      };
    }
  } catch {
    // Storage unavailable or the value corrupt: the defaults are a working
    // application, so there is nothing to report.
  }
  return { ...DEFAULTS };
}

/** Puts the settings on the root element, where the stylesheet reads them. */
export function apply(s: Settings, root: Element = document.documentElement): void {
  if (s.light) root.setAttribute(THEME_ATTR, THEME_LIGHT);
  else root.removeAttribute(THEME_ATTR);
  if (s.noMotion) root.setAttribute(MOTION_ATTR, MOTION_NONE);
  else root.removeAttribute(MOTION_ATTR);
}

let current: Settings | null = null;
const listeners = new Set<() => void>();

/** The settings in force, read from storage on first use. */
export function settings(): Settings {
  if (current === null) current = read();
  return current;
}

/** Changes one or both settings: stored, applied, and announced to subscribers. */
export function update(patch: Partial<Settings>): Settings {
  const next: Settings = { ...settings(), ...patch };
  current = next;
  try {
    localStorage.setItem(KEY, JSON.stringify(next));
  } catch {
    // Storage unavailable: the setting holds for this page and no longer.
  }
  apply(next);
  for (const listener of [...listeners]) listener();
  return next;
}

/** Forgets the cached settings, so the next read comes from storage. Tests only. */
export function reset(): void {
  current = null;
}

const REDUCED_MOTION = "(prefers-reduced-motion: reduce)";

function reducedMotion(): MediaQueryList | null {
  if (typeof window === "undefined" || typeof window.matchMedia !== "function") return null;
  try {
    return window.matchMedia(REDUCED_MOTION);
  } catch {
    return null;
  }
}

/**
 * Whether the application should draw no animation: the setting, or the
 * device asking for reduced motion. The stylesheet answers the same question
 * for what it draws; this is for the animations script starts, which CSS
 * cannot call off once they have been asked for.
 */
export function animationsOff(): boolean {
  return settings().noMotion || reducedMotion()?.matches === true;
}

/**
 * The settings, re-rendering the caller when either they or the device's
 * reduced-motion preference changes.
 */
export function useSettings(): Settings {
  const [value, setValue] = useState(settings);
  useEffect(() => {
    const refresh = () => setValue({ ...settings() });
    listeners.add(refresh);
    const query = reducedMotion();
    query?.addEventListener?.("change", refresh);
    // Storage changed in another tab of the same daemon: the attributes the
    // boot script set there are this page's business too.
    const onStorage = (e: StorageEvent) => {
      if (e.key !== null && e.key !== KEY) return;
      current = read();
      apply(current);
      refresh();
    };
    window.addEventListener("storage", onStorage);
    return () => {
      listeners.delete(refresh);
      query?.removeEventListener?.("change", refresh);
      window.removeEventListener("storage", onStorage);
    };
  }, []);
  return value;
}
