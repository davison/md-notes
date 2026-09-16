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
