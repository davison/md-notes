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
  /(?:^|\s)highlight-source-([^\s]+)/,
];

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
  const title = node.getAttribute("title");
  if (title === null || title === "") return "";
  return ` "${title.replace(/"/g, '\\"')}"`;
}

/** The language of a code block, from the `pre` or its `code` child. */
function codeLanguage(node: HTMLElement): string {
  const code = node.querySelector("code");
  // A class names the language inside a well-known prefix; a `data-lang` is
  // the language outright. Reading a bare class as a language would make
  // `<pre class="prettyprint">` a language of that name.
  for (const classes of [node.getAttribute("class"), code?.getAttribute("class")]) {
    if (classes === null || classes === undefined) continue;
    for (const pattern of LANGUAGE_PATTERNS) {
      const match = pattern.exec(classes);
      if (match !== null) return match[1] ?? "";
    }
  }
  for (const element of [node, code]) {
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

/**
 * The image this element actually shows. A lazily loaded image often carries
 * nothing useful in `src`, so `data-src` and the first `srcset` candidate are
 * tried in turn before giving up.
 */
function imageSource(node: HTMLElement): string | null {
  const direct = node.getAttribute("src") ?? node.getAttribute("data-src");
  if (direct !== null && direct.trim() !== "") return direct;
  const srcset = node.getAttribute("srcset") ?? node.getAttribute("data-srcset");
  if (srcset === null) return null;
  const first = srcset.split(",")[0]?.trim().split(/\s+/)[0];
  return first === undefined || first === "" ? null : first;
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
  // only, and writes the fence at a fixed three backticks. This one also takes
  // `lang-x`, GitHub's `highlight-source-x` and `data-lang`, covers a `pre`
  // with no `code` child, and lengthens the fence when the code contains one.
  service.addRule("clipFencedCode", {
    filter: (node) => node.nodeName === "PRE",
    replacement: (_content, node, options) => {
      const code = (node.textContent ?? "").replace(/\n+$/, "");
      const fence = fenceFor(code, options.fence ?? "```");
      return `\n\n${fence}${codeLanguage(node)}\n${code}\n${fence}\n\n`;
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
  let fence: string | null = null;
  let blanks = 0;
  for (const line of markdown.split("\n")) {
    if (fence !== null) {
      // Inside a fence: verbatim, including trailing spaces and blank lines.
      out.push(line);
      if (new RegExp(`^ {0,3}${fence[0]}{${fence.length},}[ \t]*$`).test(line)) fence = null;
      continue;
    }
    const opening = /^ {0,3}(`{3,}|~{3,})/.exec(line);
    if (opening !== null) {
      fence = opening[1]!;
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

/**
 * `html` as GitHub-flavoured markdown, with every link and image absolute
 * against `baseUrl`. The result is trimmed; the trailing newline a note wants
 * is added where the request is built, so there is one place that decides it.
 */
export function htmlToMarkdown(html: string, baseUrl: string): string {
  return tidyOutsideCode(clipTurndown(baseUrl).turndown(html));
}
