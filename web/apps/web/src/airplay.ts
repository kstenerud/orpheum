// SPDX-License-Identifier: MPL-2.0

/**
 * AirPlay from the Play page (D473, D523): the grant in the media's path, asked for as the Cast menu
 * opens on the core's name the page reaches it at (grantOriginOf), and revoked as the player goes;
 * and the film this tab plays that AirPlay is offered for (useOffersAirPlay, airplay-here.ts), its grant
 * the element's alternative, which WebKit hands to the receiver itself, and back to hls.js once
 * AirPlay is off.
 * The grant is the person's session's (sessionAirPlayGrants), or, on a Watch Together guest by
 * link's page, their membership's (D519 §1), as AirPlayGrantsContext says around the player.
 */

import type { Api } from "@orpheum/sdk";
import {
  followAirPlay,
  isAirPlayOffered,
  offerAirPlayAlternative,
  reloadOnAirPlay,
  showAirPlayPicker,
} from "@orpheum/player";
import { createContext, use, useEffect, useRef, type RefObject } from "react";

import { offerAirPlayHere, tellAirPlayChanged, type AirPlayHere } from "./airplay-here.ts";
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

// The revocation sent as AirPlay's last player went: a grant asked for after it waits for it, so that
// a revocation of everything the page made cannot land after the grant, revoking it too.
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

/** Where the film stood as AirPlay ended, in s, and whether it was paused there. */
export interface AirPlayLeft {
  readonly positionS: number;
  readonly wasPaused: boolean;
}

/**
 * How a film on its ladder takes part in AirPlay (D523): it offers each grant's alternative again as
 * its playback attaches anew (offered), is put back on hls.js where AirPlay left it (takeBack), and
 * says the audio track it plays now, by its place, which AirPlay is to play (D521 §2).
 */
export interface AirPlayHandOff {
  readonly offered: (url: string) => void;
  readonly takeBack: (left: AirPlayLeft) => void;
  readonly playingTrack: () => number | undefined;
}

/**
 * Offers AirPlay for the playback mediaToken names in the video, while the calling player lives,
 * where the page offers AirPlay (isAirPlayOffered), its grant asked for and revoked by whom
 * AirPlayGrantsContext names (D523). Started from the Cast menu (airPlayHere), the grant is the
 * element's alternative, which WebKit moves the film to; AirPlay is on as the wireless flag says
 * (followAirPlay), and once it is off the film is handed back (handOff.takeBack). A track chosen
 * while AirPlay plays, playingTrack, is reloaded on the receiver from a grant of its own (D523 §3).
 * The grants are revoked as the player goes, if AirPlay was started; one made for a menu's opening
 * alone ends with its day.
 */
export function useOffersAirPlay(
  videoRef: RefObject<HTMLVideoElement | null>,
  playback: { readonly mediaToken: string | undefined; readonly playingTrack: number | undefined },
  handOff: AirPlayHandOff,
): void {
  const { mediaToken, playingTrack } = playback;
  const { api, pathMoves } = useServices();
  const grants = use(AirPlayGrantsContext);
  const handOffRef = useLatest(handOff);
  const offerRef = useRef<AirPlayOffer | undefined>(undefined);
  useEffect(() => {
    const video = videoRef.current;
    if (video === null || mediaToken === undefined || !isAirPlayOffered(video, globalThis)) {
      return undefined;
    }
    const offer = new AirPlayOffer(video, { mediaToken, grants, handOffRef }, { api, pathMoves });
    offerRef.current = offer;
    return () => {
      offerRef.current = undefined;
      offer.end();
    };
  }, [videoRef, mediaToken, api, pathMoves, grants, handOffRef]);
  useEffect(() => {
    offerRef.current?.soundChanged();
  }, [playingTrack]);
}

/**
 * AirPlay offered for one playback in its video, till end (useOffersAirPlay): here, in the Cast menu's
 * reach while it lasts; on as the wireless flag says (followAirPlay), the film handed back once off; a
 * sound changed while on reloaded on the receiver from a grant of its own (D523 §3); its grants
 * revoked at its end where AirPlay was started.
 */
class AirPlayOffer {
  readonly #video: HTMLVideoElement;
  readonly #handOffRef: RefObject<AirPlayHandOff>;
  readonly #grants: AirPlayGrants;
  readonly #services: Pick<ReturnType<typeof useServices>, "api" | "pathMoves">;
  readonly #here: AirPlayHere;
  readonly #stopFollowing: () => void;
  readonly #unoffer: () => void;
  #isOn = false;
  #isGone = false;
  #hasStarted = false;
  #grantedTrack: number | undefined;

  constructor(
    video: HTMLVideoElement,
    offered: {
      readonly mediaToken: string;
      readonly grants: AirPlayGrants;
      readonly handOffRef: RefObject<AirPlayHandOff>;
    },
    services: Pick<ReturnType<typeof useServices>, "api" | "pathMoves">,
  ) {
    this.#video = video;
    this.#handOffRef = offered.handOffRef;
    this.#grants = offered.grants;
    this.#services = services;
    this.#here = {
      mediaToken: offered.mediaToken,
      grants: offered.grants,
      audioTrackIndex: () => this.#handOffRef.current.playingTrack(),
      isPlaying: () => this.#isOn,
      start: (url) => {
        this.#start(url);
      },
      showControls: () => {
        showAirPlayPicker(video);
      },
    };
    this.#stopFollowing = followAirPlay(video, {
      onOn: () => {
        this.#isOn = true;
        tellAirPlayChanged();
      },
      onOff: () => {
        this.#isOn = false;
        tellAirPlayChanged();
        this.#handOffRef.current.takeBack({
          positionS: video.currentTime,
          wasPaused: video.paused,
        });
      },
    });
    this.#unoffer = offerAirPlayHere(this.#here);
  }

  /** Within the press: the grant at url offered as the alternative, and the picker shown (D523 §1). */
  #start(url: string): void {
    this.#hasStarted = true;
    this.#grantedTrack = this.#handOffRef.current.playingTrack();
    this.#handOffRef.current.offered(url);
    offerAirPlayAlternative(this.#video, url);
    showAirPlayPicker(this.#video);
  }

  /** The film's sound changed: while on AirPlay, reloaded there from a grant for it (D523 §3). */
  soundChanged(): void {
    const track = this.#handOffRef.current.playingTrack();
    const { api, pathMoves } = this.#services;
    const origin = grantOriginOf(pathMoves, globalThis.location.origin, noLog);
    if (!this.#isOn || track === this.#grantedTrack || origin === undefined) {
      return;
    }
    this.#grantedTrack = track;
    askAirPlayGrant(this.#here, { api, log: noLog }, origin)
      .then((grant) => {
        if (!this.#isGone && this.#isOn) {
          this.#handOffRef.current.offered(grant.url);
          reloadOnAirPlay(this.#video, grant.url);
        }
      })
      .catch(() => undefined); // the receiver plays on with the sound it has
  }

  /** Its end, as the player goes: the grants revoked where AirPlay was started. */
  end(): void {
    this.#isGone = true;
    this.#stopFollowing();
    this.#unoffer();
    if (this.#hasStarted) {
      const doing = { api: this.#services.api, log: noLog };
      revoking = this.#grants.revoke(doing).catch(() => undefined); // its day ends it else
    }
  }
}
