/**
 * What the publish step sends, and in what order.
 *
 * This is the one thing in the repository that acts on publishing credentials
 * against somebody else's API, and the failure mode that matters is not a crash
 * — it is sending the right bytes to the wrong place, or publishing something
 * the store said it had refused. Neither shows up in a workflow that "passed".
 * So the whole of it runs against a fake Chrome Web Store: a real HTTP server
 * that records every request, and answers the way the documented API answers.
 *
 * davison/md-notes#135 (M8-R2).
 */

import { afterAll, beforeAll, beforeEach, describe, expect, it } from "vitest";
import { createServer } from "node:http";
import fs from "node:fs";
import os from "node:os";
import path from "node:path";

import {
  CREDENTIAL_NAMES,
  WebStoreError,
  credentialsFrom,
  main,
  parseArgs,
  report,
  uploadAndSubmit,
} from "./webstore.mjs";

const ITEM = "abcdefghijklmnopabcdefghijklmnop";
const CREDENTIALS = {
  clientId: "a-client.apps.googleusercontent.com",
  clientSecret: "a-secret",
  refreshToken: "a-refresh-token",
};

/** A fake store. Every request is recorded; `answers` decides the replies. */
let server;
let origin;
let seen;
let answers;

function reply(res, status, json) {
  res.writeHead(status, { "content-type": "application/json" });
  res.end(JSON.stringify(json));
}

beforeAll(async () => {
  server = createServer((req, res) => {
    const chunks = [];
    req.on("data", (c) => chunks.push(c));
    req.on("end", () => {
      const url = new URL(req.url, "http://fake.invalid");
      seen.push({
        method: req.method,
        pathname: url.pathname,
        query: Object.fromEntries(url.searchParams),
        headers: req.headers,
        body: Buffer.concat(chunks),
      });
      const answer = answers[`${req.method} ${url.pathname}`];
      if (answer === undefined) {
        reply(res, 404, { error: { message: `nothing fake answers ${req.method} ${url.pathname}` } });
        return;
      }
      reply(res, answer.status ?? 200, answer.json);
    });
  });
  await new Promise((r) => server.listen(0, "127.0.0.1", r));
  origin = `http://127.0.0.1:${server.address().port}`;
});

afterAll(async () => {
  await new Promise((r) => server.close(r));
});

beforeEach(() => {
  seen = [];
  answers = {
    "POST /token": { json: { access_token: "an-access-token", expires_in: 3599 } },
    [`PUT /upload/items/${ITEM}`]: {
      json: { kind: "chromewebstore#item", id: ITEM, uploadState: "SUCCESS" },
    },
    [`POST /api/items/${ITEM}/publish`]: {
      json: {
        kind: "chromewebstore#item",
        item_id: ITEM,
        status: ["OK"],
        statusDetail: ["Successfully created publish operation"],
      },
    },
  };
});

const endpoints = () => ({
  token: `${origin}/token`,
  upload: `${origin}/upload`,
  api: `${origin}/api`,
});

/** Near enough a zip: the local file header's signature and some bytes. */
const A_ZIP = Buffer.from([0x50, 0x4b, 0x03, 0x04, 0x14, 0x00, 0x08, 0x00, 0x2a, 0x2a]);

const run = (zip = A_ZIP) =>
  uploadAndSubmit({ itemId: ITEM, zip, credentials: CREDENTIALS, endpoints: endpoints() });

describe("uploading and submitting", () => {
  it("exchanges the refresh token, uploads, then publishes — in that order", async () => {
    const { uploaded, submitted } = await run();

    expect(seen.map((r) => `${r.method} ${r.pathname}`)).toEqual([
      "POST /token",
      `PUT /upload/items/${ITEM}`,
      `POST /api/items/${ITEM}/publish`,
    ]);
    expect(uploaded.uploadState).toBe("SUCCESS");
    expect(submitted.status).toEqual(["OK"]);
  });

  it("asks for an access token with the refresh-token grant, as a form", async () => {
    await run();
    const [token] = seen;
    expect(token.method).toBe("POST");
    expect(token.headers["content-type"]).toBe("application/x-www-form-urlencoded");
    expect(Object.fromEntries(new URLSearchParams(token.body.toString()))).toEqual({
      client_id: CREDENTIALS.clientId,
      client_secret: CREDENTIALS.clientSecret,
      refresh_token: CREDENTIALS.refreshToken,
      grant_type: "refresh_token",
    });
  });

  it("PUTs the zip itself to the item, with the access token and the API version", async () => {
    const zip = Buffer.concat([A_ZIP, Buffer.from("the actual bytes of the actual build")]);
    await run(zip);
    const upload = seen[1];
    expect(upload.method).toBe("PUT");
    expect(upload.pathname).toBe(`/upload/items/${ITEM}`);
    expect(upload.headers.authorization).toBe("Bearer an-access-token");
    expect(upload.headers["x-goog-api-version"]).toBe("2");
    // Byte for byte: a zip mangled in transit is a zip the store rejects for
    // reasons that look like anything but this.
    expect(upload.body.equals(zip)).toBe(true);
  });

  it("publishes to the public target, not to trusted testers", async () => {
    await run();
    const submit = seen[2];
    expect(submit.method).toBe("POST");
    expect(submit.pathname).toBe(`/api/items/${ITEM}/publish`);
    expect(submit.query).toEqual({ publishTarget: "default" });
    expect(submit.headers.authorization).toBe("Bearer an-access-token");
    expect(submit.headers["x-goog-api-version"]).toBe("2");
  });

  it("never sends the credentials to the store itself", async () => {
    await run();
    for (const request of seen.slice(1)) {
      const text = request.body.toString();
      expect(text).not.toContain(CREDENTIALS.clientSecret);
      expect(text).not.toContain(CREDENTIALS.refreshToken);
      expect(JSON.stringify(request.headers)).not.toContain(CREDENTIALS.refreshToken);
    }
  });
});

describe("when the store says no", () => {
  it("does not publish a package the store refused, and passes on its reasons", async () => {
    // The API answers 200 for a package it has rejected and says so in the
    // body. Reading the status alone would publish the *previous* package as a
    // new version, which is the worst outcome available here.
    answers[`PUT /upload/items/${ITEM}`] = {
      json: {
        kind: "chromewebstore#item",
        id: ITEM,
        uploadState: "FAILURE",
        itemError: [
          { error_code: "ITEM_NOT_UPDATABLE", error_detail: "Cannot update an item that is in review" },
        ],
      },
    };

    await expect(run()).rejects.toThrow(/refused the package: FAILURE/);
    await expect(run()).rejects.toThrow(/Cannot update an item that is in review/);
    expect(seen.some((r) => r.pathname.endsWith("/publish"))).toBe(false);
  });

  it("stops at a refused token exchange, saying which call failed", async () => {
    answers["POST /token"] = {
      status: 400,
      json: { error: "invalid_grant", error_description: "Token has been expired or revoked." },
    };

    await expect(run()).rejects.toThrow(
      /token exchange failed: HTTP 400 — Token has been expired or revoked\./,
    );
    expect(seen).toHaveLength(1);
  });

  it("reports a publish the store would not accept", async () => {
    answers[`POST /api/items/${ITEM}/publish`] = {
      status: 401,
      json: { error: { message: "Request had invalid authentication credentials." } },
    };

    await expect(run()).rejects.toThrow(/publish failed: HTTP 401 — Request had invalid authentication/);
  });

  it("refuses an item id that is not one, before reaching the network", async () => {
    for (const id of ["", "not-an-item-id", "ABCDEFGHIJKLMNOPABCDEFGHIJKLMNOP", `${ITEM}z`]) {
      await expect(
        uploadAndSubmit({ itemId: id, zip: A_ZIP, credentials: CREDENTIALS, endpoints: endpoints() }),
      ).rejects.toThrow(WebStoreError);
    }
    expect(seen).toHaveLength(0);
  });
});

describe("the command line", () => {
  let tmp;

  beforeAll(() => {
    tmp = fs.mkdtempSync(path.join(os.tmpdir(), "mdn-webstore-test-"));
  });

  afterAll(() => {
    fs.rmSync(tmp, { recursive: true, force: true });
  });

  const env = {
    CHROME_WEBSTORE_CLIENT_ID: CREDENTIALS.clientId,
    CHROME_WEBSTORE_CLIENT_SECRET: CREDENTIALS.clientSecret,
    CHROME_WEBSTORE_REFRESH_TOKEN: CREDENTIALS.refreshToken,
  };

  it("takes --item-id and --zip", () => {
    expect(parseArgs(["--item-id", ITEM, "--zip", "a.zip"])).toEqual({ "item-id": ITEM, zip: "a.zip" });
    expect(() => parseArgs(["--zip"])).toThrow(WebStoreError);
    expect(() => parseArgs(["a.zip"])).toThrow(WebStoreError);
  });

  it("names the credential that is missing rather than failing at the store", () => {
    for (const name of CREDENTIAL_NAMES) {
      expect(() => credentialsFrom({ ...env, [name]: "" })).toThrow(
        new RegExp(`not configured: ${name} is unset`),
      );
    }
    expect(() => credentialsFrom({})).toThrow(
      /CHROME_WEBSTORE_CLIENT_ID, CHROME_WEBSTORE_CLIENT_SECRET, CHROME_WEBSTORE_REFRESH_TOKEN are unset/,
    );
    expect(credentialsFrom(env)).toEqual(CREDENTIALS);
  });

  it("refuses to upload a zip that is not there, or is empty", async () => {
    const empty = path.join(tmp, "empty.zip");
    fs.writeFileSync(empty, "");
    await expect(main(["--item-id", ITEM, "--zip", empty], env)).rejects.toThrow(/is empty/);
    await expect(main(["--item-id", ITEM, "--zip", path.join(tmp, "nope.zip")], env)).rejects.toThrow();
    await expect(main(["--zip", empty], env)).rejects.toThrow(/--item-id is required/);
    expect(seen).toHaveLength(0);
  });
});

describe("the run summary", () => {
  it("says what was uploaded, what the store answered, and that review is the store's", () => {
    const lines = report({
      itemId: ITEM,
      uploaded: { uploadState: "SUCCESS" },
      submitted: { status: ["ITEM_PENDING_REVIEW"], statusDetail: ["Item is pending review"] },
    });
    const text = lines.join("\n");
    expect(text).toContain(ITEM);
    expect(text).toContain("SUCCESS");
    expect(text).toContain("ITEM_PENDING_REVIEW");
    expect(text).toContain("Item is pending review");
    expect(text).toContain("reviewed before it reaches anyone");
  });
});
