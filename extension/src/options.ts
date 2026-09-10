/** The options page: which daemon, and the token to present to it. */

import { listRoots } from "./daemon";
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
  try {
    const roots = await listRoots(settings);
    say(`daemon answered: ${roots.length} root${roots.length === 1 ? "" : "s"}`, "ok");
  } catch (err) {
    say(err instanceof Error ? err.message : String(err), "error");
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
