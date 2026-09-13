/**
 * The MV3 service worker. It does two things.
 *
 * The file-URL intercept: a local markdown file the browser is about to render
 * as plain text is opened in md-notes instead. It listens on
 * `chrome.tabs.onUpdated` rather than `chrome.webNavigation`, which keeps the
 * manifest free of both the `tabs` and the `webNavigation` permission:
 * `host_permissions: ["file:///*"]` alone is enough for the tab's URL to be
 * delivered here.
 *
 * The clipper: the context menu and the popup both prepare a clip by injecting
 * the converter into the page, and the popup then asks for it to be saved. The
 * daemon call happens *here*, in the worker, rather than in the popup —
 * not because a popup could not make it (CORS exemption follows
 * `host_permissions` for every extension context; only content scripts and
 * web pages are governed by CORS) but because the worker owns the clip and
 * outlives the popup, which is destroyed the moment it loses focus.
 */

import { CLIP_MENU, buildClipRequest, describeClipFailure, menuKind } from "./clip";
import { postClip } from "./daemon";
import { CLIP_ENTRY_POINT, extractionError, isExtraction, type ClipKind } from "./extraction";
import { resolveOpen, type OpenResult } from "./open-file";
import { isClipMessage, type DiscardReply, type PrepareReply, type SaveReply } from "./messages";
import { noteUrl } from "./paths";
import { clearPendingClip, getPendingClip, setPendingClip, type PendingClip } from "./pending";
import { loadSettings } from "./settings";
import { clearTabStatus, setTabStatus } from "./status";

const BADGE_COLOUR = "#b3261e";
/** A clip waiting to be saved is not a failure, so it is not the failure red. */
const CLIP_BADGE_COLOUR = "#2a6db0";

/** Navigations already being handled, so a repeated `loading` does not race. */
const inFlight = new Map<number, string>();

/**
 * Where this worker last sent a tab, so its own redirect does not look like
 * the user navigating away from the note it just opened.
 */
const redirected = new Map<number, string>();

/** Tabs carrying a status record, so unrelated navigations cost nothing. */
const marked = new Set<number>();

/**
 * The tab the pending clip came from, mirrored here so the navigation
 * listener can drop a stale clip without reading storage on every event. The
 * mirror is lost when the worker is shut down; the clip itself is not, and
 * the popup shows the URL it was taken from, so the worst a lost mirror costs
 * is a clip that outlives its page and says where it came from.
 */
let pendingTab: number | null = null;

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

/** This tab is somewhere else now: drop everything remembered about it. */
function moveOn(tabId: number) {
  redirected.delete(tabId);
  inFlight.delete(tabId);
  if (pendingTab === tabId) void forgetPendingClip().catch(() => undefined);
  if (marked.has(tabId)) void forget(tabId).catch(() => undefined);
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

/**
 * (Re)creates the menu entries. Menus survive a browser restart in the
 * profile, so they are cleared first rather than created twice.
 */
async function installMenus(): Promise<void> {
  await chrome.contextMenus.removeAll();
  for (const item of CLIP_MENU) chrome.contextMenus.create(item);
}

/**
 * Runs the converter in a tab and returns what it made of the page.
 *
 * Two injections, deliberately. The first carries Readability and Turndown,
 * which is far more code than a serialized `func` could hold, and it publishes
 * one function; the second calls that function, and its return value is
 * defined by the scripting API rather than by whatever shape the bundler gave
 * the file. Both need `scripting` plus access to the tab, which is `activeTab`
 * — granted by the click that got us here and by nothing else.
 */
async function extractFrom(tabId: number, kind: ClipKind): Promise<PrepareReply> {
  let returned: unknown;
  try {
    await chrome.scripting.executeScript({ target: { tabId }, files: ["clip-inject.js"] });
    const [frame] = await chrome.scripting.executeScript({
      target: { tabId },
      func: (entryPoint: string, which: string) => {
        const clip = (globalThis as unknown as Record<string, unknown>)[entryPoint];
        return typeof clip === "function"
          ? (clip as (k: string) => unknown)(which)
          : { error: "the clipper did not load in this page" };
      },
      args: [CLIP_ENTRY_POINT, kind],
    });
    returned = frame?.result;
  } catch (err) {
    // Chromium refuses to inject into its own pages, the extension gallery and
    // anything the extension has no access to.
    return {
      ok: false,
      message: `This page cannot be clipped (${err instanceof Error ? err.message : String(err)}).`,
    };
  }
  const failure = extractionError(returned);
  if (failure !== null) return { ok: false, message: capitalise(failure) };
  if (!isExtraction(returned)) return { ok: false, message: "The page returned no clip." };
  return { ok: true, clip: { ...returned, tabId, at: Date.now() } };
}

function capitalise(text: string): string {
  return text === "" ? text : `${text[0]!.toUpperCase()}${text.slice(1)}.`;
}

/** Prepares a clip and holds it for the popup. */
async function prepareClip(tabId: number, kind: ClipKind): Promise<PrepareReply> {
  const reply = await extractFrom(tabId, kind);
  if (!reply.ok) return reply;
  await setPendingClip(reply.clip);
  pendingTab = tabId;
  return reply;
}

/** Saves the held clip under the title the user settled on. */
async function saveClip(title: string): Promise<SaveReply> {
  const pending = await getPendingClip();
  if (pending === null) {
    return {
      ok: false,
      failure: {
        kind: "bad_response",
        message: "There is no clip waiting to be saved.",
        offerOptions: false,
      },
    };
  }
  const settings = await loadSettings();
  try {
    const result = await postClip(settings, buildClipRequest(pending, title));
    await forgetPendingClip();
    return {
      ok: true,
      root: result.root,
      path: result.path,
      url: noteUrl(settings.daemonUrl, result.root, result.path),
    };
  } catch (err) {
    // The clip is kept: the user pastes a token, or starts the daemon, and
    // presses Save again without having to find the page a second time.
    return { ok: false, failure: describeClipFailure(err, settings) };
  }
}

/** Drops the held clip and the badge that announced it. */
async function forgetPendingClip(): Promise<void> {
  const tabId = pendingTab;
  pendingTab = null;
  await clearPendingClip();
  if (tabId === null || marked.has(tabId)) return;
  await chrome.action.setBadgeText({ tabId, text: "" });
  await chrome.action.setTitle({ tabId, title: "md-notes" });
}

/**
 * Says a clip is waiting. The context menu gives no popup of its own, so on a
 * browser that has `action.openPopup` the popup is opened for the user; on one
 * that has not, the badge is the invitation to open it.
 */
async function announceClip(clip: PendingClip): Promise<void> {
  await chrome.action.setBadgeBackgroundColor({ color: CLIP_BADGE_COLOUR });
  await chrome.action.setBadgeText({ tabId: clip.tabId, text: "1" });
  await chrome.action.setTitle({
    tabId: clip.tabId,
    title: "md-notes: a clip is ready to save",
  });
  const openPopup = (chrome.action as { openPopup?: () => Promise<void> }).openPopup;
  if (typeof openPopup !== "function") return;
  try {
    await openPopup.call(chrome.action);
  } catch {
    // Some browsers, and some window states, decline. The badge still says so.
  }
}

/** Says a clip could not even be taken. */
async function announceFailure(tabId: number, message: string): Promise<void> {
  await setTabStatus(tabId, { kind: "refused", message, source: "", at: Date.now() });
  marked.add(tabId);
  await chrome.action.setBadgeBackgroundColor({ color: BADGE_COLOUR });
  await chrome.action.setBadgeText({ tabId, text: "!" });
  await chrome.action.setTitle({ tabId, title: `md-notes: ${message}` });
}

chrome.runtime.onInstalled.addListener(() => void installMenus());
chrome.runtime.onStartup.addListener(() => void installMenus());

chrome.contextMenus.onClicked.addListener((info, tab) => {
  const kind = menuKind(info.menuItemId);
  if (kind === null || tab?.id === undefined) return;
  const tabId = tab.id;
  void prepareClip(tabId, kind)
    .then((reply) => (reply.ok ? announceClip(reply.clip) : announceFailure(tabId, reply.message)))
    .catch((err: unknown) => console.warn("md-notes: preparing a clip failed:", err));
});

chrome.runtime.onMessage.addListener((message: unknown, _sender, sendResponse) => {
  if (!isClipMessage(message)) return false;
  const work = (): Promise<PrepareReply | SaveReply | DiscardReply> => {
    switch (message.type) {
      case "clip:prepare":
        return prepareClip(message.tabId, message.kind);
      case "clip:save":
        return saveClip(message.title);
      case "clip:discard":
        return forgetPendingClip().then(() => ({ ok: true }) as DiscardReply);
    }
  };
  work()
    .then(sendResponse)
    .catch((err: unknown) => {
      // A reply shaped for either caller: the popup reads `message` when it
      // asked to prepare and `failure` when it asked to save.
      const message = err instanceof Error ? err.message : String(err);
      sendResponse({ ok: false, message, failure: { kind: "bad_response", message, offerOptions: false } });
    });
  return true;
});

chrome.tabs.onUpdated.addListener((tabId, changeInfo, tab) => {
  if (changeInfo.status !== "loading") return;
  // `changeInfo.url` is set only when the URL *changes*, so a reload of a file
  // that failed carries no URL at all. Falling back to the tab's own URL —
  // delivered here on the strength of `file:///*` — is what makes every remedy
  // the popup suggests (start the daemon, fix the address, paste the token,
  // then reload) work in the tab that failed.
  const url = changeInfo.url ?? tab.url;

  // No URL at all means the tab is loading an origin this extension cannot
  // see — Chromium redacts `tab.url` outside `host_permissions`, and neither
  // `file:` nor the daemon is redacted. So this is the ordinary web, and the
  // record from whatever this tab did last should not follow it there.
  if (url === undefined) {
    moveOn(tabId);
    return;
  }

  if (!url.startsWith("file:")) {
    // Our own redirect is not the user leaving: the note we just opened keeps
    // the record saying where it came from. Any other address means this tab
    // has moved on, and its badge and record go with it.
    if (redirected.get(tabId) === url) return;
    moveOn(tabId);
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
  if (pendingTab === tabId) void clearPendingClip().catch(() => undefined);
  if (pendingTab === tabId) pendingTab = null;
  void clearTabStatus(tabId).catch(() => undefined);
});
