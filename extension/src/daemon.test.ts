import { describe, expect, it } from "vitest";
import { DaemonError, listRoots, registerRoot } from "./daemon";
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
      r.fetcher(respond(200, { roots: [{ slug: "notes", path: "/n", kind: "notes" }] })),
    );
    expect(roots).toEqual([{ slug: "notes", path: "/n", kind: "notes" }]);
    expect(r.calls[0]?.url).toBe("http://localhost:7337/api/roots");
    expect(r.calls[0]?.init?.headers).toBeUndefined();
  });

  it("reports an unreachable daemon rather than throwing the network error", async () => {
    const r = record();
    const err = await listRoots(
      settings,
      r.fetcher(() => {
        throw new TypeError("Failed to fetch");
      }),
    ).catch((e: unknown) => e);
    expect(err).toBeInstanceOf(DaemonError);
    expect((err as DaemonError).kind).toBe("unreachable");
    expect((err as DaemonError).message).toContain("http://localhost:7337");
  });

  it("refuses a reply that is not a roots list", async () => {
    const r = record();
    const err = await listRoots(settings, r.fetcher(respond(200, { nope: 1 }))).catch(
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
      r.fetcher(respond(200, { slug: "scratch", path: "/home/you/scratch", kind: "recent" })),
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
      r.fetcher(respond(200, { slug: "n", path: "/n", kind: "recent" })),
    );
    expect(r.calls[0]?.init?.headers).not.toHaveProperty("Authorization");
  });

  it("explains a 403 from the Origin guard by naming the token", async () => {
    const r = record();
    const err = await registerRoot(
      settings,
      "/n",
      r.fetcher(respond(403, { error: "cross-origin request refused" })),
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
      r.fetcher(respond(403, { error: "cross-origin request refused" })),
    ).catch((e: unknown) => e);
    expect((err as DaemonError).message).toContain("even with a token");
  });

  it("reports a rejected token", async () => {
    const r = record();
    const err = await registerRoot(
      withToken,
      "/n",
      r.fetcher(respond(401, { error: "invalid token" })),
    ).catch((e: unknown) => e);
    expect((err as DaemonError).kind).toBe("token_rejected");
  });

  it("passes the daemon's own message through for other refusals", async () => {
    const r = record();
    const err = await registerRoot(
      withToken,
      "relative/dir",
      r.fetcher(respond(400, { error: "path must be absolute" })),
    ).catch((e: unknown) => e);
    expect((err as DaemonError).kind).toBe("refused");
    expect((err as DaemonError).message).toBe("path must be absolute");
  });
});
