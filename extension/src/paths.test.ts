import { describe, expect, it } from "vitest";
import {
  encodePath,
  fileUrlToPath,
  normalisePath,
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

describe("fileUrlToPath as a security boundary", () => {
  // The path this returns is matched against the daemon's roots, and its
  // directory is what `POST /api/roots` is asked to register. A percent-
  // encoded separator that decoded into `../` would let a crafted file URL
  // escape the matched root, or register somewhere like /etc.
  it("refuses an encoded separator that would decode into a traversal", () => {
    expect(fileUrlToPath("file:///tmp/probe/notes%2F..%2Fsecretdir/x.md")).toBeNull();
    expect(
      fileUrlToPath(
        "file:///tmp/probe/nowhere%2F..%2F..%2F..%2F..%2F..%2F..%2Fetc/hosts.md",
      ),
    ).toBeNull();
    expect(fileUrlToPath("file:///home/you/notes%2F..%2F..%2F..%2Fetc/hosts.md")).toBeNull();
  });

  it("refuses an encoded separator even where it decodes to something harmless", () => {
    expect(fileUrlToPath("file:///home/you/a%2Fb.md")).toBeNull();
    expect(fileUrlToPath("file:///home/you/a%2fb.md")).toBeNull();
  });

  it("refuses an encoded dot segment that carries a separator with it", () => {
    expect(fileUrlToPath("file:///home/you/notes/%2E%2E%2Fetc/a.md")).toBeNull();
    expect(fileUrlToPath("file:///home/you/%2e%2e%2f%2e%2e%2fetc/a.md")).toBeNull();
  });

  it("accepts an encoded dot segment the URL parser has already resolved", () => {
    // `%2e%2e` on its own is a dot segment to the URL parser, which resolves
    // it before the extension sees the pathname — the resulting path is the
    // real one, and the browser is showing that same file.
    expect(fileUrlToPath("file:///home/you/notes/%2e%2e/a.md")).toBe("/home/you/a.md");
    expect(fileUrlToPath("file:///home/you/notes/%2E/a.md")).toBe("/home/you/notes/a.md");
  });

  it("refuses an encoded backslash, which is a separator on the platforms this is not", () => {
    expect(fileUrlToPath("file:///home/you/a%5Cb.md")).toBeNull();
  });

  it("refuses an empty segment: a // run, or a trailing slash", () => {
    expect(fileUrlToPath("file:///home/you//a.md")).toBeNull();
    expect(fileUrlToPath("file:///home/you/notes/")).toBeNull();
    expect(fileUrlToPath("file:///")).toBeNull();
  });

  it("still accepts a literal dot segment, which the URL parser has already resolved", () => {
    // The browser is showing the resolved file, so this is its true path.
    expect(fileUrlToPath("file:///home/you/notes/../a.md")).toBe("/home/you/a.md");
    expect(fileUrlToPath("file:///home/you/./a.md")).toBe("/home/you/a.md");
  });

  it("returns a path that is already in normal form", () => {
    for (const url of [
      "file:///home/you/notes/a.md",
      "file:///home/you/my%20notes/caf%C3%A9.md",
      "file:///home/you/notes/../a.md",
    ]) {
      const path = fileUrlToPath(url);
      expect(path).not.toBeNull();
      expect(path).toBe(normalisePath(path!));
    }
  });

  it("keeps a crafted URL out of the open-file path entirely", () => {
    expect(localMarkdownFile("file:///tmp/probe/notes%2F..%2Fsecretdir/x.md")).toBeNull();
  });
});

describe("normalisePath", () => {
  it("resolves dot segments and empty ones", () => {
    expect(normalisePath("/a/b/../c.md")).toBe("/a/c.md");
    expect(normalisePath("/a//b/./c.md")).toBe("/a/b/c.md");
    expect(normalisePath("/a/../../../etc")).toBe("/etc");
    expect(normalisePath("/")).toBe("/");
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
