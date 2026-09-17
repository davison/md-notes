/**
 * Deciding what a `file:` navigation should become. Kept free of the `chrome`
 * APIs so it can be tested directly.
 */

import { DaemonError, listRoots, registerRoot, type FailureKind } from "./daemon";
import { localMarkdownFile, noteUrl, relativeTo, rootContaining, type LocalFile, type Root } from "./paths";
import { badHostMessage, registerRefusedRemotely } from "./reach";
import type { Settings } from "./settings";

/** What the background worker should do about a navigation. */
export type OpenResult =
  | { status: "ignored" }
  | { status: "open"; url: string; slug: string; note: string }
  | { status: "failed"; kind: FailureKind; message: string };

/** The daemon calls the decision needs; injected so the tests need no network. */
export interface RootsApi {
  listRoots(settings: Settings): Promise<Root[]>;
  registerRoot(settings: Settings, dir: string, file: string): Promise<Root>;
}

export const liveRootsApi: RootsApi = {
  listRoots: (settings) => listRoots(settings),
  registerRoot: (settings, dir, file) => registerRoot(settings, dir, file),
};

/**
 * Where a `file:` URL should open in the app.
 *
 * A file already inside a registered root opens under that root. Otherwise the
 * file's directory is registered as a new root first — which is also how a
 * symlinked alias of an already registered directory finds its root, since the
 * daemon compares real paths and hands back the existing one.
 *
 * The registration names the file, and the daemon registers nothing unless
 * the file is there. So a `file:` URL for a note that does not exist leaves
 * the roots listing exactly as it was, and the tab is left where it is with
 * the refusal on the badge (M7-R1, #50).
 */
export async function resolveOpen(
  url: string,
  settings: Settings,
  api: RootsApi = liveRootsApi,
): Promise<OpenResult> {
  const file = localMarkdownFile(url);
  if (file === null) return { status: "ignored" };

  // Which of the two calls threw decides what a refusal of the *endpoint*
  // means, so each is caught where it is made rather than together at the
  // end. Misnaming which call was refused is the failure class this whole
  // module is here to stop producing.
  let roots: Root[];
  try {
    roots = await api.listRoots(settings);
  } catch (err) {
    return describeOpenFailure(err, settings, "list", file);
  }

  const existing = rootContaining(roots, file.path);
  if (existing !== null) {
    const note = relativeTo(existing.path, file.path);
    return {
      status: "open",
      url: noteUrl(settings.daemonUrl, existing.slug, note),
      slug: existing.slug,
      note,
    };
  }

  try {
    const added = await api.registerRoot(settings, file.dir, file.name);
    const note = relativeTo(added.path, file.path);
    return {
      status: "open",
      url: noteUrl(settings.daemonUrl, added.slug, note),
      slug: added.slug,
      note,
    };
  } catch (err) {
    return describeOpenFailure(err, settings, "register", file);
  }
}

/** Which daemon call was refused, so the message can name it. */
type Step = "list" | "register";

/**
 * A failed attempt, in terms that name what actually refused it.
 *
 * The two refusals a tailnet daemon URL produces are the ones worth
 * translating. `loopback_only` is the allow-list refusing an endpoint under
 * the tailnet name, and it is not a token problem however much the previous
 * wording implied one; `bad_host` is the daemon not answering to the address
 * at all, usually a `tailnet_host` missing its port. Everything else keeps the
 * daemon's own words.
 *
 * The allow-list today permits `GET /api/roots` and refuses `POST /api/roots`,
 * so a `loopback_only` on the listing cannot happen — but saying "registering
 * a folder is refused" about a *listing* that was refused would be the same
 * misattribution in a new coat, so the step decides the sentence and the
 * daemon's own words are what a refused listing gets.
 *
 * `not_found` is the third of the same family and the one #50 is about: the
 * daemon looked for the note and it was not there, so nothing was
 * registered. The sentence names the file, because the file is what the
 * person can do something about.
 */
function describeOpenFailure(
  err: unknown,
  settings: Settings,
  step: Step,
  file: LocalFile,
): OpenResult & { status: "failed" } {
  if (!(err instanceof DaemonError)) {
    return { status: "failed", kind: "bad_response", message: String(err) };
  }
  switch (err.kind) {
    case "not_found":
      return {
        status: "failed",
        kind: err.kind,
        message: missingNoteMessage(file, step, err.detail),
      };
    case "loopback_only":
      return {
        status: "failed",
        kind: err.kind,
        message:
          step === "register" ? registerRefusedRemotely(settings.daemonUrl) : err.message,
      };
    case "bad_host":
      return { status: "failed", kind: err.kind, message: badHostMessage(settings.daemonUrl, err.detail) };
    default:
      return { status: "failed", kind: err.kind, message: err.message };
  }
}

/**
 * The refusal in the terms the person reading the badge is in: there is no
 * such note, and nothing was added to md-notes because of it. Only a
 * registration can earn this today — a listing has no file in it to be
 * missing — but the step still decides the sentence, so the day something
 * else answers `not_found` it is not described as this.
 */
function missingNoteMessage(file: LocalFile, step: Step, detail: string): string {
  if (step !== "register") return detail;
  return (
    `No such note: the daemon cannot find ${file.path}, so ${file.dir} was not ` +
    "registered as a root. Nothing was added to md-notes; create the file and reload."
  );
}
