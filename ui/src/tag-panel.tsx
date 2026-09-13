import type { Tag } from "./api";

interface Props {
  slug: string;
  tags: Tag[] | null;
  active: string | null;
  /** The note path to keep in the URL when switching filters. */
  current: string;
}

/**
 * The URL for a note with a tag filter applied, or with it cleared when
 * `tag` is null. Exported for the top bar's filter chip at narrow widths,
 * which clears the filter from outside this panel.
 */
export function tagURL(slug: string, current: string, tag: string | null): string {
  const base = `/r/${encodeURIComponent(slug)}/${current
    .split("/")
    .filter(Boolean)
    .map(encodeURIComponent)
    .join("/")}`;
  return tag ? `${base}?tag=${encodeURIComponent(tag)}` : base;
}

/**
 * Tags with counts. Selecting one puts `?tag=` in the URL, which the root
 * view turns into a navigator filter; selecting it again clears it.
 */
export function TagPanel({ slug, tags, active, current }: Props) {
  if (tags === null) return <p class="muted">Loading tags…</p>;
  if (tags.length === 0) return <p class="muted">No tags.</p>;
  return (
    <section class="tags" aria-label="Tags">
      {active && (
        <a class="tag-clear" href={tagURL(slug, current, null)}>
          Clear filter: {active}
        </a>
      )}
      <ul>
        {tags.map((t) => (
          <li key={t.name}>
            <a
              class={t.name === active ? "tag active" : "tag"}
              href={tagURL(slug, current, t.name === active ? null : t.name)}
              aria-current={t.name === active ? "true" : undefined}
            >
              <span class="tag-name">#{t.name}</span>
              <span class="tag-count">{t.count}</span>
            </a>
          </li>
        ))}
      </ul>
    </section>
  );
}
