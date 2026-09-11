import { describe, expect, it } from "vitest";
import {
  CLIP_MENU,
  buildClipRequest,
  cleanTitle,
  describeClipFailure,
  menuKind,
  MAX_TITLE,
} from "./clip";
import { DaemonError } from "./daemon";
import type { Extraction } from "./extraction";
import type { Settings } from "./settings";

const page: Extraction = {
  kind: "page",
  url: "https://example.com/a",
  title: "The Cost of Abstraction",
  markdown: "# Heading\n\nBody.",
};

const withToken: Settings = { daemonUrl: "http://localhost:7337", token: "s3cret" };
const withoutToken: Settings = { daemonUrl: "http://localhost:7337", token: "" };

describe("buildClipRequest", () => {
  it("sends the url, kind and markdown, with one trailing newline", () => {
    expect(buildClipRequest(page, "")).toEqual({
      url: "https://example.com/a",
      title: "The Cost of Abstraction",
      markdown: "# Heading\n\nBody.\n",
      kind: "page",
    });
  });

  it("prefers the title the user edited", () => {
    expect(buildClipRequest(page, "  My  own   title ").title).toBe("My own title");
  });

  it("falls back to the page's title when the field was emptied", () => {
    expect(buildClipRequest(page, "   ").title).toBe("The Cost of Abstraction");
  });

  it("carries a selection as a selection", () => {
    expect(buildClipRequest({ ...page, kind: "selection" }, "x").kind).toBe("selection");
  });

  it("does not pile up newlines on markdown that already ends with one", () => {
    expect(buildClipRequest({ ...page, markdown: "Body.\n\n\n" }, "t").markdown).toBe("Body.\n");
  });

  it("keeps an empty clip empty rather than inventing content", () => {
    expect(buildClipRequest({ ...page, markdown: "" }, "t").markdown).toBe("\n");
  });
});

describe("cleanTitle", () => {
  it("collapses whitespace, including newlines", () => {
    expect(cleanTitle("A\n  long\ttitle ")).toBe("A long title");
  });

  it("bounds the title where the daemon bounds it", () => {
    expect(cleanTitle("x".repeat(400))).toHaveLength(MAX_TITLE);
  });
});

describe("describeClipFailure", () => {
  it("names an unreachable daemon and its address", () => {
    const failure = describeClipFailure(
      new DaemonError("unreachable", "daemon not reachable at http://localhost:7337 (TypeError)"),
      withToken,
    );
    expect(failure.kind).toBe("unreachable");
    expect(failure.message).toContain("not reachable at http://localhost:7337");
    expect(failure.message).toContain("mdn serve");
    expect(failure.offerOptions).toBe(true);
  });

  it("names a rejected token as a rejected token, quoting the daemon", () => {
    const failure = describeClipFailure(
      new DaemonError("token_rejected", "wrapped", 401, "invalid bearer token"),
      withToken,
    );
    expect(failure.kind).toBe("token_rejected");
    expect(failure.message).toContain("Token rejected: invalid bearer token");
    expect(failure.message).toContain("mdn token");
    expect(failure.offerOptions).toBe(true);
  });

  it("names a missing token when the extension knows it has none", () => {
    const failure = describeClipFailure(
      new DaemonError("no_token", "no token is configured for this daemon"),
      withoutToken,
    );
    expect(failure.kind).toBe("no_token");
    expect(failure.message).toContain("No token configured");
    expect(failure.offerOptions).toBe(true);
  });

  it("reads the daemon's cross_origin refusal as a missing token when none is stored", () => {
    const failure = describeClipFailure(
      new DaemonError("origin_refused", "wrapped", 403, "cross-origin request refused"),
      withoutToken,
    );
    expect(failure.message).toContain("No token configured");
    expect(failure.offerOptions).toBe(true);
  });

  it("says the origin was refused when a token was stored and still refused", () => {
    const failure = describeClipFailure(
      new DaemonError("origin_refused", "wrapped", 403, "cross-origin request refused"),
      withToken,
    );
    expect(failure.message).toContain("refused this extension's origin even with a token");
    expect(failure.message).toContain("cross-origin request refused");
  });

  it("passes any other refusal through in the daemon's own words", () => {
    const failure = describeClipFailure(
      new DaemonError("refused", "wrapped", 413, "markdown is larger than 8 MiB"),
      withToken,
    );
    expect(failure).toEqual({
      kind: "refused",
      message: "markdown is larger than 8 MiB",
      offerOptions: false,
    });
  });

  it("reports a reply that was not JSON without pointing at the options", () => {
    const failure = describeClipFailure(
      new DaemonError("bad_response", "the daemon's reply was not JSON"),
      withToken,
    );
    expect(failure).toEqual({
      kind: "bad_response",
      message: "the daemon's reply was not JSON",
      offerOptions: false,
    });
  });

  it("reports a failure that is not the daemon's at all", () => {
    expect(describeClipFailure(new Error("boom"), withToken)).toEqual({
      kind: "bad_response",
      message: "boom",
      offerOptions: false,
    });
    expect(describeClipFailure("odd", withToken).message).toBe("odd");
  });
});

describe("the context menu", () => {
  it("offers a page entry and a selection entry, each in its own context", () => {
    expect(CLIP_MENU.map((item) => [item.id, item.contexts])).toEqual([
      ["clip-page", ["page"]],
      ["clip-selection", ["selection"]],
    ]);
    for (const item of CLIP_MENU) expect(item.title).toMatch(/md-notes/);
  });

  it("maps a clicked entry to what it clips, and ignores anything else", () => {
    expect(menuKind("clip-page")).toBe("page");
    expect(menuKind("clip-selection")).toBe("selection");
    expect(menuKind("someone-elses-menu")).toBeNull();
    expect(menuKind(undefined)).toBeNull();
  });
});
