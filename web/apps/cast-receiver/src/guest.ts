// SPDX-License-Identifier: MPL-2.0

import {
  createApi,
  dataOrThrow,
  failureText,
  targetOfGuestGrant,
  type HeldSession,
} from "@orpheum/sdk";

import { CastDevice } from "./device.ts";
import { coreReachOf, reachThrough, type CoreReach, type Reached } from "./reach.ts";

/**
 * A cast to any Google Cast device under a grant for that cast alone (D465 §2): the sender's launch
 * message carries the grant and how to reach its core; the receiver trades the grant for a guest
 * session, tells the sender which device it is, and takes the cast as a paired device does
 * (device.ts), keeping nothing: the guest's device is gone once its cast ends.
 */

/** The namespace a sender casting under a grant speaks on (the web app's google-cast.ts). */
export const castNamespace = "urn:x-cast:org.orpheum.cast";

/** What a sender casting under a grant hands the receiver: the grant, and its core's reach. */
export interface GuestRequest {
  readonly grant: string;
  readonly reach: CoreReach;
}

/** A sender's message as a cast under a grant, or undefined for anything else. */
export function guestRequestOf(message: unknown): GuestRequest | undefined {
  if (typeof message !== "object" || message === null) {
    return undefined;
  }
  const type: unknown = Reflect.get(message, "type");
  const grant: unknown = Reflect.get(message, "grant");
  const reach = coreReachOf(Reflect.get(message, "reach"));
  return type === "guest" && typeof grant === "string" && grant !== "" && reach !== undefined
    ? { grant, reach }
    : undefined;
}

/** What the receiver answers the sender: the device it is now, or why it could not become it. */
export type GuestAnswer =
  | { readonly type: "ready"; readonly deviceId: number }
  | { readonly type: "failed"; readonly reason: string };

/** What a cast under a grant needs. */
export interface GuestSteps {
  /**
   * Reaches the sender's core, its offers proven by the grant until traded() holds the session the
   * trade gave, and by that session after (the owner, 2026-11-12).
   */
  readonly reach: (traded: () => HeldSession | undefined) => Promise<Reached>;
  readonly video: HTMLVideoElement;
  readonly note: (line: string) => void;
  readonly answer: (message: GuestAnswer) => void;
  /** Called as the cast is taken, before its film plays on the video: the lab stops its own film. */
  readonly onTaken: () => void;
}

/**
 * Reaches the core request names through its discovery, as reachThrough does, its first offer
 * proven by the grant (targetOfGuestGrant).
 */
export function reachOfGrant(
  request: GuestRequest,
  note: (line: string) => void,
): GuestSteps["reach"] {
  const { reach, grant } = request;
  const core = { coreId: reach.coreId, fingerprint: reach.fingerprint };
  return async (traded) =>
    await reachThrough(
      reach,
      (client) => targetOfGuestGrant(client, { core, grant, senderLocator: reach.locator }, traded),
      note,
    );
}

/**
 * Takes the cast request's grant is for: reaches the core, trades the grant, connects as the device
 * the trade made, and once the core has welcomed it answers the sender which device it is, taking
 * the cast handed to it. Returns its
 * stopping, or undefined where it failed, which it answers too. Nothing is kept in storage.
 */
export async function castAsGuest(
  request: GuestRequest,
  steps: GuestSteps,
): Promise<(() => void) | undefined> {
  const { note } = steps;
  let traded: HeldSession | undefined;
  let reached: Reached | undefined;
  try {
    note("casting under a grant: reaching the sender's core");
    reached = await steps.reach(() => traded);
    const api = createApi({ getToken: () => traded?.token, fetch: reached.fetch });
    const session = dataOrThrow(
      await api.POST("/guest-receivers", { body: { grant: request.grant } }),
    );
    const core = { coreId: request.reach.coreId, fingerprint: request.reach.fingerprint };
    traded = { core, token: session.token, locator: session.locator };
    note(`traded the grant: device ${String(session.deviceId)}`);
    const way = reached;
    let isStopped = false;
    let isAnswered = false;
    const device = new CastDevice({
      api,
      token: session.token,
      reached: way,
      video: steps.video,
      note,
      // Refused once its cast ends: the guest's device is removed then (D465 §2); refused before
      // it was ever welcomed, the sender is told it failed.
      onRefused: () => {
        if (!isAnswered) {
          isAnswered = true;
          steps.answer({ type: "failed", reason: "the core refused the device's live connection" });
        }
        stop();
      },
      onTaken: steps.onTaken,
      // Ready once the core has welcomed its live connection, not at the trade: the sender's cast
      // to it is refused while it is not yet one of its person's devices connected now.
      onConnected: () => {
        if (!isAnswered) {
          isAnswered = true;
          steps.answer({ type: "ready", deviceId: session.deviceId });
        }
      },
    });
    const stopDevice = device.start();
    const stop = (): void => {
      if (!isStopped) {
        isStopped = true;
        stopDevice();
        way.close();
      }
    };
    return stop;
  } catch (cause) {
    reached?.close();
    note(`could not cast under the grant: ${failureText(cause)}`);
    steps.answer({ type: "failed", reason: failureText(cause) });
    return undefined;
  }
}
