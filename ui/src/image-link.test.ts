import { describe, expect, it } from "vitest";
import { altFor, imageMarkdown, isImage, resourceLink } from "./image-link";

describe("resourceLink", () => {
  it("climbs from the note's folder to the root", () => {
    expect(resourceLink("top.md", "_resources/a.png")).toBe("_resources/a.png");
    expect(resourceLink("one/n.md", "_resources/a.png")).toBe("../_resources/a.png");
    expect(resourceLink("one/two/n.md", "_resources/a.png")).toBe("../../_resources/a.png");
  });

  it("percent-encodes each segment and keeps the slashes", () => {
    expect(resourceLink("a/n.md", "_resources/a b(1).png")).toBe("../_resources/a%20b%281%29.png");
  });
});

describe("altFor", () => {
  it("is the dropped file's base name without its extension", () => {
    expect(altFor("Holiday snap.PNG")).toBe("Holiday snap");
    expect(altFor("archive.tar.gz")).toBe("archive.tar");
    expect(altFor("noext")).toBe("noext");
    expect(altFor(".png")).toBe(".png");
  });

  it("escapes what would end the alt text early", () => {
    expect(altFor("a [b] c\\d.png")).toBe("a \\[b\\] c\\\\d");
  });

  it("keeps a name on one line", () => {
    expect(altFor("one\ntwo.png")).toBe("one two");
  });
});

describe("imageMarkdown", () => {
  it("writes a markdown image", () => {
    expect(imageMarkdown("", "_resources/a.png")).toBe("![](_resources/a.png)");
    expect(imageMarkdown("snap", "../_resources/a.png")).toBe("![snap](../_resources/a.png)");
  });
});

describe("isImage", () => {
  it("goes by the type the browser gives the file", () => {
    expect(isImage(new File(["x"], "a.png", { type: "image/png" }))).toBe(true);
    expect(isImage(new File(["x"], "a.png", { type: "" }))).toBe(false);
    expect(isImage(new File(["x"], "a.pdf", { type: "application/pdf" }))).toBe(false);
  });
});
