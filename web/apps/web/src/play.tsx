// SPDX-License-Identifier: MPL-2.0

import "./play.css";

import { playHls, seekTo, type Playback } from "@orpheum/player";
import { whenReady, type Api, type components, type Menu } from "@orpheum/sdk";
import { useEffect, useRef, useState, type JSX, type RefObject, type SyntheticEvent } from "react";
import { useTranslation } from "react-i18next";

import { dataOrThrow, problemText } from "./problem.ts";
import { usePlayState } from "./play-state.ts";
import { useServices } from "./services.tsx";
import { BoostSwitch, SoundLine, SoundQuestion } from "./sound.tsx";
import { FindSubtitles } from "./subtitle-search.tsx";
import { Subtitles } from "./subtitles.tsx";
import { camelKey } from "./catalogue-key.ts";
import { LevelSwitch, soundLevel } from "./levelling.tsx";
import { Loaded } from "./title.tsx";
import { useResource } from "./use-resource.ts";

/** The server's decision on how a file plays here (cut-a §6). */
export type Decision = components["schemas"]["StartPlaybackOutputBody"];

/**
 * Asks the server how a file plays in this browser. The link is local until M4 gives the
 * server an outside address.
 */
export async function requestPlayback(api: Api, fileId: number, menu: Menu): Promise<Decision> {
  const data = dataOrThrow(
    await api.POST("/playback", {
      body: { fileId, menu, link: "local" },
    }),
  );
  return data;
}

/** A decision, shown: the original playing, or a prepared copy and why. The reason is always on screen (D188). */
export function PlayView(props: {
  readonly fileId: number;
  readonly decision: Decision;
  /** How the device makes sound changed: the decision is asked for again (D222). */
  /** Asks the server again how the file plays: after the sound changed, or a subtitle was fetched. */
  readonly decideAgain: () => void;
}): JSX.Element {
  if (props.decision.kind === "direct") {
    return <DirectVideo {...props} />;
  }
  return <PreparedVideo {...props} />;
}

/** What each way of playing is given: the file, the decision, and what follows a sound change. */
type PlayProps = Parameters<typeof PlayView>[0];

/**
 * What sits under a playing video: "Start over", the subtitle menu and offset for the decision's
 * tracks, finding subtitles, the device's sound and levelling. It keeps the playback's play state on the server, starting where the person
 * left the file.
 */
function UnderTheVideo(
  props: PlayProps & { readonly videoRef: RefObject<HTMLVideoElement | null> },
): JSX.Element {
  const { t } = useTranslation();
  const { api } = useServices();
  usePlayState(api, props.videoRef, props.decision);
  return (
    <>
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
        prefs={props.decision.prefs}
        videoRef={props.videoRef}
      />
      <FindSubtitles fileId={props.fileId} fetched={props.decideAgain} />
      {props.decision.audioOutput === undefined ? null : (
        <SoundLine current={props.decision.audioOutput} changed={props.decideAgain} />
      )}
      <LevelSwitch
        level={soundLevel(props.decision)}
        isOn={props.decision.prefs.isLevelling}
        videoRef={props.videoRef}
      />
    </>
  );
}

/** Where a track's standard and boosted mixes are among a rendition's audio, for its first track. */
export function boostPair(decision: Decision): { standard: number; boosted: number } | undefined {
  const audio = decision.audio ?? [];
  const first = audio[0];
  if (first === undefined) {
    return undefined;
  }
  const standard = audio.findIndex(
    (mix) => mix.sourceTrack === first.sourceTrack && !mix.isDialogueBoost,
  );
  const boosted = audio.findIndex(
    (mix) => mix.sourceTrack === first.sourceTrack && mix.isDialogueBoost,
  );
  return standard < 0 || boosted < 0 ? undefined : { standard, boosted };
}

/**
 * A prepared copy, playing: it waits while the copy's first part is made, then plays it, with a
 * switch for the dialogue boost where the sound has a centre to raise (D217 §3).
 */
function PreparedVideo(props: PlayProps): JSX.Element {
  const { t } = useTranslation();
  const { decision } = props;
  const pair = boostPair(decision);
  const { videoRef, playbackRef, isWaiting, failure } = usePrepared(
    decision,
    decision.prefs.isDialogueBoost ? pair?.boosted : undefined,
  );
  const reasons = (decision.reasons ?? [])
    .map((reason) => t(`play.reason.${camelKey(reason)}`))
    .join(", ");
  return (
    <div className="play">
      <div className="play__stage">
        {/* eslint-disable-next-line jsx-a11y-x/media-has-caption -- JASSUB draws the subtitles over the video, not a <track> (cut-a §6) */}
        <video className="play__video" ref={videoRef} controls autoPlay playsInline />
      </div>
      <UnderTheVideo {...props} videoRef={videoRef} />
      <p className="play__reason">{t("play.prepared", { reasons })}</p>
      {isWaiting && failure === undefined ? <p>{t("play.preparing")}</p> : null}
      {failure === undefined ? null : (
        <p role="alert">{t("play.preparedFailed", { reason: failure })}</p>
      )}
      {pair === undefined ? null : (
        <BoostSwitch
          isOn={decision.prefs.isDialogueBoost}
          switched={(isOn) => {
            playbackRef.current?.setAudioRendition(isOn ? pair.boosted : pair.standard);
          }}
        />
      )}
    </div>
  );
}

/**
 * A prepared copy's playback: it waits while the copy's first part is made, then plays it,
 * starting with the audio rendition startAudio when one is given.
 */
function usePrepared(
  decision: Decision,
  startAudio: number | undefined,
): {
  videoRef: RefObject<HTMLVideoElement | null>;
  playbackRef: RefObject<Playback | undefined>;
  isWaiting: boolean;
  failure: string | undefined;
} {
  const { t } = useTranslation();
  const videoRef = useRef<HTMLVideoElement>(null);
  const playbackRef = useRef<Playback | undefined>(undefined);
  const [isWaiting, setIsWaiting] = useState(decision.isPreparing);
  const [failure, setFailure] = useState<string | undefined>(undefined);
  useEffect(() => {
    const element = videoRef.current;
    if (element === null) {
      return undefined;
    }
    const abort = new AbortController();
    whenReady(decision.url, { signal: abort.signal }).then(
      () => {
        if (!abort.signal.aborted) {
          setIsWaiting(false);
          playbackRef.current = playHls(element, decision.url, {
            failed: setFailure,
            ...(startAudio === undefined ? {} : { startAudio }),
          });
        }
      },
      (cause: unknown) => {
        if (!abort.signal.aborted) {
          setFailure(problemText(t, cause));
        }
      },
    );
    return () => {
      abort.abort();
      playbackRef.current?.destroy();
      playbackRef.current = undefined;
    };
  }, [decision.url, startAudio, t]);
  return { videoRef, playbackRef, isWaiting, failure };
}

/** The original file, playing; when the browser cannot play it, why, in words. */
function DirectVideo(props: PlayProps): JSX.Element {
  const { t } = useTranslation();
  const videoRef = useRef<HTMLVideoElement>(null);
  const [failure, setFailure] = useState<string | undefined>(undefined);
  const failed = (event: SyntheticEvent<HTMLVideoElement>): void => {
    setFailure(mediaFailure(event.currentTarget.error));
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
          onError={failed}
        />
      </div>
      <UnderTheVideo {...props} videoRef={videoRef} />
      {failure === undefined ? (
        <p className="play__reason">{t("play.direct")}</p>
      ) : (
        <p role="alert">{t("play.failed", { reason: failure })}</p>
      )}
    </div>
  );
}

/** What a media element's error says, in the browser's words, or its code when it gives none. */
export function mediaFailure(error: Pick<MediaError, "code" | "message"> | null): string {
  if (error === null) {
    return "unknown";
  }
  return error.message === "" ? `MediaError ${String(error.code)}` : error.message;
}

/** The player's screen for one file. */
/**
 * The player's screen for one file. A device not yet asked how it makes sound is asked first,
 * and the decision asked for again with the answer (D222).
 */
export function Play(props: { readonly fileId: number }): JSX.Element {
  const { api, menu, session } = useServices();
  const [decision, reload] = useResource(`play/${String(props.fileId)}`, () =>
    requestPlayback(api, props.fileId, menu()),
  );
  return (
    <Loaded
      resource={decision}
      render={(value) =>
        value.audioOutput === undefined && session.deviceId() !== undefined ? (
          <div className="play">
            <SoundQuestion answered={reload} />
          </div>
        ) : (
          <PlayView fileId={props.fileId} decision={value} decideAgain={reload} />
        )
      }
    />
  );
}
