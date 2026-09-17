import type { TreeNode } from "./api";

/**
 * How the navigator arranges the tree.
 *
 * "name" is the alphanumeric order the daemon already sends — directories
 * before files, each group case-insensitive — and is the default.
 * "recent" is the same grouping with the most recently modified note at
 * the top: notes by their modification time within a folder, folders by
 * the newest note anywhere beneath them, every tie broken alphabetically
 * by the same rule the daemon uses (davison/md-notes#116, M7-R3).
 *
 * The comparators are written out here rather than pulled from a sorting
 * library: the whole of it is two comparisons and a post-order walk, and
 * the reading page's eager bundle is not the place to spend a dependency.
 */
export type Order = "name" | "recent";

export const DEFAULT_ORDER: Order = "name";

/**
 * Where the choice is kept. One key for the browser rather than one per
 * root: M7-R3 asks for the choice to persist per browser, and a reader who
 * wants their notes by recency wants that of every root they open, not of
 * the one they happened to set it in. Expanded directories are the
 * opposite case and stay keyed by root, in ./navigator.
 */
export const ORDER_KEY = "mdn:nav:order";

function isOrder(value: unknown): value is Order {
  return value === "name" || value === "recent";
}

/** The stored order, or the default when there is none or it is unusable. */
export function readOrder(): Order {
  try {
    const raw = localStorage.getItem(ORDER_KEY);
    if (isOrder(raw)) return raw;
  } catch {
    // Storage unavailable or refused: the default is a working navigator,
    // so there is nothing to report.
  }
  return DEFAULT_ORDER;
}

/** Stores the order. A browser that refuses storage keeps it for the page. */
export function writeOrder(order: Order): void {
  try {
    localStorage.setItem(ORDER_KEY, order);
  } catch {
    // Storage unavailable: the choice holds until the page is reloaded.
  }
}

/** The daemon's own tie-break: case-insensitive, by name. */
function byName(a: TreeNode, b: TreeNode): number {
  const x = a.name.toLowerCase();
  const y = b.name.toLowerCase();
  return x < y ? -1 : x > y ? 1 : 0;
}

/**
 * One post-order pass: each node is returned with its children sorted and
 * with the newest modification time at or beneath it, so a folder is
 * ranked by its whole subtree without walking that subtree again for every
 * comparison.
 *
 * A note the daemon could not stat carries no `modified` and counts as 0,
 * which puts it last among its siblings and its folder last among folders
 * holding nothing newer — the bottom of the list rather than the top,
 * because "we do not know when this changed" is not a claim that it
 * changed just now.
 */
function arrange(node: TreeNode): [TreeNode, number] {
  if (!node.dir || !node.children) return [node, node.modified ?? 0];
  const seen = node.children.map(arrange);
  let newest = 0;
  for (const [, when] of seen) if (when > newest) newest = when;
  const children = seen
    .sort(([a, aWhen], [b, bWhen]) => {
      // Folders stay grouped before notes, which is the rule the
      // alphanumeric order has always followed and M7-R3 keeps.
      if (a.dir !== b.dir) return a.dir ? -1 : 1;
      if (aWhen !== bWhen) return bWhen - aWhen;
      return byName(a, b);
    })
    .map(([child]) => child);
  return [{ ...node, children }, newest];
}

/**
 * The tree in the given order. The alphanumeric order is the tree itself,
 * unchanged and not copied: it is what the daemon sent, and re-sorting it
 * could only find a different answer than the daemon's by being wrong.
 *
 * The recency order is a copy. The tree is state the root view holds and
 * hands to other panes, and a sort in place would reorder it under them.
 */
export function sortTree(tree: TreeNode, order: Order): TreeNode {
  if (order === "name") return tree;
  return arrange(tree)[0];
}
