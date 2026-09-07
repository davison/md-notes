export type RootKind = "notes" | "recent";

export interface Root {
  slug: string;
  path: string;
  kind: RootKind;
}

async function request<T>(path: string, init?: RequestInit): Promise<T> {
  const res = await fetch(path, init);
  if (!res.ok) {
    let message = `${res.status} ${res.statusText}`;
    try {
      const body = (await res.json()) as { error?: string };
      if (body.error) message = body.error;
    } catch {
      // not JSON; keep the status text
    }
    throw new Error(message);
  }
  return (await res.json()) as T;
}

export async function listRoots(): Promise<Root[]> {
  const body = await request<{ roots: Root[] }>("/api/roots");
  return body.roots;
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
  if (!res.ok) throw new Error(`${res.status} ${res.statusText}`);
  return res.text();
}
