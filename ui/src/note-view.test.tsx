import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/preact";
import { LocationProvider, Router, Route, useLocation } from "preact-iso";
import { NoteView, formatValue, fragmentTarget } from "./note-view";

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

beforeEach(() => {
  cleanup();
  window.location.hash = "";
});
afterEach(() => vi.unstubAllGlobals());

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
    Element.prototype.scrollIntoView = function () {
      scrolls.push((this as Element).className);
    };
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
