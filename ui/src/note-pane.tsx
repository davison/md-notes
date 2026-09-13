import { useCallback, useEffect, useMemo, useRef, useState } from "preact/hooks";
import { fetchNote, noteURL } from "./api";
import { NoteView } from "./note-view";
import { flushAll, getSession, hasStoredDraft, hasUnsaved, subscribeSessions, unsavedSessions, type Session, type SessionState } from "./session";
import { noteTabTitle, useDocumentTitle } from "./title";

export type Mode = "view" | "edit";

/**
 * The editor is the larger half of the bundle — CodeMirror and its table of
 * languages — and a page that is only reading a note never touches it, so
 * its chunk is fetched on the first toggle rather than with the page. Once
 * fetched it is held here, so every later toggle in this page is instant.
 */
const loadEditor = () => import("./editor");
type EditorComponent = Awaited<ReturnType<typeof loadEditor>>["Editor"];
let loadedEditor: EditorComponent | null = null;

/**
 * The editor component once wanted and arrived, and whether the attempt to
 * fetch it failed. The chunk can genuinely go missing: upgrade the daemon
 * under an open tab and the hashed name this page asks for is no longer in
 * the bundle, so the request is answered 404 and the import rejects.
 * Without a rejection path the pane would wait for it for ever, which is
 * worse than the static import this replaced.
 *
 * There is deliberately nothing here that tries again. A module script
 * whose fetch fails leaves a null entry in the browser's module map, and
 * every later `import()` of that URL rejects against that entry without
 * making a request at all — so a retry button would be a button that does
 * nothing, however healthy the network had become. Reloading is what cures
 * this: a new document gets a new module map, and asks for whatever the
 * daemon's current index.html names.
 */
function useEditor(wanted: boolean): { Editor: EditorComponent | null; failed: boolean } {
  // The initialiser is a thunk because the state *is* a function, which a
  // bare value would be mistaken for a lazy initialiser.
  const [editor, setEditor] = useState<EditorComponent | null>(() => loadedEditor);
  const [failed, setFailed] = useState(false);
  useEffect(() => {
    if (!wanted || editor || failed) return;
    let live = true;
    void loadEditor().then(
      (mod) => {
        loadedEditor = mod.Editor;
        if (live) setEditor(() => mod.Editor);
      },
      () => {
        if (live) setFailed(true);
      },
    );
    return () => {
      live = false;
    };
  }, [wanted, editor, failed]);
  return { Editor: editor, failed };
}

/** Re-renders the caller whenever the session changes. */
export function useSession(session: Session): SessionState {
  const [, bump] = useState(0);
  useEffect(() => session.subscribe(() => bump((n) => n + 1)), [session]);
  return session.state;
}

/**
 * Whether a keydown is the view/edit toggle: Ctrl+E with no other
 * modifier. A chord, so plain typing in the editor or the search box
 * never trips it; handled in the capture phase so it reaches the editor
 * before vim's own Ctrl+E.
 */
export function isToggleKey(e: KeyboardEvent): boolean {
  return e.ctrlKey && !e.altKey && !e.metaKey && !e.shiftKey && (e.key === "e" || e.key === "E");
}

interface Props {
  slug: string;
  path: string;
  /** Bumped when the daemon reports a change to this note. */
  version?: number;
  /** Source line to scroll the rendered view to, from a search hit. */
  line?: number | null;
}

/**
 * The note pane: a rendered view or an editor over the same note, with a
 * bar showing the mode and the save state, and a banner when the file on
 * disk and the draft disagree.
 */
export function NotePane({ slug, path, version = 0, line = null }: Props) {
  // Keyed by note in RootView, so each note mounts its own pane.
  const session = useMemo(() => getSession(slug, path), [slug, path]);
  const state = useSession(session);
  // A note with a retained draft, in memory or left by a previous page,
  // opens in the editor so the draft is never out of sight.
  const [mode, setMode] = useState<Mode>(() =>
    hasUnsaved(session.state) || (!session.opened && hasStoredDraft(slug, path)) ? "edit" : "view",
  );

  // Entering the editor reads or rechecks the note; leaving it saves now.
  useEffect(() => {
    if (mode === "edit") void session.open();
    else void session.flush();
  }, [session, mode]);

  // Navigating away saves now; the session keeps anything that does not land.
  useEffect(() => () => void session.flush(), [session]);

  // A change the daemon reported is checked against the draft.
  useEffect(() => {
    if (version > 0) void session.changed();
  }, [session, version]);

  // A save landing while viewing refreshes the render, which is otherwise
  // only refetched on the daemon's change batch.
  const [saved, setSaved] = useState(0);
  const lastRevision = useRef<string | undefined>(undefined);
  const revision = state.base?.revision;
  useEffect(() => {
    if (lastRevision.current !== undefined && revision !== undefined && revision !== lastRevision.current) {
      setSaved((n) => n + 1);
    }
    lastRevision.current = revision;
  }, [revision]);

  // The tab says which note is open. The note's own title comes from the
  // daemon, so the last one reported for this note is kept here; until one
  // arrives the file name stands in, and the marker follows the session
  // live. The pane is keyed by note, so a kept title never outlives it.
  const [title, setTitle] = useState<string | null>(null);
  useDocumentTitle(noteTabTitle(path, title, state));

  // In view mode the rendered view reports the title, refetching whenever
  // the note changes. In the editor it is unmounted and reports nothing, so
  // the pane asks for itself — through the same endpoint — whenever the text
  // under the editor is replaced from outside it: the first read, a live
  // update, a conflict resolved by loading the file. That is what the
  // session's generation counts, and a save of the reader's own typing does
  // not bump it, so this does not fire on every autosave.
  const generation = state.generation;
  const unread = state.base === null;
  const lost = state.status === "error";
  useEffect(() => {
    if (mode !== "edit") return;
    if (unread) {
      // A note the session read and then lost has no title left to keep;
      // before a first read the rendered view's last one still stands.
      if (lost) setTitle(null);
      return;
    }
    let cancelled = false;
    fetchNote(slug, path).then(
      (n) => {
        if (!cancelled) setTitle(n.title);
      },
      () => {
        if (!cancelled) setTitle(null);
      },
    );
    return () => {
      cancelled = true;
    };
  }, [mode, slug, path, generation, unread, lost]);
  const editor = useEditor(mode === "edit");

  const toggle = useCallback(() => setMode((m) => (m === "view" ? "edit" : "view")), []);

  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if (!isToggleKey(e)) return;
      e.preventDefault();
      e.stopPropagation();
      toggle();
    };
    document.addEventListener("keydown", onKey, true);
    return () => document.removeEventListener("keydown", onKey, true);
  }, [toggle]);

  return (
    <div class="note-pane">
      <div class="note-bar">
        <button type="button" class="mode-toggle" onClick={toggle} title="Ctrl+E">
          {mode === "edit" ? "View" : "Edit"}
        </button>
        <span class="mode-name">{mode === "edit" ? "Editing" : "Viewing"}</span>
        <SaveStatus session={session} state={state} mode={mode} />
      </div>
      {state.status === "conflict" && state.conflict && <ConflictBanner session={session} state={state} />}
      {mode === "edit" ? (
        <main class="editor-body">
          {state.status === "loading" && <p class="muted pad">Loading…</p>}
          {state.status === "error" && <p class="error pad">{state.error?.message}</p>}
          {state.status !== "loading" && state.status !== "error" && <EditorBody editor={editor} session={session} />}
        </main>
      ) : (
        <main class="note-body">
          <NoteView slug={slug} path={path} version={version + saved} line={line} onTitle={setTitle} />
        </main>
      )}
    </div>
  );
}

/** The editor, the wait for its chunk, or the failure to fetch it. */
function EditorBody({ editor, session }: { editor: ReturnType<typeof useEditor>; session: Session }) {
  if (editor.Editor) return <editor.Editor session={session} />;
  if (!editor.failed) return <p class="muted pad">Loading the editor…</p>;
  return (
    <p class="error pad" role="alert">
      The editor could not be loaded. The daemon was most likely updated while this page was open, so the bundle
      this page is asking for is no longer the one being served. Reloading fetches the current one; the note is
      unaffected and any unsaved draft is kept.{" "}
      <button type="button" onClick={() => location.reload()}>
        Reload the page
      </button>
    </p>
  );
}

function SaveStatus({ session, state, mode }: { session: Session; state: SessionState; mode: Mode }) {
  switch (state.status) {
    case "loading":
      return mode === "edit" ? <span class="save-status muted">Loading…</span> : null;
    case "error":
      return mode === "edit" ? <span class="save-status error">{state.error?.message}</span> : null;
    case "clean":
      return <span class="save-status muted">Saved</span>;
    case "pending":
      return <span class="save-status">Unsaved changes</span>;
    case "saving":
      return <span class="save-status">Saving…</span>;
    case "failed":
      return (
        <span class="save-status error">
          Save failed: {state.error?.message}. Draft kept.{" "}
          <button type="button" onClick={() => void session.retry()}>
            Retry
          </button>
        </span>
      );
    case "conflict":
      return <span class="save-status error">Conflict: draft kept</span>;
  }
}

function ConflictBanner({ session, state }: { session: Session; state: SessionState }) {
  const [copied, setCopied] = useState(false);
  const copy = () => {
    navigator.clipboard?.writeText(state.draft).then(
      () => setCopied(true),
      () => setCopied(false),
    );
  };
  const deleted = state.conflict?.kind === "deleted";
  return (
    <div class="conflict" role="alert">
      <p>
        {deleted
          ? "This note was deleted on disk while you had unsaved edits. Your draft is kept here, but the file " +
            "cannot be recreated from the editor: copy the draft, or recreate the file with another tool and " +
            "the conflict will become resolvable."
          : "This note changed on disk while you had unsaved edits. Your draft is kept: save it over the current " +
            "file, or drop it and load the file. Switching to View shows the file as it is now."}
      </p>
      <p class="conflict-actions">
        {!deleted && (
          <button type="button" onClick={() => void session.keepDraft()}>
            Keep my draft
          </button>
        )}
        {!deleted && (
          <button type="button" onClick={() => session.loadFile()}>
            Load the file
          </button>
        )}
        <button type="button" onClick={copy}>
          {copied ? "Copied" : "Copy draft"}
        </button>
        {deleted && (
          <button type="button" onClick={() => session.discard()}>
            Discard draft
          </button>
        )}
      </p>
    </div>
  );
}

/**
 * Page-wide guard: pending edits are sent when the window loses focus or
 * is hidden, and closing the page with unsaved work sends a final save
 * and asks before leaving. The final save is a keepalive request, which
 * browsers cap at 64 KiB of body: a larger draft is refused, stays in
 * storage, and is recovered on the next open.
 */
export function useUnsavedGuard() {
  useEffect(() => {
    const onBlur = () => void flushAll();
    const onVisibility = () => {
      if (document.visibilityState === "hidden") void flushAll(true);
    };
    const onUnload = (e: BeforeUnloadEvent) => {
      if (unsavedSessions().length === 0) return;
      void flushAll(true);
      e.preventDefault();
      e.returnValue = "";
    };
    window.addEventListener("blur", onBlur);
    document.addEventListener("visibilitychange", onVisibility);
    window.addEventListener("beforeunload", onUnload);
    return () => {
      window.removeEventListener("blur", onBlur);
      document.removeEventListener("visibilitychange", onVisibility);
      window.removeEventListener("beforeunload", onUnload);
    };
  }, []);
}

/** Notes other than the open one that still hold unsaved work, as links. */
export function UnsavedDrafts({ slug, current }: { slug: string; current: string }) {
  const [, bump] = useState(0);
  useEffect(() => subscribeSessions(() => bump((n) => n + 1)), []);
  const others = unsavedSessions().filter((s) => !(s.slug === slug && s.path === current));
  if (others.length === 0) return null;
  return (
    <span class="unsaved-drafts">
      Unsaved:{" "}
      {others.map((s, i) => (
        <span key={s.key}>
          {i > 0 && ", "}
          <a href={noteURL(s.slug, s.path)}>{s.path}</a>
        </span>
      ))}
    </span>
  );
}
