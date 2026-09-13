import { afterEach, describe, expect, it } from "vitest";
import { cleanup, render, screen } from "@testing-library/preact";
import { TagPanel } from "./tag-panel";
import { filterTree } from "./navigator";
import { lineTarget } from "./note-view";

afterEach(cleanup);

describe("TagPanel", () => {
  const tags = [
    { name: "go", count: 3, notes: ["a.md"] },
    { name: "x y", count: 1, notes: ["b.md"] },
  ];

  it("lists tags with counts and links that set or clear the filter", () => {
    render(<TagPanel slug="n" tags={tags} active="go" current="docs/a b.md" />);
    const go = screen.getByText("#go").closest("a")!;
    expect(go.classList.contains("active")).toBe(true);
    expect(go.getAttribute("href")).toBe("/r/n/docs/a%20b.md");
    const xy = screen.getByText("#x y").closest("a")!;
    expect(xy.getAttribute("href")).toBe("/r/n/docs/a%20b.md?tag=x%20y");
    expect(screen.getByText("3")).toBeTruthy();
    expect(screen.getByText("Clear filter: go").getAttribute("href")).toBe("/r/n/docs/a%20b.md");
  });

  it("handles loading and empty states", () => {
    render(<TagPanel slug="n" tags={null} active={null} current="" />);
    expect(screen.getByText("Loading tags…")).toBeTruthy();
    cleanup();
    render(<TagPanel slug="n" tags={[]} active={null} current="" />);
    expect(screen.getByText("No tags.")).toBeTruthy();
  });
});

describe("filterTree", () => {
  it("keeps only the given files and prunes empty directories", () => {
    const tree = {
      name: "",
      path: "",
      dir: true,
      children: [
        {
          name: "d",
          path: "d",
          dir: true,
          children: [
            { name: "keep.md", path: "d/keep.md", dir: false },
            { name: "drop.md", path: "d/drop.md", dir: false },
            { name: "e", path: "d/e", dir: true, children: [{ name: "n.md", path: "d/e/n.md", dir: false }] },
          ],
        },
        { name: "top.md", path: "top.md", dir: false },
      ],
    };
    const out = filterTree(tree, new Set(["d/keep.md"]));
    expect(out.children!.map((c) => c.path)).toEqual(["d"]);
    expect(out.children![0].children!.map((c) => c.path)).toEqual(["d/keep.md"]);
  });
});

describe("lineTarget", () => {
  it("picks the block starting at or nearest before the line", () => {
    document.body.innerHTML =
      '<div id="s"><p data-line="1">a</p><h2 data-line="5">b</h2><p data-line="9">c</p><p data-line="x">d</p></div>';
    const scope = document.getElementById("s");
    expect(lineTarget(scope, 5)?.textContent).toBe("b");
    expect(lineTarget(scope, 7)?.textContent).toBe("b");
    expect(lineTarget(scope, 100)?.textContent).toBe("c");
    expect(lineTarget(scope, 0)).toBeNull();
    document.body.innerHTML = "";
  });
});
