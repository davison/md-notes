/**
 * Registering the service worker (ui/src/sw.ts).
 *
 * After `load` rather than during it: registration competes with the first
 * paint for the same connection, and the worker matters to the *next* visit,
 * never to this one. Failure is swallowed — a browser with workers disabled,
 * a private window, an http: origin that is not localhost — because every one
 * of those is a browser the app still works in.
 */
export function registerServiceWorker(
  nav: Navigator = navigator,
  win: Window = window,
  path = "/sw.js",
): void {
  if (!("serviceWorker" in nav)) return;
  win.addEventListener("load", () => {
    void nav.serviceWorker.register(path).catch(() => {
      // Not installable here; the app is unaffected.
    });
  });
}
