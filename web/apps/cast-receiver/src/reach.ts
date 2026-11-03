// SPDX-License-Identifier: MPL-2.0

import {
  connectThrough,
  directFetch,
  failureText,
  firstDirectPath,
  targetOfSession,
  type FetchRequest,
} from "@orpheum/sdk";

/**
 * How the lab's receiver reaches its sender's core when it is away from it (the 11-05 batch's item
 * 10, the owner's test in Berlin): as the web client does, through the sender's discovery: the
 * core's own names first, else WebRTC through the discovery, its offers proven under the sender's
 * session. The receiver is served by the discovery's loader, so the core and the discovery answer
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
}

/** A sender's reach, or undefined for anything else: a client API on https alone. */
export function reachOf(value: unknown): Reach | undefined {
  if (typeof value !== "object" || value === null) {
    return undefined;
  }
  const field = (name: string): string | undefined => {
    const each: unknown = Reflect.get(value, name);
    return typeof each === "string" && each !== "" ? each : undefined;
  };
  const clientApi = field("clientApi");
  const coreId = field("coreId");
  const fingerprint = field("fingerprint");
  const token = field("token");
  const locator = field("locator");
  if (
    clientApi === undefined ||
    !clientApi.startsWith("https://") ||
    coreId === undefined ||
    fingerprint === undefined ||
    token === undefined ||
    locator === undefined
  ) {
    return undefined;
  }
  return { clientApi, coreId, fingerprint, token, locator };
}

/** The receiver's way to its core: the fetch its requests go by, and its ending. */
export interface Reached {
  readonly fetch: FetchRequest;
  readonly close: () => void;
}

/**
 * Reaches the core reach names: at the first of its own names that answers, else over WebRTC through
 * its discovery. Notes each step, which a device shows on its screen and in its report.
 */
export async function reachCore(reach: Reach, note: (line: string) => void): Promise<Reached> {
  const client = { clientApi: reach.clientApi };
  note(`asking ${reach.clientApi} for the core's paths`);
  const target = await targetOfSession(client, {
    core: { coreId: reach.coreId, fingerprint: reach.fingerprint },
    token: reach.token,
    locator: reach.locator,
  });
  const origin = await firstDirectPath(target.directUrls, undefined, {
    tell: (outcomes) => {
      for (const each of outcomes) {
        note(`direct ${each.url}: ${each.outcome}`);
      }
    },
  });
  if (origin !== undefined) {
    note(`reached the core at ${origin}`);
    return { fetch: directFetch(origin), close: () => undefined };
  }
  note("no direct name answered; connecting over WebRTC through the discovery");
  try {
    const transport = await connectThrough(client, target);
    note("connected over WebRTC");
    return {
      fetch: transport.fetch,
      close: () => {
        transport.close();
      },
    };
  } catch (cause) {
    throw new Error(`WebRTC through the discovery failed: ${failureText(cause)}`, { cause });
  }
}
