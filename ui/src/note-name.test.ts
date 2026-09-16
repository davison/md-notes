import { describe, expect, it } from "vitest";
import { folderOf, newNotePath } from "./note-name";

describe("folderOf", () => {
  it("is the directory part, empty at the root", () => {
    expect(folderOf("docs/deep/a.md")).toBe("docs/deep");
    expect(folderOf("a.md")).toBe("");
    expect(folderOf("")).toBe("");
  });
});

describe("newNotePath", () => {
  it("puts a bare title in the selected folder as .md", () => {
    expect(newNotePath("Shopping", "docs")).toEqual({ path: "docs/Shopping.md" });
    expect(newNotePath("Shopping", "")).toEqual({ path: "Shopping.md" });
  });

  it("keeps a markdown extension the person typed, in either spelling", () => {
    expect(newNotePath("notes.md", "docs")).toEqual({ path: "docs/notes.md" });
    expect(newNotePath("notes.MD", "")).toEqual({ path: "notes.MD" });
    expect(newNotePath("notes.markdown", "")).toEqual({ path: "notes.markdown" });
  });

  it("adds .md to a name whose extension is not markdown", () => {
    expect(newNotePath("budget.txt", "")).toEqual({ path: "budget.txt.md" });
    expect(newNotePath("v1.2", "")).toEqual({ path: "v1.2.md" });
  });

  it("treats a name with a slash as a path under the root, ignoring the folder", () => {
    expect(newNotePath("work/log", "personal")).toEqual({ path: "work/log.md" });
    expect(newNotePath("/work/log.md", "personal")).toEqual({ path: "work/log.md" });
    expect(newNotePath("a/b/c", "")).toEqual({ path: "a/b/c.md" });
  });

  it("trims what was typed", () => {
    expect(newNotePath("  Shopping  ", "docs")).toEqual({ path: "docs/Shopping.md" });
  });

  it("refuses a name that names no file", () => {
    expect(newNotePath("", "docs").error).toBeTruthy();
    expect(newNotePath("   ", "docs").error).toBeTruthy();
    expect(newNotePath("docs/", "").error).toBeTruthy();
  });

  it("leaves every other judgment to the daemon", () => {
    // Hidden, and a name that escapes the root: composed and sent, so the
    // refusal the prompt shows is the daemon's own.
    expect(newNotePath(".secret", "")).toEqual({ path: ".secret.md" });
    expect(newNotePath("../outside", "docs")).toEqual({ path: "../outside.md" });
  });
});
