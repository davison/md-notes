import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { cleanup, render } from "@testing-library/preact";
import { EditorState } from "@codemirror/state";
import { insertNewlineAndIndent } from "@codemirror/commands";
import { Vim, getCM } from "@replit/codemirror-vim";
import { EditorView } from "@codemirror/view";
import { Editor, lineEnding, withLineEnding } from "./editor";
import { Session, resetSessions } from "./session";

function session(draft = "# hi\n"): Session {
  const s = new Session("n", "a.md");
  s.state = { ...s.state, status: "clean", base: { source: draft, revision: "r1" }, draft };
  return s;
}

function viewOf(container: Element): EditorView {
  return EditorView.findFromDOM(container.querySelector<HTMLElement>(".cm-editor")!)!;
}

beforeEach(() => resetSessions());
afterEach(() => cleanup());

describe("line endings", () => {
  it("picks the ending a note uses most, not the first it contains", () => {
    expect(lineEnding("a\r\nb\r\n")).toBe("\r\n");
    expect(lineEnding("a\rb\r")).toBe("\r");
    expect(lineEnding("a\nb\n")).toBe("\n");
    expect(lineEnding("")).toBe("\n");
    // Mostly LF with one CRLF, and mostly LF with one stray CR, are LF notes.
    expect(lineEnding("a\nb\nc\r\nd\ne\n")).toBe("\n");
    expect(lineEnding("a\nb\rc\nd\n")).toBe("\n");
    // Mostly CRLF with one LF is a CRLF note; ties go to LF.
    expect(lineEnding("a\r\nb\nc\r\n")).toBe("\r\n");
    expect(lineEnding("a\r\nb\n")).toBe("\n");
    expect(lineEnding("a\rb\r\nc\r")).toBe("\r");
  });

  it("rejoins the editor's LF text with the note's ending", () => {
    expect(withLineEnding("a\nb\n", "\r\n")).toBe("a\r\nb\r\n");
    expect(withLineEnding("a\nb\n", "\r")).toBe("a\rb\r");
    expect(withLineEnding("a\nb\n", "\n")).toBe("a\nb\n");
  });
});

describe("Editor", () => {
  it("shows the draft with vim in normal mode", () => {
    const s = session();
    const { container } = render(<Editor session={s} />);
    expect(container.querySelector(".cm-content")?.textContent).toContain("# hi");
    const cm = getCM(viewOf(container))!;
    expect(cm.state.vim?.insertMode).toBeFalsy();
  });

  it("reports document changes to the session", () => {
    const s = session();
    const edit = vi.spyOn(s, "edit");
    const { container } = render(<Editor session={s} />);
    const view = viewOf(container);
    view.dispatch({ changes: { from: view.state.doc.length, insert: "more" } });
    expect(edit).toHaveBeenCalledWith("# hi\nmore");
    expect(s.state.status).toBe("pending");
  });

  it.each([
    ["CRLF", "---\r\ntitle: A\r\n---\r\n\r\n# A\r\n\r\nbody\r\n", "---\r\ntitle: A\r\n---\r\n\r\n# A\r\n\r\nbody!\r\n"],
    ["lone CR", "one\rtwo\rthree\r", "one\rtwo\rthree!\r"],
    ["LF", "one\ntwo\n", "one\ntwo!\n"],
  ])("keeps a note's %s line endings through an edit", (_, source, expected) => {
    const s = session(source);
    const { container } = render(<Editor session={s} />);
    const view = viewOf(container);
    // Type "!" at the end of the last line.
    const at = view.state.doc.length - 1;
    view.dispatch({ changes: { from: at, insert: "!" } });
    expect(s.state.draft).toBe(expected);
    expect(s.state.status).toBe("pending");
    // Pressing Enter inserts the note's own ending too.
    view.dispatch({ selection: { anchor: at + 1 } });
    insertNewlineAndIndent(view);
    expect(s.state.draft.split(lineEnding(source)).length).toBe(source.split(lineEnding(source)).length + 1);
  });

  it.each([
    ["CRLF dominant", "a\r\nb\nc\r\n", "!a\r\nb\r\nc\r\n"],
    ["LF dominant with one CRLF", "a\nb\nc\r\nd\ne\n", "!a\nb\nc\nd\ne\n"],
    ["LF dominant with a stray CR", "a\nb\rc\nd\n", "!a\nb\nc\nd\n"],
  ])("makes a note with mixed endings uniform in its dominant one (%s)", (_, source, expected) => {
    const s = session(source);
    const { container } = render(<Editor session={s} />);
    const view = viewOf(container);
    view.dispatch({ changes: { from: 0, insert: "!" } });
    expect(s.state.draft).toBe(expected);
  });

  it("opening without typing changes nothing", () => {
    const s = session("a\r\nb\r\n");
    const edit = vi.spyOn(s, "edit");
    render(<Editor session={s} />);
    expect(edit).not.toHaveBeenCalled();
    expect(s.state.draft).toBe("a\r\nb\r\n");
  });

  it(":w, :q, :x and :wq all flush the session", () => {
    const s = session();
    const flush = vi.spyOn(s, "flush").mockResolvedValue();
    const { container } = render(<Editor session={s} />);
    const cm = getCM(viewOf(container)) as Parameters<typeof Vim.handleEx>[0];
    for (const ex of ["w", "q", "x", "wq"]) {
      Vim.handleEx(cm, ex);
    }
    expect(flush).toHaveBeenCalledTimes(4);
  });

  it("parks its state on the session between mounts and restores it", () => {
    const s = session("one\ntwo\n");
    const first = render(<Editor session={s} />);
    const view = viewOf(first.container);
    view.dispatch({ selection: { anchor: 4 } });
    first.unmount();
    expect(s.editorState).toBeInstanceOf(EditorState);
    const second = render(<Editor session={s} />);
    expect(viewOf(second.container).state.selection.main.anchor).toBe(4);
  });

  it("rebuilds from the draft when the generation changes", () => {
    const s = session("one\n");
    const first = render(<Editor session={s} />);
    first.unmount();
    s.state = { ...s.state, draft: "replaced\n", generation: s.state.generation + 1 };
    const second = render(<Editor session={s} />);
    expect(viewOf(second.container).state.doc.toString()).toBe("replaced\n");
  });
});
