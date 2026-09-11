/**
 * What the popup asks the service worker to do.
 *
 * The popup *could* call the daemon itself — CORS exemption follows
 * `host_permissions` and covers every extension context, pages included, as
 * the options page's connection test demonstrates — but it should not. The
 * worker owns the clip and outlives the popup: a popup is destroyed the
 * moment it loses focus, taking an in-flight save with it, and a clip from
 * the context menu has no popup open when it is taken. One context speaks to
 * the daemon and one context holds the token. What CORS does govern is a
 * content script or a web page, and the daemon answers neither a preflight.
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
