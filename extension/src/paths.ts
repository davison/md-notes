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
 * The path with `.`, `..` and empty segments resolved away — POSIX lexical
 * normalisation, the same shape `filepath.Clean` produces on the daemon side.
 */
export function normalisePath(path: string): string {
  const out: string[] = [];
  for (const segment of path.split("/")) {
    if (segment === "" || segment === ".") continue;
    if (segment === "..") {
      out.pop();
      continue;
    }
    out.push(segment);
  }
  return "/" + out.join("/");
}

/**
 * The absolute local path a `file:` URL names, or null when the URL is not one
 * this extension can map. Only local files are mappable: a `file:` URL with a
 * host names a share on another machine, which is not a path the daemon could
 * register.
 *
 * This is a security boundary. The path it returns is matched against the
 * daemon's roots and, when no root contains it, its directory is handed to
 * `POST /api/roots` — so a path this function accepts is a directory the
 * daemon can be made to serve. Percent escapes are therefore decoded one
 * segment at a time, and a decoded segment that is a traversal or reintroduces
 * a separator is refused rather than resolved: the URL parser normalises
 * literal and `%2e%2e` dot segments before we see them, but leaves `%2F`
 * alone, and decoding the whole pathname at once turns that back into `../`.
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

  const encoded = parsed.pathname.split("/");
  // A `file:` pathname always begins with a slash, so the first piece is empty.
  if (encoded.length < 2 || encoded[0] !== "") return null;
  const segments: string[] = [];
  for (const piece of encoded.slice(1)) {
    let segment: string;
    try {
      segment = decodeURIComponent(piece);
    } catch {
      // A stray percent that is not a valid escape.
      return null;
    }
    // An empty segment is a `//` run or a trailing slash; `.` and `..` are
    // traversal; a separator or a NUL inside a decoded segment means the
    // escape was hiding structure.
    if (segment === "" || segment === "." || segment === "..") return null;
    if (segment.includes("/") || segment.includes("\\") || segment.includes("\0")) return null;
    segments.push(segment);
  }

  const path = "/" + segments.join("/");
  // Belt and braces: whatever the segment rules missed, the result must
  // already be in normal form, so nothing downstream can be surprised by it.
  if (path !== normalisePath(path)) return null;
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
