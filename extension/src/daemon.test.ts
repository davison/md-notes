import { describe, expect, it } from "vitest";
import { DaemonError, listRoots, postClip, registerRoot } from "./daemon";
import type { Settings } from "./settings";

const settings: Settings = { daemonUrl: "http://localhost:7337", token: "" };
const withToken: Settings = { ...settings, token: "s3cret" };

function respond(status: number, body: unknown, ok = status < 400): Response {
  return {
    ok,
    status,
    statusText: status === 403 ? "Forbidden" : "",
    json: async () => body,
  } as unknown as Response;
}

function record() {
  const calls: { url: string; init: RequestInit | undefined }[] = [];
  return {
    calls,
    fetcher(reply: Response | (() => never)) {
      return (async (url: string | URL | Request, init?: RequestInit) => {
        calls.push({ url: String(url), init });
        if (typeof reply === "function") reply();
        return reply;
      }) as typeof fetch;
    },
  };
}

describe("listRoots", () => {
  it("GETs the roots endpoint with no Authorization header", async () => {
    const r = record();
    const roots = await listRoots(
      withToken,
      { fetch: r.fetcher(respond(200, { roots: [{ slug: "notes", path: "/n", kind: "notes" }] })) },
    );
    expect(roots).toEqual([{ slug: "notes", path: "/n", kind: "notes" }]);
    expect(r.calls[0]?.url).toBe("http://localhost:7337/api/roots");
    expect(r.calls[0]?.init?.headers).toBeUndefined();
  });

  it("reports an unreachable daemon rather than throwing the network error", async () => {
    const r = record();
    const err = await listRoots(settings, {
      fetch: r.fetcher(() => {
        throw new TypeError("Failed to fetch");
      }),
    }).catch((e: unknown) => e);
    expect(err).toBeInstanceOf(DaemonError);
    expect((err as DaemonError).kind).toBe("unreachable");
    expect((err as DaemonError).message).toContain("http://localhost:7337");
  });

  it("sends the token when the caller asks the daemon to judge it", async () => {
    const r = record();
    await listRoots(withToken, {
      authenticated: true,
      fetch: r.fetcher(respond(200, { roots: [] })),
    });
    expect(r.calls[0]?.init?.headers).toMatchObject({ Authorization: "Bearer s3cret" });
  });

  it("sends no header when asked to authenticate with no token stored", async () => {
    const r = record();
    await listRoots(settings, { authenticated: true, fetch: r.fetcher(respond(200, { roots: [] })) });
    expect(r.calls[0]?.init?.headers).toBeUndefined();
  });

  it("reports a rejected token from an authenticated read", async () => {
    const r = record();
    const err = await listRoots(withToken, {
      authenticated: true,
      fetch: r.fetcher(respond(401, { error: "invalid token" })),
    }).catch((e: unknown) => e);
    expect((err as DaemonError).kind).toBe("token_rejected");
  });

  it("refuses a reply that is not a roots list", async () => {
    const r = record();
    const err = await listRoots(settings, { fetch: r.fetcher(respond(200, { nope: 1 })) }).catch(
      (e: unknown) => e,
    );
    expect((err as DaemonError).kind).toBe("bad_response");
  });
});

describe("registerRoot", () => {
  it("POSTs the absolute directory with the bearer token", async () => {
    const r = record();
    const root = await registerRoot(
      withToken,
      "/home/you/scratch",
      { fetch: r.fetcher(respond(200, { slug: "scratch", path: "/home/you/scratch", kind: "recent" })) },
    );
    expect(root.slug).toBe("scratch");
    const call = r.calls[0];
    expect(call?.init?.method).toBe("POST");
    expect(call?.init?.body).toBe('{"path":"/home/you/scratch"}');
    expect(call?.init?.headers).toMatchObject({
      "Content-Type": "application/json",
      Authorization: "Bearer s3cret",
    });
  });

  it("sends no Authorization header when no token is stored", async () => {
    const r = record();
    await registerRoot(
      settings,
      "/n",
      { fetch: r.fetcher(respond(200, { slug: "n", path: "/n", kind: "recent" })) },
    );
    expect(r.calls[0]?.init?.headers).not.toHaveProperty("Authorization");
  });

  it("explains a 403 from the Origin guard by naming the token", async () => {
    const r = record();
    const err = await registerRoot(
      settings,
      "/n",
      { fetch: r.fetcher(respond(403, { error: "cross-origin request refused" })) },
    ).catch((e: unknown) => e);
    expect((err as DaemonError).kind).toBe("origin_refused");
    expect((err as DaemonError).message).toContain("cross-origin request refused");
    expect((err as DaemonError).message).toContain("mdn token");
  });

  it("says the daemon may be out of date when a token was sent and still refused", async () => {
    const r = record();
    const err = await registerRoot(
      withToken,
      "/n",
      { fetch: r.fetcher(respond(403, { error: "cross-origin request refused" })) },
    ).catch((e: unknown) => e);
    expect((err as DaemonError).message).toContain("even with a token");
  });

  it("reports a rejected token", async () => {
    const r = record();
    const err = await registerRoot(
      withToken,
      "/n",
      { fetch: r.fetcher(respond(401, { error: "invalid token" })) },
    ).catch((e: unknown) => e);
    expect((err as DaemonError).kind).toBe("token_rejected");
  });

  it("passes the daemon's own message through for other refusals", async () => {
    const r = record();
    const err = await registerRoot(
      withToken,
      "relative/dir",
      { fetch: r.fetcher(respond(400, { error: "path must be absolute" })) },
    ).catch((e: unknown) => e);
    expect((err as DaemonError).kind).toBe("refused");
    expect((err as DaemonError).message).toBe("path must be absolute");
  });
});

describe("postClip", () => {
  const clip = {
    url: "https://example.com/a",
    title: "A page",
    markdown: "# A page\n",
    kind: "page" as const,
  };

  it("POSTs the clip with the bearer token and returns where it landed", async () => {
    const r = record();
    const result = await postClip(withToken, clip, {
      fetch: r.fetcher(respond(201, { root: "notes", path: "clips/2026-09-11-a-page.md" })),
    });
    expect(result).toEqual({ root: "notes", path: "clips/2026-09-11-a-page.md" });
    const call = r.calls[0];
    expect(call?.url).toBe("http://localhost:7337/api/clip");
    expect(call?.init?.method).toBe("POST");
    expect(JSON.parse(String(call?.init?.body))).toEqual(clip);
    expect(call?.init?.headers).toMatchObject({
      "Content-Type": "application/json",
      Authorization: "Bearer s3cret",
    });
  });

  it("does not ask at all when no token is stored", async () => {
    const r = record();
    const err = await postClip(settings, clip, {
      fetch: r.fetcher(respond(201, { root: "notes", path: "clips/x.md" })),
    }).catch((e: unknown) => e);
    expect((err as DaemonError).kind).toBe("no_token");
    expect(r.calls).toHaveLength(0);
  });

  it("reads the refusal envelope's code, not just its status", async () => {
    const r = record();
    const err = await postClip(withToken, clip, {
      fetch: r.fetcher(respond(403, { code: "cross_origin", error: "cross-origin request refused" })),
    }).catch((e: unknown) => e);
    expect((err as DaemonError).kind).toBe("origin_refused");
    expect((err as DaemonError).detail).toBe("cross-origin request refused");
  });

  it("tells a rotated-away token apart from a refused origin", async () => {
    const r = record();
    const err = await postClip(withToken, clip, {
      fetch: r.fetcher(respond(401, { code: "unauthorized", error: "invalid bearer token" })),
    }).catch((e: unknown) => e);
    expect((err as DaemonError).kind).toBe("token_rejected");
    expect((err as DaemonError).detail).toBe("invalid bearer token");
  });

  it("does not read a bad_host refusal as an origin problem", async () => {
    const r = record();
    const err = await postClip(withToken, clip, {
      fetch: r.fetcher(respond(403, { code: "bad_host", error: "unexpected Host header" })),
    }).catch((e: unknown) => e);
    expect((err as DaemonError).kind).toBe("refused");
    expect((err as DaemonError).message).toBe("unexpected Host header");
  });

  it("passes the clip handler's own refusals through", async () => {
    const r = record();
    const err = await postClip(withToken, clip, {
      fetch: r.fetcher(respond(413, { code: "too_large", error: "markdown is too large" })),
    }).catch((e: unknown) => e);
    expect((err as DaemonError).kind).toBe("refused");
    expect((err as DaemonError).message).toBe("markdown is too large");
  });

  it("reports an unreachable daemon", async () => {
    const r = record();
    const err = await postClip(withToken, clip, {
      fetch: r.fetcher(() => {
        throw new TypeError("Failed to fetch");
      }),
    }).catch((e: unknown) => e);
    expect((err as DaemonError).kind).toBe("unreachable");
  });

  it("refuses a success that does not say where the clip landed", async () => {
    const r = record();
    const err = await postClip(withToken, clip, {
      fetch: r.fetcher(respond(201, { path: "clips/x.md" })),
    }).catch((e: unknown) => e);
    expect((err as DaemonError).kind).toBe("bad_response");
  });
});
