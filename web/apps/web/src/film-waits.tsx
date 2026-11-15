// SPDX-License-Identifier: MPL-2.0

import "./film-waits.css";
import "./play-words.ts";

import { isWaitingToPlay, startGateWaitChangedEvent } from "@orpheum/sdk";
import { useEffect, useState, type JSX, type RefObject } from "react";
import { useTranslation } from "react-i18next";

/**
 * How long a film meant to play waits for data before its spinner shows, in ms, so a quick start or
 * seek shows none. The lead's proposal, awaiting the owner (2026-11-15): half a second, a quarter of
 * the 2 s a page's part waits before its loading note (partNoteAfterMs in part-load.ts), since a film
 * waiting is watched for its picture, where a page's part waiting is read past.
 */
export const spinnerAfterMs = 500;

/** HTMLMediaElement.HAVE_FUTURE_DATA, which a plain page may not define: it can play on. */
const haveFutureData = 3;

/**
 * Whether video is paused with no play waiting for it (isWaitingToPlay, @orpheum/sdk): a pause its
 * person asked for, or a start the browser refused, which a press of play ends. A film its start
 * gate holds until enough is loaded, at its start or after a seek, is meant to play: its controls
 * show Pause, and a press pauses it for good (the owner, 2026-11-15: a film getting ready to play
 * showed ▶, and looked paused).
 */
export function isPausedAsAsked(video: HTMLVideoElement): boolean {
  return video.paused && !isWaitingToPlay(video);
}

/**
 * Whether video is meant to play but waits for data: its start gate holds its play, at its start or
 * after a seek to where too little is loaded, or it plays with too little loaded to go on, a stall.
 * A film paused as asked, ended or failed waits for nothing.
 */
export function isWaitingForData(video: HTMLVideoElement): boolean {
  if (video.error !== null || video.ended) {
    return false;
  }
  return isWaitingToPlay(video) || (!video.paused && video.readyState < haveFutureData);
}

/** The video's events at which whether it waits for data is read again. */
const waitEvents = [
  startGateWaitChangedEvent,
  "loadstart",
  "play",
  "pause",
  "waiting",
  "playing",
  "canplay",
  "canplaythrough",
  "seeking",
  "seeked",
  "emptied",
  "ended",
  "error",
];

/**
 * Whether the spinner over the video in videoRef shows: once it has waited for data
 * (isWaitingForData) for spinnerAfterMs, until it no longer does.
 */
export function useIsWaitingShown(videoRef: RefObject<HTMLVideoElement | null>): boolean {
  const [isShown, setIsShown] = useState(false);
  useEffect(() => {
    const video = videoRef.current;
    if (video === null) {
      return undefined;
    }
    let timer: ReturnType<typeof setTimeout> | undefined;
    const read = (): void => {
      if (!isWaitingForData(video)) {
        clearTimeout(timer);
        timer = undefined;
        setIsShown(false);
        return;
      }
      timer ??= setTimeout(() => {
        setIsShown(isWaitingForData(video));
      }, spinnerAfterMs);
    };
    for (const event of waitEvents) {
      video.addEventListener(event, read);
    }
    read();
    return () => {
      clearTimeout(timer);
      for (const event of waitEvents) {
        video.removeEventListener(event, read);
      }
    };
  }, [videoRef]);
  return isShown;
}

/**
 * The spinner over a film meant to play that waits for data (useIsWaitingShown), so its person
 * knows it is getting ready, not paused (the owner, 2026-11-15): at its start, after a seek to where
 * too little is loaded, and in a stall. None over a film that failed, whose page says why. It takes
 * no pointer, and says "Getting ready to play" to a screen reader as it shows.
 */
export function FilmWaitsSpinner(props: {
  readonly videoRef: RefObject<HTMLVideoElement | null>;
  readonly isFailed: boolean;
}): JSX.Element {
  const { t } = useTranslation();
  const isShown = useIsWaitingShown(props.videoRef) && !props.isFailed;
  return (
    // A span, not an output, which the player's page keeps for its words (the reconnecting note).
    <span className="film-waits" hidden={!isShown} aria-live="polite">
      {isShown ? (
        <span className="film-waits__spinner" aria-label={t("playerControls.gettingReady")} />
      ) : null}
    </span>
  );
}
