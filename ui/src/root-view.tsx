import { useCallback, useEffect, useMemo, useState } from "preact/hooks";
import { useLocation } from "preact-iso";
import { fetchTags, fetchTree, listRoots, type Root, type Tag, type TreeNode } from "./api";
import { affects, affectsTree, type LiveUpdate, useEvents } from "./events";
import { Navigator } from "./navigator";
import { NotePane, UnsavedDrafts, useUnsavedGuard } from "./note-pane";
import { SearchPane } from "./search-pane";
import { TagPanel } from "./tag-panel";

/**
 * The three-pane shell for one root: navigator, note, and the search and
 * tags pane. `note` is the wildcard remainder of
 * the route, already URL-decoded by the router.
 */
export function RootView({ slug, note }: { slug: string; note?: string }) {
  const [root, setRoot] = useState<Root | null | undefined>(undefined);
  const [tree, setTree] = useState<TreeNode | null>(null);
  const [treeError, setTreeError] = useState<string | null>(null);
  // Bumped when the open note changes on disk, so NoteView refetches.
  const [noteVersion, setNoteVersion] = useState(0);
  // Bumped when the tree may have changed, so search results and tags refresh.
  const [treeVersion, setTreeVersion] = useState(0);
  const [tags, setTags] = useState<Tag[] | null>(null);
  const [live, setLive] = useState<LiveUpdate | null>(null);
  const current = note ?? "";
  const { query } = useLocation();
  const line = query.l && /^\d+$/.test(query.l) ? Number(query.l) : null;
  const activeTag = query.tag ? query.tag.toLowerCase() : null;
  const only = useMemo(() => {
    if (!activeTag || !tags) return null;
    const t = tags.find((x) => x.name === activeTag);
    return new Set(t ? t.notes : []);
  }, [activeTag, tags]);

  useEffect(() => {
    setRoot(undefined);
    listRoots().then(
      (roots) => setRoot(roots.find((r) => r.slug === slug) ?? null),
      () => setRoot(null),
    );
  }, [slug]);

  const loadTree = useCallback(() => {
    fetchTree(slug).then(
      (t) => {
        setTree(t);
        setTreeError(null);
      },
      (e: Error) => setTreeError(e.message),
    );
  }, [slug]);

  useEffect(() => {
    if (!root) return;
    setTree(null);
    setTreeError(null);
    loadTree();
  }, [root, loadTree]);

  useEffect(() => {
    if (!root) return;
    let cancelled = false;
    fetchTags(slug).then(
      (t) => {
        if (!cancelled) setTags(t);
      },
      () => {
        if (!cancelled) setTags([]);
      },
    );
    return () => {
      cancelled = true;
    };
  }, [root, slug, treeVersion]);

  useUnsavedGuard();

  useEvents(
    slug,
    (paths) => {
      if (affectsTree(paths)) {
        loadTree();
        setTreeVersion((v) => v + 1);
      }
      if (current && affects(paths, current)) setNoteVersion((v) => v + 1);
    },
    setLive,
  );

  if (root === undefined) return <main class="page muted">Loading…</main>;
  if (root === null) {
    return (
      <main class="page">
        <h1>Unknown root</h1>
        <p>
          No root named <code>{slug}</code>. <a href="/">Back to roots</a>
        </p>
      </main>
    );
  }

  return (
    <div class="shell">
      <header class="topbar">
        <a href="/" class="brand">mdn</a>
        <span class="root-name">{root.slug}</span>
        <span class="path">{root.path}</span>
        <UnsavedDrafts slug={slug} current={current} />
      </header>
      <aside class="nav">
        <LiveUpdateNotice live={live} />
        {treeError && <p class="error">{treeError}</p>}
        {!tree && !treeError && <p class="muted">Loading…</p>}
        {tree && (
          <Navigator
            slug={slug}
            tree={tree}
            current={current}
            only={only}
            query={activeTag ? `?tag=${encodeURIComponent(activeTag)}` : ""}
          />
        )}
      </aside>
      <div class="note">
        {current ? (
          <NotePane key={slug + "\0" + current} slug={slug} path={current} version={noteVersion} line={line} />
        ) : (
          <main class="note-body">
            <p class="muted">Select a note.</p>
          </main>
        )}
      </div>
      <aside class="side">
        <SearchPane slug={slug} refresh={treeVersion} keep={activeTag ? { tag: activeTag } : {}} />
        <TagPanel slug={slug} tags={tags} active={activeTag} current={current} />
      </aside>
    </div>
  );
}

/**
 * Says so when live update covers only part of the root — the watch budget
 * is spent, or the kernel refused watches — or none of it, when the daemon
 * has no watcher for the root at all. Changes in an unwatched directory
 * still arrive when a watched one reports them or the daemon restarts, so
 * a limited coverage is a caveat rather than an error.
 */
function LiveUpdateNotice({ live }: { live: LiveUpdate | null }) {
  if (!live) return null;
  if (live === "unavailable") {
    return (
      <p class="notice">
        Live update is not available for this root: the daemon could not watch it, and
        the daemon's log says why. Changes on disk show up when you reload the page.
      </p>
    );
  }
  if (!live.limited) return null;
  const fix = [
    live.overBudget ? `raise max_watches above ${live.budget.toLocaleString()}` : "",
    live.failed > 0 ? "raise fs.inotify.max_user_watches" : "",
  ].filter(Boolean);
  return (
    <p class="notice">
      Live update covers {live.watched.toLocaleString()} of{" "}
      {(live.watched + live.unwatched).toLocaleString()} directories in this root. A
      change in one of the other {live.unwatched.toLocaleString()} shows up when a
      watched directory reports it or the daemon restarts
      {fix.length > 0 ? ` — to cover them all, ${fix.join(", and ")}` : ""}.
    </p>
  );
}
