// SPDX-License-Identifier: MPL-2.0

import "./film-player.css";

import {
  keepScreenAwake,
  mediaErrorFailure,
  meterBufferGrowth,
  playChunked,
  playHls,
  watchStalls,
  type ChunkedPlayback,
  type HlsPlayback,
  type LinkMeter,
  type PlaybackFailure,
  type PlayedLevel,
} from "@orpheum/player";
import {
  healPageAudio,
  whenReady,
  type Api,
  type CannotKeepUp,
  type components,
} from "@orpheum/sdk";
import type { TFunction } from "i18next";
import {
  useCallback,
  useEffect,
  useRef,
  useState,
  type ComponentType,
  type JSX,
  type RefObject,
} from "react";
import { useTranslation } from "react-i18next";

import { mixIndices, startingAudioTrack } from "./audio-tracks.tsx";
import { camelKey } from "./catalogue-key.ts";
import type { LadderOffer } from "./moving-down.ts";
import { dataOrThrow, problemText, unavailableText } from "./problem.ts";
import { rateText } from "./rate-text.ts";
import { useServices } from "./services.tsx";
import { monotonicMs } from "./use-together.ts";
import type { PlaybackDecision } from "./watching.ts";

/**
 * What a page knows of the film playing, given to what it composes around the video: the video, the
 * link's speed as the playback measures it, and the way it plays: a prepared copy's playback, to
 * switch its sound; a chunked one's quality and where it started; neither for the original.
 */
export interface FilmParts {
  readonly videoRef: RefObject<HTMLVideoElement | null>;
  /** The link's speed as the playback measures it, undefined until it can say (D247 §1). */
  readonly readLinkKbps: () => number | undefined;
  /** What failed, in the viewer's words; undefined while nothing has. */
  readonly failure: string | undefined;
  readonly preparedRef?: RefObject<HlsPlayback | undefined> | undefined;
  readonly ladder?:
    | {
        readonly offer: LadderOffer;
        /** Where on the film it started, in ms. */
        readonly positionMs: number;
        readonly playing: PlayedLevel | undefined;
        /** Even its lowest quality's word that it cannot keep up (D247 §8). */
        readonly cannotKeepUp: CannotKeepUp | undefined;
      }
    | undefined;
}

/** What a page draws over or under the video: a component, given the film's parts as its props. */
export type FilmPiece = ComponentType<FilmParts>;

/**
 * How a page composes the player: whether the video shows its own controls and plays at once, which
 * file a failure is explained by asking the server of (undefined asks nothing, for a Watch Together
 * guest, who may not ask of the host's files), and what it draws over the video, inside its stage,
 * and under it. Each piece is a component of the page's own, stable across renders, which reads
 * what else it needs from the page's context.
 */
export interface FilmComposition {
  readonly hasControls: boolean;
  readonly isAutoPlay: boolean;
  readonly askAbout: number | undefined;
  readonly over?: FilmPiece;
  readonly under?: FilmPiece;
}

/** Where a stalled player moved to a lower quality (D247 §2), for its playback alone. */
export interface MovedDown {
  readonly mediaToken: string;
  readonly offer: LadderOffer;
  readonly positionMs: number;
}

/**
 * The film, playing as the server decided, the one player every page composes (the owner,
 * 2026-10-11: "The play view and controls should ideally be reusable and composable"): on the
 * ladder a stalled player moved to (D247 §2), on its film's ladder from ladderStartMs (D260), the
 * original, or a prepared copy. Each owns the video and its stage, and says why it cannot play.
 */
export function FilmPlayer(props: {
  readonly decision: PlaybackDecision;
  readonly moved: MovedDown | undefined;
  /** Where a film on its ladder starts, in ms. */
  readonly ladderStartMs: number;
  readonly composition: FilmComposition;
}): JSX.Element {
  const { decision, moved, composition } = props;
  const onLadder = decision.onLadder ?? undefined;
  if (moved?.mediaToken === decision.mediaToken) {
    return (
      <ChunkedFilm offer={moved.offer} positionMs={moved.positionMs} composition={composition} />
    );
  }
  if (onLadder !== undefined) {
    return (
      <ChunkedFilm
        offer={onLadder}
        positionMs={props.ladderStartMs}
        composition={composition}
        isOnLadder
      />
    );
  }
  if (decision.delivery === "direct") {
    return <DirectFilm decision={decision} composition={composition} />;
  }
  return <PreparedFilm decision={decision} composition={composition} />;
}

/** The black box a film plays in, before any does: the stage's size, with no video in it. */
export function EmptyStage(): JSX.Element {
  return <div className="film-player__stage film-player__stage--empty" />;
}

/**
 * The stage and its video, the page's drawing over it inside the stage, where JASSUB places its
 * canvas too, then the page's drawing under it.
 */
function Stage(
  props: FilmParts &
    Pick<FilmComposition, "hasControls" | "isAutoPlay" | "over" | "under"> & {
      readonly src?: string;
      readonly onError?: (error: MediaError | null) => void;
    },
): JSX.Element {
  const { over: Over, under: Under, videoRef, readLinkKbps, failure, preparedRef, ladder } = props;
  useEffect(() => {
    const video = videoRef.current;
    // Absent where the page is not served over HTTPS, or the browser has no Screen Wake Lock.
    const locks = "wakeLock" in navigator ? navigator.wakeLock : undefined;
    return video === null ? undefined : keepScreenAwake(video, document, locks);
  }, [videoRef]);
  useHealedOnEmptied(videoRef);
  return (
    <>
      <div className="film-player__stage">
        {/* eslint-disable-next-line jsx-a11y-x/media-has-caption -- JASSUB draws the subtitles over the video, not a <track> (cut-a §6) */}
        <video
          className="film-player__video"
          ref={videoRef}
          src={props.src}
          controls={props.hasControls}
          autoPlay={props.isAutoPlay}
          playsInline
          onError={(event) => props.onError?.(event.currentTarget.error)}
        />
        {Over === undefined ? null : (
          <Over
            videoRef={videoRef}
            readLinkKbps={readLinkKbps}
            failure={failure}
            preparedRef={preparedRef}
            ladder={ladder}
          />
        )}
      </div>
      {Under === undefined ? null : (
        <Under
          videoRef={videoRef}
          readLinkKbps={readLinkKbps}
          failure={failure}
          preparedRef={preparedRef}
          ladder={ladder}
        />
      )}
    </>
  );
}

/**
 * Heals the page's audio session each time the video's media is torn down (emptied), as when its
 * player moves to another quality or rebuilds its decoding: where an iPhone's session became stuck in
 * the silent switch's category (WebKit bug 323104, page-audio.ts).
 */
function useHealedOnEmptied(videoRef: RefObject<HTMLVideoElement | null>): void {
  useEffect(() => {
    const video = videoRef.current;
    if (video === null) {
      return undefined;
    }
    video.addEventListener("emptied", healPageAudio);
    return () => {
      video.removeEventListener("emptied", healPageAudio);
    };
  }, [videoRef]);
}

/** The original file, playing; when the browser cannot play it, why, in words. */
function DirectFilm(props: {
  readonly decision: PlaybackDecision;
  readonly composition: FilmComposition;
}): JSX.Element {
  const { t } = useTranslation();
  const videoRef = useRef<HTMLVideoElement>(null);
  const [failure, setFailure] = usePlaybackFailure(props.composition.askAbout);
  const readLinkKbps = useBufferMeter(videoRef, props.decision.fileBitRateKbps);
  return (
    <>
      <Stage
        {...props.composition}
        videoRef={videoRef}
        readLinkKbps={readLinkKbps}
        failure={failure}
        src={props.decision.url}
        onError={(error) => {
          setFailure(playbackFailureText(t, mediaErrorFailure(error)));
        }}
      />
      {failure === undefined ? null : (
        <p role="alert">{t("play.directFailed", { reason: failure })}</p>
      )}
    </>
  );
}

/**
 * A prepared copy, playing: it waits while the copy's first part is made, then plays it, starting
 * with the audio track the playback starts with, boosted as the person's preference says (D217 §3).
 */
function PreparedFilm(props: {
  readonly decision: PlaybackDecision;
  readonly composition: FilmComposition;
}): JSX.Element {
  const { t } = useTranslation();
  const { decision } = props;
  const [startIndex] = useState(() =>
    audioRenditionIndex(
      decision,
      startingAudioTrack(decision)?.audioTrackIndex ?? 0,
      decision.preferences.isDialogueBoostOn,
    ),
  );
  const { videoRef, playbackRef, isWaiting, failure } = usePreparedPlayback(
    props.composition.askAbout,
    decision,
    startIndex,
  );
  return (
    <>
      <Stage
        {...props.composition}
        videoRef={videoRef}
        readLinkKbps={() => playbackRef.current?.estimateKbps()}
        failure={failure}
        preparedRef={playbackRef}
      />
      {isWaiting && failure === undefined ? <p>{t("play.preparing")}</p> : null}
      {failure === undefined ? null : (
        <p role="alert">{t("play.preparedFailed", { reason: failure })}</p>
      )}
    </>
  );
}

/**
 * The film on its ladder, every quality of it cut into chunks, from the one offered, stepping down
 * by itself and back up with care: from where it starts (D260), or where a stalled player moved to
 * it (D247 §2, §10). It says which quality plays, and when its lowest cannot keep up (D247 §8).
 */
function ChunkedFilm(props: {
  readonly offer: LadderOffer;
  readonly positionMs: number;
  readonly isOnLadder?: boolean;
  readonly composition: FilmComposition;
}): JSX.Element {
  const { t } = useTranslation();
  const { offer, positionMs } = props;
  const videoRef = useRef<HTMLVideoElement>(null);
  const [failure, setFailure] = usePlaybackFailure(props.composition.askAbout);
  const onFailed = useCallback(
    (seen: PlaybackFailure) => {
      setFailure(playbackFailureText(t, seen));
    },
    [setFailure, t],
  );
  const { playbackRef, playing, cannotKeepUp } = useChunkedPlayback(
    videoRef,
    { offer, positionMs },
    onFailed,
  );
  return (
    <>
      <Stage
        {...props.composition}
        videoRef={videoRef}
        readLinkKbps={() => playbackRef.current?.estimateKbps()}
        failure={failure}
        ladder={{ offer, positionMs, playing, cannotKeepUp }}
      />
      <LadderNotes
        offer={offer}
        playing={playing}
        cannotKeepUp={cannotKeepUp}
        isOnLadder={props.isOnLadder === true}
      />
      {failure === undefined ? null : (
        <p role="alert">{t("play.preparedFailed", { reason: failure })}</p>
      )}
    </>
  );
}

/**
 * The audio rendition that plays a track of a prepared copy: its boosted mix when the boost is on
 * and it has one, else its standard mix; undefined when the copy has no such track.
 */
export function audioRenditionIndex(
  decision: Pick<PlaybackDecision, "audioRenditions">,
  audioTrackIndex: number,
  isBoostOn: boolean,
): number | undefined {
  const mixes = mixIndices(decision, audioTrackIndex);
  return isBoostOn && mixes?.boosted !== undefined ? mixes.boosted : mixes?.standard;
}

/**
 * A prepared copy's playback: it waits while the copy's first part is made, then plays it,
 * starting with the audio rendition startAudioRenditionIndex when one is given. A failure asks the
 * server why the file askedAbout cannot be read; undefined asks nothing, for a Watch Together
 * viewer, who may not ask of the host's files and is told the player's own words.
 */
export function usePreparedPlayback(
  askedAbout: number | undefined,
  decision: PlaybackDecision,
  startAudioRenditionIndex: number | undefined,
): {
  videoRef: RefObject<HTMLVideoElement | null>;
  playbackRef: RefObject<HlsPlayback | undefined>;
  isWaiting: boolean;
  failure: string | undefined;
} {
  const { t } = useTranslation();
  const { fetchMedia } = useServices();
  const videoRef = useRef<HTMLVideoElement>(null);
  const playbackRef = useRef<HlsPlayback | undefined>(undefined);
  const [isWaiting, setIsWaiting] = useState(decision.isPreparing);
  const [failure, setFailure] = usePlaybackFailure(askedAbout);
  useEffect(() => {
    const element = videoRef.current;
    if (element === null) {
      return undefined;
    }
    const aborter = new AbortController();
    whenReady(decision.url, { fetch: fetchMedia, signal: aborter.signal }).then(
      () => {
        if (!aborter.signal.aborted) {
          setIsWaiting(false);
          playbackRef.current = playHls(element, decision.url, {
            onFailed: (failure) => {
              setFailure(playbackFailureText(t, failure));
            },
            ...(startAudioRenditionIndex === undefined ? {} : { startAudioRenditionIndex }),
          });
        }
      },
      (cause: unknown) => {
        if (!aborter.signal.aborted) {
          setFailure(problemText(t, cause));
        }
      },
    );
    return () => {
      aborter.abort();
      playbackRef.current?.destroy();
      playbackRef.current = undefined;
    };
  }, [decision.url, startAudioRenditionIndex, t, setFailure, fetchMedia]);
  return { videoRef, playbackRef, isWaiting, failure };
}

/**
 * What the viewer is told when playback fails: what the player saw, at once, and, when the server
 * says the file cannot be read now, why instead: its machine away, or its folder or disk missing
 * (D241 §7). A file that can be read leaves the player's words.
 */
function usePlaybackFailure(
  fileId: number | undefined,
): [string | undefined, (seen: string) => void] {
  const { api } = useServices();
  const { t } = useTranslation();
  const [failure, setFailure] = useState<string | undefined>(undefined);
  const fail = useCallback(
    (seen: string) => {
      setFailure(seen);
      if (fileId === undefined) {
        return;
      }
      fetchUnavailability(api, fileId).then(
        (unavailable) => {
          if (unavailable !== undefined) {
            setFailure(unavailableText(t, unavailable));
          }
        },
        () => undefined, // the player's own words stand
      );
    },
    [api, fileId, t],
  );
  return [failure, fail];
}

/** Why a file cannot be read now, as the server says, or undefined when it can. */
async function fetchUnavailability(
  api: Api,
  fileId: number,
): Promise<components["schemas"]["UnavailableBody"] | undefined> {
  const answer = dataOrThrow(
    await api.GET("/files/{id}/availability", { params: { path: { id: fileId } } }),
  );
  return answer.unavailable;
}

/**
 * Measures the link of a video playing a file of fileBitRateKbps as it is, from how fast its buffer
 * grows (D247 §1); none for a file whose bit rate is not known.
 */
export function useBufferMeter(
  videoRef: RefObject<HTMLVideoElement | null>,
  fileBitRateKbps: number,
): () => number | undefined {
  const meterRef = useRef<LinkMeter | undefined>(undefined);
  useEffect(() => {
    const video = videoRef.current;
    if (video === null || fileBitRateKbps <= 0) {
      return undefined;
    }
    const meter = meterBufferGrowth(video, { fileBitRateKbps, monotonicMs });
    meterRef.current = meter;
    return () => {
      meter.stop();
      meterRef.current = undefined;
    };
  }, [videoRef, fileBitRateKbps]);
  return () => meterRef.current?.estimateKbps();
}

/**
 * Plays the offer's chunked master playlist in the video (D247 §10), and says which quality plays
 * and, once its lowest quality stalls, that the link cannot keep up (D247 §8).
 */
export function useChunkedPlayback(
  videoRef: RefObject<HTMLVideoElement | null>,
  starting: { readonly offer: LadderOffer; readonly positionMs: number },
  onFailed: (failure: PlaybackFailure) => void,
): {
  readonly playbackRef: RefObject<ChunkedPlayback | undefined>;
  readonly playing: PlayedLevel | undefined;
  readonly cannotKeepUp: CannotKeepUp | undefined;
} {
  const { offer, positionMs } = starting;
  const playbackRef = useRef<ChunkedPlayback | undefined>(undefined);
  const [playing, setPlaying] = useState<PlayedLevel | undefined>(undefined);
  const [cannotKeepUp, setCannotKeepUp] = useState<CannotKeepUp | undefined>(undefined);
  useEffect(() => {
    const video = videoRef.current;
    if (video === null) {
      return undefined;
    }
    const playback = playChunked(video, offer.url, {
      onFailed,
      startAtBps: offer.startAtBps,
      startPositionS: positionMs / 1000,
      monotonicMs,
    });
    playbackRef.current = playback;
    const looking = setInterval(() => {
      setPlaying(playback.playing());
    }, 1000);
    const stopWatching = watchStalls(video, () => {
      const beyond = playback.beyondTheLink();
      if (beyond !== undefined) {
        setCannotKeepUp(beyond);
      }
    });
    return () => {
      clearInterval(looking);
      stopWatching();
      playback.destroy();
      playbackRef.current = undefined;
    };
  }, [videoRef, offer.url, offer.startAtBps, positionMs, onFailed]);
  return { playbackRef, playing, cannotKeepUp };
}

/** What a chunked playback says: the quality playing, and when even the lowest cannot keep up. */
export function LadderNotes(props: {
  readonly offer: LadderOffer;
  readonly playing: PlayedLevel | undefined;
  readonly cannotKeepUp: CannotKeepUp | undefined;
  /** Whether the playback started on its film's ladder (D260), rather than moved to it after a stall (D253). */
  readonly isOnLadder?: boolean;
}): JSX.Element {
  const { t } = useTranslation();
  const { offer, playing, cannotKeepUp } = props;
  const isOnLadder = props.isOnLadder === true;
  const startingKey = isOnLadder ? "play.ladderStarting" : "play.lowerQualityStarting";
  const playingKey = isOnLadder ? "play.ladderPlaying" : "play.lowerQuality";
  const isBeyondTheLink = cannotKeepUp !== undefined || offer.isBelowTheLowest;
  const lowest = rateText(cannotKeepUp?.lowestKbps ?? offer.startAtBps / 1000);
  return (
    <>
      <p className="play__reason">
        {playing === undefined
          ? t(startingKey, {
              height: offer.height,
              rate: rateText(offer.qualityKbps),
            })
          : t(playingKey, {
              height: playing.height,
              rate: rateText(playing.averageBitrate / 1000),
            })}
      </p>
      {isBeyondTheLink ? (
        <p role="alert">
          {cannotKeepUp?.linkKbps === undefined
            ? t("play.cannotKeepUpUnmeasured", { lowest })
            : t("play.cannotKeepUp", { link: rateText(cannotKeepUp.linkKbps), lowest })}
        </p>
      ) : null}
    </>
  );
}

/** Why a video could not play, in the viewer's words, by the player's code for it. */
export function playbackFailureText(t: TFunction, failure: PlaybackFailure): string {
  return t(`play.failure.${camelKey(failure)}`);
}
