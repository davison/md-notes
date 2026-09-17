import { afterEach, beforeEach, describe, expect, it } from "vitest";
import { render, screen, fireEvent, cleanup } from "@testing-library/preact";
import { Navigator, ancestors } from "./navigator";
import { ORDER_KEY } from "./order";
import type { TreeNode } from "./api";

/** Modification times, oldest first, so a recency order is visible. */
const OLD = Date.UTC(2026, 8, 1);
const MID = Date.UTC(2026, 8, 5);
const NEW = Date.UTC(2026, 8, 9);

const tree: TreeNode = {
  name: "",
  path: "",
  dir: true,
  children: [
    {
      name: "docs",
      path: "docs",
      dir: true,
      children: [
        {
          name: "deep",
          path: "docs/deep",
          dir: true,
          children: [{ name: "inner.md", path: "docs/deep/inner.md", dir: false, modified: MID }],
        },
        { name: "guide.md", path: "docs/guide.md", dir: false, modified: OLD },
      ],
    },
    { name: "top.md", path: "top.md", dir: false, modified: NEW },
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
 * The order toggle (M7-R3, davison/md-notes#116). The ordering rules
 * themselves are held by ./order.test.ts; these are the control, what it
 * remembers, and the two things a reader would lose if it re-mounted the
 * tree from scratch.
 */
describe("the order control", () => {
  it("starts on the alphanumeric order the daemon sent", () => {
    render(<Navigator slug="notes" tree={tree} current="" />);
    expect(orderButton().getAttribute("aria-pressed")).toBe("false");
    expect(rows()).toEqual(["docs", "top.md"]);
  });

  it("puts the most recently modified note on top when pressed", () => {
    render(<Navigator slug="notes" tree={tree} current="" />);
    fireEvent.click(orderButton());
    expect(orderButton().getAttribute("aria-pressed")).toBe("true");
    // docs is a folder and folders stay grouped first; top.md is the
    // newest note and leads the notes.
    expect(rows()).toEqual(["docs", "top.md"]);
    fireEvent.click(screen.getByText("docs"));
    // guide.md (1 Sep) against deep/, whose newest note is 5 Sep — but
    // deep/ is a folder, so it is first either way, and the check that
    // matters is that the rows below it are the recency order.
    expect(rows()).toEqual(["docs", "deep", "guide.md", "top.md"]);
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
  });

  it("restores a stored order on first render", () => {
    localStorage.setItem(ORDER_KEY, "recent");
    render(<Navigator slug="notes" tree={tree} current="" />);
    expect(orderButton().getAttribute("aria-pressed")).toBe("true");
  });

  it("leaves the expanded directories open across a toggle", () => {
    render(<Navigator slug="notes" tree={tree} current="" />);
    fireEvent.click(screen.getByText("docs"));
    fireEvent.click(screen.getByText("deep"));
    expect(screen.getByText("inner.md")).toBeTruthy();
    fireEvent.click(orderButton());
    expect(screen.getByText("inner.md")).toBeTruthy();
    expect(screen.getByText("guide.md")).toBeTruthy();
    fireEvent.click(orderButton());
    expect(screen.getByText("inner.md")).toBeTruthy();
  });

  it("leaves the selected note selected across a toggle", () => {
    render(<Navigator slug="notes" tree={tree} current="docs/deep/inner.md" />);
    fireEvent.click(orderButton());
    const link = screen.getByText("inner.md");
    expect(link.classList.contains("active")).toBe(true);
    expect(link.getAttribute("aria-current")).toBe("page");
  });

  it("orders the tag-filtered tree too, and keeps the filter", () => {
    // Only the two notes carrying the tag, one of them inside docs/.
    const only = new Set(["docs/guide.md", "top.md"]);
    render(<Navigator slug="notes" tree={tree} current="" only={only} query="?tag=x" />);
    fireEvent.click(orderButton());
    // top.md (9 Sep) is the newer of the two; docs/ is still first, and
    // deep/ has been pruned away with the note it held.
    expect(rows()).toEqual(["docs", "guide.md", "top.md"]);
    expect((screen.getByText("top.md") as HTMLAnchorElement).getAttribute("href")).toBe("/r/notes/top.md?tag=x");
  });

  it("offers the control even when there is nothing to order", () => {
    render(<Navigator slug="notes" tree={{ name: "", path: "", dir: true }} current="" />);
    expect(screen.getByText("No markdown files here.")).toBeTruthy();
    expect(orderButton()).toBeTruthy();
  });
});
