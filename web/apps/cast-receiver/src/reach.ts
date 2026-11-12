// SPDX-License-Identifier: MPL-2.0

import {
  browserTogetherSocket,
  connectThrough,
  directFetch,
  failureText,
  firstDirectPath,
  rtcTogetherSocket,
  targetOfSession,
  type FetchRequest,
  type OpenTogetherSocket,
} from "@orpheum/sdk";

/**
 * How the lab's receiver reaches its sender's core when it is away from it (the 11-05 batch's item
 * 10, the owner's test in Berlin): as the web client does, through the sender's discovery: the
 * core's own names first where its page is at the core's client origin (mayGoDirect), else WebRTC
 * through the discovery, its offers proven under the sender's session. The receiver is served by the discovery's loader, so the core and the discovery answer
 * it as they answer the web client there.
 */

/** What a sender hands the receiver to reach its core: its discovery, the core, and its session. */
export interface Reach {
  /** The discovery's client API, as the sender's page names it. */
  readonly clientApi: string;
  readonly coreId: string;
  /** The core's identity key's fingerprint, which its answers are checked by. */
  readonly fingerprint: string;
  /** The sender's session, which the receiver's offers and requests are signed in by. */
  readonly token: string;
  /** The locator the discovery finds the core by for the sender's session. */
  readonly locator: string;
  /**
   * The sender's page origin, the core's client origin: the one page origin besides the core's own
   * names that the core lets reach it (D308 §4). Undefined where the sender names none.
   */
  readonly clientOrigin: string | undefined;
}

/** An address on https, else undefined. */
function onHttps(address: string | undefined): string | undefined {
  return address?.startsWith("https://") === true ? address : undefined;
}

/**
 * A sender's reach, or undefined for anything else: a client API on https alone, and a client
 * origin on https or none.
 */
export function reachOf(value: unknown): Reach | undefined {
  if (typeof value !== "object" || value === null) {
    return undefined;
  }
  const field = (name: string): string | undefined => {
    const each: unknown = Reflect.get(value, name);
    return typeof each === "string" && each !== "" ? each : undefined;
  };
  const clientApi = onHttps(field("clientApi"));
  const coreId = field("coreId");
  const fingerprint = field("fingerprint");
  const token = field("token");
  const locator = field("locator");
  if (
    clientApi === undefined ||
    coreId === undefined ||
    fingerprint === undefined ||
    token === undefined ||
    locator === undefined
  ) {
    return undefined;
  }
  return {
    clientApi,
    coreId,
    fingerprint,
    token,
    locator,
    clientOrigin: onHttps(field("clientOrigin")),
  };
}

/**
 * Whether the receiver, its page at pageOrigin, may try the core's own names: only where it is
 * served at the core's client origin, which the core lets call it. Served at the loader's origin
 * (the one receiver address the Cast console names for every core), the core answers its health
 * check but refuses every other request (cross_origin.go), so it goes over WebRTC only, at home too
 * (the owner, 2026-11-11: "ok, we try webrtc only then").
 */
export function mayGoDirect(reach: Pick<Reach, "clientOrigin">, pageOrigin: string): boolean {
  return reach.clientOrigin !== undefined && reach.clientOrigin === pageOrigin;
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
 * Reaches the core reach names: where the page may go direct (mayGoDirect), at the first of its own
 * names that answers, else over WebRTC through its discovery. Notes each step, which a device shows
 * on its screen and in its report.
 */
export async function reachCore(reach: Reach, note: (line: string) => void): Promise<Reached> {
  const client = { clientApi: reach.clientApi };
  note(`asking ${reach.clientApi} for the core's paths`);
  const target = await targetOfSession(client, {
    core: { coreId: reach.coreId, fingerprint: reach.fingerprint },
    token: reach.token,
    locator: reach.locator,
  });
  const pageOrigin = globalThis.location.origin;
  if (mayGoDirect(reach, pageOrigin)) {
    const origin = await firstDirectPath(target.directUrls, undefined, {
      tell: (outcomes) => {
        for (const each of outcomes) {
          note(`direct ${each.url}: ${each.outcome}`);
        }
      },
    });
    if (origin !== undefined) {
      note(`reached the core at ${origin}`);
      return reachedAt(origin);
    }
    note("no direct name answered; connecting over WebRTC through the discovery");
  } else {
    note(
      `served at ${pageOrigin}, not the core's client origin: over WebRTC through the discovery`,
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
