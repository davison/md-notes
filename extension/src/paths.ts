/** Pure mapping between local file paths, `file:` URLs and app URLs. */

/** A root as the daemon reports it from `GET /api/roots`. */
export interface Root {
  slug: string;
  path: string;
  kind: "notes" | "recent";
}

/** A local markdown file the extension is willing to open in the app. */
export interface LocalFile {
  /** Absolute path of the file. */
  path: string;
  /** Absolute path of the directory holding it. */
  dir: string;
  /** The file's base name. */
  name: string;
}

const MARKDOWN_SUFFIXES = [".md", ".markdown"];

/**
 * True for a path whose base name is a markdown file the daemon serves as a
 * note. A bare `.md` with no stem is not one: it is a dotfile the navigator
 * would not show either.
 */
export function isMarkdownPath(path: string): boolean {
  const name = path.slice(path.lastIndexOf("/") + 1).toLowerCase();
  return MARKDOWN_SUFFIXES.some((s) => name.endsWith(s) && name.length > s.length);
}

/**
 * The absolute local path a `file:` URL names, or null when the URL is not one
 * this extension can map. Only local files are mappable: a `file:` URL with a
 * host names a share on another machine, which is not a path the daemon could
 * register.
 */
export function fileUrlToPath(url: string): string | null {
  let parsed: URL;
  try {
    parsed = new URL(url);
  } catch {
    return null;
  }
  if (parsed.protocol !== "file:") return null;
  if (parsed.host !== "" && parsed.host !== "localhost") return null;
  let path: string;
  try {
    path = decodeURIComponent(parsed.pathname);
  } catch {
    // A stray percent that is not a valid escape.
    return null;
  }
  if (!path.startsWith("/")) return null;
  if (path.includes("\0")) return null;
  return path;
}

/** Splits an absolute path into the directory and base name. */
export function splitPath(path: string): { dir: string; name: string } {
  const cut = path.lastIndexOf("/");
  const dir = cut <= 0 ? "/" : path.slice(0, cut);
  return { dir, name: path.slice(cut + 1) };
}

/**
 * The local markdown file a navigation names, or null when the extension should
 * leave the navigation alone. A query or fragment is not part of a local file's
 * identity, so it is dropped rather than refused.
 */
export function localMarkdownFile(url: string): LocalFile | null {
  const path = fileUrlToPath(url);
  if (path === null || !isMarkdownPath(path)) return null;
  const { dir, name } = splitPath(path);
  if (name === "") return null;
  return { path, dir, name };
}

/** True when `path` is `dir` itself or lies beneath it, on a path boundary. */
export function isUnder(dir: string, path: string): boolean {
  if (path === dir) return true;
  const prefix = dir.endsWith("/") ? dir : dir + "/";
  return path.startsWith(prefix);
}

/** The path of `path` relative to the directory `dir` that contains it. */
export function relativeTo(dir: string, path: string): string {
  const prefix = dir.endsWith("/") ? dir : dir + "/";
  return path.startsWith(prefix) ? path.slice(prefix.length) : path;
}

/**
 * The registered root that already contains `path`, most specific first. Two
 * roots can overlap — a directory inside a root may be registered as a root of
 * its own — and the deepest one is the one whose navigator shows the file in
 * the least surprising place.
 */
export function rootContaining(roots: readonly Root[], path: string): Root | null {
  let best: Root | null = null;
  for (const root of roots) {
    if (!isUnder(root.path, path)) continue;
    if (best === null || root.path.length > best.path.length) best = root;
  }
  return best;
}

/** Encodes each segment of a relative path, keeping the slashes. */
export function encodePath(path: string): string {
  return path.split("/").map(encodeURIComponent).join("/");
}

/** Drops a trailing slash so a base URL concatenates cleanly. */
export function trimSlash(url: string): string {
  return url.replace(/\/+$/, "");
}

/** In-app URL of a note under a root, on the given daemon. */
export function noteUrl(daemonUrl: string, slug: string, path: string): string {
  return `${trimSlash(daemonUrl)}/r/${encodeURIComponent(slug)}/${encodePath(path)}`;
}
