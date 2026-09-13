import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/preact";
import { EditorView } from "@codemirror/view";
import { NotePane, UnsavedDrafts, isToggleKey, useUnsavedGuard } from "./note-pane";
import { getSession, resetSessions } from "./session";

/** A daemon with one note, rendered and as source; PUT applies the save. */
let file: { source: string; revision: string } | null;
let putStatus: number | null;
const calls: { method: string; url: string }[] = [];

function mockApi() {
  vi.stubGlobal(
    "fetch",
    vi.fn((url: string, init?: RequestInit) => {
      const method = init?.method ?? "GET";
      calls.push({ method, url });
      const json = (status: number, body: unknown) =>
        Promise.resolve({ ok: status < 300, status, statusText: "S", json: () => Promise.resolve(body) } as Response);
      if (url.startsWith("/api/r/n/note/")) {
        if (!file) return json(404, { error: "not found" });
        return json(200, { path: "a.md", title: "A", html: `<p>${file.source.trim()}</p>` });
      }
      if (url.startsWith("/api/r/n/source/")) {
        if (!file) return json(404, { code: "not_found", error: "note or root no longer exists" });
        if (method === "PUT") {
          if (putStatus) return json(putStatus, { code: "io_error", error: "disk full" });
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
  putStatus = null;
  calls.length = 0;
  localStorage.clear();
  resetSessions();
  mockApi();
});
afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  vi.useRealTimers();
});

const ctrlE = { key: "e", ctrlKey: true };
const bar = () => screen.getByText(/^(Editing|Viewing)$/).textContent;
const status = () => document.querySelector(".save-status")?.textContent ?? "";
const editorText = () => document.querySelector(".cm-content")?.textContent ?? "";
/** Replaces the mounted editor's document, as typing would. */
function type(text: string) {
  const view = EditorView.findFromDOM(document.querySelector(".cm-editor")!)!;
  view.dispatch({ changes: { from: 0, to: view.state.doc.length, insert: text } });
}

describe("isToggleKey", () => {
  it("is Ctrl+E alone", () => {
    expect(isToggleKey(new KeyboardEvent("keydown", { key: "e", ctrlKey: true }))).toBe(true);
    expect(isToggleKey(new KeyboardEvent("keydown", { key: "E", ctrlKey: true }))).toBe(true);
    expect(isToggleKey(new KeyboardEvent("keydown", { key: "e" }))).toBe(false);
    expect(isToggleKey(new KeyboardEvent("keydown", { key: "e", ctrlKey: true, shiftKey: true }))).toBe(false);
    expect(isToggleKey(new KeyboardEvent("keydown", { key: "e", ctrlKey: true, altKey: true }))).toBe(false);
    expect(isToggleKey(new KeyboardEvent("keydown", { key: "e", metaKey: true }))).toBe(false);
  });
});

describe("NotePane", () => {
  it("starts in view mode and toggles to the editor on Ctrl+E, reading the source once", async () => {
    render(<NotePane slug="n" path="a.md" />);
    await waitFor(() => expect(screen.getByText("body")).toBeTruthy());
    expect(bar()).toBe("Viewing");
    expect(calls.filter((c) => c.url.includes("/source/"))).toHaveLength(0);
    fireEvent.keyDown(document.body, ctrlE);
    await waitFor(() => expect(editorText()).toContain("body"));
    expect(bar()).toBe("Editing");
    expect(status()).toBe("Saved");
    expect(calls.filter((c) => c.url.includes("/source/"))).toHaveLength(1);
  });

  it("the toggle button does the same, and plain keys do not toggle", async () => {
    render(<NotePane slug="n" path="a.md" />);
    await waitFor(() => expect(screen.getByText("body")).toBeTruthy());
    fireEvent.click(screen.getByRole("button", { name: "Edit" }));
    await waitFor(() => expect(editorText()).toContain("body"));
    fireEvent.keyDown(document.querySelector(".cm-content")!, { key: "e" });
    fireEvent.keyDown(document.querySelector(".cm-content")!, { key: "e", shiftKey: true, ctrlKey: true });
    expect(bar()).toBe("Editing");
    fireEvent.click(screen.getByRole("button", { name: "View" }));
    await waitFor(() => expect(bar()).toBe("Viewing"));
  });

  it("Ctrl+E inside the editor toggles back and is consumed", async () => {
    render(<NotePane slug="n" path="a.md" />);
    await waitFor(() => expect(screen.getByText("body")).toBeTruthy());
    fireEvent.keyDown(document.body, ctrlE);
    await waitFor(() => expect(editorText()).toContain("body"));
    const consumed = !fireEvent.keyDown(document.querySelector(".cm-content")!, ctrlE);
    expect(consumed).toBe(true);
    await waitFor(() => expect(bar()).toBe("Viewing"));
  });

  it("shows the save states and refreshes the render after a save", async () => {
    vi.useFakeTimers();
    render(<NotePane slug="n" path="a.md" />);
    await vi.waitFor(() => expect(screen.getByText("body")).toBeTruthy());
    fireEvent.keyDown(document.body, ctrlE);
    await vi.waitFor(() => expect(editorText()).toContain("body"));
    type("body more\n");
    await vi.waitFor(() => expect(status()).toBe("Unsaved changes"));
    await vi.advanceTimersByTimeAsync(1000);
    await vi.waitFor(() => expect(status()).toBe("Saved"));
    expect(file?.source).toBe("body more\n");
    // Back to view: the render is refetched for the new content.
    fireEvent.keyDown(document.body, ctrlE);
    await vi.waitFor(() => expect(screen.getByText("body more")).toBeTruthy());
  });

  it("leaving the editor saves at once", async () => {
    render(<NotePane slug="n" path="a.md" />);
    await waitFor(() => expect(screen.getByText("body")).toBeTruthy());
    fireEvent.keyDown(document.body, ctrlE);
    await waitFor(() => expect(editorText()).toContain("body"));
    type("now\n");
    fireEvent.keyDown(document.body, ctrlE);
    await waitFor(() => expect(file?.source).toBe("now\n"));
  });

  it("a failed save keeps the draft, says why, and retries from the bar", async () => {
    render(<NotePane slug="n" path="a.md" />);
    await waitFor(() => expect(screen.getByText("body")).toBeTruthy());
    fireEvent.keyDown(document.body, ctrlE);
    await waitFor(() => expect(editorText()).toContain("body"));
    putStatus = 500;
    type("kept\n");
    await getSession("n", "a.md").flush();
    await waitFor(() => expect(status()).toContain("Save failed: disk full. Draft kept."));
    expect(editorText()).toContain("kept");
    // The draft is still there in view mode, with the retry.
    fireEvent.keyDown(document.body, ctrlE);
    await waitFor(() => expect(bar()).toBe("Viewing"));
    expect(status()).toContain("Save failed");
    putStatus = null;
    fireEvent.click(screen.getByRole("button", { name: "Retry" }));
    await waitFor(() => expect(status()).toBe("Saved"));
    expect(file?.source).toBe("kept\n");
  });

  it("navigating away saves pending edits", async () => {
    const r = render(<NotePane slug="n" path="a.md" />);
    await waitFor(() => expect(screen.getByText("body")).toBeTruthy());
    fireEvent.keyDown(document.body, ctrlE);
    await waitFor(() => expect(editorText()).toContain("body"));
    type("before leaving\n");
    r.unmount();
    await waitFor(() => expect(file?.source).toBe("before leaving\n"));
  });

  it("a note with a retained draft opens in the editor", async () => {
    const session = getSession("n", "a.md");
    await session.open();
    putStatus = 500;
    session.edit("retained\n");
    await session.flush();
    expect(session.state.status).toBe("failed");
    render(<NotePane slug="n" path="a.md" />);
    await waitFor(() => expect(editorText()).toContain("retained"));
    expect(bar()).toBe("Editing");
  });

  it("a draft left in storage by a previous page opens in the editor and is recovered", async () => {
    localStorage.setItem("mdn:draft:n\0a.md", JSON.stringify({ revision: "r1", draft: "from storage\n" }));
    render(<NotePane slug="n" path="a.md" />);
    await waitFor(() => expect(editorText()).toContain("from storage"));
    expect(status()).toBe("Unsaved changes");
  });

  it("a live change while viewing an opened note rechecks it", async () => {
    const { rerender } = render(<NotePane slug="n" path="a.md" version={0} />);
    await waitFor(() => expect(screen.getByText("body")).toBeTruthy());
    fireEvent.keyDown(document.body, ctrlE);
    await waitFor(() => expect(editorText()).toContain("body"));
    file = { source: "changed on disk\n", revision: "r9" };
    rerender(<NotePane slug="n" path="a.md" version={1} />);
    await waitFor(() => expect(editorText()).toContain("changed on disk"));
    expect(status()).toBe("Saved");
  });

  it("a live change under a dirty draft shows the conflict banner with its actions", async () => {
    const { rerender } = render(<NotePane slug="n" path="a.md" version={0} />);
    await waitFor(() => expect(screen.getByText("body")).toBeTruthy());
    fireEvent.keyDown(document.body, ctrlE);
    await waitFor(() => expect(editorText()).toContain("body"));
    type("mine\n");
    file = { source: "theirs\n", revision: "r9" };
    rerender(<NotePane slug="n" path="a.md" version={1} />);
    await waitFor(() => expect(screen.getByRole("alert")).toBeTruthy());
    expect(status()).toBe("Conflict: draft kept");
    expect(editorText()).toContain("mine");
    // View mode shows the file as it is now; the draft and banner remain.
    fireEvent.keyDown(document.body, ctrlE);
    await waitFor(() => expect(screen.getByText("theirs")).toBeTruthy());
    expect(screen.getByRole("alert")).toBeTruthy();
    fireEvent.keyDown(document.body, ctrlE);
    await waitFor(() => expect(editorText()).toContain("mine"));
    const write = vi.fn(() => Promise.resolve());
    vi.stubGlobal("navigator", { ...navigator, clipboard: { writeText: write } });
    fireEvent.click(screen.getByRole("button", { name: "Copy draft" }));
    expect(write).toHaveBeenCalledWith("mine\n");
    await waitFor(() => expect(screen.getByRole("button", { name: "Copied" })).toBeTruthy());
    fireEvent.click(screen.getByRole("button", { name: "Load the file" }));
    await waitFor(() => expect(editorText()).toContain("theirs"));
    expect(screen.queryByRole("alert")).toBeNull();
    expect(status()).toBe("Saved");
  });

  it("Keep my draft saves over the changed file", async () => {
    const { rerender } = render(<NotePane slug="n" path="a.md" version={0} />);
    await waitFor(() => expect(screen.getByText("body")).toBeTruthy());
    fireEvent.keyDown(document.body, ctrlE);
    await waitFor(() => expect(editorText()).toContain("body"));
    type("mine\n");
    file = { source: "theirs\n", revision: "r9" };
    rerender(<NotePane slug="n" path="a.md" version={1} />);
    await waitFor(() => expect(screen.getByRole("alert")).toBeTruthy());
    fireEvent.click(screen.getByRole("button", { name: "Keep my draft" }));
    await waitFor(() => expect(status()).toBe("Saved"));
    expect(file?.source).toBe("mine\n");
  });

  it("a deleted note keeps the draft with copy and discard only", async () => {
    const { rerender } = render(<NotePane slug="n" path="a.md" version={0} />);
    await waitFor(() => expect(screen.getByText("body")).toBeTruthy());
    fireEvent.keyDown(document.body, ctrlE);
    await waitFor(() => expect(editorText()).toContain("body"));
    type("mine\n");
    file = null;
    rerender(<NotePane slug="n" path="a.md" version={1} />);
    await waitFor(() => expect(screen.getByRole("alert").textContent).toContain("deleted on disk"));
    expect(screen.queryByRole("button", { name: "Keep my draft" })).toBeNull();
    expect(screen.queryByRole("button", { name: "Load the file" })).toBeNull();
    expect(screen.getByRole("button", { name: "Copy draft" })).toBeTruthy();
    fireEvent.click(screen.getByRole("button", { name: "Discard draft" }));
    await waitFor(() => expect(screen.getAllByText("note no longer exists").length).toBeGreaterThan(0));
    expect(document.querySelector(".cm-editor")).toBeNull();
  });

  it("shows a read failure in the editor", async () => {
    file = null;
    render(<NotePane slug="n" path="a.md" />);
    fireEvent.keyDown(document.body, ctrlE);
    await waitFor(() => expect(screen.getAllByText("note or root no longer exists").length).toBeGreaterThan(0));
    expect(document.querySelector(".cm-editor")).toBeNull();
  });
});

describe("UnsavedDrafts", () => {
  it("lists other notes with unsaved work, not the current one", async () => {
    const other = getSession("n", "b.md");
    other.state = { ...other.state, status: "failed", base: { source: "", revision: "r" }, draft: "x" };
    const cur = getSession("n", "a.md");
    cur.state = { ...cur.state, status: "pending", base: { source: "", revision: "r" }, draft: "y" };
    render(<UnsavedDrafts slug="n" current="a.md" />);
    const link = screen.getByRole("link", { name: "b.md" });
    expect(link.getAttribute("href")).toBe("/r/n/b.md");
    expect(screen.queryByRole("link", { name: "a.md" })).toBeNull();
  });

  it("renders nothing when everything is saved", () => {
    const { container } = render(<UnsavedDrafts slug="n" current="" />);
    expect(container.textContent).toBe("");
  });
});

describe("useUnsavedGuard", () => {
  function Guard() {
    useUnsavedGuard();
    return null;
  }

  it("asks before unloading with unsaved work and sends a final save", async () => {
    render(<Guard />);
    const e = new Event("beforeunload", { cancelable: true }) as BeforeUnloadEvent;
    window.dispatchEvent(e);
    expect(e.defaultPrevented).toBe(false);
    const s = getSession("n", "a.md");
    await s.open();
    s.edit("closing\n");
    const e2 = new Event("beforeunload", { cancelable: true }) as BeforeUnloadEvent;
    window.dispatchEvent(e2);
    expect(e2.defaultPrevented).toBe(true);
    await waitFor(() => expect(file?.source).toBe("closing\n"));
    const put = (fetch as unknown as ReturnType<typeof vi.fn>).mock.calls.find((c) => (c[1] as RequestInit)?.method === "PUT")!;
    expect((put[1] as RequestInit).keepalive).toBe(true);
  });

  it("saves when the window loses focus", async () => {
    render(<Guard />);
    const s = getSession("n", "a.md");
    await s.open();
    s.edit("blurred\n");
    window.dispatchEvent(new Event("blur"));
    await waitFor(() => expect(file?.source).toBe("blurred\n"));
  });
});
