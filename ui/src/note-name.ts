/**
 * The rule that turns what a person typed into the path of a new note.
 *
 * The daemon takes a full path under the root and refuses every unsafe or
 * unusable name it is handed — empty, hidden, a control character, not
 * markdown, outside the root, already taken — so this side owns one thing
 * only: composing a path out of a title and the folder the navigator is
 * showing. Everything it cannot decide locally is sent and the daemon's
 * refusal is shown in the prompt, which keeps one set of rules rather than
 * two that can disagree.
 */

/** The extensions the daemon accepts, and the one this adds. */
const MARKDOWN = /\.(md|markdown)$/i;

export type NewNote = { path: string; error?: undefined } | { path?: undefined; error: string };

/** The folder a note lives in: "docs" for "docs/a.md", "" for "a.md". */
export function folderOf(notePath: string): string {
  const cut = notePath.lastIndexOf("/");
  return cut < 0 ? "" : notePath.slice(0, cut);
}

/**
 * The path to create from what was typed and the navigator's selected
 * folder, or the one refusal worth making here: a name with nothing in it
 * to name a file with.
 *
 * A name containing "/" is a path under the root and ignores the folder;
 * any other name is joined to the folder. A final component that is not
 * already markdown gains ".md". Leading slashes are dropped so "/a/b" and
 * "a/b" mean the same note rather than one the daemon refuses.
 */
export function newNotePath(typed: string, folder: string): NewNote {
  const name = typed.trim();
  if (name === "") return { error: "Enter a name for the note." };
  const relative = name.includes("/") ? name.replace(/^\/+/, "") : join(folder, name);
  const last = relative.slice(relative.lastIndexOf("/") + 1);
  if (last === "") return { error: "Enter a name for the note, not only a folder." };
  return { path: MARKDOWN.test(last) ? relative : relative + ".md" };
}

function join(folder: string, name: string): string {
  return folder === "" ? name : folder + "/" + name;
}
