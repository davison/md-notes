/**
 * The service worker: what makes the app installable and what makes it open
 * with no daemon behind it.
 *
 * It is built on its own into dist/sw.js (ui/vite.sw.config.ts) and served,
 * like the shell, with `no-cache` — so a new build's worker is noticed on the
 * next navigation rather than up to a day later.
 *
 * The rules it keeps, all of which ui/e2e/pwa.test.mjs holds it to:
 *
 *   - Nothing under /api/ is intercepted. The events stream is not buffered,
 *     a save goes to the network or fails, and a session cookie is never
 *     written to a cache.
 *   - The hashed assets are cache-first. Their names carry their content, so
 *     a hit cannot be stale.
 *   - The shell is network-first with the cache behind it. That is the M4
 *     `no-cache` rule surviving the worker: a rebuilt shell is picked up on
 *     the next load, and the cached copy is only ever what answers when the
 *     network cannot.
 *   - The cache is named for its contents, so a rebuild is a new cache and
 *     activate deletes the old one whole.
 */

import { classify } from "./sw-policy";

/** Written by the build: the cache's name, and what install puts in it. */
declare const __MDN_CACHE__: string;
declare const __MDN_PRECACHE__: string[];

const CACHE = __MDN_CACHE__;
const PRECACHE = __MDN_PRECACHE__;

/** Every cache this worker has ever owned begins with this. */
const PREFIX = "mdn-";

/** The one key every navigation is cached under and read back from. */
const SHELL = "/index.html";

/**
 * `self` is typed as a plain worker scope by TypeScript's WebWorker lib;
 * redeclaring it is an error, so the service worker's own scope is reached
 * through a name of our own.
 */
const sw = self as unknown as ServiceWorkerGlobalScope;

/**
 * The daemon varies its assets on Accept-Encoding, which the Cache API never
 * sees: a stored response says `Vary: Accept-Encoding` and a lookup built
 * from a URL carries no such header, so matching on Vary can miss a copy that
 * is sitting right there. The bodies in the cache are already decoded, so
 * there is only ever one representation of a URL in it and ignoring Vary
 * cannot pick the wrong one.
 */
const MATCH: CacheQueryOptions = { ignoreVary: true };

sw.addEventListener("install", (event) => {
  event.waitUntil(fill());
});

/**
 * Precaches, tolerantly. `cache.addAll` would fail the whole install on one
 * bad response, and over the tailnet one bad response is all it takes: a
 * session that expired between the page load and the install returns the
 * login page for every URL here. A worker that installs with a thin cache
 * still serves the app; a worker that never installs serves nothing.
 *
 * `cache: "reload"` so the HTTP cache cannot hand the install an older copy
 * of the shell than the one the page it came from is running.
 */
async function fill(): Promise<void> {
  const cache = await caches.open(CACHE);
  await Promise.all(
    PRECACHE.map(async (url) => {
      try {
        const res = await fetch(url, { cache: "reload", credentials: "same-origin" });
        if (res.ok) await cache.put(url, res);
      } catch {
        // No network at install time; the runtime fills the cache later.
      }
    }),
  );
}

sw.addEventListener("activate", (event) => {
  event.waitUntil(
    (async () => {
      for (const key of await caches.keys()) {
        if (key.startsWith(PREFIX) && key !== CACHE) await caches.delete(key);
      }
      // Claim the page that registered us, so the first visit is controlled
      // too. This worker never calls skipWaiting: a tab running the previous
      // build keeps the previous worker, which is the only one that still has
      // that build's lazily loaded chunks. The new worker takes over when
      // the last of those tabs has gone.
      await sw.clients.claim();
    })(),
  );
});

sw.addEventListener("fetch", (event) => {
  const url = new URL(event.request.url);
  const route = classify({
    method: event.request.method,
    mode: event.request.mode,
    sameOrigin: url.origin === sw.location.origin,
    pathname: url.pathname,
  });
  // No respondWith at all: the request goes to the network exactly as it
  // would if no worker were installed.
  if (route === "network") return;
  if (route === "asset") {
    event.respondWith(cacheFirst(event.request));
    return;
  }
  event.respondWith(networkFirst(event.request, route === "shell"));
});

async function cacheFirst(request: Request): Promise<Response> {
  const cache = await caches.open(CACHE);
  const hit = await cache.match(request, MATCH);
  if (hit) return hit;
  const res = await fetch(request);
  if (res.ok) await cache.put(request, res.clone());
  return res;
}

async function networkFirst(request: Request, navigation: boolean): Promise<Response> {
  const cache = await caches.open(CACHE);
  const key = navigation ? SHELL : request;
  try {
    const res = await fetch(request);
    // Only a 200. Over the tailnet an unauthenticated navigation answers the
    // login page under a 401, and caching that would be caching a lockout.
    if (res.ok) await cache.put(key, res.clone());
    return res;
  } catch {
    const hit = await cache.match(key, MATCH);
    if (hit) return hit;
    if (navigation) return unreachable();
    return new Response("the daemon is unreachable", {
      status: 503,
      headers: { "Content-Type": "text/plain; charset=utf-8" },
    });
  }
}

/**
 * What an installed app opens to when it has never cached a shell and the
 * daemon is not there — the only case the cached shell does not cover. It
 * says which daemon and why, in the app's own two palettes, rather than
 * leaving the browser to say the site cannot be reached.
 */
function unreachable(): Response {
  const body = `<!doctype html>
<html lang="en"><head><meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1.0">
<title>MD Notes is unreachable</title>
<style>
:root { color-scheme: light dark; --bg: #fbfbfa; --fg: #1f1f1f; --muted: #6b6b6b }
@media (prefers-color-scheme: dark) { :root { --bg: #1b1b1b; --fg: #e6e6e3; --muted: #9a9a95 } }
body { background: var(--bg); color: var(--fg); margin: 0; padding: 2rem 1.25rem;
  font: 1rem/1.5 system-ui, sans-serif }
main { max-width: 32rem; margin: 0 auto }
h1 { font-size: 1.4rem; margin: 0 0 1rem }
p { color: var(--muted) }
button { font: inherit; padding: 0.6rem 1rem; min-height: 40px; border-radius: 6px;
  border: 1px solid var(--muted); background: transparent; color: var(--fg) }
</style></head>
<body><main>
<h1>The daemon is unreachable</h1>
<p>mdn is not answering on this device. It may be stopped, or this device may
be off the network that reaches it. Your notes are on the machine running the
daemon; nothing here is lost.</p>
<button onclick="location.reload()">Try again</button>
</main></body></html>`;
  return new Response(body, {
    status: 503,
    headers: { "Content-Type": "text/html; charset=utf-8", "Cache-Control": "no-store" },
  });
}
