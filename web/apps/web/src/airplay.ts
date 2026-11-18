// SPDX-License-Identifier: MPL-2.0

/**
 * AirPlay from the Play page (D473): the grant in the media's path, asked for as the Cast menu opens
 * on the core's name the page reaches it at (grantOriginOf), and revoked as AirPlay ends; and the
 * film this tab plays that AirPlay is offered for (useOffersAirPlay, airPlayHere), handed to the
 * receiver from the page's own element by the player (playOnAirPlay) and back to hls.js at its end.
 * The grant is the person's session's (sessionAirPlayGrants), or, on a Watch Together guest by
 * link's page, their membership's (D519 §1), as AirPlayGrantsContext says around the player.
 */

import type { Api } from "@orpheum/sdk";
import {
  isAirPlayOffered,
  playOnAirPlay,
  type AirPlayLeft,
  type AirPlaying,
} from "@orpheum/player";
import { createContext, use, useEffect, useSyncExternalStore, type RefObject } from "react";

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
 * What a grant is asked for: the playback its media token names, and the audio track the page plays
 * now, by its place among the film's, which the grant's master playlist names its default (D521 §2);
 * none, the playback's starting track.
 */
export interface AirPlayGrantAsked {
  readonly mediaToken: string;
  readonly audioTrackIndex?: number | undefined;
}

/** The body a grant is asked for with, naming no audio track where asked names none. */
export function grantBody(asked: AirPlayGrantAsked): {
  readonly mediaToken: string;
  readonly audioTrackIndex?: number;
} {
  const { mediaToken, audioTrackIndex } = asked;
  return audioTrackIndex === undefined ? { mediaToken } : { mediaToken, audioTrackIndex };
}

/**
 * Asks for the AirPlay grant (D473) asked names, its address resolved on origin, the address the
 * receiver is handed and must reach itself.
 */
export async function makeAirPlayGrant(
  doing: AirPlayDoing,
  asked: AirPlayGrantAsked,
  origin: string,
): Promise<AirPlayGrant> {
  doing.log("asking for the grant (POST /airplay/grants)");
  // Each ask is one click's (a way's button, or the menu opening for way B), under its own key (D244 §4).
  const made = dataOrThrow(
    await doing.api.POST("/airplay/grants", {
      body: grantBody(asked),
      ...idempotent(newIdempotencyKey()),
    }),
  );
  return grantAt(made, origin, doing.log);
}

/** The grant the server made, its master path resolved on origin, logged. */
export function grantAt(
  made: { readonly masterPath: string; readonly expiresAt: string },
  origin: string,
  log: Log,
): AirPlayGrant {
  const grant = { url: new URL(made.masterPath, origin).href, expiresAt: made.expiresAt };
  log(`grant fetched: ${grant.url}, until ${grant.expiresAt}`);
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

/** How AirPlay's grant is asked for (make) and revoked at AirPlay's end (revoke), and by whom. */
export interface AirPlayGrants {
  readonly make: (
    doing: AirPlayDoing,
    asked: AirPlayGrantAsked,
    origin: string,
  ) => Promise<AirPlayGrant>;
  readonly revoke: (doing: AirPlayDoing) => Promise<number>;
}

/** The person's session's grants (POST and DELETE /airplay/grants, D473). */
export const sessionAirPlayGrants: AirPlayGrants = {
  make: makeAirPlayGrant,
  revoke: revokeAirPlayGrants,
};

/** Whose grants the player under it AirPlays by: the person's session's, but a guest's page's. */
export const AirPlayGrantsContext = createContext<AirPlayGrants>(sessionAirPlayGrants);

// The revocation sent as AirPlay last ended: a grant asked for after it waits for it, so that a
// revocation of everything the page made cannot land after the grant, revoking it too.
let revoking: Promise<unknown> = Promise.resolve();

/**
 * Asks for here's grant, by whoever here AirPlays by, once AirPlay's last revocation is answered,
 * with the audio track here plays as it is asked (D521 §2).
 */
export async function askAirPlayGrant(
  here: AirPlayHere,
  doing: AirPlayDoing,
  origin: string,
): Promise<AirPlayGrant> {
  await revoking;
  const asked = { mediaToken: here.mediaToken, audioTrackIndex: here.audioTrackIndex?.() };
  return await here.grants.make(doing, asked, origin);
}

/** The film this tab plays that AirPlay is offered for, as the Cast menu reaches it. */
export interface AirPlayHere {
  /** The playback's media token, which its grant is asked for by. */
  readonly mediaToken: string;
  /** Who its grant is asked for and revoked by. */
  readonly grants: AirPlayGrants;
  /** The audio track the film plays now, by its place, which AirPlay plays too; none, its starting one. */
  readonly audioTrackIndex?: () => number | undefined;
  /** Whether the film plays on AirPlay now. */
  readonly isPlaying: () => boolean;
  /** Hands the film to AirPlay from the grant at url, within the press it is called in. */
  readonly start: (url: string) => void;
  /**
   * Within a press, shows AirPlay's picker for the person to choose this device, AirPlay ending as
   * they do, the film handed back to the page where it stood (D521 §1).
   */
  readonly askToStop: () => void;
}

let offered: AirPlayHere | undefined;
const watchers = new Set<() => void>();

function tellWatchers(): void {
  for (const watcher of watchers) {
    watcher();
  }
}

function watchOffered(watcher: () => void): () => void {
  watchers.add(watcher);
  return () => {
    watchers.delete(watcher);
  };
}

/** The film this tab offers AirPlay for now, if any. */
export function airPlayHere(): AirPlayHere | undefined {
  return offered;
}

/**
 * The film this tab offers AirPlay for, re-rendering as another is offered or it goes: a menu or
 * dialog left open as the film changes offers the new one's.
 */
export function useAirPlayHere(): AirPlayHere | undefined {
  return useSyncExternalStore(watchOffered, airPlayHere, airPlayHere);
}

/** Offers AirPlay for here, the film this tab plays, till the returned function takes it back. */
export function offerAirPlayHere(here: AirPlayHere): () => void {
  offered = here;
  tellWatchers();
  return () => {
    if (offered === here) {
      offered = undefined;
      tellWatchers();
    }
  };
}

/**
 * How a film on its ladder lets its element go to AirPlay, and takes it back where it stood; and the
 * audio track it plays now, by its place, which AirPlay is to play (D521 §2).
 */
export interface AirPlayHandOff {
  readonly letGoOfHls: () => void;
  readonly takeBack: (left: AirPlayLeft) => void;
  readonly playingTrack: () => number | undefined;
}

/**
 * Offers AirPlay for the playback mediaToken names in the video, while the calling player lives,
 * where the page offers AirPlay (isAirPlayOffered): started from the Cast menu (airPlayHere), it is
 * handed back (handOff) and its grant revoked as AirPlay ends, as the target stops being wireless (the
 * person choosing this device, in the picker Stop shows or in Control Center); the player going away
 * ends it at once. Its grant is asked for and revoked by whom
 * AirPlayGrantsContext names.
 */
export function useOffersAirPlay(
  videoRef: RefObject<HTMLVideoElement | null>,
  mediaToken: string | undefined,
  handOff: AirPlayHandOff,
): void {
  const { api } = useServices();
  const grants = use(AirPlayGrantsContext);
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
      grants,
      audioTrackIndex: () => handOffRef.current.playingTrack(),
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
            revoking = grants.revoke({ api, log: noLog }).catch(() => undefined); // its day ends it else
            if (!isGone) {
              handOffRef.current.takeBack(left);
            }
          },
        });
      },
      askToStop: () => {
        playing?.askToStop();
      },
    };
    const unoffer = offerAirPlayHere(here);
    return () => {
      isGone = true;
      playing?.stop();
      unoffer();
    };
  }, [videoRef, mediaToken, api, grants, handOffRef]);
}
