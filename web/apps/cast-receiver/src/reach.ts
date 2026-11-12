// SPDX-License-Identifier: MPL-2.0

import {
  browserTogetherSocket,
  connectThrough,
  directFetch,
  failureText,
  firstDirectPath,
  opensItsAPIToThisPage,
  rtcTogetherSocket,
  targetOfSession,
  type DiscoveryClient,
  type FetchRequest,
  type OpenTogetherSocket,
  type Target,
} from "@orpheum/sdk";

/**
 * How the lab's receiver reaches its sender's core when it is away from it (the 11-05 batch's item
 * 10, the owner's test in Berlin): as the web client does, through the sender's discovery: the
 * core's own names first, where the core lets the receiver's page call its API (D467), else WebRTC
 * through the discovery, its offers proven under the sender's session. The receiver is served by the
 * discovery's loader, whose origin a core from D467 on lets call its API as it lets its client's.
 */

/**
 * What a sender hands the receiver to find its core: its discovery, the core, and the locator of
 * the sender's session. A Google Cast device casting under a grant is handed this alone (D465 §2).
 */
export interface CoreReach {
  /** The discovery's client API, as the sender's page names it. */
  readonly clientApi: string;
  readonly coreId: string;
  /** The core's identity key's fingerprint, which its answers are checked by. */
  readonly fingerprint: string;
  /** The locator the discovery finds the core by for the sender's session. */
  readonly locator: string;
  /**
   * The sender's page origin, the core's client origin (D308 §4). Undefined where the sender names
   * none.
   */
  readonly clientOrigin: string | undefined;
}

/** What the lab's sender hands: a core's reach, with the sender's session, which signs it in. */
export interface Reach extends CoreReach {
  /** The sender's session, which the receiver's offers and requests are signed in by. */
  readonly token: string;
}

/** An address on https, else undefined. */
function onHttps(address: string | undefined): string | undefined {
  return address?.startsWith("https://") === true ? address : undefined;
}

/** The non-empty string at name in value, or undefined. */
function stringAt(value: object, name: string): string | undefined {
  const each: unknown = Reflect.get(value, name);
  return typeof each === "string" && each !== "" ? each : undefined;
}

/**
 * A sender's reach of its core, or undefined for anything else: a client API on https alone, and a
 * client origin on https or none.
 */
export function coreReachOf(value: unknown): CoreReach | undefined {
  if (typeof value !== "object" || value === null) {
    return undefined;
  }
  const clientApi = onHttps(stringAt(value, "clientApi"));
  const coreId = stringAt(value, "coreId");
  const fingerprint = stringAt(value, "fingerprint");
  const locator = stringAt(value, "locator");
  if (
    clientApi === undefined ||
    coreId === undefined ||
    fingerprint === undefined ||
    locator === undefined
  ) {
    return undefined;
  }
  return {
    clientApi,
    coreId,
    fingerprint,
    locator,
    clientOrigin: onHttps(stringAt(value, "clientOrigin")),
  };
}

/** A lab sender's reach, its core's with the sender's session, or undefined for anything else. */
export function reachOf(value: unknown): Reach | undefined {
  const core = coreReachOf(value);
  const token = typeof value === "object" && value !== null ? stringAt(value, "token") : undefined;
  return core === undefined || token === undefined ? undefined : { ...core, token };
}

/**
 * The receiver's way to its core: the fetch its requests go by, the sockets its live and cast
 * connections open on the same path (as the web client's kept-path.ts opens them), and its ending.
 */
export interface Reached {
  readonly fetch: FetchRequest;
  readonly openSocket: OpenTogetherSocket;
  readonly close: () => void;
}

/** The core at origin, reached by its name: the page's own core where it is served by one. */
export function reachedAt(origin: string): Reached {
  return {
    fetch: directFetch(origin),
    openSocket: browserTogetherSocket(origin),
    close: () => undefined,
  };
}

/**
 * Reaches the core reach names, signed in by the sender's session (the lab's): reachThrough, its
 * offers proven under that session.
 */
export async function reachCore(reach: Reach, note: (line: string) => void): Promise<Reached> {
  return await reachThrough(
    reach,
    (client) =>
      targetOfSession(client, {
        core: { coreId: reach.coreId, fingerprint: reach.fingerprint },
        token: reach.token,
        locator: reach.locator,
      }),
    note,
  );
}

/**
 * Reaches the core reach names, at the target targetOf finds through its discovery: at the first of
 * its own names that answers, where the core lets this page call its API (opensItsAPIToThisPage),
 * else over WebRTC, its offers proven as the target says. Notes each step, which a device shows on its screen and in its
 * report.
 */
export async function reachThrough(
  reach: CoreReach,
  targetOf: (client: DiscoveryClient) => Promise<Target>,
  note: (line: string) => void,
): Promise<Reached> {
  const client = { clientApi: reach.clientApi };
  note(`asking ${reach.clientApi} for the core's paths`);
  const target = await targetOf(client);
  const origin = await firstDirectPath(target.directUrls, undefined, {
    tell: (outcomes) => {
      for (const each of outcomes) {
        note(`direct ${each.url}: ${each.outcome}`);
      }
    },
  });
  if (origin === undefined) {
    note("no direct name answered; connecting over WebRTC through the discovery");
  } else if (await opensItsAPIToThisPage(origin)) {
    note(`reached the core at ${origin}`);
    return reachedAt(origin);
  } else {
    note(
      `${origin} answered but lets ${globalThis.location.origin} call only its health: over WebRTC through the discovery`,
    );
  }
  try {
    const transport = await connectThrough(client, target);
    note("connected over WebRTC");
    return {
      fetch: transport.fetch,
      openSocket: rtcTogetherSocket(() => transport.openChannel()),
      close: () => {
        transport.close();
      },
    };
  } catch (cause) {
    throw new Error(`WebRTC through the discovery failed: ${failureText(cause)}`, { cause });
  }
}
