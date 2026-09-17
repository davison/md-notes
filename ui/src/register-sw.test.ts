import { describe, expect, it, vi } from "vitest";
import { registerServiceWorker } from "./register-sw";

function fakeWindow() {
  const listeners: Array<() => void> = [];
  return {
    addEventListener: (type: string, fn: () => void) => {
      if (type === "load") listeners.push(fn);
    },
    load: () => listeners.forEach((fn) => fn()),
  };
}

describe("registering the service worker", () => {
  it("registers after load, not before", () => {
    const register = vi.fn().mockResolvedValue({});
    const win = fakeWindow();
    registerServiceWorker({ serviceWorker: { register } } as unknown as Navigator, win as unknown as Window);
    expect(register).not.toHaveBeenCalled();
    win.load();
    expect(register).toHaveBeenCalledWith("/sw.js");
  });

  it("does nothing where the browser has no worker to register", () => {
    const win = fakeWindow();
    registerServiceWorker({} as Navigator, win as unknown as Window);
    expect(() => win.load()).not.toThrow();
  });

  it("swallows a registration the browser refuses", async () => {
    const register = vi.fn().mockRejectedValue(new Error("insecure origin"));
    const win = fakeWindow();
    registerServiceWorker({ serviceWorker: { register } } as unknown as Navigator, win as unknown as Window);
    win.load();
    await Promise.resolve();
    expect(register).toHaveBeenCalled();
  });
});
