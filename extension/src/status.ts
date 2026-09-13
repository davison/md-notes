/**
 * The last thing the extension tried in a tab, kept so the popup can explain a
 * badge. Session storage: it is per browser run and never reaches disk.
 */

import type { FailureKind } from "./daemon";

export interface TabStatus {
  /** "opened" when the tab was redirected into the app. */
  kind: "opened" | FailureKind;
  message: string;
  /** The `file:` URL the extension was acting on. */
  source: string;
  at: number;
}

/** The slice of `chrome.storage.StorageArea` the status needs. */
export interface StatusStore {
  get(keys: string[]): Promise<Record<string, unknown>>;
  set(items: Record<string, unknown>): Promise<void>;
  remove(keys: string[]): Promise<void>;
}

function sessionArea(): StatusStore {
  return chrome.storage.session as unknown as StatusStore;
}

export function statusKey(tabId: number): string {
  return `status:${tabId}`;
}

export async function setTabStatus(
  tabId: number,
  status: TabStatus,
  store: StatusStore = sessionArea(),
): Promise<void> {
  await store.set({ [statusKey(tabId)]: status });
}

export async function getTabStatus(
  tabId: number,
  store: StatusStore = sessionArea(),
): Promise<TabStatus | null> {
  const key = statusKey(tabId);
  const stored = await store.get([key]);
  const value = stored[key];
  return isTabStatus(value) ? value : null;
}

export async function clearTabStatus(
  tabId: number,
  store: StatusStore = sessionArea(),
): Promise<void> {
  await store.remove([statusKey(tabId)]);
}

function isTabStatus(value: unknown): value is TabStatus {
  if (value === null || typeof value !== "object") return false;
  const s = value as Partial<TabStatus>;
  return typeof s.kind === "string" && typeof s.message === "string";
}
