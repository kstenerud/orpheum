// SPDX-License-Identifier: MPL-2.0

/**
 * How a page served by a discovery reaches its core (webrtc-transport.md W4c; the discovery
 * protocol's §3.1–§3.3): a link's fragment, or the core this device reached before, found through
 * the discovery's client API, and a WebRTC connection through it whose offers are proven and whose
 * answers the core signed. On the core's own origin none of it runs.
 */

import {
  connectRtc,
  discoverySignaller,
  locate,
  offerKey,
  pathsOf,
  readLinkFragment,
  sessionOfferKey,
  type DiscoveryClient,
  type Held,
  type LinkFragment,
  type RtcTransport,
} from "@orpheum/sdk";

import { storageThatNeverThrows, type StringStorage } from "./browser-storage.ts";

/** The meta element a discovery's page names its client API by; the core's own page has none. */
const clientApiMetaName = "orpheum-discovery-client-api";

/**
 * The client API of the discovery whose page this is, as the page names it, or "" on the core's own
 * origin: the server tells the client, which reads no environment (D204).
 */
export function discoveryClientApiOf(page: Pick<Document, "querySelector">): string {
  return page.querySelector(`meta[name="${clientApiMetaName}"]`)?.getAttribute("content") ?? "";
}

/** The core this device last reached at this discovery: its id and its identity key's fingerprint. */
export interface KeptCore {
  readonly coreId: string;
  readonly fingerprint: string;
}

/** Where the last core reached is kept, apart from each core's own credentials. */
const keptCoreKey = "orpheum.discovery.core";

/** How a page at a discovery's origin starts. */
export type Start =
  | { readonly kind: "link"; readonly link: LinkFragment }
  | { readonly kind: "remembered"; readonly core: KeptCore }
  | { readonly kind: "none" };

function stringField(kept: unknown, name: string): string | undefined {
  const value: unknown =
    typeof kept === "object" && kept !== null ? Reflect.get(kept, name) : undefined;
  return typeof value === "string" ? value : undefined;
}

/** The core this device last reached, when one is kept. */
export function rememberedCore(refusable: StringStorage | undefined): KeptCore | undefined {
  const text = storageThatNeverThrows(refusable).getItem(keptCoreKey);
  if (text === null) {
    return undefined;
  }
  try {
    const kept: unknown = JSON.parse(text);
    const coreId = stringField(kept, "coreId");
    const fingerprint = stringField(kept, "fingerprint");
    return coreId === undefined || fingerprint === undefined ? undefined : { coreId, fingerprint };
  } catch {
    return undefined;
  }
}

/** Keeps core as the one this device reached, for its next visit. */
export function rememberCore(refusable: StringStorage | undefined, core: KeptCore): void {
  storageThatNeverThrows(refusable).setItem(keptCoreKey, JSON.stringify(core));
}

/** How the page starts: from the link its address carries, the core it reached before, or neither. */
export function startOf(hash: string, refusable: StringStorage | undefined): Start {
  const link = readLinkFragment(hash);
  if (link !== undefined) {
    return { kind: "link", link };
  }
  const core = rememberedCore(refusable);
  return core === undefined ? { kind: "none" } : { kind: "remembered", core };
}

/** A way into the core: its id, the fingerprint held for it, what the device holds, and its key. */
export interface Target {
  readonly coreId: string;
  readonly fingerprint: string;
  readonly held: Held;
  readonly key: Uint8Array<ArrayBuffer>;
  readonly iceServers: RTCIceServer[];
}

/**
 * Finds the core by a link, spending a use of it: the click that asked, never the page's loading
 * (the protocol's §3.1). The target's offers are proven under the link's key.
 */
export async function targetOfLink(client: DiscoveryClient, link: LinkFragment): Promise<Target> {
  const located = await locate(client, link.secret);
  if (located.coreId !== link.coreId || located.bootstrapToken === undefined) {
    throw new Error("the discovery answered for another server than the link names");
  }
  return {
    coreId: link.coreId,
    fingerprint: link.fingerprint,
    held: { bootstrapToken: located.bootstrapToken },
    key: await offerKey(link.secret),
    iceServers: located.iceServers,
  };
}

/** The session a device holds with a core it reached before. */
export interface HeldSession {
  readonly core: KeptCore;
  readonly token: string;
  readonly locator: string;
}

/** Finds the remembered core by this device's locator; its offers are proven under its session. */
export async function targetOfSession(client: DiscoveryClient, held: HeldSession): Promise<Target> {
  const located = await pathsOf(client, { locator: held.locator });
  if (located.coreId !== held.core.coreId) {
    throw new Error("the discovery answered for another server than this device reached");
  }
  return {
    coreId: held.core.coreId,
    fingerprint: held.core.fingerprint,
    held: { locator: held.locator },
    key: await sessionOfferKey(held.token),
    iceServers: located.iceServers,
  };
}

/** Connects to the target through the discovery, its offers proven and its answers checked. */
export function connectThrough(client: DiscoveryClient, target: Target): Promise<RtcTransport> {
  return connectRtc(
    discoverySignaller({
      clientApi: client.clientApi,
      coreId: target.coreId,
      fingerprint: target.fingerprint,
      held: target.held,
      key: target.key,
      fetch: client.fetch,
    }),
    { iceServers: target.iceServers },
  );
}
