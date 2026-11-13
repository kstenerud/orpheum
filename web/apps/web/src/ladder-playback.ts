// SPDX-License-Identifier: MPL-2.0

/**
 * A film on its ladder, played by the player's chunked playback (D247 §10): the hook a film player
 * starts it with, which follows the quality playing and its stalls, and hands the element to AirPlay
 * and takes it back (D473).
 */

import {
  playChunked,
  watchStalls,
  type ChunkedPlayback,
  type PlaybackFailed,
  type PlayedLevel,
} from "@orpheum/player";
import { playerSwitchedQualityEvent, startGateOf, type CannotKeepUp } from "@orpheum/sdk";
import { useEffect, useMemo, useRef, useState, type RefObject } from "react";

import { useOffersAirPlay, type AirPlayHandOff } from "./airplay.ts";
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
  offer: Pick<LadderOffer, "rememberedLinkKbps" | "rememberedLinkSpread" | "rememberedLinkSamples">,
  shouldStartByRememberedLink: boolean | undefined,
): {
  readonly rememberedLinkKbps: number | undefined;
  readonly rememberedLinkSpread: number | undefined;
  readonly rememberedLinkSamples: number | undefined;
  readonly shouldStartByRememberedLink: boolean | undefined;
} {
  const { rememberedLinkKbps, rememberedLinkSpread, rememberedLinkSamples } = offer;
  return useMemo(
    () => ({
      rememberedLinkKbps,
      rememberedLinkSpread,
      rememberedLinkSamples,
      shouldStartByRememberedLink,
    }),
    [rememberedLinkKbps, rememberedLinkSpread, rememberedLinkSamples, shouldStartByRememberedLink],
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

/**
 * Plays the offer's chunked master playlist in the video (D247 §10), and says which quality plays
 * and, once its lowest quality stalls, that the link cannot keep up (D247 §8). Each playback starts
 * with the audio rendition startAudioRef holds as it starts, the playlist's default when none. Where
 * a media token is given and the page offers AirPlay, the film may be handed to AirPlay from the Cast
 * menu (D473): hls.js lets go of the element, and plays it again where AirPlay left it, paused if it
 * was (useAirPlayStage).
 */
export function useChunkedPlayback(
  videoRef: RefObject<HTMLVideoElement | null>,
  starting: {
    readonly offer: LadderOffer;
    readonly positionMs: number;
    readonly startAudioRef?: RefObject<number | undefined>;
    /** Whether it chooses its first quality from the link its device remembers (D381). */
    readonly shouldStartByRememberedLink?: boolean;
    /** The playback's media token, which AirPlay's grant is asked for by; none offers no AirPlay. */
    readonly mediaToken?: string | undefined;
    /** What plays it: the player's chunked playback, but in a test. */
    readonly play?: typeof playChunked;
  },
  onFailed: PlaybackFailed,
): {
  readonly playbackRef: RefObject<ChunkedPlayback | undefined>;
  readonly playing: PlayedLevel | undefined;
  readonly cannotKeepUp: CannotKeepUp | undefined;
} {
  const { startAudioRef, offer, play = playChunked } = starting;
  const { url, startAtBps, linkSharePercent, steadiness } = offer;
  const remembered = useRememberedLink(offer, starting.shouldStartByRememberedLink);
  const playbackRef = useRef<ChunkedPlayback | undefined>(undefined);
  const [playing, setPlaying] = useState<PlayedLevel | undefined>(undefined);
  const [cannotKeepUp, setCannotKeepUp] = useState<CannotKeepUp | undefined>(undefined);
  const { stage, letGoRef, startMs } = useAirPlayStage(videoRef, starting);
  useEffect(() => {
    const video = videoRef.current;
    if (video === null || stage.isOnAirPlay) {
      return undefined;
    }
    keepBack(video, stage.backAt);
    const playback = play(video, url, {
      onFailed,
      ...startAudioOf(startAudioRef),
      ...startOn({ startAtBps, linkSharePercent, steadiness, ...remembered }, startMs),
      monotonicMs,
    });
    const following = { playbackRef, letGoRef, setPlaying, setCannotKeepUp };
    letGoRef.current = holdPlayback(video, playback, following);
    return () => {
      letGoRef.current?.();
    };
  }, [
    videoRef,
    url,
    startAtBps,
    linkSharePercent,
    steadiness,
    remembered,
    startMs,
    onFailed,
    startAudioRef,
    stage,
    letGoRef,
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
    readonly letGoRef: RefObject<(() => void) | undefined>;
  },
): () => void {
  const { playbackRef, letGoRef } = following;
  playbackRef.current = playback;
  const stopFollowing = followChunkedPlayback(video, playback, following);
  return () => {
    letGoRef.current = undefined;
    stopFollowing();
    playback.destroy();
    playbackRef.current = undefined;
  };
}

/**
 * Where a chunked playback stands as to AirPlay: on it, its element let go of by hls.js; or not, and,
 * once it was, where AirPlay left the film (backAt), which it starts from again, paused if it was.
 */
interface AirPlayStage {
  readonly isOnAirPlay: boolean;
  readonly backAt?: { readonly positionMs: number; readonly isPaused: boolean };
}

/**
 * A chunked playback's AirPlay stage, and the letting go of its hls.js playback as AirPlay takes the
 * element (letGoRef, which the playback sets as it starts), AirPlay offered for its media token's playback
 * (useOffersAirPlay) with its hand-off.
 */
function useAirPlayStage(
  videoRef: RefObject<HTMLVideoElement | null>,
  starting: { readonly positionMs: number; readonly mediaToken?: string | undefined },
): {
  readonly stage: AirPlayStage;
  readonly letGoRef: RefObject<(() => void) | undefined>;
  /** Where the playback starts: where AirPlay left it, once it did, else where it was to start. */
  readonly startMs: number;
} {
  const [stage, setStage] = useState<AirPlayStage>({ isOnAirPlay: false });
  const letGoRef = useRef<(() => void) | undefined>(undefined);
  const handOff = useMemo(
    (): AirPlayHandOff => ({
      letGoOfHls: () => {
        letGoRef.current?.(); // within the press: the picker is shown in its gesture
        setStage({ isOnAirPlay: true });
      },
      takeBack: (left) => {
        const positionMs = Math.round(left.positionS * 1000);
        setStage({ isOnAirPlay: false, backAt: { positionMs, isPaused: left.wasPaused } });
      },
    }),
    [],
  );
  useOffersAirPlay(videoRef, starting.mediaToken, handOff);
  return { stage, letGoRef, startMs: stage.backAt?.positionMs ?? starting.positionMs };
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
function keepBack(video: HTMLVideoElement, backAt: AirPlayStage["backAt"]): void {
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
