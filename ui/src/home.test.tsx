import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/preact";
import { Home } from "./home";

/**
 * The home page's remove control (M7-R2). A root registered by accident —
 * the intercept's own case in
 * [#50](https://github.com/davison/md-notes/issues/50) — had to be taken
 * out of the state file by hand before this.
 */

const roots = [
  { slug: "notes", path: "/home/you/notes", kind: "notes" },
  { slug: "scratch", path: "/tmp/scratch", kind: "recent" },
  { slug: "proj", path: "/home/you/proj", kind: "recent" },
];

let calls: { method: string; url: string }[] = [];
/** What DELETE answers, so a refusal can be shown where it was asked for. */
let refusal: { status: number; code: string; error: string } | null = null;

function mockApi() {
  vi.stubGlobal(
    "fetch",
    vi.fn((url: string, init?: RequestInit) => {
      const method = init?.method ?? "GET";
      calls.push({ method, url });
      if (method === "DELETE") {
        if (refusal !== null) {
          return Promise.resolve({
            ok: false,
            status: refusal.status,
            statusText: "",
            json: () => Promise.resolve({ code: refusal!.code, error: refusal!.error }),
          } as Response);
        }
        return Promise.resolve({ ok: true, status: 204, statusText: "No Content" } as Response);
      }
      return Promise.resolve({
        ok: true,
        status: 200,
        statusText: "OK",
        json: () => Promise.resolve({ roots }),
      } as Response);
    }),
  );
}

beforeEach(() => {
  calls = [];
  refusal = null;
  mockApi();
});
afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

describe("the home page's remove control", () => {
  it("offers one on every recent root and none on the notes root", async () => {
    render(<Home />);
    await screen.findByText("scratch");
    const controls = screen.getAllByRole("button", { name: /^Remove / });
    expect(controls.map((b) => b.getAttribute("aria-label"))).toEqual([
      "Remove scratch",
      "Remove proj",
    ]);
    expect(screen.queryByLabelText("Remove notes")).toBeNull();
  });

  it("asks first, naming the path, and removes nothing when the answer is no", async () => {
    render(<Home />);
    await screen.findByText("scratch");
    fireEvent.click(screen.getByLabelText("Remove scratch"));

    const dialog = await screen.findByRole("dialog");
    expect(dialog.textContent).toContain("/tmp/scratch");
    // The one thing a person needs to be sure of before answering.
    expect(dialog.textContent).toMatch(/nothing is removed from disk/i);

    fireEvent.click(screen.getByRole("button", { name: "Cancel" }));
    await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
    expect(calls.filter((c) => c.method === "DELETE")).toEqual([]);
    expect(screen.getByText("scratch")).toBeTruthy();
  });

  it("deletes the root and drops it from the list when the answer is yes", async () => {
    render(<Home />);
    await screen.findByText("scratch");
    fireEvent.click(screen.getByLabelText("Remove scratch"));
    await screen.findByRole("dialog");
    fireEvent.click(screen.getByRole("button", { name: "Remove" }));

    await waitFor(() => expect(screen.queryByText("scratch")).toBeNull());
    expect(calls.filter((c) => c.method === "DELETE")).toEqual([
      { method: "DELETE", url: "/api/roots/scratch" },
    ]);
    // The other roots are where they were.
    expect(screen.getByText("proj")).toBeTruthy();
    expect(screen.getByText("notes")).toBeTruthy();
  });

  it("shows the daemon's refusal beside the question rather than losing it", async () => {
    refusal = { status: 403, code: "notes_root", error: "the notes root cannot be removed" };
    render(<Home />);
    await screen.findByText("scratch");
    fireEvent.click(screen.getByLabelText("Remove scratch"));
    await screen.findByRole("dialog");
    fireEvent.click(screen.getByRole("button", { name: "Remove" }));

    await screen.findByText("the notes root cannot be removed");
    // The prompt is still open, with the root still listed behind it.
    expect(screen.queryByRole("dialog")).toBeTruthy();
    expect(screen.getByText("scratch")).toBeTruthy();
  });
});
