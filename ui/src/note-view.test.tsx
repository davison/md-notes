import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { act, cleanup, fireEvent, render, screen, waitFor } from "@testing-library/preact";
import { LocationProvider, Router, Route, useLocation } from "preact-iso";
import { NoteView, formatValue, fragmentTarget } from "./note-view";
import { KEY, reset, update } from "./settings";

function mockNote(note: unknown, status = 200) {
  vi.stubGlobal(
    "fetch",
    vi.fn(() =>
      Promise.resolve({
        ok: status === 200,
        status,
        statusText: status === 200 ? "OK" : "Not Found",
        json: () => Promise.resolve(status === 200 ? note : { error: "not found" }),
      } as Response),
    ),
  );
}

/**
 * jsdom implements no `Element.prototype.scrollIntoView`, so the cases that
 * watch for one put their own there. It comes off again in `afterEach`: a
 * method left on the prototype outlives the case that wanted it, and the
 * next one would be watching a stranger's spy.
 */
const nativeScrollIntoView = Object.getOwnPropertyDescriptor(Element.prototype, "scrollIntoView");
function stubScrollIntoView(fn: (this: Element) => void) {
  Object.defineProperty(Element.prototype, "scrollIntoView", { value: fn, configurable: true, writable: true });
}

beforeEach(() => {
  window.location.hash = "";
  localStorage.clear();
  reset();
});

/**
 * Every case unmounts what it rendered, and this is where.
 *
 * `@testing-library/preact` installs its own `afterEach(cleanup)` only when
 * the test globals are injected, and this project runs vitest without them,
 * so the unmount is ours to make. Making it in `beforeEach` instead is not
 * enough, and that is the flake this file was carrying (davison/md-notes#87):
 * it leaves the last case's tree mounted for the rest of the file's life.
 * The fetch a `NoteView` starts from its mount effect resolves after the
 * `act()` that rendered it has returned, so the commit it causes schedules
 * its effect flush on preact's own requestAnimationFrame-or-100 ms path
 * rather than into act's collector, and no later `act()` owns it — measured:
 * one flush still queued when the last case ends. Normally it lands a frame
 * later and does nothing of note; under the load of the whole suite it lands
 * after vitest has torn the jsdom environment down, and the effect
 * dereferences `window` (`ReferenceError: window is not defined`, note-view.tsx
 * line 96). Unmounting here is what makes it harmless: preact skips the
 * queued effects of a component that is no longer mounted.
 */
afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  if (nativeScrollIntoView) Object.defineProperty(Element.prototype, "scrollIntoView", nativeScrollIntoView);
  else delete (Element.prototype as Partial<Element>).scrollIntoView;
});

describe("fragmentTarget", () => {
  it("looks only inside the note", () => {
    document.body.innerHTML = '<div id="app"><article><h2 id="sec">s</h2><h2 id="fn:1">f</h2></article></div>';
    const scope = document.querySelector("article");
    expect(fragmentTarget(scope, "#sec")?.textContent).toBe("s");
    expect(fragmentTarget(scope, "#fn%3A1")?.textContent).toBe("f");
    expect(fragmentTarget(scope, "#app")).toBeNull();
    expect(fragmentTarget(scope, "#")).toBeNull();
    expect(fragmentTarget(scope, '#a%0Ab"]')).toBeNull();
    expect(fragmentTarget(scope, "#%ZZ")).toBeNull();
    document.body.innerHTML = "";
  });
});

describe("formatValue", () => {
  it("renders scalars, arrays, and objects readably", () => {
    expect(formatValue("x")).toBe("x");
    expect(formatValue(3)).toBe("3");
    expect(formatValue(["a", "b"])).toBe("a, b");
    expect(formatValue({ k: 1 })).toBe('{"k":1}');
    expect(formatValue(null)).toBe("");
  });
});

describe("NoteView", () => {
  it("renders the title and body and requests the right URL", async () => {
    mockNote({ path: "a b.md", title: "Hello", html: "<p>Body <strong>text</strong></p>" });
    render(<NoteView slug="my notes" path="dir/a b.md" />);
    await waitFor(() => expect(screen.getByText("Hello")).toBeTruthy());
    expect(screen.getByText("text").tagName).toBe("STRONG");
    expect(screen.queryByText("Metadata")).toBeNull();
    const url = (fetch as unknown as ReturnType<typeof vi.fn>).mock.calls[0][0] as string;
    expect(url).toBe("/api/r/my%20notes/note/dir/a%20b.md");
  });

  it("shows frontmatter in a collapsed panel", async () => {
    mockNote({ path: "x.md", title: "T", html: "<p>b</p>", frontmatter: { tags: ["a", "b"], draft: true } });
    render(<NoteView slug="n" path="x.md" />);
    await waitFor(() => expect(screen.getByText("Metadata")).toBeTruthy());
    const details = screen.getByText("Metadata").closest("details")!;
    expect(details.open).toBe(false);
    expect(screen.getByText("a, b")).toBeTruthy();
    expect(screen.getByText("true")).toBeTruthy();
    fireEvent.click(screen.getByText("Metadata"));
  });

  it("refetches in place when the version bumps, without clearing the body", async () => {
    mockNote({ path: "x.md", title: "First", html: "<p>one</p>" });
    const { rerender } = render(<NoteView slug="n" path="x.md" version={0} />);
    await waitFor(() => expect(screen.getByText("First")).toBeTruthy());
    mockNote({ path: "x.md", title: "Second", html: "<p>two</p>" });
    rerender(<NoteView slug="n" path="x.md" version={1} />);
    expect(screen.getByText("First")).toBeTruthy();
    await waitFor(() => expect(screen.getByText("Second")).toBeTruthy());
    expect(screen.getByText("two")).toBeTruthy();
  });

  it("recovers from an error when a different note is opened", async () => {
    mockNote(null, 404);
    const { rerender } = render(<NoteView slug="n" path="gone.md" version={3} />);
    await waitFor(() => expect(screen.getByText("not found")).toBeTruthy());
    mockNote({ path: "other.md", title: "Other", html: "<p>fine</p>" });
    rerender(<NoteView slug="n" path="other.md" version={3} />);
    await waitFor(() => expect(screen.getByText("Other")).toBeTruthy());
    expect(screen.queryByText("not found")).toBeNull();
  });

  it("clears an error when the same note reappears", async () => {
    mockNote(null, 404);
    const { rerender } = render(<NoteView slug="n" path="x.md" version={1} />);
    await waitFor(() => expect(screen.getByText("not found")).toBeTruthy());
    mockNote({ path: "x.md", title: "Back", html: "<p>again</p>" });
    rerender(<NoteView slug="n" path="x.md" version={2} />);
    await waitFor(() => expect(screen.getByText("Back")).toBeTruthy());
    expect(screen.queryByText("not found")).toBeNull();
  });

  it("scrolls to the requested line once, and flashes the block after an anchor", async () => {
    mockNote({
      path: "x.md",
      title: "T",
      html: '<p data-line="1">a</p><div class="line-anchor" data-line="5"></div><pre>code</pre><p data-line="9">c</p>',
    });
    const scrolls: string[] = [];
    stubScrollIntoView(function (this: Element) {
      scrolls.push(this.className);
    });
    const { rerender } = render(<NoteView slug="n" path="x.md" version={0} line={6} />);
    await waitFor(() => expect(scrolls.length).toBe(1));
    expect(scrolls[0]).toContain("line-anchor");
    expect(document.querySelector("pre")!.classList.contains("flash")).toBe(true);
    // A live refetch of the same note and line does not scroll again.
    rerender(<NoteView slug="n" path="x.md" version={1} line={6} />);
    await new Promise((r) => setTimeout(r, 20));
    expect(scrolls.length).toBe(1);
    // A different line does.
    rerender(<NoteView slug="n" path="x.md" version={1} line={9} />);
    await waitFor(() => expect(scrolls.length).toBe(2));
  });

  it("does not flash the block when animations are off", async () => {
    // The scroll still happens: centring the block is what says where the
    // hit is, and it is not an animation. The flash is not merely an
    // animation the stylesheet has cancelled — the class never goes on.
    localStorage.setItem(KEY, JSON.stringify({ light: false, noMotion: true }));
    reset();
    mockNote({ path: "x.md", title: "T", html: '<p data-line="1">a</p><pre data-line="5">code</pre>' });
    const scrolls: string[] = [];
    stubScrollIntoView(function (this: Element) {
      scrolls.push(this.className);
    });
    render(<NoteView slug="n" path="x.md" version={0} line={5} />);
    await waitFor(() => expect(scrolls.length).toBe(1));
    expect(document.querySelector("pre")!.classList.contains("flash")).toBe(false);
    // Nothing is left to clean up either: no timer holds a class to remove.
    await new Promise((r) => setTimeout(r, 20));
    expect(document.querySelector("pre")!.classList.contains("flash")).toBe(false);
  });

  it("does not flash the block when the device asks for reduced motion", async () => {
    vi.stubGlobal(
      "matchMedia",
      vi.fn((query: string) => ({
        matches: query.includes("prefers-reduced-motion"),
        media: query,
        addEventListener: () => {},
        removeEventListener: () => {},
      })),
    );
    mockNote({ path: "x.md", title: "T", html: '<pre data-line="5">code</pre>' });
    stubScrollIntoView(() => {});
    render(<NoteView slug="n" path="x.md" version={0} line={5} />);
    await waitFor(() => expect(screen.getByText("T")).toBeTruthy());
    await new Promise((r) => setTimeout(r, 20));
    expect(document.querySelector("pre")!.classList.contains("flash")).toBe(false);
  });

  it("scrolls a fresh open to the top when the requested line has no block", async () => {
    mockNote({ path: "x.md", title: "T", html: '<p data-line="6">a</p>' });
    const { container } = render(
      <main>
        <NoteView slug="n" path="x.md" version={0} line={2} />
      </main>,
    );
    const pane = container.querySelector("main")!;
    pane.scrollTop = 500;
    await waitFor(() => expect(screen.getByText("a")).toBeTruthy());
    await waitFor(() => expect(pane.scrollTop).toBe(0));
  });

  it("surfaces the daemon's error", async () => {
    mockNote(null, 404);
    render(<NoteView slug="n" path="missing.md" />);
    await waitFor(() => expect(screen.getByText("not found")).toBeTruthy());
    expect(screen.getByText("missing.md")).toBeTruthy();
  });

  it("routes in-app links client-side and leaves raw links to the browser", async () => {
    mockNote({
      path: "x.md",
      title: "T",
      html: '<p><a href="/r/n/other.md">next</a> <a href="/api/r/n/raw/f.pdf" target="_blank">pdf</a></p>',
    });
    let seen = "";
    function Probe() {
      seen = useLocation().url;
      return null;
    }
    history.replaceState(null, "", "/r/n/x.md");
    render(
      <LocationProvider>
        <Probe />
        <Router>
          <Route path="/r/:slug/:note*" component={() => <NoteView slug="n" path="x.md" />} />
          <Route default component={() => null} />
        </Router>
      </LocationProvider>,
    );
    await waitFor(() => expect(screen.getByText("next")).toBeTruthy());
    fireEvent.click(screen.getByText("next"));
    await waitFor(() => expect(seen).toBe("/r/n/other.md"));
    expect(window.location.pathname).toBe("/r/n/other.md");

    const pdf = screen.getByText("pdf");
    const notPrevented = fireEvent.click(pdf);
    expect(notPrevented).toBe(true);
    expect(window.location.pathname).toBe("/r/n/other.md");
    history.replaceState(null, "", "/");
  });

  it("scrolls to a fragment link inside the note without routing", async () => {
    mockNote({ path: "x.md", title: "T", html: '<p><a href="#sec">go</a></p><h2 id="sec">Sec</h2>' });
    render(<NoteView slug="n" path="x.md" />);
    await waitFor(() => expect(screen.getByText("go")).toBeTruthy());
    const target = document.getElementById("sec")!;
    target.scrollIntoView = vi.fn();
    const link = screen.getByText("go");
    const prevented = !fireEvent.click(link);
    expect(prevented).toBe(true);
    expect(target.scrollIntoView).toHaveBeenCalled();
    expect(window.location.hash).toBe("#sec");
  });
});

describe("NoteView's diagrams", () => {
  const html =
    '<div class="line-anchor" data-line="3"></div>\n<pre><code class="language-mermaid">graph TD; A--&gt;B\n</code></pre>\n';
  const listed = { path: "d/x.md", title: "T", html, diagrams: [{ line: 3, hash: "abc" }] };

  /** A device whose colour scheme the test sets, and can change. */
  function stubScheme(dark: boolean) {
    const listeners = new Set<() => void>();
    const query = {
      matches: dark,
      media: "(prefers-color-scheme: dark)",
      addEventListener: (_: string, fn: () => void) => listeners.add(fn),
      removeEventListener: (_: string, fn: () => void) => listeners.delete(fn),
    };
    vi.stubGlobal(
      "matchMedia",
      vi.fn((q: string) => (q.includes("prefers-color-scheme") ? query : { matches: false, media: q, addEventListener() {}, removeEventListener() {} })),
    );
    return (next: boolean) => {
      query.matches = next;
      for (const fn of listeners) fn();
    };
  }

  it("shows a listed flowchart as the daemon's image, in the device's scheme", async () => {
    stubScheme(true);
    mockNote(listed);
    const { container } = render(<NoteView slug="n b" path="d/x.md" />);
    await waitFor(() => expect(container.querySelector("img.diagram")).toBeTruthy());
    const img = container.querySelector("img.diagram")!;
    expect(img.getAttribute("src")).toBe("/api/r/n%20b/diagram/d/x.md?h=abc&theme=dark");
    expect(container.querySelector("pre")!.classList.contains("diagram-source")).toBe(true);
  });

  it("follows the scheme and the light override as they change", async () => {
    const setDark = stubScheme(false);
    mockNote(listed);
    const { container } = render(<NoteView slug="n" path="d/x.md" />);
    await waitFor(() => expect(container.querySelector("img.diagram")).toBeTruthy());
    const img = container.querySelector("img.diagram")!;
    expect(img.getAttribute("src")).toContain("theme=light");

    act(() => setDark(true));
    await waitFor(() => expect(img.getAttribute("src")).toContain("theme=dark"));
    act(() => {
      update({ light: true });
    });
    await waitFor(() => expect(img.getAttribute("src")).toContain("theme=eink"));
    act(() => {
      update({ light: false });
    });
    await waitFor(() => expect(img.getAttribute("src")).toContain("theme=dark"));
    // Still the one element: a theme change is a new src, not a new image.
    expect(container.querySelectorAll("img.diagram").length).toBe(1);
    expect(container.querySelector("img.diagram")).toBe(img);
  });

  it("leaves a note without a list exactly as the daemon rendered it", async () => {
    stubScheme(false);
    mockNote({ path: "x.md", title: "T", html });
    const { container } = render(<NoteView slug="n" path="x.md" />);
    await waitFor(() => expect(screen.getByText("T")).toBeTruthy());
    expect(container.querySelector("img")).toBeNull();
    expect(container.querySelector("pre")!.classList.contains("diagram-source")).toBe(false);
  });

  it("flashes the image, not the hidden code, for a search hit on the block", async () => {
    stubScheme(false);
    mockNote(listed);
    stubScrollIntoView(() => {});
    const { container } = render(<NoteView slug="n" path="d/x.md" line={4} />);
    await waitFor(() => expect(container.querySelector("img.diagram.flash")).toBeTruthy());
    expect(container.querySelector("pre")!.classList.contains("flash")).toBe(false);
  });
});
