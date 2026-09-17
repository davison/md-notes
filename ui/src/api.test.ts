import { afterEach, describe, expect, it, vi } from "vitest";
import {
  SourceError,
  UNREACHABLE,
  createNote,
  deleteNote,
  fetchRaw,
  fetchSource,
  listRoots,
  saveSource,
} from "./api";

function mockFetch(status: number, body: unknown) {
  const res = {
    ok: status >= 200 && status < 300,
    status,
    statusText: status === 200 ? "OK" : "Error",
    json: () => Promise.resolve(body),
  } as Response;
  vi.stubGlobal("fetch", vi.fn(() => Promise.resolve(res)));
}

afterEach(() => vi.unstubAllGlobals());

describe("listRoots", () => {
  it("returns the roots array", async () => {
    mockFetch(200, { roots: [{ slug: "notes", path: "/n", kind: "notes" }] });
    const roots = await listRoots();
    expect(roots).toHaveLength(1);
    expect(roots[0].slug).toBe("notes");
  });

  it("surfaces the daemon's error message", async () => {
    // What the guard actually sends: a refusal now carries a code beside
    // the message, and the message names the remedy.
    mockFetch(403, {
      code: "cross_origin",
      error:
        "cross-origin request refused; present the bearer token to write from another origin",
    });
    await expect(listRoots()).rejects.toThrow("cross-origin request refused");
  });

  it("falls back to the status when the body is not JSON", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(() =>
        Promise.resolve({
          ok: false,
          status: 502,
          statusText: "Bad Gateway",
          json: () => Promise.reject(new Error("not json")),
        } as unknown as Response),
      ),
    );
    await expect(listRoots()).rejects.toThrow("502 Bad Gateway");
  });
});

describe("fetchRaw", () => {
  it("surfaces the daemon's error message", async () => {
    mockFetch(403, { error: "path is outside the root" });
    await expect(fetchRaw("notes", "x.md")).rejects.toThrow("path is outside the root");
  });
});

describe("source", () => {
  it("fetchSource asks for the source uncached and returns it", async () => {
    mockFetch(200, { source: "# a\n", revision: "r1" });
    const s = await fetchSource("my notes", "dir/a b.md");
    expect(s).toEqual({ source: "# a\n", revision: "r1" });
    const [url, init] = (fetch as unknown as ReturnType<typeof vi.fn>).mock.calls[0] as [string, RequestInit];
    expect(url).toBe("/api/r/my%20notes/source/dir/a%20b.md");
    expect(init.cache).toBe("no-store");
  });

  it("saveSource PUTs JSON with the revision and returns the new one", async () => {
    mockFetch(200, { source: "# b\n", revision: "r2" });
    const s = await saveSource("n", "a.md", "# b\n", "r1", true);
    expect(s.revision).toBe("r2");
    const [url, init] = (fetch as unknown as ReturnType<typeof vi.fn>).mock.calls[0] as [string, RequestInit];
    expect(url).toBe("/api/r/n/source/a.md");
    expect(init.method).toBe("PUT");
    expect((init.headers as Record<string, string>)["Content-Type"]).toBe("application/json");
    expect(JSON.parse(init.body as string)).toEqual({ source: "# b\n", revision: "r1" });
    expect(init.keepalive).toBe(true);
  });

  it("surfaces the daemon's code and message as a SourceError", async () => {
    mockFetch(409, { code: "conflict", error: "note changed; reload before saving" });
    const err = await saveSource("n", "a.md", "x", "r1").catch((e: unknown) => e);
    expect(err).toBeInstanceOf(SourceError);
    expect(err).toMatchObject({ status: 409, code: "conflict", message: "note changed; reload before saving" });
  });

  it("treats a non-JSON failure as an I/O error", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(() =>
        Promise.resolve({
          ok: false,
          status: 502,
          statusText: "Bad Gateway",
          json: () => Promise.reject(new Error("not json")),
        } as unknown as Response),
      ),
    );
    const err = await fetchSource("n", "a.md").catch((e: unknown) => e);
    expect(err).toMatchObject({ status: 502, code: "io_error", message: "502 Bad Gateway" });
  });
});

describe("createNote", () => {
  it("POSTs the path with no body and no Content-Type, and returns the daemon's note", async () => {
    const res = {
      ok: true,
      status: 201,
      statusText: "Created",
      json: () => Promise.resolve({ root: "n", path: "docs/a.md", source: "", revision: "r1" }),
    } as Response;
    const fetchMock = vi.fn(() => Promise.resolve(res));
    vi.stubGlobal("fetch", fetchMock);
    const note = await createNote("my notes", "docs/a b.md");
    expect(fetchMock).toHaveBeenCalledWith("/api/r/my%20notes/source/docs/a%20b.md", { method: "POST" });
    expect(note.path).toBe("docs/a.md");
    expect(note.revision).toBe("r1");
  });

  it("rejects with the daemon's code so the prompt can say what was wrong", async () => {
    mockFetch(409, { code: "exists", error: "a note by that name already exists" });
    await expect(createNote("n", "a.md")).rejects.toMatchObject({
      name: "SourceError",
      status: 409,
      code: "exists",
      message: "a note by that name already exists",
    });
  });
});

describe("deleteNote", () => {
  it("DELETEs the note's own URL", async () => {
    const fetchMock = vi.fn(() => Promise.resolve({ ok: true, status: 204, statusText: "No Content" } as Response));
    vi.stubGlobal("fetch", fetchMock);
    await deleteNote("n", "docs/a.md");
    expect(fetchMock).toHaveBeenCalledWith("/api/r/n/source/docs/a.md", { method: "DELETE" });
  });

  it("rejects with the daemon's code", async () => {
    mockFetch(422, { code: "unsupported_source", error: "not a regular file" });
    await expect(deleteNote("n", "a.md")).rejects.toBeInstanceOf(SourceError);
  });
});

describe("a daemon that is not there", () => {
  // The service worker can open the app with nothing behind it, and the
  // browser's own "Failed to fetch" is what the pages would otherwise show
  // for it. Every call that can be made from a cold shell goes through the
  // same wrapper.
  it("reads as the daemon rather than as fetch", async () => {
    vi.stubGlobal("fetch", vi.fn(() => Promise.reject(new TypeError("Failed to fetch"))));
    await expect(listRoots()).rejects.toThrow(UNREACHABLE);
    await expect(fetchRaw("notes", "a.md")).rejects.toThrow(UNREACHABLE);
    await expect(fetchSource("notes", "a.md")).rejects.toThrow(UNREACHABLE);
    await expect(saveSource("notes", "a.md", "x", "rev")).rejects.toThrow(UNREACHABLE);
    await expect(deleteNote("notes", "a.md")).rejects.toThrow(UNREACHABLE);
  });

  it("leaves a refusal the daemon did send alone", async () => {
    mockFetch(403, { code: "cross_origin", error: "cross-origin request refused" });
    await expect(listRoots()).rejects.toThrow("cross-origin request refused");
  });
});
