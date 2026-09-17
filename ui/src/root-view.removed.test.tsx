import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { cleanup, render, screen, waitFor } from "@testing-library/preact";
import { LocationProvider } from "preact-iso";
import { RootView } from "./root-view";
import { resetSessions } from "./session";

/**
 * A tab open on a root that has just been removed (M7-R2).
 *
 * The daemon ends the root's event stream when it unregisters it, and the
 * reconnect the browser makes meets the 404 every route under that slug now
 * gives. A stream the daemon could not open at all closes the same way, so
 * the roots listing is what tells the two apart: a slug that is gone sends
 * the tab home, and one that is still there keeps today's "live update is
 * not available" notice.
 */

/** The roots the daemon is listing at this moment in the test. */
let roots: { slug: string; path: string; kind: string }[];

class FakeEventSource {
  static last: FakeEventSource | null = null;
  readyState = 1;
  onerror: (() => void) | null = null;
  onopen: (() => void) | null = null;
  constructor(public url: string) {
    FakeEventSource.last = this;
  }
  addEventListener() {}
  close() {}
  /** What the browser does when a stream closes for good. */
  fail() {
    this.readyState = 2;
    this.onerror?.();
  }
}

beforeEach(() => {
  roots = [{ slug: "n", path: "/n", kind: "notes" }];
  localStorage.clear();
  resetSessions();
  vi.stubGlobal("EventSource", FakeEventSource);
  vi.stubGlobal(
    "fetch",
    vi.fn((url: string) => {
      let body: unknown = {};
      if (url === "/api/roots") body = { roots };
      else if (url === "/api/r/n/tree") body = { name: "", path: "", dir: true, children: [] };
      else if (url === "/api/r/n/tags") body = { tags: [] };
      return Promise.resolve({
        ok: true,
        status: 200,
        statusText: "OK",
        json: () => Promise.resolve(body),
      } as Response);
    }),
  );
});
afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

function mount() {
  history.replaceState(null, "", "/r/n/");
  return render(
    <LocationProvider>
      <RootView slug="n" />
    </LocationProvider>,
  );
}

describe("a root removed under an open tab", () => {
  it("lands on the home page when the stream ends and the root is gone", async () => {
    mount();
    await waitFor(() => expect(screen.getByText("Select a note.")).toBeTruthy());

    roots = [];
    FakeEventSource.last!.fail();

    await waitFor(() => expect(location.pathname).toBe("/"));
  });

  it("stays put when the stream ended for any other reason", async () => {
    mount();
    await waitFor(() => expect(screen.getByText("Select a note.")).toBeTruthy());

    FakeEventSource.last!.fail();

    // The notice is in the page twice by design — once in the navigator and
    // once above the note — and the stylesheet shows one at each width.
    await waitFor(() =>
      expect(screen.getAllByText(/Live update is not available/).length).toBeGreaterThan(0),
    );
    expect(location.pathname).toBe("/r/n/");
  });
});
