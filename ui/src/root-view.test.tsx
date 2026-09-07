import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { cleanup, render, screen, waitFor } from "@testing-library/preact";
import { RootView } from "./root-view";

class FakeEventSource {
  static last: FakeEventSource | null = null;
  private listeners: ((e: MessageEvent) => void)[] = [];
  onerror: (() => void) | null = null;
  onopen: (() => void) | null = null;
  constructor(public url: string) {
    FakeEventSource.last = this;
  }
  addEventListener(_: string, fn: (e: MessageEvent) => void) {
    this.listeners.push(fn);
  }
  emit(paths: string[]) {
    for (const fn of this.listeners) fn({ data: JSON.stringify({ paths }) } as MessageEvent);
  }
  close() {}
}

const calls: string[] = [];
function mockApi() {
  vi.stubGlobal(
    "fetch",
    vi.fn((url: string) => {
      calls.push(url);
      let body: unknown;
      if (url === "/api/roots") body = { roots: [{ slug: "n", path: "/n", kind: "notes" }] };
      else if (url === "/api/r/n/tree")
        body = { name: "", path: "", dir: true, children: [{ name: "a.md", path: "docs/a.md", dir: false }] };
      else body = { path: "docs/a.md", title: "A", html: "<p>body</p>" };
      return Promise.resolve({ ok: true, status: 200, statusText: "OK", json: () => Promise.resolve(body) } as Response);
    }),
  );
}

beforeEach(() => {
  calls.length = 0;
  vi.stubGlobal("EventSource", FakeEventSource);
  mockApi();
});
afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

describe("RootView live update", () => {
  it("refetches the tree on any batch and the note only when it is affected", async () => {
    render(<RootView slug="n" note="docs/a.md" />);
    await waitFor(() => expect(screen.getByText("body")).toBeTruthy());
    const treeCalls = () => calls.filter((u) => u === "/api/r/n/tree").length;
    const noteCalls = () => calls.filter((u) => u.startsWith("/api/r/n/note/")).length;
    expect(treeCalls()).toBe(1);
    expect(noteCalls()).toBe(1);

    FakeEventSource.last!.emit(["other.md"]);
    await waitFor(() => expect(treeCalls()).toBe(2));
    expect(noteCalls()).toBe(1);

    FakeEventSource.last!.emit(["docs"]);
    await waitFor(() => expect(noteCalls()).toBe(2));
    expect(treeCalls()).toBe(3);

    FakeEventSource.last!.emit([]);
    await waitFor(() => expect(noteCalls()).toBe(3));
    expect(treeCalls()).toBe(4);

    FakeEventSource.last!.emit(["img/pic.png"]);
    await new Promise((r) => setTimeout(r, 20));
    expect(treeCalls()).toBe(4);
    expect(noteCalls()).toBe(3);
  });
});
