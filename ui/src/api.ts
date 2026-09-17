export type RootKind = "notes" | "recent";

export interface Root {
  slug: string;
  path: string;
  kind: RootKind;
}

/** Builds an Error from a failed response, preferring the daemon's message. */
async function errorFrom(res: Response): Promise<Error> {
  let message = `${res.status} ${res.statusText}`;
  try {
    const body = (await res.json()) as { error?: string };
    if (body.error) message = body.error;
  } catch {
    // not JSON; keep the status text
  }
  return new Error(message);
}

async function request<T>(path: string, init?: RequestInit): Promise<T> {
  const res = await fetch(path, init);
  if (!res.ok) throw await errorFrom(res);
  return (await res.json()) as T;
}

export async function listRoots(): Promise<Root[]> {
  const body = await request<{ roots: Root[] }>("/api/roots");
  return body.roots;
}

/**
 * Unregisters a recent root. The folder and its notes stay on disk; what
 * goes is the daemon's serving of them (M7-R2). Rejects with a SourceError
 * carrying the daemon's code — `notes_root` for the configured notes root,
 * `not_found` for a slug the daemon does not have.
 */
export async function removeRoot(slug: string): Promise<void> {
  const res = await fetch(`/api/roots/${encodeURIComponent(slug)}`, { method: "DELETE" });
  if (!res.ok) throw await sourceErrorFrom(res);
}

export interface TreeNode {
  name: string;
  path: string;
  dir: boolean;
  children?: TreeNode[];
}

export function fetchTree(slug: string): Promise<TreeNode> {
  return request<TreeNode>(`/api/r/${encodeURIComponent(slug)}/tree`);
}

/** URL of a file inside a root, served through confinement. */
export function rawURL(slug: string, path: string): string {
  return `/api/r/${encodeURIComponent(slug)}/raw/${encodePath(path)}`;
}

/** In-app URL of a note. */
export function noteURL(slug: string, path: string): string {
  return `/r/${encodeURIComponent(slug)}/${encodePath(path)}`;
}

/** Encodes each segment of a relative path, keeping the slashes. */
export function encodePath(path: string): string {
  return path.split("/").map(encodeURIComponent).join("/");
}

export async function fetchRaw(slug: string, path: string): Promise<string> {
  const res = await fetch(rawURL(slug, path));
  if (!res.ok) throw await errorFrom(res);
  return res.text();
}

export interface Note {
  path: string;
  title: string;
  frontmatter?: Record<string, unknown>;
  html: string;
}

export function fetchNote(slug: string, path: string): Promise<Note> {
  return request<Note>(`/api/r/${encodeURIComponent(slug)}/note/${encodePath(path)}`);
}

export interface Hit {
  path: string;
  line: number;
  text: string;
  /** [start, end) offsets into text, in string units. */
  matches: [number, number][];
  before?: string;
  after?: string;
}

export interface SearchResult {
  hits: Hit[];
  truncated: boolean;
}

export function searchNotes(slug: string, query: string, signal?: AbortSignal): Promise<SearchResult> {
  const q = encodeURIComponent(query);
  return request<SearchResult>(`/api/r/${encodeURIComponent(slug)}/search?q=${q}`, { signal });
}

export interface Tag {
  name: string;
  count: number;
  notes: string[];
}

export async function fetchTags(slug: string): Promise<Tag[]> {
  const body = await request<{ tags: Tag[] }>(`/api/r/${encodeURIComponent(slug)}/tags`);
  return body.tags;
}

export interface Source {
  source: string;
  revision: string;
}

/** A failed source read or save, carrying the daemon's error code. */
export class SourceError extends Error {
  constructor(
    public status: number,
    public code: string,
    message: string,
  ) {
    super(message);
    this.name = "SourceError";
  }
}

function sourceURL(slug: string, path: string): string {
  return `/api/r/${encodeURIComponent(slug)}/source/${encodePath(path)}`;
}

async function sourceErrorFrom(res: Response): Promise<SourceError> {
  let code = "io_error";
  let message = `${res.status} ${res.statusText}`;
  try {
    const body = (await res.json()) as { code?: string; error?: string };
    if (body.code) code = body.code;
    if (body.error) message = body.error;
  } catch {
    // not JSON; keep the status text
  }
  return new SourceError(res.status, code, message);
}

/** The unmodified markdown source of a note and its current revision. */
export async function fetchSource(slug: string, path: string): Promise<Source> {
  const res = await fetch(sourceURL(slug, path), { cache: "no-store" });
  if (!res.ok) throw await sourceErrorFrom(res);
  return (await res.json()) as Source;
}

/**
 * Saves the complete source of a note against the revision it was read at.
 * Rejects with a SourceError; a "conflict" code means the note changed on
 * disk and the caller must reread before saving again. With keepalive the
 * request outlives the page, for a final flush on unload.
 */
export async function saveSource(
  slug: string,
  path: string,
  source: string,
  revision: string,
  keepalive = false,
): Promise<Source> {
  const res = await fetch(sourceURL(slug, path), {
    method: "PUT",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ source, revision }),
    keepalive,
  });
  if (!res.ok) throw await sourceErrorFrom(res);
  return (await res.json()) as Source;
}

/** A note the daemon has just created. */
export interface CreatedNote {
  root: string;
  /**
   * The daemon's cleaned form of the path, which is not always the string
   * that was sent: this is the note to route to, read and save.
   */
  path: string;
  source: string;
  revision: string;
}

/**
 * Creates a note that does not exist yet. With no source the request goes
 * with no body and no Content-Type: the daemon takes that as "an empty
 * note", and a Content-Type with nothing behind it is refused. A source is
 * sent as JSON, which is how a draft orphaned by a deleted file is written
 * back under the same name.
 *
 * Rejects with a SourceError carrying the daemon's code — "exists" for a
 * name already taken, "invalid_path", "not_markdown", "outside_root".
 */
export async function createNote(slug: string, path: string, source?: string): Promise<CreatedNote> {
  const init: RequestInit = { method: "POST" };
  if (source !== undefined) {
    init.headers = { "Content-Type": "application/json" };
    init.body = JSON.stringify({ source });
  }
  const res = await fetch(sourceURL(slug, path), init);
  if (!res.ok) throw await sourceErrorFrom(res);
  return (await res.json()) as CreatedNote;
}

/** Deletes one markdown note. Rejects with a SourceError. */
export async function deleteNote(slug: string, path: string): Promise<void> {
  const res = await fetch(sourceURL(slug, path), { method: "DELETE" });
  if (!res.ok) throw await sourceErrorFrom(res);
}
