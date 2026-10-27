// SPDX-License-Identifier: MPL-2.0

/**
 * How a page served by a discovery reaches its core (webrtc-transport.md W4c; the discovery
 * protocol's §3.1–§3.3): a link's fragment, or the core this device reached before, found through
 * the discovery's client API, and a WebRTC connection through it whose offers are proven and whose
 * answers the core signed. On the core's own origin none of it runs.
 */

import { browserLocalNetwork, type LocalNetwork, type LocalNetworkState } from "./local-network.ts";
import { pathChoiceOf, type PathChoice } from "./path-choice.ts";
import {
  connectRtc,
  discoverySignaller,
  firstDirectPath,
  handOffOfferKey,
  locate,
  offerKey,
  pathsOf,
  realmName,
  realmSignIn,
  sessionOfferKey,
  type DirectOutcome,
  type DiscoveryClient,
  type Held,
  type LinkFragment,
  type Proving,
  type RealmTyped,
  type RtcTransport,
} from "@orpheum/sdk";

import { storageThatNeverThrows, type StringStorage } from "./browser-storage.ts";
import type { Membership } from "./together-storage.ts";

/** The meta element a discovery's page names its client API by; the core's own page has none. */
const clientApiMetaName = "orpheum-discovery-client-api";

/**
 * The client API of the discovery whose page this is, as the page names it, or "" on the core's own
 * origin: the server tells the client, which reads no environment (D204).
 */
export function discoveryClientApiOf(page: Pick<Document, "querySelector">): string {
  return page.querySelector(`meta[name="${clientApiMetaName}"]`)?.getAttribute("content") ?? "";
}

/** The meta element a paid discovery's page says it has realms by (D298 §1). */
const realmsMetaName = "orpheum-discovery-realms";

/** Whether the discovery whose page this is has realms, so a person signs in by one there. */
export function discoveryHasRealms(page: Pick<Document, "querySelector">): boolean {
  return page.querySelector(`meta[name="${realmsMetaName}"]`)?.getAttribute("content") === "true";
}

/**
 * The core this device last reached at this discovery: its id, its identity key's fingerprint, and
 * the realm its person signed in to it by, when they did, which the page shows (D297 §5).
 */
export interface KeptCore {
  readonly coreId: string;
  readonly fingerprint: string;
  readonly realm?: string | undefined;
}

/** How a page at a discovery's origin starts. */
export type Start =
  | { readonly kind: "link"; readonly link: LinkFragment }
  | { readonly kind: "remembered"; readonly core: KeptCore }
  | { readonly kind: "handoff"; readonly core: KeptCore; readonly handOff: HandOff }
  | { readonly kind: "none" };

/**
 * A realm sign-in the discovery's loader handed on to the core's client origin (D308 §4): the code
 * the client origin takes its session by, once; the bootstrap token the sign-in got, which finds the
 * core's paths for five minutes; the core's fingerprint; and the realm.
 */
export interface HandOff {
  readonly code: string;
  readonly bootstrapToken: string;
  readonly fingerprint: string;
  readonly realm: string;
}

/** The fragment a loader sends a browser on to a client origin with, carrying handOff. */
export function handOffFragment(handOff: HandOff): string {
  return `#${new URLSearchParams({
    handoff: handOff.code,
    bootstrap: handOff.bootstrapToken,
    fingerprint: handOff.fingerprint,
    realm: handOff.realm,
  }).toString()}`;
}

/** The hand-off a fragment carries, or undefined for any fragment that is not one whole. */
export function readHandOffFragment(hash: string): HandOff | undefined {
  const fields = new URLSearchParams(hash.replace(/^#/u, ""));
  const code = fields.get("handoff") ?? "";
  const bootstrapToken = fields.get("bootstrap") ?? "";
  const fingerprint = fields.get("fingerprint") ?? "";
  if (code === "" || bootstrapToken === "" || fingerprint === "") {
    return undefined;
  }
  return { code, bootstrapToken, fingerprint, realm: fields.get("realm") ?? "" };
}

/**
 * Calls then once, the first time session goes from signed in to signed out: at a discovery's page a
 * person who signs out, or is signed out, starts again at the realm sign-in, not at the core's own
 * sign-in over a connection to the server they just left. Returns what stops the watch.
 */
export function whenSignedOut(
  session: {
    readonly token: () => string | undefined;
    readonly subscribe: (listener: () => void) => () => void;
  },
  then: () => void,
): () => void {
  let wasSignedIn = session.token() !== undefined;
  const stop = session.subscribe(() => {
    const isSignedIn = session.token() !== undefined;
    if (wasSignedIn && !isSignedIn) {
      stop();
      then();
    }
    wasSignedIn = isSignedIn;
  });
  return stop;
}

/** Where this device keeps the realm its person asked it to remember (D297 §5). */
const keptRealmKey = "orpheum.discovery.realm";

/** The realm this device was asked to remember, when it was. */
export function rememberedRealm(refusable: StringStorage | undefined): string | undefined {
  return storageThatNeverThrows(refusable).getItem(keptRealmKey) ?? undefined;
}

/** Keeps realm for this device's next sign-in, or, undefined, forgets any it kept. */
export function rememberRealm(
  refusable: StringStorage | undefined,
  realm: string | undefined,
): void {
  const storage = storageThatNeverThrows(refusable);
  if (realm === undefined) {
    storage.removeItem(keptRealmKey);
  } else {
    storage.setItem(keptRealmKey, realm);
  }
}

/** A way into the core: its id, the fingerprint held for it, what the device holds, and its key. */
export interface Target {
  readonly coreId: string;
  readonly fingerprint: string;
  readonly held: Held;
  readonly key: Uint8Array<ArrayBuffer>;
  readonly iceServers: RTCIceServer[];
  /** The core's published names, tried before WebRTC (W2c). */
  readonly directUrls: readonly string[];
  /** Those of directUrls that are the core's LAN names. */
  readonly lanUrls: readonly string[];
  /**
   * What the device holds by the time a later offer is made, when it holds more than the target
   * began with: the session or membership a link's person gained (DiscoverySignalling.latest).
   */
  readonly latest?: (() => Promise<Proving | undefined>) | undefined;
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
    directUrls: located.directUrls,
    lanUrls: located.lanUrls,
  };
}

/**
 * The core a realm sign-in finds, and the target to reach it by: its offers proven by the person's
 * realm key, its answers checked by the identity key the core's tag proved its own (D297 §3).
 */
export async function targetOfRealm(
  client: DiscoveryClient,
  typed: RealmTyped,
): Promise<{ readonly core: KeptCore; readonly target: Target; readonly claimedAt: number }> {
  const found = await realmSignIn(client, typed);
  const { located } = found;
  if (located.bootstrapToken === undefined) {
    throw new Error("the discovery's answer to a realm sign-in has no bootstrap token");
  }
  return {
    core: {
      coreId: located.coreId,
      fingerprint: found.fingerprint,
      realm: realmName(typed.realm),
    },
    target: {
      coreId: located.coreId,
      fingerprint: found.fingerprint,
      held: { bootstrapToken: located.bootstrapToken },
      key: found.key,
      iceServers: located.iceServers,
      directUrls: located.directUrls,
      lanUrls: located.lanUrls,
    },
    claimedAt: found.claimedAt,
  };
}

/**
 * Finds the core a hand-off names by the bootstrap token its realm sign-in got; the target's offers
 * are proven under the hand-off code's key until the code is taken (D308 §4).
 */
export async function targetOfHandOff(
  client: DiscoveryClient,
  core: KeptCore,
  handOff: HandOff,
): Promise<Target> {
  const located = await pathsOf(client, { bootstrapToken: handOff.bootstrapToken });
  if (located.coreId !== core.coreId) {
    throw new Error("the discovery answered for another server than the hand-off names");
  }
  return {
    coreId: core.coreId,
    fingerprint: core.fingerprint,
    held: { bootstrapToken: handOff.bootstrapToken },
    key: await handOffOfferKey(handOff.code),
    iceServers: located.iceServers,
    directUrls: located.directUrls,
    lanUrls: located.lanUrls,
  };
}

/**
 * What a device holds with a core it reached before: a signed-in session's token, or a Watch Together
 * guest's membership credential, which the core proves offers under alike, with the locator the
 * discovery finds the core by for it.
 */
export interface HeldSession {
  readonly core: KeptCore;
  readonly token: string;
  readonly locator: string;
}

/** What a device keeps that a holding is made of: its session's token and locator, for one core. */
export interface KeptSession {
  readonly token: () => string | undefined;
  readonly locator: () => string | undefined;
}

/**
 * What this device holds with core: its signed-in session, else its Watch Together membership as a
 * guest, which the core finds and proves offers by alike (the hub's OfferKey); undefined when it holds
 * neither, or a membership kept before memberships kept their locator.
 */
export function heldWith(
  core: KeptCore,
  session: KeptSession,
  membership: Pick<Membership, "credential" | "locator"> | undefined,
): HeldSession | undefined {
  const token = session.token();
  const locator = session.locator();
  if (token !== undefined && locator !== undefined) {
    return { core, token, locator };
  }
  return membership?.locator === undefined
    ? undefined
    : { core, token: membership.credential, locator: membership.locator };
}

/** What an offer made with held is sent and proven with: its locator, under its token's key. */
export async function provingOf(held: HeldSession): Promise<Proving> {
  return { held: { locator: held.locator }, key: await sessionOfferKey(held.token) };
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
    ...(await provingOf(held)),
    iceServers: located.iceServers,
    directUrls: located.directUrls,
    lanUrls: located.lanUrls,
  };
}

/** How the core was reached: at one of its own names, or over WebRTC through the discovery. */
export type Reached =
  | { readonly kind: "direct"; readonly origin: string }
  | { readonly kind: "webrtc"; readonly transport: RtcTransport };

/**
 * The core reached, and how to reach it again (webrtc-transport.md W2c items 5 and 6): another path
 * when the one in use fails, and a better one than the one in use, when one answers now.
 */
export interface Reaching {
  readonly first: Reached;
  /** How the first path was chosen, and why, for the core's log (path-choice.ts). */
  readonly choice: PathChoice;
  readonly another: (failed: Reached) => Promise<Reached>;
  readonly better: (current: Reached) => Promise<Reached | undefined>;
  /** Tells listener when the browser's local-network permission is granted; returns its removal. */
  readonly onLocalNetworkGranted: (listener: () => void) => () => void;
  /**
   * Tells listener whether the browser waits on the person to allow the local network, which the
   * first race's request to the LAN name had it ask (W2c item 8); never told where the target has
   * no LAN name or the browser refuses it. Returns its removal.
   */
  readonly onLocalNetworkAsking: (listener: (isAsking: boolean) => void) => () => void;
}

/** Whether path is one of the target's LAN names. */
function isLanOf(target: Target, path: Reached): boolean {
  return (
    path.kind === "direct" && target.lanUrls.some((url) => new URL(url).origin === path.origin)
  );
}

/** Where path stands in the discovery's order of the target's names, WebRTC after them all. */
function rankOf(target: Target, path: Reached): number {
  const origins = target.directUrls.map((url) => new URL(url).origin);
  const index = path.kind === "direct" ? origins.indexOf(path.origin) : -1;
  return index < 0 ? origins.length : index;
}

/**
 * Reaches the target: at the first of its published names to answer, else over WebRTC through the
 * discovery (the protocol's §3.3; W2c, which races the names and then falls back). Its WebRTC
 * connection, once made, serves every later move to WebRTC. The LAN name is tried as the browser's
 * local-network permission allows (W2c item 1): never once denied, and while the browser is still to
 * ask, asked once to raise the prompt, the race never waiting on it (raced); the look for a better
 * path leaves it out until the person answers.
 */
export async function reachCore(
  client: DiscoveryClient,
  target: Target,
  localNetwork: LocalNetwork = browserLocalNetwork(),
): Promise<Reaching> {
  const notLan = (urls: readonly string[]): readonly string[] =>
    urls.filter((url) => !target.lanUrls.includes(url));
  let webrtc: Promise<Reached> | undefined;
  const overWebrtc = (): Promise<Reached> => {
    webrtc ??= connectThrough(client, target).then(
      (transport): Reached => ({ kind: "webrtc", transport }),
      (problem: unknown) => {
        webrtc = undefined;
        throw problem;
      },
    );
    return webrtc;
  };
  const direct = async (urls: readonly string[]): Promise<Reached | undefined> => {
    const origin = await firstDirectPath(urls, client.fetch);
    return origin === undefined ? undefined : { kind: "direct", origin };
  };
  const state = await localNetwork.state();
  const race = await firstRace(client, target, state);
  const first = race.first ?? (await overWebrtc());
  // Whether the page is in the house: its LAN name answered. There WebRTC finds the LAN pair, and
  // the core's public names are never better than it: through them the page is away, with no admin
  // (D300 §4; the owner's Firefox at home, 2026-10-15, moved to the public IPv6 name and lost admin).
  let isAtHome = isLanOf(target, first);
  return {
    first,
    choice: race.choice(first),
    another: async (failed) => {
      const others = target.directUrls.filter(
        (url) => failed.kind !== "direct" || new URL(url).origin !== failed.origin,
      );
      // A LAN name that answered and then failed says the page is in the house, where Firefox fails
      // a controlled page's requests to it: WebRTC finds the LAN pair there, and the core's public
      // names would take it out through the VPS, away from admin (D300 §4).
      if (isLanOf(target, failed)) {
        isAtHome = true;
        return await overWebrtc().catch(async (problem: unknown) => {
          const named = await direct(others);
          if (named === undefined) {
            throw problem;
          }
          return named;
        });
      }
      return (await direct(others)) ?? (await overWebrtc());
    },
    better: async (current) => {
      const state = await localNetwork.state();
      const isLanAllowed = state === "granted" || state === "unknown";
      const candidates = isAtHome ? target.lanUrls : target.directUrls;
      const asked = isLanAllowed ? candidates : notLan(candidates);
      const best =
        rankOf(target, current) === 0 || asked.length === 0 ? undefined : await direct(asked);
      return best !== undefined && rankOf(target, best) < rankOf(target, current)
        ? best
        : undefined;
    },
    onLocalNetworkGranted: (listener) => localNetwork.onGranted(listener),
    onLocalNetworkAsking: askingWhereTried(target, state === "denied", localNetwork),
  };
}

/**
 * The first race of the target's names, as the browser's local-network permission, state, allows: the
 * name that answered, if one did, and how the choice of the path taken, first, was made and why.
 */
async function firstRace(
  client: DiscoveryClient,
  target: Target,
  state: LocalNetworkState,
): Promise<{ readonly first?: Reached; readonly choice: (first: Reached) => PathChoice }> {
  let outcomes: readonly DirectOutcome[] = [];
  const origin = await firstDirectPath(raced(target, state, client.fetch), client.fetch, {
    tell: (told) => {
      outcomes = told;
    },
  });
  const choice = (first: Reached): PathChoice =>
    pathChoiceOf({
      ...target,
      raced: outcomes,
      localNetwork: state,
      userAgent: globalThis.navigator.userAgent,
      directOrigin: first.kind === "direct" ? first.origin : undefined,
    });
  return origin === undefined ? { choice } : { first: { kind: "direct", origin }, choice };
}

/**
 * The names the first race tries, as the browser's local-network permission allows. While the browser
 * is still to ask, the LAN name is asked once apart, which is what raises its prompt, and left out of
 * the race: Firefox holds that request until the person answers, and a race waiting on it kept the
 * page on "Connecting" with the prompt up and no word of why (the owner's staging test, 2026-10-20).
 */
function raced(
  target: Target,
  state: LocalNetworkState,
  send: typeof fetch | undefined,
): readonly string[] {
  if (state === "granted" || state === "unknown") {
    return target.directUrls;
  }
  if (state === "prompt" && target.lanUrls.length > 0) {
    firstDirectPath(target.lanUrls, send).catch(() => undefined);
  }
  return target.directUrls.filter((url) => !target.lanUrls.includes(url));
}

/**
 * Whether the browser asks for the local network, told only where the first race tried the target's
 * LAN name, which is what has it ask (W2c item 8): never where it has none, or the browser refuses it.
 */
function askingWhereTried(
  target: Target,
  isDenied: boolean,
  localNetwork: LocalNetwork,
): Reaching["onLocalNetworkAsking"] {
  return (listener) =>
    isDenied || target.lanUrls.length === 0 ? () => undefined : localNetwork.onAsking(listener);
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
      latest: target.latest,
      fetch: client.fetch,
    }),
    { iceServers: target.iceServers },
  );
}
