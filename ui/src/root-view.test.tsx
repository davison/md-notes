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
const methods: { method: string; url: string }[] = [];
/** The daemon's markdown, which create and delete change under the tree. */
let files: string[] = [];

/** The tree endpoint's answer, built from `files` so it moves with them. */
function treeOf(paths: string[]) {
  const root = { name: "", path: "", dir: true, children: [] as Record<string, unknown>[] };
  const dirs = new Map<string, Record<string, unknown>>();
  for (const path of paths) {
    const cut = path.lastIndexOf("/");
    const name = path.slice(cut + 1);
    let into = root.children;
    if (cut > 0) {
      const dir = path.slice(0, cut);
      let node = dirs.get(dir);
      if (!node) {
        node = { name: dir, path: dir, dir: true, children: [] };
        dirs.set(dir, node);
        root.children.push(node);
      }
      into = node.children as Record<string, unknown>[];
    }
    into.push({ name, path, dir: false });
  }
  return root;
}

function mockApi() {
  vi.stubGlobal(
    "fetch",
    vi.fn((url: string, init?: RequestInit) => {
      const method = init?.method ?? "GET";
      calls.push(url);
      methods.push({ method, url });
      const path = decodeURIComponent(url.replace("/api/r/n/source/", ""));
      if (url.includes("/source/") && method === "POST") {
        if (files.includes(path))
          return Promise.resolve({
            ok: false,
            status: 409,
            statusText: "Conflict",
            json: () => Promise.resolve({ code: "exists", error: "a note by that name already exists" }),
          } as Response);
        files.push(path);
        return Promise.resolve({
          ok: true,
          status: 201,
          statusText: "Created",
          json: () => Promise.resolve({ root: "n", path, source: "", revision: "r1" }),
        } as Response);
      }
      if (url.includes("/source/") && method === "DELETE") {
        files = files.filter((f) => f !== path);
        return Promise.resolve({ ok: true, status: 204, statusText: "No Content" } as Response);
      }
      let body: unknown;
      if (url === "/api/roots") body = { roots: [{ slug: "n", path: "/n", kind: "notes" }] };
      else if (url === "/api/r/n/tree") body = treeOf(files);
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
  methods.length = 0;
  files = ["docs/a.md", "b.md"];
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
    const notice = (await screen.findAllByText(/Live update covers/))[0];
    expect(notice.textContent).toContain("raise max_watches above 8,192");
    expect(notice.textContent).toContain("fs.inotify.max_user_watches");
  });

  it("puts the notice in the navigator and above the note, one for each width", async () => {
    render(
      <LocationProvider>
        <RootView slug="n" />
      </LocationProvider>,
    );
    await screen.findByText("docs");
    const es = FakeEventSource.last!;
    es.readyState = 2;
    es.onerror!();
    // The stylesheet shows the navigator's copy at wide widths and the
    // note's at narrow ones, where the navigator is behind the drawer.
    await waitFor(() => expect(document.querySelectorAll(".notice")).toHaveLength(2));
    expect(document.querySelector(".nav > .notice")).toBeTruthy();
    expect(document.querySelector(".note > .notice")).toBeTruthy();
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
    expect((await screen.findAllByText(/Live update is not available/))[0].textContent).toContain("reload");
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
    await waitFor(() => expect(screen.queryAllByText(/Live update is not available/)).toHaveLength(0));
  });

  it("says how much of the root is watched, and how to cover the rest", async () => {
    render(
      <LocationProvider>
        <RootView slug="n" />
      </LocationProvider>,
    );
    await screen.findByText("docs");
    FakeEventSource.last!.emitStatus(limited);
    const notice = (await screen.findAllByText(/Live update covers/))[0];
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
      "new-note",
      "root-name",
      "path",
      "topbar-spacer",
      "tag-chip",
      "unsaved-drafts",
      "drawer-toggle find-toggle",
      "settings",
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

function mountAt(url: string, note?: string) {
  history.replaceState(null, "", url);
  return render(
    <LocationProvider>
      <RootView slug="n" note={note} />
    </LocationProvider>,
  );
}
const treeCalls = () => calls.filter((u) => u === "/api/r/n/tree").length;
const submit = () => fireEvent.submit(document.querySelector(".modal form")!);

/**
 * Creating from the top bar. Nothing here refreshes the tree by hand —
 * the daemon's change batch is what updates it, exactly as it does for a
 * note written by another tool.
 */
describe("creating a note", () => {
  const posts = () => methods.filter((c) => c.method === "POST");
  const nameBox = () => screen.getByLabelText("Title or path");

  it("puts the create control in the top bar, not in the navigator", async () => {
    // The navigator's list scrolls; the top bar does not, and on a long
    // tree the control used to be above everything and out of sight (#85).
    mountAt("/r/n/docs/a.md", "docs/a.md");
    await waitFor(() => expect(screen.getByText("b.md")).toBeTruthy());
    expect(document.querySelector(".topbar .new-note")).toBeTruthy();
    expect(document.querySelector(".nav .new-note")).toBeNull();
    // Named the same whether or not the stylesheet is showing its label.
    const control = screen.getByRole("button", { name: "New note" });
    expect(control.querySelector(".new-note-label")!.textContent).toBe("New note");
    expect(control.closest(".topbar")).toBeTruthy();
  });

  it("creates a bare title in the open note's folder and opens it in the editor", async () => {
    mountAt("/r/n/docs/a.md", "docs/a.md");
    await waitFor(() => expect(screen.getByText("b.md")).toBeTruthy());

    fireEvent.click(screen.getByRole("button", { name: "New note" }));
    // The prompt says where a bare title lands: the open note's folder.
    expect(document.querySelector(".modal-folder")!.textContent).toBe("docs");
    fireEvent.input(nameBox(), { target: { value: "Shopping" } });
    submit();

    await waitFor(() => expect(posts()).toHaveLength(1));
    expect(posts()[0].url).toBe("/api/r/n/source/docs/Shopping.md");
    await waitFor(() => expect(location.pathname).toBe("/r/n/docs/Shopping.md"));
    expect(document.querySelector(".modal")).toBeNull();
  });

  it("puts a bare title in the root when no note is open", async () => {
    mountAt("/r/n/");
    await waitFor(() => expect(screen.getByText("b.md")).toBeTruthy());
    fireEvent.click(screen.getByRole("button", { name: "New note" }));
    fireEvent.input(nameBox(), { target: { value: "Shopping" } });
    submit();
    await waitFor(() => expect(posts()).toHaveLength(1));
    expect(posts()[0].url).toBe("/api/r/n/source/Shopping.md");
  });

  it("lets the change batch put the new note in the navigator, with no refresh of its own", async () => {
    mountAt("/r/n/docs/a.md", "docs/a.md");
    await waitFor(() => expect(screen.getByText("b.md")).toBeTruthy());
    const before = treeCalls();

    fireEvent.click(screen.getByRole("button", { name: "New note" }));
    fireEvent.input(nameBox(), { target: { value: "Shopping" } });
    submit();
    await waitFor(() => expect(posts()).toHaveLength(1));
    // The create path asks for no tree of its own.
    expect(treeCalls()).toBe(before);
    expect(screen.queryByText("Shopping.md")).toBeNull();

    FakeEventSource.last!.emit(["docs/Shopping.md"]);
    await waitFor(() => expect(screen.getByText("Shopping.md")).toBeTruthy());
  });
});

/** Deleting from the note bar, and where the shell goes afterwards. */
describe("deleting the open note", () => {
  it("deletes it after the confirmation and leaves it for the root's home", async () => {
    mountAt("/r/n/docs/a.md", "docs/a.md");
    await waitFor(() => expect(screen.getByText("b.md")).toBeTruthy());
    const before = treeCalls();

    fireEvent.click(screen.getByRole("button", { name: "Delete" }));
    expect(methods.filter((c) => c.method === "DELETE")).toHaveLength(0);
    submit();
    await waitFor(() => expect(location.pathname).toBe("/r/n/"));
    expect(methods.filter((c) => c.method === "DELETE")).toHaveLength(1);

    // And the navigator loses it the same way it gained the other one.
    expect(treeCalls()).toBe(before);
    FakeEventSource.last!.emit(["docs/a.md"]);
    await waitFor(() => expect(screen.queryByText("a.md")).toBeNull());
  });
});
