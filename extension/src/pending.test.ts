import { describe, expect, it } from "vitest";
import { PENDING_KEY, clearPendingClip, getPendingClip, setPendingClip } from "./pending";
import type { StatusStore } from "./status";

function fakeStore(): StatusStore & { items: Record<string, unknown> } {
  const items: Record<string, unknown> = {};
  return {
    items,
    async get(keys) {
      const out: Record<string, unknown> = {};
      for (const k of keys) if (k in items) out[k] = items[k];
      return out;
    },
    async set(next) {
      Object.assign(items, next);
    },
    async remove(keys) {
      for (const k of keys) delete items[k];
    },
  };
}

const clip = {
  kind: "selection" as const,
  url: "https://example.com/a",
  title: "A page",
  markdown: "Body.",
  tabId: 7,
  at: 1,
};

describe("the pending clip", () => {
  it("round-trips, so a worker that was shut down loses nothing", async () => {
    const store = fakeStore();
    await setPendingClip(clip, store);
    expect(store.items[PENDING_KEY]).toEqual(clip);
    expect(await getPendingClip(store)).toEqual(clip);
  });

  it("is nothing when none was taken", async () => {
    expect(await getPendingClip(fakeStore())).toBeNull();
  });

  it("is gone once it is cleared", async () => {
    const store = fakeStore();
    await setPendingClip(clip, store);
    await clearPendingClip(store);
    expect(await getPendingClip(store)).toBeNull();
  });

  it("ignores a stored value of the wrong shape", async () => {
    const store = fakeStore();
    for (const bad of [{ kind: "page" }, { ...clip, at: "now" }, { ...clip, tabId: null }]) {
      store.items[PENDING_KEY] = bad;
      expect(await getPendingClip(store)).toBeNull();
    }
  });
});
