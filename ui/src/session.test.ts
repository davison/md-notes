import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import {
  SAVE_DELAY,
  Session,
  flushAll,
  getSession,
  hasStoredDraft,
  resetSessions,
  unsavedSessions,
} from "./session";

/**
 * A fake daemon holding one note. GET returns the file; PUT checks the
 * revision, applies the save and mints a new revision. `fail` makes the
 * next PUT answer with an error instead, and `hold` parks responses until
 * `release` is called, so a save can be caught in flight.
 */
class Daemon {
  file: { source: string; revision: string } | null;
  fail: { status: number; code: string; error: string } | null = null;
  hold = false;
  private waiting: (() => void)[] = [];
  requests: { method: string; body?: { source: string; revision: string } }[] = [];
  private n = 1;

  constructor(source: string) {
    this.file = { source, revision: "r1" };
  }

  /** An external edit: new content and a new revision. */
  write(source: string) {
    this.n++;
    this.file = { source, revision: "r" + this.n };
  }

  /** An external touch: same content, new revision. */
  touch() {
    if (!this.file) return;
    this.n++;
    this.file = { ...this.file, revision: "r" + this.n };
  }

  release() {
    this.hold = false;
    const w = this.waiting;
    this.waiting = [];
    for (const fn of w) fn();
  }

  fetch = async (url: string, init?: RequestInit): Promise<Response> => {
    const method = init?.method ?? "GET";
    const req: { method: string; body?: { source: string; revision: string } } = { method };
    if (init?.body) req.body = JSON.parse(init.body as string);
    this.requests.push(req);
    if (this.hold) await new Promise<void>((resolve) => this.waiting.push(resolve));
    expect(url).toBe("/api/r/n/source/a.md");
    const json = (status: number, body: unknown) =>
      ({ ok: status < 300, status, statusText: "S", json: () => Promise.resolve(body) }) as Response;
    if (method === "GET") {
      if (!this.file) return json(404, { code: "not_found", error: "note or root no longer exists" });
      return json(200, this.file);
    }
    if (this.fail) {
      const f = this.fail;
      this.fail = null;
      return json(f.status, { code: f.code, error: f.error });
    }
    if (!this.file) return json(404, { code: "not_found", error: "note or root no longer exists" });
    if (req.body!.revision !== this.file.revision) {
      return json(409, { code: "conflict", error: "note changed; reload before saving" });
    }
    this.n++;
    this.file = { source: req.body!.source, revision: "r" + this.n };
    return json(200, this.file);
  };

  puts() {
    return this.requests.filter((r) => r.method === "PUT");
  }
}

let daemon: Daemon;

beforeEach(() => {
  vi.useFakeTimers();
  localStorage.clear();
  resetSessions();
  daemon = new Daemon("one\n");
  vi.stubGlobal("fetch", vi.fn(daemon.fetch));
});
afterEach(() => {
  vi.unstubAllGlobals();
  vi.useRealTimers();
});

async function settle() {
  // Let every pending promise in the fetch/response chain run.
  for (let i = 0; i < 10; i++) await Promise.resolve();
}

async function opened(): Promise<Session> {
  const s = getSession("n", "a.md");
  await s.open();
  expect(s.state.status).toBe("clean");
  return s;
}

describe("Session: reading", () => {
  it("reads the source and revision on open", async () => {
    const s = await opened();
    expect(s.state.base).toEqual({ source: "one\n", revision: "r1" });
    expect(s.state.draft).toBe("one\n");
    expect(s.opened).toBe(true);
  });

  it("reports a missing note as an error with nothing to keep", async () => {
    daemon.file = null;
    const s = getSession("n", "a.md");
    await s.open();
    expect(s.state.status).toBe("error");
    expect(s.state.error?.code).toBe("not_found");
    expect(unsavedSessions()).toEqual([]);
  });

  it("returns the same session for the same note", () => {
    expect(getSession("n", "a.md")).toBe(getSession("n", "a.md"));
    expect(getSession("n", "b.md")).not.toBe(getSession("n", "a.md"));
  });
});

describe("Session: autosave", () => {
  it("saves the draft after the quiet period, with the base revision", async () => {
    const s = await opened();
    s.edit("on");
    s.edit("one two\n");
    expect(s.state.status).toBe("pending");
    expect(daemon.puts()).toHaveLength(0);
    await vi.advanceTimersByTimeAsync(SAVE_DELAY - 1);
    expect(daemon.puts()).toHaveLength(0);
    await vi.advanceTimersByTimeAsync(1);
    await settle();
    expect(daemon.puts()).toEqual([{ method: "PUT", body: { source: "one two\n", revision: "r1" } }]);
    expect(s.state.status).toBe("clean");
    expect(s.state.base?.revision).toBe("r2");
    expect(daemon.file?.source).toBe("one two\n");
  });

  it("restarts the quiet period on every edit", async () => {
    const s = await opened();
    s.edit("a");
    await vi.advanceTimersByTimeAsync(SAVE_DELAY - 10);
    s.edit("ab");
    await vi.advanceTimersByTimeAsync(SAVE_DELAY - 10);
    expect(daemon.puts()).toHaveLength(0);
    await vi.advanceTimersByTimeAsync(10);
    await settle();
    expect(daemon.puts()).toHaveLength(1);
    expect(daemon.puts()[0].body?.source).toBe("ab");
  });

  it("goes back to clean when the draft returns to the file's content", async () => {
    const s = await opened();
    s.edit("x");
    s.edit("one\n");
    expect(s.state.status).toBe("clean");
    await vi.advanceTimersByTimeAsync(SAVE_DELAY);
    expect(daemon.puts()).toHaveLength(0);
  });

  it("flush saves now and passes keepalive through", async () => {
    const s = await opened();
    s.edit("now\n");
    await s.flush(true);
    expect(daemon.puts()).toHaveLength(1);
    const init = (fetch as unknown as ReturnType<typeof vi.fn>).mock.calls.at(-1)![1] as RequestInit;
    expect(init.keepalive).toBe(true);
    expect(s.state.status).toBe("clean");
  });

  it("saves again when the draft changed while a save was in flight", async () => {
    const s = await opened();
    s.edit("first");
    daemon.hold = true;
    const p = s.flush();
    expect(s.state.status).toBe("saving");
    s.edit("first second");
    expect(s.state.status).toBe("saving");
    daemon.release();
    await p;
    expect(s.state.status).toBe("pending");
    expect(s.state.base?.source).toBe("first");
    await vi.advanceTimersByTimeAsync(SAVE_DELAY);
    await settle();
    expect(daemon.puts().map((r) => r.body?.source)).toEqual(["first", "first second"]);
    expect(daemon.puts()[1].body?.revision).toBe("r2");
    expect(s.state.status).toBe("clean");
  });

  it("does not send a second save while one is in flight", async () => {
    const s = await opened();
    s.edit("a");
    daemon.hold = true;
    const p1 = s.flush();
    const p2 = s.flush();
    expect(p2).toBe(p1);
    daemon.release();
    await p1;
    expect(daemon.puts()).toHaveLength(1);
  });
});

describe("Session: failed saves", () => {
  it.each([
    [403, "permission_denied"],
    [403, "outside_root"],
    [413, "too_large"],
    [422, "unsupported_source"],
    [500, "io_error"],
  ])("keeps the draft and the reason on %i %s, and retries on request", async (status, code) => {
    const s = await opened();
    s.edit("kept\n");
    daemon.fail = { status, code, error: "refused: " + code };
    await s.flush();
    expect(s.state.status).toBe("failed");
    expect(s.state.error).toEqual({ code, message: "refused: " + code });
    expect(s.state.draft).toBe("kept\n");
    expect(daemon.file?.source).toBe("one\n");
    expect(unsavedSessions()).toEqual([s]);
    await s.retry();
    expect(s.state.status).toBe("clean");
    expect(daemon.file?.source).toBe("kept\n");
  });

  it("keeps the draft on a network failure", async () => {
    const s = await opened();
    s.edit("kept\n");
    vi.stubGlobal(
      "fetch",
      vi.fn(() => Promise.reject(new TypeError("Failed to fetch"))),
    );
    await s.flush();
    expect(s.state.status).toBe("failed");
    expect(s.state.error).toEqual({ code: "network", message: "Failed to fetch" });
    expect(s.state.draft).toBe("kept\n");
  });

  it("retries on the next edit after a failure", async () => {
    const s = await opened();
    s.edit("a");
    daemon.fail = { status: 500, code: "io_error", error: "disk" };
    await s.flush();
    expect(s.state.status).toBe("failed");
    s.edit("ab");
    expect(s.state.status).toBe("pending");
    await vi.advanceTimersByTimeAsync(SAVE_DELAY);
    await settle();
    expect(s.state.status).toBe("clean");
    expect(daemon.file?.source).toBe("ab");
  });

  it("does nothing for retry outside the failed state", async () => {
    const s = await opened();
    await s.retry();
    expect(daemon.puts()).toHaveLength(0);
  });
});

describe("Session: conflicts", () => {
  it("enters conflict on a stale save, keeping the draft and fetching the current file", async () => {
    const s = await opened();
    s.edit("mine\n");
    daemon.write("theirs\n");
    await s.flush();
    expect(s.state.status).toBe("conflict");
    expect(s.state.conflict).toEqual({ kind: "changed", current: { source: "theirs\n", revision: "r2" } });
    expect(s.state.draft).toBe("mine\n");
    expect(daemon.file?.source).toBe("theirs\n");
    // No autosave fires while the conflict stands.
    s.edit("mine more\n");
    await vi.advanceTimersByTimeAsync(SAVE_DELAY * 2);
    expect(daemon.puts()).toHaveLength(1);
    expect(s.state.status).toBe("conflict");
  });

  it("keepDraft saves the draft over the current file", async () => {
    const s = await opened();
    s.edit("mine\n");
    daemon.write("theirs\n");
    await s.flush();
    await s.keepDraft();
    expect(s.state.status).toBe("clean");
    expect(s.state.conflict).toBeNull();
    expect(daemon.file?.source).toBe("mine\n");
    expect(daemon.puts().at(-1)?.body?.revision).toBe("r2");
  });

  it("loadFile drops the draft and takes the file, bumping the generation", async () => {
    const s = await opened();
    const g = s.state.generation;
    s.edit("mine\n");
    daemon.write("theirs\n");
    await s.flush();
    s.loadFile();
    expect(s.state.status).toBe("clean");
    expect(s.state.draft).toBe("theirs\n");
    expect(s.state.base?.revision).toBe("r2");
    expect(s.state.generation).toBe(g + 1);
    expect(unsavedSessions()).toEqual([]);
  });

  it("a conflict can be kept even after the file changes again", async () => {
    const s = await opened();
    s.edit("mine\n");
    daemon.write("theirs\n");
    await s.flush();
    daemon.write("theirs again\n");
    await s.changed();
    expect(s.state.conflict?.current?.revision).toBe("r3");
    await s.keepDraft();
    expect(s.state.status).toBe("clean");
    expect(daemon.file?.source).toBe("mine\n");
  });

  it("enters a deleted conflict when the note vanishes under a save", async () => {
    const s = await opened();
    s.edit("mine\n");
    daemon.file = null;
    await s.flush();
    expect(s.state.status).toBe("conflict");
    expect(s.state.conflict).toEqual({ kind: "deleted", current: null });
    expect(s.state.draft).toBe("mine\n");
    // Nothing to keep the draft against, so keepDraft and loadFile are no-ops.
    await s.keepDraft();
    s.loadFile();
    expect(s.state.status).toBe("conflict");
  });

  it("a deleted conflict becomes a changed one when the file reappears", async () => {
    const s = await opened();
    s.edit("mine\n");
    daemon.file = null;
    await s.flush();
    daemon.write("back\n");
    await s.changed();
    expect(s.state.conflict).toEqual({ kind: "changed", current: { source: "back\n", revision: "r2" } });
    await s.keepDraft();
    expect(daemon.file).toMatchObject({ source: "mine\n" });
  });

  it("discard drops the draft of a deleted note", async () => {
    const s = await opened();
    s.edit("mine\n");
    daemon.file = null;
    await s.flush();
    s.discard();
    expect(s.state.status).toBe("error");
    expect(s.state.draft).toBe("");
    expect(unsavedSessions()).toEqual([]);
    expect(hasStoredDraft("n", "a.md")).toBe(false);
    // The note coming back is picked up cleanly.
    daemon.write("back\n");
    await s.changed();
    expect(s.state.status).toBe("clean");
    expect(s.state.draft).toBe("back\n");
  });

  it("discard on a changed conflict loads the file", async () => {
    const s = await opened();
    s.edit("mine\n");
    daemon.write("theirs\n");
    await s.flush();
    s.discard();
    expect(s.state.status).toBe("clean");
    expect(s.state.draft).toBe("theirs\n");
  });
});

describe("Session: external changes", () => {
  it("a clean draft follows the file", async () => {
    const s = await opened();
    const g = s.state.generation;
    daemon.write("edited elsewhere\n");
    await s.changed();
    expect(s.state.status).toBe("clean");
    expect(s.state.draft).toBe("edited elsewhere\n");
    expect(s.state.base?.revision).toBe("r2");
    expect(s.state.generation).toBe(g + 1);
  });

  it("a dirty draft enters conflict and its autosave is cancelled", async () => {
    const s = await opened();
    s.edit("mine\n");
    daemon.write("theirs\n");
    await s.changed();
    expect(s.state.status).toBe("conflict");
    expect(s.state.conflict?.current?.source).toBe("theirs\n");
    await vi.advanceTimersByTimeAsync(SAVE_DELAY * 2);
    expect(daemon.puts()).toHaveLength(0);
  });

  it("a failed draft enters conflict when the file changes", async () => {
    const s = await opened();
    s.edit("mine\n");
    daemon.fail = { status: 500, code: "io_error", error: "disk" };
    await s.flush();
    daemon.write("theirs\n");
    await s.changed();
    expect(s.state.status).toBe("conflict");
    expect(s.state.draft).toBe("mine\n");
  });

  it("a deleted file under a clean draft keeps the text as a deleted conflict", async () => {
    const s = await opened();
    daemon.file = null;
    await s.changed();
    expect(s.state.status).toBe("conflict");
    expect(s.state.conflict?.kind).toBe("deleted");
    expect(s.state.draft).toBe("one\n");
  });

  it("the same revision changes nothing", async () => {
    const s = await opened();
    s.edit("mine\n");
    await s.changed();
    expect(s.state.status).toBe("pending");
    expect(s.state.draft).toBe("mine\n");
  });

  it("a new revision with the same content is adopted without a conflict", async () => {
    const s = await opened();
    s.edit("mine\n");
    daemon.touch();
    await s.changed();
    expect(s.state.status).toBe("pending");
    expect(s.state.base?.revision).toBe("r2");
    await vi.advanceTimersByTimeAsync(SAVE_DELAY);
    await settle();
    expect(s.state.status).toBe("clean");
    expect(daemon.puts()[0].body?.revision).toBe("r2");
  });

  it("a save refused for a touch is sent again once against the new revision", async () => {
    const s = await opened();
    s.edit("mine\n");
    daemon.touch();
    await s.flush();
    expect(daemon.puts().map((r) => r.body?.revision)).toEqual(["r1", "r2"]);
    expect(s.state.status).toBe("clean");
    expect(daemon.file?.source).toBe("mine\n");
  });

  it("a fresh edit after a refused resend allows one more resend", async () => {
    const s = await opened();
    s.edit("mine\n");
    daemon.touch();
    const orig = daemon.fetch;
    vi.stubGlobal(
      "fetch",
      vi.fn((url: string, init?: RequestInit) => {
        if (init?.method === "PUT" && daemon.puts().length === 1) daemon.touch();
        return orig(url, init);
      }),
    );
    await s.flush();
    expect(s.state.status).toBe("failed");
    vi.stubGlobal("fetch", vi.fn(orig));
    s.edit("mine again\n");
    daemon.touch();
    await s.flush();
    expect(s.state.status).toBe("clean");
    expect(daemon.file?.source).toBe("mine again\n");
  });

  it("a save refused twice for touches stays failed rather than looping", async () => {
    const s = await opened();
    s.edit("mine\n");
    daemon.touch();
    const orig = daemon.fetch;
    // Touch again the moment the second attempt arrives.
    vi.stubGlobal(
      "fetch",
      vi.fn((url: string, init?: RequestInit) => {
        if (init?.method === "PUT" && daemon.puts().length === 1) daemon.touch();
        return orig(url, init);
      }),
    );
    await s.flush();
    expect(daemon.puts()).toHaveLength(2);
    expect(s.state.status).toBe("failed");
    expect(s.state.error?.code).toBe("conflict");
    expect(s.state.draft).toBe("mine\n");
    await s.retry();
    expect(s.state.status).toBe("clean");
  });

  it("a change reported during a save is checked after the response, and our own save is recognised", async () => {
    const s = await opened();
    s.edit("mine\n");
    daemon.hold = true;
    const p = s.flush();
    const c = s.changed();
    expect(daemon.requests.filter((r) => r.method === "GET")).toHaveLength(1);
    daemon.release();
    await p;
    await c;
    await settle();
    expect(daemon.requests.filter((r) => r.method === "GET")).toHaveLength(2);
    expect(s.state.status).toBe("clean");
    expect(s.state.base?.revision).toBe("r2");
  });

  it("a draft that matches the file is simply clean", async () => {
    const s = await opened();
    s.edit("same\n");
    daemon.write("same\n");
    await s.changed();
    expect(s.state.status).toBe("clean");
    expect(s.state.base?.revision).toBe("r2");
  });

  it("an unopened session ignores change reports", async () => {
    const s = getSession("n", "a.md");
    await s.changed();
    expect(daemon.requests).toHaveLength(0);
  });

  it("a transient read failure leaves the draft alone", async () => {
    const s = await opened();
    s.edit("mine\n");
    vi.stubGlobal(
      "fetch",
      vi.fn(() => Promise.reject(new TypeError("Failed to fetch"))),
    );
    await s.changed();
    expect(s.state.status).toBe("pending");
    expect(s.state.draft).toBe("mine\n");
  });
});

describe("Session: reopening", () => {
  it("rechecks the file when opened again after a change while away", async () => {
    const s = await opened();
    daemon.write("changed while away\n");
    await s.open();
    expect(s.state.draft).toBe("changed while away\n");
  });

  it("an open during a save waits for the save rather than conflicting with it", async () => {
    const s = await opened();
    s.edit("mine\n");
    daemon.hold = true;
    const p = s.flush();
    const o = s.open();
    daemon.release();
    await p;
    await o;
    await settle();
    expect(s.state.status).toBe("clean");
  });
});

describe("Session: stored drafts", () => {
  it("mirrors a dirty draft to storage and clears it once saved", async () => {
    const s = await opened();
    s.edit("mine\n");
    expect(hasStoredDraft("n", "a.md")).toBe(true);
    expect(JSON.parse(localStorage.getItem("mdn:draft:n\0a.md")!)).toEqual({ revision: "r1", draft: "mine\n" });
    await s.flush();
    expect(hasStoredDraft("n", "a.md")).toBe(false);
  });

  it("recovers a stored draft made against the current revision as pending", async () => {
    localStorage.setItem("mdn:draft:n\0a.md", JSON.stringify({ revision: "r1", draft: "recovered\n" }));
    const s = getSession("n", "a.md");
    await s.open();
    expect(s.state.status).toBe("pending");
    expect(s.state.draft).toBe("recovered\n");
    await vi.advanceTimersByTimeAsync(SAVE_DELAY);
    await settle();
    expect(daemon.file?.source).toBe("recovered\n");
  });

  it("recovers a stored draft made against an older revision as a conflict", async () => {
    localStorage.setItem("mdn:draft:n\0a.md", JSON.stringify({ revision: "r0", draft: "recovered\n" }));
    const s = getSession("n", "a.md");
    await s.open();
    expect(s.state.status).toBe("conflict");
    expect(s.state.conflict?.current?.source).toBe("one\n");
    expect(s.state.draft).toBe("recovered\n");
  });

  it("a stored draft that already landed is not a conflict", async () => {
    localStorage.setItem("mdn:draft:n\0a.md", JSON.stringify({ revision: "r0", draft: "one\n" }));
    const s = getSession("n", "a.md");
    await s.open();
    expect(s.state.status).toBe("clean");
    expect(hasStoredDraft("n", "a.md")).toBe(false);
  });

  it("a stored draft whose note is gone is kept as a deleted conflict", async () => {
    localStorage.setItem("mdn:draft:n\0a.md", JSON.stringify({ revision: "r1", draft: "recovered\n" }));
    daemon.file = null;
    const s = getSession("n", "a.md");
    await s.open();
    expect(s.state.status).toBe("conflict");
    expect(s.state.conflict?.kind).toBe("deleted");
    expect(s.state.draft).toBe("recovered\n");
  });

  it("another tab reaching clean leaves this tab's stored draft alone", async () => {
    // Tab B opens the note with nothing in storage; tab A then stores a draft.
    const b = await opened();
    const recordA = JSON.stringify({ revision: "r1", draft: "tab A draft\n" });
    localStorage.setItem("mdn:draft:n\0a.md", recordA);
    // Tab B settling to clean again does not remove a record it did not write.
    daemon.touch();
    await b.changed();
    expect(b.state.status).toBe("clean");
    expect(localStorage.getItem("mdn:draft:n\0a.md")).toBe(recordA);
    // Tab B's own record is written over the shared slot and cleared by its save.
    b.edit("tab B\n");
    await b.flush();
    expect(localStorage.getItem("mdn:draft:n\0a.md")).toBeNull();
  });

  it("a session clears a record it recovered once the draft lands", async () => {
    localStorage.setItem("mdn:draft:n\0a.md", JSON.stringify({ revision: "r1", draft: "recovered\n" }));
    const s = getSession("n", "a.md");
    await s.open();
    await s.flush();
    expect(s.state.status).toBe("clean");
    expect(hasStoredDraft("n", "a.md")).toBe(false);
  });

  it("ignores corrupt storage", async () => {
    localStorage.setItem("mdn:draft:n\0a.md", "{not json");
    expect(hasStoredDraft("n", "a.md")).toBe(false);
    const s = getSession("n", "a.md");
    await s.open();
    expect(s.state.status).toBe("clean");
  });
});

describe("flushAll", () => {
  it("saves every pending session", async () => {
    const a = await opened();
    a.edit("a\n");
    await flushAll();
    expect(a.state.status).toBe("clean");
  });
});
