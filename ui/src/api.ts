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
