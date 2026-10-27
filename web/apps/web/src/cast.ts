// SPDX-License-Identifier: MPL-2.0

/**
 * The server's casts between a person's own devices (D337): starting one, the ones this device
 * sends or receives, moving one on to the next episode, and ending one.
 */

import type { Api, components } from "@orpheum/sdk";

import type { StringStorage } from "./browser-storage.ts";
import { idempotent, newIdempotencyKey } from "./idempotent.ts";
import { dataOrThrow } from "./problem.ts";

/** A cast as one of its devices sees it, with this device's credential for its session of one. */
export type Cast = components["schemas"]["CastBody"];

/**
 * Why a device ends a cast: control taken back on the receiver, Stop on the remote, the film over;
 * or, asked whether to play it, the receiver's Not now or no answer (D358 §3, §4).
 */
export type CastEnd = "takenBack" | "stopped" | "filmEnded" | "declined" | "unanswered";

/**
 * How long a receiver asks whether to play a cast before it takes no answer as no, in milliseconds
 * (D358 §4): a person across the room finds the remote and presses OK well within a minute, and a
 * sender left waiting longer has given up on it. A justified constant.
 */
export const castAnswerWithinMs = 60_000;

/**
 * How much longer than castAnswerWithinMs a sender waits for its receiver's answer before ending the
 * cast as unanswered itself, in milliseconds: a receiver whose page went away answers nothing, and
 * one still there ends it first, its own minute counted from the moment it was handed the cast. A
 * justified constant, a few round trips.
 */
export const castAnswerGraceMs = 5000;

/** Where a cast stood as it ended. */
export type CastEnded = components["schemas"]["EndCastOutputBody"];

/**
 * Casts fileId from this device to toDeviceId, another of the person's connected now, from
 * positionMs, or, undefined, from where the person left it.
 */
export async function startCast(
  api: Api,
  cast: { readonly toDeviceId: number; readonly fileId: number; readonly positionMs?: number },
): Promise<Cast> {
  const positionMs =
    cast.positionMs === undefined ? {} : { positionMs: Math.max(0, Math.round(cast.positionMs)) };
  return dataOrThrow(
    await api.POST("/me/casts", {
      body: { toDeviceId: cast.toDeviceId, fileId: cast.fileId, ...positionMs },
      ...idempotent(newIdempotencyKey()),
    }),
  );
}

/** The casts this device sends or receives, each with a new credential for it. */
export async function listMyCasts(api: Api): Promise<Cast[]> {
  return dataOrThrow(await api.GET("/me/casts")).casts ?? [];
}

/** Ends the cast castId, saying why, and says where it stood. */
export async function endCast(api: Api, castId: number, reason: CastEnd): Promise<CastEnded> {
  return dataOrThrow(
    await api.POST("/me/casts/{id}/end", {
      params: { path: { id: castId } },
      body: { reason },
      ...idempotent(newIdempotencyKey()),
    }),
  );
}

/**
 * Moves the cast castId on to what plays after its film, the next episode, which its receiver is
 * handed (plans/up-next.md), and says the cast as it is now.
 */
export async function moveCastOn(api: Api, castId: number): Promise<Cast> {
  return dataOrThrow(
    await api.POST("/me/casts/{id}/next", {
      params: { path: { id: castId } },
      ...idempotent(newIdempotencyKey()),
    }),
  );
}

/**
 * The tab's key that says it sends a cast, while it does, so a film played here goes on to its
 * receiver (plans/cast-to-device.md §9 Q3); the cast itself is the server's (listMyCasts).
 */
export const castSendingKey = "orpheum.cast.sending";

/** Notes in storage that the tab sends a cast, or, isSending false, that it no longer does. */
export function noteSending(storage: StringStorage, isSending: boolean): void {
  if (isSending) {
    storage.setItem(castSendingKey, "1");
  } else {
    storage.removeItem(castSendingKey);
  }
}

/** Whether storage says the tab sends a cast. */
export function isNotedSending(storage: StringStorage): boolean {
  return storage.getItem(castSendingKey) === "1";
}
