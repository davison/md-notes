import { useEffect, useRef, useState } from "preact/hooks";
import { fetchNote, type Note } from "./api";

/**
 * A rendered note: title, a collapsed metadata panel for its frontmatter,
 * and the sanitised HTML body from the daemon. In-app links are handled by
 * the router; fragment links scroll within the note.
 */
export function NoteView({ slug, path, version = 0 }: { slug: string; path: string; version?: number }) {
  const [note, setNote] = useState<Note | null>(null);
  const [error, setError] = useState<string | null>(null);
  const body = useRef<HTMLDivElement>(null);

  // Opening a different note clears the pane; a version bump for the same
  // note refetches in place so a live update does not flash.
  const shown = useRef("");
  useEffect(() => {
    let cancelled = false;
    const key = slug + "\0" + path;
    if (shown.current !== key) {
      shown.current = key;
      setNote(null);
      setError(null);
    }
    fetchNote(slug, path).then(
      (n) => {
        if (cancelled) return;
        setNote(n);
        setError(null);
      },
      (e: Error) => {
        if (!cancelled) setError(e.message);
      },
    );
    return () => {
      cancelled = true;
    };
  }, [slug, path, version]);

  // Scroll to the top of a newly opened note, or to its fragment. A live
  // refetch of the same note keeps the reader's place.
  const opened = useRef("");
  useEffect(() => {
    if (!note) return;
    const key = slug + "\0" + path;
    if (opened.current === key) return;
    opened.current = key;
    const pane = body.current?.closest("main");
    const target = fragmentTarget(body.current, window.location.hash);
    if (target) target.scrollIntoView();
    else if (pane) pane.scrollTop = 0;
  }, [note, slug, path]);

  const onClick = (e: MouseEvent) => {
    const a = (e.target as HTMLElement | null)?.closest("a");
    if (!a) return;
    const href = a.getAttribute("href") ?? "";
    if (!href.startsWith("#")) return;
    const target = fragmentTarget(body.current, href);
    if (!target) return;
    e.preventDefault();
    target.scrollIntoView();
    history.replaceState(null, "", href);
  };

  if (error) {
    return (
      <article>
        <h1 class="note-title">{path.split("/").pop()}</h1>
        <p class="error">{error}</p>
      </article>
    );
  }
  if (!note) return <p class="muted">Loading…</p>;

  return (
    <article class="note-article" onClick={onClick}>
      <h1 class="note-title">{note.title}</h1>
      {note.frontmatter && Object.keys(note.frontmatter).length > 0 && <Metadata data={note.frontmatter} />}
      <div ref={body} class="markdown" dangerouslySetInnerHTML={{ __html: note.html }} />
    </article>
  );
}

/**
 * Finds the element a fragment points at, looking only inside the note so
 * a heading cannot resolve to one of the app's own elements.
 */
export function fragmentTarget(scope: Element | null, hash: string): Element | null {
  if (!scope || !hash || hash.length < 2) return null;
  let id = hash.slice(1);
  try {
    id = decodeURIComponent(id);
  } catch {
    // keep the raw id
  }
  // Compared as strings rather than built into a selector, so any id a
  // note can produce is safe to look up.
  for (const el of scope.querySelectorAll("[id]")) {
    if (el.id === id) return el;
  }
  return null;
}

export function formatValue(v: unknown): string {
  if (v === null || v === undefined) return "";
  if (Array.isArray(v)) return v.map(formatValue).join(", ");
  if (typeof v === "object") return JSON.stringify(v);
  return String(v);
}

function Metadata({ data }: { data: Record<string, unknown> }) {
  return (
    <details class="metadata">
      <summary>Metadata</summary>
      <table>
        <tbody>
          {Object.entries(data).map(([k, v]) => (
            <tr key={k}>
              <th scope="row">{k}</th>
              <td>{formatValue(v)}</td>
            </tr>
          ))}
        </tbody>
      </table>
    </details>
  );
}
