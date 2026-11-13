// SPDX-License-Identifier: MPL-2.0

/**
 * AirPlay from the Play page (D473): the grant in the media's path, asked for as the Cast menu opens
 * on the core's name the page reaches it at (grantOriginOf), and revoked as AirPlay ends; and the
 * film this tab plays that AirPlay is offered for (useOffersAirPlay, airPlayHere), handed to the
 * receiver from the page's own element by the player (playOnAirPlay) and back to hls.js at its end.
 */

import type { Api } from "@orpheum/sdk";
import {
  isAirPlayOffered,
  playOnAirPlay,
  type AirPlayLeft,
  type AirPlaying,
} from "@orpheum/player";
import { useEffect, type RefObject } from "react";

import { idempotent, newIdempotencyKey } from "./idempotent.ts";
import { useLatest } from "./latest.ts";
import { dataOrThrow } from "./problem.ts";
import { useServices, type PathMoves } from "./services.tsx";

/** A line for the page's log. */
export type Log = (line: string) => void;

/** The grant made for a playback: its master playlist's whole address, and when it ends. */
export interface AirPlayGrant {
  readonly url: string;
  readonly expiresAt: string;
}

/** What a step is given: the server, and the log it writes each step to. */
export interface AirPlayDoing {
  readonly api: Api;
  readonly log: Log;
}

/**
 * The origin the grant's address is resolved on, the one the receiver must reach the core at
 * (plans/airplay.md item 2): on the core's own origin, where pathMoves names no direct origin, the
 * page's, pageOrigin; under a discovery's loader, the core's name the page's requests reach it at,
 * never the page's, the client origin, which serves no media to a player running none of the page's
 * code (the owner's staging test, 2026-11-13: the Mac fetched nothing). Undefined, the reason
 * logged, where the core is reached over WebRTC alone, which no receiver can fetch through, or the
 * loader is too old to say which name it reached.
 */
export function grantOriginOf(
  pathMoves: Pick<PathMoves, "directOrigin" | "isOverWebrtc"> | undefined,
  pageOrigin: string,
  log: Log,
): string | undefined {
  if (pathMoves?.directOrigin === undefined) {
    return pageOrigin;
  }
  const origin = pathMoves.directOrigin();
  if (origin === undefined) {
    log(
      pathMoves.isOverWebrtc()
        ? "the core is reached over WebRTC alone: a receiver cannot reach it, so AirPlay cannot work here (airplay.md item 2); no grant asked for"
        : "the loader does not say which of the core's names it reached (older than its minor 6): no address a receiver reaches; no grant asked for",
    );
  }
  return origin;
}

/**
 * Asks for an AirPlay grant (D473) for the playback mediaToken names, its address resolved on
 * origin, the address the receiver is handed and must reach itself.
 */
export async function makeAirPlayGrant(
  doing: AirPlayDoing,
  mediaToken: string,
  origin: string,
): Promise<AirPlayGrant> {
  doing.log("asking for the grant (POST /airplay/grants)");
  // Each ask is one click's (a way's button, or the menu opening for way B), under its own key (D244 §4).
  const made = dataOrThrow(
    await doing.api.POST("/airplay/grants", {
      body: { mediaToken },
      ...idempotent(newIdempotencyKey()),
    }),
  );
  const grant = { url: new URL(made.masterPath, origin).href, expiresAt: made.expiresAt };
  doing.log(`grant fetched: ${grant.url}, until ${grant.expiresAt}`);
  return grant;
}

/** Revokes the session's AirPlay grants, as Stop does, and says how many were revoked. */
export async function revokeAirPlayGrants(doing: AirPlayDoing): Promise<number> {
  doing.log("revoking the grants (DELETE /airplay/grants)");
  const { revoked } = dataOrThrow(await doing.api.DELETE("/airplay/grants", {}));
  doing.log(`revocation answered: ${String(revoked)} revoked`);
  return revoked;
}

/** A log that keeps nothing, for the product's steps, which the lab's log names. */
export const noLog: Log = () => undefined;

/** The film this tab plays that AirPlay is offered for, as the Cast menu reaches it. */
export interface AirPlayHere {
  /** The playback's media token, which its grant is asked for by. */
  readonly mediaToken: string;
  /** Whether the film plays on AirPlay now. */
  readonly isPlaying: () => boolean;
  /** Hands the film to AirPlay from the grant at url, within the press it is called in. */
  readonly start: (url: string) => void;
  /** Ends AirPlay, handing the film back to the page where it stood. */
  readonly stop: () => void;
}

let offered: AirPlayHere | undefined;

/** The film this tab offers AirPlay for now, if any. */
export function airPlayHere(): AirPlayHere | undefined {
  return offered;
}

/** How a film on its ladder lets its element go to AirPlay, and takes it back where it stood. */
export interface AirPlayHandOff {
  readonly letGoOfHls: () => void;
  readonly takeBack: (left: AirPlayLeft) => void;
}

/**
 * Offers AirPlay for the playback mediaToken names in the video, while the calling player lives,
 * where the page offers AirPlay (isAirPlayOffered): started from the Cast menu (airPlayHere), it is
 * handed back (handOff) and its grant revoked as AirPlay ends, at Stop or as the target stops being
 * wireless; the player going away ends it too.
 */
export function useOffersAirPlay(
  videoRef: RefObject<HTMLVideoElement | null>,
  mediaToken: string | undefined,
  handOff: AirPlayHandOff,
): void {
  const { api } = useServices();
  const handOffRef = useLatest(handOff);
  useEffect(() => {
    const video = videoRef.current;
    if (video === null || mediaToken === undefined || !isAirPlayOffered(video, globalThis)) {
      return undefined;
    }
    let playing: AirPlaying | undefined;
    let isGone = false;
    const here: AirPlayHere = {
      mediaToken,
      isPlaying: () => playing !== undefined,
      start: (url) => {
        if (playing !== undefined) {
          return;
        }
        playing = playOnAirPlay(video, url, {
          letGoOfHls: () => {
            handOffRef.current.letGoOfHls();
          },
          onEnded: (left) => {
            playing = undefined;
            revokeAirPlayGrants({ api, log: noLog }).catch(() => undefined); // its day ends it else
            if (!isGone) {
              handOffRef.current.takeBack(left);
            }
          },
        });
      },
      stop: () => {
        playing?.stop();
      },
    };
    offered = here;
    return () => {
      isGone = true;
      playing?.stop();
      if (offered === here) {
        offered = undefined;
      }
    };
  }, [videoRef, mediaToken, api, handOffRef]);
}
