import { beforeEach, describe, expect, it } from "vitest";
import { render, screen, fireEvent, cleanup } from "@testing-library/preact";
import { Navigator, ancestors } from "./navigator";
import type { TreeNode } from "./api";

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
          children: [{ name: "inner.md", path: "docs/deep/inner.md", dir: false }],
        },
        { name: "guide.md", path: "docs/guide.md", dir: false },
      ],
    },
    { name: "top.md", path: "top.md", dir: false },
  ],
};

beforeEach(() => {
  cleanup();
  localStorage.clear();
});

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
