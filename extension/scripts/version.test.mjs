/**
 * The manifest's version is derived, so the derivation is the thing that has
 * to hold: a release tag reaches the manifest unchanged but for its `v`, and
 * everything the Web Store would reject becomes the development version.
 */
import { describe, expect, it } from "vitest";
import { DEV_VERSION, manifestVersion, withVersion } from "./version.mjs";

describe("manifestVersion", () => {
  it("takes a release tag and drops its v", () => {
    expect(manifestVersion("v0.1.0")).toBe("0.1.0");
    expect(manifestVersion("v1.20.300")).toBe("1.20.300");
    expect(manifestVersion("0.1.0")).toBe("0.1.0");
  });

  it("trims the surrounding whitespace a shell leaves behind", () => {
    expect(manifestVersion(" v0.1.0\n")).toBe("0.1.0");
  });

  it("calls everything git describe writes about an untagged build a dev build", () => {
    // What `git describe --tags --always --dirty` produces off a tag, on a
    // repository with no tags at all, and with uncommitted changes.
    for (const raw of ["v0.1.0-3-gabc1234", "4bcf322", "v0.1.0-dirty", "dev", "", undefined, null]) {
      expect(manifestVersion(raw)).toBe(DEV_VERSION);
    }
  });

  it("refuses what the Chrome Web Store would refuse", () => {
    // Leading zeros, a fifth component, a component past 65535, and anything
    // that is not a number: all rejected rather than passed on for the store
    // to reject after the release has been published.
    for (const raw of ["v0.01.0", "1.2.3.4.5", "1.65536.0", "v1.2.x", "v1..2"]) {
      expect(manifestVersion(raw)).toBe(DEV_VERSION);
    }
  });

  it("keeps the four components and the bound the store allows", () => {
    expect(manifestVersion("1.2.3.4")).toBe("1.2.3.4");
    expect(manifestVersion("65535.65535.65535")).toBe("65535.65535.65535");
  });
});

describe("withVersion", () => {
  const template = {
    manifest_version: 3,
    name: "md-notes",
    description: "Open local markdown files in md-notes.",
    permissions: ["storage"],
  };

  it("carries the version it was given into the manifest", () => {
    expect(withVersion(template, "v0.1.0").version).toBe("0.1.0");
  });

  it("changes nothing else about the manifest", () => {
    const { version, ...rest } = withVersion(template, "v0.1.0");
    expect(version).toBe("0.1.0");
    expect(rest).toEqual(template);
  });

  it("puts the version where a reader looks for it, third", () => {
    expect(Object.keys(withVersion(template, "v0.1.0")).slice(0, 3)).toEqual([
      "manifest_version",
      "name",
      "version",
    ]);
  });

  it("replaces a version already present rather than keeping two", () => {
    const stamped = withVersion({ ...template, version: "9.9.9" }, "v0.1.0");
    expect(stamped.version).toBe("0.1.0");
    expect(Object.keys(stamped).filter((key) => key === "version")).toHaveLength(1);
  });
});
