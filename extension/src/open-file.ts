/**
 * Deciding what a `file:` navigation should become. Kept free of the `chrome`
 * APIs so it can be tested directly.
 */

import { DaemonError, listRoots, registerRoot, type FailureKind } from "./daemon";
import { localMarkdownFile, noteUrl, relativeTo, rootContaining, type Root } from "./paths";
import type { Settings } from "./settings";

/** What the background worker should do about a navigation. */
export type OpenResult =
  | { status: "ignored" }
  | { status: "open"; url: string; slug: string; note: string }
  | { status: "failed"; kind: FailureKind; message: string };

/** The daemon calls the decision needs; injected so the tests need no network. */
export interface RootsApi {
  listRoots(settings: Settings): Promise<Root[]>;
  registerRoot(settings: Settings, dir: string): Promise<Root>;
}

export const liveRootsApi: RootsApi = {
  listRoots: (settings) => listRoots(settings),
  registerRoot: (settings, dir) => registerRoot(settings, dir),
};

/**
 * Where a `file:` URL should open in the app.
 *
 * A file already inside a registered root opens under that root. Otherwise the
 * file's directory is registered as a new root first — which is also how a
 * symlinked alias of an already registered directory finds its root, since the
 * daemon compares real paths and hands back the existing one.
 */
export async function resolveOpen(
  url: string,
  settings: Settings,
  api: RootsApi = liveRootsApi,
): Promise<OpenResult> {
  const file = localMarkdownFile(url);
  if (file === null) return { status: "ignored" };

  try {
    const roots = await api.listRoots(settings);
    const existing = rootContaining(roots, file.path);
    if (existing !== null) {
      const note = relativeTo(existing.path, file.path);
      return {
        status: "open",
        url: noteUrl(settings.daemonUrl, existing.slug, note),
        slug: existing.slug,
        note,
      };
    }
    const added = await api.registerRoot(settings, file.dir);
    const note = relativeTo(added.path, file.path);
    return {
      status: "open",
      url: noteUrl(settings.daemonUrl, added.slug, note),
      slug: added.slug,
      note,
    };
  } catch (err) {
    if (err instanceof DaemonError) {
      return { status: "failed", kind: err.kind, message: err.message };
    }
    return { status: "failed", kind: "bad_response", message: String(err) };
  }
}
