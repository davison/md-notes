import { useEffect, useState } from "preact/hooks";
import { useSettings, type Settings } from "./settings";

/**
 * Flowcharts in the reading view (davison/md-notes#171).
 *
 * The daemon draws each drawable ```mermaid block as an SVG and lists the
 * blocks beside the note's HTML — `{line, hash}`, where `line` is the
 * `data-line` of the anchor the renderer puts before every fenced block. The
 * HTML itself still holds the code block. This module puts an `<img>` of the
 * drawing between the anchor and the `<pre>`, and hides the `<pre>` while the
 * image stands; if the image fails for any reason — refused by a bound,
 * offline, a stale note — it takes itself out and the code block is back, so
 * nothing ever shows a broken-image icon.
 *
 * The image is never inline SVG and never markup from the note: it is an
 * element this code makes, pointing at the daemon's diagram route, so the
 * drawing cannot run, style or insert anything in the page.
 */

/** The palettes the daemon draws in, by the name its route takes. */
export type DiagramTheme = "light" | "dark" | "eink";

/** The class on the image, and the one that hides the code block behind it. */
export const DIAGRAM_CLASS = "diagram";
export const SOURCE_HIDDEN_CLASS = "diagram-source";

/** What the note's JSON says about one drawable block. */
export interface DiagramRef {
  line: number;
  hash: string;
}

/**
 * The palette for the page as it is now. The light override is the e-ink
 * setting (#52) and gets the e-ink palette — black on white, heavier lines —
 * which is what a panel with no backlight draws best and is still plain on
 * any screen. Otherwise the palette follows the device's scheme, as the page
 * does.
 */
export function diagramTheme(s: Settings, dark: boolean): DiagramTheme {
  if (s.light) return "eink";
  return dark ? "dark" : "light";
}

const DARK = "(prefers-color-scheme: dark)";

function darkQuery(): MediaQueryList | null {
  if (typeof window === "undefined" || typeof window.matchMedia !== "function") return null;
  try {
    return window.matchMedia(DARK);
  } catch {
    return null;
  }
}

/** The palette, re-rendering the caller when the setting or the device's scheme changes. */
export function useDiagramTheme(): DiagramTheme {
  const s = useSettings();
  const [dark, setDark] = useState(() => darkQuery()?.matches === true);
  useEffect(() => {
    const query = darkQuery();
    const refresh = () => setDark(query?.matches === true);
    refresh();
    query?.addEventListener?.("change", refresh);
    return () => query?.removeEventListener?.("change", refresh);
  }, []);
  return diagramTheme(s, dark);
}

/** The images one note view has made, kept so a re-render can reuse them. */
export type DiagramPool = Map<string, HTMLImageElement>;

/**
 * Puts each listed diagram's image in front of its code block inside
 * `scope`, which holds the note's rendered HTML.
 *
 * It can be called again whenever the HTML or the theme changes. An image
 * already made for the same block (the same source, the same occurrence of
 * it) is the same element moved into place, so an unchanged diagram is not
 * fetched again after a live update; a changed theme is a changed `src`.
 * Images for blocks that are gone are dropped from the pool.
 *
 * An entry whose anchor is missing or is not directly followed by a `<pre>`
 * is skipped: the list and the markup come from the same parse, so that is
 * a mismatch to survive rather than to guess around.
 */
export function decorate(
  scope: Element,
  diagrams: readonly DiagramRef[],
  urlFor: (hash: string) => string,
  pool: DiagramPool,
): void {
  const anchors = new Map<number, Element>();
  for (const a of scope.querySelectorAll("div.line-anchor[data-line]")) {
    const line = Number(a.getAttribute("data-line"));
    if (!anchors.has(line)) anchors.set(line, a);
  }
  const occurrences = new Map<string, number>();
  const placed = new Set<string>();
  for (const d of diagrams) {
    const n = occurrences.get(d.hash) ?? 0;
    occurrences.set(d.hash, n + 1);
    const key = `${d.hash}#${n}`;
    const anchor = anchors.get(d.line);
    const pre = anchor?.nextElementSibling;
    // The image may already stand between them, from an earlier call over
    // this same HTML.
    const block = pre instanceof HTMLImageElement && pre === pool.get(key) ? pre.nextElementSibling : pre;
    if (!anchor || !block || block.tagName !== "PRE") continue;

    let img = pool.get(key);
    if (!img) {
      img = document.createElement("img");
      img.className = DIAGRAM_CLASS;
      img.decoding = "async";
      img.loading = "lazy";
      pool.set(key, img);
    }
    const image = img;
    image.alt = block.textContent ?? "";
    // Whatever goes wrong, the code block comes back and the image goes:
    // a later call makes a fresh one and tries again.
    image.onerror = () => {
      image.remove();
      block.classList.remove(SOURCE_HIDDEN_CLASS);
      if (pool.get(key) === image) pool.delete(key);
    };
    const src = urlFor(d.hash);
    if (image.getAttribute("src") !== src) image.setAttribute("src", src);
    if (anchor.nextElementSibling !== image) anchor.after(image);
    block.classList.add(SOURCE_HIDDEN_CLASS);
    placed.add(key);
  }
  for (const [key, img] of pool) {
    if (placed.has(key)) continue;
    img.remove();
    pool.delete(key);
  }
}
