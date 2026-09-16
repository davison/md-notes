/**
 * How the extension reaches the daemon, and what that costs.
 *
 * A daemon on this machine and a daemon reached over the tailnet are the same
 * program behind two different rules. On loopback everything the daemon serves
 * is served, and a GET from the extension's background context — which carries
 * no `Origin` — passes the guard with no token at all. Under a `tailnet_host`
 * name nothing is served without proof the caller holds the token, and even
 * then only the allow-list's endpoints are reachable: reads, the source
 * writes and `POST /api/clip`, yes; `POST /api/roots`, no (see
 * `internal/server/tailnet.go`).
 *
 * So the configured URL decides two things — whether to present the bearer
 * token, and what to say when the daemon refuses. Both live here, pure, so
 * both are tested without a browser or a network.
 */

import { trimSlash } from "./paths";
import type { Settings } from "./settings";

/** A `127.0.0.0/8` literal: the whole block is the local machine, not just .1. */
const IPV4_LOOPBACK = /^127(?:\.(?:25[0-5]|2[0-4]\d|1\d\d|[1-9]?\d)){3}$/;

/**
 * True when the daemon URL names the machine the browser runs on.
 *
 * The names are the ones a browser treats as loopback: `localhost` and
 * anything under it, the whole of `127.0.0.0/8`, and `::1`. A URL that will
 * not parse is not loopback — the safe way round, since the only cost of
 * treating a daemon as remote is a bearer token the user configured for it
 * travelling to the address they typed, while the reverse silently drops the
 * one header the tailnet daemon requires.
 */
export function isLoopbackUrl(daemonUrl: string): boolean {
  let host: string;
  try {
    host = new URL(daemonUrl).hostname.toLowerCase();
  } catch {
    return false;
  }
  if (host === "localhost" || host.endsWith(".localhost")) return true;
  if (host === "[::1]" || host === "::1") return true;
  return IPV4_LOOPBACK.test(host);
}

/**
 * Whether a request should carry the bearer token.
 *
 * `asked` is a caller that wants the daemon to judge the token — the options
 * page's connection test, and every write, which carries an `Origin` the
 * daemon refuses without it. Everything else presents the token only when the
 * daemon is not on this machine: on loopback an unauthenticated read is what
 * the file-URL intercept relies on, and it must keep working for someone who
 * has pasted no token, or a stale one. There is nothing to present when no
 * token is stored.
 */
export function presentsBearer(settings: Settings, asked: boolean): boolean {
  if (settings.token === "") return false;
  return asked || !isLoopbackUrl(settings.daemonUrl);
}

/** The daemon's host and port, for a sentence that has to name it. */
export function daemonHost(daemonUrl: string): string {
  try {
    return new URL(daemonUrl).host;
  } catch {
    return trimSlash(daemonUrl);
  }
}

/**
 * Why registering a folder cannot work under a tailnet name. The refusal is
 * the daemon's allow-list, not the token: `POST /api/roots` is the path from
 * a network credential to any directory on the machine, so it is served on
 * loopback only and deliberately stays that way.
 */
export function registerRefusedRemotely(daemonUrl: string): string {
  return (
    `Registering a folder is refused over ${daemonHost(daemonUrl)}: the daemon serves ` +
    "`POST /api/roots` on loopback only, so a file outside every registered root " +
    "cannot be opened from here. Register the folder on the machine running the " +
    "daemon (`mdn open DIR`), and the file opens over the tailnet after that."
  );
}

/**
 * Why a clip was refused under a tailnet name *now that it should not be*.
 * Since M6-R1 the daemon admits `POST /api/clip` there, so a `loopback_only`
 * on a clip means the daemon on the other end is older than this extension —
 * not that the token is wrong, which is the misdiagnosis #51 ended.
 */
export function clipRefusedByOlderDaemon(daemonUrl: string, detail: string): string {
  return (
    `The daemon at ${daemonHost(daemonUrl)} still serves \`POST /api/clip\` on ` +
    `loopback only (${detail}). That daemon predates the version that admits ` +
    "clipping over the tailnet: update `mdn` on the machine holding the notes, or " +
    "point this extension back at its loopback address."
  );
}

/**
 * A `bad_host` refusal: the daemon did not recognise the name the request
 * arrived under as one of its own. Under a tailnet name this is almost always
 * the port — `tailnet_host` has to carry it unless the daemon is reached on
 * 443 through a `tailscale serve` proxy.
 */
export function badHostMessage(daemonUrl: string, detail: string): string {
  return (
    `The daemon does not answer to ${daemonHost(daemonUrl)} (${detail}). Its ` +
    "`tailnet_host` must be exactly the host and port this URL names — the port " +
    "included, unless a `tailscale serve` proxy terminates TLS for it on 443."
  );
}

/**
 * What the tailnet allow-list leaves working, said in one clause so the popup
 * and the connection test can agree. Clipping joined the list in M6-R1; what
 * is left on the right-hand side is registering a folder, which is the step
 * from a network credential to any directory on the machine.
 */
export const TAILNET_LIMITS =
  "opening a file already inside a registered root and clipping both work here, " +
  "but registering a folder is refused — the daemon serves `POST /api/roots` on " +
  "loopback only";

/** The daemon line at the top of the popup, which names the limits up front. */
export function describeDaemonReach(settings: Settings): string {
  const where = `Daemon: ${trimSlash(settings.daemonUrl)}`;
  return isLoopbackUrl(settings.daemonUrl) ? where : `${where} — over the tailnet, ${TAILNET_LIMITS}.`;
}
