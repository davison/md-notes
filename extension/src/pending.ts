/**
 * The clip waiting to be saved.
 *
 * A clip is prepared in one act — the toolbar button, or the context menu —
 * and saved in another, after the user has looked at the title. Between the
 * two it lives in session storage, because the service worker that prepared
 * it may well have been shut down before the popup opens. There is one popup,
 * so there is one pending clip; it carries the tab and URL it came from, and
 * the popup shows both, so a clip is never saved under a page the user has
 * forgotten about.
 */

import type { Extraction } from "./extraction";
import type { StatusStore } from "./status";

export interface PendingClip extends Extraction {
  /** The tab the clip was taken from. */
  tabId: number;
  /** When it was taken, so the popup can say. */
  at: number;
}

export const PENDING_KEY = "pendingClip";

function sessionArea(): StatusStore {
  return chrome.storage.session as unknown as StatusStore;
}

export async function setPendingClip(
  clip: PendingClip,
  store: StatusStore = sessionArea(),
): Promise<void> {
  await store.set({ [PENDING_KEY]: clip });
}

export async function getPendingClip(
  store: StatusStore = sessionArea(),
): Promise<PendingClip | null> {
  const stored = await store.get([PENDING_KEY]);
  const value = stored[PENDING_KEY];
  return isPendingClip(value) ? value : null;
}

export async function clearPendingClip(store: StatusStore = sessionArea()): Promise<void> {
  await store.remove([PENDING_KEY]);
}

function isPendingClip(value: unknown): value is PendingClip {
  if (value === null || typeof value !== "object") return false;
  const c = value as Partial<PendingClip>;
  return (
    (c.kind === "page" || c.kind === "selection") &&
    typeof c.url === "string" &&
    typeof c.title === "string" &&
    typeof c.markdown === "string" &&
    typeof c.tabId === "number"
  );
}
