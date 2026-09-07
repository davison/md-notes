import { fetchSource, saveSource, SourceError, type Source } from "./api";

/**
 * An editing session for one note: the last source known to match the
 * file on disk, the draft being edited, and where the two stand.
 *
 * Sessions live outside the component tree so a draft survives a mode
 * switch or navigating to another note and back, and every session with
 * unsaved work is known to the unload guard. Dirty drafts are mirrored to
 * localStorage, best effort, so a reload recovers them too.
 *
 *   loading  the first read has not completed
 *   error    the first read failed; there is no draft to keep
 *   clean    the draft matches the file
 *   pending  the draft differs and a save is scheduled
 *   saving   a save is in flight
 *   failed   the last save was refused; the draft is kept and can be retried
 *   conflict the file changed or vanished under a draft; the draft is kept
 *            until the conflict is resolved one way or the other
 */
export type Status = "loading" | "error" | "clean" | "pending" | "saving" | "failed" | "conflict";

export interface Conflict {
  kind: "changed" | "deleted";
  /** The note as it is on disk now, or null when it no longer exists. */
  current: Source | null;
}

export interface Failure {
  code: string;
  message: string;
}

export interface SessionState {
  status: Status;
  /** The source and revision the draft was made against; null before the first read. */
  base: Source | null;
  draft: string;
  /** Why the last read or save failed. */
  error: Failure | null;
  conflict: Conflict | null;
  /**
   * Bumped whenever the draft is replaced from outside the editor (a
   * refresh from disk, a conflict resolution), so the editor knows to
   * reset its document rather than treat the change as typing.
   */
  generation: number;
}

/** Milliseconds of quiet after the last edit before a save is sent. */
export const SAVE_DELAY = 1000;

interface Stored {
  revision: string;
  draft: string;
}

function storageKey(key: string) {
  return "mdn:draft:" + key;
}

function readStored(key: string): Stored | null {
  try {
    const raw = localStorage.getItem(storageKey(key));
    if (!raw) return null;
    const v = JSON.parse(raw) as Stored;
    if (typeof v.draft !== "string" || typeof v.revision !== "string") return null;
    return v;
  } catch {
    return null;
  }
}

function writeStored(key: string, v: Stored | null) {
  try {
    if (v) localStorage.setItem(storageKey(key), JSON.stringify(v));
    else localStorage.removeItem(storageKey(key));
  } catch {
    // storage unavailable or full: the in-memory draft still stands
  }
}

/** Whether a session holds work that would be lost if it were dropped. */
export function hasUnsaved(s: SessionState): boolean {
  return s.status === "pending" || s.status === "saving" || s.status === "failed" || s.status === "conflict";
}

export class Session {
  readonly key: string;
  state: SessionState = { status: "loading", base: null, draft: "", error: null, conflict: null, generation: 0 };
  /** The editor's own state (cursor, history), parked here between mounts. */
  editorState: unknown = null;
  editorGeneration = -1;

  private listeners = new Set<() => void>();
  private timer: ReturnType<typeof setTimeout> | null = null;
  private inflight: Promise<void> | null = null;
  private sent = "";
  private recheckAfterSave = false;
  private resent = false;
  private recovered = false;
  private stored: Stored | null = null;
  private opening: Promise<void> | null = null;

  constructor(
    readonly slug: string,
    readonly path: string,
  ) {
    this.key = slug + "\0" + path;
  }

  subscribe(fn: () => void): () => void {
    this.listeners.add(fn);
    return () => this.listeners.delete(fn);
  }

  private set(patch: Partial<SessionState>) {
    this.state = { ...this.state, ...patch };
    const s = this.state;
    if (hasUnsaved(s)) writeStored(this.key, { revision: s.base?.revision ?? "", draft: s.draft });
    else if (s.status === "clean") writeStored(this.key, null);
    for (const fn of this.listeners) fn();
    for (const fn of globalListeners) fn();
  }

  /** Whether the session has ever read its note; an unopened session has nothing to track. */
  get opened(): boolean {
    return this.recovered;
  }

  /**
   * Reads the note, or rechecks it against disk when it was read before.
   * The first open also picks up a draft a previous page left in storage.
   */
  open(): Promise<void> {
    if (this.opening) return this.opening;
    if (!this.recovered) {
      this.recovered = true;
      this.stored = readStored(this.key);
    }
    if (this.inflight) {
      this.recheckAfterSave = true;
      return this.inflight;
    }
    this.opening = this.recheck().finally(() => {
      this.opening = null;
    });
    return this.opening;
  }

  /** The editor's document changed. */
  edit(draft: string) {
    const s = this.state;
    if (draft === s.draft) return;
    if (s.status === "conflict" || s.status === "error" || s.status === "loading") {
      this.set({ draft });
      return;
    }
    if (s.status === "saving") {
      this.set({ draft });
      return;
    }
    if (s.base && draft === s.base.source) {
      this.cancel();
      this.set({ draft, status: "clean", error: null });
      return;
    }
    this.set({ draft, status: "pending", error: null });
    this.schedule();
  }

  private schedule() {
    this.cancel();
    this.timer = setTimeout(() => {
      this.timer = null;
      void this.save();
    }, SAVE_DELAY);
  }

  private cancel() {
    if (this.timer !== null) {
      clearTimeout(this.timer);
      this.timer = null;
    }
  }

  /** Saves now rather than after the quiet period. Resolves when any save settles. */
  flush(keepalive = false): Promise<void> {
    if (this.state.status === "pending") {
      this.cancel();
      return this.save(keepalive);
    }
    return this.inflight ?? Promise.resolve();
  }

  /** Sends a failed save again. */
  retry(): Promise<void> {
    if (this.state.status !== "failed") return Promise.resolve();
    this.resent = false;
    return this.save();
  }

  private save(keepalive = false): Promise<void> {
    if (this.inflight) return this.inflight;
    const s = this.state;
    if (!s.base || s.status === "conflict" || s.status === "error" || s.status === "loading") return Promise.resolve();
    if (s.draft === s.base.source) {
      this.set({ status: "clean", error: null });
      return Promise.resolve();
    }
    this.sent = s.draft;
    this.set({ status: "saving", error: null });
    const base = s.base;
    this.inflight = saveSource(this.slug, this.path, this.sent, base.revision, keepalive)
      .then(
        (saved) => {
          this.inflight = null;
          this.resent = false;
          if (this.state.draft === this.sent) {
            this.set({ base: saved, status: "clean" });
          } else {
            this.set({ base: saved, status: "pending" });
            this.schedule();
          }
        },
        (e: Error) => {
          this.inflight = null;
          const err = e instanceof SourceError ? e : new SourceError(0, "network", e.message);
          if (err.code === "not_found") {
            this.set({ status: "conflict", conflict: { kind: "deleted", current: null }, error: null });
            return;
          }
          this.set({ status: "failed", error: { code: err.code, message: err.message } });
          // A refused revision means the file moved on: find out how.
          if (err.code === "conflict") return this.recheck();
        },
      )
      .then(() => {
        if (this.recheckAfterSave) {
          this.recheckAfterSave = false;
          return this.recheck();
        }
      });
    return this.inflight;
  }

  /**
   * The daemon reported a change to this note. A change seen while a save
   * is in flight is usually the save itself, so the check waits for the
   * response and then compares revisions.
   */
  changed(): Promise<void> {
    if (!this.recovered) return Promise.resolve();
    if (this.inflight) {
      this.recheckAfterSave = true;
      return this.inflight;
    }
    return this.recheck();
  }

  /**
   * Compares the note on disk with what the session knows. Same revision:
   * nothing. A clean draft follows the file; a dirty one enters conflict.
   */
  private async recheck(): Promise<void> {
    let fetched: Source;
    try {
      fetched = await fetchSource(this.slug, this.path);
    } catch (e) {
      const err = e instanceof SourceError ? e : new SourceError(0, "network", (e as Error).message);
      this.rereadFailed(err);
      return;
    }
    const s = this.state;
    if (s.status === "saving") {
      // A save started while the read was out; compare after it settles.
      this.recheckAfterSave = true;
      return;
    }
    const stored = this.stored;
    this.stored = null;
    if (stored && !s.base) {
      if (stored.draft === fetched.source) {
        // The final save on the way out landed; nothing to recover.
        this.set({ base: fetched, draft: fetched.source, status: "clean", generation: s.generation + 1 });
      } else if (stored.revision === fetched.revision) {
        this.set({ base: fetched, draft: stored.draft, status: "pending", generation: s.generation + 1 });
        this.schedule();
      } else {
        this.set({
          base: fetched,
          draft: stored.draft,
          status: "conflict",
          conflict: { kind: "changed", current: fetched },
          generation: s.generation + 1,
        });
      }
      return;
    }
    if (s.status === "conflict") {
      if (s.conflict?.current && s.conflict.current.revision === fetched.revision) return;
      this.set({ conflict: { kind: "changed", current: fetched } });
      return;
    }
    if (s.base && s.base.revision === fetched.revision) {
      if (s.status === "error") this.set({ status: "clean", error: null });
      return;
    }
    if (s.base && s.base.source === fetched.source) {
      // Same content under a new revision (a touch, a permission change,
      // a same-bytes rewrite): the draft still stands on the same text.
      // A save this refused is sent again, once, against the new revision.
      this.set({ base: fetched });
      if (s.status === "failed" && s.error?.code === "conflict" && !this.resent) {
        this.resent = true;
        return this.save();
      }
      return;
    }
    const clean = s.status === "clean" || s.status === "loading" || s.status === "error" || !s.base;
    if (clean || s.draft === fetched.source) {
      const changed = s.draft !== fetched.source;
      this.set({
        base: fetched,
        draft: fetched.source,
        status: "clean",
        error: null,
        generation: changed || !s.base ? s.generation + 1 : s.generation,
      });
      return;
    }
    this.cancel();
    this.set({ status: "conflict", conflict: { kind: "changed", current: fetched }, error: null });
  }

  private rereadFailed(err: SourceError) {
    const s = this.state;
    if (err.code === "not_found") {
      const stored = this.stored;
      this.stored = null;
      if (stored) {
        this.set({
          draft: stored.draft,
          status: "conflict",
          conflict: { kind: "deleted", current: null },
          generation: s.generation + 1,
        });
      } else if (!s.base) {
        this.set({ status: "error", error: { code: err.code, message: err.message } });
      } else {
        this.cancel();
        this.set({ status: "conflict", conflict: { kind: "deleted", current: null }, error: null });
      }
      return;
    }
    if (s.status === "loading") {
      this.set({ status: "error", error: { code: err.code, message: err.message } });
    }
    // Otherwise a transient read failure changes nothing; the draft stands,
    // and a stored draft waits for the next open.
  }

  /** Resolves a conflict by saving the draft over the current file. */
  keepDraft(): Promise<void> {
    const s = this.state;
    if (s.status !== "conflict" || !s.conflict?.current) return Promise.resolve();
    this.set({ base: s.conflict.current, status: "pending", conflict: null });
    return this.save();
  }

  /** Resolves a conflict by dropping the draft and taking the file as it is. */
  loadFile() {
    const s = this.state;
    if (s.status !== "conflict" || !s.conflict?.current) return;
    const current = s.conflict.current;
    this.set({
      base: current,
      draft: current.source,
      status: "clean",
      conflict: null,
      error: null,
      generation: s.generation + 1,
    });
  }

  /** Drops the draft of a note that no longer exists. */
  discard() {
    const s = this.state;
    if (s.status !== "conflict") return;
    if (s.conflict?.current) {
      this.loadFile();
      return;
    }
    writeStored(this.key, null);
    this.set({
      base: null,
      draft: "",
      status: "error",
      conflict: null,
      error: { code: "not_found", message: "note no longer exists" },
      generation: s.generation + 1,
    });
  }
}

const sessions = new Map<string, Session>();
const globalListeners = new Set<() => void>();

/** Notified when any session changes; for the page-wide unsaved indicator. */
export function subscribeSessions(fn: () => void): () => void {
  globalListeners.add(fn);
  return () => globalListeners.delete(fn);
}

/** The session for a note, created on first use and kept for the page's lifetime. */
export function getSession(slug: string, path: string): Session {
  const key = slug + "\0" + path;
  let s = sessions.get(key);
  if (!s) {
    s = new Session(slug, path);
    sessions.set(key, s);
  }
  return s;
}

/** Whether a previous page left an unsaved draft of this note in storage. */
export function hasStoredDraft(slug: string, path: string): boolean {
  return readStored(slug + "\0" + path) !== null;
}

/** Sessions holding work that is not on disk yet. */
export function unsavedSessions(): Session[] {
  return [...sessions.values()].filter((s) => hasUnsaved(s.state));
}

/** Sends every pending draft now. */
export function flushAll(keepalive = false): Promise<void> {
  return Promise.all([...sessions.values()].map((s) => s.flush(keepalive))).then(() => undefined);
}

/** Forgets every session; for tests. */
export function resetSessions() {
  sessions.clear();
}
