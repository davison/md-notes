/** The options page: which daemon, and the token to present to it. */

import { DaemonError, listRoots } from "./daemon";
import {
  TAILNET_LIMITS,
  badHostMessage,
  daemonHost,
  isLoopbackUrl,
} from "./reach";
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

  if (!isLoopbackUrl(settings.daemonUrl)) {
    await testRemote(settings);
    return;
  }

  // Two questions with different answers: an unauthenticated read says
  // whether the daemon is there at all, and only a read carrying the token
  // says whether the token is any good.
  let roots;
  try {
    roots = await listRoots(settings);
  } catch (err) {
    say(failureText(err, settings), "error");
    return;
  }
  const reached = countRoots(roots.length);

  if (settings.token === "") {
    say(`${reached}; no token stored, so registering a folder or clipping will be refused`, "ok");
    return;
  }
  const judging = await checksTokens(settings);
  if (judging === "ignores") {
    say(`${reached}; this daemon does not check tokens yet, so yours was not verified`, "ok");
    return;
  }
  if (judging !== "checks") {
    say(`${reached}; could not tell whether it checks tokens (${judging.unknown})`, "ok");
    return;
  }
  try {
    await listRoots(settings, { authenticated: true });
    say(`${reached}, and the token was accepted`, "ok");
  } catch (err) {
    say(`${reached}, but ${failureText(err, settings)}`, "error");
  }
}

/**
 * The same button against a daemon that is not on this machine.
 *
 * The loopback sequence above cannot tell the truth here, and used to tell a
 * specific lie: its first question is an unauthenticated read, which under
 * `tailnet_host` is a 401 whatever the token is, so the button reported "the
 * daemon rejected the token" for a perfectly good one and never reached the
 * read that would have said so ([#51](https://github.com/davison/md-notes/issues/51)).
 *
 * So the authenticated read comes first and is the only one: it is the single
 * question such a daemon can answer. The "does this daemon judge tokens?"
 * probe is dropped with it — a daemon serving nothing unauthenticated has
 * demonstrably answered that already — and the success says what the tailnet
 * allow-list leaves working, because a bare "the token was accepted" here is
 * true and still misleading.
 */
async function testRemote(settings: { daemonUrl: string; token: string }) {
  if (settings.token === "") {
    say(
      `no token stored, and a daemon reached at ${daemonHost(settings.daemonUrl)} serves ` +
        "nothing without one — paste the token `mdn token` prints",
      "error",
    );
    return;
  }
  let roots;
  try {
    roots = await listRoots(settings, { authenticated: true });
  } catch (err) {
    say(failureText(err, settings), "error");
    return;
  }
  say(`${countRoots(roots.length)}, and the token was accepted; ${TAILNET_LIMITS}`, "ok");
}

function countRoots(n: number): string {
  return `daemon answered: ${n} root${n === 1 ? "" : "s"}`;
}

/**
 * A failure in the words the surface needs. `bad_host` is the one the daemon
 * cannot phrase usefully by itself: "unexpected Host header" is true and says
 * nothing about the `tailnet_host` that has to match this URL.
 */
function failureText(err: unknown, settings: { daemonUrl: string }): string {
  if (err instanceof DaemonError && err.kind === "bad_host") {
    return badHostMessage(settings.daemonUrl, err.detail);
  }
  return err instanceof Error ? err.message : String(err);
}

/**
 * Whether this daemon judges bearer tokens at all, asked by presenting one it
 * cannot have issued. A daemon that predates M3-R1 ignores the header and
 * answers 200, and reporting "the token was accepted" on the strength of that
 * would be a lie — the whole point of the button is to catch a bad token.
 *
 * A third answer is possible and is worth keeping apart from the second: any
 * other refusal, or a request that never arrived, means the question went
 * unanswered rather than answered "no".
 */
type TokenJudgement = "checks" | "ignores" | { unknown: string };

async function checksTokens(settings: { daemonUrl: string }): Promise<TokenJudgement> {
  const sentinel = { daemonUrl: settings.daemonUrl, token: `not-a-token-${crypto.randomUUID()}` };
  try {
    await listRoots(sentinel, { authenticated: true });
    return "ignores";
  } catch (err) {
    if (err instanceof DaemonError && err.kind === "token_rejected") return "checks";
    return { unknown: err instanceof Error ? err.message : String(err) };
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
