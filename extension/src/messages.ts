/**
 * What the popup asks the service worker to do.
 *
 * The popup cannot talk to the daemon itself: an `Authorization` header makes
 * a cross-origin fetch non-simple, the daemon answers no preflight, and a
 * popup document is an ordinary page as far as CORS is concerned. Only the
 * MV3 service worker is exempt. So the popup sends these, and the worker does
 * the work.
 */

import type { ClipFailure } from "./clip";
import type { ClipKind } from "./extraction";
import type { PendingClip } from "./pending";

export type ClipMessage =
  /** Extract a clip from a tab and hold it for the popup to review. */
  | { type: "clip:prepare"; tabId: number; kind: ClipKind }
  /** Save the held clip under this title. */
  | { type: "clip:save"; title: string }
  /** Throw the held clip away. */
  | { type: "clip:discard" };

export type PrepareReply = { ok: true; clip: PendingClip } | { ok: false; message: string };

export type SaveReply =
  | { ok: true; root: string; path: string; url: string }
  | { ok: false; failure: ClipFailure };

export type DiscardReply = { ok: true };

export function isClipMessage(value: unknown): value is ClipMessage {
  if (value === null || typeof value !== "object") return false;
  const m = value as { type?: unknown; tabId?: unknown; kind?: unknown; title?: unknown };
  switch (m.type) {
    case "clip:prepare":
      return (
        typeof m.tabId === "number" && (m.kind === "page" || m.kind === "selection")
      );
    case "clip:save":
      return typeof m.title === "string";
    case "clip:discard":
      return true;
    default:
      return false;
  }
}

/** Sends a message to the worker and returns its reply. */
export async function ask<T>(message: ClipMessage): Promise<T> {
  return (await chrome.runtime.sendMessage(message)) as T;
}
