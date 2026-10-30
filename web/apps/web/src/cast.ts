// SPDX-License-Identifier: MPL-2.0

/**
 * The server's casts between a person's own devices (D337), each a session with a device while it
 * lasts (D364): starting one, playing in it, its receiver's Use, the ones this device sends or
 * receives, moving one on to the next episode, and ending one.
 */

import type { Api, components } from "@orpheum/sdk";

import { browserStorage, tabStorage } from "./browser-storage.ts";
import { idempotent, newIdempotencyKey } from "./idempotent.ts";
import { dataOrThrow } from "./problem.ts";
import { readMembership } from "./together-storage.ts";

/** A cast as one of its devices sees it, with this device's credential for its session of one. */
export type Cast = components["schemas"]["CastBody"];

/**
 * Why a device ends a cast: control taken back on the receiver, Stop casting on the sender; or, asked
 * whether to take it, the receiver's Not now or no answer (D358 §3, §4).
 */
export type CastEnd = "takenBack" | "stopped" | "declined" | "unanswered";

/**
 * How long a receiver asks whether to take a cast before it takes no answer as no, in milliseconds
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

/** What a cast plays: a film from a position, or, from undefined, where the person left it. */
export interface CastFilm {
  readonly fileId: number;
  readonly positionMs?: number | undefined;
}

/** A film's fields in a cast request: none for nothing, the receiver ready (D364 §5). */
function filmFields(film: CastFilm | undefined): { readonly fileId?: number; positionMs?: number } {
  if (film === undefined) {
    return {};
  }
  return film.positionMs === undefined
    ? { fileId: film.fileId }
    : { fileId: film.fileId, positionMs: Math.max(0, Math.round(film.positionMs)) };
}

/**
 * Starts casting from this device to toDeviceId, another of the person's connected now: a session
 * with it (D364), playing film, the video playing here as it starts (§1), or nothing for undefined.
 * A page that is a Watch Together guest's, hosting nothing, says its membership, whose part the
 * receiver takes, following the group (D371 §4): the person's own device joined as a guest of the
 * session they host casts as its guest, not as their host's player (VQ193).
 */
export async function startCast(
  api: Api,
  toDeviceId: number,
  film: CastFilm | undefined,
): Promise<Cast> {
  const guests =
    readMembership(tabStorage(), "host") === undefined
      ? readMembership(browserStorage(), "viewer")
      : undefined;
  return dataOrThrow(
    await api.POST("/me/casts", {
      body: {
        toDeviceId,
        ...filmFields(film),
        ...(guests === undefined ? {} : { togetherCredential: guests.credential }),
      },
      ...idempotent(newIdempotencyKey()),
    }),
  );
}

/**
 * Plays film on the receiver of the cast castId, or, for undefined, nothing, the receiver ready for
 * the next video (D364 §3, §5), and says the cast as it is now.
 */
export async function playInCast(
  api: Api,
  castId: number,
  film: CastFilm | undefined,
): Promise<Cast> {
  return dataOrThrow(
    await api.POST("/me/casts/{id}/play", {
      params: { path: { id: castId } },
      body: filmFields(film),
      ...idempotent(newIdempotencyKey()),
    }),
  );
}

/** Says Use, on its receiver, to the cast castId, which it is asked once (D364 §2). */
export async function acceptCast(api: Api, castId: number): Promise<Cast> {
  return dataOrThrow(
    await api.POST("/me/casts/{id}/accept", {
      params: { path: { id: castId } },
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
