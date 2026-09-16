import { useId, useState } from "preact/hooks";
import { createNote } from "./api";
import { Dialog } from "./dialog";
import { newNotePath } from "./note-name";
import { markCreated } from "./session";

/**
 * The name prompt behind the navigator's create control. Nothing is written
 * until it is confirmed, and a refusal — most often a name already taken —
 * is shown here with the typed name still in the box, so correcting it is
 * one edit rather than a retyped title.
 *
 * `folder` is the navigator's selected folder: the folder of the open note,
 * or the root when no note is open. The dialog says which it is, because a
 * bare title lands there and a name with a "/" does not.
 */
export function NewNoteDialog({
  slug,
  folder,
  onClose,
  onCreated,
}: {
  slug: string;
  folder: string;
  onClose: () => void;
  /** The created note's path, in the daemon's own cleaned form. */
  onCreated: (path: string) => void;
}) {
  const [name, setName] = useState("");
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);
  const id = useId();

  const create = () => {
    const wanted = newNotePath(name, folder);
    if (wanted.error !== undefined) {
      setError(wanted.error);
      return;
    }
    setBusy(true);
    setError(null);
    createNote(slug, wanted.path).then(
      (note) => {
        // The note opens in the editor, and at the daemon's path rather
        // than the one that was sent.
        markCreated(slug, note.path);
        onCreated(note.path);
      },
      (e: Error) => {
        setBusy(false);
        setError(e.message);
      },
    );
  };

  return (
    <Dialog title="New note" confirmLabel="Create" onConfirm={create} onCancel={onClose} busy={busy}>
      <label class="modal-label" for={id}>
        Title or path
      </label>
      <input
        id={id}
        class="modal-name"
        type="text"
        value={name}
        autocomplete="off"
        spellcheck={false}
        onInput={(e) => setName((e.currentTarget as HTMLInputElement).value)}
      />
      <p class="modal-hint">
        A name with no <code>/</code> is created in{" "}
        <span class="modal-folder">{folder === "" ? "the root of this folder" : <code>{folder}</code>}</span>, and gains{" "}
        <code>.md</code> if you leave the extension off. A name with a <code>/</code> is a path under the root.
      </p>
      {error && (
        <p class="modal-error" role="alert">
          {error}
        </p>
      )}
    </Dialog>
  );
}
