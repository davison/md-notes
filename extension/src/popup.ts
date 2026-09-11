/**
 * The toolbar popup: clip this page or this selection, review the title, save.
 *
 * It never talks to the daemon itself — see `messages.ts` for why — and it
 * never touches the page: both are the service worker's, and the popup is the
 * face on them. It also still reports what the file-URL intercept last did in
 * this tab.
 */

import { ask, type DiscardReply, type PrepareReply, type SaveReply } from "./messages";
import { getPendingClip, type PendingClip } from "./pending";
import { loadSettings } from "./settings";
import { getTabStatus } from "./status";

function el<T extends HTMLElement>(id: string): T {
  const node = document.getElementById(id);
  if (node === null) throw new Error(`missing element #${id}`);
  return node as T;
}

const actions = el("clip-actions");
const clipSection = el("clip");
const kindLine = el("clip-kind");
const titleInput = el<HTMLInputElement>("clip-title");
const sourceLine = el("clip-source");
const clipStatus = el("clip-status");
const links = el("clip-links");
const openLink = el<HTMLAnchorElement>("clip-open");
const optionsLink = el<HTMLAnchorElement>("clip-options");

function say(text: string, kind: "" | "ok" | "error" = "") {
  clipStatus.textContent = text;
  clipStatus.className = `status ${kind}`.trimEnd();
}

/**
 * The tab this popup is acting on. A real toolbar popup is opened over the
 * active tab; opened as an ordinary page — which is how the end-to-end tests
 * drive it, since a browser action popup cannot be clicked from a test — it
 * takes the tab from `?tab=`. Nothing is reachable that way that clicking the
 * toolbar button would not also reach: the popup is not web-accessible, so
 * only the extension and the user can open it at all.
 */
async function targetTab(): Promise<number | null> {
  const asked = new URLSearchParams(location.search).get("tab");
  if (asked !== null && /^\d+$/.test(asked)) return Number(asked);
  const [tab] = await chrome.tabs.query({ active: true, currentWindow: true });
  return tab?.id ?? null;
}

function showClip(clip: PendingClip) {
  kindLine.textContent = clip.kind === "selection" ? "Selection clip" : "Page clip";
  titleInput.value = clip.title;
  sourceLine.textContent = clip.url;
  sourceLine.title = clip.url;
  clipSection.hidden = false;
  actions.hidden = true;
  showLinks({});
  titleInput.focus();
  titleInput.select();
}

function hideClip() {
  clipSection.hidden = true;
  actions.hidden = false;
}

function busy(on: boolean) {
  for (const id of ["clip-page", "clip-selection", "clip-save", "clip-discard"]) {
    el<HTMLButtonElement>(id).disabled = on;
  }
}

async function prepare(kind: "page" | "selection") {
  const tabId = await targetTab();
  if (tabId === null) {
    say("There is no page here to clip.", "error");
    return;
  }
  busy(true);
  say("Reading the page…");
  try {
    const reply = await ask<PrepareReply>({ type: "clip:prepare", tabId, kind });
    if (!reply.ok) {
      say(reply.message, "error");
      return;
    }
    say("");
    showClip(reply.clip);
  } catch (err) {
    // The worker threw outside its handler, or the port closed under us.
    // Without this the status would sit on "Reading the page…" for ever.
    say(workerFailure(err), "error");
  } finally {
    busy(false);
  }
}

/** A worker that did not answer, in words the user can act on. */
function workerFailure(err: unknown): string {
  const message = err instanceof Error ? err.message : String(err);
  return `The extension's background worker did not answer (${message}). Reload the page and try again.`;
}

async function save() {
  busy(true);
  say("Saving…");
  try {
    const reply = await ask<SaveReply>({ type: "clip:save", title: titleInput.value });
    if (reply.ok) {
      hideClip();
      say(`Saved to ${reply.path}`, "ok");
      showLinks({ note: reply.url });
      return;
    }
    say(reply.failure.message, "error");
    // A failure the user can act on is acted on in the options page, so the
    // way there is offered rather than described.
    showLinks({ options: reply.failure.offerOptions });
  } catch (err) {
    // The clip is still held by the worker, so Save can be pressed again.
    say(workerFailure(err), "error");
  } finally {
    busy(false);
  }
}

/** Which of the two links under the message are offered, if either. */
function showLinks(which: { note?: string; options?: boolean }) {
  if (which.note !== undefined) openLink.href = which.note;
  openLink.hidden = which.note === undefined;
  optionsLink.hidden = which.options !== true;
  links.hidden = openLink.hidden && optionsLink.hidden;
}

async function discard() {
  try {
    await ask<DiscardReply>({ type: "clip:discard" });
  } catch (err) {
    say(workerFailure(err), "error");
    return;
  }
  hideClip();
  say("");
  showLinks({});
}

/**
 * The buttons are wired as this module runs, before anything is awaited: a
 * popup is clicked the instant it appears, and a listener attached after two
 * round trips to storage would miss that click.
 */
el<HTMLButtonElement>("clip-page").addEventListener("click", () => void prepare("page"));
el<HTMLButtonElement>("clip-selection").addEventListener("click", () => void prepare("selection"));
el<HTMLButtonElement>("clip-save").addEventListener("click", () => void save());
el<HTMLButtonElement>("clip-discard").addEventListener("click", () => void discard());
el<HTMLButtonElement>("options").addEventListener("click", () => {
  void chrome.runtime.openOptionsPage();
});
optionsLink.addEventListener("click", (event) => {
  event.preventDefault();
  void chrome.runtime.openOptionsPage();
});

async function init() {
  const settings = await loadSettings();
  el("daemon").textContent = `Daemon: ${settings.daemonUrl}`;

  const status = el("status");
  const tabId = await targetTab();
  const record = tabId === null ? null : await getTabStatus(tabId);
  if (record === null) {
    status.textContent = "Nothing to report for this tab.";
    status.className = "status";
  } else {
    status.textContent = record.message;
    status.className = `status ${record.kind === "opened" ? "ok" : "error"}`;
  }

  // A clip taken from the context menu, waiting for this popup to open.
  const pending = await getPendingClip();
  if (pending !== null && clipSection.hidden) showClip(pending);
}

void init();
