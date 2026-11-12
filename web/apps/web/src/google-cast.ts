// SPDX-License-Identifier: MPL-2.0

/**
 * The Cast menu's Chromecast (D465 §3): in Chrome alone, Chrome's own picker finds a Google Cast
 * device on the network, the loader's receiver application is launched on it (D465 §5), and it is
 * handed a grant for one cast (D465 §2) over the Cast channel, the only place the grant goes. Once it
 * says it is ready, the cast is made to it as to any of the person's devices (cast.ts's startCast),
 * and the Cast session and the cast end together, whichever ends first.
 */

import type { Api } from "@orpheum/sdk";
import { useEffect } from "react";

import { endCastRetrying, startCast, type Cast, type CastFilm } from "./cast.ts";
import {
  receiverDiscoveryOf,
  receiverReachOf,
  type ReceiverDiscovery,
  type ReceiverReach,
} from "./cast-reach.ts";
import {
  at,
  channelOf,
  isCancelledByThePerson,
  loadCastSender,
  openCastSession,
  type CastChannel,
} from "./cast-sdk.ts";
import { useLive } from "./live.tsx";
import { dataOrThrow } from "./problem.ts";
import type { AskGuestCastGrant, Services } from "./services.tsx";

/** The namespace the sender and the receiver speak on of a cast under a grant. */
export const guestCastNamespace = "urn:x-cast:org.orpheum.cast";

/**
 * Whether this browser is Google's Chrome, on a computer or on Android, which alone casts to a Google
 * Cast device from a web page: its client hints name it. Edge, Opera and Android's WebView name
 * themselves; Safari, Firefox and every browser on an iPhone or iPad give no hints at all.
 */
export function isChromeBrowser(browser: unknown = globalThis.navigator): boolean {
  const brands = at(browser, "userAgentData", "brands");
  return (
    Array.isArray(brands) && brands.some((entry: unknown) => at(entry, "brand") === "Google Chrome")
  );
}

/** Asks the core of api for a grant for one cast to the Google Cast device receiverName (D465 §2). */
export function askGuestCastGrantOf(api: Api): AskGuestCastGrant {
  return async (receiverName) =>
    dataOrThrow(await api.POST("/me/casts/guest-grants", { body: { receiverName } })).grant;
}

/** What a page casts to a Google Cast device with: the loader's application, the grant, the reach. */
export interface GuestCastWays {
  readonly appId: string;
  readonly askGrant: AskGuestCastGrant;
  readonly discovery: ReceiverDiscovery;
}

/**
 * What the page at origin casts to a Google Cast device with, or undefined where it cannot, and the
 * Cast menu offers no Chromecast: a browser not Chrome; the core's own page, whose loader names no
 * receiver application and which no discovery reaches; or no way to ask for a grant.
 */
export function guestCastOf(
  services: Pick<Services, "castReceiverAppId" | "askGuestCastGrant" | "throughDiscovery">,
  origin: string,
  browser: unknown = globalThis.navigator,
): GuestCastWays | undefined {
  const { castReceiverAppId: appId, askGuestCastGrant: askGrant } = services;
  const discovery = receiverDiscoveryOf(services.throughDiscovery, origin);
  if (!isChromeBrowser(browser) || appId === undefined || askGrant === undefined) {
    return undefined;
  }
  return discovery === undefined ? undefined : { appId, askGrant, discovery };
}

/**
 * Casts film to the Google Cast device the person picks in Chrome's picker (the press that asks must
 * be the person's): its device ready, the cast is made to it as to any device of the person's, and
 * its Cast session linked to it (linkCastSession); answers the cast and the device's name, or
 * undefined where the person picked none. locator is the session's, which the device reaches the core
 * by from away.
 */
export async function castToAGoogleCastDevice(
  api: Api,
  ways: GuestCastWays,
  how: { readonly locator: string | undefined; readonly film: CastFilm | undefined },
): Promise<{ readonly cast: Cast; readonly name: string } | undefined> {
  const reach = receiverReachOf(ways.discovery, how.locator);
  if (reach === undefined || !(await loadCastSender())) {
    throw new GuestCastFailure("this page cannot cast to a Google Cast device");
  }
  const receiver = await openGuestReceiver(ways.appId, { askGrant: ways.askGrant, reach });
  if (receiver === undefined) {
    return undefined;
  }
  const { channel, deviceId } = receiver;
  try {
    const cast = await startCast(api, deviceId, how.film);
    linkCastSession(api, channel, cast.castId);
    return { cast, name: channel.receiverName };
  } catch (cause) {
    endQuietly(channel);
    throw cause;
  }
}

/**
 * How long a Google Cast device may take, its grant handed, to say it is ready, in milliseconds: the
 * grant's life (D465 §2), after which it could trade it no longer. A justified constant.
 */
export const guestReadyWithinMs = 60_000;

/**
 * A Google Cast device that could not take the cast: it said it failed, the Cast session ended, or it
 * did not say it was ready in time. Its message never holds the grant.
 */
export class GuestCastFailure extends Error {}

/** A Google Cast device ready for its cast: its Cast session, and the device the core made of it. */
export interface GuestReceiver {
  readonly channel: CastChannel;
  readonly deviceId: number;
}

/**
 * Opens a Cast session with the receiver application appId on the device the person picks in
 * Chrome's picker, hands it a grant for one cast (askGrant) and how to reach the core (reach), and
 * answers it once it says it is ready; undefined where the person closed the picker picking none. A
 * device that fails has its Cast session ended.
 */
export async function openGuestReceiver(
  appId: string,
  ways: { readonly askGrant: AskGuestCastGrant; readonly reach: ReceiverReach },
): Promise<GuestReceiver | undefined> {
  let session: unknown;
  try {
    session = await openCastSession(appId);
  } catch (cause) {
    if (isCancelledByThePerson(cause)) {
      return undefined;
    }
    throw cause;
  }
  const channel = channelOf(session, guestCastNamespace);
  const ready = readyHeard(channel);
  try {
    const grant = await ways.askGrant(channel.receiverName);
    await channel.send({ type: "guest", grant, reach: ways.reach });
    return { channel, deviceId: await ready.deviceId };
  } catch (cause) {
    ready.stop();
    endQuietly(channel);
    throw cause;
  }
}

/**
 * What the device says of its grant: the device the core made of it once it is ready, or a
 * GuestCastFailure; stop gives up on it.
 */
function readyHeard(channel: CastChannel): {
  readonly deviceId: Promise<number>;
  readonly stop: () => void;
} {
  let finish: (outcome: number | Error) => void = () => undefined;
  const deviceId = new Promise<number>((resolve, reject) => {
    let isOver = false;
    const timer = setTimeout(() => {
      finish(new GuestCastFailure("the Google Cast device did not say it was ready in time"));
    }, guestReadyWithinMs);
    finish = (outcome) => {
      if (isOver) {
        return;
      }
      isOver = true;
      clearTimeout(timer);
      if (typeof outcome === "number") {
        resolve(outcome);
      } else {
        reject(outcome);
      }
    };
  });
  // Given up on before anyone waits on it, its failure is no one's to hear.
  deviceId.catch(() => undefined);
  channel.onMessage((message) => {
    const said = at(message, "type");
    const id = at(message, "deviceId");
    if (said === "ready" && typeof id === "number") {
      finish(id);
    } else if (said === "failed") {
      const reason = at(message, "reason");
      finish(new GuestCastFailure(`the Google Cast device failed: ${String(reason)}`));
    }
  });
  channel.onEnded(() => {
    finish(new GuestCastFailure("the Cast session ended before the device was ready"));
  });
  return {
    deviceId,
    stop: () => {
      finish(new GuestCastFailure("given up on"));
    },
  };
}

/** Ends channel's Cast session, one the SDK may have ended already. */
function endQuietly(channel: CastChannel): void {
  try {
    channel.end();
  } catch {
    // The session is gone already: nothing is left to end.
  }
}

/**
 * The Cast session of the cast this page made to a Google Cast device, if any, and whether the live
 * connection has said this device sends it (isSent): the page keeps it across the Cast menu's
 * closing, which the cast's taking closes.
 */
let linked:
  { readonly channel: CastChannel; readonly castId: number; readonly isSent: boolean } | undefined;

/** The cast the live connection last said this device sends, which may be said before it is linked. */
let sentCastId: number | undefined;

/**
 * Links channel's Cast session to the cast castId made to its device: the session ended from
 * Chrome's own Cast control ends the cast, as Stop casting does (endCastRetrying, "stopped"), and
 * the cast's end ends the session (castSentNow).
 */
export function linkCastSession(api: Api, channel: CastChannel, castId: number): void {
  if (linked !== undefined && linked.castId !== castId) {
    endLinked();
  }
  linked = { channel, castId, isSent: sentCastId === castId };
  channel.onEnded(() => {
    if (linked?.castId !== castId) {
      return; // its cast ended first, and ended it
    }
    linked = undefined;
    // Failing twice, the core ends it itself once its receiver is gone.
    endCastRetrying(api, castId, "stopped").catch(() => undefined);
  });
}

/**
 * Tells the link which cast the live connection says this device sends now, castId, or undefined for
 * none: the linked cast said sent and then no longer is over (Stop casting, its receiver's end, the
 * server's), and its Cast session is ended with it.
 */
export function castSentNow(castId: number | undefined): void {
  sentCastId = castId;
  if (linked === undefined) {
    return;
  }
  if (castId === linked.castId) {
    linked = { ...linked, isSent: true };
  } else if (linked.isSent) {
    endLinked();
  }
}

/** Ends the Cast session linked to castId, a cast its device did not take, if it is the one linked. */
export function endCastSessionOf(castId: number): void {
  if (linked?.castId === castId) {
    endLinked();
  }
}

/** Ends the linked Cast session and forgets it. */
function endLinked(): void {
  if (linked === undefined) {
    return;
  }
  const { channel } = linked;
  linked = undefined;
  endQuietly(channel);
}

/**
 * Ends the Cast session of a cast to a Google Cast device once the cast is over, as the live
 * connection says it (castSentNow); mounted where the page always has it, the status tray, since the
 * Cast menu closes once the cast is taken.
 */
export function useEndsTheCastSessionWithItsCast(): void {
  const castId = useLive().sending?.castId;
  useEffect(() => {
    castSentNow(castId);
  }, [castId]);
}
