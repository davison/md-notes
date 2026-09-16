import { describe, expect, it } from "vitest";
import {
  TAILNET_LIMITS,
  badHostMessage,
  clipRefusedByOlderDaemon,
  daemonHost,
  describeDaemonReach,
  isLoopbackUrl,
  presentsBearer,
  registerRefusedRemotely,
} from "./reach";
import type { Settings } from "./settings";

describe("isLoopbackUrl", () => {
  it.each([
    "http://localhost:7337",
    "http://localhost",
    "https://LOCALHOST:7337",
    "http://mdn.localhost:7337",
    "http://127.0.0.1:7337",
    "http://127.0.0.1",
    "http://127.1.2.3:7337",
    "http://127.255.255.255:7337",
    "http://[::1]:7337",
  ])("calls %s the local machine", (url) => {
    expect(isLoopbackUrl(url)).toBe(true);
  });

  it.each([
    "https://laptop.tailnet.ts.net",
    "http://laptop.tailnet.ts.net:7337",
    "http://laptop:7337",
    "http://192.168.1.10:7337",
    // Neighbours of the loopback block, and a name that only looks like one.
    "http://128.0.0.1:7337",
    "http://12.7.0.0.1:7337",
    "http://127.0.0.1.example.com:7337",
    "http://localhost.example.com:7337",
    "http://notlocalhost:7337",
  ])("calls %s somewhere else", (url) => {
    expect(isLoopbackUrl(url)).toBe(false);
  });

  it("treats a URL it cannot parse as somewhere else", () => {
    // The safe way round: a token the user configured travels to the address
    // they typed, rather than a tailnet daemon silently losing its one header.
    expect(isLoopbackUrl("not a url")).toBe(false);
    expect(isLoopbackUrl("")).toBe(false);
  });
});

describe("presentsBearer", () => {
  const local = (token: string): Settings => ({ daemonUrl: "http://localhost:7337", token });
  const remote = (token: string): Settings => ({ daemonUrl: "https://laptop.ts.net", token });

  it("keeps the unauthenticated read on loopback, which the intercept relies on", () => {
    expect(presentsBearer(local("s3cret"), false)).toBe(false);
  });

  it("presents the token on loopback when the caller insists", () => {
    expect(presentsBearer(local("s3cret"), true)).toBe(true);
  });

  it("presents the token to a daemon that is not on this machine, unasked", () => {
    expect(presentsBearer(remote("s3cret"), false)).toBe(true);
  });

  it("has nothing to present when no token is stored, wherever the daemon is", () => {
    expect(presentsBearer(local(""), true)).toBe(false);
    expect(presentsBearer(remote(""), false)).toBe(false);
    expect(presentsBearer(remote(""), true)).toBe(false);
  });
});

describe("daemonHost", () => {
  it("is the host and port, without the scheme", () => {
    expect(daemonHost("http://laptop.ts.net:7337")).toBe("laptop.ts.net:7337");
    expect(daemonHost("https://laptop.ts.net")).toBe("laptop.ts.net");
  });

  it("falls back to the text when it is not a URL", () => {
    expect(daemonHost("laptop.ts.net/")).toBe("laptop.ts.net");
  });
});

describe("the messages the tailnet allow-list needs", () => {
  it("names the endpoint and the action when registering a folder is refused", () => {
    const message = registerRefusedRemotely("https://laptop.ts.net");
    expect(message).toContain("laptop.ts.net");
    expect(message).toContain("`POST /api/roots`");
    expect(message).toContain("loopback only");
    expect(message).toContain("mdn open DIR");
    // Never the token: that is the misdiagnosis this whole task exists to end.
    expect(message).not.toContain("mdn token");
    expect(message).not.toMatch(/rejected the token/);
  });

  it("blames an out-of-date daemon when a clip is refused as loopback-only", () => {
    // M6-R1 admits the clip under a tailnet name, so this refusal can only
    // come from a daemon older than the extension talking to it.
    const message = clipRefusedByOlderDaemon(
      "https://laptop.ts.net",
      "this endpoint is served on loopback only; it is not reachable under laptop.ts.net",
    );
    expect(message).toContain("laptop.ts.net");
    expect(message).toContain("`POST /api/clip`");
    expect(message).toContain("predates");
    expect(message).toContain("update `mdn`");
    expect(message).not.toContain("mdn token");
    expect(message).not.toMatch(/rejected the token/);
  });

  it("blames the port, not the token, for a host the daemon does not answer to", () => {
    const message = badHostMessage("http://laptop.ts.net:7337", "unexpected Host header");
    expect(message).toContain("laptop.ts.net:7337");
    expect(message).toContain("unexpected Host header");
    expect(message).toContain("`tailnet_host`");
    expect(message).toContain("port");
    expect(message).not.toContain("mdn token");
  });
});

describe("describeDaemonReach", () => {
  it("says only where the daemon is when it is on this machine", () => {
    expect(describeDaemonReach({ daemonUrl: "http://localhost:7337", token: "t" })).toBe(
      "Daemon: http://localhost:7337",
    );
  });

  it("names what the tailnet refuses before anything is pressed", () => {
    const line = describeDaemonReach({ daemonUrl: "https://laptop.ts.net", token: "t" });
    expect(line).toContain("Daemon: https://laptop.ts.net");
    expect(line).toContain(TAILNET_LIMITS);
    expect(line).toMatch(/registering a folder is refused/);
    // Clipping is admitted over the tailnet since M6-R1, so the line no
    // longer warns about it before the button is pressed.
    expect(line).not.toMatch(/clipping (?:is|are) refused/);
    expect(line).toMatch(/clipping both work here/);
  });
});
