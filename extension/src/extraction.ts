/**
 * What the page hands back when it is clipped.
 *
 * The extraction runs inside the page (it needs a live DOM for Readability and
 * for the selection) and crosses into the service worker through the
 * structured clone `chrome.scripting.executeScript` performs, so this module
 * holds the vocabulary both sides share — and the validator the worker applies
 * to whatever actually arrived.
 */

/** Which of the two things the user asked to clip. */
export type ClipKind = "page" | "selection";

/** A page or selection, converted and ready to be saved. */
export interface Extraction {
  kind: ClipKind;
  /** The page's own URL, which becomes the note's `source`. */
  url: string;
  /** The article's title, or the document's; the user may edit it. */
  title: string;
  /** GitHub-flavoured markdown. */
  markdown: string;
}

/** What the injected function returns: an extraction, or why there is none. */
export type ExtractionResult = Extraction | { error: string };

/**
 * The name the injected bundle publishes its entry point under, on the
 * isolated world's global object. The worker injects the bundle as a file —
 * it carries Readability and Turndown, which a serialized function could not —
 * and then calls this global with a second, tiny `executeScript`, whose return
 * value is defined by the API rather than by the bundler's output shape.
 */
export const CLIP_ENTRY_POINT = "__mdNotesClip";

/** True when `value` is an extraction the worker can go on to save. */
export function isExtraction(value: unknown): value is Extraction {
  if (value === null || typeof value !== "object") return false;
  const e = value as Partial<Extraction>;
  return (
    (e.kind === "page" || e.kind === "selection") &&
    typeof e.url === "string" &&
    e.url !== "" &&
    typeof e.title === "string" &&
    typeof e.markdown === "string"
  );
}

/** The failure an extraction result carries, or null when it is an extraction. */
export function extractionError(value: unknown): string | null {
  if (value === null || value === undefined) return "the page returned nothing";
  if (typeof value === "object" && typeof (value as { error?: unknown }).error === "string") {
    return (value as { error: string }).error;
  }
  return isExtraction(value) ? null : "the page returned something this extension cannot read";
}
