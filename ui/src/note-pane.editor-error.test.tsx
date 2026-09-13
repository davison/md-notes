import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/preact";
import { NotePane } from "./note-pane";
import { resetSessions } from "./session";

// The chunk that is not there. Upgrading the daemon under an open tab is the
// real case: the hashed name this page asks for has gone, the single-page
// fallback answers with index.html, and the import rejects.
const chunk = vi.hoisted(() => ({ fails: true, attempts: 0 }));
vi.mock("./editor", () => {
  chunk.attempts++;
  if (chunk.fails) throw new Error("Failed to fetch dynamically imported module");
  return { Editor: () => <div class="cm-editor">editor</div> };
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
  expect(alert.textContent).toContain("reloading the page");
  expect(document.body.textContent).not.toContain("Loading the editor…");
  expect(document.querySelector(".cm-editor")).toBeNull();

  // Retrying asks for the chunk again rather than reusing the failure.
  const before = chunk.attempts;
  fireEvent.click(screen.getByRole("button", { name: "Try again" }));
  await waitFor(() => expect(chunk.attempts).toBeGreaterThan(before));
});
