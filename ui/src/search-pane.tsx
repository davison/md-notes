import { useEffect, useRef, useState } from "preact/hooks";
import { noteURL, searchNotes, type Hit } from "./api";

interface Props {
  slug: string;
  /** Bumped when the root changed on disk, to re-run the current search. */
  refresh?: number;
  /** Extra query parameters to keep on hit links, such as the tag filter. */
  keep?: Record<string, string>;
}

/**
 * The search box and its results, grouped by file. Each hit links to the
 * note with `?l=<line>` so the note view scrolls to the match.
 */
export function SearchPane({ slug, refresh = 0, keep = {} }: Props) {
  const [query, setQuery] = useState("");
  const [hits, setHits] = useState<Hit[] | null>(null);
  const [truncated, setTruncated] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const abort = useRef<AbortController | null>(null);

  useEffect(() => {
    abort.current?.abort();
    if (query.trim() === "") {
      setHits(null);
      setError(null);
      return;
    }
    const ctl = new AbortController();
    abort.current = ctl;
    const timer = setTimeout(() => {
      searchNotes(slug, query, ctl.signal).then(
        (r) => {
          if (ctl.signal.aborted) return;
          setHits(r.hits);
          setTruncated(r.truncated);
          setError(null);
        },
        (e: Error) => {
          if (ctl.signal.aborted) return;
          setError(e.message);
        },
      );
    }, 200);
    return () => {
      clearTimeout(timer);
      ctl.abort();
    };
  }, [slug, query, refresh]);

  useEffect(() => {
    setQuery("");
  }, [slug]);

  const groups = groupByPath(hits ?? []);
  const extra = Object.entries(keep)
    .map(([k, v]) => `&${encodeURIComponent(k)}=${encodeURIComponent(v)}`)
    .join("");

  return (
    <section class="search">
      <input
        type="search"
        placeholder="Search notes"
        aria-label="Search notes"
        value={query}
        onInput={(e) => setQuery((e.target as HTMLInputElement).value)}
      />
      {error && <p class="error">{error}</p>}
      {hits && hits.length === 0 && <p class="muted">No matches.</p>}
      {truncated && <p class="muted">Showing the first {hits!.length} matches.</p>}
      {groups.map((g) => (
        <div class="hit-group" key={g.path}>
          <div class="hit-path">{g.path}</div>
          {g.hits.map((h) => (
            <a key={h.line} class="hit" href={`${noteURL(slug, h.path)}?l=${h.line}${extra}`}>
              {h.before !== undefined && h.before !== "" && <span class="ctx">{h.before}</span>}
              <span class="hit-line">
                <span class="ln">{h.line}</span>
                {emphasise(h)}
              </span>
              {h.after !== undefined && h.after !== "" && <span class="ctx">{h.after}</span>}
            </a>
          ))}
        </div>
      ))}
    </section>
  );
}

export function groupByPath(hits: Hit[]): { path: string; hits: Hit[] }[] {
  const out: { path: string; hits: Hit[] }[] = [];
  for (const h of hits) {
    const last = out[out.length - 1];
    if (last && last.path === h.path) last.hits.push(h);
    else out.push({ path: h.path, hits: [h] });
  }
  return out;
}

/** Splits the line so matched ranges render as <mark>. */
export function emphasise(h: Hit) {
  const parts = [];
  let pos = 0;
  for (const [start, end] of h.matches) {
    if (start > pos) parts.push(h.text.slice(pos, start));
    parts.push(<mark key={start}>{h.text.slice(start, end)}</mark>);
    pos = end;
  }
  if (pos < h.text.length) parts.push(h.text.slice(pos));
  return parts;
}
