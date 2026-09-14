import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
// The shipped page itself, so the copy of the boot script in it is the one
// under test rather than a transcription of it.
import indexHTML from "../index.html?raw";
// And the built page, when there is one. Vite's glob answers {} rather than
// throwing when the build output is absent, so `pnpm test` on a clean tree
// still runs; `make test` builds the UI first, so CI always has it.
const built = import.meta.glob("../dist/index.html", { query: "?raw", import: "default", eager: true }) as Record<
  string,
  string
>;
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

  it("is a plain inline script, not a deferred one", () => {
    // type="module", defer or async would each move it after the parse and
    // give up the whole point: a module script runs too late to beat the
    // paint. The opening tag carries nothing at all.
    expect(html).toContain(`<script>\n      ${BOOT}\n    </script>`);
  });

  it("is the last thing in the head, where Vite injects after it", () => {
    // Vite appends its <script type="module"> and its stylesheet <link> to
    // the end of <head>, so "last in the source head" is what puts the boot
    // script ahead of both in the built page. Anything added after it here —
    // a <link>, a <style>, another script — would take that away, which is
    // what this notices and the old shape of this test could not: it asserted
    // that the source lacks the module script Vite has not injected yet, and
    // so could never fail.
    const head = /<head>([\s\S]*?)<\/head>/.exec(html)![1];
    expect(head).toContain(BOOT);
    const after = head.slice(head.indexOf(BOOT));
    expect(after).toContain("</script>");
    expect(after.slice(after.indexOf("</script>")).trim()).toBe("</script>");
  });

  it("precedes the module script and the stylesheet in the built page", () => {
    const page = built["../dist/index.html"];
    if (page === undefined) {
      // Not a skip: the source assertions above are what hold when there is
      // no build to look at, and `make test` always builds one.
      expect(html).toContain(BOOT);
      return;
    }
    const boot = page.indexOf(BOOT);
    expect(boot).toBeGreaterThanOrEqual(0);
    for (const re of [/<script[^>]+type="module"/, /<link[^>]+rel="stylesheet"/]) {
      const at = re.exec(page)?.index ?? -1;
      expect(at).toBeGreaterThanOrEqual(0);
      expect(boot).toBeLessThan(at);
    }
  });

  it("asks the browser to resize the content for the on-screen keyboard", () => {
    // Chromium's default shrinks the visual viewport only, which leaves the
    // caret and the note bar under the keyboard on a shell sized in dvh.
    expect(/<meta\s+name="viewport"[\s\S]*?interactive-widget=resizes-content/.test(html)).toBe(true);
  });
});
