import { afterEach, describe, expect, it, vi } from "vitest";
import { fetchRaw, listRoots } from "./api";

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
    mockFetch(403, { error: "cross-origin request refused" });
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
