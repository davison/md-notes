import { useEffect, useRef } from "preact/hooks";
import { EditorState } from "@codemirror/state";
import { EditorView, drawSelection, highlightActiveLine, keymap } from "@codemirror/view";
import { defaultKeymap, history, historyKeymap, indentWithTab } from "@codemirror/commands";
import { HighlightStyle, syntaxHighlighting } from "@codemirror/language";
import { markdown } from "@codemirror/lang-markdown";
import { languages } from "@codemirror/language-data";
import { tags } from "@lezer/highlight";
import { Vim, vim } from "@replit/codemirror-vim";
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

/** Builds the editor state for a draft; exported so tests can drive the keymap. */
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
        if (u.docChanged) session.edit(u.state.doc.toString());
      }),
    ],
  });
}

let exDefined = false;
function defineEx() {
  if (exDefined) return;
  exDefined = true;
  // :w saves now; the editor never closes, so :wq and :q just save too.
  Vim.defineEx("write", "w", (cm: { cm6: EditorView }) => {
    void flushFor(cm.cm6);
  });
  Vim.defineEx("wq", "wq", (cm: { cm6: EditorView }) => {
    void flushFor(cm.cm6);
  });
}

const owners = new WeakMap<EditorView, Session>();
function flushFor(view: EditorView) {
  return owners.get(view)?.flush() ?? Promise.resolve();
}

/**
 * A CodeMirror 6 markdown editor with vim keybindings over a session's
 * draft. The editor state is parked on the session between mounts so a
 * mode switch keeps the cursor and undo history, and it is rebuilt when
 * the session replaces the draft from disk.
 */
export function Editor({ session }: { session: Session }) {
  const host = useRef<HTMLDivElement>(null);
  const generation = session.state.generation;

  useEffect(() => {
    defineEx();
    const parent = host.current!;
    let state: EditorState;
    if (session.editorState instanceof EditorState && session.editorGeneration === generation) {
      state = session.editorState;
    } else {
      state = createState(session.state.draft, session);
    }
    const view = new EditorView({ state, parent });
    owners.set(view, session);
    view.focus();
    return () => {
      session.editorState = view.state;
      session.editorGeneration = generation;
      view.destroy();
    };
  }, [session, generation]);

  return <div ref={host} class="editor" />;
}
