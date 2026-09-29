import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render } from "@testing-library/preact";
import { undo } from "@codemirror/commands";
import { EditorView } from "@codemirror/view";
import { Vim, getCM } from "@replit/codemirror-vim";
import { Editor } from "./editor";
import { Session, resetSessions } from "./session";
import { UNREACHABLE } from "./api";

// jsdom has no layout: CodeMirror measures ranges to scroll a change into
// view and to draw the selection, and these answer "nowhere" rather than
// throwing out of the middle of an update.
Range.prototype.getClientRects = () => [] as unknown as DOMRectList;
Range.prototype.getBoundingClientRect = () => new DOMRect();

function sessionAt(path: string, draft: string): Session {
  const s = new Session("n", path);
  s.state = { ...s.state, status: "clean", base: { source: draft, revision: "r1" }, draft };
  return s;
}

function viewOf(container: Element): EditorView {
  return EditorView.findFromDOM(container.querySelector<HTMLElement>(".cm-editor")!)!;
}

/**
 * An editor over the note, in vim's insert mode: where a reader who is
 * writing is when they paste. Normal mode has its own rule for where a
 * paste goes, which the vim test below holds to.
 */
function editing(s: Session) {
  const r = render(<Editor session={s} />);
  const view = viewOf(r.container);
  Vim.handleKey(getCM(view)!, "i", "user");
  return { ...r, view };
}

function png(name: string): File {
  return new File([new Uint8Array([0x89, 0x50, 0x4e, 0x47])], name, { type: "image/png" });
}

interface Upload {
  url: string;
  body: unknown;
}

/**
 * A daemon that answers uploads by `answer` (a name per upload, or an
 * error), and every other call — the autosave — with the note as saved.
 * Each upload waits on `hold` when one is set, so a test can act while
 * the upload is in flight.
 */
function daemon(answer: (name: string | null, n: number) => Response | "unreachable") {
  const uploads: Upload[] = [];
  const gate: { hold: Promise<void> | null } = { hold: null };
  vi.stubGlobal(
    "fetch",
    vi.fn(async (url: string, init?: RequestInit) => {
      if (url.includes("/resources")) {
        uploads.push({ url, body: init?.body });
        if (gate.hold) await gate.hold;
        const name = new URL(url, "http://x").searchParams.get("name");
        const a = answer(name, uploads.length);
        if (a === "unreachable") throw new TypeError("Failed to fetch");
        return a;
      }
      return new Response(JSON.stringify({ source: "", revision: "r2" }), { status: 200 });
    }),
  );
  return { uploads, gate };
}

function created(name: string): Response {
  return new Response(JSON.stringify({ root: "n", path: `_resources/${name}`, name, created: true }), { status: 201 });
}

function refused(status: number, code: string, error: string): Response {
  return new Response(JSON.stringify({ code, error }), { status });
}

/** A paste event carrying files and, optionally, text, as a browser builds one. */
function paste(target: Element, files: File[], text = "") {
  const ev = new Event("paste", { bubbles: true, cancelable: true });
  Object.defineProperty(ev, "clipboardData", {
    value: {
      files,
      types: [...(files.length ? ["Files"] : []), ...(text ? ["text/plain"] : [])],
      getData: (t: string) => (t === "text/plain" || t === "Text" ? text : ""),
    },
  });
  target.dispatchEvent(ev);
  return ev;
}

function drop(target: Element, files: File[]) {
  const ev = new Event("drop", { bubbles: true, cancelable: true });
  Object.assign(ev, { clientX: 1, clientY: 1 });
  Object.defineProperty(ev, "dataTransfer", {
    value: { files, types: files.length ? ["Files"] : [], getData: () => "" },
  });
  target.dispatchEvent(ev);
  return ev;
}

beforeEach(() => {
  resetSessions();
  localStorage.clear();
});
afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
});

describe("pasting an image", () => {
  it("uploads it and links it at the cursor, with empty alt text", async () => {
    const { uploads } = daemon(() => created("0123abcd.png"));
    const s = sessionAt("top.md", "before after\n");
    const { view } = editing(s);
    view.dispatch({ selection: { anchor: 7 } });

    const ev = paste(view.contentDOM, [png("image.png")]);
    expect(ev.defaultPrevented).toBe(true);
    await vi.waitFor(() => expect(view.state.doc.toString()).toBe("before ![](_resources/0123abcd.png)after\n"));
    // A paste sends no name: the daemon names it by its content.
    expect(uploads).toHaveLength(1);
    expect(uploads[0].url).toBe("/api/r/n/resources");
    expect(uploads[0].body).toBeInstanceOf(File);
    // The caret follows the link, as it would typed text.
    expect(view.state.selection.main.head).toBe(7 + "![](_resources/0123abcd.png)".length);
    // And the edit reaches the session like any other: autosave takes it.
    expect(s.state.draft).toBe("before ![](_resources/0123abcd.png)after\n");
    expect(s.state.status).not.toBe("clean");
  });

  it("replaces a selection", async () => {
    daemon(() => created("x.png"));
    const s = sessionAt("top.md", "keep DROP keep\n");
    const { view } = editing(s);
    view.dispatch({ selection: { anchor: 5, head: 9 } });
    paste(view.contentDOM, [png("image.png")]);
    await vi.waitFor(() => expect(view.state.doc.toString()).toBe("keep ![](_resources/x.png) keep\n"));
  });

  it("climbs to the root from a note two folders down", async () => {
    daemon(() => created("x.png"));
    const s = sessionAt("one/two/deep.md", "\n");
    const { view } = editing(s);
    paste(view.contentDOM, [png("image.png")]);
    await vi.waitFor(() => expect(view.state.doc.toString()).toBe("![](../../_resources/x.png)\n"));
  });

  it("is undone by one undo, which leaves the rest of the history alone", async () => {
    daemon(() => created("x.png"));
    const s = sessionAt("top.md", "a\n");
    const { view } = editing(s);
    // Typed text just before the paste would join the paste's undo group
    // were the insert not its own history event.
    view.dispatch({ changes: { from: 1, insert: "b" }, selection: { anchor: 2 }, userEvent: "input.type" });
    paste(view.contentDOM, [png("image.png")]);
    await vi.waitFor(() => expect(view.state.doc.toString()).toBe("ab![](_resources/x.png)\n"));
    undo(view);
    expect(view.state.doc.toString()).toBe("ab\n");
    undo(view);
    expect(view.state.doc.toString()).toBe("a\n");
  });

  it("lands where the paste was, though the note was typed in during the upload", async () => {
    const { gate } = daemon(() => created("x.png"));
    let release!: () => void;
    gate.hold = new Promise((r) => (release = r));
    const s = sessionAt("top.md", "one two\n");
    const { view } = editing(s);
    view.dispatch({ selection: { anchor: 4 } });
    paste(view.contentDOM, [png("image.png")]);
    // Typing at the start of the line moves the paste point along, and
    // typing at the paste point itself goes after the link.
    view.dispatch({ changes: { from: 0, insert: ">> " } });
    view.dispatch({ changes: { from: 7, insert: "zz" }, selection: { anchor: 9 } });
    release();
    await vi.waitFor(() => expect(view.state.doc.toString()).toBe(">> one ![](_resources/x.png)zztwo\n"));
    // The reader had moved on; the caret stays after what they typed.
    expect(view.state.selection.main.head).toBe(">> one ![](_resources/x.png)zz".length);
  });

  it("goes where a text paste goes in vim's normal mode, and leaves the same mode", async () => {
    daemon(() => created("x.png"));
    // The same note, the same caret, in normal mode: once pasting text,
    // once pasting an image. The image's link must land where the text
    // did — whatever vim's rule for that is — and leave vim as the text did.
    const outcome = async (files: File[], text: string) => {
      const s = sessionAt("top.md", "before after\n");
      const r = render(<Editor session={s} />);
      const view = viewOf(r.container);
      const cm = getCM(view)!;
      view.dispatch({ selection: { anchor: 7 } });
      expect(cm.state.vim?.insertMode).toBeFalsy();
      paste(view.contentDOM, files, text);
      await vi.waitFor(() => expect(view.state.doc.toString()).not.toBe("before after\n"));
      const result = { doc: view.state.doc.toString(), insert: cm.state.vim?.insertMode };
      r.unmount();
      return result;
    };
    const link = "![](_resources/x.png)";
    const text = await outcome([], link);
    const image = await outcome([png("image.png")], "");
    expect(image).toEqual(text);
  });

  it("is not taken when the clipboard holds text", async () => {
    const { uploads } = daemon(() => created("x.png"));
    const s = sessionAt("top.md", "a\n");
    const { view } = editing(s);
    view.dispatch({ selection: { anchor: 1 } });
    paste(view.contentDOM, [], "https://example.com/pic.png");
    // CodeMirror's own paste handled it, as before this change.
    expect(view.state.doc.toString()).toBe("ahttps://example.com/pic.png\n");
    expect(uploads).toHaveLength(0);
  });
});

describe("a refused or failed upload", () => {
  it.each([
    ["too large", () => refused(413, "too_large", "image exceeds 16 MiB"), "image exceeds 16 MiB"],
    ["not an image", () => refused(415, "unsupported_type", "not a PNG, JPEG, GIF, WebP or SVG image"), "not a PNG"],
    ["the daemon unreachable", () => "unreachable" as const, UNREACHABLE],
  ])("shows a message and inserts nothing: %s", async (_, answer, message) => {
    daemon(answer);
    const s = sessionAt("top.md", "a\n");
    const { container, findByRole, view } = editing(s);
    paste(view.contentDOM, [png("image.png")]);
    const alert = await findByRole("alert");
    expect(alert.textContent).toContain("pasted image");
    expect(alert.textContent).toContain(message);
    expect(view.state.doc.toString()).toBe("a\n");
    expect(s.state.draft).toBe("a\n");
    expect(s.state.status).toBe("clean");
    // Dismissed, it goes.
    fireEvent.click(alert.querySelector("button")!);
    expect(container.querySelector("[role=alert]")).toBeNull();
  });

  it("is cleared by the next upload that works", async () => {
    daemon((_, n) => (n === 1 ? refused(415, "unsupported_type", "nope") : created("x.png")));
    const s = sessionAt("top.md", "a\n");
    const { container, findByRole, view } = editing(s);
    paste(view.contentDOM, [png("image.png")]);
    await findByRole("alert");
    paste(view.contentDOM, [png("image.png")]);
    await vi.waitFor(() => expect(view.state.doc.toString()).toContain("_resources/x.png"));
    expect(container.querySelector("[role=alert]")).toBeNull();
  });
});

describe("dropping images", () => {
  it("links each file at the drop point, named for the file, and one undo removes them all", async () => {
    const { uploads } = daemon((name) => created(name!.replace(/ /g, "-")));
    const s = sessionAt("a/n.md", "0123456789\n");
    const { view } = editing(s);
    vi.spyOn(view, "posAtCoords").mockReturnValue(5);

    const ev = drop(view.contentDOM, [png("first shot.png"), png("second.png")]);
    expect(ev.defaultPrevented).toBe(true);
    await vi.waitFor(() =>
      expect(view.state.doc.toString()).toBe(
        "01234![first shot](../_resources/first-shot.png)\n![second](../_resources/second.png)56789\n",
      ),
    );
    expect(uploads.map((u) => u.url)).toEqual([
      "/api/r/n/resources?name=first%20shot.png",
      "/api/r/n/resources?name=second.png",
    ]);
    undo(view);
    expect(view.state.doc.toString()).toBe("0123456789\n");
  });

  it("links the files that went and names the ones that did not", async () => {
    daemon((name) => (name === "bad.png" ? refused(415, "unsupported_type", "not an image after all") : created(name!)));
    const s = sessionAt("n.md", "\n");
    const { findByRole, view } = editing(s);
    vi.spyOn(view, "posAtCoords").mockReturnValue(0);
    const pdf = new File(["%PDF"], "report.pdf", { type: "application/pdf" });
    drop(view.contentDOM, [png("good.png"), png("bad.png"), pdf]);
    const alert = await findByRole("alert");
    expect(view.state.doc.toString()).toBe("![good](_resources/good.png)\n");
    expect(alert.textContent).toContain("bad.png");
    expect(alert.textContent).toContain("not an image after all");
    expect(alert.textContent).toContain("report.pdf");
  });

  it("leaves a drop with no image to CodeMirror", () => {
    const { uploads } = daemon(() => created("x.png"));
    const s = sessionAt("n.md", "\n");
    const { container, view } = editing(s);
    const pdf = new File(["%PDF"], "report.pdf", { type: "application/pdf" });
    const ev = drop(view.contentDOM, [pdf]);
    expect(uploads).toHaveLength(0);
    // Not ours: nothing here said no to the browser on CodeMirror's behalf.
    expect(container.querySelector("[role=alert]")).toBeNull();
    expect(ev).toBeTruthy();
  });
});
