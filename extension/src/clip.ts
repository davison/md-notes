/**
 * Turning an extraction into a request, and a failure into what the popup
 * says. Pure: no `chrome`, no `fetch`, so both halves are tested directly.
 */

import { DaemonError, type ClipRequest, type FailureKind } from "./daemon";
import type { ClipKind, Extraction } from "./extraction";
import type { Settings } from "./settings";

/**
 * The two context-menu entries. `page` fires on any right-click in a page and
 * `selection` only when there is a selection under the pointer, so the menu
 * offers exactly what is available. (The types are chrome's ambient ones; the
 * value is data, and nothing here calls an extension API.)
 */
export const CLIP_MENU: chrome.contextMenus.CreateProperties[] = [
  { id: "clip-page", title: "Clip page to md-notes", contexts: ["page"] },
  { id: "clip-selection", title: "Clip selection to md-notes", contexts: ["selection"] },
];

/** What a clicked menu entry asks for, or null when it is not one of ours. */
export function menuKind(id: string | number | undefined): ClipKind | null {
  if (id === "clip-page") return "page";
  if (id === "clip-selection") return "selection";
  return null;
}

/**
 * The longest title the daemon keeps. It cuts at the same length itself; the
 * extension does it too so the popup's field shows what will actually be
 * written rather than something silently truncated later.
 */
export const MAX_TITLE = 300;

/** One line, no runs of whitespace, bounded — what a frontmatter title may be. */
export function cleanTitle(title: string): string {
  return title.replace(/\s+/g, " ").trim().slice(0, MAX_TITLE);
}

/**
 * The clip request for an extraction saved under `title` — the title the user
 * may have edited in the popup, falling back to the one the page gave.
 *
 * The markdown gains exactly one trailing newline. The daemon writes what it
 * is sent byte for byte and adds nothing, so a note that ends without one is
 * the extension's doing, and this is the one place that decides it.
 */
export function buildClipRequest(extraction: Extraction, title: string): ClipRequest {
  const chosen = cleanTitle(title);
  return {
    url: extraction.url,
    title: chosen !== "" ? chosen : cleanTitle(extraction.title),
    markdown: extraction.markdown.replace(/\s+$/, "") + "\n",
    kind: extraction.kind,
  };
}

/** What the popup shows when a clip could not be saved. */
export interface ClipFailure {
  kind: FailureKind;
  /** One sentence, carrying the daemon's own words where it had any. */
  message: string;
  /** Whether the remedy is on the options page, so the popup links to it. */
  offerOptions: boolean;
}

/**
 * A failed save, in the terms M3-R3 asks to be told apart: the daemon not
 * being reachable, a token it rejected, and no token configured at all —
 * which the daemon reports as `cross_origin`, because without the header it
 * never sees a token to judge, and which the extension also knows before
 * asking when its own settings are empty.
 */
export function describeClipFailure(error: unknown, settings: Settings): ClipFailure {
  if (!(error instanceof DaemonError)) {
    return {
      kind: "bad_response",
      message: error instanceof Error ? error.message : String(error),
      offerOptions: false,
    };
  }
  switch (error.kind) {
    case "unreachable":
      return {
        kind: error.kind,
        message: `Daemon not reachable at ${settings.daemonUrl}. Start \`mdn serve\`, or correct the address in the options.`,
        offerOptions: true,
      };
    case "no_token":
      return {
        kind: error.kind,
        message:
          "No token configured. Run `mdn token` and paste it on the options page; clipping cannot work without it.",
        offerOptions: true,
      };
    case "token_rejected":
      return {
        kind: error.kind,
        message: `Token rejected: ${error.detail}. \`mdn token\` prints the current one.`,
        offerOptions: true,
      };
    case "origin_refused":
      return {
        kind: error.kind,
        message:
          settings.token === ""
            ? "No token configured. Run `mdn token` and paste it on the options page; clipping cannot work without it."
            : `The daemon refused this extension's origin even with a token: ${error.detail}.`,
        offerOptions: true,
      };
    default:
      // A refusal of the clip itself — too large, a name collision, a clips
      // directory outside the root. The daemon's wording is the useful part.
      return { kind: error.kind, message: error.detail, offerOptions: false };
  }
}
