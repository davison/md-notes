/**
 * The version the built manifest carries, derived from the build's `VERSION`
 * rather than committed as a literal.
 *
 * There is one version in this repository and it is the git tag: the Makefile
 * derives `VERSION` from `git describe`, the Go binary gets it through
 * `-ldflags -X main.version`, and this module gives the same tag to the
 * manifest. `public/manifest.json` therefore carries no `version` key at all —
 * a literal there is a second source that drifts, and the one that drifts is
 * always the one nobody bumps.
 *
 * The Chrome Web Store will not take a version the way `git describe` writes
 * one: it requires one to four dot-separated integers, each 0-65535, with no
 * leading zeros and nothing else — no leading `v`, no `-3-gabc1234` commit
 * suffix, no `-dirty`. So a clean release tag is normalised (`v0.1.0` becomes
 * `0.1.0`) and anything else is a development build, which gets DEV_VERSION:
 * numeric, loadable, and obviously not a release.
 */

/** What an untagged, dirty or otherwise non-release build calls itself. */
export const DEV_VERSION = "0.0.0";

/** One to four components, no leading zeros, after an optional leading `v`. */
const RELEASE = /^v?(0|[1-9]\d*)(\.(0|[1-9]\d*)){0,3}$/;

/** The largest value a Chrome manifest version component may hold. */
const MAX_COMPONENT = 65535;

/**
 * The manifest version for a build of `raw`, which is a git tag, a
 * `git describe` string, or nothing at all.
 *
 * @param {string | undefined | null} raw
 * @returns {string}
 */
export function manifestVersion(raw) {
  const trimmed = (raw ?? "").trim();
  if (!RELEASE.test(trimmed)) return DEV_VERSION;
  const version = trimmed.startsWith("v") ? trimmed.slice(1) : trimmed;
  if (version.split(".").some((part) => Number(part) > MAX_COMPONENT)) return DEV_VERSION;
  return version;
}

/**
 * `manifest` with its version set from `raw`, the key placed where a reader
 * expects it: third, after `manifest_version` and `name`.
 *
 * @template {Record<string, unknown>} T
 * @param {T} manifest
 * @param {string | undefined | null} raw
 * @returns {Record<string, unknown>}
 */
export function withVersion(manifest, raw) {
  const { manifest_version, name, version: _discarded, ...rest } = manifest;
  return { manifest_version, name, version: manifestVersion(raw), ...rest };
}
