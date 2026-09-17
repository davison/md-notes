import { describe, expect, it } from "vitest";
import { DaemonError } from "./daemon";
import { resolveOpen, type RootsApi } from "./open-file";
import type { Root } from "./paths";
import type { Settings } from "./settings";

const settings: Settings = { daemonUrl: "http://localhost:7337", token: "s3cret" };

function api(roots: Root[], onRegister?: (dir: string) => Promise<Root>): RootsApi & {
  registered: string[];
  verified: string[];
} {
  const registered: string[] = [];
  const verified: string[] = [];
  return {
    registered,
    verified,
    async listRoots() {
      return roots;
    },
    async registerRoot(_settings, dir, file) {
      registered.push(dir);
      verified.push(file);
      if (onRegister !== undefined) return onRegister(dir);
      const slug = dir.slice(dir.lastIndexOf("/") + 1);
      return { slug, path: dir, kind: "recent" };
    },
  };
}

/**
 * A daemon that verifies before it registers, which is what the one behind
 * `POST /api/roots` does since M7-R1: the folder joins its roots only when
 * the note named with it is one of `files`, and a registration refused
 * leaves the listing exactly as it was.
 */
function verifyingDaemon(files: string[]): RootsApi & { roots: Root[] } {
  const roots: Root[] = [notes];
  return {
    roots,
    async listRoots() {
      return [...roots];
    },
    async registerRoot(_settings, dir, file) {
      if (!files.includes(`${dir}/${file}`)) {
        const words = `no such note under that folder: ${file}`;
        throw new DaemonError("not_found", words, 404, words);
      }
      const root: Root = { slug: dir.slice(dir.lastIndexOf("/") + 1), path: dir, kind: "recent" };
      roots.push(root);
      return root;
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

  it("names the tailnet allow-list when registering a folder is refused", async () => {
    // Under `tailnet_host` the roots listing succeeds — it now carries the
    // token — and the registration behind it is what the allow-list refuses.
    const remote: Settings = { daemonUrl: "https://laptop.ts.net", token: "s3cret" };
    const a: RootsApi = {
      async listRoots() {
        return [notes];
      },
      async registerRoot() {
        throw new DaemonError(
          "loopback_only",
          "this endpoint is served on loopback only; it is not reachable under laptop.ts.net",
          403,
          "this endpoint is served on loopback only; it is not reachable under laptop.ts.net",
        );
      },
    };
    const result = await resolveOpen("file:///tmp/scratch/a.md", remote, a);
    expect(result).toMatchObject({ status: "failed", kind: "loopback_only" });
    expect((result as { message: string }).message).toContain(
      "Registering a folder is refused over laptop.ts.net",
    );
    expect((result as { message: string }).message).toContain("`POST /api/roots`");
    expect((result as { message: string }).message).toContain("mdn open DIR");
    expect((result as { message: string }).message).not.toContain("mdn token");
  });

  it("does not call a refused listing a refused registration", async () => {
    // The allow-list permits GET /api/roots, so this cannot happen today. If
    // it ever did, "Registering a folder is refused" would be the same
    // misattribution this module exists to stop: the listing gets the
    // daemon's own words instead.
    const remote: Settings = { daemonUrl: "https://laptop.ts.net", token: "s3cret" };
    const a: RootsApi = {
      async listRoots() {
        throw new DaemonError(
          "loopback_only",
          "this endpoint is served on loopback only; it is not reachable under laptop.ts.net",
          403,
          "this endpoint is served on loopback only; it is not reachable under laptop.ts.net",
        );
      },
      async registerRoot() {
        throw new Error("not reached");
      },
    };
    expect(await resolveOpen("file:///home/you/notes/a.md", remote, a)).toEqual({
      status: "failed",
      kind: "loopback_only",
      message: "this endpoint is served on loopback only; it is not reachable under laptop.ts.net",
    });
  });

  it("blames the port when the daemon does not answer to the address", async () => {
    const remote: Settings = { daemonUrl: "http://laptop.ts.net:7337", token: "s3cret" };
    const a: RootsApi = {
      async listRoots() {
        throw new DaemonError("bad_host", "unexpected Host header", 403, "unexpected Host header");
      },
      async registerRoot() {
        throw new Error("not reached");
      },
    };
    const result = await resolveOpen("file:///home/you/notes/a.md", remote, a);
    expect(result).toMatchObject({ status: "failed", kind: "bad_host" });
    expect((result as { message: string }).message).toContain(
      "does not answer to laptop.ts.net:7337",
    );
    expect((result as { message: string }).message).toContain("`tailnet_host`");
  });

  it("opens a file inside a registered root under a tailnet daemon URL", async () => {
    const remote: Settings = { daemonUrl: "https://laptop.ts.net", token: "s3cret" };
    const a = api([notes]);
    expect(await resolveOpen("file:///home/you/notes/deep/a.md", remote, a)).toEqual({
      status: "open",
      url: "https://laptop.ts.net/r/notes/deep/a.md",
      slug: "notes",
      note: "deep/a.md",
    });
    expect(a.registered).toEqual([]);
  });

  it("names the note the daemon could not find, and blames nothing else", async () => {
    // M7-R1: the file-URL intercept tells the daemon which note it is
    // opening, and a note that is not there is refused before anything is
    // registered. What the badge and the popup then say has to be the
    // refusal it is — #50's own case, `file:///etc/no-such-note.md`, used
    // to register /etc; before this it would have read as a token problem.
    const daemon = verifyingDaemon(["/tmp/scratch/todo.md"]);
    const result = await resolveOpen("file:///tmp/scratch/no-such-note.md", settings, daemon);
    expect(result).toMatchObject({ status: "failed", kind: "not_found" });
    const message = (result as { message: string }).message;
    expect(message).toMatch(/no such note/i);
    expect(message).toContain("/tmp/scratch/no-such-note.md");
    expect(message).not.toMatch(/token/i);
    expect(message).not.toMatch(/not reachable/i);
    // and the roots listing is exactly what it was
    expect(daemon.roots).toEqual([notes]);
  });

  it("registers the folder when the note it names is there", async () => {
    const daemon = verifyingDaemon(["/tmp/scratch/todo.md"]);
    expect(await resolveOpen("file:///tmp/scratch/todo.md", settings, daemon)).toEqual({
      status: "open",
      url: "http://localhost:7337/r/scratch/todo.md",
      slug: "scratch",
      note: "todo.md",
    });
    expect(daemon.roots).toHaveLength(2);
  });

  it("sends the file's own name beside the directory", async () => {
    const a = api([notes]);
    await resolveOpen("file:///tmp/scratch/my%20note.md", settings, a);
    expect(a.registered).toEqual(["/tmp/scratch"]);
    expect(a.verified).toEqual(["my note.md"]);
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
