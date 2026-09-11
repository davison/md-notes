/**
 * The conversion is the part of the clipper a user reads afterwards, so it is
 * tested on fixtures that look like the pages it will meet: an article, and a
 * fragment torn out of the middle of one.
 */
import { describe, expect, it } from "vitest";
import { absoluteUrl, clipTurndown, htmlToMarkdown } from "./markdown";

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

  it("leaves fenced code exactly as the page wrote it", () => {
    // PEP 8 puts two blank lines between top-level definitions, and trailing
    // spaces are a line break in a markdown sample and a real difference in a
    // patch. Neither tidy may reach inside a fence.
    const code = 'def a():\n    return 1  \n\n\ndef b():\n\treturn "  "\n';
    const md = htmlToMarkdown(
      `<pre><code class="language-python">${code}</code></pre>`,
      PAGE,
    );
    expect(md).toBe("```python\n" + code.replace(/\n+$/, "") + "\n```");
    expect(md).toContain("return 1  \n");
    expect(md).toContain("\n\n\ndef b():");
  });

  // Turndown does not leave a fence at the left margin when it is inside
  // something: a list item's continuation lines are indented four spaces, a
  // blockquote's lines carry `> `, and the first line of a list item shares
  // its marker's line. A tracker that only recognises a fence at the margin
  // reflows all three — which is the shape a tutorial's numbered steps take.
  const INDENTED_CODE = 'def one():\n    return 1  \n\n\ndef two():\n    x = 1';

  /** What Turndown produced, before the tidy ran over it. */
  const untidied = (html: string) => clipTurndown(PAGE).turndown(html);

  /** The fenced region of a document, from the first fence line to the last. */
  const fencedRegion = (markdown: string) => {
    const lines = markdown.split("\n");
    const fences = lines
      .map((line, index) => (line.includes("```") ? index : -1))
      .filter((index) => index >= 0);
    return lines.slice(fences[0], (fences[fences.length - 1] ?? 0) + 1).join("\n");
  };

  it("leaves a fence inside a list item alone", () => {
    const html = `<ol><li><p>Install it:</p><pre><code class="language-python">${INDENTED_CODE}</code></pre></li></ol>`;
    const md = htmlToMarkdown(html, PAGE);
    expect(fencedRegion(md)).toBe(fencedRegion(untidied(html)));
    expect(md).toBe(
      "1.  Install it:\n\n    ```python\n    def one():\n        return 1  \n    \n    \n    def two():\n        x = 1\n    ```",
    );
  });

  it("leaves a fence inside a blockquote alone", () => {
    const html = `<blockquote><p>Note:</p><pre><code>${INDENTED_CODE}</code></pre></blockquote>`;
    const md = htmlToMarkdown(html, PAGE);
    expect(fencedRegion(md)).toBe(fencedRegion(untidied(html)));
    expect(md).toContain(">     return 1  \n> \n> \n> def two():");
  });

  it("leaves a fence inside a list inside a blockquote alone", () => {
    // Turndown opens this one on `> 1.  ``` ` and closes it on `>     ``` `,
    // so the two prefixes are not the same string.
    const html = `<blockquote><ol><li><pre><code>${INDENTED_CODE}</code></pre></li></ol></blockquote>`;
    const md = htmlToMarkdown(html, PAGE);
    expect(fencedRegion(md)).toBe(fencedRegion(untidied(html)));
    expect(md).toContain(">         return 1  \n>     \n>     \n>     def two():");
  });

  it("still tidies the prose around a fence", () => {
    const md = htmlToMarkdown(
      "<p>One   </p><p></p><p></p><pre><code>a\n\n\nb</code></pre><p>Two   </p>",
      PAGE,
    );
    expect(md).toBe("One\n\n```\na\n\n\nb\n```\n\nTwo");
  });

  it("does not mistake a fence inside a fence for the end of it", () => {
    const md = htmlToMarkdown("<pre><code>```   \n\n\nstill inside</code></pre>", PAGE);
    expect(md).toBe("````\n```   \n\n\nstill inside\n````");
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

  it("lengthens the fence inside a highlight div too", () => {
    // The GFM plugin's own rule claims this shape and writes three backticks
    // whatever the code contains, which is broken markdown.
    expect(
      htmlToMarkdown('<div class="highlight highlight-source-js"><pre>a ``` b</pre></div>', PAGE),
    ).toBe("````js\na ``` b\n````");
  });

  it("keeps a highlight div's data-lang, which the plugin's rule ignores", () => {
    expect(
      htmlToMarkdown('<div class="highlight" data-lang="zig"><pre>x</pre></div>', PAGE),
    ).toBe("```zig\nx\n```");
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

  it("looks past a data: placeholder to the real address the page gave", () => {
    // The dominant lazy-loading shape: a 1x1 in src, the image in data-src.
    const placeholder = "data:image/gif;base64,R0lGOD";
    expect(htmlToMarkdown(`<img src="${placeholder}" data-src="/real.png" alt="r">`, PAGE)).toBe(
      "![r](https://example.com/real.png)",
    );
    expect(
      htmlToMarkdown(`<img src="${placeholder}" srcset="/wide.png 2x" alt="r">`, PAGE),
    ).toBe("![r](https://example.com/wide.png)");
  });

  it("collapses whitespace in a link or image title, which would break the link", () => {
    expect(htmlToMarkdown('<a href="/x" title="one\ntwo">t</a>', PAGE)).toBe(
      '[t](https://example.com/x "one two")',
    );
    expect(htmlToMarkdown('<img src="/i.png" alt="a" title="one\ntwo">', PAGE)).toBe(
      '![a](https://example.com/i.png "one two")',
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
