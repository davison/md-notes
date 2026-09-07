import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/preact";
import { NoteView, formatValue } from "./note-view";

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

  it("surfaces the daemon's error", async () => {
    mockNote(null, 404);
    render(<NoteView slug="n" path="missing.md" />);
    await waitFor(() => expect(screen.getByText("not found")).toBeTruthy());
    expect(screen.getByText("missing.md")).toBeTruthy();
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
