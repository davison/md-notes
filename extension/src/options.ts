/** The options page: which daemon, and the token to present to it. */

import { DaemonError, listRoots } from "./daemon";
import {
  DEFAULT_DAEMON_URL,
  loadSettings,
  normaliseDaemonUrl,
  originPattern,
  saveSettings,
} from "./settings";

function el<T extends HTMLElement>(id: string): T {
  const node = document.getElementById(id);
  if (node === null) throw new Error(`missing element #${id}`);
  return node as T;
}

const urlInput = el<HTMLInputElement>("daemon-url");
const tokenInput = el<HTMLInputElement>("token");
const reveal = el<HTMLInputElement>("reveal");
const status = el<HTMLParagraphElement>("status");
const fileAccess = el<HTMLParagraphElement>("file-access");

function say(text: string, kind: "" | "ok" | "error" = "") {
  status.textContent = text;
  status.className = `status ${kind}`.trimEnd();
}

/**
 * Makes sure the browser will let the extension talk to this daemon. The
 * manifest only grants the default loopback origin, so anything else is an
 * optional permission the user grants from this click.
 */
async function ensureHostPermission(daemonUrl: string): Promise<boolean> {
  const origins = [originPattern(daemonUrl)];
  if (await chrome.permissions.contains({ origins })) return true;
  return chrome.permissions.request({ origins });
}

async function currentSettings() {
  const daemonUrl = normaliseDaemonUrl(urlInput.value);
  return { daemonUrl, token: tokenInput.value.trim() };
}

async function save() {
  let settings;
  try {
    settings = await currentSettings();
  } catch (err) {
    say(String(err instanceof Error ? err.message : err), "error");
    return;
  }
  if (!(await ensureHostPermission(settings.daemonUrl))) {
    say(`the browser did not grant access to ${settings.daemonUrl}; nothing saved`, "error");
    return;
  }
  await saveSettings(settings);
  urlInput.value = settings.daemonUrl;
  say("saved", "ok");
}

async function test() {
  let settings;
  try {
    settings = await currentSettings();
  } catch (err) {
    say(String(err instanceof Error ? err.message : err), "error");
    return;
  }
  if (!(await chrome.permissions.contains({ origins: [originPattern(settings.daemonUrl)] }))) {
    say("save first: the browser has not granted access to that address yet", "error");
    return;
  }
  say("testing…");

  // Two questions with different answers: an unauthenticated read says
  // whether the daemon is there at all, and only a read carrying the token
  // says whether the token is any good.
  let roots;
  try {
    roots = await listRoots(settings);
  } catch (err) {
    say(err instanceof Error ? err.message : String(err), "error");
    return;
  }
  const reached = `daemon answered: ${roots.length} root${roots.length === 1 ? "" : "s"}`;

  if (settings.token === "") {
    say(`${reached}; no token stored, so registering a folder or clipping will be refused`, "ok");
    return;
  }
  if (!(await checksTokens(settings))) {
    say(`${reached}; this daemon does not check tokens yet, so yours was not verified`, "ok");
    return;
  }
  try {
    await listRoots(settings, { authenticated: true });
    say(`${reached}, and the token was accepted`, "ok");
  } catch (err) {
    say(`${reached}, but ${err instanceof Error ? err.message : String(err)}`, "error");
  }
}

/**
 * Whether this daemon judges bearer tokens at all, asked by presenting one it
 * cannot have issued. A daemon that predates M3-R1 ignores the header and
 * answers 200, and reporting "the token was accepted" on the strength of that
 * would be a lie — the whole point of the button is to catch a bad token.
 */
async function checksTokens(settings: { daemonUrl: string }): Promise<boolean> {
  const sentinel = { daemonUrl: settings.daemonUrl, token: `not-a-token-${crypto.randomUUID()}` };
  try {
    await listRoots(sentinel, { authenticated: true });
    return false;
  } catch (err) {
    return err instanceof DaemonError && err.kind === "token_rejected";
  }
}

async function describeFileAccess() {
  const allowed = await new Promise<boolean>((resolve) =>
    chrome.extension.isAllowedFileSchemeAccess(resolve),
  );
  fileAccess.textContent = allowed
    ? "Enabled. Opening a local .md or .markdown file redirects it into the app."
    : 'Off. Local markdown files are left alone until "Allow access to file URLs" is switched on for this extension on the browser\'s extensions page.';
}

async function init() {
  const settings = await loadSettings();
  urlInput.value = settings.daemonUrl;
  urlInput.placeholder = DEFAULT_DAEMON_URL;
  tokenInput.value = settings.token;
  reveal.addEventListener("change", () => {
    tokenInput.type = reveal.checked ? "text" : "password";
  });
  el<HTMLButtonElement>("save").addEventListener("click", () => void save());
  el<HTMLButtonElement>("test").addEventListener("click", () => void test());
  await describeFileAccess();
}

void init();
