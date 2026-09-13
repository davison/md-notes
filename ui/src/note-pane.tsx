import { useCallback, useEffect, useMemo, useRef, useState } from "preact/hooks";
import { noteURL } from "./api";
import { Editor } from "./editor";
import { NoteView } from "./note-view";
import { flushAll, getSession, hasStoredDraft, hasUnsaved, subscribeSessions, unsavedSessions, type Session, type SessionState } from "./session";

export type Mode = "view" | "edit";

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
          {state.status !== "loading" && state.status !== "error" && <Editor session={session} />}
        </main>
      ) : (
        <main class="note-body">
          <NoteView slug={slug} path={path} version={version + saved} line={line} />
        </main>
      )}
    </div>
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
