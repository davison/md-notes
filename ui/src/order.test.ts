import { beforeEach, describe, expect, it, vi } from "vitest";
import type { TreeNode } from "./api";
import { DEFAULT_ORDER, ORDER_KEY, readOrder, sortTree, writeOrder } from "./order";

/**
 * Times far enough apart to read at a glance. The alphabetical order and
 * the recency order disagree about every pair here, which is the point:
 * a case that passed under both would prove nothing.
 */
const t = (day: number) => Date.UTC(2026, 8, day);

const file = (name: string, path: string, modified?: number): TreeNode => ({
  name,
  path,
  dir: false,
  ...(modified === undefined ? {} : { modified }),
});

const dir = (name: string, path: string, children: TreeNode[]): TreeNode => ({
  name,
  path,
  dir: true,
  children,
});

/**
 * The daemon's own output shape: directories before files, each group
 * alphabetical, times on files only.
 *
 *   archive/       old.md     1 Sep
 *   docs/          guide.md   9 Sep
 *                  deep/      inner.md  3 Sep
 *   alpha.md                  5 Sep
 *   zeta.md                   7 Sep
 */
const tree: TreeNode = dir("", "", [
  dir("archive", "archive", [file("old.md", "archive/old.md", t(1))]),
  dir("docs", "docs", [
    dir("deep", "docs/deep", [file("inner.md", "docs/deep/inner.md", t(3))]),
    file("guide.md", "docs/guide.md", t(9)),
  ]),
  file("alpha.md", "alpha.md", t(5)),
  file("zeta.md", "zeta.md", t(7)),
]);

/** The names of a node's children, in the order they came out. */
const names = (n: TreeNode) => (n.children ?? []).map((c) => c.name);

const child = (n: TreeNode, name: string) => (n.children ?? []).find((c) => c.name === name)!;

beforeEach(() => {
  localStorage.clear();
});

describe("sortTree", () => {
  it("leaves the alphanumeric order exactly as the daemon sent it", () => {
    expect(sortTree(tree, "name")).toBe(tree);
  });

  it("puts the most recently modified note at the top of its folder", () => {
    const sorted = sortTree(tree, "recent");
    // zeta.md (7 Sep) before alpha.md (5 Sep), which is the reverse of
    // the alphabetical order.
    expect(names(sorted).slice(-2)).toEqual(["zeta.md", "alpha.md"]);
  });

  it("orders folders by the newest note beneath them, at any depth", () => {
    const sorted = sortTree(tree, "recent");
    // docs holds guide.md (9 Sep); archive holds old.md (1 Sep). Nothing
    // in docs' own row says 9 Sep — it is the note two levels down that
    // decides, and the folders are still ahead of the files.
    expect(names(sorted)).toEqual(["docs", "archive", "zeta.md", "alpha.md"]);
  });

  it("keeps folders grouped before notes however new a note is", () => {
    const withNewNote = dir("", "", [
      dir("archive", "archive", [file("old.md", "archive/old.md", t(1))]),
      file("newest.md", "newest.md", t(30)),
    ]);
    expect(names(sortTree(withNewNote, "recent"))).toEqual(["archive", "newest.md"]);
  });

  it("breaks a tie alphabetically, case-insensitively, as the daemon does", () => {
    const same = t(4);
    const tied = dir("", "", [
      file("Zebra.md", "Zebra.md", same),
      file("apple.md", "apple.md", same),
      file("Banana.md", "Banana.md", same),
    ]);
    expect(names(sortTree(tied, "recent"))).toEqual(["apple.md", "Banana.md", "Zebra.md"]);
  });

  it("sorts a note the daemon could not stat last, then alphabetically", () => {
    const unknown = dir("", "", [
      file("b-unreadable.md", "b-unreadable.md"),
      file("a-unreadable.md", "a-unreadable.md"),
      file("dated.md", "dated.md", t(1)),
    ]);
    expect(names(sortTree(unknown, "recent"))).toEqual([
      "dated.md",
      "a-unreadable.md",
      "b-unreadable.md",
    ]);
  });

  it("sorts an empty folder last among folders, not first", () => {
    const withEmpty = dir("", "", [
      dir("empty", "empty", []),
      dir("archive", "archive", [file("old.md", "archive/old.md", t(1))]),
    ]);
    expect(names(sortTree(withEmpty, "recent"))).toEqual(["archive", "empty"]);
  });

  it("sorts every level, not only the top", () => {
    const sorted = sortTree(tree, "recent");
    const docs = child(sorted, "docs");
    // guide.md (9 Sep) is newer than anything in deep/ (3 Sep), and deep/
    // is a folder, so the grouping rule keeps it first anyway.
    expect(names(docs)).toEqual(["deep", "guide.md"]);
  });

  it("does not mutate the tree it was given", () => {
    const before = JSON.stringify(tree);
    sortTree(tree, "recent");
    expect(JSON.stringify(tree)).toBe(before);
  });

  it("leaves a childless node's shape alone", () => {
    const bare: TreeNode = { name: "", path: "", dir: true };
    expect(sortTree(bare, "recent")).toEqual(bare);
  });
});

describe("the stored order", () => {
  it("defaults to the alphanumeric order with nothing stored", () => {
    expect(DEFAULT_ORDER).toBe("name");
    expect(readOrder()).toBe("name");
  });

  it("round-trips through localStorage, per browser rather than per root", () => {
    writeOrder("recent");
    expect(localStorage.getItem(ORDER_KEY)).toBe("recent");
    expect(readOrder()).toBe("recent");
    writeOrder("name");
    expect(readOrder()).toBe("name");
  });

  it("falls back to the default when the stored value is not an order", () => {
    localStorage.setItem(ORDER_KEY, "by-vibes");
    expect(readOrder()).toBe("name");
  });

  it("works where storage throws, reading and writing alike", () => {
    const get = vi.spyOn(Storage.prototype, "getItem").mockImplementation(() => {
      throw new DOMException("denied", "SecurityError");
    });
    const set = vi.spyOn(Storage.prototype, "setItem").mockImplementation(() => {
      throw new DOMException("denied", "SecurityError");
    });
    try {
      expect(readOrder()).toBe("name");
      expect(() => writeOrder("recent")).not.toThrow();
    } finally {
      get.mockRestore();
      set.mockRestore();
    }
  });
});
