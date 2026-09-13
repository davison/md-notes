import { useEffect } from "preact/hooks";
import { hasUnsaved, type SessionState } from "./session";

/**
 * The browser tab title. One note is open at a time, so the tab says which
 * one, and the rules live here rather than in the components that apply
 * them: the note pane knows the note and the save state, the root view
 * knows the root, and the other pages know only that no note is open.
 */

/** Shown by any page with no note and no root behind it. */
export const FALLBACK_TITLE = "MD Notes";

/** Leads the title of a note whose draft has not reached the file. */
export const UNSAVED_MARKER = "•";

/** Leads the title of a note whose draft and file disagree. */
export const CONFLICT_MARKER = "⚠";

/**
 * The leading marker for an editing session: a conflict outranks the
 * plain unsaved states, since it needs the reader's decision rather than
 * just time. A session with nothing outstanding contributes nothing.
 */
export function marker(state: SessionState): string {
  if (state.status === "conflict") return CONFLICT_MARKER + " ";
  return hasUnsaved(state) ? UNSAVED_MARKER + " " : "";
}

/**
 * A note's file name without its extension, which is what the daemon falls
 * back to when a note has neither a frontmatter title nor an H1
 * (internal/render). Matching it keeps the stand-in the pane shows before
 * the daemon answers from changing under the reader a moment later. A name
 * that is all extension — a dotfile — keeps its own name rather than
 * becoming nothing.
 */
export function fileTitle(path: string): string {
  const name = path.split("/").pop() || path;
  const dot = name.lastIndexOf(".");
  return dot > 0 ? name.slice(0, dot) : name;
}

/**
 * The tab title for an open note: the daemon's title for it — the
 * rendered H1 or the frontmatter title — behind any marker for the
 * editing session. Until the daemon has answered, and for a note that no
 * longer renders at all, the file name stands in.
 */
export function noteTabTitle(path: string, title: string | null, state: SessionState): string {
  return marker(state) + (title?.trim() || fileTitle(path));
}

/**
 * Sets the tab title while the caller is mounted. `null` means the caller
 * does not own the title on this render — the root view passes it when a
 * note pane below it is the one that knows the answer — and leaves the
 * title alone rather than flashing a fallback over it.
 */
export function useDocumentTitle(title: string | null): void {
  useEffect(() => {
    if (title !== null) document.title = title;
  }, [title]);
}
