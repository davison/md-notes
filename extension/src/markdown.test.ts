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

  it("takes GitHub's rendered code block, clipboard container and all", () => {
    // The real markup: the `pre` first, a copy button beside it. The code is
    // the `pre`'s, and the fence is long enough for what is inside it.
    const github =
      '<div class="highlight highlight-source-js notranslate position-relative overflow-auto" dir="auto">' +
      "<pre>x ``` y</pre>" +
      '<div class="zeroclipboard-container"><clipboard-copy>Copy</clipboard-copy></div>' +
      "</div>";
    expect(htmlToMarkdown(github, PAGE)).toBe("````js\nx ``` y\n````");
  });

  it("does not swallow a div that only happens to contain a pre", () => {
    expect(
      htmlToMarkdown('<div class="highlight"><p>Prose first.</p><pre>x</pre></div>', PAGE),
    ).toBe("Prose first.\n\n```\nx\n```");
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

  // Beside a fenced code block a page puts three kinds of thing: a copy
  // button, which is the browser's business and not the note's; a caption or a
  // filename, which is content; and, before this, nothing that survived at all
  // (davison/md-notes#47).

  it("keeps a caption beside GitHub's code block and drops the copy container", () => {
    const html =
      '<div class="highlight highlight-source-go notranslate position-relative overflow-auto" dir="auto">' +
      "<pre>func main() {}</pre>" +
      '<div class="zeroclipboard-container">' +
      '<clipboard-copy aria-label="Copy" class="js-clipboard-copy">Copy</clipboard-copy>' +
      "</div>" +
      "<p>Listing 1 — the whole program.</p>" +
      "</div>";
    expect(htmlToMarkdown(html, PAGE)).toBe(
      "```go\nfunc main() {}\n```\n\nListing 1 — the whole program.",
    );
  });

  it("keeps a figure's caption after the code, and the wrapper's language with it", () => {
    const html =
      '<figure class="highlight highlight-source-python">' +
      '<pre><code>print("hi")</code></pre>' +
      "<figcaption>Figure 2 — printing.</figcaption>" +
      "</figure>";
    expect(htmlToMarkdown(html, PAGE)).toBe(
      '```python\nprint("hi")\n```\n\nFigure 2 — printing.',
    );
  });

  it("keeps a filename line before the code, which used to cost the block its language", () => {
    const html =
      '<div class="highlight highlight-source-go">' +
      '<div class="filename">cmd/mdn/main.go</div>' +
      "<pre>package main</pre>" +
      '<button class="copy-button" type="button">Copy</button>' +
      "</div>";
    expect(htmlToMarkdown(html, PAGE)).toBe(
      "cmd/mdn/main.go\n\n```go\npackage main\n```",
    );
  });

  it("drops an aria-hidden decoration and keeps the prose beside it", () => {
    const html =
      '<div class="highlight" data-lang="sh">' +
      '<span aria-hidden="true">$</span>' +
      "<pre>ls</pre>" +
      '<span role="button">Copy</span>' +
      "<p>Lists the directory.</p>" +
      "</div>";
    expect(htmlToMarkdown(html, PAGE)).toBe("```sh\nls\n```\n\nLists the directory.");
  });

  it("keeps a bare text node beside the code, which the old rule dropped", () => {
    expect(
      htmlToMarkdown('<div class="highlight"><pre>x</pre>A trailing caption.</div>', PAGE),
      ).toBe("```\nx\n```\n\nA trailing caption.");
  });

  // A table with no header row is outside GFM, and the plugin keeps what it
  // cannot convert — which left the page's own `<table>` HTML in the note
  // (davison/md-notes#45). So are spans and nesting, in their own ways.

  it("gives a headerless table a synthesised empty header and keeps every row", () => {
    const html =
      "<table>" +
      "<tr><td>Rows</td><td>with no header</td></tr>" +
      "<tr><td>Two</td><td>Second</td></tr>" +
      "</table>";
    const md = htmlToMarkdown(html, PAGE);
    expect(md).toBe("|  |  |\n| --- | --- |\n| Rows | with no header |\n| Two | Second |");
    expect(md).not.toContain("<table>");
  });

  it("lays a colspan and a rowspan out on a grid that still lines up", () => {
    const html =
      "<table>" +
      "<thead><tr><th>Quarter</th><th>Plan</th><th>Actual</th></tr></thead>" +
      "<tbody>" +
      '<tr><td colspan="2">Q1 (combined)</td><td>10</td></tr>' +
      '<tr><td rowspan="2">Q2</td><td>5</td><td>6</td></tr>' +
      "<tr><td>7</td><td>8</td></tr>" +
      "</tbody></table>";
    expect(htmlToMarkdown(html, PAGE)).toBe(
      [
        "| Quarter | Plan | Actual |",
        "| --- | --- | --- |",
        "| Q1 (combined) |  | 10 |",
        "| Q2 | 5 | 6 |",
        "|  | 7 | 8 |",
      ].join("\n"),
    );
  });

  it("flattens a nested table into the cell that holds it", () => {
    const html =
      "<table>" +
      "<thead><tr><th>Region</th><th>Quarters</th></tr></thead>" +
      "<tbody><tr><td>North</td><td>" +
      "<table><tr><td>Q1</td><td>10</td></tr><tr><td>Q2</td><td>12</td></tr></table>" +
      "</td></tr></tbody></table>";
    const md = htmlToMarkdown(html, PAGE);
    expect(md).toBe(
      "| Region | Quarters |\n| --- | --- |\n| North | Q1 / 10; Q2 / 12 |",
    );
    expect(md).not.toContain("<table>");
  });

  it("keeps a table's caption, as a paragraph before it", () => {
    const html =
      "<table><caption>Costs by layer</caption>" +
      "<tr><td>One</td><td>Low</td></tr></table>";
    expect(htmlToMarkdown(html, PAGE)).toBe(
      "Costs by layer\n\n|  |  |\n| --- | --- |\n| One | Low |",
    );
  });

  it("takes the delimiter row's alignment from the header cells", () => {
    const html =
      "<table><thead><tr>" +
      '<th align="left">L</th><th align="center">C</th><th align="right">R</th>' +
      "</tr></thead><tbody><tr><td>1</td><td>2</td><td>3</td></tr></tbody></table>";
    expect(htmlToMarkdown(html, PAGE)).toBe(
      "| L | C | R |\n| :-- | :-: | --: |\n| 1 | 2 | 3 |",
    );
  });

  it("escapes a pipe in a cell and flattens a cell written as two blocks", () => {
    const html =
      "<table><thead><tr><th>Pattern</th><th>Notes</th></tr></thead>" +
      "<tbody><tr><td>a|b</td><td><p>One.</p><p>Two.</p></td></tr></tbody></table>";
    expect(htmlToMarkdown(html, PAGE)).toBe(
      "| Pattern | Notes |\n| --- | --- |\n| a\\|b | One. Two. |",
    );
  });

  it("converts the cells of a headerless table, links and all", () => {
    const html =
      '<table><tr><td><a href="/x">x</a></td><td><strong>b</strong></td></tr></table>';
    expect(htmlToMarkdown(html, PAGE)).toBe(
      "|  |  |\n| --- | --- |\n| [x](https://example.com/x) | **b** |",
    );
  });

  // Not every `<table>` is a table. A page laying an article out in one, and
  // Pygments' line-number wrapper, hold the structure the note is for, and a
  // grid built out of them squashes it onto one line
  // (review of PR #105).

  it("converts a layout table as blocks, keeping the data table inside it", () => {
    const html =
      '<table width="100%"><tr><td>' +
      "<h2>Quarterly</h2><p>Some prose.</p>" +
      "<table><thead><tr><th>Quarter</th><th>Spend</th></tr></thead>" +
      "<tbody><tr><td>Q1</td><td>10</td></tr><tr><td>Q2</td><td>20</td></tr></tbody></table>" +
      "<p>After.</p>" +
      "</td></tr></table>";
    const md = htmlToMarkdown(html, PAGE);
    expect(md).toBe(
      [
        "## Quarterly",
        "",
        "Some prose.",
        "",
        "| Quarter | Spend |",
        "| --- | --- |",
        "| Q1 | 10 |",
        "| Q2 | 20 |",
        "",
        "After.",
      ].join("\n"),
    );
    expect(md).not.toContain("<table>");
  });

  it("keeps the code in a line-number wrapper, line breaks and language and all", () => {
    // Pygments, Sphinx and MkDocs put the line numbers in one cell and the
    // code in another. Flattened into a grid the code becomes one line, and a
    // note cannot get it back.
    const html =
      '<table class="highlighttable"><tr>' +
      '<td class="linenos"><pre>1\n2</pre></td>' +
      '<td class="code"><div class="highlight">' +
      '<pre><code class="language-go">func main() {\n}</code></pre>' +
      "</div></td>" +
      "</tr></table>";
    const md = htmlToMarkdown(html, PAGE);
    expect(md).toBe("```\n1\n2\n```\n\n```go\nfunc main() {\n}\n```");
    expect(md).not.toContain("<table");
  });

  it("takes the page's own word for it when a table says role=presentation", () => {
    expect(
      htmlToMarkdown('<table role="presentation"><tr><td>Left</td><td>Right</td></tr></table>', PAGE),
    ).toBe("Left\n\nRight");
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
