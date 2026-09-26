// SPDX-License-Identifier: MPL-2.0

import "./play.css";

import { playHls, type Playback } from "@orpheum/player";
import { whenReady, type Api, type components, type Menu } from "@orpheum/sdk";
import { useEffect, useRef, useState, type JSX, type RefObject, type SyntheticEvent } from "react";
import { useTranslation } from "react-i18next";

import { useServices } from "./services.tsx";
import { Subtitles } from "./subtitles.tsx";
import { camelKey, Loaded } from "./title.tsx";
import { useResource } from "./use-resource.ts";

/** The server's decision on how a file plays here (cut-a §6). */
export type Decision = components["schemas"]["StartPlaybackOutputBody"];

/**
 * Asks the server how a file plays in this browser. The link is local until M4 gives the
 * server an outside address.
 */
export async function requestPlayback(api: Api, fileId: number, menu: Menu): Promise<Decision> {
  const { data, error } = await api.POST("/playback", {
    body: { fileId, menu, link: "local" },
  });
  if (data === undefined) {
    throw new Error(error.error.message);
  }
  return data;
}

/** A decision, shown: the original playing, or a prepared copy and why. The reason is always on screen (D188). */
export function PlayView(props: {
  readonly fileId: number;
  readonly decision: Decision;
}): JSX.Element {
  const { decision } = props;
  if (decision.kind === "direct") {
    return <DirectVideo fileId={props.fileId} decision={decision} />;
  }
  return <PreparedVideo fileId={props.fileId} decision={decision} />;
}

/** The subtitle menu and offset under a playing video, for the decision's tracks. */
function SubtitlesOf(props: {
  readonly fileId: number;
  readonly decision: Decision;
  readonly videoRef: RefObject<HTMLVideoElement | null>;
}): JSX.Element {
  return (
    <Subtitles
      fileId={props.fileId}
      tracks={props.decision.subtitles ?? []}
      startingOffsetMs={props.decision.prefs.subtitleOffsetMs}
      videoRef={props.videoRef}
    />
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
function PreparedVideo(props: {
  readonly fileId: number;
  readonly decision: Decision;
}): JSX.Element {
  const { t } = useTranslation();
  const { decision } = props;
  const { videoRef, playbackRef, isWaiting, failure } = usePrepared(decision);
  const reasons = (decision.reasons ?? [])
    .map((reason) => t(`play.reason.${camelKey(reason)}`))
    .join(", ");
  const pair = boostPair(decision);
  return (
    <div className="play">
      <div className="play__stage">
        {/* eslint-disable-next-line jsx-a11y-x/media-has-caption -- JASSUB draws the subtitles over the video, not a <track> (cut-a §6) */}
        <video className="play__video" ref={videoRef} controls autoPlay playsInline />
      </div>
      <SubtitlesOf fileId={props.fileId} decision={decision} videoRef={videoRef} />
      <p className="play__reason">{t("play.prepared", { reasons })}</p>
      {isWaiting && failure === undefined ? <p>{t("play.preparing")}</p> : null}
      {failure === undefined ? null : (
        <p role="alert">{t("play.preparedFailed", { reason: failure })}</p>
      )}
      {pair === undefined ? null : (
        <label className="play__boost">
          <input
            type="checkbox"
            onChange={(event) => {
              playbackRef.current?.setAudioRendition(
                event.currentTarget.checked ? pair.boosted : pair.standard,
              );
            }}
          />
          {t("play.boost")}
        </label>
      )}
    </div>
  );
}

/** A prepared copy's playback: it waits while the copy's first part is made, then plays it. */
function usePrepared(decision: Decision): {
  videoRef: RefObject<HTMLVideoElement | null>;
  playbackRef: RefObject<Playback | undefined>;
  isWaiting: boolean;
  failure: string | undefined;
} {
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
          playbackRef.current = playHls(element, decision.url, setFailure);
        }
      },
      (cause: unknown) => {
        if (!abort.signal.aborted) {
          setFailure(cause instanceof Error ? cause.message : String(cause));
        }
      },
    );
    return () => {
      abort.abort();
      playbackRef.current?.destroy();
      playbackRef.current = undefined;
    };
  }, [decision.url]);
  return { videoRef, playbackRef, isWaiting, failure };
}

/** The original file, playing; when the browser cannot play it, why, in words. */
function DirectVideo(props: { readonly fileId: number; readonly decision: Decision }): JSX.Element {
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
      <SubtitlesOf fileId={props.fileId} decision={props.decision} videoRef={videoRef} />
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
export function Play(props: { readonly fileId: number }): JSX.Element {
  const { api, menu } = useServices();
  const [decision] = useResource(`play/${String(props.fileId)}`, () =>
    requestPlayback(api, props.fileId, menu()),
  );
  return (
    <Loaded
      resource={decision}
      render={(value) => <PlayView fileId={props.fileId} decision={value} />}
    />
  );
}
