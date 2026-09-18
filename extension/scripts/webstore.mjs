/**
 * Publishes a built extension zip to the Chrome Web Store.
 *
 *     node extension/scripts/webstore.mjs \
 *       --publisher-id <id> --item-id <id> --zip dist/mdn-extension-v0.1.0.zip
 *
 * Three calls, in this order and no other: exchange the refresh token for an
 * access token, POST the zip over the item's package, POST publish to submit it
 * for review. It is what `.github/workflows/publish-webstore.yml` runs when a
 * GitHub Release is published (davison/md-notes#135, M8-R2).
 *
 * ## Which API
 *
 * The **V2** API, `chromewebstore.googleapis.com`. V1
 * (`www.googleapis.com/chromewebstore/v1.1`) is deprecated: its reference opens
 * "The Chrome Web Store API (V1) is deprecated and will only be supported until
 * 15th October 2026" (developer.chrome.com/docs/webstore/api/v1), and the
 * current tutorial documents only V2
 * (developer.chrome.com/docs/webstore/using-api). The paths, the response
 * shapes and the enums below are from that tutorial, the REST reference under
 * developer.chrome.com/docs/webstore/api/reference/rest/v2/, and the service's
 * own discovery document, https://chromewebstore.googleapis.com/$discovery/rest?version=v2,
 * which is the authority where the prose disagrees with itself.
 *
 * V2 addresses an item as `publishers/<publisherId>/items/<itemId>`, so it
 * needs a publisher id that V1 did not: it is in the developer dashboard under
 * **Publisher > Settings**.
 *
 * Nothing here has been run against the real store — there is no account to run
 * it against, and a publishing credential is not a thing to experiment with.
 * The request shaping is tested against a fake store in `webstore.test.mjs`,
 * built from the documented shapes above.
 *
 * ## Credentials
 *
 * From the environment, never the command line, because a command line is
 * visible to every process on the machine:
 *
 *   CHROME_WEBSTORE_CLIENT_ID       OAuth client id      (repository secret)
 *   CHROME_WEBSTORE_CLIENT_SECRET   OAuth client secret  (repository secret)
 *   CHROME_WEBSTORE_REFRESH_TOKEN   OAuth refresh token  (repository secret)
 *
 * The publisher id and the item id are arguments rather than secrets: both are
 * account identifiers rather than credentials, neither grants anything without
 * the token above, and a secret would be masked out of the log line that says
 * which item was uploaded to (davison/md-notes#135).
 *
 * Written by hand rather than through `chrome-webstore-upload-cli` because this
 * is the one job in the repository that holds publishing credentials: what it
 * sends, and where, should be something this repository's own tests can hold.
 */

import fs from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";

/** Google's OAuth 2 token endpoint. */
export const TOKEN_ENDPOINT = "https://oauth2.googleapis.com/token";

/** The Web Store V2 service. Upload goes to `/upload/v2`, everything to `/v2`. */
export const API_ROOT = "https://chromewebstore.googleapis.com";

/** The scope a refresh token for this API carries. */
export const SCOPE = "https://www.googleapis.com/auth/chromewebstore";

/** The names the workflow and the documentation both use. */
export const CREDENTIAL_NAMES = [
  "CHROME_WEBSTORE_CLIENT_ID",
  "CHROME_WEBSTORE_CLIENT_SECRET",
  "CHROME_WEBSTORE_REFRESH_TOKEN",
];

/** An item id is 32 lowercase letters, a–p. */
const ITEM_ID = /^[a-p]{32}$/;

/**
 * The `ItemState` a publish can answer with, and what each one means — the
 * enum and the wording are the discovery document's.
 *
 * Only the first four mean the submission was accepted. The store answers HTTP
 * 200 for the other three, so a caller that reads the status code alone reports
 * a version as submitted that was not: that is the failure this table exists to
 * prevent (davison/md-notes#151, review finding 1).
 */
export const ITEM_STATES = {
  PENDING_REVIEW: { accepted: true, means: "the item is pending review" },
  STAGED: { accepted: true, means: "the item has been approved and is ready to be published" },
  PUBLISHED: { accepted: true, means: "the item is published publicly" },
  PUBLISHED_TO_TESTERS: { accepted: true, means: "the item is published to trusted testers" },
  REJECTED: { accepted: false, means: "the item has been rejected for publishing" },
  CANCELLED: { accepted: false, means: "the item submission has been cancelled" },
  ITEM_STATE_UNSPECIFIED: { accepted: false, means: "the store returned no state at all" },
};

/**
 * The `UploadState` an upload can answer with, from the same source.
 *
 * `IN_PROGRESS` is not a refusal — the package is still being processed, and
 * the reference says to poll `fetchStatus` — so it is the one state that is
 * neither an accept nor a failure. The reference's prose spells it
 * `UPLOAD_IN_PROGRESS` in one sentence and `IN_PROGRESS` in its own enum;
 * both are treated as in progress rather than betting on which is the typo.
 */
export const UPLOAD_STATES = {
  SUCCEEDED: { accepted: true, means: "the upload succeeded" },
  IN_PROGRESS: { pending: true, means: "the upload is still being processed" },
  UPLOAD_IN_PROGRESS: { pending: true, means: "the upload is still being processed" },
  FAILED: { accepted: false, means: "the store could not accept the package" },
  NOT_FOUND: {
    accepted: false,
    means: "the store has no such item for this publisher — check the publisher id and the item id",
  },
  UPLOAD_STATE_UNSPECIFIED: { accepted: false, means: "the store returned no upload state at all" },
};

/**
 * The failure every call here raises: a sentence naming the call, and the
 * store's own words where it gave any. `detail` carries the parsed body when
 * there was one, so a caller can report more than the message.
 */
export class WebStoreError extends Error {
  constructor(message, detail = null) {
    super(message);
    this.name = "WebStoreError";
    this.detail = detail;
  }
}

/** How V2 addresses an item. */
export function itemName(publisherId, itemId) {
  return `publishers/${publisherId}/items/${itemId}`;
}

/** Reads a response as JSON, or raises with the status and the text it sent. */
async function body(response, what) {
  const text = await response.text();
  let parsed = null;
  try {
    parsed = text === "" ? null : JSON.parse(text);
  } catch {
    parsed = null;
  }
  if (!response.ok) {
    // Google's APIs answer `{"error": {"code", "message", "status"}}`; the
    // OAuth endpoint answers `{"error", "error_description"}`.
    const said = parsed?.error_description ?? parsed?.error?.message ?? text.trim();
    throw new WebStoreError(
      `${what} failed: HTTP ${response.status}${said === "" ? "" : ` — ${said}`}`,
      parsed,
    );
  }
  if (parsed === null) throw new WebStoreError(`${what} answered ${response.status} with no JSON body`);
  return parsed;
}

/**
 * Exchanges the refresh token for an access token.
 *
 * @param {{clientId: string, clientSecret: string, refreshToken: string,
 *          endpoint?: string, fetch?: typeof globalThis.fetch}} options
 * @returns {Promise<string>}
 */
export async function accessToken({
  clientId,
  clientSecret,
  refreshToken,
  endpoint = TOKEN_ENDPOINT,
  fetch = globalThis.fetch,
}) {
  const response = await fetch(endpoint, {
    method: "POST",
    headers: { "content-type": "application/x-www-form-urlencoded" },
    body: new URLSearchParams({
      client_id: clientId,
      client_secret: clientSecret,
      refresh_token: refreshToken,
      grant_type: "refresh_token",
    }).toString(),
  });
  const json = await body(response, "the token exchange");
  if (typeof json.access_token !== "string" || json.access_token === "") {
    throw new WebStoreError("the token exchange returned no access_token", json);
  }
  return json.access_token;
}

/**
 * Replaces the item's package with `zip`.
 *
 * `POST /upload/v2/publishers/<p>/items/<i>:upload` with the package as the
 * whole body — the tutorial's `curl -X POST -T <file>`. The API answers 200
 * with `{name, itemId, crxVersion, uploadState}` whatever it thought of the
 * package, so `uploadState` is what decides, not the status code.
 *
 * @param {{publisherId: string, itemId: string, zip: Uint8Array, token: string,
 *          root?: string, fetch?: typeof globalThis.fetch}} options
 */
export async function upload({
  publisherId,
  itemId,
  zip,
  token,
  root = API_ROOT,
  fetch = globalThis.fetch,
}) {
  const response = await fetch(`${root}/upload/v2/${itemName(publisherId, itemId)}:upload`, {
    method: "POST",
    headers: {
      authorization: `Bearer ${token}`,
      "content-type": "application/zip",
    },
    body: zip,
  });
  return await body(response, "the upload");
}

/**
 * The item's current status, including how the last upload ended.
 *
 * `GET /v2/publishers/<p>/items/<i>:fetchStatus`. Used to follow an upload the
 * store answered `IN_PROGRESS` for, which the reference says to poll for.
 */
export async function fetchStatus({
  publisherId,
  itemId,
  token,
  root = API_ROOT,
  fetch = globalThis.fetch,
}) {
  const response = await fetch(`${root}/v2/${itemName(publisherId, itemId)}:fetchStatus`, {
    method: "GET",
    headers: { authorization: `Bearer ${token}` },
  });
  return await body(response, "the status check");
}

/**
 * Submits the item's uploaded package for review.
 *
 * `POST /v2/publishers/<p>/items/<i>:publish`. On the Web Store, publishing
 * *is* submitting: the item goes out when a reviewer passes it, which is why
 * this reports what it was told rather than waiting for anything.
 *
 * `publishType` is sent explicitly although `DEFAULT_PUBLISH` is the
 * documented default: this workflow means "publish when approved", and a
 * default that changes under us would change what a release does silently.
 */
export async function publish({
  publisherId,
  itemId,
  token,
  publishType = "DEFAULT_PUBLISH",
  root = API_ROOT,
  fetch = globalThis.fetch,
}) {
  const response = await fetch(`${root}/v2/${itemName(publisherId, itemId)}:publish`, {
    method: "POST",
    headers: {
      authorization: `Bearer ${token}`,
      "content-type": "application/json",
    },
    body: JSON.stringify({ publishType }),
  });
  return await body(response, "the publish");
}

/** Waits, unless a test has handed us something that does not. */
const wait = (ms) => new Promise((resolve) => setTimeout(resolve, ms));

/**
 * An upload the store is still processing, followed to its end.
 *
 * The reference: "If `upload_state` is `UPLOAD_IN_PROGRESS`, you can poll for
 * updates using the fetchStatus method." `lastAsyncUploadState` is where
 * `fetchStatus` reports it.
 */
async function settleUpload({ publisherId, itemId, token, root, fetch, sleep, attempts, intervalMs }) {
  for (let attempt = 1; attempt <= attempts; attempt++) {
    await sleep(intervalMs);
    const status = await fetchStatus({ publisherId, itemId, token, root, fetch });
    const state = status.lastAsyncUploadState;
    if (UPLOAD_STATES[state]?.pending !== true) return { state, status };
  }
  return { state: null, status: null };
}

/**
 * Uploads `zip` to the item and submits it, refusing at the first thing the
 * store did not accept.
 *
 * @returns {Promise<{uploaded: object, submitted: object}>}
 */
export async function uploadAndSubmit({
  publisherId,
  itemId,
  zip,
  credentials,
  expectVersion = null,
  endpoints = {},
  fetch = globalThis.fetch,
  sleep = wait,
  attempts = 20,
  intervalMs = 15000,
}) {
  if (!ITEM_ID.test(itemId)) {
    throw new WebStoreError(
      `"${itemId}" is not a Chrome Web Store item id: 32 letters a-p, the last segment of the item's store URL`,
    );
  }
  if (typeof publisherId !== "string" || publisherId.trim() === "") {
    throw new WebStoreError(
      "no publisher id: it is in the developer dashboard under Publisher > Settings, and the V2 API addresses every item through it",
    );
  }
  const root = endpoints.root ?? API_ROOT;
  const token = await accessToken({ ...credentials, endpoint: endpoints.token, fetch });

  const uploaded = await upload({ publisherId, itemId, zip, token, root, fetch });
  let state = uploaded.uploadState;
  let version = uploaded.crxVersion;
  if (UPLOAD_STATES[state]?.pending === true) {
    const settled = await settleUpload({ publisherId, itemId, token, root, fetch, sleep, attempts, intervalMs });
    if (settled.state === null) {
      throw new WebStoreError(
        `the store was still processing the package after ${attempts} checks; it may yet succeed, so look at the dashboard before uploading again`,
        uploaded,
      );
    }
    state = settled.state;
    version = settled.status?.submittedItemRevisionStatus?.distributionChannels?.[0]?.crxVersion ?? version;
  }
  if (UPLOAD_STATES[state]?.accepted !== true) {
    throw new WebStoreError(
      `the store did not accept the package: ${state ?? "no upload state"}${meaning(UPLOAD_STATES, state)}`,
      uploaded,
    );
  }
  if (expectVersion !== null && typeof version === "string" && version !== expectVersion) {
    throw new WebStoreError(
      `the store read version ${version} out of the package, but this release is ${expectVersion}: the wrong zip was uploaded, and nothing has been submitted`,
      uploaded,
    );
  }

  const submitted = await publish({ publisherId, itemId, token, root, fetch });
  const published = submitted.state;
  if (ITEM_STATES[published]?.accepted !== true) {
    // HTTP 200 with a state that means the submission did not happen. Reading
    // the status code alone here reports a version as submitted that was not.
    throw new WebStoreError(
      `the store did not accept the submission: ${published ?? "no state"}${meaning(ITEM_STATES, published)}${warnings(submitted)}`,
      submitted,
    );
  }
  return { uploaded: { ...uploaded, uploadState: state, crxVersion: version }, submitted };
}

/** " — what the enum says it means", or nothing for a value we do not know. */
function meaning(table, value) {
  const known = table[value];
  return known === undefined ? "" : ` — ${known.means}`;
}

/** The store's non-blocking warnings, as a sentence, or nothing. */
function warnings(submitted) {
  const list = submitted?.warningInfo?.warnings;
  if (!Array.isArray(list) || list.length === 0) return "";
  const said = list
    .map((w) => [w.reason, w.description].filter(Boolean).join(": "))
    .filter((s) => s !== "");
  return said.length === 0 ? "" : ` (warnings: ${said.join("; ")})`;
}

/** What the store said, as the lines a run summary wants. */
export function report({ publisherId, itemId, uploaded, submitted }) {
  const state = submitted.state ?? "no state";
  const list = submitted?.warningInfo?.warnings ?? [];
  return [
    `Uploaded to \`${itemName(publisherId, itemId)}\`: ${uploaded.uploadState}${
      uploaded.crxVersion === undefined ? "" : `, version ${uploaded.crxVersion}`
    }.`,
    `Submitted for review: **${state}**${meaning(ITEM_STATES, state)}.`,
    ...list.map((w) => `- warning: ${[w.reason, w.description].filter(Boolean).join(" — ")}`),
    "",
    "A new version is reviewed before it reaches anyone; the store decides when,",
    "and this run reports only that it was submitted.",
    `<https://chromewebstore.google.com/detail/${itemId}>`,
  ];
}

/** `--name value` pairs; anything else is an error worth naming. */
export function parseArgs(argv) {
  const args = {};
  for (let i = 0; i < argv.length; i += 2) {
    const flag = argv[i];
    if (!flag.startsWith("--") || argv[i + 1] === undefined) {
      throw new WebStoreError(`unexpected argument: ${flag}`);
    }
    args[flag.slice(2)] = argv[i + 1];
  }
  return args;
}

/** Everything the run needs, or a message naming precisely what is missing. */
export function credentialsFrom(env) {
  const missing = CREDENTIAL_NAMES.filter((name) => (env[name] ?? "") === "");
  if (missing.length > 0) {
    throw new WebStoreError(
      `not configured: ${missing.join(", ")} ${missing.length === 1 ? "is" : "are"} unset. ` +
        "They are repository secrets; davison/md-notes#135 records how they are made.",
    );
  }
  return {
    clientId: env.CHROME_WEBSTORE_CLIENT_ID,
    clientSecret: env.CHROME_WEBSTORE_CLIENT_SECRET,
    refreshToken: env.CHROME_WEBSTORE_REFRESH_TOKEN,
  };
}

/**
 * The command line: `--publisher-id <id> --item-id <id> --zip <path>`, and
 * optionally `--expect-version <x.y.z>`, the version the package should carry.
 *
 * @returns {Promise<string[]>} the lines to report
 */
export async function main(argv, env = process.env, options = {}) {
  const args = parseArgs(argv);
  for (const required of ["publisher-id", "item-id", "zip"]) {
    if ((args[required] ?? "") === "") throw new WebStoreError(`--${required} is required`);
  }
  const zip = fs.readFileSync(args.zip);
  if (zip.length === 0) throw new WebStoreError(`${args.zip} is empty`);
  const { uploaded, submitted } = await uploadAndSubmit({
    publisherId: args["publisher-id"],
    itemId: args["item-id"],
    zip,
    expectVersion: args["expect-version"] ?? null,
    credentials: credentialsFrom(env),
    ...options,
  });
  return report({ publisherId: args["publisher-id"], itemId: args["item-id"], uploaded, submitted });
}

// Run only when this file is the program, so the tests can import it.
if (process.argv[1] !== undefined && path.resolve(process.argv[1]) === fileURLToPath(import.meta.url)) {
  try {
    const lines = await main(process.argv.slice(2));
    console.log(lines.join("\n"));
    const summary = process.env.GITHUB_STEP_SUMMARY;
    if (summary !== undefined && summary !== "") fs.appendFileSync(summary, `${lines.join("\n")}\n`);
  } catch (error) {
    console.error(error instanceof WebStoreError ? error.message : error);
    const summary = process.env.GITHUB_STEP_SUMMARY;
    if (summary !== undefined && summary !== "") {
      fs.appendFileSync(summary, `**Not submitted to the Chrome Web Store.** ${error.message}\n`);
    }
    process.exit(1);
  }
}
