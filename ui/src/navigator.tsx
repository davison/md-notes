import { useEffect, useMemo, useState } from "preact/hooks";
import { noteURL, type TreeNode } from "./api";
import { readOrder, sortTree, writeOrder, type Order } from "./order";

interface Props {
  slug: string;
  tree: TreeNode;
  /** Path of the note being shown, or empty when none. */
  current: string;
  /** When set, only these note paths are shown and empty directories are pruned. */
  only?: Set<string> | null;
  /** Query string, including the leading "?", to keep on note links (such as the tag filter). */
  query?: string;
}

/** Returns a copy of tree keeping only the files in keep, pruning empty directories. */
export function filterTree(tree: TreeNode, keep: Set<string>): TreeNode {
  const children = (tree.children ?? [])
    .map((c) => (c.dir ? filterTree(c, keep) : keep.has(c.path) ? c : null))
    .filter((c): c is TreeNode => c !== null && (!c.dir || (c.children?.length ?? 0) > 0));
  return { ...tree, children };
}

function storageKey(slug: string) {
  return `mdn:nav:${slug}`;
}

function loadExpanded(slug: string): Set<string> {
  try {
    const raw = localStorage.getItem(storageKey(slug));
    if (raw) return new Set(JSON.parse(raw) as string[]);
  } catch {
    // storage unavailable or corrupt: start collapsed
  }
  return new Set();
}

function saveExpanded(slug: string, expanded: Set<string>) {
  try {
    localStorage.setItem(storageKey(slug), JSON.stringify([...expanded]));
  } catch {
    // storage unavailable: forget between loads
  }
}

/** Directories that must be open for path to be visible. */
export function ancestors(path: string): string[] {
  const parts = path.split("/");
  parts.pop();
  const out: string[] = [];
  for (let i = 1; i <= parts.length; i++) out.push(parts.slice(0, i).join("/"));
  return out;
}

/**
 * The navigator pane: a collapsible tree of the root's markdown files.
 * Expanded directories are remembered per root, and the current note's
 * ancestors are opened so it is always visible.
 *
 * Its header carries the order control, which switches the tree between
 * the daemon's alphanumeric order and last-modified with the most recent
 * note on top (M7-R3). The control lives here rather than in the root view
 * so that the wide layout's pane and the drawer's Notes tab get it from
 * one place: the drawer is the same element at a different width.
 *
 * The order is applied after the tag filter, so a filtered tree is ordered
 * by the notes it is actually showing — which is the whole reason the
 * daemon sends times for notes and no aggregate for folders.
 */
export function Navigator({ slug, tree: fullTree, current, only = null, query = "" }: Props) {
  const [order, setOrder] = useState<Order>(readOrder);
  const tree = useMemo(
    () => sortTree(only ? filterTree(fullTree, only) : fullTree, order),
    [fullTree, only, order],
  );
  const [expanded, setExpanded] = useState<Set<string>>(() => loadExpanded(slug));

  useEffect(() => {
    setExpanded(loadExpanded(slug));
  }, [slug]);

  useEffect(() => {
    if (!current) return;
    setExpanded((prev) => {
      const next = new Set(prev);
      for (const a of ancestors(current)) next.add(a);
      if (next.size === prev.size) return prev;
      saveExpanded(slug, next);
      return next;
    });
  }, [slug, current]);

  useEffect(() => {
    if (!only) return;
    setExpanded((prev) => {
      const next = new Set(prev);
      for (const p of only) for (const a of ancestors(p)) next.add(a);
      return next.size === prev.size ? prev : next;
    });
  }, [only]);

  const toggle = (path: string) => {
    setExpanded((prev) => {
      const next = new Set(prev);
      if (next.has(path)) next.delete(path);
      else next.add(path);
      saveExpanded(slug, next);
      return next;
    });
  };

  const empty = useMemo(() => !tree.children || tree.children.length === 0, [tree]);

  // Plain list semantics rather than an ARIA tree: nested lists of links
  // and buttons are keyboard-reachable as they are, while a proper tree
  // widget would need roving focus to be an improvement.
  return (
    <nav aria-label="Notes">
      <div class="nav-head">
        <OrderToggle
          order={order}
          onChange={(next) => {
            setOrder(next);
            writeOrder(next);
          }}
        />
      </div>
      {empty ? (
        <p class="muted">{only ? "No notes match the filter." : "No markdown files here."}</p>
      ) : (
        <ul class="tree">
          {tree.children!.map((n) => (
            <Entry key={n.path} node={n} slug={slug} current={current} expanded={expanded} toggle={toggle} query={query} />
          ))}
        </ul>
      )}
    </nav>
  );
}

/**
 * The order control: one toggle, labelled for the order it selects and
 * pressed when that order is on.
 *
 * The accessible name is the visible text and never changes, so a reader
 * who has moved to it is not told the control became a different control
 * when they used it; `aria-pressed` is what states the order in force,
 * which is the same thing the highlight states to a reader who can see it.
 * A name that swapped between "Recent first" and "A to Z" would say what
 * the button does next while the pressed state said what is on now, and
 * the two would be read out together.
 */
function OrderToggle({ order, onChange }: { order: Order; onChange: (next: Order) => void }) {
  const on = order === "recent";
  return (
    <button
      type="button"
      class={on ? "nav-order on" : "nav-order"}
      aria-pressed={on}
      title="Order the notes by when they were last modified, newest first"
      onClick={() => onChange(on ? "name" : "recent")}
    >
      Recent first
    </button>
  );
}

function Entry({
  node,
  slug,
  current,
  expanded,
  toggle,
  query,
}: {
  node: TreeNode;
  slug: string;
  current: string;
  expanded: Set<string>;
  toggle: (path: string) => void;
  query: string;
}) {
  if (node.dir) {
    const open = expanded.has(node.path);
    return (
      <li>
        <button type="button" class="dir" aria-expanded={open} onClick={() => toggle(node.path)}>
          <span class="twisty" aria-hidden="true">
            {open ? "▾" : "▸"}
          </span>
          {node.name}
        </button>
        {open && node.children && (
          <ul>
            {node.children.map((c) => (
              <Entry key={c.path} node={c} slug={slug} current={current} expanded={expanded} toggle={toggle} query={query} />
            ))}
          </ul>
        )}
      </li>
    );
  }
  const active = node.path === current;
  return (
    <li>
      <a href={noteURL(slug, node.path) + query} class={active ? "file active" : "file"} aria-current={active ? "page" : undefined}>
        {node.name}
      </a>
    </li>
  );
}
