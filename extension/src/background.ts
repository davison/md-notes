/**
 * The MV3 service worker. Its only job in this milestone is the file-URL
 * intercept: a local markdown file the browser is about to render as plain
 * text is opened in md-notes instead.
 *
 * It listens on `chrome.tabs.onUpdated` rather than `chrome.webNavigation`,
 * which keeps the manifest free of both the `tabs` and the `webNavigation`
 * permission: `host_permissions: ["file:///*"]` alone is enough for the tab's
 * URL to be delivered here.
 */

import { resolveOpen, type OpenResult } from "./open-file";
import { loadSettings } from "./settings";
import { clearTabStatus, setTabStatus } from "./status";

const BADGE_COLOUR = "#b3261e";

/** Navigations already being handled, so a repeated `loading` does not race. */
const inFlight = new Map<number, string>();

async function showFailure(tabId: number, result: OpenResult & { status: "failed" }, source: string) {
  await setTabStatus(tabId, {
    kind: result.kind,
    message: result.message,
    source,
    at: Date.now(),
  });
  await chrome.action.setBadgeBackgroundColor({ color: BADGE_COLOUR });
  await chrome.action.setBadgeText({ tabId, text: "!" });
  await chrome.action.setTitle({ tabId, title: `md-notes: ${result.message}` });
}

async function showOpened(tabId: number, source: string, url: string) {
  await setTabStatus(tabId, {
    kind: "opened",
    message: `opened in md-notes at ${url}`,
    source,
    at: Date.now(),
  });
  await chrome.action.setBadgeText({ tabId, text: "" });
  await chrome.action.setTitle({ tabId, title: "md-notes" });
}

/**
 * Handles one navigation. Exported so the worker's behaviour is reachable from
 * a test without driving Chromium's event plumbing.
 */
export async function handleNavigation(tabId: number, url: string): Promise<OpenResult> {
  const settings = await loadSettings();
  const result = await resolveOpen(url, settings);
  switch (result.status) {
    case "ignored":
      break;
    case "open":
      await showOpened(tabId, url, result.url);
      await chrome.tabs.update(tabId, { url: result.url });
      break;
    case "failed":
      // The page is left exactly as it is; the badge says why.
      await showFailure(tabId, result, url);
      break;
  }
  return result;
}

chrome.tabs.onUpdated.addListener((tabId, changeInfo) => {
  const url = changeInfo.url;
  if (changeInfo.status !== "loading" || url === undefined) return;
  if (!url.startsWith("file:")) {
    // Navigating away from a file URL clears whatever the last attempt said.
    if (inFlight.get(tabId) !== undefined) inFlight.delete(tabId);
    return;
  }
  if (inFlight.get(tabId) === url) return;
  inFlight.set(tabId, url);
  void handleNavigation(tabId, url).finally(() => {
    if (inFlight.get(tabId) === url) inFlight.delete(tabId);
  });
});

chrome.tabs.onRemoved.addListener((tabId) => {
  inFlight.delete(tabId);
  void clearTabStatus(tabId);
});
