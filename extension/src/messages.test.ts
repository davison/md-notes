import { describe, expect, it } from "vitest";
import { isClipMessage } from "./messages";

describe("isClipMessage", () => {
  it("accepts the three the popup sends", () => {
    expect(isClipMessage({ type: "clip:prepare", tabId: 3, kind: "page" })).toBe(true);
    expect(isClipMessage({ type: "clip:prepare", tabId: 3, kind: "selection" })).toBe(true);
    expect(isClipMessage({ type: "clip:save", title: "" })).toBe(true);
    expect(isClipMessage({ type: "clip:discard" })).toBe(true);
  });

  it("rejects anything else, so another extension's message is left alone", () => {
    for (const bad of [
      null,
      "clip:save",
      { type: "clip:prepare", tabId: "3", kind: "page" },
      { type: "clip:prepare", tabId: 3, kind: "everything" },
      { type: "clip:save" },
      { type: "something-else" },
    ]) {
      expect(isClipMessage(bad)).toBe(false);
    }
  });
});
