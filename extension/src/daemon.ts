/** The daemon's API, as the extension sees it. */

import type { ClipKind } from "./extraction";
import type { Root } from "./paths";
import { trimSlash } from "./paths";
import { presentsBearer } from "./reach";
import type { Settings } from "./settings";

/** Why a call failed, in terms the popup can explain to the user. */
export type FailureKind =
  | "unreachable"
  | "no_token"
  | "origin_refused"
  | "token_rejected"
  /** The tailnet allow-list: this endpoint is served on loopback only. */
  | "loopback_only"
  /** The daemon does not answer to the name the request arrived under. */
  | "bad_host"
  /** The daemon could not find the note a registration was to be verified against. */
  | "not_found"
  | "refused"
  | "bad_response";

export class DaemonError extends Error {
  readonly kind: FailureKind;
  readonly status: number | undefined;
  /**
   * The daemon's own words, without the hint this extension wraps them in —
   * so a caller can quote the daemon rather than unpick a sentence.
   */
  readonly detail: string;

  constructor(kind: FailureKind, message: string, status?: number, detail?: string) {
    super(message);
    this.name = "DaemonError";
    this.kind = kind;
    this.status = status;
    this.detail = detail ?? message;
  }
}

/** The `fetch` the calls use; injected so the tests need no network. */
export type Fetch = typeof fetch;

export interface CallOptions {
  /**
   * Insist on the bearer token. A GET without it carries no `Origin` header
   * from the extension's background context and passes the daemon's guard
   * unauthenticated, which is what the intercept relies on over loopback;
   * adding the header makes the request one the daemon must accept on the
   * token's merit, which is what the options page's connection test wants.
   *
   * A GET to a daemon that is *not* on loopback carries the token whether or
   * not this is set — under `tailnet_host` nothing at all is served without
   * it — so this only ever adds the header, never removes one. See
   * `presentsBearer` in `reach.ts`.
   */
  authenticated?: boolean;
  fetch?: Fetch;
}

function headers(settings: Settings, json: boolean, authenticated: boolean): Record<string, string> {
  const h: Record<string, string> = {};
  if (json) h["Content-Type"] = "application/json";
  // The daemon's Origin guard refuses a chrome-extension origin unless the
  // request carries the installation's bearer token.
  if (authenticated && settings.token !== "") h["Authorization"] = `Bearer ${settings.token}`;
  return h;
}

/**
 * A refusal as the daemon words it: its `{code, error}` envelope, falling back
 * to the status line for a body that is not one. The code is what a client is
 * meant to branch on — `unauthorized` and `cross_origin` are told apart there
 * rather than by status alone.
 */
async function refusal(res: Response): Promise<{ code: string; message: string }> {
  try {
    const body = (await res.json()) as { error?: unknown; code?: unknown };
    return {
      code: typeof body.code === "string" ? body.code : "",
      message:
        typeof body.error === "string" && body.error !== ""
          ? body.error
          : `${res.status} ${res.statusText}`.trim(),
    };
  } catch {
    // not JSON; the status line is all there is
    return { code: "", message: `${res.status} ${res.statusText}`.trim() };
  }
}

async function failureFor(res: Response, settings: Settings): Promise<DaemonError> {
  const { code, message } = await refusal(res);
  if (code === "unauthorized" || res.status === 401) {
    // A daemon that answers 401 wanted a token. Which complaint that is
    // depends on whether there was one to send: with nothing stored the
    // token was never judged, and "the daemon rejected the token" would be
    // a lie — the one a tailnet daemon URL used to tell on every call.
    if (settings.token === "") {
      return new DaemonError(
        "no_token",
        `the daemon serves nothing without a token (${message}) — paste the one \`mdn token\` prints`,
        res.status,
        message,
      );
    }
    return new DaemonError(
      "token_rejected",
      `the daemon rejected the token (${message}) — check it against \`mdn token\``,
      res.status,
      message,
    );
  }
  // The tailnet allow-list, and a name the daemon does not answer to. Both
  // are refusals of the *endpoint* or the *address*, and reading either as a
  // token problem sends the user to fix something that is not broken.
  // A registration refused because the note named with it is not there
  // (M7-R1). It is not a refusal of the endpoint, the origin or the token,
  // and the caller has to be able to say which it is: this is the one
  // failure whose remedy is the file, not the setup.
  if (code === "not_found") return new DaemonError("not_found", message, res.status, message);
  if (code === "loopback_only") return new DaemonError("loopback_only", message, res.status, message);
  if (code === "bad_host") return new DaemonError("bad_host", message, res.status, message);
  if (code === "cross_origin" || (res.status === 403 && code === "")) {
    const hint =
      settings.token === ""
        ? "no token is stored — run `mdn token` and paste it on the options page"
        : "the daemon refused the extension's origin even with a token — is it up to date?";
    return new DaemonError("origin_refused", `${message}: ${hint}`, res.status, message);
  }
  return new DaemonError("refused", message, res.status, message);
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
 * The roots the daemon serves.
 *
 * A GET from the extension's background context carries no Origin header, so
 * against a daemon on this machine it needs no token and is sent without one:
 * the file-URL intercept works for someone who has pasted none. Against a
 * daemon that is not on loopback the token goes with it, because under
 * `tailnet_host` an unauthenticated read is a 401 and nothing downstream ever
 * gets to decide anything. `authenticated` forces the header on either.
 */
export async function listRoots(settings: Settings, options: CallOptions = {}): Promise<Root[]> {
  const init: RequestInit = { method: "GET" };
  if (presentsBearer(settings, options.authenticated === true)) {
    init.headers = headers(settings, false, true);
  }
  const body = await call(settings, "/api/roots", init, options.fetch ?? fetch);
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
 *
 * `file` is the note the registration is for — a path inside `dir`, which
 * for this extension is a base name — and the daemon registers nothing
 * unless it is there, answering `404 not_found` instead. That is what stops
 * a `file:` URL for a note that does not exist from leaving its directory
 * registered for ever
 * ([#50](https://github.com/davison/md-notes/issues/50)); the check is the
 * daemon's, so there is no window in which the root exists and this
 * extension still has to undo it. An empty `file` sends no field at all and
 * is the registration `mdn open` makes.
 */
export async function registerRoot(
  settings: Settings,
  dir: string,
  file: string,
  options: CallOptions = {},
): Promise<Root> {
  const payload: { path: string; file?: string } = { path: dir };
  if (file !== "") payload.file = file;
  const body = await call(
    settings,
    "/api/roots",
    {
      method: "POST",
      headers: headers(settings, true, true),
      body: JSON.stringify(payload),
    },
    options.fetch ?? fetch,
  );
  return asRoot(body);
}

/** The body of `POST /api/clip`, as the daemon documents it. */
export interface ClipRequest {
  url: string;
  title: string;
  markdown: string;
  kind: ClipKind;
}

/** Where the clip landed: the notes root's slug, and the path inside it. */
export interface ClipResult {
  root: string;
  path: string;
}

/**
 * Creates a note from a clip and returns where it landed.
 *
 * Made from the service worker, which owns the clip and outlives the popup.
 * An extension page could issue the same request — CORS exemption follows
 * `host_permissions` for every extension context — but a popup closes the
 * moment it loses focus and would take the save with it. A content script or
 * a web page is a different matter: those are governed by CORS, and the
 * daemon answers no preflight, which is the boundary it draws.
 *
 * A missing token is reported here rather than sent, because the refusal it
 * would earn (`cross_origin`) reads like an origin problem and is not one.
 */
export async function postClip(
  settings: Settings,
  clip: ClipRequest,
  options: CallOptions = {},
): Promise<ClipResult> {
  if (settings.token === "") {
    throw new DaemonError("no_token", "no token is configured for this daemon");
  }
  const body = await call(
    settings,
    "/api/clip",
    {
      method: "POST",
      headers: headers(settings, true, true),
      body: JSON.stringify(clip),
    },
    options.fetch ?? fetch,
  );
  const result = body as Partial<ClipResult> | null;
  if (
    result === null ||
    typeof result !== "object" ||
    typeof result.root !== "string" ||
    typeof result.path !== "string"
  ) {
    throw new DaemonError("bad_response", "the daemon did not say where the clip landed");
  }
  return { root: result.root, path: result.path };
}
