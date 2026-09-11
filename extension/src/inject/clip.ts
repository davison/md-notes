/**
 * The half of the clipper that runs in the page.
 *
 * It is built as its own IIFE bundle (`clip-inject.js`) because
 * `chrome.scripting.executeScript({files})` injects a *classic* script, and
 * because Readability and Turndown are far too large to travel inside a
 * serialized `func`. It runs in the isolated world, so the page's own scripts
 * and CSP neither see it nor constrain it.
 *
 * It publishes one function and does nothing else; the worker calls it.
 */

import { Readability } from "@mozilla/readability";
import { htmlToMarkdown } from "../markdown";
import { CLIP_ENTRY_POINT, type ClipKind, type ExtractionResult } from "../extraction";

/**
 * The readable article, as HTML, or null when Readability declines the page.
 * It is given a clone: `parse()` rewrites the document it is handed, and the
 * page the user is looking at must survive being clipped.
 */
function readableHtml(): { html: string; title: string } | null {
  try {
    // `keepClasses` because Readability strips every class by default, and a
    // fenced code block's language lives in one (`language-go`).
    const article = new Readability(document.cloneNode(true) as Document, {
      keepClasses: true,
    }).parse();
    const html = article?.content ?? "";
    if (html.trim() === "") return null;
    return { html, title: (article?.title ?? "").trim() };
  } catch {
    // Readability throws on documents it cannot walk at all.
    return null;
  }
}

function clipPage(): ExtractionResult {
  const readable = readableHtml();
  // A page Readability declines — a dashboard, a search result, a wiki index —
  // is still worth clipping: the whole body converts to something a person can
  // read and edit, which beats refusing.
  const html = readable?.html ?? document.body?.innerHTML ?? "";
  const markdown = htmlToMarkdown(html, location.href);
  if (markdown === "") return { error: "there is no text on this page to clip" };
  return {
    kind: "page",
    url: location.href,
    title: readable?.title !== undefined && readable.title !== "" ? readable.title : document.title,
    markdown,
  };
}

function clipSelection(): ExtractionResult {
  const selection = window.getSelection();
  if (selection === null || selection.rangeCount === 0 || selection.isCollapsed) {
    return { error: "nothing is selected on this page" };
  }
  const container = document.createElement("div");
  for (let i = 0; i < selection.rangeCount; i++) {
    container.appendChild(selection.getRangeAt(i).cloneContents());
  }
  const markdown = htmlToMarkdown(container.innerHTML, location.href);
  if (markdown === "") return { error: "the selection has no text in it" };
  return { kind: "selection", url: location.href, title: document.title, markdown };
}

function clip(kind: ClipKind): ExtractionResult {
  return kind === "selection" ? clipSelection() : clipPage();
}

(globalThis as unknown as Record<string, unknown>)[CLIP_ENTRY_POINT] = clip;
