import { describe, expect, it } from "vitest";
import { clearTabStatus, getTabStatus, setTabStatus, statusKey, type StatusStore } from "./status";

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

describe("tab status", () => {
  it("round-trips a record under a per-tab key", async () => {
    const store = fakeStore();
    const status = {
      kind: "unreachable" as const,
      message: "daemon not reachable",
      source: "file:///n/a.md",
      at: 1,
    };
    await setTabStatus(7, status, store);
    expect(store.items[statusKey(7)]).toEqual(status);
    expect(await getTabStatus(7, store)).toEqual(status);
  });

  it("has nothing to say about a tab it never saw", async () => {
    expect(await getTabStatus(9, fakeStore())).toBeNull();
  });

  it("forgets a tab when it closes", async () => {
    const store = fakeStore();
    await setTabStatus(7, { kind: "opened", message: "ok", source: "file:///n/a.md", at: 1 }, store);
    await clearTabStatus(7, store);
    expect(await getTabStatus(7, store)).toBeNull();
  });

  it("ignores a stored value of the wrong shape", async () => {
    const store = fakeStore();
    store.items[statusKey(3)] = "nonsense";
    expect(await getTabStatus(3, store)).toBeNull();
  });
});
