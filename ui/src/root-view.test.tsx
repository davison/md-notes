import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/preact";
import { LocationProvider } from "preact-iso";
import { RootView } from "./root-view";
import { getSession, resetSessions } from "./session";

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
        body = {
          name: "",
          path: "",
          dir: true,
          children: [
            { name: "docs", path: "docs", dir: true, children: [{ name: "a.md", path: "docs/a.md", dir: false }] },
            { name: "b.md", path: "b.md", dir: false },
          ],
        };
      else if (url === "/api/r/n/tags") body = { tags: [{ name: "x", count: 1, notes: ["b.md"] }] };
      else if (url.includes("/source/")) body = { source: "body\n", revision: "r1" };
      else body = { path: "docs/a.md", title: "A", html: "<p>body</p>" };
      return Promise.resolve({ ok: true, status: 200, statusText: "OK", json: () => Promise.resolve(body) } as Response);
    }),
  );
}

beforeEach(() => {
  calls.length = 0;
  localStorage.clear();
  resetSessions();
  vi.stubGlobal("EventSource", FakeEventSource);
  mockApi();
});
afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

function mount(url: string) {
  history.replaceState(null, "", url);
  return render(
    <LocationProvider>
      <RootView slug="n" note="docs/a.md" />
    </LocationProvider>,
  );
}

describe("RootView live update", () => {
  it("refetches the tree on any batch and the note only when it is affected", async () => {
    mount("/r/n/docs/a.md");
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

  it("routes a batch naming the open note into its editing session, and nothing else", async () => {
    mount("/r/n/docs/a.md");
    await waitFor(() => expect(screen.getByText("body")).toBeTruthy());
    const session = getSession("n", "docs/a.md");
    const sourceCalls = () => calls.filter((u) => u.includes("/source/")).length;
    // Before the note is opened for editing, a batch is the viewer's business only.
    FakeEventSource.last!.emit(["docs/a.md"]);
    await waitFor(() => expect(calls.filter((u) => u.startsWith("/api/r/n/note/")).length).toBe(2));
    expect(sourceCalls()).toBe(0);

    fireEvent.keyDown(document.body, { key: "e", ctrlKey: true });
    await waitFor(() => expect(session.state.status).toBe("clean"));
    expect(document.querySelector("main.editor-body")).toBeTruthy();
    expect(sourceCalls()).toBe(1);
    const changed = vi.spyOn(session, "changed");

    FakeEventSource.last!.emit(["other.md"]);
    await waitFor(() => expect(calls.filter((u) => u === "/api/r/n/tree").length).toBe(3));
    expect(changed).not.toHaveBeenCalled();

    FakeEventSource.last!.emit(["docs"]);
    await waitFor(() => expect(changed).toHaveBeenCalledTimes(1));
    await waitFor(() => expect(sourceCalls()).toBe(2));
    expect(session.state.status).toBe("clean");
  });

  it("refreshes tags when the tree changes", async () => {
    mount("/r/n/docs/a.md");
    await waitFor(() => expect(screen.getByText("#x")).toBeTruthy());
    const tagCalls = () => calls.filter((u) => u === "/api/r/n/tags").length;
    expect(tagCalls()).toBe(1);
    FakeEventSource.last!.emit(["new.md"]);
    await waitFor(() => expect(tagCalls()).toBe(2));
  });
});

describe("RootView tag filter", () => {
  it("filters the navigator to the tag's notes and offers to clear", async () => {
    mount("/r/n/docs/a.md?tag=x");
    await waitFor(() => expect(screen.getByText("b.md")).toBeTruthy());
    expect(screen.queryByText("docs")).toBeNull();
    expect(screen.getByText("Clear filter: x")).toBeTruthy();
    expect(screen.getByText("#x").closest("a")!.classList.contains("active")).toBe(true);
  });

  it("passes the line query to the note view", async () => {
    mount("/r/n/docs/a.md?l=7");
    await waitFor(() => expect(screen.getByText("body")).toBeTruthy());
    // No marker at or before line 7 in the fixture body, so nothing to scroll; the view still renders.
    expect(screen.getByText("A")).toBeTruthy();
  });
});
