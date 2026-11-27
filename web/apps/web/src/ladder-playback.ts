// SPDX-License-Identifier: MPL-2.0

/**
 * A film on its ladder, played by the player's chunked playback (D247 §10): the hook a film player
 * starts it with, which follows the quality playing and its stalls, and offers the film to AirPlay
 * and takes it back (D523).
 */

import {
  pageLinkOutages,
  playChunked,
  watchStalls,
  type ChunkedPlayback,
  type PlaybackFailed,
  type PlayedLevel,
} from "@orpheum/player";
import { playerSwitchedQualityEvent, startGateOf, type CannotKeepUp } from "@orpheum/sdk";
import { useEffect, useMemo, useRef, useState, type RefObject } from "react";

import { useOffersAirPlay, type AirPlayHandOff } from "./airplay.ts";
import { useLatest } from "./latest.ts";
import { deviceDataCapKbps } from "./data-cap.ts";
import { startOn, type LadderOffer } from "./moving-down.ts";
import { noteIfStartRefused } from "./start-refused.ts";
import { monotonicMs } from "./use-together.ts";

/**
 * The link an offer says its device last measured on its network (D257 §1), with its spread and how
 * many samples it counts as (bandwidth-sampling-2026-10-29.md P2), and whether the player starts by
 * it (D381), one object while they stay the same, so a playback is not started again for a new offer
 * that says the same.
 */
function useRememberedLink(
  offer: Pick<
    LadderOffer,
    | "rememberedLinkKbps"
    | "rememberedLinkSpread"
    | "rememberedLinkSamples"
    | "isStartHeldByNodeSpeed"
  >,
  shouldStartByRememberedLink: boolean | undefined,
): {
  readonly rememberedLinkKbps: number | undefined;
  readonly rememberedLinkSpread: number | undefined;
  readonly rememberedLinkSamples: number | undefined;
  readonly shouldStartByRememberedLink: boolean | undefined;
  readonly isStartHeldByNodeSpeed: boolean | undefined;
} {
  const { rememberedLinkKbps, rememberedLinkSpread, rememberedLinkSamples } = offer;
  const { isStartHeldByNodeSpeed } = offer;
  return useMemo(
    () => ({
      rememberedLinkKbps,
      rememberedLinkSpread,
      rememberedLinkSamples,
      shouldStartByRememberedLink,
      isStartHeldByNodeSpeed,
    }),
    [
      rememberedLinkKbps,
      rememberedLinkSpread,
      rememberedLinkSamples,
      shouldStartByRememberedLink,
      isStartHeldByNodeSpeed,
    ],
  );
}

/**
 * Follows playback in video: the quality playing, read as it starts and at each switch the player
 * tells (playerSwitchedQualityEvent), never on a timer (efficiency audit, 2026-10-28); and, at each
 * stall, whether even its lowest quality is beyond the link (D247 §8). Returns the end of it.
 */
export function followChunkedPlayback(
  video: HTMLVideoElement,
  playback: Pick<ChunkedPlayback, "playing" | "beyondTheLink">,
  tell: {
    readonly setPlaying: (playing: PlayedLevel | undefined) => void;
    readonly setCannotKeepUp: (cannotKeepUp: CannotKeepUp) => void;
  },
): () => void {
  const switched = (): void => {
    tell.setPlaying(playback.playing());
  };
  switched();
  video.addEventListener(playerSwitchedQualityEvent, switched);
  const stopWatching = watchStalls(video, () => {
    const beyond = playback.beyondTheLink();
    if (beyond !== undefined) {
      tell.setCannotKeepUp(beyond);
    }
  });
  return () => {
    video.removeEventListener(playerSwitchedQualityEvent, switched);
    stopWatching();
  };
}

/** What a film's chunked playback (useChunkedPlayback) starts from. */
export interface ChunkedPlaybackStart {
  readonly offer: LadderOffer;
  readonly positionMs: number;
  readonly startAudioRef?: RefObject<number | undefined>;
  /** Whether it chooses its first quality from the link its device remembers (D381). */
  readonly shouldStartByRememberedLink?: boolean;
  /** The playback's media token, which AirPlay's grant is asked for by; none offers no AirPlay. */
  readonly mediaToken?: string | undefined;
  /**
   * The audio track it plays now, by its place, which AirPlay plays too (D521 §2), and reloads on
   * the receiver as it changes during AirPlay (D523 §3).
   */
  readonly playingTrack?: number | undefined;
  /** What plays it: the player's chunked playback, but in a test. */
  readonly play?: typeof playChunked;
}

/**
 * Plays the offer's chunked master playlist in the video (D247 §10), and says which quality plays
 * and, once its lowest quality stalls, that the link cannot keep up (D247 §8). Each playback starts
 * with the audio rendition startAudioRef holds as it starts, the playlist's default when none. Where
 * a media token is given and the page offers AirPlay, the film may be handed to AirPlay from the Cast
 * menu, WebKit moving it to the grant's alternative itself; once AirPlay is off, a new playback plays
 * it on hls.js where AirPlay left it, paused if it was (D523 §4, useAirPlayStage).
 */
export function useChunkedPlayback(
  videoRef: RefObject<HTMLVideoElement | null>,
  starting: ChunkedPlaybackStart,
  onFailed: PlaybackFailed,
): {
  readonly playbackRef: RefObject<ChunkedPlayback | undefined>;
  readonly playing: PlayedLevel | undefined;
  readonly cannotKeepUp: CannotKeepUp | undefined;
} {
  const { startAudioRef, offer, play = playChunked } = starting;
  const { url, startAtBps, linkSharePercent, steadiness, playlists } = offer;
  const remembered = useRememberedLink(offer, starting.shouldStartByRememberedLink);
  const playbackRef = useRef<ChunkedPlayback | undefined>(undefined);
  const [playing, setPlaying] = useState<PlayedLevel | undefined>(undefined);
  const [cannotKeepUp, setCannotKeepUp] = useState<CannotKeepUp | undefined>(undefined);
  const { backAt, alternativeRef, startMs } = useAirPlayStage(videoRef, starting);
  useEffect(() => {
    const video = videoRef.current;
    if (video === null) {
      return undefined;
    }
    keepBack(video, backAt);
    const playback = play(video, url, {
      onFailed,
      ...startAudioOf(startAudioRef),
      ...startOn({ startAtBps, linkSharePercent, steadiness, ...remembered }, startMs),
      loadedPlaylists: playlists, // the start's playlists, which hls.js need not ask for (D502 §3)
      monotonicMs,
      outages: pageLinkOutages, // loads deeper after its link was out, in this film or the next (D487)
      dataCapKbps: deviceDataCapKbps, // within this device's cap, read at each chunk (D488)
      airPlayAlternativeUrl: alternativeRef.current, // ready for the next AirPlay (D523 §4)
    });
    return holdPlayback(video, playback, { playbackRef, setPlaying, setCannotKeepUp });
  }, [
    videoRef,
    url,
    startAtBps,
    linkSharePercent,
    steadiness,
    playlists,
    remembered,
    startMs,
    onFailed,
    startAudioRef,
    backAt,
    alternativeRef,
    play,
  ]);
  return { playbackRef, playing, cannotKeepUp };
}

/**
 * Holds playback, playing in video, as the playing one (playbackRef) and follows it
 * (followChunkedPlayback); returns its letting go, which ends both and destroys it.
 */
function holdPlayback(
  video: HTMLVideoElement,
  playback: ChunkedPlayback,
  following: Parameters<typeof followChunkedPlayback>[2] & {
    readonly playbackRef: RefObject<ChunkedPlayback | undefined>;
  },
): () => void {
  const { playbackRef } = following;
  playbackRef.current = playback;
  const stopFollowing = followChunkedPlayback(video, playback, following);
  return () => {
    stopFollowing();
    playback.destroy();
    playbackRef.current = undefined;
  };
}

/** Where AirPlay left the film as it ended, which its playback starts from again, paused if it was. */
interface BackFromAirPlay {
  readonly positionMs: number;
  readonly isPaused: boolean;
}

/**
 * A chunked playback's part in AirPlay (D523), offered for its media token's playback
 * (useOffersAirPlay): the grant's alternative each new playback offers as it attaches (alternativeRef),
 * and where AirPlay left the film once it ended (backAt), a new playback put there on hls.js (§4).
 */
function useAirPlayStage(
  videoRef: RefObject<HTMLVideoElement | null>,
  starting: {
    readonly positionMs: number;
    readonly mediaToken?: string | undefined;
    readonly playingTrack?: number | undefined;
  },
): {
  readonly backAt: BackFromAirPlay | undefined;
  readonly alternativeRef: RefObject<string | undefined>;
  /** Where the playback starts: where AirPlay left it, once it did, else where it was to start. */
  readonly startMs: number;
} {
  const [backAt, setBackAt] = useState<BackFromAirPlay | undefined>(undefined);
  const alternativeRef = useRef<string | undefined>(undefined);
  const playingTrackRef = useLatest(starting.playingTrack);
  const handOff = useMemo(
    (): AirPlayHandOff => ({
      offered: (url) => {
        alternativeRef.current = url;
      },
      takeBack: (left) => {
        const positionMs = Math.round(left.positionS * 1000);
        setBackAt({ positionMs, isPaused: left.wasPaused });
      },
      playingTrack: () => playingTrackRef.current,
    }),
    [playingTrackRef],
  );
  const { mediaToken, playingTrack } = starting;
  useOffersAirPlay(videoRef, { mediaToken, playingTrack }, handOff);
  return { backAt, alternativeRef, startMs: backAt?.positionMs ?? starting.positionMs };
}

/** The audio rendition a playback starts with, as startAudioRef holds it as it starts, if any. */
function startAudioOf(startAudioRef: RefObject<number | undefined> | undefined): {
  readonly startAudioRenditionIndex?: number;
} {
  const startAudioRenditionIndex = startAudioRef?.current;
  return startAudioRenditionIndex === undefined ? {} : { startAudioRenditionIndex };
}

/**
 * Keeps a film handed back from AirPlay as AirPlay left it (backAt), if it was: playing, it plays on as hls.js has it
 * again; paused, the page's start (useStartsWithEnoughAhead) is not let to play it as it loads.
 */
function keepBack(video: HTMLVideoElement, backAt: BackFromAirPlay | undefined): void {
  if (backAt === undefined) {
    return;
  }
  if (!backAt.isPaused) {
    video.play().catch(noteIfStartRefused(video));
    return;
  }
  video.addEventListener(
    "loadstart",
    () => {
      startGateOf(video).cancel();
    },
    { once: true },
  );
}
