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

/**
 * Where this worker last sent a tab, so its own redirect does not look like
 * the user navigating away from the note it just opened.
 */
const redirected = new Map<number, string>();

/** Tabs carrying a status record, so unrelated navigations cost nothing. */
const marked = new Set<number>();

async function showFailure(tabId: number, result: OpenResult & { status: "failed" }, source: string) {
  marked.add(tabId);
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
  marked.add(tabId);
  await setTabStatus(tabId, {
    kind: "opened",
    message: `opened in md-notes at ${url}`,
    source,
    at: Date.now(),
  });
  await chrome.action.setBadgeText({ tabId, text: "" });
  await chrome.action.setTitle({ tabId, title: "md-notes" });
}

/** Forgets a tab's badge and record once it has moved on to another page. */
async function forget(tabId: number) {
  marked.delete(tabId);
  await chrome.action.setBadgeText({ tabId, text: "" });
  await chrome.action.setTitle({ tabId, title: "md-notes" });
  await clearTabStatus(tabId);
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
      redirected.set(tabId, result.url);
      await chrome.tabs.update(tabId, { url: result.url });
      break;
    case "failed":
      // The page is left exactly as it is; the badge says why.
      await showFailure(tabId, result, url);
      break;
  }
  return result;
}

chrome.tabs.onUpdated.addListener((tabId, changeInfo, tab) => {
  if (changeInfo.status !== "loading") return;
  // `changeInfo.url` is set only when the URL *changes*, so a reload of a file
  // that failed carries no URL at all. Falling back to the tab's own URL —
  // delivered here on the strength of `file:///*` — is what makes every remedy
  // the popup suggests (start the daemon, fix the address, paste the token,
  // then reload) work in the tab that failed.
  const url = changeInfo.url ?? tab.url;
  if (url === undefined) return;

  if (!url.startsWith("file:")) {
    // Our own redirect is not the user leaving: the note we just opened keeps
    // the record saying where it came from. Any other address means this tab
    // has moved on, and its badge and record go with it.
    if (redirected.get(tabId) === url) return;
    redirected.delete(tabId);
    inFlight.delete(tabId);
    if (marked.has(tabId)) void forget(tabId).catch(() => undefined);
    return;
  }

  redirected.delete(tabId);
  // One attempt at a time per tab. A reload after that attempt finished is a
  // fresh one, and repeating either daemon call is safe: the roots list is a
  // GET, and registering a directory the daemon already serves returns the
  // existing root.
  if (inFlight.has(tabId)) return;
  inFlight.set(tabId, url);
  void handleNavigation(tabId, url)
    .catch((err: unknown) => {
      // A tab can go away mid-flight, and the chrome.* calls reject when it
      // does; an unhandled rejection would be logged as the worker's fault.
      console.warn("md-notes: handling", url, "failed:", err);
    })
    .finally(() => {
      if (inFlight.get(tabId) === url) inFlight.delete(tabId);
    });
});

chrome.tabs.onRemoved.addListener((tabId) => {
  inFlight.delete(tabId);
  redirected.delete(tabId);
  marked.delete(tabId);
  void clearTabStatus(tabId).catch(() => undefined);
});
