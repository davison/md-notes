import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
// The shipped page itself, so the copy of the boot script in it is the one
// under test rather than a transcription of it.
import indexHTML from "../index.html?raw";
import {
  BOOT,
  DEFAULTS,
  KEY,
  MOTION_ATTR,
  MOTION_NONE,
  THEME_ATTR,
  THEME_LIGHT,
  animationsOff,
  apply,
  read,
  reset,
  settings,
  update,
} from "./settings";

/** A matchMedia that answers the reduced-motion query with `reduce`. */
function stubReducedMotion(matches: boolean) {
  vi.stubGlobal(
    "matchMedia",
    vi.fn((query: string) => ({
      matches: matches && query.includes("prefers-reduced-motion"),
      media: query,
      addEventListener: () => {},
      removeEventListener: () => {},
    })),
  );
}

beforeEach(() => {
  localStorage.clear();
  reset();
  document.documentElement.removeAttribute(THEME_ATTR);
  document.documentElement.removeAttribute(MOTION_ATTR);
});
afterEach(() => vi.unstubAllGlobals());

describe("read", () => {
  it("gives the defaults when nothing is stored", () => {
    expect(read()).toEqual(DEFAULTS);
    expect(DEFAULTS).toEqual({ light: false, noMotion: false });
  });

  it("reads what was stored", () => {
    localStorage.setItem(KEY, JSON.stringify({ light: true, noMotion: true }));
    expect(read()).toEqual({ light: true, noMotion: true });
  });

  it("takes only true for true, and survives a corrupt value", () => {
    localStorage.setItem(KEY, JSON.stringify({ light: "yes", noMotion: 1 }));
    expect(read()).toEqual({ light: false, noMotion: false });
    localStorage.setItem(KEY, "{not json");
    expect(read()).toEqual(DEFAULTS);
  });
});

describe("apply", () => {
  it("puts each setting on the root element and takes it off again", () => {
    const root = document.documentElement;
    apply({ light: true, noMotion: true });
    expect(root.getAttribute(THEME_ATTR)).toBe(THEME_LIGHT);
    expect(root.getAttribute(MOTION_ATTR)).toBe(MOTION_NONE);
    apply({ light: false, noMotion: false });
    expect(root.hasAttribute(THEME_ATTR)).toBe(false);
    expect(root.hasAttribute(MOTION_ATTR)).toBe(false);
  });
});

describe("update", () => {
  it("persists the change, applies it, and leaves the other setting alone", () => {
    update({ light: true });
    expect(JSON.parse(localStorage.getItem(KEY)!)).toEqual({ light: true, noMotion: false });
    expect(document.documentElement.getAttribute(THEME_ATTR)).toBe(THEME_LIGHT);
    update({ noMotion: true });
    expect(settings()).toEqual({ light: true, noMotion: true });
    expect(document.documentElement.getAttribute(THEME_ATTR)).toBe(THEME_LIGHT);
    expect(document.documentElement.getAttribute(MOTION_ATTR)).toBe(MOTION_NONE);
  });

  it("survives storage it cannot write to", () => {
    const setItem = Storage.prototype.setItem;
    Storage.prototype.setItem = () => {
      throw new Error("quota");
    };
    try {
      expect(() => update({ light: true })).not.toThrow();
      expect(document.documentElement.getAttribute(THEME_ATTR)).toBe(THEME_LIGHT);
    } finally {
      Storage.prototype.setItem = setItem;
    }
  });
});

describe("animationsOff", () => {
  it("is the setting, or the device asking for reduced motion", () => {
    stubReducedMotion(false);
    expect(animationsOff()).toBe(false);
    update({ noMotion: true });
    expect(animationsOff()).toBe(true);
    update({ noMotion: false });
    expect(animationsOff()).toBe(false);
    stubReducedMotion(true);
    expect(animationsOff()).toBe(true);
  });

  it("is false where the browser has no matchMedia", () => {
    vi.stubGlobal("matchMedia", undefined);
    expect(animationsOff()).toBe(false);
  });
});

describe("the boot script", () => {
  const html = indexHTML;

  it("is what index.html runs, so the duplication cannot drift", () => {
    const inline = /<script>([\s\S]*?)<\/script>/.exec(html);
    expect(inline).not.toBeNull();
    expect(inline![1].trim()).toBe(BOOT);
  });

  it("sets the attributes before the application is loaded", () => {
    // The document is what a browser has at this point in the parse: the
    // stylesheet has not painted and the module bundle has not run.
    localStorage.setItem(KEY, JSON.stringify({ light: true, noMotion: true }));
    new Function(BOOT)();
    expect(document.documentElement.getAttribute(THEME_ATTR)).toBe(THEME_LIGHT);
    expect(document.documentElement.getAttribute(MOTION_ATTR)).toBe(MOTION_NONE);
  });

  it("leaves the attributes off when nothing is stored, and when storage is corrupt", () => {
    new Function(BOOT)();
    expect(document.documentElement.hasAttribute(THEME_ATTR)).toBe(false);
    expect(document.documentElement.hasAttribute(MOTION_ATTR)).toBe(false);
    localStorage.setItem(KEY, "{not json");
    expect(() => new Function(BOOT)()).not.toThrow();
    expect(document.documentElement.hasAttribute(THEME_ATTR)).toBe(false);
  });

  it("runs before the stylesheet the build injects can paint", () => {
    // Vite injects its <link rel="stylesheet"> and its module script into
    // <head>; an inline classic script there runs during the parse, ahead of
    // the first paint, which a deferred module does not. The test that
    // matters is that it is inline and in the head.
    const head = /<head>([\s\S]*?)<\/head>/.exec(html)![1];
    expect(head).toContain(BOOT);
    expect(/<script\s+type="module"/.test(head)).toBe(false);
  });
});
