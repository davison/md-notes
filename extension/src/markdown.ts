/**
 * HTML to GitHub-flavoured markdown, for a clipped page or selection.
 *
 * Turndown with its GFM plugin does the work; what is added here is the part a
 * clip needs and a generic converter cannot know. Every link and image is
 * resolved against the page's own URL, so a note that has left the browser
 * still points at something. A code block keeps what a page puts beside it —
 * a caption, a filename — and loses only the copy button. And a table the
 * plugin declines, because it has no header row or because it spans or nests,
 * is written as the nearest readable GFM rather than left as the page's HTML.
 * The functions are pure and take the base URL as an argument rather than
 * reading `document`, which is what lets them be tested under Node and run
 * unchanged inside the page.
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
 * A class that names a copy-to-clipboard control rather than content.
 *
 * `clipboard` matches anywhere in the attribute, because `zeroclipboard-container`
 * is GitHub's own name for the thing; `copy` has to be a word of its own, so a
 * caption class is not mistaken for a button on the strength of the letters.
 * Where the two rules disagree with a page, the cost is a lost caption one way
 * and a stray "Copy" in the note the other; this pair is a judgement, not a
 * specification.
 */
const CHROME_CLASS = /clipboard|(?:^|[\s_-])copy(?:$|[\s_-])/i;

/**
 * The `pre` a code-block container is wrapping, or null when this element is
 * not one.
 *
 * The test is deliberately looser than the GFM plugin's, which asks that the
 * div's first *node* is a `pre` and that the class carries
 * `highlight-source-…` or `highlight-text-…`. This one accepts any
 * `div.highlight` — a bare `highlight` with a `data-lang` is a common shape —
 * and a `figure.highlight` beside it, and it takes the first `pre` among the
 * element children *wherever it sits*: a filename strip or a copy button
 * before the code no longer costs the block the language its wrapper names,
 * which is what the old first-element test did.
 *
 * The code is then taken from that `pre` and not from the container, because
 * what sits beside it is a mixture — GitHub's clipboard container on one hand,
 * a caption or a filename that belongs in the note on the other. `aroundTheCode`
 * sorts them.
 */
function containerPre(node: HTMLElement): HTMLElement | null {
  if (node.nodeName !== "DIV" && node.nodeName !== "FIGURE") return null;
  if (!HIGHLIGHT_CLASS.test(node.getAttribute("class") ?? "")) return null;
  for (const child of Array.from(node.children)) {
    if (child.nodeName === "PRE") return child as HTMLElement;
  }
  return null;
}

/**
 * Whether a node beside the code is a control rather than content: a button,
 * GitHub's `clipboard-copy` element, anything a class names as a copy control,
 * and anything the page has already hidden from a screen reader — which is how
 * a decorative icon or a duplicated label announces itself.
 */
function isChrome(node: ChildNode): boolean {
  if (node.nodeType !== 1) return false;
  const element = node as HTMLElement;
  if (element.nodeName === "BUTTON" || element.nodeName === "CLIPBOARD-COPY") return true;
  if (element.getAttribute("role") === "button") return true;
  if (element.getAttribute("aria-hidden") === "true") return true;
  return CHROME_CLASS.test(element.getAttribute("class") ?? "");
}

/**
 * What a code-block container holds before and after its `pre`, converted as
 * ordinary markdown with the chrome dropped.
 *
 * Each side is gathered into one throwaway element and converted in a single
 * pass rather than node by node, so a caption written as several inline nodes
 * stays one paragraph, and a bare text node beside the `pre` is kept rather
 * than lost. Turndown clones whatever it is handed before touching it, so
 * nothing here reaches the document the clip came from.
 */
function aroundTheCode(
  service: TurndownService,
  container: HTMLElement,
  pre: HTMLElement,
): [string, string] {
  const sides = [
    container.ownerDocument.createElement("div"),
    container.ownerDocument.createElement("div"),
  ];
  let side = 0;
  for (const child of Array.from(container.childNodes)) {
    if (child === pre) {
      side = 1;
      continue;
    }
    if (isChrome(child)) continue;
    sides[side]!.appendChild(child.cloneNode(true));
  }
  return [service.turndown(sides[0]!).trim(), service.turndown(sides[1]!).trim()];
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

/** A cell as it will be written: its text, and the border its column takes. */
interface Cell {
  text: string;
  border: string;
}

/** The delimiter row's spelling of an `align` attribute. */
const ALIGNMENT: Record<string, string> = { left: ":--", right: "--:", center: ":-:" };

/**
 * A `colspan` or `rowspan`, clamped. `rowspan="0"` means "to the end of the
 * section" in HTML and is read as one here; a four-figure span is a page's
 * mistake or an attack on this converter's memory, and nothing legible needs
 * one.
 */
function span(cell: HTMLElement, name: string): number {
  const value = Number.parseInt(cell.getAttribute(name) ?? "", 10);
  return Number.isNaN(value) ? 1 : Math.min(Math.max(value, 1), 100);
}

/**
 * The table's own rows, walked rather than taken from `HTMLTableElement.rows`.
 *
 * That property is a descendant search in the DOM the tests run under, so a
 * table with a table inside a cell reports the inner table's rows as its own
 * and the clip grows rows that belong to another grid — while a browser's
 * `rows` does not. That is the whole reason for walking; the walk then has to
 * put the sections back in the order `rows` uses — header, then bodies, then
 * footer — because HTML 4 required `tfoot` before `tbody` and a page that
 * writes `thead` last would otherwise arrive headerless with its header row
 * written out as data.
 */
function rowsOf(table: HTMLElement): HTMLElement[] {
  const sections: Record<string, HTMLElement[]> = { THEAD: [], BODY: [], TFOOT: [] };
  for (const child of Array.from(table.children)) {
    if (child.nodeName === "TR") sections["BODY"]!.push(child as HTMLElement);
    const into = sections[child.nodeName] ?? (child.nodeName === "TBODY" ? sections["BODY"] : null);
    if (into === null || into === undefined) continue;
    for (const row of Array.from(child.children)) {
      if (row.nodeName === "TR") into.push(row as HTMLElement);
    }
  }
  return [...sections["THEAD"]!, ...sections["BODY"]!, ...sections["TFOOT"]!];
}

/**
 * A row's own cells, which `children` gives and `querySelectorAll` would not:
 * a nested table's cells are not this row's.
 */
function cellsOf(row: HTMLElement): HTMLElement[] {
  return Array.from(row.children).filter(
    (child) => child.nodeName === "TD" || child.nodeName === "TH",
  ) as HTMLElement[];
}

/**
 * Content a cell cannot be flattened around: a `pre`, whose line breaks *are*
 * the code, and a heading, which no page writes inside a data cell.
 *
 * Matched by name over the cell's descendants rather than with a selector
 * list, because the DOM the tests run under answers `querySelector("pre, h1")`
 * with the first element it finds whatever its name is, and every table would
 * be a layout table.
 */
const CELL_BLOCK = new Set(["PRE", "H1", "H2", "H3", "H4", "H5", "H6"]);

function holdsBlock(cell: HTMLElement): boolean {
  return Array.from(cell.querySelectorAll("*")).some((child) => CELL_BLOCK.has(child.nodeName));
}

/**
 * Whether this table is carrying data, and not a page's layout.
 *
 * A `<table>` is not always a table. An old manual lays its page out in one,
 * Pygments and Sphinx put the line numbers in one cell and the code in
 * another, and the note wants none of that squashed into a grid: writing a
 * heading, two paragraphs and a data table onto one line destroys exactly the
 * structure the clip is for. The test, in order:
 *
 * - `role="presentation"` (or `none`) is the page saying so itself;
 * - a `th` anywhere is the page saying the opposite — a header means the rows
 *   below it are data, whatever else is in them;
 * - otherwise a table no row of which has two cells is a wrapper, not a grid;
 * - and a cell holding a `pre` or a heading is a layout cell: that is the
 *   line-number wrapper, and the manual.
 *
 * What is not a data table is converted as ordinary blocks instead — never as
 * the page's own HTML, which is the defect this all started from.
 */
function isDataTable(table: HTMLElement): boolean {
  const role = (table.getAttribute("role") ?? "").toLowerCase();
  if (role === "presentation" || role === "none") return false;
  const rows = rowsOf(table).map(cellsOf);
  if (rows.some((cells) => cells.some((cell) => cell.nodeName === "TH"))) return true;
  if (rows.reduce((widest, cells) => Math.max(widest, cells.length), 0) < 2) return false;
  return !rows.some((cells) => cells.some(holdsBlock));
}

/**
 * One cell's content, converted and then flattened onto a single line: a GFM
 * cell holds inline content only, and a newline inside one ends the row.
 *
 * The conversion re-enters the same service on the cell, rather than reading
 * the `content` Turndown has already assembled, because the table is built
 * from its own geometry — which cell sits in which column — and that is lost
 * once the cells are one string. Re-entry is safe: `turndown` clones its
 * input and holds no state between calls.
 */
function cellText(service: TurndownService, cell: HTMLElement): string {
  return service
    .turndown(cell)
    .replace(/\s*\n+\s*/g, " ")
    .replace(/\|/g, "\\|")
    .trim();
}

function borderFor(cell: HTMLElement): string {
  return ALIGNMENT[(cell.getAttribute("align") ?? "").toLowerCase()] ?? "---";
}

/**
 * The table laid out as a rectangle, the way HTML defines one: each cell is
 * placed at the first free column of its row and claims the rectangle its
 * `colspan` and `rowspan` cover, the covered cells are empty, and every row is
 * padded to the width of the widest. A spanning table is then a grid that
 * still lines up, with every value written once and in the column it was
 * written in — which is what GFM can carry of it, since it has no spans.
 */
function tableGrid(service: TurndownService, table: HTMLElement): Cell[][] {
  const grid: Cell[][] = [];
  const rows = rowsOf(table);
  rows.forEach((row, index) => {
    const line = (grid[index] ??= []);
    let column = 0;
    for (const cell of cellsOf(row)) {
      while (line[column] !== undefined) column += 1;
      const placed: Cell = { text: cellText(service, cell), border: borderFor(cell) };
      const columns = span(cell, "colspan");
      // A `rowspan` is clamped to the rows the table actually has, the way
      // HTML's table model and a browser both clamp it: an overrunning span is
      // a common export mistake, and honouring it literally would grow rows in
      // the note that no `<tr>` in the page produced. A `colspan` needs no such
      // clamp — a browser really does widen the table there.
      const down_to = Math.min(span(cell, "rowspan"), rows.length - index);
      for (let down = 0; down < down_to; down += 1) {
        const covered = (grid[index + down] ??= []);
        for (let across = 0; across < columns; across += 1) {
          covered[column + across] =
            down === 0 && across === 0 ? placed : { text: "", border: placed.border };
        }
      }
      column += columns;
    }
  });
  const width = grid.reduce((widest, line) => Math.max(widest, line.length), 0);
  return grid.map((line) =>
    Array.from({ length: width }, (_, column) => line[column] ?? { text: "", border: "---" }),
  );
}

/**
 * Whether the first row is the header. The GFM plugin's test, minus the part
 * about which row it is: only the first is ever asked here.
 */
function isHeaderRow(row: HTMLElement): boolean {
  if (row.parentNode?.nodeName === "THEAD") return true;
  const cells = cellsOf(row);
  return cells.length > 0 && cells.every((cell) => cell.nodeName === "TH");
}

function tableLine(cells: Cell[]): string {
  return `| ${cells.map((cell) => cell.text).join(" | ")} |`;
}

/**
 * Whether this table sits in a cell of a table that is being written as a grid
 * — the only place a table has to be flattened, because only there is it
 * standing where GFM allows inline content and nothing else.
 *
 * A table inside a *layout* table's cell is not flattened: that cell is going
 * to be written as ordinary blocks, where a real table is welcome. When the
 * cell is the root of a conversion it has no table above it at all, which is
 * the re-entrant call `cellText` makes — and that call is only ever made for a
 * table already judged a grid.
 */
function insideGridCell(node: HTMLElement): boolean {
  let cell: Node | null = null;
  for (let parent: Node | null = node.parentNode; parent !== null; parent = parent.parentNode) {
    if (parent.nodeName === "TD" || parent.nodeName === "TH") {
      cell = parent;
      break;
    }
  }
  if (cell === null) return false;
  for (let parent: Node | null = cell.parentNode; parent !== null; parent = parent.parentNode) {
    if (parent.nodeName === "TABLE") return isDataTable(parent as HTMLElement);
  }
  return true;
}

/**
 * A nested table's values on one line — cells joined with ` / `, rows with
 * `; `.
 *
 * GFM has no way to put a table inside a cell, and the alternatives are worse:
 * raw HTML is the defect this is fixing, and lifting the inner table out to
 * stand on its own separates it from the row it describes. This keeps every
 * value, in order, inside the cell it belongs to, and the table around it
 * stays a table.
 */
function nestedTable(grid: Cell[][]): string {
  return grid
    .map((line) =>
      line
        .map((cell) => cell.text)
        .filter((text) => text !== "")
        .join(" / "),
    )
    .filter((line) => line !== "")
    .join("; ");
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
  // What a container holds *beside* the `pre` is sorted rather than dropped:
  // a caption after the code and a filename line before it are content and are
  // converted around the fence; a copy button is chrome and is not (#47).
  service.addRule("clipFencedCode", {
    filter: (node) => node.nodeName === "PRE" || containerPre(node) !== null,
    replacement: (_content, node, options) => {
      const container = node.nodeName === "PRE" ? null : node;
      const pre = container === null ? node : containerPre(container)!;
      const code = (pre.textContent ?? "").replace(/\n+$/, "");
      const fence = fenceFor(code, options.fence ?? "```");
      const block = `${fence}${codeLanguage(node, pre)}\n${code}\n${fence}`;
      if (container === null) return `\n\n${block}\n\n`;
      const [before, after] = aroundTheCode(service, container, pre);
      return `\n\n${[before, block, after].filter((part) => part !== "").join("\n\n")}\n\n`;
    },
  });

  // A table a page is using for layout — one cell wrapped round an article,
  // Pygments' line-number wrapper — is not a grid and must not be written as
  // one: its cells hold the structure the note is for. Its parts are converted
  // as ordinary blocks instead, which is what these rules are for; the GFM
  // plugin's own cell and row rules would write pipes around them.
  service.addRule("clipTableBlocks", {
    filter: ["td", "th", "caption"],
    replacement: (content) => (content.trim() === "" ? "" : `\n\n${content.trim()}\n\n`),
  });
  service.addRule("clipTableParts", {
    filter: ["tr", "thead", "tbody", "tfoot"],
    replacement: (content) => content,
  });

  // The GFM plugin converts a table only when its first row is a heading row
  // and `keep`s every other table as the page's own HTML (#45), and its cell
  // rules ignore `colspan` and `rowspan`, so a spanning table comes out as
  // rows of different widths. This rule takes every table that is carrying
  // data and writes it from the table's geometry: a header row synthesised
  // when the page has none — GFM requires one, and promoting a data row would
  // assert something the page does not — spans laid out on a rectangular grid,
  // and a nested table flattened into the cell that holds it. Rules added
  // later win, so this one is reached before both the plugin's rule and its
  // `keep`, and no table reaches the note as HTML either way.
  service.addRule("clipTable", {
    filter: "table",
    replacement: (content, node) => {
      // Not a grid: the rules above have already converted the cells as
      // blocks, so the content is the page's own structure, in order.
      if (!isDataTable(node)) return `\n\n${content.trim()}\n\n`;
      const grid = tableGrid(service, node);
      const caption = Array.from(node.children).find((child) => child.nodeName === "CAPTION");
      const title =
        caption === undefined ? "" : service.turndown(caption as HTMLElement).trim();
      if (insideGridCell(node)) {
        return [title, nestedTable(grid)].filter((part) => part !== "").join(": ");
      }
      // No rows, or rows with no cells in them: there is no table to write, and
      // a delimiter row of no columns is not one — goldmark reads the result as
      // a paragraph of pipes.
      if (grid.length === 0 || grid[0]!.length === 0) {
        return title === "" ? "" : `\n\n${title}\n\n`;
      }
      const rows = rowsOf(node);
      const headed = rows[0] !== undefined && isHeaderRow(rows[0]);
      // A synthesised header keeps the first row's alignments: the column is
      // the column whether or not the page wrote a heading over it.
      const header = headed
        ? grid[0]!
        : grid[0]!.map((cell) => ({ text: "", border: cell.border }) as Cell);
      const body = headed ? grid.slice(1) : grid;
      const table = [
        tableLine(header),
        `| ${header.map((cell) => cell.border).join(" | ")} |`,
        ...body.map(tableLine),
      ].join("\n");
      return `\n\n${[title, table].filter((part) => part !== "").join("\n\n")}\n\n`;
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
