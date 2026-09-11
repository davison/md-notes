/**
 * The conversion is the part of the clipper a user reads afterwards, so it is
 * tested on fixtures that look like the pages it will meet: an article, and a
 * fragment torn out of the middle of one.
 */
import { describe, expect, it } from "vitest";
import { absoluteUrl, htmlToMarkdown } from "./markdown";

const PAGE = "https://example.com/posts/abstraction/index.html";

const ARTICLE = `
<article>
  <h1>The Cost of Abstraction</h1>
  <p>An <em>early</em> <strong>cost</strong> is <code>indirection</code>.</p>
  <h2>Three kinds</h2>
  <ul>
    <li>Leaky
      <ul><li>at the seams</li><li>under load</li></ul>
    </li>
    <li>Expensive</li>
  </ul>
  <ol><li>First</li><li>Second</li></ol>
  <pre><code class="language-go">func main() {
\tprintln("hi")
}</code></pre>
  <table>
    <thead><tr><th>Layer</th><th>Cost</th></tr></thead>
    <tbody><tr><td>One</td><td>Low</td></tr><tr><td>Two</td><td>High</td></tr></tbody>
  </table>
  <p>See <a href="../other/">the other post</a> and <a href="/about">about</a>.</p>
  <p><img src="diagram.png" alt="A diagram"></p>
  <blockquote><p>Quoted.</p></blockquote>
</article>`;

describe("htmlToMarkdown", () => {
  const markdown = htmlToMarkdown(ARTICLE, PAGE);

  it("writes ATX headings", () => {
    expect(markdown).toContain("# The Cost of Abstraction");
    expect(markdown).toContain("## Three kinds");
  });

  it("keeps nested and ordered lists", () => {
    expect(markdown).toContain("-   Leaky");
    expect(markdown).toMatch(/\n {4}-\s+at the seams/);
    expect(markdown).toContain("1.  First");
    expect(markdown).toContain("2.  Second");
  });

  it("fences code with its language", () => {
    expect(markdown).toContain('```go\nfunc main() {\n\tprintln("hi")\n}\n```');
  });

  it("writes a GFM table", () => {
    expect(markdown).toContain("| Layer | Cost |");
    expect(markdown).toContain("| One | Low |");
  });

  it("resolves relative links and images against the page", () => {
    expect(markdown).toContain("[the other post](https://example.com/posts/other/)");
    expect(markdown).toContain("[about](https://example.com/about)");
    expect(markdown).toContain(
      "![A diagram](https://example.com/posts/abstraction/diagram.png)",
    );
  });

  it("keeps inline formatting, quotes and code spans", () => {
    expect(markdown).toContain("_early_");
    expect(markdown).toContain("**cost**");
    expect(markdown).toContain("`indirection`");
    expect(markdown).toContain("> Quoted.");
  });

  it("leaves no trailing whitespace and no run of blank lines", () => {
    expect(markdown).toBe(markdown.trim());
    expect(markdown).not.toMatch(/[ \t]+\n/);
    expect(markdown).not.toMatch(/\n{3}/);
  });

  it("converts a selection fragment, which has no document around it", () => {
    const fragment =
      '<p>A <a href="./two.html">link</a> and <del>a strike</del>.</p><p>Second paragraph.</p>';
    expect(htmlToMarkdown(fragment, PAGE)).toBe(
      "A [link](https://example.com/posts/abstraction/two.html) and ~a strike~.\n\nSecond paragraph.",
    );
  });

  it("takes a language from lang-, highlight-source- and data-lang too", () => {
    expect(htmlToMarkdown('<pre><code class="lang-sh">ls</code></pre>', PAGE)).toBe(
      "```sh\nls\n```",
    );
    expect(
      htmlToMarkdown('<div class="highlight highlight-source-js"><pre>a()</pre></div>', PAGE),
    ).toBe("```js\na()\n```");
    expect(htmlToMarkdown('<pre class="highlight-source-js">a()</pre>', PAGE)).toBe(
      "```js\na()\n```",
    );
    expect(htmlToMarkdown('<pre data-lang="rust"><code>fn f() {}</code></pre>', PAGE)).toBe(
      "```rust\nfn f() {}\n```",
    );
  });

  it("does not read a bare class as a language", () => {
    expect(htmlToMarkdown('<pre class="prettyprint"><code>x</code></pre>', PAGE)).toBe("```\nx\n```");
  });

  it("lengthens the fence when the code contains one", () => {
    const md = htmlToMarkdown("<pre><code>a ``` b</code></pre>", PAGE);
    expect(md).toBe("````\na ``` b\n````");
  });

  it("fences a pre with no code child, and keeps its whitespace", () => {
    expect(htmlToMarkdown("<pre>  two  spaces\n  and a line</pre>", PAGE)).toBe(
      "```\n  two  spaces\n  and a line\n```",
    );
  });

  it("keeps a task list and strikethrough from the GFM plugin", () => {
    const md = htmlToMarkdown(
      '<ul><li><input type="checkbox" checked>done</li><li><input type="checkbox">todo</li></ul>',
      PAGE,
    );
    expect(md).toContain("[x] done");
    expect(md).toContain("[ ] todo");
  });

  it("drops a javascript: link but keeps its text", () => {
    expect(htmlToMarkdown('<p><a href="javascript:alert(1)">Click</a></p>', PAGE)).toBe("Click");
  });

  it("drops an inlined data: image rather than carrying base64 into the note", () => {
    const md = htmlToMarkdown(
      '<p>before<img src="data:image/gif;base64,R0lGOD" alt="pixel">after</p>',
      PAGE,
    );
    expect(md).toBe("beforeafter");
  });

  it("takes a lazily loaded image from data-src or srcset", () => {
    expect(htmlToMarkdown('<img data-src="/a.png" alt="a">', PAGE)).toBe(
      "![a](https://example.com/a.png)",
    );
    expect(htmlToMarkdown('<img srcset="/b.png 1x, /b2.png 2x" alt="b">', PAGE)).toBe(
      "![b](https://example.com/b.png)",
    );
  });

  it("wraps a target containing spaces in angle brackets", () => {
    expect(htmlToMarkdown('<a href="/a b.html">x</a>', PAGE)).toBe(
      "[x](https://example.com/a%20b.html)",
    );
  });

  it("drops script and style content", () => {
    const md = htmlToMarkdown("<div><script>alert(1)</script><style>p{}</style><p>Text</p></div>", PAGE);
    expect(md).toBe("Text");
  });
});

describe("absoluteUrl", () => {
  it("resolves a relative reference against the page", () => {
    expect(absoluteUrl(PAGE, "../x")).toBe("https://example.com/posts/x");
    expect(absoluteUrl(PAGE, "https://other.example/y")).toBe("https://other.example/y");
  });

  it("is null for nothing and for a value no parser accepts", () => {
    expect(absoluteUrl(PAGE, null)).toBeNull();
    expect(absoluteUrl(PAGE, "   ")).toBeNull();
    expect(absoluteUrl("not a base", "relative")).toBeNull();
  });
});
