import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/preact";
import { NotePane } from "./note-pane";
import { resetSessions } from "./session";

// Records the moment the editor module is evaluated. A static import would
// have evaluated it while `./note-pane` above was being imported; a dynamic
// one waits for the first toggle, which is the whole point of the change.
const editor = vi.hoisted(() => ({ loaded: false }));
vi.mock("./editor", () => {
  editor.loaded = true;
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

it("loads the editor chunk on the first toggle, not with the page", async () => {
  expect(editor.loaded).toBe(false);
  render(<NotePane slug="n" path="a.md" />);
  await waitFor(() => expect(screen.getByText("body")).toBeTruthy());
  expect(screen.getByText("Viewing")).toBeTruthy();
  expect(editor.loaded).toBe(false);

  fireEvent.keyDown(document.body, { key: "e", ctrlKey: true });
  await waitFor(() => expect(document.querySelector(".cm-editor")).toBeTruthy());
  expect(editor.loaded).toBe(true);
});
