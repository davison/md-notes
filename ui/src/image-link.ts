/**
 * The markdown the editor writes for a pasted or dropped image. The file is
 * in the root's one `_resources` directory, so the link climbs from the
 * note's folder to the root: a plain relative path, with no scheme and no
 * leading slash, which the reading view, Markor and any other markdown
 * reader all resolve the same way.
 */
import { encodePath } from "./api";

/** The link from the note at `notePath` to `resourcePath`, both root-relative. */
export function resourceLink(notePath: string, resourcePath: string): string {
  const depth = notePath.split("/").length - 1;
  // encodeURIComponent leaves parentheses alone, and a bare ")" would end
  // the link early.
  return "../".repeat(depth) + encodePath(resourcePath).replace(/[()]/g, (c) => (c === "(" ? "%28" : "%29"));
}

/**
 * The alt text for a dropped file: its base name without the extension, on
 * one line, with the characters that would end the text early escaped. A
 * paste has no name worth keeping and gets empty alt text instead.
 */
export function altFor(fileName: string): string {
  const dot = fileName.lastIndexOf(".");
  const base = dot > 0 ? fileName.slice(0, dot) : fileName;
  return base.replace(/[\r\n]+/g, " ").replace(/[[\]\\]/g, (c) => "\\" + c);
}

export function imageMarkdown(alt: string, link: string): string {
  return `![${alt}](${link})`;
}

/**
 * Whether the editor takes this file as an image. The browser's type is
 * only the first look: the daemon decides by the file's content, and
 * refuses anything this lets through that is not one.
 */
export function isImage(file: File): boolean {
  return file.type.startsWith("image/");
}
