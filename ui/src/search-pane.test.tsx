import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/preact";
import { SearchPane, emphasise, groupByPath } from "./search-pane";
import type { Hit } from "./api";

const hits: Hit[] = [
  { path: "a.md", line: 2, text: "The Needle is here", matches: [[4, 10]], before: "first", after: "last" },
  { path: "a.md", line: 9, text: "needle again", matches: [[0, 6]] },
  { path: "b/c.md", line: 1, text: "x needle y needle", matches: [[2, 8], [11, 17]] },
];

beforeEach(() => {
  vi.useFakeTimers();
  vi.stubGlobal(
    "fetch",
    vi.fn((url: string) =>
      Promise.resolve({
        ok: true,
        status: 200,
        statusText: "OK",
        json: () => Promise.resolve(url.includes("q=none") ? { hits: [], truncated: false } : { hits, truncated: true }),
      } as Response),
    ),
  );
});
afterEach(() => {
  cleanup();
  vi.useRealTimers();
  vi.unstubAllGlobals();
});

describe("groupByPath and emphasise", () => {
  it("groups consecutive hits by file", () => {
    expect(groupByPath(hits).map((g) => [g.path, g.hits.length])).toEqual([
      ["a.md", 2],
      ["b/c.md", 1],
    ]);
  });
  it("wraps matched ranges in mark", () => {
    const { container } = render(<span>{emphasise(hits[2])}</span>);
    expect(container.querySelectorAll("mark").length).toBe(2);
    expect(container.textContent).toBe("x needle y needle");
  });
});

describe("SearchPane", () => {
  it("debounces, requests the encoded query, and links hits with the line", async () => {
    render(<SearchPane slug="my notes" keep={{ tag: "t" }} />);
    const input = screen.getByLabelText("Search notes") as HTMLInputElement;
    fireEvent.input(input, { target: { value: "nee" } });
    fireEvent.input(input, { target: { value: "needle x" } });
    expect(fetch).not.toHaveBeenCalled();
    await vi.advanceTimersByTimeAsync(250);
    expect(fetch).toHaveBeenCalledTimes(1);
    const url = (fetch as unknown as ReturnType<typeof vi.fn>).mock.calls[0][0] as string;
    expect(url).toBe("/api/r/my%20notes/search?q=needle%20x");
    await waitFor(() => expect(screen.getByText("a.md")).toBeTruthy());
    const link = screen.getAllByRole("link")[0] as HTMLAnchorElement;
    expect(link.getAttribute("href")).toBe("/r/my%20notes/a.md?l=2&tag=t");
    expect(screen.getByText("first")).toBeTruthy();
    expect(screen.getByText(/Showing the first 3 matches/)).toBeTruthy();
  });

  it("says when nothing matches and clears when the box empties", async () => {
    render(<SearchPane slug="n" />);
    const input = screen.getByLabelText("Search notes") as HTMLInputElement;
    fireEvent.input(input, { target: { value: "none" } });
    await vi.advanceTimersByTimeAsync(250);
    await waitFor(() => expect(screen.getByText("No matches.")).toBeTruthy());
    fireEvent.input(input, { target: { value: "" } });
    await vi.advanceTimersByTimeAsync(250);
    expect(screen.queryByText("No matches.")).toBeNull();
    expect(fetch).toHaveBeenCalledTimes(1);
  });
});
