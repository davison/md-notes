import { useEffect, useState } from "preact/hooks";
import { fetchTree, listRoots, type Root, type TreeNode } from "./api";
import { Navigator } from "./navigator";
import { NoteView } from "./note-view";

/**
 * The three-pane shell for one root: navigator, note, and the search and
 * tags pane that a later task fills. `note` is the wildcard remainder of
 * the route, already URL-decoded by the router.
 */
export function RootView({ slug, note }: { slug: string; note?: string }) {
  const [root, setRoot] = useState<Root | null | undefined>(undefined);
  const [tree, setTree] = useState<TreeNode | null>(null);
  const [treeError, setTreeError] = useState<string | null>(null);
  const current = note ?? "";

  useEffect(() => {
    setRoot(undefined);
    listRoots().then(
      (roots) => setRoot(roots.find((r) => r.slug === slug) ?? null),
      () => setRoot(null),
    );
  }, [slug]);

  useEffect(() => {
    if (!root) return;
    setTree(null);
    setTreeError(null);
    fetchTree(slug).then(setTree, (e: Error) => setTreeError(e.message));
  }, [root, slug]);

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
      </header>
      <aside class="nav">
        {treeError && <p class="error">{treeError}</p>}
        {!tree && !treeError && <p class="muted">Loading…</p>}
        {tree && <Navigator slug={slug} tree={tree} current={current} />}
      </aside>
      <main class="note">
        {current ? <NoteView slug={slug} path={current} /> : <p class="muted">Select a note.</p>}
      </main>
      <aside class="side">
        <p class="muted">Search and tags</p>
      </aside>
    </div>
  );
}
