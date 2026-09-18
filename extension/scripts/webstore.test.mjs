/**
 * What the publish step sends, and what it does with what comes back.
 *
 * This is the one thing in the repository that acts on publishing credentials
 * against somebody else's API, and the failure mode that matters is not a crash
 * — it is sending the right bytes to the wrong place, or reporting a version as
 * submitted when the store said it was not. Neither shows up in a workflow that
 * "passed". So the whole of it runs against a fake Chrome Web Store: a real
 * HTTP server that records every request and answers the way the V2 API's own
 * discovery document says the real one answers.
 *
 * The shapes below — paths, response fields, and every enum value — are from
 * https://chromewebstore.googleapis.com/$discovery/rest?version=v2 and the
 * reference under developer.chrome.com/docs/webstore/api/reference/rest/v2/.
 * Nothing here has ever touched the real store.
 *
 * davison/md-notes#135 (M8-R2), reworked for the V2 API and the publish-status
 * findings of the review on davison/md-notes#151.
 */

import { afterAll, beforeAll, beforeEach, describe, expect, it } from "vitest";
import { createServer } from "node:http";
import fs from "node:fs";
import os from "node:os";
import path from "node:path";

import {
  CREDENTIAL_NAMES,
  ITEM_STATES,
  UPLOAD_STATES,
  WebStoreError,
  credentialsFrom,
  itemName,
  main,
  parseArgs,
  report,
  uploadAndSubmit,
} from "./webstore.mjs";

const PUBLISHER = "1234567890123456789";
const ITEM = "abcdefghijklmnopabcdefghijklmnop";
const NAME = `publishers/${PUBLISHER}/items/${ITEM}`;
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
      const key = `${req.method} ${url.pathname}`;
      const answer = answers[key];
      if (answer === undefined) {
        reply(res, 404, { error: { code: 404, message: `nothing fake answers ${key}` } });
        return;
      }
      // An array of answers is consumed one per call, so a poll can be given a
      // sequence: still processing, still processing, done.
      const next = Array.isArray(answer) ? answer.shift() : answer;
      reply(res, next.status ?? 200, next.json);
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
    [`POST /upload/v2/${NAME}:upload`]: {
      json: { name: NAME, itemId: ITEM, crxVersion: "0.1.0", uploadState: "SUCCEEDED" },
    },
    [`POST /v2/${NAME}:publish`]: {
      json: { name: NAME, itemId: ITEM, state: "PENDING_REVIEW" },
    },
  };
});

/** Near enough a zip: the local file header's signature and some bytes. */
const A_ZIP = Buffer.from([0x50, 0x4b, 0x03, 0x04, 0x14, 0x00, 0x08, 0x00, 0x2a, 0x2a]);

const run = (overrides = {}) =>
  uploadAndSubmit({
    publisherId: PUBLISHER,
    itemId: ITEM,
    zip: A_ZIP,
    credentials: CREDENTIALS,
    endpoints: { token: `${origin}/token`, root: origin },
    // Nothing waits in a test; the poll's timing is not what is under test.
    sleep: () => Promise.resolve(),
    intervalMs: 0,
    ...overrides,
  });

const paths = () => seen.map((r) => `${r.method} ${r.pathname}`);

describe("the V2 endpoints", () => {
  it("exchanges the refresh token, uploads, then publishes — in that order", async () => {
    const { uploaded, submitted } = await run();

    expect(paths()).toEqual([
      "POST /token",
      `POST /upload/v2/${NAME}:upload`,
      `POST /v2/${NAME}:publish`,
    ]);
    expect(uploaded.uploadState).toBe("SUCCEEDED");
    expect(submitted.state).toBe("PENDING_REVIEW");
  });

  it("addresses the item through the publisher, as V2 requires", () => {
    // V1 addressed `items/<id>` and needed no publisher at all. Getting this
    // wrong is a 404 from an API that is otherwise configured correctly.
    expect(itemName(PUBLISHER, ITEM)).toBe(`publishers/${PUBLISHER}/items/${ITEM}`);
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

  it("POSTs the zip itself as the whole body, with the access token", async () => {
    const zip = Buffer.concat([A_ZIP, Buffer.from("the actual bytes of the actual build")]);
    await run({ zip });
    const upload = seen[1];
    expect(upload.method).toBe("POST");
    expect(upload.pathname).toBe(`/upload/v2/${NAME}:upload`);
    expect(upload.headers.authorization).toBe("Bearer an-access-token");
    // Byte for byte: a zip mangled in transit is a zip the store rejects for
    // reasons that look like anything but this.
    expect(upload.body.equals(zip)).toBe(true);
  });

  it("publishes with the type it means, rather than relying on the default", async () => {
    await run();
    const submit = seen[2];
    expect(submit.method).toBe("POST");
    expect(submit.pathname).toBe(`/v2/${NAME}:publish`);
    expect(submit.headers["content-type"]).toBe("application/json");
    expect(JSON.parse(submit.body.toString())).toEqual({ publishType: "DEFAULT_PUBLISH" });
    expect(submit.headers.authorization).toBe("Bearer an-access-token");
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

describe("a publish the store did not accept", () => {
  // The whole point of this block. `:publish` answers HTTP 200 and says what
  // happened in `state`; a caller that reads the status code alone reports a
  // version as submitted that was not, in a step that stays green.
  const refusals = Object.entries(ITEM_STATES)
    .filter(([, meaning]) => meaning.accepted !== true)
    .map(([state]) => state);

  it("covers every refusing state the reference lists", () => {
    expect(refusals.sort()).toEqual(["CANCELLED", "ITEM_STATE_UNSPECIFIED", "REJECTED"]);
  });

  for (const state of refusals) {
    it(`fails on ${state}, and says what it means`, async () => {
      answers[`POST /v2/${NAME}:publish`] = { json: { name: NAME, itemId: ITEM, state } };
      await expect(run()).rejects.toThrow(
        new RegExp(`did not accept the submission: ${state} — ${ITEM_STATES[state].means}`),
      );
    });
  }

  it("fails on a state the reference does not list at all", async () => {
    // The enum can grow. An unknown state is not an accepted one.
    answers[`POST /v2/${NAME}:publish`] = { json: { name: NAME, state: "SOMETHING_NEW" } };
    await expect(run()).rejects.toThrow(/did not accept the submission: SOMETHING_NEW/);
  });

  it("fails on a publish that answers 200 with no state", async () => {
    answers[`POST /v2/${NAME}:publish`] = { json: { name: NAME, itemId: ITEM } };
    await expect(run()).rejects.toThrow(/did not accept the submission: no state/);
  });

  it("carries the store's warnings into the failure", async () => {
    answers[`POST /v2/${NAME}:publish`] = {
      json: {
        name: NAME,
        state: "REJECTED",
        warningInfo: {
          warnings: [{ reason: "BROAD_HOST_PERMISSIONS", description: "Requests access to all hosts" }],
        },
      },
    };
    await expect(run()).rejects.toThrow(/BROAD_HOST_PERMISSIONS: Requests access to all hosts/);
  });

  for (const state of ["PENDING_REVIEW", "STAGED", "PUBLISHED", "PUBLISHED_TO_TESTERS"]) {
    it(`accepts ${state}`, async () => {
      answers[`POST /v2/${NAME}:publish`] = { json: { name: NAME, itemId: ITEM, state } };
      const { submitted } = await run();
      expect(submitted.state).toBe(state);
    });
  }
});

describe("an upload the store did not accept", () => {
  it("does not publish a package the store refused, and says why", async () => {
    // Same class of bug as the publish state, on the other half of the path:
    // the API answers 200 for a package it has rejected. Publishing anyway
    // would submit the *previous* package as a new version.
    answers[`POST /upload/v2/${NAME}:upload`] = {
      json: { name: NAME, itemId: ITEM, uploadState: "FAILED" },
    };
    await expect(run()).rejects.toThrow(/did not accept the package: FAILED — the store could not accept/);
    expect(paths().some((p) => p.endsWith(":publish"))).toBe(false);
  });

  it("reads NOT_FOUND as the wrong ids rather than as a refused package", async () => {
    answers[`POST /upload/v2/${NAME}:upload`] = {
      json: { name: NAME, uploadState: "NOT_FOUND" },
    };
    await expect(run()).rejects.toThrow(/NOT_FOUND — the store has no such item for this publisher/);
    expect(paths().some((p) => p.endsWith(":publish"))).toBe(false);
  });

  it("treats no upload state at all as a refusal", async () => {
    answers[`POST /upload/v2/${NAME}:upload`] = { json: { name: NAME } };
    await expect(run()).rejects.toThrow(/did not accept the package: no upload state/);
  });

  it("follows an upload that is still processing, then publishes", async () => {
    // IN_PROGRESS is not a refusal — the reference says to poll fetchStatus —
    // so the old message "the store refused the package: IN_PROGRESS" was
    // simply untrue.
    answers[`POST /upload/v2/${NAME}:upload`] = {
      json: { name: NAME, itemId: ITEM, uploadState: "IN_PROGRESS" },
    };
    answers[`GET /v2/${NAME}:fetchStatus`] = [
      { json: { name: NAME, lastAsyncUploadState: "IN_PROGRESS" } },
      {
        json: {
          name: NAME,
          lastAsyncUploadState: "SUCCEEDED",
          submittedItemRevisionStatus: {
            state: "PENDING_REVIEW",
            distributionChannels: [{ crxVersion: "0.1.0" }],
          },
        },
      },
    ];

    const { uploaded, submitted } = await run();
    expect(paths()).toEqual([
      "POST /token",
      `POST /upload/v2/${NAME}:upload`,
      `GET /v2/${NAME}:fetchStatus`,
      `GET /v2/${NAME}:fetchStatus`,
      `POST /v2/${NAME}:publish`,
    ]);
    expect(uploaded.uploadState).toBe("SUCCEEDED");
    expect(uploaded.crxVersion).toBe("0.1.0");
    expect(submitted.state).toBe("PENDING_REVIEW");
  });

  it("accepts the reference's other spelling of the in-progress state", async () => {
    // The reference's prose says UPLOAD_IN_PROGRESS where its own enum says
    // IN_PROGRESS. Betting on which is the typo is not worth a failed release.
    expect(UPLOAD_STATES.UPLOAD_IN_PROGRESS.pending).toBe(true);
    answers[`POST /upload/v2/${NAME}:upload`] = {
      json: { name: NAME, uploadState: "UPLOAD_IN_PROGRESS" },
    };
    answers[`GET /v2/${NAME}:fetchStatus`] = { json: { lastAsyncUploadState: "SUCCEEDED" } };
    const { uploaded } = await run();
    expect(uploaded.uploadState).toBe("SUCCEEDED");
  });

  it("fails on a processing upload that never settles, without publishing", async () => {
    answers[`POST /upload/v2/${NAME}:upload`] = { json: { name: NAME, uploadState: "IN_PROGRESS" } };
    answers[`GET /v2/${NAME}:fetchStatus`] = { json: { lastAsyncUploadState: "IN_PROGRESS" } };
    await expect(run({ attempts: 3 })).rejects.toThrow(/still processing the package after 3 checks/);
    expect(paths().filter((p) => p.endsWith(":fetchStatus"))).toHaveLength(3);
    expect(paths().some((p) => p.endsWith(":publish"))).toBe(false);
  });

  it("does not publish a package whose version is not this release's", async () => {
    // The store reads the version out of the manifest and hands it back, so
    // this is the authoritative check that the right zip was sent.
    await expect(run({ expectVersion: "0.2.0" })).rejects.toThrow(
      /store read version 0\.1\.0 out of the package, but this release is 0\.2\.0/,
    );
    expect(paths().some((p) => p.endsWith(":publish"))).toBe(false);
  });

  it("publishes when the version is the one expected", async () => {
    const { submitted } = await run({ expectVersion: "0.1.0" });
    expect(submitted.state).toBe("PENDING_REVIEW");
  });
});

describe("when the call itself fails", () => {
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

  it("passes on a Google API error body", async () => {
    answers[`POST /v2/${NAME}:publish`] = {
      status: 403,
      json: { error: { code: 403, message: "The caller does not have permission", status: "PERMISSION_DENIED" } },
    };
    await expect(run()).rejects.toThrow(/publish failed: HTTP 403 — The caller does not have permission/);
  });

  it("refuses an item id that is not one, before reaching the network", async () => {
    for (const id of ["", "not-an-item-id", "ABCDEFGHIJKLMNOPABCDEFGHIJKLMNOP", `${ITEM}z`]) {
      await expect(run({ itemId: id })).rejects.toThrow(WebStoreError);
    }
    expect(seen).toHaveLength(0);
  });

  it("refuses a missing publisher id, and says where to find one", async () => {
    for (const id of ["", "   ", undefined]) {
      await expect(run({ publisherId: id })).rejects.toThrow(/Publisher > Settings/);
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

  it("takes the publisher, the item, the zip and the version", () => {
    expect(
      parseArgs(["--publisher-id", PUBLISHER, "--item-id", ITEM, "--zip", "a.zip", "--expect-version", "0.1.0"]),
    ).toEqual({ "publisher-id": PUBLISHER, "item-id": ITEM, zip: "a.zip", "expect-version": "0.1.0" });
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

  it("refuses an argument that is missing, and a zip that is not there or empty", async () => {
    const empty = path.join(tmp, "empty.zip");
    fs.writeFileSync(empty, "");
    const args = ["--publisher-id", PUBLISHER, "--item-id", ITEM, "--zip", empty];
    await expect(main(args, env)).rejects.toThrow(/is empty/);
    await expect(
      main(["--publisher-id", PUBLISHER, "--item-id", ITEM, "--zip", path.join(tmp, "nope.zip")], env),
    ).rejects.toThrow();
    await expect(main(["--item-id", ITEM, "--zip", empty], env)).rejects.toThrow(
      /--publisher-id is required/,
    );
    await expect(main(["--publisher-id", PUBLISHER, "--zip", empty], env)).rejects.toThrow(
      /--item-id is required/,
    );
    expect(seen).toHaveLength(0);
  });

  it("carries its arguments through to the store", async () => {
    const zip = path.join(tmp, "package.zip");
    fs.writeFileSync(zip, A_ZIP);
    const lines = await main(
      ["--publisher-id", PUBLISHER, "--item-id", ITEM, "--zip", zip, "--expect-version", "0.1.0"],
      env,
      { endpoints: { token: `${origin}/token`, root: origin }, sleep: () => Promise.resolve() },
    );
    expect(paths()).toEqual([
      "POST /token",
      `POST /upload/v2/${NAME}:upload`,
      `POST /v2/${NAME}:publish`,
    ]);
    expect(lines.join("\n")).toContain("PENDING_REVIEW");
  });
});

describe("the run summary", () => {
  it("says what was uploaded, what the store answered, and that review is the store's", () => {
    const lines = report({
      publisherId: PUBLISHER,
      itemId: ITEM,
      uploaded: { uploadState: "SUCCEEDED", crxVersion: "0.1.0" },
      submitted: {
        state: "PENDING_REVIEW",
        warningInfo: { warnings: [{ reason: "A_WARNING", description: "worth reading" }] },
      },
    });
    const text = lines.join("\n");
    expect(text).toContain(NAME);
    expect(text).toContain("SUCCEEDED");
    expect(text).toContain("version 0.1.0");
    expect(text).toContain("PENDING_REVIEW");
    expect(text).toContain("the item is pending review");
    expect(text).toContain("A_WARNING — worth reading");
    expect(text).toContain("reviewed before it reaches anyone");
    expect(text).toContain(`https://chromewebstore.google.com/detail/${ITEM}`);
  });
});
