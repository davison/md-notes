import { afterEach, describe, expect, it } from "vitest";
import { DEFAULTS } from "./settings";
import { DIAGRAM_CLASS, DiagramPool, SOURCE_HIDDEN_CLASS, decorate, diagramTheme } from "./diagrams";

/** The markup the daemon renders for a fenced mermaid block, anchor first. */
function block(line: number, source: string): string {
  return `<div class="line-anchor" data-line="${line}"></div>\n<pre><code class="language-mermaid">${source}</code></pre>\n`;
}

function note(html: string): HTMLDivElement {
  const div = document.createElement("div");
  div.className = "markdown";
  div.innerHTML = html;
  document.body.append(div);
  return div;
}

const url = (theme: string) => (hash: string) => `/api/r/n/diagram/x.md?h=${hash}&theme=${theme}`;

afterEach(() => {
  document.body.innerHTML = "";
});

describe("diagramTheme", () => {
  it("follows the device's scheme, and the light override is the e-ink palette", () => {
    expect(diagramTheme(DEFAULTS, false)).toBe("light");
    expect(diagramTheme(DEFAULTS, true)).toBe("dark");
    expect(diagramTheme({ ...DEFAULTS, light: true }, true)).toBe("eink");
    expect(diagramTheme({ ...DEFAULTS, light: true }, false)).toBe("eink");
  });
});

describe("decorate", () => {
  it("puts the image between the anchor and its code block, and hides the code", () => {
    const scope = note("<p data-line=\"1\">p</p>\n" + block(3, "graph TD; A--&gt;B\n") + block(7, "graph LR; C--&gt;D\n"));
    const pool = new DiagramPool();
    decorate(scope, [{ line: 7, hash: "h7" }], url("light"), pool);

    const img = scope.querySelector("img")!;
    expect(img.className).toBe(DIAGRAM_CLASS);
    expect(img.getAttribute("src")).toBe("/api/r/n/diagram/x.md?h=h7&theme=light");
    expect(img.previousElementSibling!.getAttribute("data-line")).toBe("7");
    expect(img.nextElementSibling!.tagName).toBe("PRE");
    expect(img.nextElementSibling!.classList.contains(SOURCE_HIDDEN_CLASS)).toBe(true);
    // The source is the image's text alternative, as the author wrote it.
    expect(img.alt).toBe("graph LR; C-->D\n");
    // The block not listed is left exactly as it was.
    const other = scope.querySelector('[data-line="3"]')!.nextElementSibling!;
    expect(other.tagName).toBe("PRE");
    expect(other.classList.contains(SOURCE_HIDDEN_CLASS)).toBe(false);
  });

  it("reserves the drawing's box from the size the daemon measured", () => {
    const scope = note(block(3, "s") + block(8, "t"));
    decorate(
      scope,
      [
        { line: 3, hash: "a", width: 185, height: 94.6 },
        { line: 8, hash: "b" },
      ],
      url("light"),
      new DiagramPool(),
    );
    const [sized, unsized] = [...scope.querySelectorAll("img")];
    expect([sized.getAttribute("width"), sized.getAttribute("height")]).toEqual(["185", "94.6"]);
    expect(unsized.hasAttribute("width") || unsized.hasAttribute("height")).toBe(false);
  });

  it("brings the code block back and drops the image when it cannot load", () => {
    const scope = note(block(3, "graph TD; A--&gt;B\n"));
    const pool = new DiagramPool();
    decorate(scope, [{ line: 3, hash: "h" }], url("light"), pool);
    const img = scope.querySelector("img")!;
    img.dispatchEvent(new Event("error"));

    expect(scope.querySelector("img")).toBeNull();
    expect(scope.querySelector("pre")!.classList.contains(SOURCE_HIDDEN_CLASS)).toBe(false);
    expect(pool.size).toBe(0);
    // A later pass — a live update, a theme change — tries again.
    decorate(scope, [{ line: 3, hash: "h" }], url("dark"), pool);
    const again = scope.querySelector("img")!;
    expect(again).not.toBe(img);
    expect(again.getAttribute("src")).toContain("theme=dark");
  });

  it("leaves a block whose drawing failed as code until its source or the theme changes", () => {
    // The round-one review of PR #175, N3: without this, every live update
    // or theme change hid the code block again until the cached refusal
    // came back, and the page jumped.
    const scope = note(block(3, "refused") + block(8, "fine"));
    const pool = new DiagramPool();
    const both = [
      { line: 3, hash: "r" },
      { line: 8, hash: "f" },
    ];
    decorate(scope, both, url("light"), pool);
    scope.querySelector("img")!.dispatchEvent(new Event("error"));
    const refusedPre = scope.querySelector('[data-line="3"]')!.nextElementSibling!;
    expect(refusedPre.tagName).toBe("PRE");

    // A live update over the same source, in the same theme.
    let hidden = 0;
    new MutationObserver(() => {
      if (refusedPre.classList.contains(SOURCE_HIDDEN_CLASS)) hidden++;
    }).observe(refusedPre, { attributes: true, attributeFilter: ["class"] });
    decorate(scope, both, url("light"), pool);
    expect(scope.querySelectorAll("img").length).toBe(1);
    expect(refusedPre.classList.contains(SOURCE_HIDDEN_CLASS)).toBe(false);

    // A changed source is a new hash, and is tried.
    decorate(scope, [{ line: 3, hash: "r2" }, both[1]], url("light"), pool);
    expect(scope.querySelectorAll("img").length).toBe(2);
    return Promise.resolve().then(() => expect(hidden).toBe(1));
  });

  it("brings the code back when the theme returns to one whose drawing failed", () => {
    const scope = note(block(3, "s"));
    const pool = new DiagramPool();
    const one = [{ line: 3, hash: "h" }];
    decorate(scope, one, url("light"), pool);
    scope.querySelector("img")!.dispatchEvent(new Event("error"));
    decorate(scope, one, url("dark"), pool);
    expect(scope.querySelector("img")!.getAttribute("src")).toContain("theme=dark");
    decorate(scope, one, url("light"), pool);
    expect(scope.querySelector("img")).toBeNull();
    expect(scope.querySelector("pre")!.classList.contains(SOURCE_HIDDEN_CLASS)).toBe(false);
  });

  it("skips an entry whose anchor is missing or is not followed by a code block", () => {
    const scope = note('<div class="line-anchor" data-line="2"></div><p>not code</p>' + block(9, "x"));
    decorate(
      scope,
      [
        { line: 2, hash: "a" },
        { line: 5, hash: "b" },
      ],
      url("light"),
      new DiagramPool(),
    );
    expect(scope.querySelector("img")).toBeNull();
    expect(scope.querySelector("pre")!.classList.contains(SOURCE_HIDDEN_CLASS)).toBe(false);
  });

  it("finds only the renderer's anchor, not an element that merely carries the line", () => {
    // The daemon strips both of these from note HTML; the look-up does not
    // rely on that alone: only an anchor div is an anchor.
    const scope = note('<p data-line="4">text</p><pre>decoy</pre>' + block(4, "real"));
    decorate(scope, [{ line: 4, hash: "h" }], url("light"), new DiagramPool());
    const img = scope.querySelector("img")!;
    expect(img.alt).toBe("real");
    expect(scope.querySelector("pre")!.textContent).toBe("decoy");
    expect(scope.querySelector("pre")!.classList.contains(SOURCE_HIDDEN_CLASS)).toBe(false);
  });

  it("changes only the src when the theme changes", () => {
    const scope = note(block(3, "s"));
    const pool = new DiagramPool();
    decorate(scope, [{ line: 3, hash: "h" }], url("light"), pool);
    const img = scope.querySelector("img")!;
    decorate(scope, [{ line: 3, hash: "h" }], url("eink"), pool);
    expect(scope.querySelectorAll("img").length).toBe(1);
    expect(scope.querySelector("img")).toBe(img);
    expect(img.getAttribute("src")).toContain("theme=eink");
  });

  it("reuses the element for an unchanged diagram when the note is rendered again", () => {
    const scope = note(block(3, "one") + block(8, "two"));
    const pool = new DiagramPool();
    decorate(
      scope,
      [
        { line: 3, hash: "one" },
        { line: 8, hash: "two" },
      ],
      url("light"),
      pool,
    );
    const [one, two] = [...scope.querySelectorAll("img")];
    let sets = 0;
    const observe = (img: HTMLImageElement) =>
      new MutationObserver((records) => (sets += records.length)).observe(img, { attributes: true, attributeFilter: ["src"] });
    observe(one);
    observe(two);

    // A live update: new HTML, a paragraph added above, the second diagram
    // edited. The first keeps its element and its src; the second is new.
    scope.innerHTML = '<p data-line="1">added</p>\n' + block(5, "one") + block(10, "two, edited");
    decorate(
      scope,
      [
        { line: 5, hash: "one" },
        { line: 10, hash: "two-edited" },
      ],
      url("light"),
      pool,
    );
    const now = [...scope.querySelectorAll("img")];
    expect(now.length).toBe(2);
    expect(now[0]).toBe(one);
    expect(now[0].previousElementSibling!.getAttribute("data-line")).toBe("5");
    expect(now[1]).not.toBe(two);
    expect(now[1].getAttribute("src")).toContain("h=two-edited");
    expect(pool.size).toBe(2);
    expect([...pool.values()]).not.toContain(two);
    return Promise.resolve().then(() => expect(sets).toBe(0));
  });

  it("tells two identical diagrams apart by their order", () => {
    const scope = note(block(3, "same") + block(8, "same"));
    const pool = new DiagramPool();
    const same = [
      { line: 3, hash: "s" },
      { line: 8, hash: "s" },
    ];
    decorate(scope, same, url("light"), pool);
    const imgs = [...scope.querySelectorAll("img")];
    expect(imgs.length).toBe(2);
    decorate(scope, same, url("light"), pool);
    expect([...scope.querySelectorAll("img")]).toEqual(imgs);
  });

  it("is idempotent over the same markup", () => {
    const scope = note(block(3, "s"));
    const pool = new DiagramPool();
    decorate(scope, [{ line: 3, hash: "h" }], url("light"), pool);
    const before = scope.innerHTML;
    decorate(scope, [{ line: 3, hash: "h" }], url("light"), pool);
    expect(scope.innerHTML).toBe(before);
  });
});
