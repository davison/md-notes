import { useEffect, useRef, useState } from "preact/hooks";
import { EditorState, StateEffect, StateField, Transaction } from "@codemirror/state";
import { EditorView, drawSelection, highlightActiveLine, keymap } from "@codemirror/view";
import { defaultKeymap, history, historyKeymap, indentWithTab, isolateHistory } from "@codemirror/commands";
import { HighlightStyle, syntaxHighlighting } from "@codemirror/language";
import { markdown } from "@codemirror/lang-markdown";
import { languages } from "@codemirror/language-data";
import { tags } from "@lezer/highlight";
import { Vim, getCM, vim } from "@replit/codemirror-vim";
import type { Session } from "./session";
import { uploadImage } from "./api";
import { altFor, imageMarkdown, isImage, resourceLink } from "./image-link";

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
 * Where an image paste or drop will put its links, held while the uploads
 * run. The range is mapped through every change made meanwhile — typing,
 * a save's reload, another paste — so the links land where the reader put
 * them rather than at a stale offset. A caret's position keeps to the left
 * of text typed at that very point, so what the reader types after pasting
 * stays after the link.
 */
interface Pending {
  id: number;
  from: number;
  to: number;
}
const addPending = StateEffect.define<Pending>();
const dropPending = StateEffect.define<number>();
const pendingInserts = StateField.define<Pending[]>({
  create: () => [],
  update(value, tr) {
    let next = tr.docChanged
      ? value.map((p) => {
          if (p.from === p.to) {
            const at = tr.changes.mapPos(p.from, -1);
            return { ...p, from: at, to: at };
          }
          // A selection being replaced keeps text typed at either edge
          // outside it, so nothing the reader added is replaced too.
          const from = tr.changes.mapPos(p.from, 1);
          return { ...p, from, to: Math.max(from, tr.changes.mapPos(p.to, -1)) };
        })
      : value;
    for (const e of tr.effects) {
      if (e.is(addPending)) next = [...next, e.value];
      else if (e.is(dropPending)) next = next.filter((p) => p.id !== e.value);
    }
    return next;
  },
});
let pendingId = 0;

/** What the editor tells the reader about an image paste or drop. */
export interface Notice {
  kind: "busy" | "error";
  text: string;
}
const notifiers = new WeakMap<EditorView, (n: Notice | null) => void>();
function notify(view: EditorView, n: Notice | null) {
  notifiers.get(view)?.(n);
}

/**
 * The files an event carries that the editor takes as images. None means
 * the event is not the editor's: text, a URL and a non-image file all go
 * on to CodeMirror's own handling, as they did before images were taken.
 */
function imagesIn(list: FileList | null | undefined): { images: File[]; others: File[] } {
  const files = list ? Array.from(list) : [];
  return { images: files.filter(isImage), others: files.filter((f) => !isImage(f)) };
}

/**
 * Uploads pasted or dropped images and links them at from–to, replacing
 * whatever was selected there. The links go in as one transaction that is
 * its own history event, so one undo takes them all out and nothing typed
 * before or after joins it; the uploaded files stay. A file the daemon
 * refuses, or that never reaches it, is left out and named in the notice.
 * A paste sends no name and gets empty alt text; a drop sends the file's
 * name and uses it for the alt text.
 */
async function insertImages(
  view: EditorView,
  session: Session,
  images: File[],
  others: File[],
  from: number,
  to: number,
  dropped: boolean,
) {
  const id = ++pendingId;
  view.dispatch({ effects: addPending.of({ id, from, to }) });
  notify(view, { kind: "busy", text: images.length === 1 ? "Adding the image…" : `Adding ${images.length} images…` });
  const results = await Promise.allSettled(images.map((f) => uploadImage(session.slug, f, dropped ? f.name : undefined)));
  // A view destroyed meanwhile — the reader switched to the rendered view
  // — has nowhere to put the links; the files are uploaded, and a paste
  // or drop again links the same files without storing them twice.
  if (!notifiers.has(view)) return;
  const at = view.state.field(pendingInserts).find((p) => p.id === id);
  const links: string[] = [];
  const problems: string[] = [];
  results.forEach((r, i) => {
    const label = dropped ? images[i].name : "the pasted image";
    if (r.status === "fulfilled") {
      links.push(imageMarkdown(dropped ? altFor(images[i].name) : "", resourceLink(session.path, r.value.path)));
    } else {
      const why = r.reason instanceof Error ? r.reason.message : String(r.reason);
      problems.push(`Could not add ${label}: ${why}${why.endsWith(".") ? "" : "."}`);
    }
  });
  for (const f of others) problems.push(`Left out ${f.name}: not an image.`);
  if (!at || links.length === 0) {
    view.dispatch({ effects: dropPending.of(id) });
  } else {
    const insert = links.join("\n");
    const main = view.state.selection.main;
    // The caret follows the link when it is still where the paste was, as
    // it would after typed text; a reader who has moved on is left there.
    const follow = main.empty ? main.head === at.to || main.head === at.from : main.from === at.from && main.to === at.to;
    view.dispatch({
      changes: { from: at.from, to: at.to, insert },
      selection: follow ? { anchor: at.from + insert.length } : undefined,
      effects: dropPending.of(id),
      annotations: [isolateHistory.of("full"), Transaction.userEvent.of(dropped ? "input.drop" : "input.paste")],
      scrollIntoView: follow,
    });
  }
  notify(view, problems.length ? { kind: "error", text: problems.join(" ") } : null);
}

/**
 * Paste and drop of image files. Registered before CodeMirror's own
 * handlers run, and taking the event only when it carries an image.
 */
function imageHandlers(session: Session) {
  return EditorView.domEventHandlers({
    paste(event, view) {
      const { images, others } = imagesIn(event.clipboardData?.files);
      if (images.length === 0) return false;
      event.preventDefault();
      const { from, to } = view.state.selection.main;
      void insertImages(view, session, images, others, from, to, false);
      return true;
    },
    drop(event, view) {
      const { images, others } = imagesIn(event.dataTransfer?.files);
      if (images.length === 0) return false;
      event.preventDefault();
      let pos: number | null = null;
      try {
        pos = view.posAtCoords({ x: event.clientX, y: event.clientY });
      } catch {
        // No layout to measure against; the caret is the next best place.
      }
      const at = pos ?? view.state.selection.main.head;
      void insertImages(view, session, images, others, at, at, true);
      return true;
    },
  });
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
      pendingInserts,
      imageHandlers(session),
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
  const [notice, setNotice] = useState<Notice | null>(null);

  useEffect(() => {
    defineEx();
    const parent = host.current!;
    const parked = session.editorState instanceof EditorState ? session.editorState : null;
    const keep = parked !== null && (session.editorGeneration === generation || holdsDraft(parked, session.state.draft));
    const state = keep ? parked : createState(session.state.draft, session);
    const view = new EditorView({ state, parent });
    owners.set(view, session);
    notifiers.set(view, setNotice);
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
      notifiers.delete(view);
      view.destroy();
    };
  }, [session, generation]);

  return (
    <>
      {notice && (
        <div class={`editor-notice ${notice.kind}`} role={notice.kind === "error" ? "alert" : "status"}>
          <span>{notice.text}</span>
          {notice.kind === "error" && (
            <button type="button" onClick={() => setNotice(null)}>
              Dismiss
            </button>
          )}
        </div>
      )}
      <div ref={host} class="editor" />
    </>
  );
}
