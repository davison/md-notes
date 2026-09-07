import { useEffect, useState } from "preact/hooks";
import { listRoots, type Root } from "./api";

/**
 * The three-pane shell for one root. Later tasks fill the panes with the
 * navigator, the rendered note, and search; this task only proves the
 * route, the layout, and the root lookup.
 */
export function RootView({ slug }: { slug: string }) {
  const [root, setRoot] = useState<Root | null | undefined>(undefined);

  useEffect(() => {
    listRoots().then(
      (roots) => setRoot(roots.find((r) => r.slug === slug) ?? null),
      () => setRoot(null),
    );
  }, [slug]);

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
        <p class="muted">Navigator</p>
      </aside>
      <main class="note">
        <p class="muted">Select a note.</p>
      </main>
      <aside class="side">
        <p class="muted">Search and tags</p>
      </aside>
    </div>
  );
}
