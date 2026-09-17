import { describe, expect, it } from "vitest";
import { classify, type RequestFacts } from "./sw-policy";

function facts(over: Partial<RequestFacts> = {}): RequestFacts {
  return { method: "GET", mode: "no-cors", sameOrigin: true, pathname: "/", ...over };
}

describe("the service worker's routing", () => {
  it("never answers anything under /api/", () => {
    for (const pathname of [
      "/api",
      "/api/roots",
      "/api/r/notes/tree",
      "/api/r/notes/events",
      "/api/r/notes/source/alpha.md",
      "/api/r/notes/raw/image.png",
    ]) {
      expect(classify(facts({ pathname })), pathname).toBe("network");
    }
  });

  it("leaves the tailnet login form to the network", () => {
    expect(classify(facts({ pathname: "/login" }))).toBe("network");
    expect(classify(facts({ pathname: "/login", mode: "navigate" }))).toBe("network");
  });

  it("leaves everything that is not a same-origin GET to the network", () => {
    for (const method of ["PUT", "POST", "DELETE", "HEAD"]) {
      expect(classify(facts({ method, pathname: "/index.html" })), method).toBe("network");
    }
    expect(classify(facts({ sameOrigin: false, pathname: "/assets/index-abc.js" }))).toBe("network");
  });

  it("takes the hashed assets from the cache first", () => {
    expect(classify(facts({ pathname: "/assets/index-BaxrYMo-.js" }))).toBe("asset");
    expect(classify(facts({ pathname: "/assets/editor-xUy3Me0R.js" }))).toBe("asset");
  });

  it("treats every client-side route as the shell", () => {
    for (const pathname of ["/", "/r/notes/", "/r/notes/docs/guide.md", "/nonesuch"]) {
      expect(classify(facts({ pathname, mode: "navigate" })), pathname).toBe("shell");
    }
  });

  it("treats the manifest and the icons as ordinary static files", () => {
    for (const pathname of ["/manifest.webmanifest", "/icon-192.png", "/icon.svg"]) {
      expect(classify(facts({ pathname })), pathname).toBe("static");
    }
  });
});
