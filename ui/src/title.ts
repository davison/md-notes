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

/** The last segment of a note's path, which is the file's own name. */
export function fileName(path: string): string {
  return path.split("/").pop() || path;
}

/**
 * The tab title for an open note: the daemon's title for it — the
 * rendered H1 or the frontmatter title — behind any marker for the
 * editing session. Until the daemon has answered, and while an editor is
 * open over a note that was never rendered, the file name stands in.
 */
export function noteTabTitle(path: string, title: string | null, state: SessionState): string {
  return marker(state) + (title?.trim() || fileName(path));
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
