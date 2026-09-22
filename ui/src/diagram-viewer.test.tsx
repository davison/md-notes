import { afterEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render } from "@testing-library/preact";
import { DiagramViewer } from "./diagram-viewer";

afterEach(cleanup);

function show() {
  const opener = document.createElement("button");
  document.body.append(opener);
  const onClose = vi.fn();
  const view = render(
    <DiagramViewer src="/d.svg?theme=light" alt="graph LR; A-->B" width="1891" height="171" opener={opener} onClose={onClose} />,
  );
  return { opener, onClose, view };
}

describe("DiagramViewer", () => {
  it("shows the drawing at its natural size, with focus on the close button", () => {
    const { view } = show();
    const img = view.container.querySelector(".diagram-viewer img")!;
    expect(img.getAttribute("src")).toBe("/d.svg?theme=light");
    expect([img.getAttribute("width"), img.getAttribute("height")]).toEqual(["1891", "171"]);
    expect(img.getAttribute("alt")).toBe("graph LR; A-->B");
    expect(document.activeElement?.className).toBe("diagram-viewer-close");
  });

  it("closes on Escape, and no document listener underneath sees the key", () => {
    const { onClose } = show();
    const underneath = vi.fn();
    document.addEventListener("keydown", underneath, true);
    try {
      fireEvent.keyDown(document.activeElement!, { key: "Escape" });
    } finally {
      document.removeEventListener("keydown", underneath, true);
    }
    expect(onClose).toHaveBeenCalledOnce();
    expect(underneath).not.toHaveBeenCalled();
  });

  it("closes on a click beside the image, not on the image", () => {
    const { onClose, view } = show();
    fireEvent.click(view.container.querySelector(".diagram-viewer img")!);
    expect(onClose).not.toHaveBeenCalled();
    fireEvent.click(view.container.querySelector(".diagram-viewer-scroll")!);
    expect(onClose).toHaveBeenCalledOnce();
  });

  it("hands focus back to the diagram that opened it", () => {
    const { opener, view } = show();
    view.unmount();
    expect(document.activeElement).toBe(opener);
  });
});
