import { afterEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen } from "@testing-library/preact";
import { useState } from "preact/hooks";
import { Dialog } from "./dialog";

afterEach(cleanup);

function Harness({ onConfirm, onCancel }: { onConfirm: () => void; onCancel: () => void }) {
  const [open, setOpen] = useState(false);
  return (
    <>
      <button type="button" onClick={() => setOpen(true)}>
        Open
      </button>
      {open && (
        <Dialog
          title="A question"
          confirmLabel="Yes"
          onConfirm={onConfirm}
          onCancel={() => {
            setOpen(false);
            onCancel();
          }}
        >
          <input class="modal-name" aria-label="Name" />
        </Dialog>
      )}
    </>
  );
}

function open() {
  const confirm = vi.fn();
  const cancel = vi.fn();
  render(<Harness onConfirm={confirm} onCancel={cancel} />);
  const opener = screen.getByText("Open");
  opener.focus();
  fireEvent.click(opener);
  return { confirm, cancel, opener };
}

describe("Dialog", () => {
  it("is a modal dialog naming itself", () => {
    open();
    const box = document.querySelector(".modal")!;
    expect(box.getAttribute("role")).toBe("dialog");
    expect(box.getAttribute("aria-modal")).toBe("true");
    const label = document.getElementById(box.getAttribute("aria-labelledby")!);
    expect(label?.textContent).toBe("A question");
  });

  it("moves focus to the text box and hands it back on close", () => {
    const { cancel, opener } = open();
    expect(document.activeElement).toBe(screen.getByLabelText("Name"));
    fireEvent.click(screen.getByText("Cancel"));
    expect(cancel).toHaveBeenCalled();
    expect(document.activeElement).toBe(opener);
  });

  it("confirms on Enter, through the form", () => {
    const { confirm } = open();
    fireEvent.submit(document.querySelector(".modal form")!);
    expect(confirm).toHaveBeenCalledTimes(1);
  });

  it("cancels on Escape, and stops it reaching the handlers below", () => {
    const below = vi.fn();
    document.addEventListener("keydown", below, true);
    const { cancel } = open();
    fireEvent.keyDown(document.querySelector(".modal")!, { key: "Escape" });
    document.removeEventListener("keydown", below, true);
    expect(cancel).toHaveBeenCalledTimes(1);
    expect(below).not.toHaveBeenCalled();
    expect(document.querySelector(".modal")).toBeNull();
  });

  it("cancels on the backdrop", () => {
    const { cancel } = open();
    fireEvent.click(document.querySelector(".modal-backdrop")!);
    expect(cancel).toHaveBeenCalledTimes(1);
  });

  it("keeps Tab inside itself", () => {
    open();
    const items = [...document.querySelectorAll<HTMLElement>(".modal input, .modal button")];
    const last = items[items.length - 1];
    last.focus();
    fireEvent.keyDown(document.querySelector(".modal")!, { key: "Tab" });
    expect(document.activeElement).toBe(items[0]);
    fireEvent.keyDown(document.querySelector(".modal")!, { key: "Tab", shiftKey: true });
    expect(document.activeElement).toBe(last);
  });

  it("cannot be dismissed while a write is in flight, and still owns Escape", () => {
    // The dialog disables its controls to say "not now"; Escape and the
    // backdrop have to mean the same thing, or the one way out that still
    // works is the one that cannot be seen to be disabled. The keystroke is
    // still consumed here, so the drawer underneath does not answer it.
    const cancel = vi.fn();
    const below = vi.fn();
    document.addEventListener("keydown", below, true);
    render(
      <Dialog title="t" confirmLabel="Delete" busy onConfirm={() => {}} onCancel={cancel}>
        <p>body</p>
      </Dialog>,
    );
    fireEvent.keyDown(document.querySelector(".modal")!, { key: "Escape" });
    document.removeEventListener("keydown", below, true);
    expect(cancel).not.toHaveBeenCalled();
    expect(below).not.toHaveBeenCalled();
    fireEvent.click(document.querySelector(".modal-backdrop")!);
    expect(cancel).not.toHaveBeenCalled();
  });

  it("keeps focus and Tab inside itself when every control is disabled", () => {
    // A confirmation with a write in flight has no enabled control at all,
    // so there is nothing for the trap to cycle between and nothing holding
    // focus: without this the browser drops focus to the body and Tab walks
    // into the page behind the dialog.
    render(
      <Dialog title="t" confirmLabel="Delete" busy onConfirm={() => {}} onCancel={() => {}}>
        <p>body</p>
      </Dialog>,
    );
    const box = document.querySelector(".modal")!;
    expect(box.contains(document.activeElement)).toBe(true);
    const consumed = !fireEvent.keyDown(box, { key: "Tab" });
    expect(consumed).toBe(true);
    expect(document.activeElement).toBe(box);
  });

  it("does not confirm twice while a request is in flight", () => {
    const confirm = vi.fn();
    render(
      <Dialog title="t" confirmLabel="Yes" busy onConfirm={confirm} onCancel={() => {}}>
        <p>body</p>
      </Dialog>,
    );
    fireEvent.submit(document.querySelector(".modal form")!);
    expect(confirm).not.toHaveBeenCalled();
    expect((screen.getByText("Yes") as HTMLButtonElement).disabled).toBe(true);
  });
});
