import { useEffect, useRef } from "preact/hooks";
import { EditorState } from "@codemirror/state";
import { EditorView, drawSelection, highlightActiveLine, keymap } from "@codemirror/view";
import { defaultKeymap, history, historyKeymap, indentWithTab } from "@codemirror/commands";
import { HighlightStyle, syntaxHighlighting } from "@codemirror/language";
import { markdown } from "@codemirror/lang-markdown";
import { languages } from "@codemirror/language-data";
import { tags } from "@lezer/highlight";
import { Vim, getCM, vim } from "@replit/codemirror-vim";
import type { Session } from "./session";

const markdownStyle = HighlightStyle.define([
  { tag: tags.heading, fontWeight: "bold", color: "var(--accent)" },
  { tag: tags.emphasis, fontStyle: "italic" },
  { tag: tags.strong, fontWeight: "bold" },
  { tag: tags.strikethrough, textDecoration: "line-through" },
  { tag: tags.link, color: "var(--accent)" },
  { tag: tags.url, color: "var(--muted)" },
  { tag: tags.monospace, color: "var(--fg)" },
  { tag: tags.quote, color: "var(--muted)" },
  { tag: tags.meta, color: "var(--muted)" },
  { tag: tags.processingInstruction, color: "var(--muted)" },
  { tag: tags.list, color: "var(--accent)" },
  { tag: tags.keyword, color: "var(--accent)" },
  { tag: tags.comment, color: "var(--muted)", fontStyle: "italic" },
  { tag: tags.string, color: "var(--fg)" },
]);

const theme = EditorView.theme({
  "&": { height: "100%", fontSize: "0.95em" },
  ".cm-scroller": { fontFamily: 'ui-monospace, "JetBrains Mono", Menlo, monospace', lineHeight: "1.55" },
  ".cm-content": { padding: "1rem 0", caretColor: "var(--fg)" },
  ".cm-line": { padding: "0 2rem" },
  "&.cm-focused": { outline: "none" },
  ".cm-activeLine": { backgroundColor: "color-mix(in srgb, var(--accent) 7%, transparent)" },
  ".cm-selectionBackground, &.cm-focused .cm-selectionBackground": {
    backgroundColor: "color-mix(in srgb, var(--accent) 25%, transparent)",
  },
  ".cm-cursor": { borderLeftColor: "var(--fg)" },
  ".cm-fat-cursor": { background: "var(--accent) !important", color: "var(--pane) !important" },
  "&:not(.cm-focused) .cm-fat-cursor": { outlineColor: "var(--accent) !important" },
  ".cm-vim-panel": { color: "var(--muted)", padding: "0.2rem 0.5rem" },
  ".cm-vim-panel input": { color: "var(--fg)", font: "inherit" },
  ".cm-panels": { background: "var(--bg)", color: "var(--fg)", borderColor: "var(--line)" },
});

/**
 * The line ending a note uses most: CRLF, a lone CR, or LF. Ties go to
 * LF, then CRLF, so a stray carriage return never decides the whole file.
 */
export function lineEnding(text: string): "\r\n" | "\r" | "\n" {
  let crlf = 0;
  let cr = 0;
  let lf = 0;
  for (let i = 0; i < text.length; i++) {
    const c = text.charCodeAt(i);
    if (c === 13) {
      if (text.charCodeAt(i + 1) === 10) {
        crlf++;
        i++;
      } else cr++;
    } else if (c === 10) lf++;
  }
  if (crlf > lf && crlf >= cr) return "\r\n";
  if (cr > lf && cr > crlf) return "\r";
  return "\n";
}

/**
 * CodeMirror splits a document on any line ending and joins with LF, so
 * the text it hands back is rejoined with the ending the note had. A note
 * with mixed endings comes back uniform in its dominant one.
 */
export function withLineEnding(text: string, ending: string): string {
  return ending === "\n" ? text : text.replaceAll("\n", ending);
}

/**
 * Builds the editor state for a draft; exported so tests can drive the
 * keymap.
 *
 * The ending is read from the session's draft as each edit is written back,
 * rather than captured here when the state is built. Both answer the same
 * question — the draft carries the note's endings, and an edit through
 * `withLineEnding` keeps them — but a captured one is a second copy of the
 * note's state that can go stale while the document does not: the file's
 * endings can change on disk under a session with nothing unsaved, or under
 * **Load the file**, and the draft that arrives has the same text in the
 * other ending. Read at write time there is nothing to keep in step.
 */
export function createState(doc: string, session: Session): EditorState {
  return EditorState.create({
    doc,
    extensions: [
      // Vim must come before any other keymap so it sees keys first.
      vim(),
      history(),
      drawSelection(),
      highlightActiveLine(),
      EditorView.lineWrapping,
      markdown({ codeLanguages: languages }),
      syntaxHighlighting(markdownStyle),
      keymap.of([...defaultKeymap, ...historyKeymap, indentWithTab]),
      theme,
      EditorView.updateListener.of((u) => {
        if (u.docChanged) {
          session.edit(withLineEnding(u.state.doc.toString(), lineEnding(session.state.draft)));
        }
      }),
    ],
  });
}

let exDefined = false;
function defineEx() {
  if (exDefined) return;
  exDefined = true;
  // :w saves now. The editor never closes, so the commands a vim user
  // reaches for to mean "done here" (:q, :x, :wq) save too, and nothing else.
  const save = (cm: { cm6: EditorView }) => {
    void flushFor(cm.cm6);
  };
  Vim.defineEx("write", "w", save);
  Vim.defineEx("quit", "q", save);
  Vim.defineEx("xit", "x", save);
  Vim.defineEx("wq", "wq", save);
}

const owners = new WeakMap<EditorView, Session>();
function flushFor(view: EditorView) {
  return owners.get(view)?.flush() ?? Promise.resolve();
}

/**
 * Whether a parked editor state already holds this draft. CodeMirror
 * splits a document on any line ending and joins the text it hands back
 * with LF, while the draft keeps the note's own ending, so the two are
 * compared on that footing — a draft that differs from the parked document
 * only in its endings is the same document, and the ending it is written
 * back in is read from the draft at that moment rather than from anything
 * this state remembers.
 */
function holdsDraft(state: EditorState, draft: string): boolean {
  return state.doc.toString() === draft.replace(/\r\n?/g, "\n");
}

/**
 * A CodeMirror 6 markdown editor with vim keybindings over a session's
 * draft. The editor state is parked on the session between mounts so a
 * mode switch keeps the cursor and undo history, and it is rebuilt when
 * the session replaces the draft from disk.
 *
 * "Replaces" is the text, not the counter. The generation says the draft
 * came from outside the editor, and the pane hangs other work on the same
 * edge — refetching the note's title, for one — so it can move over a
 * document that has not changed at all: a note recreated from its own
 * draft is written back byte for byte (#100). Rebuilding then would throw
 * away the selection and the scroll for nothing, which on a long note is
 * the reader's place in it, so a parked state holding this very text is
 * kept and only the counter moves.
 *
 * Vim's mode is not part of that state: it lives on the view, and a new
 * view starts in normal mode. So whether the reader was in insert mode is
 * parked beside the state and put back whenever the state is, with the
 * selection restored after it so the caret does not move — a reader who was
 * typing when the note vanished is typing again when **Recreate the note**
 * brings it back (#112), and a mode switch and back is the same case. A
 * rebuilt state is a different document, and starts in normal mode.
 */
export function Editor({ session }: { session: Session }) {
  const host = useRef<HTMLDivElement>(null);
  const generation = session.state.generation;

  useEffect(() => {
    defineEx();
    const parent = host.current!;
    const parked = session.editorState instanceof EditorState ? session.editorState : null;
    const keep = parked !== null && (session.editorGeneration === generation || holdsDraft(parked, session.state.draft));
    const state = keep ? parked : createState(session.state.draft, session);
    const view = new EditorView({ state, parent });
    owners.set(view, session);
    const cm = getCM(view);
    if (keep && session.editorInsert && cm) {
      Vim.handleKey(cm, "i", "api");
      view.dispatch({ selection: state.selection });
    }
    view.focus();
    return () => {
      session.editorState = view.state;
      session.editorGeneration = generation;
      session.editorInsert = getCM(view)?.state.vim?.insertMode === true;
      view.destroy();
    };
  }, [session, generation]);

  return <div ref={host} class="editor" />;
}
