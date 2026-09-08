import { useEffect, useRef } from "preact/hooks";

/**
 * How much of a root the daemon watches. An unwatched directory is not
 * invisible: its changes still arrive when a watched directory above it
 * reports them, or when the daemon restarts.
 */
export type Coverage = {
  watched: number;
  unwatched: number;
  budget: number;
  overBudget: boolean;
  failed: number;
  limited: boolean;
};

/**
 * Subscribes to a root's change stream. onChange receives the relative
 * paths of a batch; after the stream reconnects it receives an empty list,
 * meaning "anything may have changed", so the caller refetches everything.
 * onCoverage, if given, receives the root's watch coverage when the stream
 * opens and whenever it changes.
 */
export function useEvents(
  slug: string,
  onChange: (paths: string[]) => void,
  onCoverage?: (coverage: Coverage) => void,
) {
  const handler = useRef(onChange);
  handler.current = onChange;
  const status = useRef(onCoverage);
  status.current = onCoverage;

  useEffect(() => {
    if (typeof EventSource === "undefined") return;
    const es = new EventSource(`/api/r/${encodeURIComponent(slug)}/events`);
    let dropped = false;
    es.addEventListener("change", (e) => {
      try {
        const batch = JSON.parse((e as MessageEvent).data) as { paths?: string[] };
        handler.current(batch.paths ?? []);
      } catch {
        handler.current([]);
      }
    });
    es.addEventListener("status", (e) => {
      try {
        status.current?.(JSON.parse((e as MessageEvent).data) as Coverage);
      } catch {
        // A status we cannot read tells us nothing; the stream carries on.
      }
    });
    es.onerror = () => {
      dropped = true;
    };
    es.onopen = () => {
      if (dropped) {
        dropped = false;
        handler.current([]);
      }
    };
    return () => es.close();
  }, [slug]);
}

/** Whether a change batch touches path, directly or through a parent. */
export function affects(paths: string[], path: string): boolean {
  if (paths.length === 0) return true;
  return paths.some((p) => p === path || path.startsWith(p + "/"));
}

/**
 * Whether a change batch can alter the navigator. A batch made only of
 * files with non-markdown extensions cannot; anything else, including a
 * deleted path whose kind is unknown, can.
 */
export function affectsTree(paths: string[]): boolean {
  if (paths.length === 0) return true;
  return paths.some((p) => {
    const name = p.slice(p.lastIndexOf("/") + 1);
    const dot = name.lastIndexOf(".");
    if (dot <= 0) return true;
    const ext = name.slice(dot + 1).toLowerCase();
    return ext === "md" || ext === "markdown";
  });
}
