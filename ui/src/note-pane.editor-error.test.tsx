import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/preact";
import { NotePane } from "./note-pane";
import { resetSessions } from "./session";

// The chunk that is not there. Upgrading the daemon under an open tab is the
// real case: the hashed name this page asks for has gone from the bundle,
// the daemon answers 404, and the import rejects.
vi.mock("./editor", () => {
  throw new Error("Failed to fetch dynamically imported module");
});

beforeEach(() => {
  localStorage.clear();
  resetSessions();
  vi.stubGlobal(
    "fetch",
    vi.fn((url: string) => {
      const body = url.includes("/source/") ? { source: "body\n", revision: "r1" } : { path: "a.md", title: "A", html: "<p>body</p>" };
      return Promise.resolve({ ok: true, status: 200, statusText: "OK", json: () => Promise.resolve(body) } as Response);
    }),
  );
});
afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

it("surfaces a failed editor chunk instead of waiting for it for ever", async () => {
  render(<NotePane slug="n" path="a.md" />);
  await waitFor(() => expect(screen.getByText("body")).toBeTruthy());
  fireEvent.keyDown(document.body, { key: "e", ctrlKey: true });

  const alert = await screen.findByRole("alert");
  expect(alert.textContent).toContain("The editor could not be loaded");
  expect(document.body.textContent).not.toContain("Loading the editor…");
  expect(document.querySelector(".cm-editor")).toBeNull();

  // Reloading is the only offer, and the only thing that works: a module
  // script whose fetch failed leaves a null entry in the browser's module
  // map, so importing the same URL again rejects without a request. A
  // "Try again" here would be a button that cannot succeed — and vitest,
  // which re-invokes a mock factory per import, is the one environment that
  // would have let such a test pass.
  expect(screen.getByRole("button", { name: "Reload the page" })).toBeTruthy();
  expect(screen.queryByRole("button", { name: "Try again" })).toBeNull();
});
