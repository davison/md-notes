import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/preact";
import { EditorView } from "@codemirror/view";
import { LocationProvider } from "preact-iso";
import { App } from "./app";
import { Home } from "./home";
import { NotePane } from "./note-pane";
import { RootView } from "./root-view";
import { resetSessions, type SessionState } from "./session";
import { CONFLICT_MARKER, FALLBACK_TITLE, UNSAVED_MARKER, fileTitle, marker, noteTabTitle, rootTabTitle } from "./title";

/** A session state with only the fields the title rules read. */
function state(patch: Partial<SessionState> = {}): SessionState {
  return { status: "clean", base: null, draft: "", error: null, conflict: null, generation: 0, ...patch };
}

describe("the title rules", () => {
  it("names a note by the daemon's title, and by its file name until one arrives", () => {
    expect(noteTabTitle("docs/a.md", "A Heading", state())).toBe("A Heading");
    expect(noteTabTitle("docs/a.md", null, state())).toBe("a");
    // A title that is only whitespace is no title.
    expect(noteTabTitle("docs/a.md", "   ", state())).toBe("a");
  });

  it("falls back to the file name the daemon would use: the stem, not the whole name", () => {
    expect(fileTitle("a.md")).toBe("a");
    expect(fileTitle("docs/deep/file-name.md")).toBe("file-name");
    expect(fileTitle("docs/a.b.md")).toBe("a.b");
    expect(fileTitle("no-extension")).toBe("no-extension");
    // All extension and no name: the daemon's trim would leave nothing.
    expect(fileTitle(".hidden")).toBe(".hidden");
  });

  it("gives the root's slug only to a page with no note", () => {
    const root = { slug: "n", path: "/n", kind: "notes" } as const;
    expect(rootTabTitle(root, "n", false)).toBe("n");
    // A note is named by the pane below, which knows it.
    expect(rootTabTitle(root, "n", true)).toBe(null);
    // Still loading: the slug, but never over a note that is about to be named.
    expect(rootTabTitle(undefined, "n", false)).toBe("n");
    expect(rootTabTitle(undefined, "n", true)).toBe(null);
    // No such root: a page with neither a note nor a root.
    expect(rootTabTitle(null, "n", false)).toBe(FALLBACK_TITLE);
    expect(rootTabTitle(null, "n", true)).toBe(FALLBACK_TITLE);
  });

  it("marks the unsaved states, and a conflict over them", () => {
    expect(marker(state({ status: "clean" }))).toBe("");
    expect(marker(state({ status: "loading" }))).toBe("");
    expect(marker(state({ status: "error" }))).toBe("");
    for (const status of ["pending", "saving", "failed"] as const) {
      expect(marker(state({ status }))).toBe(UNSAVED_MARKER + " ");
    }
    expect(marker(state({ status: "conflict" }))).toBe(CONFLICT_MARKER + " ");
    expect(noteTabTitle("a.md", "A", state({ status: "pending" }))).toBe(`${UNSAVED_MARKER} A`);
    expect(noteTabTitle("a.md", "A", state({ status: "conflict" }))).toBe(`${CONFLICT_MARKER} A`);
  });
});

/** A daemon with one root and one note, rendered and as source. */
let file: { source: string; revision: string } | null;
let noteTitle: string;
let roots: { slug: string; path: string; kind: string }[];

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
  close() {}
}

function mockApi() {
  vi.stubGlobal(
    "fetch",
    vi.fn((url: string, init?: RequestInit) => {
      const method = init?.method ?? "GET";
      const json = (status: number, body: unknown) =>
        Promise.resolve({ ok: status < 300, status, statusText: "S", json: () => Promise.resolve(body) } as Response);
      if (url === "/api/roots") return json(200, { roots });
      if (url.startsWith("/api/r/n/tree")) return json(200, { name: "", path: "", dir: true, children: [] });
      if (url.startsWith("/api/r/n/tags")) return json(200, { tags: [] });
      if (url.startsWith("/api/r/n/note/")) {
        if (!file) return json(404, { error: "not found" });
        return json(200, { path: "docs/a.md", title: noteTitle, html: `<p>${file.source.trim()}</p>` });
      }
      if (url.startsWith("/api/r/n/source/")) {
        if (!file) return json(404, { code: "not_found", error: "gone" });
        if (method === "PUT") {
          const body = JSON.parse(init!.body as string) as { source: string; revision: string };
          if (body.revision !== file.revision) return json(409, { code: "conflict", error: "note changed" });
          file = { source: body.source, revision: file.revision + "+" };
        }
        return json(200, file);
      }
      return json(404, { error: "no" });
    }),
  );
}

beforeEach(() => {
  file = { source: "body\n", revision: "r1" };
  noteTitle = "A Rendered Heading";
  roots = [{ slug: "n", path: "/n", kind: "notes" }];
  localStorage.clear();
  resetSessions();
  document.title = "unset";
  vi.stubGlobal("EventSource", FakeEventSource);
  mockApi();
});
afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

const ctrlE = { key: "e", ctrlKey: true };
const editorText = () => document.querySelector(".cm-content")?.textContent ?? "";
/** Replaces the mounted editor's document, as typing would. */
function type(text: string) {
  const view = EditorView.findFromDOM(document.querySelector(".cm-editor")!)!;
  view.dispatch({ changes: { from: 0, to: view.state.doc.length, insert: text } });
}

describe("the tab title over an open note", () => {
  it("is the note's own title, and the file name while the daemon has not answered", async () => {
    render(<NotePane slug="n" path="docs/a.md" />);
    // Before the render lands, the pane can only name the file.
    expect(document.title).toBe("a");
    await waitFor(() => expect(document.title).toBe("A Rendered Heading"));
  });

  it("gives up the title of a note that stops rendering", async () => {
    // Rendered first, so there is a title to give up: a note deleted under
    // an open tab must not leave its name on it.
    const { rerender } = render(<NotePane slug="n" path="docs/a.md" version={0} />);
    await waitFor(() => expect(document.title).toBe("A Rendered Heading"));
    file = null;
    rerender(<NotePane slug="n" path="docs/a.md" version={1} />);
    await waitFor(() => expect(screen.getByText("not found")).toBeTruthy());
    await waitFor(() => expect(document.title).toBe("a"));
  });

  it("follows a live update of the note's title", async () => {
    const { rerender } = render(<NotePane slug="n" path="docs/a.md" version={0} />);
    await waitFor(() => expect(document.title).toBe("A Rendered Heading"));
    noteTitle = "Renamed On Disk";
    file = { source: "body\n", revision: "r2" };
    rerender(<NotePane slug="n" path="docs/a.md" version={1} />);
    await waitFor(() => expect(document.title).toBe("Renamed On Disk"));
  });

  it("takes the unsaved marker while a draft is outstanding and drops it on the save", async () => {
    render(<NotePane slug="n" path="docs/a.md" />);
    await waitFor(() => expect(document.title).toBe("A Rendered Heading"));
    fireEvent.keyDown(document.body, ctrlE);
    await waitFor(() => expect(editorText()).toContain("body"));
    expect(document.title).toBe("A Rendered Heading");
    type("edited\n");
    await waitFor(() => expect(document.title).toBe(`${UNSAVED_MARKER} A Rendered Heading`));
    await waitFor(() => expect(document.title).toBe("A Rendered Heading"), { timeout: 3000 });
    expect(file!.source).toBe("edited\n");
  });

  it("follows a live update that lands while the editor is open", async () => {
    const { rerender } = render(<NotePane slug="n" path="docs/a.md" version={0} />);
    await waitFor(() => expect(document.title).toBe("A Rendered Heading"));
    fireEvent.keyDown(document.body, ctrlE);
    await waitFor(() => expect(editorText()).toContain("body"));
    // The rendered view is unmounted, so nothing there can report the title.
    expect(document.querySelector(".note-body")).toBeNull();
    noteTitle = "Retitled While Editing";
    file = { source: "# Retitled While Editing\n", revision: "r2" };
    rerender(<NotePane slug="n" path="docs/a.md" version={1} />);
    await waitFor(() => expect(editorText()).toContain("Retitled While Editing"));
    await waitFor(() => expect(document.title).toBe("Retitled While Editing"));
  });

  it("follows the file when a conflict is resolved by loading it", async () => {
    const { rerender } = render(<NotePane slug="n" path="docs/a.md" version={0} />);
    await waitFor(() => expect(document.title).toBe("A Rendered Heading"));
    fireEvent.keyDown(document.body, ctrlE);
    await waitFor(() => expect(editorText()).toContain("body"));
    type("mine\n");
    noteTitle = "Theirs Heading";
    file = { source: "# Theirs Heading\n", revision: "r9" };
    rerender(<NotePane slug="n" path="docs/a.md" version={1} />);
    await waitFor(() => expect(document.title).toBe(`${CONFLICT_MARKER} A Rendered Heading`));
    fireEvent.click(screen.getByRole("button", { name: "Load the file" }));
    await waitFor(() => expect(editorText()).toContain("Theirs Heading"));
    // The marker goes with the conflict, and the title with the draft it named.
    await waitFor(() => expect(document.title).toBe("Theirs Heading"));
  });

  it("asks for the title of a note opened straight into the editor", async () => {
    // A recovered draft opens the editor without ever rendering the note.
    localStorage.setItem("mdn:draft:n\u0000docs/a.md", JSON.stringify({ revision: "r1", draft: "mine\n" }));
    render(<NotePane slug="n" path="docs/a.md" />);
    await waitFor(() => expect(editorText()).toContain("mine"));
    await waitFor(() => expect(document.title).toBe(`${UNSAVED_MARKER} A Rendered Heading`));
  });

  it("takes the conflict marker when the file changes under a draft", async () => {
    const { rerender } = render(<NotePane slug="n" path="docs/a.md" version={0} />);
    await waitFor(() => expect(document.title).toBe("A Rendered Heading"));
    fireEvent.keyDown(document.body, ctrlE);
    await waitFor(() => expect(editorText()).toContain("body"));
    type("mine\n");
    file = { source: "theirs\n", revision: "r9" };
    rerender(<NotePane slug="n" path="docs/a.md" version={1} />);
    await waitFor(() => expect(screen.getByRole("alert")).toBeTruthy());
    await waitFor(() => expect(document.title).toBe(`${CONFLICT_MARKER} A Rendered Heading`));
  });
});

describe("the tab title on pages with no note", () => {
  function mountRoot(slug: string, note?: string) {
    history.replaceState(null, "", note ? `/r/${slug}/${note}` : `/r/${slug}/`);
    return render(
      <LocationProvider>
        <RootView slug={slug} note={note} />
      </LocationProvider>,
    );
  }

  it("is the root's slug when a root is open with no note", async () => {
    mountRoot("n");
    await waitFor(() => expect(screen.getByText("Select a note.")).toBeTruthy());
    await waitFor(() => expect(document.title).toBe("n"));
  });

  it("is the fallback for an unknown root", async () => {
    mountRoot("nope");
    await waitFor(() => expect(screen.getByText("Unknown root")).toBeTruthy());
    await waitFor(() => expect(document.title).toBe(FALLBACK_TITLE));
  });

  it("is the fallback on the home page", async () => {
    render(<Home />);
    await waitFor(() => expect(screen.getByText("Notes")).toBeTruthy());
    await waitFor(() => expect(document.title).toBe(FALLBACK_TITLE));
  });

  it("is restored when a note is left for the home page", async () => {
    history.replaceState(null, "", "/r/n/docs/a.md");
    render(<App />);
    await waitFor(() => expect(document.title).toBe("A Rendered Heading"));
    fireEvent.click(screen.getByText("mdn"));
    await waitFor(() => expect(screen.getByText("Notes")).toBeTruthy());
    await waitFor(() => expect(document.title).toBe(FALLBACK_TITLE));
  });
});
