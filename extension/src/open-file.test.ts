import { describe, expect, it } from "vitest";
import { DaemonError } from "./daemon";
import { resolveOpen, type RootsApi } from "./open-file";
import type { Root } from "./paths";
import type { Settings } from "./settings";

const settings: Settings = { daemonUrl: "http://localhost:7337", token: "s3cret" };

function api(roots: Root[], onRegister?: (dir: string) => Promise<Root>): RootsApi & {
  registered: string[];
} {
  const registered: string[] = [];
  return {
    registered,
    async listRoots() {
      return roots;
    },
    async registerRoot(_settings, dir) {
      registered.push(dir);
      if (onRegister !== undefined) return onRegister(dir);
      const slug = dir.slice(dir.lastIndexOf("/") + 1);
      return { slug, path: dir, kind: "recent" };
    },
  };
}

const notes: Root = { slug: "notes", path: "/home/you/notes", kind: "notes" };

describe("resolveOpen", () => {
  it("ignores a navigation that is not a local markdown file", async () => {
    const a = api([notes]);
    expect(await resolveOpen("https://example.com/page.md", settings, a)).toEqual({
      status: "ignored",
    });
    expect(await resolveOpen("file:///home/you/notes/a.txt", settings, a)).toEqual({
      status: "ignored",
    });
    expect(a.registered).toEqual([]);
  });

  it("opens a file inside a registered root under that root, registering nothing", async () => {
    const a = api([notes]);
    expect(await resolveOpen("file:///home/you/notes/deep/a.md", settings, a)).toEqual({
      status: "open",
      url: "http://localhost:7337/r/notes/deep/a.md",
      slug: "notes",
      note: "deep/a.md",
    });
    expect(a.registered).toEqual([]);
  });

  it("registers the file's own directory when no root contains it", async () => {
    const a = api([notes]);
    expect(await resolveOpen("file:///tmp/scratch/todo.md", settings, a)).toEqual({
      status: "open",
      url: "http://localhost:7337/r/scratch/todo.md",
      slug: "scratch",
      note: "todo.md",
    });
    expect(a.registered).toEqual(["/tmp/scratch"]);
  });

  it("uses the root path the daemon returns, so a symlinked alias still resolves", async () => {
    // The daemon compares real paths: registering an alias hands back the
    // existing root, whose path is the real one.
    const a = api([], async () => ({ slug: "notes", path: "/home/you/notes", kind: "notes" }));
    expect(await resolveOpen("file:///home/you/link/a.md", settings, a)).toMatchObject({
      status: "open",
      slug: "notes",
    });
  });

  it("encodes a name with spaces on the way into the app URL", async () => {
    const a = api([notes]);
    expect(await resolveOpen("file:///home/you/notes/my%20note.md", settings, a)).toMatchObject({
      url: "http://localhost:7337/r/notes/my%20note.md",
    });
  });

  it("reports an unreachable daemon and leaves the tab to the caller", async () => {
    const a: RootsApi = {
      async listRoots() {
        throw new DaemonError("unreachable", "daemon not reachable at http://localhost:7337");
      },
      async registerRoot() {
        throw new Error("not reached");
      },
    };
    expect(await resolveOpen("file:///home/you/notes/a.md", settings, a)).toEqual({
      status: "failed",
      kind: "unreachable",
      message: "daemon not reachable at http://localhost:7337",
    });
  });

  it("reports the Origin guard refusing the registration", async () => {
    const a: RootsApi = {
      async listRoots() {
        return [];
      },
      async registerRoot() {
        throw new DaemonError("origin_refused", "cross-origin request refused: no token", 403);
      },
    };
    expect(await resolveOpen("file:///tmp/scratch/a.md", settings, a)).toEqual({
      status: "failed",
      kind: "origin_refused",
      message: "cross-origin request refused: no token",
    });
  });

  it("turns an unexpected error into a failure rather than rejecting", async () => {
    const a: RootsApi = {
      async listRoots() {
        throw new Error("boom");
      },
      async registerRoot() {
        throw new Error("not reached");
      },
    };
    expect(await resolveOpen("file:///home/you/notes/a.md", settings, a)).toMatchObject({
      status: "failed",
      kind: "bad_response",
    });
  });
});
