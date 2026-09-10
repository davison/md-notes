import { describe, expect, it } from "vitest";
import {
  DEFAULT_DAEMON_URL,
  loadSettings,
  normaliseDaemonUrl,
  originPattern,
  saveSettings,
  type StorageArea,
} from "./settings";

function fakeStore(initial: Record<string, unknown> = {}): StorageArea & {
  items: Record<string, unknown>;
} {
  const items = { ...initial };
  return {
    items,
    async get(keys: string[]) {
      const out: Record<string, unknown> = {};
      for (const k of keys) if (k in items) out[k] = items[k];
      return out;
    },
    async set(next: Record<string, unknown>) {
      Object.assign(items, next);
    },
  };
}

describe("loadSettings", () => {
  it("defaults to the loopback daemon and no token", async () => {
    expect(await loadSettings(fakeStore())).toEqual({
      daemonUrl: DEFAULT_DAEMON_URL,
      token: "",
    });
  });

  it("returns what was stored, without a trailing slash", async () => {
    const store = fakeStore({ daemonUrl: "http://localhost:9000/", token: "abc" });
    expect(await loadSettings(store)).toEqual({
      daemonUrl: "http://localhost:9000",
      token: "abc",
    });
  });

  it("ignores values of the wrong shape", async () => {
    const store = fakeStore({ daemonUrl: 42, token: null });
    expect(await loadSettings(store)).toEqual({ daemonUrl: DEFAULT_DAEMON_URL, token: "" });
  });
});

describe("saveSettings", () => {
  it("normalises the URL and trims the token", async () => {
    const store = fakeStore();
    await saveSettings({ daemonUrl: "localhost:9000/", token: "  abc  " }, store);
    expect(store.items).toEqual({ daemonUrl: "http://localhost:9000", token: "abc" });
  });
});

describe("normaliseDaemonUrl", () => {
  it("adds the scheme, drops path, query and trailing slash", () => {
    expect(normaliseDaemonUrl("localhost:7337")).toBe("http://localhost:7337");
    expect(normaliseDaemonUrl(" http://127.0.0.1:7337/r/notes?x=1 ")).toBe("http://127.0.0.1:7337");
    expect(normaliseDaemonUrl("https://notes.example.ts.net/")).toBe(
      "https://notes.example.ts.net",
    );
  });

  it("falls back to the default when the field is left empty", () => {
    expect(normaliseDaemonUrl("   ")).toBe(DEFAULT_DAEMON_URL);
  });

  it("refuses a non-http scheme and an unparseable value", () => {
    expect(() => normaliseDaemonUrl("ftp://localhost:7337")).toThrow(/http or https/);
    expect(() => normaliseDaemonUrl("http://")).toThrow(/not a URL/);
  });
});

describe("originPattern", () => {
  it("is the match pattern for exactly that origin, port included", () => {
    expect(originPattern(DEFAULT_DAEMON_URL)).toBe("http://localhost:7337/*");
    expect(originPattern("http://127.0.0.1:9000")).toBe("http://127.0.0.1:9000/*");
  });
});
