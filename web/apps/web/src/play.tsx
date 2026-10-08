// SPDX-License-Identifier: MPL-2.0

import "./play.css";

import {
  mediaErrorFailure,
  meterBufferGrowth,
  playChunked,
  playHls,
  seekTo,
  watchStalls,
  type ChunkedPlayback,
  type HlsPlayback,
  type LinkMeter,
  type PlaybackFailure,
  type PlayedLevel,
} from "@orpheum/player";
import { whenReady, type Api, type CannotKeepUp, type components } from "@orpheum/sdk";
import {
  useCallback,
  useEffect,
  useRef,
  useState,
  type JSX,
  type RefObject,
  type SyntheticEvent,
} from "react";
import type { TFunction } from "i18next";
import { useTranslation } from "react-i18next";

import { AudioMenu, mixIndices, startingAudioTrack, type AudioTrack } from "./audio-tracks.tsx";
import { dataOrThrow, problemText, unavailableText } from "./problem.ts";
import type { LadderOffer } from "./moving-down.ts";
import { usePlayState } from "./play-state.ts";
import { useServices } from "./services.tsx";
import { BoostSwitch, SoundLine, SoundQuestion } from "./sound.tsx";
import { FindSubtitles } from "./subtitle-search.tsx";
import { Subtitles } from "./subtitles.tsx";
import { camelKey } from "./catalogue-key.ts";
import { LevelSwitch, soundLevelOf, type SoundLevel } from "./levelling.tsx";
import { Loaded } from "./title.tsx";
import { HostPlaysTogether } from "./together-host.tsx";
import { rateText } from "./rate-text.ts";
import { useResource } from "./use-resource.ts";
import { monotonicMs } from "./use-together.ts";
import { watchingOf, type PlaybackDecision } from "./watching.ts";

/**
 * A decision, shown: the original playing, or a prepared copy and why. The reason is always on
 * screen (D188). Another audio track chosen in the original asks for the decision again, which
 * prepares a copy, since a browser plays the original's first (D230 §5).
 */
export function PlayView(props: {
  readonly fileId: number;
  readonly decision: PlaybackDecision;
  /** Asks the server again how the file plays: after the sound changed (D222) or a subtitle was fetched. */
  readonly onDecideAgain: () => void;
}): JSX.Element {
  // The lower quality a stalled player moved to, and where it was (D247 §2); per playback.
  const [moved, setMoved] = useState<
    | { readonly mediaToken: string; readonly offer: LadderOffer; readonly positionMs: number }
    | undefined
  >(undefined);
  // The video playing now, which the host's Watch Together session follows: kept here, above the
  // way of playing, so a move to a lower quality is no film stopped and put on again for everyone
  // (the owner's test, 2026-10-05).
  const [video, setVideo] = useState<HTMLVideoElement | null>(null);
  // Whether its lowest quality cannot keep up, which a host's session tells everyone (D247 §8).
  const [cannotKeepUp, setCannotKeepUp] = useState<CannotKeepUp | undefined>(undefined);
  const { mediaToken } = props.decision;
  const onLowerQuality = useCallback(
    (offer: LadderOffer, positionMs: number) => {
      setMoved((already) => already ?? { mediaToken, offer, positionMs });
    },
    [mediaToken],
  );
  const ways = { ...props, onLowerQuality, onVideo: setVideo, onCannotKeepUp: setCannotKeepUp };
  let playing: JSX.Element;
  const onLadder = props.decision.onLadder ?? undefined;
  if (moved?.mediaToken === mediaToken) {
    playing = <ChunkedVideo {...ways} offer={moved.offer} positionMs={moved.positionMs} />;
  } else if (onLadder !== undefined) {
    // A film cut into chunks plays from its ladder from the start (D260).
    playing = (
      <ChunkedVideo
        {...ways}
        offer={onLadder}
        positionMs={props.decision.resumePositionMs}
        isOnLadder
      />
    );
  } else if (props.decision.delivery === "direct") {
    playing = <DirectVideo {...ways} />;
  } else {
    playing = <PreparedVideo {...ways} />;
  }
  const { titleName } = props.decision;
  return (
    <>
      {titleName === undefined ? null : <h1 className="play__title">{titleName}</h1>}
      {playing}
      <HostPlaysTogether
        fileId={props.fileId}
        video={video}
        startPositionMs={props.decision.resumePositionMs}
        cannotKeepUp={cannotKeepUp}
        audioOutput={props.decision.audioOutput}
      />
    </>
  );
}

/**
 * What each way of playing is given: the file, the decision, what follows a sound change or a
 * fetched subtitle, and where a lower quality a stalled player is offered goes (D247 §2).
 */
type PlayProps = Parameters<typeof PlayView>[0] & {
  readonly onLowerQuality: (offer: LadderOffer, positionMs: number) => void;
  /** Told the video element each way of playing shows, and null as it goes. */
  readonly onVideo: (video: HTMLVideoElement | null) => void;
  /** Told when even the lowest quality cannot keep up, and undefined when it can (D247 §8). */
  readonly onCannotKeepUp: (cannotKeepUp: CannotKeepUp | undefined) => void;
};

/**
 * Where a player moving to a lower quality goes on from: where its video is, once it has loaded
 * enough to say; before that, where it was asked to start. A video not loaded reads 0, and a host
 * resuming Bram at 51:22 was moved to its first minute, whose chunks no one had made (the owner,
 * 2026-10-08).
 */
export function positionToMoveFrom(
  video: Pick<HTMLVideoElement, "readyState" | "currentTime"> | null,
  startPositionMs: number,
): number {
  const haveMetadata = 1; // HTMLMediaElement.HAVE_METADATA, which a plain page may not define
  return video !== null && video.readyState >= haveMetadata
    ? video.currentTime * 1000
    : startPositionMs;
}

/**
 * Keeps the playback's play state on the server, and hands a lower quality the server offered a
 * stalled player on, with where the video is (D247 §2).
 */
function useReportedPlayState(
  props: PlayProps & {
    readonly videoRef: RefObject<HTMLVideoElement | null>;
    readonly readLinkKbps: () => number | undefined;
  },
): { readonly refusal: unknown; readonly stopMessage: string | undefined } {
  const { api } = useServices();
  const { onLowerQuality, videoRef } = props;
  const { refusal, stopMessage, lowerQuality } = usePlayState(api, videoRef, {
    mediaToken: props.decision.mediaToken,
    resumePositionMs: props.decision.resumePositionMs,
    readLinkKbps: props.readLinkKbps,
  });
  const startPositionMs = props.decision.resumePositionMs;
  useEffect(() => {
    if (lowerQuality !== undefined) {
      onLowerQuality(lowerQuality, positionToMoveFrom(videoRef.current, startPositionMs));
    }
  }, [lowerQuality, onLowerQuality, videoRef, startPositionMs]);
  return { refusal, stopMessage };
}

/**
 * What sits under a playing video: "Start over", the subtitle menu and offset for the decision's
 * tracks, finding subtitles, the audio track, the device's sound and levelling. It keeps the
 * playback's play state on the server, starting where the person left the file. A chosen audio
 * track is kept for the person, and onAudioChosen() plays it.
 */
function UnderTheVideo(
  props: PlayProps & {
    readonly videoRef: RefObject<HTMLVideoElement | null>;
    readonly onAudioChosen: (track: AudioTrack) => void;
    /** The sound playing now, which levelling levels. */
    readonly level: SoundLevel;
    /** The link's speed as the playback measures it, undefined until it can say (D247 §1). */
    readonly readLinkKbps: () => number | undefined;
  },
): JSX.Element {
  const { t } = useTranslation();
  const { refusal: reportRefusal, stopMessage } = useReportedPlayState(props);
  const { onVideo, videoRef } = props;
  useEffect(() => {
    onVideo(videoRef.current);
    return () => {
      onVideo(null);
    };
  }, [onVideo, videoRef]);
  return (
    <>
      {stopMessage === undefined ? null : (
        <p role="alert">{t("play.stoppedByAdmin", { message: stopMessage })}</p>
      )}
      {reportRefusal === undefined || stopMessage !== undefined ? null : (
        <p role="alert">{t("play.positionNotKept", { reason: problemText(t, reportRefusal) })}</p>
      )}
      <button
        type="button"
        className="play__start-over"
        onClick={() => {
          const video = props.videoRef.current;
          if (video !== null) {
            seekTo(video, 0);
          }
        }}
      >
        {t("play.startOver")}
      </button>
      <Subtitles
        fileId={props.fileId}
        tracks={props.decision.subtitles ?? []}
        preferences={props.decision.preferences}
        videoRef={props.videoRef}
      />
      <FindSubtitles fileId={props.fileId} onFetched={props.onDecideAgain} />
      <AudioMenu fileId={props.fileId} decision={props.decision} onChosen={props.onAudioChosen} />
      {props.decision.audioOutput === undefined ? null : (
        <SoundLine current={props.decision.audioOutput} onChanged={props.onDecideAgain} />
      )}
      <LevelSwitch
        level={props.level}
        isInitiallyOn={props.decision.preferences.isLevellingOn}
        videoRef={props.videoRef}
      />
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
 * A prepared copy, playing: it waits while the copy's first part is made, then plays it, starting
 * with the audio track the playback starts with, with a switch for the dialogue boost where that
 * track has a centre to raise (D217 §3). Another track chosen plays at once (D23).
 */
function PreparedVideo(props: PlayProps): JSX.Element {
  const { t } = useTranslation();
  const { decision } = props;
  const [audioTrackIndex, setAudioTrackIndex] = useState(
    () => startingAudioTrack(decision)?.audioTrackIndex ?? 0,
  );
  const [isBoostOn, setIsBoostOn] = useState(decision.preferences.isDialogueBoostOn);
  const [startIndex] = useState(() =>
    audioRenditionIndex(decision, audioTrackIndex, decision.preferences.isDialogueBoostOn),
  );
  const { videoRef, playbackRef, isWaiting, failure } = usePreparedPlayback(
    props.fileId,
    decision,
    startIndex,
  );
  const boostMixes = mixIndices(decision, audioTrackIndex);
  const play = (trackIndex: number, isBoosted: boolean): void => {
    const index = audioRenditionIndex(decision, trackIndex, isBoosted);
    if (index !== undefined) {
      playbackRef.current?.setAudioRendition(index);
    }
  };
  const reasons = (decision.notDirectReasons ?? [])
    .map((reason) => t(`play.reason.${camelKey(reason)}`))
    .join(", ");
  return (
    <div className="play">
      <div className="play__stage">
        {/* eslint-disable-next-line jsx-a11y-x/media-has-caption -- JASSUB draws the subtitles over the video, not a <track> (cut-a §6) */}
        <video className="play__video" ref={videoRef} controls autoPlay playsInline />
      </div>
      <UnderTheVideo
        {...props}
        videoRef={videoRef}
        level={soundLevelOf(decision, audioTrackIndex)}
        readLinkKbps={() => playbackRef.current?.estimateKbps()}
        onAudioChosen={(track) => {
          setAudioTrackIndex(track.audioTrackIndex);
          play(track.audioTrackIndex, isBoostOn);
        }}
      />
      <p className="play__reason">{t("play.prepared", { reasons })}</p>
      {isWaiting && failure === undefined ? <p>{t("play.preparing")}</p> : null}
      {failure === undefined ? null : (
        <p role="alert">{t("play.preparedFailed", { reason: failure })}</p>
      )}
      {boostMixes?.boosted === undefined ? null : (
        <BoostSwitch
          isInitiallyOn={isBoostOn}
          onSwitched={(isOn) => {
            setIsBoostOn(isOn);
            play(audioTrackIndex, isOn);
          }}
        />
      )}
    </div>
  );
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

/** The original file, playing; when the browser cannot play it, why, in words. */
function DirectVideo(props: PlayProps): JSX.Element {
  const { t } = useTranslation();
  const videoRef = useRef<HTMLVideoElement>(null);
  const [failure, setFailure] = usePlaybackFailure(props.fileId);
  const readLinkKbps = useBufferMeter(videoRef, props.decision.fileBitRateKbps);
  const showMediaError = (event: SyntheticEvent<HTMLVideoElement>): void => {
    setFailure(playbackFailureText(t, mediaErrorFailure(event.currentTarget.error)));
  };
  return (
    <div className="play">
      <div className="play__stage">
        {/* eslint-disable-next-line jsx-a11y-x/media-has-caption -- JASSUB draws the subtitles over the video, not a <track> (cut-a §6) */}
        <video
          className="play__video"
          ref={videoRef}
          src={props.decision.url}
          controls
          autoPlay
          playsInline
          onError={showMediaError}
        />
      </div>
      <UnderTheVideo
        {...props}
        videoRef={videoRef}
        level={soundLevelOf(props.decision, undefined)}
        onAudioChosen={props.onDecideAgain}
        readLinkKbps={readLinkKbps}
      />
      {failure === undefined ? (
        <p className="play__reason">{t("play.direct")}</p>
      ) : (
        <p role="alert">{t("play.directFailed", { reason: failure })}</p>
      )}
    </div>
  );
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

/**
 * A chunked playback, on its film's ladder: every quality of the film cut into chunks, starting at
 * the one offered, stepping down by itself and back up with care, from where the playback resumes
 * (D260) or where a stalled player moved to it (D247 §2, §10). Its sound carries its levelling, as a
 * prepared copy's does.
 */
function ChunkedVideo(
  props: PlayProps & {
    readonly offer: LadderOffer;
    readonly positionMs: number;
    readonly isOnLadder?: boolean;
  },
): JSX.Element {
  const { t } = useTranslation();
  const { offer } = props;
  const videoRef = useRef<HTMLVideoElement>(null);
  const [failure, setFailure] = usePlaybackFailure(props.fileId);
  const onFailed = useCallback(
    (seen: PlaybackFailure) => {
      setFailure(playbackFailureText(t, seen));
    },
    [setFailure, t],
  );
  const { playbackRef, playing, cannotKeepUp } = useChunkedPlayback(
    videoRef,
    { offer, positionMs: props.positionMs },
    onFailed,
  );
  const { onCannotKeepUp } = props;
  useEffect(() => {
    onCannotKeepUp(cannotKeepUp);
    return () => {
      onCannotKeepUp(undefined);
    };
  }, [cannotKeepUp, onCannotKeepUp]);
  // It starts where the player stalled, not where the person last left the file.
  const decision = { ...props.decision, resumePositionMs: Math.round(props.positionMs) };
  const level: SoundLevel = {
    levelling: { isMeasured: offer.soundCarriedGainDb !== 0, gainDb: offer.soundCarriedGainDb },
    carriedGainDb: offer.soundCarriedGainDb,
  };
  return (
    <div className="play">
      <div className="play__stage">
        {/* eslint-disable-next-line jsx-a11y-x/media-has-caption -- JASSUB draws the subtitles over the video, not a <track> (cut-a §6) */}
        <video className="play__video" ref={videoRef} controls autoPlay playsInline />
      </div>
      <UnderTheVideo
        {...props}
        decision={decision}
        videoRef={videoRef}
        level={level}
        onAudioChosen={props.onDecideAgain}
        readLinkKbps={() => playbackRef.current?.estimateKbps()}
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
    </div>
  );
}

/** Why a video could not play, in the viewer's words, by the player's code for it. */
export function playbackFailureText(t: TFunction, failure: PlaybackFailure): string {
  return t(`play.failure.${camelKey(failure)}`);
}

/**
 * The player's screen for one file. A device not yet asked how it makes sound is asked first,
 * and the decision asked for again with the answer (D222).
 */
export function Play(props: { readonly fileId: number }): JSX.Element {
  const { api, playableFormats, session } = useServices();
  const [decision, decideAgain] = useResource(`play/${String(props.fileId)}`, () =>
    watchingOf(api).decide(props.fileId, playableFormats()),
  );
  return (
    <Loaded
      resource={decision}
      render={(value) =>
        value.audioOutput === undefined && session.deviceId() !== undefined ? (
          <div className="play">
            <SoundQuestion onAnswered={decideAgain} />
          </div>
        ) : (
          <PlayView fileId={props.fileId} decision={value} onDecideAgain={decideAgain} />
        )
      }
    />
  );
}
