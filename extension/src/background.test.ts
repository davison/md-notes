/**
 * The service worker's own half of the file-URL intercept: what the badge,
 * the title and the record the popup reads say after one navigation.
 *
 * `handleNavigation` is the worker's behaviour without Chromium's event
 * plumbing, and the `chrome` API it uses is stubbed here, so the daemon's
 * answers are the only input. The end-to-end suite drives the same path
 * through a real browser; this is where the wording lives.
 */

import { beforeEach, describe, expect, it, vi } from "vitest";

/** The slice of `chrome` the worker touches, recorded rather than performed. */
function stubChrome() {
  const session: Record<string, unknown> = {};
  const local: Record<string, unknown> = {};
  const area = (items: Record<string, unknown>) => ({
    // `null` is the whole area, as in chrome.storage.
    get: async (keys: string[] | null) => {
      const out: Record<string, unknown> = {};
      for (const k of keys ?? Object.keys(items)) if (k in items) out[k] = items[k];
      return out;
    },
    set: async (next: Record<string, unknown>) => {
      Object.assign(items, next);
    },
    remove: async (keys: string[]) => {
      for (const k of keys) delete items[k];
    },
  });
  const badge = { text: new Map<number, string>(), title: new Map<number, string>(), colour: "" };
  const updated: { tabId: number; url: string }[] = [];
  const listener = { addListener: (_fn: unknown) => undefined };
  /** The navigation listeners, kept so a test can deliver an event. */
  type OnUpdated = (tabId: number, change: { status?: string; url?: string }, tab: { url?: string }) => void;
  const onUpdated: OnUpdated[] = [];
  const chrome = {
    storage: { session: area(session), local: area(local) },
    action: {
      setBadgeBackgroundColor: async (o: { color: string }) => {
        badge.colour = o.color;
      },
      setBadgeText: async (o: { tabId: number; text: string }) => {
        badge.text.set(o.tabId, o.text);
      },
      setTitle: async (o: { tabId: number; title: string }) => {
        badge.title.set(o.tabId, o.title);
      },
    },
    tabs: {
      update: async (tabId: number, o: { url: string }) => {
        updated.push({ tabId, url: o.url });
      },
      onUpdated: { addListener: (fn: OnUpdated) => void onUpdated.push(fn) },
      onRemoved: listener,
    },
    runtime: { onInstalled: listener, onStartup: listener, onMessage: listener },
    contextMenus: { onClicked: listener, removeAll: async () => undefined, create: () => undefined },
  };
  return { chrome, session, local, badge, updated, onUpdated };
}

const stub = stubChrome();
vi.stubGlobal("chrome", stub.chrome);

// Imported after the stub: the module registers its listeners as it loads.
const { handleNavigation } = await import("./background");

/** A daemon that serves /home/you/notes and verifies before registering. */
function daemon(files: string[]) {
  return vi.fn(async (input: string | URL | Request, init?: RequestInit) => {
    const url = String(input);
    if (url.endsWith("/api/roots") && (init?.method ?? "GET") === "GET") {
      return json(200, { roots: [{ slug: "notes", path: "/home/you/notes", kind: "notes" }] });
    }
    if (url.endsWith("/api/roots")) {
      const body = JSON.parse(String(init?.body)) as { path: string; file?: string };
      if (!files.includes(`${body.path}/${body.file ?? ""}`)) {
        return json(404, {
          code: "not_found",
          error: `no such note under that folder: ${body.file ?? ""}`,
        });
      }
      const slug = body.path.slice(body.path.lastIndexOf("/") + 1);
      return json(200, { slug, path: body.path, kind: "recent" });
    }
    throw new Error(`unexpected call to ${url}`);
  }) as unknown as typeof fetch;
}

function json(status: number, body: unknown): Response {
  return { ok: status < 400, status, statusText: "", json: async () => body } as Response;
}

beforeEach(() => {
  for (const k of Object.keys(stub.session)) delete stub.session[k];
  stub.badge.text.clear();
  stub.badge.title.clear();
  stub.updated.length = 0;
  stub.local.token = "s3cret";
});

describe("the file-URL intercept in the worker", () => {
  it("badges the tab with the refusal by name when the note is not there", async () => {
    vi.stubGlobal("fetch", daemon([]));
    const result = await handleNavigation(4, "file:///tmp/scratch/no-such-note.md");

    expect(result).toMatchObject({ status: "failed", kind: "not_found" });
    // The tab is left where it is: nothing was registered, so there is
    // nowhere to send it (#50).
    expect(stub.updated).toEqual([]);
    expect(stub.badge.text.get(4)).toBe("!");
    const title = stub.badge.title.get(4) ?? "";
    expect(title).toMatch(/no such note/i);
    expect(title).toContain("/tmp/scratch/no-such-note.md");
    expect(title).not.toMatch(/token/i);
    // and the record the popup reads says the same thing.
    const record = stub.session["status:4"] as { kind: string; message: string };
    expect(record.kind).toBe("not_found");
    expect(record.message).toMatch(/no such note/i);
  });

  it("opens a note that is there, with no badge", async () => {
    vi.stubGlobal("fetch", daemon(["/tmp/scratch/todo.md"]));
    const result = await handleNavigation(5, "file:///tmp/scratch/todo.md");

    expect(result).toMatchObject({ status: "open", slug: "scratch", note: "todo.md" });
    expect(stub.updated).toEqual([{ tabId: 5, url: "http://localhost:7337/r/scratch/todo.md" }]);
    expect(stub.badge.text.get(5)).toBe("");
    expect((stub.session["status:5"] as { kind: string }).kind).toBe("opened");
  });
});

describe("a tab that moves on to a local file that is not a note", () => {
  // `file:` URLs are delivered to the worker whatever they name, and one the
  // intercept ignores — a text file, a directory listing — is still the tab
  // moving on from the note its record names.
  it("drops the record the last note left", async () => {
    vi.stubGlobal("fetch", daemon([]));
    await handleNavigation(6, "file:///tmp/scratch/no-such-note.md");
    expect(stub.session["status:6"]).toBeDefined();

    const result = await handleNavigation(6, "file:///tmp/scratch/list.txt");
    expect(result).toMatchObject({ status: "ignored" });
    expect(stub.session["status:6"]).toBeUndefined();
    expect(stub.badge.text.get(6)).toBe("");
  });
});

describe("a worker that was stopped between the record and the navigation", () => {
  // Chromium stops an idle MV3 worker after about thirty seconds, and the
  // next event starts a fresh one whose in-memory state is empty. The record
  // the popup reads is in session storage and outlives it; it carries the
  // `file:` URL the tab was showing, so it must not outlive the tab moving on
  // (the review of PR #203, finding 1).
  it("still drops the record, and the badge, when the tab moves on", async () => {
    stub.session["status:9"] = {
      kind: "unreachable",
      message: "daemon not reachable at http://localhost:7337",
      source: "file:///home/you/elsewhere/a.md",
      at: 1,
    };
    stub.badge.text.set(9, "!");

    // A fresh module is a fresh worker.
    vi.resetModules();
    stub.onUpdated.length = 0;
    await import("./background");
    expect(stub.onUpdated).toHaveLength(1);

    // A `loading` with no URL: an origin outside the extension's permissions,
    // which is to say the ordinary web.
    stub.onUpdated[0](9, { status: "loading" }, {});
    await vi.waitFor(() => expect(stub.session["status:9"]).toBeUndefined());
    expect(stub.badge.text.get(9)).toBe("");
  });

  it("leaves another tab's record alone", async () => {
    stub.session["status:9"] = { kind: "unreachable", message: "m", source: "file:///a.md", at: 1 };
    stub.session["status:10"] = { kind: "opened", message: "m", source: "file:///b.md", at: 1 };
    vi.resetModules();
    stub.onUpdated.length = 0;
    await import("./background");

    stub.onUpdated[0](9, { status: "loading" }, {});
    await vi.waitFor(() => expect(stub.session["status:9"]).toBeUndefined());
    expect(stub.session["status:10"]).toBeDefined();
  });
});
