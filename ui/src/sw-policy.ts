/**
 * What the service worker does with one request, as a pure function.
 *
 * The worker itself is a browser-only script that no unit test can load
 * (ui/src/sw.ts); this is the part of it worth testing without a browser,
 * and the part whose rules are promises the rest of the application depends
 * on — above all that nothing under /api/ is ever answered from a cache.
 */

/** Where the API lives. The worker never comes between it and the app. */
export const API_PREFIX = "/api/";

/**
 * The tailnet login form. It exists only under the tailnet host, carries
 * `Cache-Control: no-store`, and is the one page whose staleness would lock
 * a reader out of their own notes, so it is left to the network like the API.
 */
export const LOGIN_PATH = "/login";

/** Vite's hashed output directory: immutable by construction. */
export const ASSETS_PREFIX = "/assets/";

export type Route =
  /** Not intercepted at all: no respondWith, so the browser behaves as if there were no worker. */
  | "network"
  /** Cache first. The name carries the content hash, so a hit cannot be stale. */
  | "asset"
  /** Network first, the cached shell as the offline fallback. */
  | "shell"
  /** Network first, the cached copy as the offline fallback. */
  | "static";

export interface RequestFacts {
  method: string;
  /** The request's mode; "navigate" is a document the browser is opening. */
  mode: string;
  sameOrigin: boolean;
  pathname: string;
}

/**
 * The routing table.
 *
 * "network" is a refusal to participate rather than a strategy: the worker
 * returns without calling respondWith, and the request goes to the network
 * exactly as it would with no worker installed. That is what keeps the
 * events stream unbuffered — a worker that passes a stream through
 * respondWith can still hold it — and what keeps a save, a login and a
 * session cookie out of any cache.
 *
 * Everything the worker does answer is GET and same-origin, so no request
 * that changes anything can be replayed from a cache, and no other origin's
 * response is ever stored under this one's name.
 */
export function classify(r: RequestFacts): Route {
  if (r.method !== "GET" || !r.sameOrigin) return "network";
  if (r.pathname === API_PREFIX.slice(0, -1) || r.pathname.startsWith(API_PREFIX)) return "network";
  if (r.pathname === LOGIN_PATH) return "network";
  if (r.pathname.startsWith(ASSETS_PREFIX)) return "asset";
  if (r.mode === "navigate") return "shell";
  return "static";
}
