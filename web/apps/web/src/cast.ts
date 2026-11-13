// SPDX-License-Identifier: MPL-2.0

/**
 * The server's casts between a person's own devices (D337), each a session with a device while it
 * lasts (D364): starting one, playing in it, its receiver's Use, the ones this device sends or
 * receives, moving one on to the next episode, and ending one.
 */

import type { Api, components } from "@orpheum/sdk";

import { browserStorage, tabStorage } from "./browser-storage.ts";
import { idempotent, newIdempotencyKey } from "./idempotent.ts";
import { ApiProblem, dataOrThrow } from "./problem.ts";
import { readMembership } from "./together-storage.ts";

/** A cast as one of its devices sees it, with this device's credential for its session of one. */
export type Cast = components["schemas"]["CastBody"];

/**
 * Why a device ends a cast: Stop casting here on the receiver, Stop casting on the sender; or a
 * receiver that never took it (unanswered), as its sender gives up on it. A receiver built before
 * D468 still sends its person's Not now (declined) or no answer (D358 §3, §4, D446 §1). The server's
 * own list, so no reason it refuses is sent.
 */
export type CastEnd = components["schemas"]["EndCastInputBody"]["reason"];

/**
 * How long a sender waits for its receiver to take a cast before ending it as unanswered itself, in
 * milliseconds: a receiver takes it at once, a round trip after it is handed (D468 §1), but one whose
 * page went away takes nothing, and a receiver built before D468, its page not loaded again since,
 * still asks its person for a minute (D358 §4) and ends the cast itself at its end; a few round trips
 * past that minute. A justified constant.
 */
export const castTakenWithinMs = 65_000;

/** Where a cast stood as it ended. */
export type CastEnded = components["schemas"]["EndCastOutputBody"];

/** What a cast plays: a film from a position, or, from undefined, where the person left it. */
export interface CastFilm {
  readonly fileId: number;
  readonly positionMs?: number | undefined;
  /** Whether it stands paused here, so a cast started from it opens it paused (the owner, 2026-11-08). */
  readonly isPaused?: boolean | undefined;
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
        ...(film?.isPaused === true ? { isPaused: true } : {}),
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

/** Takes, on its receiver, the cast castId, at once as it is handed, asking nothing (D468 §1). */
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
 * Ends the cast castId as endCast does, tried once more where the first try fails: a request lost on
 * a connection that has just dropped, the next going on its replacement (principles §28; the owner,
 * 2026-11-03). A cast the server no longer has is ended, answering undefined: nothing is left to
 * end. Fails where both tries do, for the person to be told.
 */
export async function endCastRetrying(
  api: Api,
  castId: number,
  reason: CastEnd,
): Promise<CastEnded | undefined> {
  const ending = (): Promise<CastEnded | undefined> =>
    endCast(api, castId, reason).catch(undefinedIfGone);
  return await ending().catch(ending);
}

/** Takes a refusal that the cast is gone as its end, answering undefined; rethrows any other. */
function undefinedIfGone(cause: unknown): undefined {
  if (cause instanceof ApiProblem && cause.code === "notFound") {
    return undefined;
  }
  throw cause;
}

/**
 * Moves the cast castId on to what plays after its film, the next episode, which its receiver is
 * handed (plans/up-next.md), and says the cast as it is now.
 */
export async function moveCastOn(api: Api, castId: number): Promise<Cast> {
  return await movedCast(api, castId, "/me/casts/{id}/next");
}

/**
 * Moves the cast castId back to what plays before its film, its previous episode, as moveCastOn moves
 * it on (D471 §1); rejects where nothing that can be read now plays before it.
 */
export async function moveCastBack(api: Api, castId: number): Promise<Cast> {
  return await movedCast(api, castId, "/me/casts/{id}/previous");
}

/** The cast castId moved along its film's sequence by path. */
async function movedCast(
  api: Api,
  castId: number,
  path: "/me/casts/{id}/next" | "/me/casts/{id}/previous",
): Promise<Cast> {
  return dataOrThrow(
    await api.POST(path, {
      params: { path: { id: castId } },
      ...idempotent(newIdempotencyKey()),
    }),
  );
}
