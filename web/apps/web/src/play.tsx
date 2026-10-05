// SPDX-License-Identifier: MPL-2.0

import "./play.css";

import {
  mediaErrorFailure,
  playHls,
  seekTo,
  type HlsPlayback,
  type PlaybackFailure,
} from "@orpheum/player";
import { whenReady, type Api, type components, type PlayableFormats } from "@orpheum/sdk";
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
import { usePlayState } from "./play-state.ts";
import { useServices } from "./services.tsx";
import { BoostSwitch, SoundLine, SoundQuestion } from "./sound.tsx";
import { FindSubtitles } from "./subtitle-search.tsx";
import { Subtitles } from "./subtitles.tsx";
import { camelKey } from "./catalogue-key.ts";
import { LevelSwitch, soundLevelOf, type SoundLevel } from "./levelling.tsx";
import { Loaded } from "./title.tsx";
import { HostPlaysTogether } from "./together-host.tsx";
import { useResource } from "./use-resource.ts";

/** The server's decision on how a file plays here (cut-a §6). */
export type PlaybackDecision = components["schemas"]["StartPlaybackOutputBody"];

/**
 * Asks the server how a file plays in this browser. The link is local until M4 gives the
 * server an outside address.
 */
export async function requestPlayback(
  api: Api,
  fileId: number,
  playableFormats: PlayableFormats,
): Promise<PlaybackDecision> {
  return dataOrThrow(
    await api.POST("/playback", {
      body: { fileId, playableFormats, network: "local" },
    }),
  );
}

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
  if (props.decision.delivery === "direct") {
    return <DirectVideo {...props} />;
  }
  return <PreparedVideo {...props} />;
}

/**
 * What each way of playing is given: the file, the decision, and what follows a sound change or a
 * fetched subtitle.
 */
type PlayProps = Parameters<typeof PlayView>[0];

/**
 * What sits under a playing video: the host's Watch Together session while one is on, "Start
 * over", the subtitle menu and offset for the decision's
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
  },
): JSX.Element {
  const { t } = useTranslation();
  const { api } = useServices();
  const { refusal: reportRefusal, stopMessage } = usePlayState(api, props.videoRef, props.decision);
  return (
    <>
      {stopMessage === undefined ? null : (
        <p role="alert">{t("play.stoppedByAdmin", { message: stopMessage })}</p>
      )}
      {reportRefusal === undefined || stopMessage !== undefined ? null : (
        <p role="alert">{t("play.positionNotKept", { reason: problemText(t, reportRefusal) })}</p>
      )}
      <HostPlaysTogether
        fileId={props.fileId}
        videoRef={props.videoRef}
        startPositionMs={props.decision.resumePositionMs}
      />
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
 * starting with the audio rendition startAudioRenditionIndex when one is given.
 */
export function usePreparedPlayback(
  fileId: number,
  decision: PlaybackDecision,
  startAudioRenditionIndex: number | undefined,
): {
  videoRef: RefObject<HTMLVideoElement | null>;
  playbackRef: RefObject<HlsPlayback | undefined>;
  isWaiting: boolean;
  failure: string | undefined;
} {
  const { t } = useTranslation();
  const videoRef = useRef<HTMLVideoElement>(null);
  const playbackRef = useRef<HlsPlayback | undefined>(undefined);
  const [isWaiting, setIsWaiting] = useState(decision.isPreparing);
  const [failure, setFailure] = usePlaybackFailure(fileId);
  useEffect(() => {
    const element = videoRef.current;
    if (element === null) {
      return undefined;
    }
    const aborter = new AbortController();
    whenReady(decision.url, { signal: aborter.signal }).then(
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
  }, [decision.url, startAudioRenditionIndex, t, setFailure]);
  return { videoRef, playbackRef, isWaiting, failure };
}

/**
 * What the viewer is told when playback fails: what the player saw, at once, and, when the server
 * says the file cannot be read now, why instead: its machine away, or its folder or disk missing
 * (D241 §7). A file that can be read leaves the player's words.
 */
function usePlaybackFailure(fileId: number): [string | undefined, (seen: string) => void] {
  const { api } = useServices();
  const { t } = useTranslation();
  const [failure, setFailure] = useState<string | undefined>(undefined);
  const fail = useCallback(
    (seen: string) => {
      setFailure(seen);
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

/** The original file, playing; when the browser cannot play it, why, in words. */
function DirectVideo(props: PlayProps): JSX.Element {
  const { t } = useTranslation();
  const videoRef = useRef<HTMLVideoElement>(null);
  const [failure, setFailure] = usePlaybackFailure(props.fileId);
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
      />
      {failure === undefined ? (
        <p className="play__reason">{t("play.direct")}</p>
      ) : (
        <p role="alert">{t("play.directFailed", { reason: failure })}</p>
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
    requestPlayback(api, props.fileId, playableFormats()),
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
