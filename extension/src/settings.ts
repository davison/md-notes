/** The extension's stored configuration: which daemon, and its token. */

import { trimSlash } from "./paths";

export const DEFAULT_DAEMON_URL = "http://localhost:7337";

export interface Settings {
  /** Base URL of the daemon, with no trailing slash. */
  daemonUrl: string;
  /** Bearer token from `mdn token`; empty when the user has not pasted one. */
  token: string;
}

export const DEFAULT_SETTINGS: Settings = {
  daemonUrl: DEFAULT_DAEMON_URL,
  token: "",
};

/** The slice of `chrome.storage.StorageArea` the settings need. */
export interface StorageArea {
  get(keys: string[]): Promise<Record<string, unknown>>;
  set(items: Record<string, unknown>): Promise<void>;
}

function localArea(): StorageArea {
  return chrome.storage.local as unknown as StorageArea;
}

/** Reads the settings, filling anything unset with its default. */
export async function loadSettings(store: StorageArea = localArea()): Promise<Settings> {
  const stored = await store.get(["daemonUrl", "token"]);
  return {
    daemonUrl:
      typeof stored.daemonUrl === "string" && stored.daemonUrl !== ""
        ? trimSlash(stored.daemonUrl)
        : DEFAULT_SETTINGS.daemonUrl,
    token: typeof stored.token === "string" ? stored.token : DEFAULT_SETTINGS.token,
  };
}

/** Writes the settings back, normalised. */
export async function saveSettings(
  settings: Settings,
  store: StorageArea = localArea(),
): Promise<void> {
  await store.set({
    daemonUrl: normaliseDaemonUrl(settings.daemonUrl),
    token: settings.token.trim(),
  });
}

/**
 * Canonical form of a daemon URL the user typed. A bare host gets `http://`,
 * a trailing slash and any path, query or fragment are dropped. Throws when
 * the result is not an http(s) origin.
 */
export function normaliseDaemonUrl(input: string): string {
  const text = input.trim();
  if (text === "") return DEFAULT_DAEMON_URL;
  const withScheme = /^[a-z][a-z0-9+.-]*:\/\//i.test(text) ? text : `http://${text}`;
  let parsed: URL;
  try {
    parsed = new URL(withScheme);
  } catch {
    throw new Error(`"${input}" is not a URL`);
  }
  if (parsed.protocol !== "http:" && parsed.protocol !== "https:") {
    throw new Error("the daemon URL must be http or https");
  }
  return trimSlash(parsed.origin);
}

/**
 * The host permission match pattern covering a daemon URL. Chromium enforces
 * the port, so a daemon on a port other than the manifest's default needs an
 * optional permission granted from the options page.
 */
export function originPattern(daemonUrl: string): string {
  return `${trimSlash(daemonUrl)}/*`;
}
