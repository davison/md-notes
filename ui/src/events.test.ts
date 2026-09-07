import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { renderHook, cleanup } from "@testing-library/preact";
import { affects, affectsTree, useEvents } from "./events";

class FakeEventSource {
  static instances: FakeEventSource[] = [];
  url: string;
  closed = false;
  onerror: (() => void) | null = null;
  onopen: (() => void) | null = null;
  private listeners: Record<string, ((e: MessageEvent) => void)[]> = {};
  constructor(url: string) {
    this.url = url;
    FakeEventSource.instances.push(this);
  }
  addEventListener(type: string, fn: (e: MessageEvent) => void) {
    (this.listeners[type] ??= []).push(fn);
  }
  emit(type: string, data: string) {
    for (const fn of this.listeners[type] ?? []) fn({ data } as MessageEvent);
  }
  close() {
    this.closed = true;
  }
}

beforeEach(() => {
  FakeEventSource.instances = [];
  vi.stubGlobal("EventSource", FakeEventSource);
});
afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

describe("affects", () => {
  it("matches the path itself, a parent directory, or an empty batch", () => {
    expect(affects(["a/b.md"], "a/b.md")).toBe(true);
    expect(affects(["a"], "a/b.md")).toBe(true);
    expect(affects(["a/b.md"], "a/b.markdown")).toBe(false);
    expect(affects(["ab"], "a/b.md")).toBe(false);
    expect(affects([], "a/b.md")).toBe(true);
  });
});

describe("affectsTree", () => {
  it("skips batches made only of non-markdown files", () => {
    expect(affectsTree(["img/pic.png", "data.json"])).toBe(false);
    expect(affectsTree(["img/pic.png", "note.md"])).toBe(true);
    expect(affectsTree(["docs"])).toBe(true);
    expect(affectsTree(["a/README.MD"])).toBe(true);
    expect(affectsTree([".hidden"])).toBe(true);
    expect(affectsTree([])).toBe(true);
  });
});

describe("useEvents", () => {
  it("subscribes to the root's stream and passes batches through", () => {
    const seen: string[][] = [];
    renderHook(() => useEvents("my notes", (p) => seen.push(p)));
    const es = FakeEventSource.instances[0];
    expect(es.url).toBe("/api/r/my%20notes/events");
    es.emit("change", JSON.stringify({ paths: ["x.md"] }));
    expect(seen).toEqual([["x.md"]]);
    es.emit("change", "garbage");
    expect(seen).toEqual([["x.md"], []]);
  });

  it("signals a full refresh after a reconnect, not on first open", () => {
    const seen: string[][] = [];
    renderHook(() => useEvents("n", (p) => seen.push(p)));
    const es = FakeEventSource.instances[0];
    es.onopen!();
    expect(seen).toEqual([]);
    es.onerror!();
    es.onopen!();
    expect(seen).toEqual([[]]);
  });

  it("closes on unmount and resubscribes when the root changes", () => {
    const { rerender, unmount } = renderHook(({ slug }: { slug: string }) => useEvents(slug, () => {}), {
      initialProps: { slug: "a" },
    });
    rerender({ slug: "b" });
    expect(FakeEventSource.instances.map((e) => [e.url, e.closed])).toEqual([
      ["/api/r/a/events", true],
      ["/api/r/b/events", false],
    ]);
    unmount();
    expect(FakeEventSource.instances[1].closed).toBe(true);
  });
});
