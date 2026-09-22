// Package diagram draws a mermaid flowchart as an SVG image, on the server,
// from a typed model (davison/md-notes#169, #170).
//
// Mermaid-js itself is not used. Its advisories include critical XSS under
// its strictest setting, and in this application any script on the page's
// origin can read and write every note. So the diagram is parsed here into
// plain data, laid out here, and written as an SVG built only from
// elements this package writes, with all source text escaped. The page
// shows it through <img>, so even that SVG cannot run anything.
//
// # The subset
//
// A block is drawn when it is a `flowchart` or `graph` with an optional
// direction (TB, TD, BT, LR, RL) and uses only:
//
//   - nodes, bare (`A`) or shaped: `[rect]`, `(round)`, `([stadium])`,
//     `[[subroutine]]`, `[(cylinder)]`, `((circle))`, `(((double circle)))`,
//     `>asymmetric]`, `{rhombus}`, `{{hexagon}}`, `[/parallelogram/]`,
//     `[\parallelogram\]`, `[/trapezoid\]`, `[\trapezoid/]`;
//   - links: solid `---` `-->`, dotted `-.-` `-.->`, thick `===` `==>`,
//     invisible `~~~`; circle and cross ends (`--o`, `--x`); both ends
//     (`<-->`, `o--o`, `x--x`); longer links (`--->`, `-..->`) up to
//     Limits.MaxLength; with a label as `-- text -->` or `-->|text|`;
//   - chains (`A --> B --> C`) and groups (`A & B --> C & D`);
//   - `subgraph id`, `subgraph id [Title]`, `subgraph "Title"`, nested,
//     closed by `end`, and links to and from a subgraph; a `direction`
//     inside one is accepted and not honoured;
//   - labels, quoted or not, with `<br>` for a line break and mermaid's
//     entity codes (`#quot;`, `#35;`);
//   - `%%` comments.
//
// The styling and interaction statements `style`, `classDef`, `class`,
// `:::class`, `linkStyle` and `click` are skipped whole, up to their
// newline or semicolon: colour is the theme's, and an image has nothing to
// click. Bidi controls and zero-width characters are dropped from labels.
// A block with nothing to draw is refused. Everything else — `%%{…}%%`
// directives, front matter, `@{…}` metadata, markdown or HTML labels
// (except `<br>`), icons, accessibility statements, other diagram types,
// and any text that does not parse — is refused with a *Refusal, and the
// caller shows the block as code. Nothing is ever half-drawn.
//
// # Drawing
//
// The layout is layered, as mermaid's is. A link back against the flow
// runs in its own straight lane outside the nodes, leaving and entering
// them by the side that faces it; self-loops go on the other side. The
// longest chains of links are drawn straight where the layout allows. A
// subgraph's title is placed where no link crosses it. In TB and BT, if
// there is no such place, the subgraph is widened by the room the title
// lacks, at most twice; otherwise the title stays centred
// (davison/md-notes#188).
//
// # Bounds
//
// DefaultLimits bound the input's size, its node, edge and subgraph
// counts, its nesting, link lengths, label sizes, the size of the working
// graph the layout builds, and the time the whole render may take.
//
// # Text
//
// Labels are measured with the advance widths of Noto Sans Regular, taken
// from the font by genmetrics into metrics.go. Only the numbers are used;
// no glyph data is included.
package diagram
