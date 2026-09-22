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
 *
 * A wide drawing is shrunk to the reading column only so far: below a floor
 * on its scale its labels stop being readable, so it keeps that size and
 * scrolls sideways in a box of its own, and the page never does (#187). The
 * image is also a button that opens the drawing at its natural size.
 */

/** The palettes the daemon draws in, by the name its route takes. */
export type DiagramTheme = "light" | "dark" | "eink";

/** The class on the image, and the one that hides the code block behind it. */
export const DIAGRAM_CLASS = "diagram";
export const SOURCE_HIDDEN_CLASS = "diagram-source";

/** The scroll box the image stands in, and the button inside it that opens the natural-size view. */
export const BOX_CLASS = "diagram-box";
export const OPEN_CLASS = "diagram-open";

/**
 * The custom property that carries a drawing's natural width to the
 * stylesheet, which shows it at no less than the floor's share of that and
 * no more than all of it. Set through the CSSOM, which the page's content
 * security policy does not govern, rather than as a `style` attribute.
 */
export const WIDTH_PROPERTY = "--diagram-width";

/** The box an image of the pool stands in: the button's parent. */
export function boxOf(img: HTMLImageElement): HTMLElement | null {
  const box = img.parentElement?.parentElement;
  return box?.classList.contains(BOX_CLASS) ? box : null;
}

/** Makes the box and the button once, around an image that has neither. */
function frame(img: HTMLImageElement): HTMLElement {
  const existing = boxOf(img);
  if (existing) return existing;
  const box = document.createElement("div");
  box.className = BOX_CLASS;
  const open = document.createElement("button");
  open.type = "button";
  open.className = OPEN_CLASS;
  open.title = "Open at natural size";
  open.setAttribute("aria-haspopup", "dialog");
  open.append(img);
  box.append(open);
  return box;
}

/** What the note's JSON says about one drawable block. */
export interface DiagramRef {
  line: number;
  hash: string;
  /**
   * The drawing's natural size, from the daemon's layout. Set on the image
   * so its box is reserved before the SVG arrives: a scroll to a line below
   * it lands, and nothing below it moves when it loads (#177).
   */
  width?: number;
  height?: number;
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

/**
 * The images one note view has made, kept so a re-render can reuse them,
 * and the drawing URLs that failed on this page. A failed URL is not asked
 * for again: the block stays code across live updates and re-renders until
 * its source or the theme changes, which is a different URL (the round-one
 * review of PR #175, N3). Opening the note again starts a fresh pool.
 */
export class DiagramPool extends Map<string, HTMLImageElement> {
  readonly failed = new Set<string>();
}

/**
 * Puts each listed diagram's image in front of its code block inside
 * `scope`, which holds the note's rendered HTML.
 *
 * It can be called again whenever the HTML or the theme changes. An image
 * already made for the same block (the same source, the same occurrence of
 * it) is the same element moved into place, so an unchanged diagram is not
 * fetched again after a live update; a changed theme is a changed `src`.
 * Images for blocks that are gone are dropped from the pool. A block whose
 * drawing already failed at this URL is left as code without asking again.
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
    // The image's box may already stand between them, from an earlier call
    // over this same HTML.
    const standing = pool.get(key);
    const block = standing && pre && pre === boxOf(standing) ? pre.nextElementSibling : pre;
    if (!anchor || !block || block.tagName !== "PRE") continue;

    const src = urlFor(d.hash);
    if (pool.failed.has(src)) {
      // An image for another theme may stand here; the loop below takes it
      // away, so the code has to come back with it.
      block.classList.remove(SOURCE_HIDDEN_CLASS);
      continue;
    }

    let img = pool.get(key);
    if (!img) {
      img = document.createElement("img");
      img.className = DIAGRAM_CLASS;
      img.decoding = "async";
      img.loading = "lazy";
      pool.set(key, img);
    }
    const image = img;
    const box = frame(image);
    image.alt = block.textContent ?? "";
    // Whatever goes wrong, the code block comes back and the image goes,
    // and this URL is not asked for again on this page; a different one —
    // an edited source, another theme — is.
    image.onerror = () => {
      pool.failed.add(image.getAttribute("src") ?? "");
      box.remove();
      block.classList.remove(SOURCE_HIDDEN_CLASS);
      if (pool.get(key) === image) pool.delete(key);
    };
    // Before the src, so the box is there from the first layout, at the size
    // it will be shown at: the attributes give the aspect ratio and the
    // property the width, and the stylesheet does the rest. An image the
    // daemon had no time to measure learns its width when it loads.
    if (d.width && d.height) {
      image.setAttribute("width", String(d.width));
      image.setAttribute("height", String(d.height));
      image.style.setProperty(WIDTH_PROPERTY, `${d.width}px`);
    } else {
      image.onload = () => {
        if (image.naturalWidth > 0) image.style.setProperty(WIDTH_PROPERTY, `${image.naturalWidth}px`);
      };
    }
    if (image.getAttribute("src") !== src) image.setAttribute("src", src);
    if (anchor.nextElementSibling !== box) anchor.after(box);
    block.classList.add(SOURCE_HIDDEN_CLASS);
    placed.add(key);
  }
  for (const [key, img] of pool) {
    if (placed.has(key)) continue;
    (boxOf(img) ?? img).remove();
    pool.delete(key);
  }
}

/** What the reader does to take the scroll over from the page. */
const TAKEOVER = ["wheel", "touchstart", "keydown", "pointerdown"] as const;

/**
 * Keeps a scroll target in place while the diagram images above it that the
 * daemon did not measure settle (#177). Such an image has no box until it
 * loads, and each one that loads, or fails and brings its code block back,
 * moves everything below it; the target is scrolled to again each time. The
 * measured ones need nothing: their boxes are reserved.
 *
 * The page lets go as soon as the reader scrolls, clicks or types, when
 * every such image has settled, or when the returned function is called.
 */
export function holdInView(target: Element): () => void {
  const scope = target.closest(".markdown") ?? target.ownerDocument;
  const pending = [...scope.querySelectorAll<HTMLImageElement>(`img.${DIAGRAM_CLASS}:not([height])`)].filter(
    (img) => !img.complete && img.compareDocumentPosition(target) & Node.DOCUMENT_POSITION_FOLLOWING,
  );
  if (pending.length === 0) return () => {};
  let left = pending.length;
  const settle = () => {
    target.scrollIntoView({ block: "center" });
    if (--left === 0) release();
  };
  const release = () => {
    for (const img of pending) {
      img.removeEventListener("load", settle);
      img.removeEventListener("error", settle);
    }
    for (const type of TAKEOVER) window.removeEventListener(type, release, true);
  };
  for (const img of pending) {
    img.addEventListener("load", settle, { once: true });
    img.addEventListener("error", settle, { once: true });
  }
  for (const type of TAKEOVER) window.addEventListener(type, release, { capture: true, passive: true });
  return release;
}
