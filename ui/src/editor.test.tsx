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

/**
 * A session over a daemon holding one note, for the cases that change the
 * file on disk under an open editor. The returned record is live: assign to
 * `source` and `revision` and the next read sees it.
 */
function served(source: string) {
  const file = { source, revision: "r1" };
  vi.stubGlobal(
    "fetch",
    vi.fn(() =>
      Promise.resolve({
        ok: true,
        status: 200,
        statusText: "OK",
        json: () => Promise.resolve({ ...file }),
      } as Response),
    ),
  );
  return { file, session: new Session("n", "a.md") };
}

beforeEach(() => {
  resetSessions();
  // Sessions mirror an unsaved draft to storage under a key that is the
  // note's, not the object's: a new Session over the same note would
  // otherwise recover the previous case's draft on its first read.
  localStorage.clear();
});
afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

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

  it.each([
    ["LF", "one\ntwo\nthree\nfour\n", "one\ntwo\nthree!\nfour\n"],
    ["CRLF", "one\r\ntwo\r\nthree\r\nfour\r\n", "one\r\ntwo\r\nthree!\r\nfour\r\n"],
  ])("keeps the caret and the note's %s endings when a recreate writes the draft back", (_, draft, typed) => {
    // The deleted-on-disk banner's way back writes the draft to disk byte
    // for byte and the session takes the file in place, bumping the
    // generation so the pane refetches the note's title (#100). The
    // document has not changed, so the reader's place in it must not
    // either — on a long note that is where they were reading.
    const s = session(draft);
    s.state = {
      ...s.state,
      status: "conflict",
      conflict: { kind: "deleted", current: null },
    };
    const { container, rerender } = render(<Editor session={s} />);
    const view = viewOf(container);
    // Offsets are into the document CodeMirror holds, which is LF whatever
    // the note's own ending is: the end of the third line, either way.
    view.dispatch({ selection: { anchor: 13 } });
    const before = s.state.generation;

    expect(s.recreated({ source: draft, revision: "r2" })).toBe(true);
    expect(s.state.generation).toBe(before + 1);
    rerender(<Editor session={s} />);

    const after = viewOf(container);
    expect(after.state.doc.toString()).toBe("one\ntwo\nthree\nfour\n");
    expect(after.state.selection.main.anchor).toBe(13);
    // And the kept state still writes the note back in its own ending.
    after.dispatch({ changes: { from: 13, insert: "!" } });
    expect(s.state.draft).toBe(typed);
  });

  describe("the vim mode across a recreate (#112)", () => {
    /** An editor over a deleted conflict, caret at the end of the third line. */
    function orphaned() {
      const draft = "one\ntwo\nthree\nfour\n";
      const s = session(draft);
      s.state = { ...s.state, status: "conflict", conflict: { kind: "deleted", current: null } };
      const r = render(<Editor session={s} />);
      const view = viewOf(r.container);
      view.dispatch({ selection: { anchor: 13 } });
      return { s, draft, ...r, cm: () => getCM(viewOf(r.container))! };
    }

    it("returns a reader who was typing to insert mode, caret where it was", () => {
      const { s, draft, rerender, cm, container } = orphaned();
      // `a` on the last character of the line: the insert-mode caret sits
      // after it, at the end of the line, where normal mode cannot put it.
      viewOf(container).dispatch({ selection: { anchor: 12 } });
      Vim.handleKey(cm(), "a", "user");
      expect(cm().state.vim?.insertMode).toBe(true);
      expect(viewOf(container).state.selection.main.head).toBe(13);

      expect(s.recreated({ source: draft, revision: "r2" })).toBe(true);
      rerender(<Editor session={s} />);

      expect(cm().state.vim?.insertMode).toBe(true);
      expect(viewOf(container).state.selection.main.head).toBe(13);
      // The next key is a character, not a command: vim leaves it to the
      // editor rather than consuming it (the browser test types it for real).
      expect(Vim.handleKey(cm(), "x", "user")).toBeFalsy();
      expect(s.state.draft).toBe(draft);
    });

    it("leaves a reader who was in normal mode in normal mode", () => {
      const { s, draft, rerender, cm, container } = orphaned();
      expect(cm().state.vim?.insertMode).toBeFalsy();
      const head = viewOf(container).state.selection.main.head;

      expect(s.recreated({ source: draft, revision: "r2" })).toBe(true);
      rerender(<Editor session={s} />);

      expect(cm().state.vim?.insertMode).toBeFalsy();
      expect(viewOf(container).state.selection.main.head).toBe(head);
    });

    it("keeps insert mode through a Ctrl+E round trip, at the same generation", () => {
      // The #192 gate, answered (a): the mode is parked with the caret, so a
      // flip to the rendered view and back — an unmount and a remount with
      // nothing replaced — returns a reader who was typing to insert mode,
      // not only a recreate.
      const s = session("one\ntwo\n");
      const first = render(<Editor session={s} />);
      const view = viewOf(first.container);
      view.dispatch({ selection: { anchor: 2 } });
      Vim.handleKey(getCM(view)!, "a", "user");
      expect(view.state.selection.main.head).toBe(3);
      const generation = s.state.generation;
      first.unmount();

      const second = render(<Editor session={s} />);
      expect(s.state.generation).toBe(generation);
      const again = viewOf(second.container);
      expect(getCM(again)!.state.vim?.insertMode).toBe(true);
      expect(again.state.selection.main.head).toBe(3);
      second.unmount();

      // And a round trip begun in normal mode comes back in normal mode.
      const third = render(<Editor session={s} />);
      Vim.handleKey(getCM(viewOf(third.container))!, "<Esc>", "user");
      expect(getCM(viewOf(third.container))!.state.vim?.insertMode).toBeFalsy();
      third.unmount();
      const fourth = render(<Editor session={s} />);
      expect(getCM(viewOf(fourth.container))!.state.vim?.insertMode).toBeFalsy();
    });

    it("starts in normal mode when the text is replaced from disk", () => {
      // A rebuilt state is a different document: the caret is not kept, and
      // neither is the mode, as before.
      const s = session("one\n");
      const r = render(<Editor session={s} />);
      Vim.handleKey(getCM(viewOf(r.container))!, "i", "user");
      s.state = { ...s.state, draft: "replaced\n", base: { source: "replaced\n", revision: "r2" }, generation: s.state.generation + 1 };
      r.rerender(<Editor session={s} />);
      expect(viewOf(r.container).state.doc.toString()).toBe("replaced\n");
      expect(getCM(viewOf(r.container))!.state.vim?.insertMode).toBeFalsy();
    });
  });

  it.each([
    ["a CRLF note normalised to LF", "one\r\ntwo\r\n", "one\ntwo\n", "one\ntwo\nthree\n"],
    ["an LF note gaining CRLF", "one\ntwo\n", "one\r\ntwo\r\n", "one\r\ntwo\r\nthree\r\n"],
  ])("follows the file's endings when they change on disk under a clean session (%s)", async (_, before, after, expected) => {
    // The ordinary clean adoption: no conflict, no banner, nothing for the
    // reader to decide — the file is rewritten with the same text in the
    // other ending, the session takes it and bumps the generation, and the
    // editor keeps the state it had parked because the document did not
    // change. What must not survive that is the ending the state was built
    // with: the next keystroke writes the whole file, and it writes it in
    // the ending the file has now.
    const { file, session: s } = served(before);
    await s.open();
    expect(s.state.draft).toBe(before);
    const { container, rerender } = render(<Editor session={s} />);
    viewOf(container).dispatch({ selection: { anchor: 4 } });

    file.source = after;
    file.revision = "r2";
    await s.changed();
    expect(s.state.draft).toBe(after);
    rerender(<Editor session={s} />);

    const view = viewOf(container);
    // The parked state was kept — which is what puts the case in reach.
    expect(view.state.selection.main.anchor).toBe(4);
    view.dispatch({ changes: { from: view.state.doc.length, insert: "three\n" } });
    expect(s.state.draft).toBe(expected);
  });

  it("follows the file's endings through Load the file", () => {
    // The same staleness by the other door: an endings-only change under an
    // unsaved draft, resolved by taking the file.
    const crlf = "a\r\nb\r\n";
    const lf = "a\nb\n";
    const s = session(crlf);
    s.state = {
      ...s.state,
      status: "conflict",
      conflict: { kind: "changed", current: { source: lf, revision: "r2" } },
    };
    const { container, rerender } = render(<Editor session={s} />);
    s.loadFile();
    expect(s.state.draft).toBe(lf);
    rerender(<Editor session={s} />);

    const view = viewOf(container);
    view.dispatch({ changes: { from: view.state.doc.length, insert: "c\n" } });
    expect(s.state.draft).toBe("a\nb\nc\n");
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
