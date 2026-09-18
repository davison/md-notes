import { useCallback, useEffect, useMemo, useState } from "preact/hooks";
import { useLocation } from "preact-iso";
import { fetchTags, fetchTree, listRoots, noteURL, type Root, type Tag, type TreeNode } from "./api";
import { Drawer, FindToggle, NavToggle, useDrawer } from "./drawer";
import { affects, affectsTree, type LiveUpdate, useEvents } from "./events";
import { Navigator } from "./navigator";
import { NewNoteButton, NewNoteDialog } from "./new-note";
import { folderOf } from "./note-name";
import { NotePane, UnsavedDrafts, useUnsavedGuard } from "./note-pane";
import { SearchPane } from "./search-pane";
import { SettingsMenu } from "./settings-panel";
import { TagPanel, tagURL } from "./tag-panel";
import { rootTabTitle, useDocumentTitle } from "./title";

/**
 * The three-pane shell for one root: navigator, note, and the search and
 * tags pane. `note` is the wildcard remainder of
 * the route, already URL-decoded by the router.
 *
 * Below the narrow breakpoint the same markup is a different application:
 * the note has the viewport under a compact top bar, and the two side panes
 * are tabs of one drawer the top bar's burger and magnifier open. The
 * stylesheet makes that switch; see ./drawer.
 */
export function RootView({ slug, note }: { slug: string; note?: string }) {
  const [root, setRoot] = useState<Root | null | undefined>(undefined);
  /**
   * Why the roots listing did not produce a root, when it did not. A listing
   * that *failed* and a listing that *succeeded without this slug* are two
   * different things and were once the same `null`: with the service worker
   * opening the app offline, that made every note route say the root did not
   * exist — "my notes are gone" — where the daemon being unreachable was the
   * whole of it.
   */
  const [rootError, setRootError] = useState<string | null>(null);
  const [tree, setTree] = useState<TreeNode | null>(null);
  const [treeError, setTreeError] = useState<string | null>(null);
  // Bumped when the open note changes on disk, so NoteView refetches.
  const [noteVersion, setNoteVersion] = useState(0);
  // Bumped when the tree may have changed, so search results and tags refresh.
  const [treeVersion, setTreeVersion] = useState(0);
  const [tags, setTags] = useState<Tag[] | null>(null);
  const [live, setLive] = useState<LiveUpdate | null>(null);
  /**
   * The create prompt, when it is open, and what it opens with: an empty
   * name for the top bar's control, and the lost note's path and orphaned
   * draft when the deleted-on-disk banner is recovering a note.
   */
  const [creating, setCreating] = useState<{ name: string; body?: string } | null>(null);
  const drawer = useDrawer();
  const current = note ?? "";
  const { query, route } = useLocation();
  const line = query.l && /^\d+$/.test(query.l) ? Number(query.l) : null;
  const activeTag = query.tag ? query.tag.toLowerCase() : null;
  const only = useMemo(() => {
    if (!activeTag || !tags) return null;
    const t = tags.find((x) => x.name === activeTag);
    return new Set(t ? t.notes : []);
  }, [activeTag, tags]);

  useEffect(() => {
    setRoot(undefined);
    setRootError(null);
    listRoots().then(
      (roots) => setRoot(roots.find((r) => r.slug === slug) ?? null),
      (e: Error) => setRootError(e.message),
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

  // With a note open the pane below owns the tab title, since it is the
  // side that knows the note's title and its save state; this view names
  // the root itself when no note is open.
  useDocumentTitle(rootTabTitle(root, slug, current !== ""));

  /**
   * The navigator's selected folder, which is where a bare title becomes a
   * note. The navigator has no folder selection of its own — its rows are
   * links and its directories only open and close — so the folder of the
   * open note is the one thing on screen that says where the reader is; with
   * no note open that is the root. The prompt names the folder it will use,
   * and a "/" in the name overrides it either way.
   */
  const folder = folderOf(current);

  /**
   * Where the app goes when the open note is deleted. There is no route for
   * a folder in this application — /r/:slug/:note* is a note or nothing — so
   * the parent folder and the root's home are one page, and the navigator
   * arrives with the deleted note's folder still open, because expansion is
   * remembered per root and its ancestors were expanded while it was.
   */
  const rootHome = `/r/${encodeURIComponent(slug)}/`;

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
    // The root was unregistered from the home page, or from another tab.
    // There is nothing here to look at any more and no route to stay on,
    // so this tab goes back to the list of roots (M7-R2).
    () => route("/", true),
  );

  // Before the two states below, because neither of them is true: nothing was
  // listed, so nothing is known about this root either way.
  if (rootError) {
    return (
      <main class="page">
        <h1>mdn</h1>
        <p class="error">{rootError}</p>
        <p>
          <a href="/">Back to roots</a>
        </p>
      </main>
    );
  }
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
        <NavToggle state={drawer} />
        <a href="/" class="brand">mdn</a>
        <NewNoteButton onClick={() => setCreating({ name: "" })} />
        <span class="root-name">{root.slug}</span>
        <span class="path">{root.path}</span>
        <span class="topbar-spacer" />
        <TagChip slug={slug} current={current} active={activeTag} />
        <UnsavedDrafts slug={slug} current={current} />
        <FindToggle state={drawer} />
        <SettingsMenu />
      </header>
      <Drawer state={drawer}>
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
        <aside class="side">
          <SearchPane slug={slug} refresh={treeVersion} keep={activeTag ? { tag: activeTag } : {}} />
          <TagPanel slug={slug} tags={tags} active={activeTag} current={current} />
        </aside>
      </Drawer>
      <div class="note">
        {/* The same notice twice, one shown at each width: in the navigator
            where it has always been, and above the note for the narrow
            layout, where the navigator is behind a drawer and a caveat
            nobody opens is a caveat nobody reads. The stylesheet shows one
            and hides the other, so only one is ever in the page. */}
        <LiveUpdateNotice live={live} />
        {current ? (
          <NotePane
            key={slug + "\0" + current}
            slug={slug}
            path={current}
            version={noteVersion}
            line={line}
            onDeleted={() => route(rootHome)}
            onRecreate={(lost, draft) => setCreating({ name: lost, body: draft })}
          />
        ) : (
          <main class="note-body">
            <p class="muted">Select a note.</p>
          </main>
        )}
      </div>
      {/* Outside the drawer, which at narrow widths is a fixed, scrolling
          box of its own: a dialog that asks about the whole application
          belongs over it rather than inside it. */}
      {creating && (
        <NewNoteDialog
          slug={slug}
          folder={folder}
          name={creating.name}
          body={creating.body}
          onClose={() => setCreating(null)}
          onCreated={(path) => {
            setCreating(null);
            drawer.close();
            route(noteURL(slug, path));
          }}
        />
      )}
    </div>
  );
}

/**
 * The active tag filter, in the top bar. At narrow widths the tag panel is
 * behind a drawer, so the filter that is pruning the navigator would
 * otherwise be invisible and its only escape two taps away; the chip both
 * says which tag is filtering and clears it. At wide widths the panel is on
 * screen with its own clear link and the stylesheet hides this.
 */
function TagChip({ slug, current, active }: { slug: string; current: string; active: string | null }) {
  if (!active) return null;
  return (
    <a class="tag-chip" href={tagURL(slug, current, null)} aria-label={`Clear the tag filter ${active}`}>
      <span class="tag-chip-name">#{active}</span>
      <span class="tag-chip-x" aria-hidden="true">
        ×
      </span>
    </a>
  );
}

/** The remedy for directories the filesystem refused, which no limit covers. */
function refusedFix(n: number, reason?: string) {
  const what = n === 1 ? "directory" : "directories";
  return `give the daemon access to the ${n.toLocaleString()} ${what} it could not watch${
    reason ? ` (${reason})` : ""
  }`;
}

/** Joins remedies as prose: "a", "a and b", "a, b and c". */
function remedies(fix: string[]) {
  if (fix.length < 3) return fix.join(" and ");
  return `${fix.slice(0, -1).join(", ")} and ${fix[fix.length - 1]}`;
}

/**
 * Says so when live update covers only part of the root — the watch budget
 * is spent, the kernel is out of watches, or the filesystem refused a
 * directory — or none of it, when the daemon has no watcher for the root at
 * all. Changes in an unwatched directory still arrive when a watched one
 * reports them or the daemon restarts, so a limited coverage is a caveat
 * rather than an error.
 *
 * Each cause carries its own remedy: a directory the daemon may not read is
 * not the kernel's watch pool running dry, and raising
 * fs.inotify.max_user_watches would never cover it (M8-R7).
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
    live.refused > 0 ? refusedFix(live.refused, live.reason) : "",
  ].filter(Boolean);
  return (
    <p class="notice">
      Live update covers {live.watched.toLocaleString()} of{" "}
      {(live.watched + live.unwatched).toLocaleString()} directories in this root. A
      change in one of the other {live.unwatched.toLocaleString()} shows up when a
      watched directory reports it or the daemon restarts
      {fix.length > 0 ? ` — to cover them all, ${remedies(fix)}` : ""}.
    </p>
  );
}
