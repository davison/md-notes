import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { cleanup, render } from "@testing-library/preact";
import { EditorState } from "@codemirror/state";
import { Vim, getCM } from "@replit/codemirror-vim";
import { EditorView } from "@codemirror/view";
import { Editor } from "./editor";
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
