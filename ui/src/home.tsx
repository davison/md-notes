import { useEffect, useState } from "preact/hooks";
import { listRoots, removeRoot, type Root } from "./api";
import { Dialog } from "./dialog";
import { FALLBACK_TITLE, useDocumentTitle } from "./title";

export function Home() {
  const [roots, setRoots] = useState<Root[] | null>(null);
  const [error, setError] = useState<string | null>(null);
  /** The root the remove control is asking about, while it is asking. */
  const [removing, setRemoving] = useState<Root | null>(null);

  useDocumentTitle(FALLBACK_TITLE);

  useEffect(() => {
    listRoots().then(setRoots, (e: Error) => setError(e.message));
  }, []);

  // The notes root and the permanent roots are both the daemon's
  // configuration, listed together in the order configured, and neither
  // is offered a Remove control (M10-R5).
  const notes = roots?.filter((r) => r.kind === "notes" || r.kind === "permanent") ?? [];
  const recent = roots?.filter((r) => r.kind === "recent") ?? [];

  return (
    <main class="page">
      <h1>mdn</h1>
      {error && <p class="error">{error}</p>}
      {roots === null && !error && <p class="muted">Loading…</p>}
      {roots && (
        <>
          <RootList title="Notes" roots={notes} />
          <RootList
            title="Recent"
            roots={recent}
            empty="Nothing yet. Run mdn open <dir> to add a folder."
            onRemove={setRemoving}
          />
        </>
      )}
      {removing && (
        <RemoveRootDialog
          root={removing}
          onCancel={() => setRemoving(null)}
          onRemoved={(slug) => {
            setRoots((rs) => rs?.filter((r) => r.slug !== slug) ?? null);
            setRemoving(null);
          }}
        />
      )}
    </main>
  );
}

function RootList({
  title,
  roots,
  empty,
  onRemove,
}: {
  title: string;
  roots: Root[];
  empty?: string;
  /**
   * Given for the recent roots, which can be unregistered, and not for the
   * notes root or a permanent root, which are the daemon's configuration
   * and would only come back at the next start.
   */
  onRemove?: (root: Root) => void;
}) {
  return (
    <section>
      <h2>{title}</h2>
      {roots.length === 0 ? (
        <p class="muted">{empty ?? "None."}</p>
      ) : (
        <ul class="roots">
          {roots.map((r) => (
            <li key={r.slug}>
              <a href={`/r/${r.slug}/`}>{r.slug}</a>
              <span class="path">{r.path}</span>
              {onRemove && (
                <button
                  type="button"
                  class="root-remove"
                  aria-label={`Remove ${r.slug}`}
                  onClick={() => onRemove(r)}
                >
                  Remove
                </button>
              )}
            </li>
          ))}
        </ul>
      )}
    </section>
  );
}

/**
 * The confirmation before a root is unregistered.
 *
 * It names the path rather than the slug: the slug is this daemon's
 * shorthand, and the folder on disk is what the person is deciding about.
 * It also says what is *not* happening, because "remove" beside a folder
 * full of notes reads like a delete and is not one — nothing leaves the
 * disk, and `mdn open` puts the folder back.
 *
 * A refusal is shown here rather than thrown away with the prompt: the
 * daemon has four of them (the notes root, a permanent root, an unknown
 * slug, and the tailnet allow-list), and each is worth reading where it was asked for.
 */
function RemoveRootDialog({
  root,
  onCancel,
  onRemoved,
}: {
  root: Root;
  onCancel: () => void;
  onRemoved: (slug: string) => void;
}) {
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const remove = () => {
    setBusy(true);
    setError(null);
    removeRoot(root.slug).then(
      () => onRemoved(root.slug),
      (e: Error) => {
        setError(e.message);
        setBusy(false);
      },
    );
  };

  return (
    <Dialog
      title="Stop serving this folder?"
      confirmLabel="Remove"
      busy={busy}
      onConfirm={remove}
      onCancel={onCancel}
    >
      <p>
        <code class="modal-path">{root.path}</code> is unregistered, and its notes are no
        longer reachable through this daemon.
      </p>
      <p>
        Nothing is removed from disk: the folder and every note in it stay exactly as they
        are, and <code>mdn open</code> adds it back.
      </p>
      {error && (
        <p class="modal-error" role="alert">
          {error}
        </p>
      )}
    </Dialog>
  );
}
