/**
 * The manifest is the extension's permission contract with the user, so it is
 * asserted rather than reviewed: M3-R5 asks for the least it needs, with host
 * permission limited to the daemon's configured origin.
 */
import { readFileSync, existsSync } from "node:fs";
import { fileURLToPath } from "node:url";
import { dirname, resolve } from "node:path";
import { describe, expect, it } from "vitest";
import { DEFAULT_DAEMON_URL, originPattern } from "./settings";

const here = dirname(fileURLToPath(import.meta.url));
const root = resolve(here, "..");
const manifest = JSON.parse(readFileSync(resolve(root, "public/manifest.json"), "utf8")) as {
  manifest_version: number;
  permissions: string[];
  host_permissions: string[];
  optional_host_permissions: string[];
  background: { service_worker: string; type: string };
  action: { default_popup: string; default_icon: Record<string, string> };
  options_ui: { page: string };
  icons: Record<string, string>;
  content_scripts?: unknown;
  web_accessible_resources?: unknown;
};

describe("manifest", () => {
  it("is Manifest V3 with a module service worker", () => {
    expect(manifest.manifest_version).toBe(3);
    expect(manifest.background).toEqual({ service_worker: "background.js", type: "module" });
  });

  it("asks for exactly the four API permissions the extension uses", () => {
    // storage: the options page and the per-tab status. contextMenus: the two
    // clip entries. activeTab: the page being clipped, and only at the moment
    // the user invokes the extension on it. scripting: injecting the converter
    // into that page, which activeTab alone does not allow.
    expect([...manifest.permissions].sort()).toEqual([
      "activeTab",
      "contextMenus",
      "scripting",
      "storage",
    ]);
  });

  it("asks for no permission that would expose every tab or navigation", () => {
    for (const broad of ["tabs", "webNavigation", "webRequest", "history", "downloads", "cookies"]) {
      expect(manifest.permissions).not.toContain(broad);
    }
  });

  it("limits host access to the default daemon origin and local files", () => {
    expect([...manifest.host_permissions].sort()).toEqual([
      "file:///*",
      originPattern("http://127.0.0.1:7337"),
      originPattern(DEFAULT_DAEMON_URL),
    ].sort());
  });

  it("names a port on every http host permission, so no other local service is in reach", () => {
    for (const pattern of manifest.host_permissions) {
      if (pattern.startsWith("file:")) continue;
      expect(pattern).toMatch(/^https?:\/\/[^/*]+:\d+\/\*$/);
    }
    expect(manifest.host_permissions).not.toContain("<all_urls>");
  });

  it("keeps any other origin optional, for a daemon that is not on the default port", () => {
    expect([...manifest.optional_host_permissions].sort()).toEqual(["http://*/*", "https://*/*"]);
  });

  it("registers no content script and exposes no resource to web pages", () => {
    expect(manifest.content_scripts).toBeUndefined();
    expect(manifest.web_accessible_resources).toBeUndefined();
  });

  it("points at pages and icons that exist in the source tree", () => {
    for (const file of [manifest.action.default_popup, manifest.options_ui.page]) {
      expect(existsSync(resolve(root, file))).toBe(true);
    }
    for (const icon of [...Object.values(manifest.icons), ...Object.values(manifest.action.default_icon)]) {
      expect(existsSync(resolve(root, "public", icon))).toBe(true);
    }
  });
});
