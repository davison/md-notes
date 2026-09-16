import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/preact";
import { NewNoteDialog } from "./new-note";
import { resetSessions, takeCreated } from "./session";

/** A daemon that refuses a name already taken and creates anything else. */
const taken = new Set<string>(["docs/Taken.md"]);
const calls: { method: string; url: string; type: string | null; body: unknown }[] = [];

function mockApi() {
  vi.stubGlobal(
    "fetch",
    vi.fn((url: string, init?: RequestInit) => {
      const headers = (init?.headers ?? {}) as Record<string, string>;
      calls.push({ method: init?.method ?? "GET", url, type: headers["Content-Type"] ?? null, body: init?.body ?? null });
      const path = decodeURIComponent(url.replace("/api/r/n/source/", ""));
      const json = (status: number, body: unknown) =>
        Promise.resolve({ ok: status < 300, status, statusText: "S", json: () => Promise.resolve(body) } as Response);
      if (taken.has(path)) return json(409, { code: "exists", error: "a note by that name already exists" });
      return json(201, { root: "n", path, source: "", revision: "r1" });
    }),
  );
}

beforeEach(() => {
  calls.length = 0;
  resetSessions();
  mockApi();
});
afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

function mount(folder = "docs") {
  const created = vi.fn();
  const closed = vi.fn();
  render(<NewNoteDialog slug="n" folder={folder} onClose={closed} onCreated={created} />);
  return { created, closed };
}

const nameBox = () => screen.getByLabelText("Title or path") as HTMLInputElement;
const type = (text: string) => fireEvent.input(nameBox(), { target: { value: text } });
const submit = () => fireEvent.submit(document.querySelector(".modal form")!);

describe("NewNoteDialog", () => {
  it("creates a bare title in the selected folder and opens it", async () => {
    const { created } = mount("docs");
    type("Shopping");
    submit();
    await waitFor(() => expect(created).toHaveBeenCalledWith("docs/Shopping.md"));
    expect(calls).toHaveLength(1);
    expect(calls[0].method).toBe("POST");
    expect(calls[0].url).toBe("/api/r/n/source/docs/Shopping.md");
    // No body and no Content-Type: the daemon makes an empty note.
    expect(calls[0].body).toBeNull();
    expect(calls[0].type).toBeNull();
    // The pane that mounts next opens in the editor.
    expect(takeCreated("n", "docs/Shopping.md")).toBe(true);
  });

  it("takes a name with a slash as a path under the root", async () => {
    const { created } = mount("docs");
    type("work/log");
    submit();
    await waitFor(() => expect(created).toHaveBeenCalledWith("work/log.md"));
    expect(calls[0].url).toBe("/api/r/n/source/work/log.md");
  });

  it("routes to the daemon's own path, not the one it sent", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(() =>
        Promise.resolve({
          ok: true,
          status: 201,
          statusText: "Created",
          json: () => Promise.resolve({ root: "n", path: "docs/clean.md", source: "", revision: "r1" }),
        } as Response),
      ),
    );
    const { created } = mount("docs");
    type("./clean");
    submit();
    await waitFor(() => expect(created).toHaveBeenCalledWith("docs/clean.md"));
  });

  it("shows the daemon's refusal and keeps the name for correcting", async () => {
    const { created } = mount("docs");
    type("Taken");
    submit();
    await waitFor(() => expect(screen.getByRole("alert").textContent).toBe("a note by that name already exists"));
    expect(created).not.toHaveBeenCalled();
    expect(nameBox().value).toBe("Taken");
    // Correcting it and trying again works, and nothing was lost.
    type("Taken again");
    submit();
    await waitFor(() => expect(created).toHaveBeenCalledWith("docs/Taken again.md"));
  });

  it("refuses an empty name without asking the daemon", async () => {
    const { created } = mount("docs");
    type("   ");
    submit();
    await waitFor(() => expect(screen.getByRole("alert")).toBeTruthy());
    expect(calls).toHaveLength(0);
    expect(created).not.toHaveBeenCalled();
  });

  it("says which folder a bare title lands in", () => {
    mount("docs/deep");
    expect(document.querySelector(".modal-folder")!.textContent).toBe("docs/deep");
    cleanup();
    mount("");
    expect(document.querySelector(".modal-folder")!.textContent).toBe("the root of this folder");
  });

  it("writes nothing when it is cancelled", () => {
    const { closed, created } = mount("docs");
    type("Shopping");
    fireEvent.click(screen.getByText("Cancel"));
    expect(closed).toHaveBeenCalled();
    expect(calls).toHaveLength(0);
    expect(created).not.toHaveBeenCalled();
  });
});
