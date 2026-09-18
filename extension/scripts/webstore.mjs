/**
 * Publishes a built extension zip to the Chrome Web Store.
 *
 *     node extension/scripts/webstore.mjs --item-id <id> --zip dist/mdn-extension-v0.1.0.zip
 *
 * Three calls, in this order and no other: exchange the refresh token for an
 * access token, PUT the zip over the item's package, POST publish to submit it
 * for review. It is what `.github/workflows/publish-webstore.yml` runs when a
 * GitHub Release is published (davison/md-notes#135, M8-R2).
 *
 * The credentials come from the environment, never the command line, because a
 * command line is visible to every process on the machine and a workflow log
 * masks a secret in a `run:` line only if it recognises it:
 *
 *   CHROME_WEBSTORE_CLIENT_ID       OAuth client id      (repository secret)
 *   CHROME_WEBSTORE_CLIENT_SECRET   OAuth client secret  (repository secret)
 *   CHROME_WEBSTORE_REFRESH_TOKEN   OAuth refresh token  (repository secret)
 *
 * The item id is an argument rather than a secret: it is the last segment of
 * the store URL every install link is built from, so it is public, and a secret
 * would be masked out of the log exactly where someone is reading it to find
 * out which item was uploaded to (davison/md-notes#135).
 *
 * Written by hand rather than through `chrome-webstore-upload-cli` because this
 * is the one job in the repository that holds publishing credentials: what it
 * sends, and where, should be something this repository's own tests can hold —
 * and they do, against a fake server, in `webstore.test.mjs`.
 */

import fs from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";

/** Google's OAuth 2 token endpoint. */
export const TOKEN_ENDPOINT = "https://oauth2.googleapis.com/token";
/** The Web Store's upload endpoint; a package PUT goes here. */
export const UPLOAD_ENDPOINT = "https://www.googleapis.com/upload/chromewebstore/v1.1";
/** The Web Store's ordinary API endpoint; publish goes here. */
export const API_ENDPOINT = "https://www.googleapis.com/chromewebstore/v1.1";

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
 * The API answers 200 for a package it has refused, saying so in the body, so
 * a failed upload is a body to read rather than a status to check: `uploadState`
 * is SUCCESS, IN_PROGRESS, FAILURE or NOT_FOUND, with `itemError` carrying the
 * store's reasons.
 *
 * @param {{itemId: string, zip: Uint8Array, token: string,
 *          endpoint?: string, fetch?: typeof globalThis.fetch}} options
 */
export async function upload({ itemId, zip, token, endpoint = UPLOAD_ENDPOINT, fetch = globalThis.fetch }) {
  const response = await fetch(`${endpoint}/items/${itemId}`, {
    method: "PUT",
    headers: {
      authorization: `Bearer ${token}`,
      "x-goog-api-version": "2",
      "content-type": "application/zip",
    },
    body: zip,
  });
  const json = await body(response, "the upload");
  if (json.uploadState !== "SUCCESS") {
    throw new WebStoreError(
      `the store refused the package: ${json.uploadState ?? "no uploadState"}${errors(json)}`,
      json,
    );
  }
  return json;
}

/**
 * Submits the item's uploaded package for review.
 *
 * On the Web Store, publishing *is* submitting: an item that needs review
 * answers with a pending status and goes out when a reviewer passes it, which
 * is why this reports what it was told rather than waiting for anything.
 *
 * @param {{itemId: string, token: string, target?: string,
 *          endpoint?: string, fetch?: typeof globalThis.fetch}} options
 */
export async function publish({
  itemId,
  token,
  target = "default",
  endpoint = API_ENDPOINT,
  fetch = globalThis.fetch,
}) {
  const response = await fetch(`${endpoint}/items/${itemId}/publish?publishTarget=${target}`, {
    method: "POST",
    headers: {
      authorization: `Bearer ${token}`,
      "x-goog-api-version": "2",
      // The API wants a body it does not read; a POST with none is refused by
      // some of Google's front ends with a 411.
      "content-length": "0",
    },
  });
  return await body(response, "the publish");
}

/** The store's own reasons, as a sentence, or nothing. */
function errors(json) {
  const list = Array.isArray(json.itemError) ? json.itemError : [];
  const said = list
    .map((e) => [e.error_code, e.error_detail].filter(Boolean).join(": "))
    .filter((s) => s !== "");
  return said.length === 0 ? "" : ` — ${said.join("; ")}`;
}

/**
 * Uploads `zip` to `itemId` and submits it, returning what the store said.
 *
 * @returns {Promise<{uploaded: object, submitted: object}>}
 */
export async function uploadAndSubmit({ itemId, zip, credentials, endpoints = {}, fetch = globalThis.fetch }) {
  if (!ITEM_ID.test(itemId)) {
    throw new WebStoreError(
      `"${itemId}" is not a Chrome Web Store item id: 32 letters a-p, the last segment of the item's store URL`,
    );
  }
  const token = await accessToken({ ...credentials, endpoint: endpoints.token, fetch });
  const uploaded = await upload({ itemId, zip, token, endpoint: endpoints.upload, fetch });
  const submitted = await publish({ itemId, token, endpoint: endpoints.api, fetch });
  return { uploaded, submitted };
}

/** What the store said, as the lines a run summary wants. */
export function report({ itemId, uploaded, submitted }) {
  const status = Array.isArray(submitted.status) ? submitted.status.join(", ") : String(submitted.status ?? "");
  const detail = Array.isArray(submitted.statusDetail) ? submitted.statusDetail : [];
  return [
    `Uploaded to item \`${itemId}\`: ${uploaded.uploadState}.`,
    `Submitted for review: **${status === "" ? "no status" : status}**.`,
    ...detail.map((d) => `- ${d}`),
    "",
    "A new version is reviewed before it reaches anyone; the store decides when,",
    "and this run reports only that it was submitted.",
    `<https://chrome.google.com/webstore/detail/${itemId}>`,
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
 * The command line: `--item-id <id> --zip <path>`.
 *
 * @returns {Promise<string[]>} the lines to report
 */
export async function main(argv, env = process.env, fetch = globalThis.fetch) {
  const args = parseArgs(argv);
  for (const required of ["item-id", "zip"]) {
    if ((args[required] ?? "") === "") throw new WebStoreError(`--${required} is required`);
  }
  const zip = fs.readFileSync(args.zip);
  if (zip.length === 0) throw new WebStoreError(`${args.zip} is empty`);
  const { uploaded, submitted } = await uploadAndSubmit({
    itemId: args["item-id"],
    zip,
    credentials: credentialsFrom(env),
    fetch,
  });
  return report({ itemId: args["item-id"], uploaded, submitted });
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
    process.exit(1);
  }
}
