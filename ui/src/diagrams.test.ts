import { afterEach, describe, expect, it, vi } from "vitest";
import { DEFAULTS } from "./settings";
import {
  BOX_CLASS,
  DIAGRAM_CLASS,
  DiagramPool,
  OPEN_CLASS,
  SOURCE_HIDDEN_CLASS,
  WIDTH_PROPERTY,
  decorate,
  diagramKeys,
  diagramTheme,
  followViewed,
  holdInView,
} from "./diagrams";

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
    // The image is the button in a scroll box of its own, and the box stands
    // between the anchor and the code block (#187).
    const button = img.parentElement!;
    const box = button.parentElement!;
    expect(button.tagName).toBe("BUTTON");
    expect(button.className).toBe(OPEN_CLASS);
    expect(button.getAttribute("type")).toBe("button");
    // The action is its name, and the source its description (review of PR #202, nit 3).
    expect(button.getAttribute("aria-label")).toBe("Open diagram at natural size");
    expect(button.getAttribute("aria-describedby")).toBe(img.id);
    expect(img.id).toMatch(/^diagram:\d+$/);
    expect(box.className).toBe(BOX_CLASS);
    expect(box.previousElementSibling!.getAttribute("data-line")).toBe("7");
    expect(box.nextElementSibling!.tagName).toBe("PRE");
    expect(box.nextElementSibling!.classList.contains(SOURCE_HIDDEN_CLASS)).toBe(true);
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
    // The natural width goes to the stylesheet through the CSSOM, which
    // sizes the image against the floor (#187); no style attribute is
    // written, which the page's policy would refuse.
    expect(sized.style.getPropertyValue(WIDTH_PROPERTY)).toBe("185px");
    expect(unsized.hasAttribute("width") || unsized.hasAttribute("height")).toBe(false);
    expect(unsized.style.getPropertyValue(WIDTH_PROPERTY)).toBe("");
  });

  it("gives an unmeasured image its natural width once it loads", () => {
    const scope = note(block(3, "s"));
    decorate(scope, [{ line: 3, hash: "b" }], url("light"), new DiagramPool());
    const img = scope.querySelector("img")!;
    Object.defineProperty(img, "naturalWidth", { value: 1234 });
    img.dispatchEvent(new Event("load"));
    expect(img.style.getPropertyValue(WIDTH_PROPERTY)).toBe("1234px");
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
    expect(now[0].parentElement!.parentElement!.previousElementSibling!.getAttribute("data-line")).toBe("5");
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

describe("followViewed", () => {
  const list = (...hashes: string[]) => hashes.map((hash, i) => ({ line: 3 + 5 * i, hash }));
  const html = (n: number) => Array.from({ length: n }, (_, i) => block(3 + 5 * i, `s${i}`)).join("");

  it("keeps to the same diagram, with its current image, and its new place in the list", () => {
    const scope = note(html(2));
    const pool = new DiagramPool();
    decorate(scope, list("a", "b"), url("light"), pool);
    const viewed = { key: "b#0", index: 1, keys: ["a#0", "b#0"] };
    decorate(scope, list("a", "b"), url("dark"), pool);
    const now = followViewed(pool, list("a", "b"), viewed)!;
    expect(now.image.getAttribute("src")).toContain("h=b&theme=dark");
    scope.innerHTML = html(3);
    decorate(scope, list("new", "a", "b"), url("dark"), pool);
    expect(followViewed(pool, list("new", "a", "b"), viewed)!.viewed).toEqual({ key: "b#0", index: 2, keys: ["new#0", "a#0", "b#0"] });
  });

  it("follows a diagram edited in place to its new drawing", () => {
    const scope = note(html(2));
    const pool = new DiagramPool();
    decorate(scope, list("a", "b"), url("light"), pool);
    // A live update: the note's HTML again, with the second block's source changed.
    scope.innerHTML = html(2);
    decorate(scope, list("a", "b2"), url("light"), pool);
    const now = followViewed(pool, list("a", "b2"), { key: "b#0", index: 1, keys: ["a#0", "b#0"] })!;
    expect(now.viewed.key).toBe("b2#0");
    expect(now.image.getAttribute("src")).toContain("h=b2");
  });

  it("does not move to a diagram that was already there: a delete and an append, or a reordering", () => {
    const scope = note(html(3));
    const pool = new DiagramPool();
    decorate(scope, list("a", "b", "c"), url("light"), pool);
    const viewed = { key: "b#0", index: 1, keys: ["a#0", "b#0", "c#0"] };
    // b deleted, d added at the end: the same count, and c now where b was.
    scope.innerHTML = html(3);
    decorate(scope, list("a", "c", "d"), url("light"), pool);
    expect(followViewed(pool, list("a", "c", "d"), viewed)).toBeNull();
    // b edited and moved to the front: a is now where b was.
    scope.innerHTML = html(3);
    decorate(scope, list("b2", "a", "c"), url("light"), pool);
    expect(followViewed(pool, list("b2", "a", "c"), viewed)).toBeNull();
  });

  it("finds nothing when the diagram is gone, or has failed", () => {
    const scope = note(html(2));
    const pool = new DiagramPool();
    decorate(scope, list("a", "b"), url("light"), pool);
    decorate(scope, list("a"), url("light"), pool);
    expect(followViewed(pool, list("a"), { key: "b#0", index: 1, keys: ["a#0", "b#0"] })).toBeNull();
    pool.get("a#0")!.dispatchEvent(new Event("error"));
    expect(followViewed(pool, list("a", "b"), { key: "a#0", index: 0, keys: ["a#0", "b#0"] })).toBeNull();
    expect(diagramKeys(list("s", "t", "s"))).toEqual(["s#0", "t#0", "s#1"]);
  });
});

describe("holdInView", () => {
  /**
   * A note with images above the target the daemon did not measure (#177,
   * review of PR #181), which have no box until they load, and one it did,
   * which can still fail: the scroll is taken again as each one settles —
   * until the reader scrolls.
   */
  function setup() {
    const scope = note(block(3, "a") + block(8, "b") + '<p data-line="12">target</p>' + block(14, "after"));
    decorate(
      scope,
      [
        { line: 3, hash: "a" },
        { line: 8, hash: "b", width: 100, height: 50 },
        { line: 14, hash: "c" },
      ],
      url("light"),
      new DiagramPool(),
    );
    const target = scope.querySelector('[data-line="12"]')!;
    const scrolls = vi.fn();
    (target as HTMLElement).scrollIntoView = scrolls;
    const [unmeasured, measured, below] = [...scope.querySelectorAll("img")];
    return { target, scrolls, unmeasured, measured, below };
  }

  it("scrolls to the target again when an unmeasured image above it loads or fails", () => {
    const { target, scrolls, unmeasured, measured, below } = setup();
    const stop = holdInView(target);
    below.dispatchEvent(new Event("load"));
    // A measured image's load moves nothing, so it scrolls nothing: a reader
    // who has moved on is not pulled back (review of PR #204, nit 1).
    measured.dispatchEvent(new Event("load"));
    expect(scrolls).not.toHaveBeenCalled();
    unmeasured.dispatchEvent(new Event("load"));
    expect(scrolls).toHaveBeenCalledTimes(1);
    expect(scrolls).toHaveBeenCalledWith({ block: "center" });
    stop();
  });

  it("scrolls to the target again when a measured image above it fails and its code block comes back", () => {
    // Review of PR #202, nit 2, taken by #189: the reserved box goes, and the
    // code block that replaces it is another height.
    const { target, scrolls, measured } = setup();
    const stop = holdInView(target);
    measured.dispatchEvent(new Event("error"));
    expect(scrolls).toHaveBeenCalledTimes(1);
    expect(measured.isConnected).toBe(false);
    stop();
  });

  it("does so after a failure too, once the code block is back", () => {
    const { target, scrolls, unmeasured } = setup();
    const stop = holdInView(target);
    unmeasured.dispatchEvent(new Event("error"));
    expect(scrolls).toHaveBeenCalledTimes(1);
    stop();
  });

  it("lets go when the reader scrolls", () => {
    const { target, scrolls, unmeasured } = setup();
    const stop = holdInView(target);
    window.dispatchEvent(new Event("wheel"));
    unmeasured.dispatchEvent(new Event("load"));
    expect(scrolls).not.toHaveBeenCalled();
    stop();
  });

  it("lets go when stopped", () => {
    const { target, scrolls, unmeasured } = setup();
    holdInView(target)();
    unmeasured.dispatchEvent(new Event("load"));
    expect(scrolls).not.toHaveBeenCalled();
  });
});
