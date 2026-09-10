/** The daemon's roots API, as the extension sees it. */

import type { Root } from "./paths";
import { trimSlash } from "./paths";
import type { Settings } from "./settings";

/** Why a call failed, in terms the popup can explain to the user. */
export type FailureKind =
  | "unreachable"
  | "origin_refused"
  | "token_rejected"
  | "refused"
  | "bad_response";

export class DaemonError extends Error {
  readonly kind: FailureKind;
  readonly status: number | undefined;

  constructor(kind: FailureKind, message: string, status?: number) {
    super(message);
    this.name = "DaemonError";
    this.kind = kind;
    this.status = status;
  }
}

/** The `fetch` the calls use; injected so the tests need no network. */
export type Fetch = typeof fetch;

function headers(settings: Settings, json: boolean): Record<string, string> {
  const h: Record<string, string> = {};
  if (json) h["Content-Type"] = "application/json";
  // The daemon's Origin guard refuses a chrome-extension origin unless the
  // request carries the installation's bearer token.
  if (settings.token !== "") h["Authorization"] = `Bearer ${settings.token}`;
  return h;
}

async function daemonMessage(res: Response): Promise<string> {
  try {
    const body = (await res.json()) as { error?: unknown };
    if (typeof body.error === "string" && body.error !== "") return body.error;
  } catch {
    // not JSON; fall through to the status line
  }
  return `${res.status} ${res.statusText}`.trim();
}

async function failureFor(res: Response, settings: Settings): Promise<DaemonError> {
  const message = await daemonMessage(res);
  if (res.status === 401) {
    return new DaemonError(
      "token_rejected",
      `the daemon rejected the token (${message}) — check it against \`mdn token\``,
      res.status,
    );
  }
  if (res.status === 403) {
    const hint =
      settings.token === ""
        ? "no token is stored — run `mdn token` and paste it on the options page"
        : "the daemon refused the extension's origin even with a token — is it up to date?";
    return new DaemonError("origin_refused", `${message}: ${hint}`, res.status);
  }
  return new DaemonError("refused", message, res.status);
}

async function call(
  settings: Settings,
  path: string,
  init: RequestInit,
  doFetch: Fetch,
): Promise<unknown> {
  let res: Response;
  try {
    res = await doFetch(`${trimSlash(settings.daemonUrl)}${path}`, init);
  } catch (err) {
    throw new DaemonError(
      "unreachable",
      `daemon not reachable at ${trimSlash(settings.daemonUrl)} (${String(err)})`,
    );
  }
  if (!res.ok) throw await failureFor(res, settings);
  try {
    return await res.json();
  } catch {
    throw new DaemonError("bad_response", "the daemon's reply was not JSON");
  }
}

function asRoot(value: unknown): Root {
  const r = value as Partial<Root> | null;
  if (
    r === null ||
    typeof r !== "object" ||
    typeof r.slug !== "string" ||
    typeof r.path !== "string"
  ) {
    throw new DaemonError("bad_response", "the daemon returned a root without a slug and path");
  }
  return { slug: r.slug, path: r.path, kind: r.kind === "notes" ? "notes" : "recent" };
}

/**
 * The roots the daemon serves. A GET from the extension's background context
 * carries no Origin header, so this call needs no token.
 */
export async function listRoots(settings: Settings, doFetch: Fetch = fetch): Promise<Root[]> {
  const body = await call(settings, "/api/roots", { method: "GET" }, doFetch);
  const roots = (body as { roots?: unknown }).roots;
  if (!Array.isArray(roots)) {
    throw new DaemonError("bad_response", "the daemon returned no roots list");
  }
  return roots.map(asRoot);
}

/**
 * Registers an absolute directory as a root and returns it, or the existing
 * root when the daemon already serves that real path. The POST carries the
 * extension's Origin, so it needs the token.
 */
export async function registerRoot(
  settings: Settings,
  dir: string,
  doFetch: Fetch = fetch,
): Promise<Root> {
  const body = await call(
    settings,
    "/api/roots",
    { method: "POST", headers: headers(settings, true), body: JSON.stringify({ path: dir }) },
    doFetch,
  );
  return asRoot(body);
}
