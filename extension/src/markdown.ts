/**
 * HTML to GitHub-flavoured markdown, for a clipped page or selection.
 *
 * Turndown with its GFM plugin does the work; what is added here is the part a
 * clip needs and a generic converter cannot know: every link and image is
 * resolved against the page's own URL, so a note that has left the browser
 * still points at something. The functions are pure and take the base URL as
 * an argument rather than reading `document`, which is what lets them be
 * tested under Node and run unchanged inside the page.
 */

import TurndownService from "turndown";
import { gfm } from "turndown-plugin-gfm";

/**
 * Schemes a link may keep. Anything else — `javascript:` above all — becomes
 * plain text: a clipped note is read in an editor and later in the app, and a
 * link that runs code has no business travelling there.
 */
const LINK_SCHEMES = new Set(["http:", "https:", "mailto:", "tel:", "ftp:"]);

/**
 * Schemes an image may keep. `data:` is deliberately absent: an inlined image
 * is often megabytes of base64 in the middle of a note, which is neither
 * readable in an editor nor worth the daemon's size limit.
 */
const IMAGE_SCHEMES = new Set(["http:", "https:"]);

/** Class and attribute shapes that name a code block's language. */
const LANGUAGE_PATTERNS = [
  /(?:^|\s)language-([^\s]+)/,
  /(?:^|\s)lang-([^\s]+)/,
  /(?:^|\s)highlight-(?:text|source)-([^\s]+)/,
];

/**
 * GitHub's shape — `<div class="highlight highlight-source-go"><pre>…` — and
 * the plainer `<div class="highlight" data-lang="go">` beside it.
 */
const HIGHLIGHT_CLASS = /(?:^|\s)highlight(?:$|[\s-])/;

/**
 * The `pre` a highlight wrapper is wrapping, or null when this is not one.
 *
 * The wrapper's *first* element child must be the `pre`, which is the test
 * the GFM plugin's own rule makes, and the code is taken from that `pre` and
 * not from the div: GitHub's rendered markup puts a clipboard-copy container
 * beside the code, and a div whose content starts with something other than
 * a `pre` is not a code block at all and must not be swallowed as one.
 */
function highlightPre(node: HTMLElement): HTMLElement | null {
  if (node.nodeName !== "DIV") return null;
  if (!HIGHLIGHT_CLASS.test(node.getAttribute("class") ?? "")) return null;
  const first = node.children[0];
  return first !== undefined && first.nodeName === "PRE" ? (first as HTMLElement) : null;
}

/** `value` resolved against `base`, or null when it is not a URL at all. */
export function absoluteUrl(base: string, value: string | null | undefined): string | null {
  if (value === null || value === undefined) return null;
  const text = value.trim();
  if (text === "") return null;
  try {
    return new URL(text, base).href;
  } catch {
    return null;
  }
}

function scheme(url: string): string {
  try {
    return new URL(url).protocol;
  } catch {
    return "";
  }
}

/** A URL as it can appear between the parentheses of a markdown link. */
function inlineTarget(url: string): string {
  const escaped = url.replace(/([()])/g, "\\$1");
  return /\s/.test(escaped) ? `<${escaped}>` : escaped;
}

function titleSuffix(node: HTMLElement): string {
  // A newline in the attribute would split the link over two lines and it
  // would stop being a link, so the page does not get to choose that.
  const title = (node.getAttribute("title") ?? "").replace(/\s+/g, " ").trim();
  if (title === "") return "";
  return ` "${title.replace(/"/g, '\\"')}"`;
}

/**
 * The language of a code block, from the element the rule matched, the `pre`
 * inside it, or that `pre`'s `code` child.
 */
function codeLanguage(node: HTMLElement, pre: HTMLElement): string {
  const code = pre.querySelector("code");
  // A class names the language inside a well-known prefix; a `data-lang` is
  // the language outright. Reading a bare class as a language would make
  // `<pre class="prettyprint">` a language of that name.
  for (const classes of [
    node.getAttribute("class"),
    pre.getAttribute("class"),
    code?.getAttribute("class"),
  ]) {
    if (classes === null || classes === undefined) continue;
    for (const pattern of LANGUAGE_PATTERNS) {
      const match = pattern.exec(classes);
      if (match !== null) return match[1] ?? "";
    }
  }
  for (const element of [node, pre, code]) {
    const named = element?.getAttribute("data-lang") ?? element?.getAttribute("data-language");
    if (named !== null && named !== undefined && named.trim() !== "") return named.trim();
  }
  return "";
}

/** A fence long enough to hold code that contains backtick runs of its own. */
function fenceFor(code: string, marker: string): string {
  let longest = 0;
  for (const run of code.match(/`+/g) ?? []) longest = Math.max(longest, run.length);
  return marker[0]!.repeat(Math.max(3, longest + 1));
}

/** The first candidate of a `srcset`, which is a URL followed by a descriptor. */
function firstCandidate(srcset: string | null): string | null {
  const first = srcset?.split(",")[0]?.trim().split(/\s+/)[0];
  return first === undefined || first === "" ? null : first;
}

/**
 * The image this element actually shows.
 *
 * A lazily loaded image usually carries a 1×1 `data:` placeholder in `src`
 * and the real address in `data-src` or `srcset`, so a `data:` `src` is not
 * the answer when another attribute has one — the page has told us where the
 * image is. A `data:` URI is only returned when it is all there is, and the
 * caller drops it.
 */
function imageSource(node: HTMLElement): string | null {
  const candidates = [
    node.getAttribute("src"),
    node.getAttribute("data-src"),
    firstCandidate(node.getAttribute("srcset")),
    firstCandidate(node.getAttribute("data-srcset")),
  ].filter((value): value is string => value !== null && value.trim() !== "");
  return candidates.find((value) => !/^data:/i.test(value.trim())) ?? candidates[0] ?? null;
}

/**
 * A Turndown service configured for clipping, with `baseUrl` as the page every
 * relative URL is resolved against.
 */
export function clipTurndown(baseUrl: string): TurndownService {
  const service = new TurndownService({
    headingStyle: "atx",
    hr: "---",
    bulletListMarker: "-",
    codeBlockStyle: "fenced",
    fence: "```",
    emDelimiter: "_",
    strongDelimiter: "**",
    linkStyle: "inlined",
  });
  service.use(gfm);

  // Nothing that is not prose. Turndown would otherwise emit a script's source
  // as the paragraph it thinks it is.
  service.remove(["script", "style", "noscript"]);

  // Turndown's own fenced-code rule reads `language-x` from the `code` element
  // only, and writes the fence at a fixed three backticks; the GFM plugin's
  // `highlightedCodeBlock` does the same for GitHub's wrapper div, and would
  // otherwise claim that shape before this rule saw it. This one also takes
  // `lang-x`, `highlight-source-x` and `data-lang`, covers a `pre` with no
  // `code` child, and lengthens the fence when the code contains one.
  service.addRule("clipFencedCode", {
    filter: (node) => node.nodeName === "PRE" || highlightPre(node) !== null,
    replacement: (_content, node, options) => {
      const pre = node.nodeName === "PRE" ? node : highlightPre(node)!;
      const code = (pre.textContent ?? "").replace(/\n+$/, "");
      const fence = fenceFor(code, options.fence ?? "```");
      return `\n\n${fence}${codeLanguage(node, pre)}\n${code}\n${fence}\n\n`;
    },
  });

  service.addRule("clipLink", {
    filter: (node) => node.nodeName === "A" && node.getAttribute("href") !== null,
    replacement: (content, node) => {
      if (content.trim() === "") return "";
      const href = absoluteUrl(baseUrl, node.getAttribute("href"));
      // An unresolvable or non-navigable target keeps the text and loses the
      // link, which is the honest half of what was there.
      if (href === null || !LINK_SCHEMES.has(scheme(href))) return content;
      return `[${content}](${inlineTarget(href)}${titleSuffix(node)})`;
    },
  });

  service.addRule("clipImage", {
    filter: "img",
    replacement: (_content, node) => {
      const src = absoluteUrl(baseUrl, imageSource(node));
      if (src === null || !IMAGE_SCHEMES.has(scheme(src))) return "";
      const alt = (node.getAttribute("alt") ?? "").replace(/\s+/g, " ").trim();
      return `![${alt}](${inlineTarget(src)}${titleSuffix(node)})`;
    },
  });

  return service;
}

/**
 * Trailing whitespace dropped and runs of blank lines collapsed — **outside
 * fenced code only**.
 *
 * Run over the whole document these two tidies edit the clip: PEP 8 puts two
 * blank lines between top-level definitions, and trailing spaces are a line
 * break in a markdown sample and a real difference in a diff or a patch. The
 * daemon writes what it is sent byte for byte; the extension must not quietly
 * reflow what it found on the page either.
 */
function tidyOutsideCode(markdown: string): string {
  const out: string[] = [];
  let open: Fence | null = null;
  let blanks = 0;
  for (const line of markdown.split("\n")) {
    if (open !== null) {
      // Inside a fence: verbatim, including trailing spaces and blank lines.
      out.push(line);
      if (closesFence(line, open)) open = null;
      continue;
    }
    const opening = openingFence(line);
    if (opening !== null) {
      open = opening;
      blanks = 0;
      out.push(line.replace(/[ \t]+$/, ""));
      continue;
    }
    const trimmed = line.replace(/[ \t]+$/, "");
    if (trimmed === "") {
      blanks += 1;
      if (blanks > 1) continue;
    } else {
      blanks = 0;
    }
    out.push(trimmed);
  }
  return out.join("\n").trim();
}

/** An open fence: what it is made of, and what it hangs off. */
interface Fence {
  /** Everything before the backticks: indentation, `>` markers, a list marker. */
  prefix: string;
  /** The run of backticks or tildes itself. */
  fence: string;
}

/**
 * A fence is not always at the left margin. Turndown indents a list item's
 * continuation lines, prefixes a blockquote's lines with `> `, and puts the
 * first line of a list item's content on the same line as its marker — so a
 * fence can open on `    \`\`\`python`, on `> \`\`\`` or on `> 1.  \`\`\``.
 */
const FENCE = /^((?:[ \t]*>)*[ \t]*(?:(?:[-*+]|\d+[.)])[ \t]+)?[ \t]*)(`{3,}|~{3,})/;

function openingFence(line: string): Fence | null {
  const match = FENCE.exec(line);
  return match === null ? null : { prefix: match[1]!, fence: match[2]! };
}

function quoteDepth(prefix: string): number {
  return (prefix.match(/>/g) ?? []).length;
}

/**
 * Whether this line closes `open`.
 *
 * The prefixes are compared by blockquote depth rather than character for
 * character, because Turndown's own output does not repeat them exactly: a
 * fenced block inside a list inside a blockquote opens on `> 1.  \`\`\`` and
 * closes on `>     \`\`\``. What keeps a line of code from closing its own
 * block is the fence itself — a closer must be at least as long as the
 * opener, and `fenceFor` has already lengthened the opener past anything the
 * code contains.
 */
function closesFence(line: string, open: Fence): boolean {
  const match = FENCE.exec(line);
  if (match === null) return false;
  const [, prefix, fence] = match as unknown as [string, string, string];
  if (fence[0] !== open.fence[0] || fence.length < open.fence.length) return false;
  if (quoteDepth(prefix) !== quoteDepth(open.prefix)) return false;
  // Nothing but the fence on the line: an info string means it opens a block.
  return /^[ \t]*$/.test(line.slice(match[0].length));
}

/**
 * `html` as GitHub-flavoured markdown, with every link and image absolute
 * against `baseUrl` — which is the document's *base* URI, not its address: a
 * `<base href>` is what relative URLs in the markup resolve against, and the
 * two differ whenever a page carries one. The result is trimmed; the trailing newline a note wants
 * is added where the request is built, so there is one place that decides it.
 */
export function htmlToMarkdown(html: string, baseUrl: string): string {
  return tidyOutsideCode(clipTurndown(baseUrl).turndown(html));
}
