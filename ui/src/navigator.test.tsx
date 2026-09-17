import { afterEach, beforeEach, describe, expect, it } from "vitest";
import { render, screen, fireEvent, cleanup } from "@testing-library/preact";
import { Navigator, ancestors } from "./navigator";
import { ORDER_KEY } from "./order";
import type { TreeNode } from "./api";

/** A day in September 2026, as the daemon would send it. */
const on = (day: number) => Date.UTC(2026, 8, day);

/**
 * The fixture, chosen so that the alphanumeric order and the recency order
 * are a different list at *every* level — the root's folders, the root's
 * notes, inside `docs`, inside `docs/deep`, and under the tag filter the
 * cases below apply. A fixture whose two orders coincide cannot fail when
 * the ordering is removed, whatever a case is named
 * (davison/md-notes#119, review finding 1).
 *
 *                             modified   alphanumeric   by recency
 *   archive/  old.md           3 Sep      1st folder     2nd folder
 *   docs/     deep/  beta.md   5 Sep      1st in deep    2nd in deep
 *                   inner.md   7 Sep      2nd in deep    1st in deep
 *             guide.md         1 Sep      1st note       2nd note
 *             notes.md         9 Sep      2nd note       1st note
 *   alpha.md                   2 Sep      1st note       2nd note
 *   top.md                     8 Sep      2nd note       1st note
 *
 * `docs` is ahead of `archive` by recency because `notes.md` (9 Sep) lies
 * beneath it, which is the folders-by-newest-descendant rule; both stay
 * ahead of the notes, which is the grouping rule.
 */
const tree: TreeNode = {
  name: "",
  path: "",
  dir: true,
  children: [
    {
      name: "archive",
      path: "archive",
      dir: true,
      children: [{ name: "old.md", path: "archive/old.md", dir: false, modified: on(3) }],
    },
    {
      name: "docs",
      path: "docs",
      dir: true,
      children: [
        {
          name: "deep",
          path: "docs/deep",
          dir: true,
          children: [
            { name: "beta.md", path: "docs/deep/beta.md", dir: false, modified: on(5) },
            { name: "inner.md", path: "docs/deep/inner.md", dir: false, modified: on(7) },
          ],
        },
        { name: "guide.md", path: "docs/guide.md", dir: false, modified: on(1) },
        { name: "notes.md", path: "docs/notes.md", dir: false, modified: on(9) },
      ],
    },
    { name: "alpha.md", path: "alpha.md", dir: false, modified: on(2) },
    { name: "top.md", path: "top.md", dir: false, modified: on(8) },
  ],
};

/** The order control, by the accessible name it keeps whichever order is on. */
const orderButton = () => screen.getByRole("button", { name: "Recent first" });

/** The note and directory rows on screen, in the order they are rendered. */
const rows = () => [...document.querySelectorAll(".tree .dir, .tree .file")].map((e) => e.textContent!.replace(/^[▾▸]/, ""));

beforeEach(() => {
  localStorage.clear();
});

/**
 * The tree the case rendered goes away with the case, the same rule
 * note-view.test.tsx states at length (davison/md-notes#101): cleaning up in
 * `beforeEach` alone leaves the last case's tree mounted for the rest of the
 * file's life, and this project runs vitest without the test globals that
 * would have `@testing-library/preact` install its own `afterEach(cleanup)`.
 * `Navigator` has no fetch and no effect that reaches a global, so nothing
 * here was ever going to outlive the environment the way #87 did — this is
 * the same hygiene, not a second fault.
 */
afterEach(cleanup);

describe("ancestors", () => {
  it("lists every directory above a path", () => {
    expect(ancestors("a/b/c.md")).toEqual(["a", "a/b"]);
    expect(ancestors("top.md")).toEqual([]);
  });
});

describe("Navigator", () => {
  it("renders files and collapsed directories", () => {
    render(<Navigator slug="notes" tree={tree} current="" />);
    expect(screen.getByText("top.md")).toBeTruthy();
    expect(screen.getByText("docs")).toBeTruthy();
    expect(screen.queryByText("guide.md")).toBeNull();
  });

  it("expands and collapses a directory, remembering the state", () => {
    render(<Navigator slug="notes" tree={tree} current="" />);
    fireEvent.click(screen.getByText("docs"));
    expect(screen.getByText("guide.md")).toBeTruthy();
    expect(JSON.parse(localStorage.getItem("mdn:nav:notes")!)).toEqual(["docs"]);
    fireEvent.click(screen.getByText("docs"));
    expect(screen.queryByText("guide.md")).toBeNull();
  });

  it("links files to their in-app URL", () => {
    render(<Navigator slug="my notes" tree={tree} current="" />);
    const link = screen.getByText("top.md") as HTMLAnchorElement;
    expect(link.getAttribute("href")).toBe("/r/my%20notes/top.md");
  });

  it("keeps the query string on note links", () => {
    render(<Navigator slug="notes" tree={tree} current="" query="?tag=x" />);
    expect((screen.getByText("top.md") as HTMLAnchorElement).getAttribute("href")).toBe("/r/notes/top.md?tag=x");
  });

  it("reveals and highlights the current note", () => {
    render(<Navigator slug="notes" tree={tree} current="docs/deep/inner.md" />);
    const link = screen.getByText("inner.md");
    expect(link.classList.contains("active")).toBe(true);
    expect(link.getAttribute("aria-current")).toBe("page");
    expect(screen.getByText("guide.md")).toBeTruthy();
  });

  it("restores expansion from storage per root", () => {
    localStorage.setItem("mdn:nav:notes", JSON.stringify(["docs"]));
    render(<Navigator slug="notes" tree={tree} current="" />);
    expect(screen.getByText("guide.md")).toBeTruthy();
    cleanup();
    render(<Navigator slug="other" tree={tree} current="" />);
    expect(screen.queryByText("guide.md")).toBeNull();
  });

  it("says so when a root has no markdown", () => {
    render(<Navigator slug="notes" tree={{ name: "", path: "", dir: true }} current="" />);
    expect(screen.getByText("No markdown files here.")).toBeTruthy();
  });
});

/**
 * The order toggle (M7-R3, davison/md-notes#116). ./order.test.ts holds
 * the ordering rules on their own terms; these hold the control, what it
 * remembers, and the two things a reader would lose if it re-mounted the
 * tree from scratch — but they hold the order too, and they have to: a
 * fixture whose two orders coincide leaves cases that pass with the
 * ordering taken out (davison/md-notes#119, review finding 1). Every
 * expected list below therefore differs from the other order at every
 * level it names.
 */
describe("the order control", () => {
  /** The whole tree open, which is where the two orders differ the most. */
  const openAll = () => {
    fireEvent.click(screen.getByText("archive"));
    fireEvent.click(screen.getByText("docs"));
    fireEvent.click(screen.getByText("deep"));
  };

  const ALPHANUMERIC = [
    "archive",
    "old.md",
    "docs",
    "deep",
    "beta.md",
    "inner.md",
    "guide.md",
    "notes.md",
    "alpha.md",
    "top.md",
  ];

  const BY_RECENCY = [
    "docs",
    "deep",
    "inner.md",
    "beta.md",
    "notes.md",
    "guide.md",
    "archive",
    "old.md",
    "top.md",
    "alpha.md",
  ];

  it("starts on the alphanumeric order the daemon sent", () => {
    render(<Navigator slug="notes" tree={tree} current="" />);
    expect(orderButton().getAttribute("aria-pressed")).toBe("false");
    openAll();
    expect(rows()).toEqual(ALPHANUMERIC);
  });

  it("orders every level by recency when pressed", () => {
    render(<Navigator slug="notes" tree={tree} current="" />);
    openAll();
    fireEvent.click(orderButton());
    expect(orderButton().getAttribute("aria-pressed")).toBe("true");
    // Folders before notes at both levels; docs ahead of archive on a
    // note two levels down; inner.md ahead of beta.md inside deep.
    expect(rows()).toEqual(BY_RECENCY);
    fireEvent.click(orderButton());
    expect(rows()).toEqual(ALPHANUMERIC);
  });

  it("keeps its accessible name whichever order is on", () => {
    render(<Navigator slug="notes" tree={tree} current="" />);
    const before = orderButton().textContent;
    fireEvent.click(orderButton());
    // getByRole would have thrown above if the name had changed; this
    // says the visible text is the name, which is what WCAG's
    // label-in-name asks of a control whose state a reader can see.
    expect(orderButton().textContent).toBe(before);
  });

  it("remembers the order for the browser, not for one root", () => {
    render(<Navigator slug="notes" tree={tree} current="" />);
    fireEvent.click(orderButton());
    expect(localStorage.getItem(ORDER_KEY)).toBe("recent");
    cleanup();
    render(<Navigator slug="another-root" tree={tree} current="" />);
    expect(orderButton().getAttribute("aria-pressed")).toBe("true");
    openAll();
    expect(rows()).toEqual(BY_RECENCY);
  });

  it("restores a stored order on first render", () => {
    localStorage.setItem(ORDER_KEY, "recent");
    render(<Navigator slug="notes" tree={tree} current="" />);
    expect(orderButton().getAttribute("aria-pressed")).toBe("true");
    openAll();
    expect(rows()).toEqual(BY_RECENCY);
  });

  it("leaves the expanded directories open across a toggle", () => {
    render(<Navigator slug="notes" tree={tree} current="" />);
    openAll();
    fireEvent.click(orderButton());
    expect(rows()).toEqual(BY_RECENCY);
    fireEvent.click(orderButton());
    expect(rows()).toEqual(ALPHANUMERIC);
  });

  it("leaves the selected note selected across a toggle", () => {
    render(<Navigator slug="notes" tree={tree} current="docs/deep/inner.md" />);
    fireEvent.click(orderButton());
    const link = screen.getByText("inner.md");
    expect(link.classList.contains("active")).toBe(true);
    expect(link.getAttribute("aria-current")).toBe("page");
    // The reveal put docs and deep open, and the order applies inside them.
    expect(rows()).toEqual(["docs", "deep", "inner.md", "beta.md", "notes.md", "guide.md", "archive", "top.md", "alpha.md"]);
  });

  it("orders the tag-filtered tree by the notes it is showing", () => {
    // The tag is on the two oldest notes in the two folders, and on both
    // top-level notes. docs is left showing guide.md (1 Sep) and archive
    // old.md (3 Sep), so the filtered recency order puts archive first —
    // the reverse of the unfiltered recency order, where docs leads on a
    // note the filter has pruned away. That is the whole reason the
    // daemon sends no folder aggregate.
    const only = new Set(["archive/old.md", "docs/guide.md", "alpha.md", "top.md"]);
    render(<Navigator slug="notes" tree={tree} current="" only={only} query="?tag=x" />);

    // The filter opens the folders holding a match; deep/ was pruned with
    // the notes it held.
    expect(rows()).toEqual(["archive", "old.md", "docs", "guide.md", "alpha.md", "top.md"]);

    fireEvent.click(orderButton());
    expect(rows()).toEqual(["archive", "old.md", "docs", "guide.md", "top.md", "alpha.md"]);
    expect((screen.getByText("top.md") as HTMLAnchorElement).getAttribute("href")).toBe("/r/notes/top.md?tag=x");
  });

  it("offers the control even when there is nothing to order", () => {
    render(<Navigator slug="notes" tree={{ name: "", path: "", dir: true }} current="" />);
    expect(screen.getByText("No markdown files here.")).toBeTruthy();
    expect(orderButton()).toBeTruthy();
  });
});
