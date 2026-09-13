import { afterEach, describe, expect, it, vi } from "vitest";
import { SourceError, fetchRaw, fetchSource, listRoots, saveSource } from "./api";

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
