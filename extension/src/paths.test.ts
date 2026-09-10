import { describe, expect, it } from "vitest";
import {
  encodePath,
  fileUrlToPath,
  isMarkdownPath,
  isUnder,
  localMarkdownFile,
  noteUrl,
  relativeTo,
  rootContaining,
  splitPath,
  trimSlash,
  type Root,
} from "./paths";

describe("isMarkdownPath", () => {
  it("accepts the two suffixes the daemon serves, in any case", () => {
    expect(isMarkdownPath("/n/a.md")).toBe(true);
    expect(isMarkdownPath("/n/a.markdown")).toBe(true);
    expect(isMarkdownPath("/n/A.MD")).toBe(true);
    expect(isMarkdownPath("/n/a.MarkDown")).toBe(true);
    expect(isMarkdownPath("/n/.hidden.md")).toBe(true);
  });

  it("rejects anything else, and a bare suffix with no name", () => {
    expect(isMarkdownPath("/n/a.txt")).toBe(false);
    expect(isMarkdownPath("/n/a.mdx")).toBe(false);
    expect(isMarkdownPath("/n/readme")).toBe(false);
    expect(isMarkdownPath("/n/.md")).toBe(false);
    expect(isMarkdownPath("/n/.markdown")).toBe(false);
  });
});

describe("fileUrlToPath", () => {
  it("maps a file URL to its absolute Linux path", () => {
    expect(fileUrlToPath("file:///home/you/notes/a.md")).toBe("/home/you/notes/a.md");
  });

  it("decodes percent escapes, including spaces and non-ASCII", () => {
    expect(fileUrlToPath("file:///home/you/my%20notes/caf%C3%A9.md")).toBe(
      "/home/you/my notes/café.md",
    );
    expect(fileUrlToPath("file:///home/you/a%23b.md")).toBe("/home/you/a#b.md");
  });

  it("drops a query and a fragment, which are not part of a file's identity", () => {
    expect(fileUrlToPath("file:///n/a.md?x=1")).toBe("/n/a.md");
    expect(fileUrlToPath("file:///n/a.md#top")).toBe("/n/a.md");
  });

  it("treats an explicit localhost host as local, and refuses any other host", () => {
    expect(fileUrlToPath("file://localhost/n/a.md")).toBe("/n/a.md");
    expect(fileUrlToPath("file://fileserver/share/a.md")).toBeNull();
  });

  it("refuses other schemes, unparseable URLs and broken escapes", () => {
    expect(fileUrlToPath("http://localhost:7337/n/a.md")).toBeNull();
    expect(fileUrlToPath("not a url")).toBeNull();
    expect(fileUrlToPath("file:///n/%ZZ.md")).toBeNull();
  });

  it("refuses a path carrying a NUL", () => {
    expect(fileUrlToPath("file:///n/a%00b.md")).toBeNull();
  });
});

describe("splitPath", () => {
  it("splits a nested path", () => {
    expect(splitPath("/home/you/notes/a.md")).toEqual({ dir: "/home/you/notes", name: "a.md" });
  });

  it("keeps the root directory as /", () => {
    expect(splitPath("/a.md")).toEqual({ dir: "/", name: "a.md" });
  });
});

describe("localMarkdownFile", () => {
  it("describes a local markdown file", () => {
    expect(localMarkdownFile("file:///home/you/notes/deep/a.md")).toEqual({
      path: "/home/you/notes/deep/a.md",
      dir: "/home/you/notes/deep",
      name: "a.md",
    });
  });

  it("returns null for a navigation the extension must leave alone", () => {
    expect(localMarkdownFile("file:///home/you/notes/a.txt")).toBeNull();
    expect(localMarkdownFile("https://example.com/a.md")).toBeNull();
    expect(localMarkdownFile("file:///home/you/notes/")).toBeNull();
  });
});

describe("isUnder", () => {
  it("matches the directory itself and anything beneath it", () => {
    expect(isUnder("/n", "/n")).toBe(true);
    expect(isUnder("/n", "/n/a.md")).toBe(true);
    expect(isUnder("/n", "/n/deep/a.md")).toBe(true);
  });

  it("does not match a sibling that merely shares a prefix", () => {
    expect(isUnder("/n", "/notes/a.md")).toBe(false);
    expect(isUnder("/home/you/notes", "/home/you/notes-old/a.md")).toBe(false);
  });

  it("handles a root directory that already ends in a slash", () => {
    expect(isUnder("/", "/a.md")).toBe(true);
  });
});

describe("relativeTo", () => {
  it("strips the root and its slash", () => {
    expect(relativeTo("/n", "/n/deep/a.md")).toBe("deep/a.md");
    expect(relativeTo("/", "/a.md")).toBe("a.md");
  });
});

const roots: Root[] = [
  { slug: "notes", path: "/home/you/notes", kind: "notes" },
  { slug: "notes-old", path: "/home/you/notes-old", kind: "recent" },
  { slug: "project", path: "/home/you/notes/project", kind: "recent" },
];

describe("rootContaining", () => {
  it("finds the root a file already lives in", () => {
    expect(rootContaining(roots, "/home/you/notes/a.md")?.slug).toBe("notes");
  });

  it("prefers the deepest of two overlapping roots", () => {
    expect(rootContaining(roots, "/home/you/notes/project/a.md")?.slug).toBe("project");
  });

  it("does not mistake a prefix-sharing sibling for a match", () => {
    expect(rootContaining(roots, "/home/you/notes-old/a.md")?.slug).toBe("notes-old");
    expect(rootContaining([roots[0]!], "/home/you/notes-old/a.md")).toBeNull();
  });

  it("returns null when no root contains the file", () => {
    expect(rootContaining(roots, "/tmp/scratch/a.md")).toBeNull();
  });
});

describe("noteUrl", () => {
  it("builds the app URL the UI's own router expects", () => {
    expect(noteUrl("http://localhost:7337", "notes", "deep/a.md")).toBe(
      "http://localhost:7337/r/notes/deep/a.md",
    );
  });

  it("encodes each segment but keeps the slashes", () => {
    expect(encodePath("my notes/café #1.md")).toBe("my%20notes/caf%C3%A9%20%231.md");
    expect(noteUrl("http://localhost:7337", "my notes", "a b.md")).toBe(
      "http://localhost:7337/r/my%20notes/a%20b.md",
    );
  });

  it("tolerates a daemon URL with a trailing slash", () => {
    expect(trimSlash("http://localhost:7337/")).toBe("http://localhost:7337");
    expect(noteUrl("http://localhost:7337/", "notes", "a.md")).toBe(
      "http://localhost:7337/r/notes/a.md",
    );
  });
});
