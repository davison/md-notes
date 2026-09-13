import { useEffect, useState } from "preact/hooks";
import { listRoots, type Root } from "./api";

export function Home() {
  const [roots, setRoots] = useState<Root[] | null>(null);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    listRoots().then(setRoots, (e: Error) => setError(e.message));
  }, []);

  const notes = roots?.filter((r) => r.kind === "notes") ?? [];
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
          />
        </>
      )}
    </main>
  );
}

function RootList({ title, roots, empty }: { title: string; roots: Root[]; empty?: string }) {
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
            </li>
          ))}
        </ul>
      )}
    </section>
  );
}
