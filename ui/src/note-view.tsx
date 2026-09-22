import { useEffect, useLayoutEffect, useRef, useState } from "preact/hooks";
import { diagramURL, fetchNote, type Note } from "./api";
import { DiagramViewer } from "./diagram-viewer";
import {
  DiagramPool,
  OPEN_CLASS,
  decorate,
  diagramKeys,
  followViewed,
  holdInView,
  useDiagramTheme,
  type ViewedDiagram,
} from "./diagrams";
import { animationsOff } from "./settings";

/**
 * A rendered note: title, a collapsed metadata panel for its frontmatter,
 * and the sanitised HTML body from the daemon. In-app links are handled by
 * the router; fragment links scroll within the note.
 */
interface NoteProps {
  slug: string;
  path: string;
  version?: number;
  /** Source line to scroll to, from a search hit. */
  line?: number | null;
  /**
   * Called with the daemon's title for the note whenever a fetch lands, and
   * with null when one fails: a note that no longer renders has no title
   * for the caller to go on holding.
   */
  onTitle?: (title: string | null) => void;
}

export function NoteView({ slug, path, version = 0, line = null, onTitle }: NoteProps) {
  const [note, setNote] = useState<Note | null>(null);
  const noteRef = useRef(note);
  noteRef.current = note;
  const [error, setError] = useState<string | null>(null);
  const body = useRef<HTMLDivElement>(null);
  const theme = useDiagramTheme();
  const pool = useRef(new DiagramPool());
  // The diagram whose natural-size view is open, and what the view shows of
  // it now. See the effect that follows it, below.
  const [view, setView] = useState<View | null>(null);
  const viewRef = useRef(view);
  viewRef.current = view;

  // Opening a different note clears the pane; a version bump for the same
  // note refetches in place so a live update does not flash.
  const shown = useRef("");
  useEffect(() => {
    let cancelled = false;
    // Abandoned when the reader moves on, so the daemon stops measuring a
    // note nobody is reading (review of PR #181, N2).
    const abort = new AbortController();
    const key = slug + "\0" + path;
    if (shown.current !== key) {
      shown.current = key;
      setNote(null);
      setError(null);
      setView(null);
    }
    fetchNote(slug, path, { sizes: true, signal: abort.signal }).then(
      (n) => {
        if (cancelled) return;
        setNote(n);
        setError(null);
        onTitle?.(n.title);
      },
      (e: Error) => {
        if (cancelled) return;
        setError(e.message);
        onTitle?.(null);
      },
    );
    return () => {
      cancelled = true;
      abort.abort();
    };
    // onTitle is deliberately not a dependency: it is reported from the
    // fetch, and refetching because the caller passed a fresh closure
    // would be a loop.
  }, [slug, path, version]);

  // The flowcharts go in front of their code blocks before the frame is
  // painted, so a note with a diagram does not show its source first; and
  // before the scroll below, so a search hit on a diagram flashes the image.
  // A different note starts a fresh pool: nothing of the last one is reused.
  const pooled = useRef("");
  useLayoutEffect(() => {
    const key = slug + "\0" + path;
    if (pooled.current !== key) {
      pooled.current = key;
      pool.current = new DiagramPool();
    }
    if (!note || !body.current) return;
    decorate(body.current, note.diagrams ?? [], (hash) => diagramURL(slug, path, hash, theme), pool.current);
  }, [note, slug, path, theme]);

  // Scroll to the top of a newly opened note, or to its fragment. A live
  // refetch of the same note keeps the reader's place. A requested line
  // wins over both and is honoured whenever it changes.
  const opened = useRef("");
  const scrolled = useRef("");
  useEffect(() => {
    if (!note) return;
    const key = slug + "\0" + path;
    const fresh = opened.current !== key;
    opened.current = key;
    if (line !== null) {
      // Honour a requested line once per note and line, so a live refetch
      // of the same note keeps the reader's place.
      const lineKey = key + "\0" + line;
      if (scrolled.current === lineKey) return;
      scrolled.current = lineKey;
      const target = lineTarget(body.current, line);
      if (target) {
        target.scrollIntoView({ block: "center" });
        // A diagram above the target that the daemon did not measure has no
        // box until it loads, and any diagram that fails gives its place to
        // a code block of another height; keep the target where it was put
        // as each of those settles (#177, #189).
        const release = holdInView(target);
        // No flash when animations are off: the class is not added at all,
        // rather than added and left to a stylesheet that has cancelled the
        // animation, so nothing repaints and nothing is left behind if the
        // page navigates before the timer. Centring the block is what says
        // where the hit is; that is a scroll, not an animation.
        if (animationsOff()) return release;
        const flash = target.classList.contains("line-anchor") ? (target.nextElementSibling ?? target) : target;
        flash.classList.add("flash");
        const t = setTimeout(() => flash.classList.remove("flash"), 1500);
        return () => {
          clearTimeout(t);
          release();
        };
      }
      // No block at or before that line (a hit in the frontmatter, say):
      // fall through to the usual top-of-note behaviour for a fresh open.
    }
    if (!fresh) return;
    const pane = body.current?.closest("main");
    const target = fragmentTarget(body.current, window.location.hash);
    if (target) target.scrollIntoView();
    else if (pane) pane.scrollTop = 0;
  }, [note, slug, path, line]);

  // The natural-size view shows the diagram as it is now, not as it was
  // when it opened (review of PR #202, finding 1). Whatever changes the
  // note's images — a new palette is a new src, a live update re-decorates,
  // an image that fails takes its box away — is a change to the markup
  // below, so the view is checked against the pool on every one of them. A
  // diagram edited in place is followed to its new drawing; one that is gone
  // closes the view, and focus goes to the note's title.
  //
  // One observer for as long as the view is open, reading the note and the
  // view through refs, which are current by the time any mutation is
  // delivered: the list it checks against is always the one the pool was
  // just decorated from. An observer tied to each version of the note would
  // still be connected when the next version is decorated, since an effect's
  // cleanup runs after the commit, and would judge the new images against
  // the old list — which is how the view once lost focus to the title on an
  // edit it then went on to follow (review of PR #202, round two).
  const viewOpen = view !== null;
  useEffect(() => {
    const scope = body.current;
    if (!viewOpen || !scope) return;
    const check = () => {
      const current = viewRef.current;
      const shown = noteRef.current;
      if (!current || !shown) return;
      const now = followViewed(pool.current, shown.diagrams ?? [], current.viewed);
      if (!now) {
        closeView();
        return;
      }
      const next = snapshot(now.image, now.viewed);
      if (sameView(current, next)) return;
      viewRef.current = next;
      setView(next);
    };
    check();
    const observer = new MutationObserver(check);
    observer.observe(scope, { subtree: true, childList: true, attributes: true, attributeFilter: ["src"] });
    return () => observer.disconnect();
  }, [viewOpen]);

  // A note that stops rendering — deleted, say — takes the view and the
  // title it would hand focus to with it; focus goes to the error's title.
  const article = useRef<HTMLElement>(null);
  useLayoutEffect(() => {
    if (!error || !viewRef.current) return;
    viewRef.current = null;
    setView(null);
    article.current?.querySelector<HTMLElement>(".note-title")?.focus();
  }, [error]);

  /**
   * Closes the view and puts focus back on the diagram it showed, or on the
   * note's title when that diagram is no longer in the note. Focus moves
   * before the view goes, so it never falls to the document's body, and
   * without scrolling: the note stays where the reader left it, rather than
   * jumping to a title far above (review of PR #204).
   */
  const closeView = () => {
    const current = viewRef.current;
    const shown = noteRef.current;
    const now = current && shown ? followViewed(pool.current, shown.diagrams ?? [], current.viewed) : null;
    const target = now?.image.parentElement ?? article.current?.querySelector<HTMLElement>(".note-title");
    target?.focus({ preventScroll: true });
    viewRef.current = null;
    setView(null);
  };

  const onClick = (e: MouseEvent) => {
    // A diagram opens at its natural size: a click, a tap, or Enter or
    // Space on its button, which the browser turns into a click.
    const open = (e.target as HTMLElement | null)?.closest<HTMLElement>(`button.${OPEN_CLASS}`);
    if (open && body.current?.contains(open) && note) {
      const keys = diagramKeys(note.diagrams ?? []);
      for (const [key, img] of pool.current) {
        if (img.parentElement !== open) continue;
        setView(snapshot(img, { key, index: keys.indexOf(key), keys }));
        break;
      }
      return;
    }
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
      <article ref={article}>
        <h1 class="note-title" tabIndex={-1}>
          {path.split("/").pop()}
        </h1>
        <p class="error">{error}</p>
      </article>
    );
  }
  if (!note) return <p class="muted">Loading…</p>;

  return (
    <article ref={article} class="note-article" onClick={onClick}>
      <h1 class="note-title" tabIndex={-1}>
        {note.title}
      </h1>
      {note.frontmatter && Object.keys(note.frontmatter).length > 0 && <Metadata data={note.frontmatter} />}
      <div ref={body} class="markdown" dangerouslySetInnerHTML={{ __html: note.html }} />
      {view && (
        <DiagramViewer src={view.src} alt={view.alt} width={view.width} height={view.height} onClose={closeView} />
      )}
    </article>
  );
}

/** An open natural-size view: which diagram, and what is shown of it. */
interface View {
  viewed: ViewedDiagram;
  src: string;
  alt: string;
  width?: string;
  height?: string;
}

function snapshot(img: HTMLImageElement, viewed: ViewedDiagram): View {
  return {
    viewed,
    src: img.getAttribute("src") ?? "",
    alt: img.alt,
    width: img.getAttribute("width") ?? undefined,
    height: img.getAttribute("height") ?? undefined,
  };
}

function sameView(a: View, b: View): boolean {
  return (
    a.viewed.key === b.viewed.key &&
    a.viewed.index === b.viewed.index &&
    a.viewed.keys.join("\n") === b.viewed.keys.join("\n") &&
    a.src === b.src &&
    a.alt === b.alt &&
    a.width === b.width &&
    a.height === b.height
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

/**
 * The block that starts at or nearest before a source line, using the
 * data-line markers the daemon puts on block elements.
 */
export function lineTarget(scope: Element | null, line: number): Element | null {
  if (!scope) return null;
  let best: Element | null = null;
  let bestLine = -1;
  for (const el of scope.querySelectorAll("[data-line]")) {
    const n = Number(el.getAttribute("data-line"));
    if (!Number.isFinite(n) || n > line || n < bestLine) continue;
    best = el;
    bestLine = n;
  }
  return best;
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
