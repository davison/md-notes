import { afterEach, beforeEach, describe, expect, it } from "vitest";
import { cleanup, fireEvent, render, screen } from "@testing-library/preact";
import { SettingsMenu } from "./settings-panel";
import { KEY, MOTION_ATTR, THEME_ATTR, reset, settings } from "./settings";

beforeEach(() => {
  cleanup();
  localStorage.clear();
  reset();
  document.documentElement.removeAttribute(THEME_ATTR);
  document.documentElement.removeAttribute(MOTION_ATTR);
});
afterEach(cleanup);

function open() {
  fireEvent.click(screen.getByLabelText("Display settings"));
}

describe("the settings panel", () => {
  it("is closed until the gear is pressed, and closes again on it", () => {
    render(<SettingsMenu />);
    const gear = screen.getByLabelText("Display settings");
    expect(gear.getAttribute("aria-expanded")).toBe("false");
    expect(screen.queryByRole("dialog")).toBeNull();
    fireEvent.click(gear);
    expect(gear.getAttribute("aria-expanded")).toBe("true");
    expect(screen.getByRole("dialog")).toBeTruthy();
    fireEvent.click(gear);
    expect(screen.queryByRole("dialog")).toBeNull();
  });

  it("applies and persists the light override, and takes it back", () => {
    render(<SettingsMenu />);
    open();
    const box = screen.getByLabelText(/Always use the light theme/) as HTMLInputElement;
    expect(box.checked).toBe(false);
    fireEvent.click(box);
    expect(document.documentElement.getAttribute(THEME_ATTR)).toBe("light");
    expect(JSON.parse(localStorage.getItem(KEY)!).light).toBe(true);
    expect((screen.getByLabelText(/Always use the light theme/) as HTMLInputElement).checked).toBe(true);
    fireEvent.click(screen.getByLabelText(/Always use the light theme/));
    expect(document.documentElement.hasAttribute(THEME_ATTR)).toBe(false);
    expect(JSON.parse(localStorage.getItem(KEY)!).light).toBe(false);
  });

  it("applies and persists the no-animation setting", () => {
    render(<SettingsMenu />);
    open();
    fireEvent.click(screen.getByLabelText(/No animation/));
    expect(document.documentElement.getAttribute(MOTION_ATTR)).toBe("none");
    expect(settings().noMotion).toBe(true);
    expect(JSON.parse(localStorage.getItem(KEY)!).noMotion).toBe(true);
  });

  it("shows what was stored before the page loaded", () => {
    localStorage.setItem(KEY, JSON.stringify({ light: true, noMotion: true }));
    reset();
    render(<SettingsMenu />);
    open();
    expect((screen.getByLabelText(/Always use the light theme/) as HTMLInputElement).checked).toBe(true);
    expect((screen.getByLabelText(/No animation/) as HTMLInputElement).checked).toBe(true);
  });

  it("closes on Escape and on a press outside it, but not on a press within", () => {
    render(<SettingsMenu />);
    open();
    fireEvent.pointerDown(screen.getByRole("dialog"));
    expect(screen.queryByRole("dialog")).toBeTruthy();
    fireEvent.keyDown(document, { key: "Escape" });
    expect(screen.queryByRole("dialog")).toBeNull();
    open();
    fireEvent.pointerDown(document.body);
    expect(screen.queryByRole("dialog")).toBeNull();
  });
});
