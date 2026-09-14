import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/preact";
import { LocationProvider } from "preact-iso";
import { RootView } from "./root-view";
import { getSession, resetSessions } from "./session";

class FakeEventSource {
  static last: FakeEventSource | null = null;
  private listeners: Record<string, ((e: MessageEvent) => void)[]> = {};
  readyState = 1;
  onerror: (() => void) | null = null;
  onopen: (() => void) | null = null;
  constructor(public url: string) {
    FakeEventSource.last = this;
  }
  addEventListener(type: string, fn: (e: MessageEvent) => void) {
    (this.listeners[type] ??= []).push(fn);
  }
  emit(paths: string[]) {
    for (const fn of this.listeners.change ?? []) fn({ data: JSON.stringify({ paths }) } as MessageEvent);
  }
  emitStatus(coverage: unknown) {
    for (const fn of this.listeners.status ?? []) fn({ data: JSON.stringify(coverage) } as MessageEvent);
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
      else if (url.startsWith("/api/r/n/search"))
        body = { hits: [{ path: "b.md", line: 3, text: "a needle here", matches: [[2, 8]] }], truncated: false };
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
    // The tag panel's own entry, not the top bar's chip, which says "#x" too.
    expect(document.querySelector(".tags .tag.active .tag-name")!.textContent).toBe("#x");
  });

  it("passes the line query to the note view", async () => {
    mount("/r/n/docs/a.md?l=7");
    await waitFor(() => expect(screen.getByText("body")).toBeTruthy());
    // No marker at or before line 7 in the fixture body, so nothing to scroll; the view still renders.
    expect(screen.getByText("A")).toBeTruthy();
  });
});

describe("watch coverage", () => {
  const complete = { watched: 4, unwatched: 0, budget: 8192, overBudget: false, failed: 0, limited: false };
  const limited = { watched: 8192, unwatched: 4498, budget: 8192, overBudget: true, failed: 0, limited: true };

  it("says nothing while the whole root is watched", async () => {
    render(
      <LocationProvider>
        <RootView slug="n" />
      </LocationProvider>,
    );
    await screen.findByText("docs");
    FakeEventSource.last!.emitStatus(complete);
    await waitFor(() => expect(screen.queryByText(/Live update covers/)).toBeNull());
  });

  it("names both remedies when the budget is spent and the kernel refused watches", async () => {
    render(
      <LocationProvider>
        <RootView slug="n" />
      </LocationProvider>,
    );
    await screen.findByText("docs");
    FakeEventSource.last!.emitStatus({ ...limited, failed: 12, unwatched: 4510 });
    const notice = await screen.findByText(/Live update covers/);
    expect(notice.textContent).toContain("raise max_watches above 8,192");
    expect(notice.textContent).toContain("fs.inotify.max_user_watches");
  });

  it("says when the root has no live update at all", async () => {
    render(
      <LocationProvider>
        <RootView slug="n" />
      </LocationProvider>,
    );
    await screen.findByText("docs");
    const es = FakeEventSource.last!;
    es.readyState = 2;
    es.onerror!();
    expect((await screen.findByText(/Live update is not available/)).textContent).toContain("reload");
  });

  it("stays quiet while a dropped stream is still reconnecting", async () => {
    render(
      <LocationProvider>
        <RootView slug="n" />
      </LocationProvider>,
    );
    await screen.findByText("docs");
    const es = FakeEventSource.last!;
    es.readyState = 0;
    es.onerror!();
    await waitFor(() => expect(screen.queryByText(/Live update is not available/)).toBeNull());
  });

  it("says how much of the root is watched, and how to cover the rest", async () => {
    render(
      <LocationProvider>
        <RootView slug="n" />
      </LocationProvider>,
    );
    await screen.findByText("docs");
    FakeEventSource.last!.emitStatus(limited);
    const notice = await screen.findByText(/Live update covers/);
    expect(notice.textContent).toContain("8,192 of 12,690 directories");
    expect(notice.textContent).toContain("raise max_watches above 8,192");
  });
});

describe("RootView drawer at narrow widths", () => {
  // The stylesheet, not the markup, decides whether the drawer's chrome is
  // on screen: here everything is in the document at every width, so these
  // exercise the behaviour the media query switches on.
  const panes = () => document.querySelector(".panes")!;
  const burger = () => screen.getByLabelText("Open navigation");
  const magnifier = () => screen.getByLabelText("Search and tags");
  const backdrop = () => document.querySelector(".drawer-backdrop");

  async function mounted(url = "/r/n/docs/a.md") {
    mount(url);
    await waitFor(() => expect(screen.getByText("b.md")).toBeTruthy());
  }

  it("starts closed, with no dialog and nothing to tap through", async () => {
    await mounted();
    expect(panes().classList.contains("open")).toBe(false);
    expect(panes().getAttribute("role")).toBeNull();
    expect(panes().getAttribute("aria-modal")).toBeNull();
    expect(burger().getAttribute("aria-expanded")).toBe("false");
    expect(backdrop()).toBeNull();
  });

  it("the burger opens it on the notes tab as a labelled modal, with focus inside", async () => {
    await mounted();
    fireEvent.click(burger());
    expect(panes().classList.contains("open")).toBe(true);
    expect(panes().getAttribute("data-tab")).toBe("notes");
    expect(panes().getAttribute("role")).toBe("dialog");
    expect(panes().getAttribute("aria-modal")).toBe("true");
    expect(panes().getAttribute("aria-label")).toBeTruthy();
    expect(burger().getAttribute("aria-expanded")).toBe("true");
    expect(burger().getAttribute("aria-controls")).toBe(panes().id);
    expect(panes().contains(document.activeElement)).toBe(true);
    expect(backdrop()).toBeTruthy();
  });

  it("the magnifier opens the same drawer on the search tab, in the search box", async () => {
    await mounted();
    fireEvent.click(magnifier());
    expect(panes().classList.contains("open")).toBe(true);
    expect(panes().getAttribute("data-tab")).toBe("find");
    expect(document.activeElement).toBe(screen.getByLabelText("Search notes"));
  });

  it("Escape closes it and hands focus back to the button that opened it", async () => {
    await mounted();
    fireEvent.click(burger());
    fireEvent.keyDown(document, { key: "Escape" });
    expect(panes().classList.contains("open")).toBe(false);
    expect(document.activeElement).toBe(burger());
  });

  it("a tap on the backdrop closes it", async () => {
    await mounted();
    fireEvent.click(magnifier());
    fireEvent.click(backdrop()!);
    expect(panes().classList.contains("open")).toBe(false);
    expect(document.activeElement).toBe(magnifier());
  });

  it("the close button closes it", async () => {
    await mounted();
    fireEvent.click(burger());
    fireEvent.click(screen.getByLabelText("Close navigation"));
    expect(panes().classList.contains("open")).toBe(false);
  });

  it("selecting a note closes it, since the drawer covers the note", async () => {
    await mounted();
    fireEvent.click(burger());
    fireEvent.click(screen.getByText("b.md"));
    expect(panes().classList.contains("open")).toBe(false);
  });

  it("selecting a search hit closes it", async () => {
    await mounted();
    fireEvent.click(magnifier());
    fireEvent.input(screen.getByLabelText("Search notes"), { target: { value: "needle" } });
    const hit = await screen.findByText("needle");
    expect(panes().classList.contains("open")).toBe(true);
    fireEvent.click(hit);
    expect(panes().classList.contains("open")).toBe(false);
  });

  it("selecting a tag shows the notes tab rather than closing, so the filter is visible", async () => {
    await mounted();
    fireEvent.click(magnifier());
    fireEvent.click(document.querySelector(".tags .tag")!);
    expect(panes().classList.contains("open")).toBe(true);
    expect(panes().getAttribute("data-tab")).toBe("notes");
  });

  it("keeps the navigator's expanded directories across open and close", async () => {
    await mounted();
    fireEvent.click(burger());
    fireEvent.click(screen.getByText("docs"));
    expect(screen.queryByText("a.md")).toBeNull();
    fireEvent.keyDown(document, { key: "Escape" });
    fireEvent.click(burger());
    expect(screen.queryByText("a.md")).toBeNull();
    fireEvent.click(screen.getByText("docs"));
    expect(screen.getByText("a.md")).toBeTruthy();
  });

  it("closes when the window crosses back to the wide layout", async () => {
    await mounted();
    fireEvent.click(burger());
    expect(panes().classList.contains("open")).toBe(true);
    // A resize inside the narrow range leaves it alone: the stylesheet still
    // makes the wrapper a box, so there is still a drawer to be open.
    fireEvent(window, new Event("resize"));
    await new Promise((r) => requestAnimationFrame(() => r(null)));
    expect(panes().classList.contains("open")).toBe(true);
    // Past the breakpoint the wrapper is `display: contents` and has no
    // dialog to be. jsdom computes no media queries, so the stylesheet's
    // answer is stood in for.
    const real = window.getComputedStyle;
    vi.stubGlobal("getComputedStyle", (el: Element) =>
      el === panes() ? ({ display: "contents" } as CSSStyleDeclaration) : real(el),
    );
    fireEvent(window, new Event("resize"));
    await waitFor(() => expect(panes().classList.contains("open")).toBe(false));
    expect(panes().getAttribute("role")).toBeNull();
    expect(panes().getAttribute("aria-modal")).toBeNull();
  });

  it("cycles Tab within the open drawer, skipping the tab that is not shown", async () => {
    await mounted();
    fireEvent.click(burger());
    const inside = [...panes().querySelectorAll<HTMLElement>("a[href], button, input")].filter((n) => !n.closest(".side"));
    const last = inside[inside.length - 1];
    last.focus();
    fireEvent.keyDown(panes(), { key: "Tab" });
    expect(document.activeElement).toBe(inside[0]);
    fireEvent.keyDown(panes(), { key: "Tab", shiftKey: true });
    expect(document.activeElement).toBe(last);
  });
});

describe("RootView top bar", () => {
  // Every element in the top bar has an answer in the stylesheet's narrow
  // block: hidden (the path), clamped to what fits (the root name, the
  // unsaved-draft notice), or a fixed tap target (the two toggles, the
  // chip). Nothing in jsdom computes those rules, so what this pins is the
  // set: an element added here without a decision about the compact bar
  // fails this, which is how an unclamped draft path once pushed the
  // magnifier off an iPhone 14.
  it("holds only the elements the narrow layout accounts for", async () => {
    mount("/r/n/docs/a.md?tag=x");
    await waitFor(() => expect(screen.getByText("b.md")).toBeTruthy());
    // A draft on another note, so the unsaved notice is in the bar too.
    const buried = getSession("n", "docs/deep/deeper/deepest/buried.md");
    await buried.open();
    buried.edit("a draft that never reached the file");
    await waitFor(() => expect(document.querySelector(".unsaved-drafts")).toBeTruthy());
    expect([...document.querySelector(".topbar")!.children].map((n) => n.className)).toEqual([
      "drawer-toggle nav-toggle",
      "brand",
      "root-name",
      "path",
      "tag-chip",
      "unsaved-drafts",
      "drawer-toggle find-toggle",
    ]);
    await buried.flush();
  });
});

describe("RootView tag chip", () => {
  it("is absent with no filter and names the active tag, clearing it", async () => {
    mount("/r/n/docs/a.md");
    await waitFor(() => expect(screen.getByText("b.md")).toBeTruthy());
    expect(document.querySelector(".tag-chip")).toBeNull();
    cleanup();

    mount("/r/n/docs/a.md?tag=x");
    await waitFor(() => expect(screen.getByText("b.md")).toBeTruthy());
    const chip = document.querySelector<HTMLAnchorElement>(".topbar .tag-chip")!;
    expect(chip.textContent).toContain("#x");
    expect(chip.getAttribute("aria-label")).toBe("Clear the tag filter x");
    expect(chip.getAttribute("href")).toBe("/r/n/docs/a.md");
  });
});
