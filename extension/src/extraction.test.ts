import { describe, expect, it } from "vitest";
import { extractionError, isExtraction } from "./extraction";

const good = { kind: "page", url: "https://example.com/", title: "T", markdown: "# T" };

describe("isExtraction", () => {
  it("accepts a page and a selection", () => {
    expect(isExtraction(good)).toBe(true);
    expect(isExtraction({ ...good, kind: "selection" })).toBe(true);
  });

  it("rejects anything the worker could not go on to save", () => {
    for (const bad of [
      null,
      undefined,
      "a string",
      { ...good, kind: "other" },
      { ...good, url: "" },
      { ...good, markdown: 3 },
      { error: "nothing is selected" },
    ]) {
      expect(isExtraction(bad)).toBe(false);
    }
  });
});

describe("extractionError", () => {
  it("is null for an extraction", () => {
    expect(extractionError(good)).toBeNull();
  });

  it("passes the page's own reason through", () => {
    expect(extractionError({ error: "nothing is selected on this page" })).toBe(
      "nothing is selected on this page",
    );
  });

  it("says so when the page returned nothing at all", () => {
    // An injection that never ran, or a frame that went away mid-flight.
    expect(extractionError(undefined)).toMatch(/nothing/);
    expect(extractionError(null)).toMatch(/nothing/);
  });

  it("says so when the page returned something unreadable", () => {
    expect(extractionError({ kind: "page" })).toMatch(/cannot read/);
  });
});
