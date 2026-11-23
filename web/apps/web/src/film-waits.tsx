// SPDX-License-Identifier: MPL-2.0

import "./film-waits.css";
import "./play-words.ts";

import type { LinkWait, LinkWaitWatch } from "@orpheum/player";
import { isWaitingToPlay, startGateWaitChangedEvent } from "@orpheum/sdk";
import { useEffect, useRef, useState, type JSX, type RefObject } from "react";
import { useTranslation } from "react-i18next";

import { WaitingIcon } from "./icons.tsx";
import { Pill } from "./pill.tsx";

/**
 * How long a film meant to play waits for data before its spinner shows, in ms, so a quick start or
 * seek shows none. The lead's proposal, awaiting the owner (2026-11-15): half a second, a quarter of
 * the 2 s a page's part waits before its loading note (partNoteAfterMs in part-load.ts), since a film
 * waiting is watched for its picture, where a page's part waiting is read past.
 */
export const spinnerAfterMs = 500;

/**
 * How long a wait has been the link's, in ms, before the spinner's bar shows how much of its chunks
 * has come over the link (the owner, 2026-11-17): a wait that long is the link's holding the film up,
 * not the server's (D506 §4).
 */
export const linkWaitBarAfterMs = 3000;

/** How often the spinner reads its wait's chunks, in ms, for its bar to move as they come. */
const linkWaitReadEveryMs = 250;

/** The bar's least and most filled, so it never looks empty nor done while it waits (the owner, 2026-11-17). */
const leastShare = 0.1;
const mostShare = 0.9;

/**
 * How much of the bar under the spinner is filled for wait, between a tenth and nine tenths of it:
 * the bytes of its chunks come against those needed, the least while no head has said a length, the
 * most once all of it has come; undefined, no bar, where wait says nothing (the server making a chunk
 * of it, a player that loads no chunks of its own), or before it has been the link's for
 * linkWaitBarAfterMs. A bar shown already, isBarShown, stays without waiting again: once shown, it
 * shows until the film has enough to play, hidden only while the server makes a chunk (D506 §1, §4).
 */
export function linkWaitShare(wait: LinkWait | undefined, isBarShown = false): number | undefined {
  if (wait === undefined || (!isBarShown && wait.linksForMs < linkWaitBarAfterMs)) {
    return undefined;
  }
  if (wait.neededBytes === 0) {
    return leastShare;
  }
  return Math.min(mostShare, Math.max(leastShare, wait.arrivedBytes / wait.neededBytes));
}

/**
 * How much of the spinner's bar is filled (linkWaitShare) while isShown, read every
 * linkWaitReadEveryMs from a watch watchLinkWait begins as the spinner shows, or once it can (its
 * player made), stopped as the spinner hides; undefined while it is hidden.
 */
function useLinkWaitShare(
  isShown: boolean,
  watchLinkWait: (() => LinkWaitWatch | undefined) | undefined,
): number | undefined {
  const [share, setShare] = useState<number | undefined>(undefined);
  const watchLinkWaitRef = useRef(watchLinkWait);
  useEffect(() => {
    watchLinkWaitRef.current = watchLinkWait;
  }, [watchLinkWait]);
  useEffect(() => {
    if (!isShown) {
      return undefined;
    }
    let watch = watchLinkWaitRef.current?.();
    let isBarShown = false;
    const read = (): void => {
      watch ??= watchLinkWaitRef.current?.();
      const next = linkWaitShare(watch?.read(), isBarShown);
      isBarShown = next !== undefined;
      setShare(next);
    };
    const timer = setInterval(read, linkWaitReadEveryMs);
    return () => {
      clearInterval(timer);
      watch?.stop();
      setShare(undefined);
    };
  }, [isShown]);
  return share;
}

/** HTMLMediaElement.HAVE_FUTURE_DATA, which a plain page may not define: it can play on. */
const haveFutureData = 3;

/**
 * Whether video is paused with no play waiting for it (isWaitingToPlay, @orpheum/sdk): a pause its
 * person asked for, or a start the browser refused, which a press of play ends. A film its start
 * gate holds until enough is loaded, at its start or after a seek, is meant to play: its controls
 * show Pause, and a press pauses it for good (the owner, 2026-11-15: a film getting ready to play
 * showed ▶, and looked paused). A film whose load failed waits for nothing, whatever its gate
 * still holds: it shows ▶, as before.
 */
export function isPausedAsAsked(video: HTMLVideoElement): boolean {
  return video.paused && !isMeantToPlay(video);
}

/**
 * Whether video's start gate holds a play for it (isWaitingToPlay, @orpheum/sdk) that can still
 * come: not once its load failed, which leaves the gate's play pending with nothing to fill its wait.
 */
export function isMeantToPlay(video: HTMLVideoElement): boolean {
  return video.error === null && isWaitingToPlay(video);
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
  return isMeantToPlay(video) || (!video.paused && video.readyState < haveFutureData);
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
 * no pointer, and says "Getting ready to play" to a screen reader as it shows. It is a turning ring,
 * or, asked for less motion, the still-reading hourglass pulsing (film-waits.css). Under it, once
 * the wait has been the link's for linkWaitBarAfterMs (D506 §4), a bar shows how much of
 * the wait's chunks has come (linkWaitShare; the owner, 2026-11-17), watched by watchLinkWait, none
 * where it is undefined; it adds no words, so a screen reader is not told it.
 */
export function FilmWaitsSpinner(props: {
  readonly videoRef: RefObject<HTMLVideoElement | null>;
  readonly isFailed: boolean;
  readonly watchLinkWait?: (() => LinkWaitWatch | undefined) | undefined;
}): JSX.Element {
  const isShown = useIsWaitingShown(props.videoRef) && !props.isFailed;
  const share = useLinkWaitShare(isShown, props.watchLinkWait);
  return (
    // Its words are a Pill's, read out once as they show (D538 §1).
    <span className="film-waits" hidden={!isShown}>
      {isShown ? <FilmWaitsLook share={share} /> : null}
    </span>
  );
}

/**
 * What the spinner draws while it shows: its sign, the ring or, asked for less motion, the
 * hourglass (film-waits.css); under it, where share is not undefined, the bar filled to that share
 * (linkWaitShare), which says nothing; and under those "Getting ready to play", a Pill whose words
 * take the place of the sign's label (D538 §1). The UI gallery draws
 * it too, so it shows the real thing (the owner, 2026-11-17).
 */
export function FilmWaitsLook(props: { readonly share: number | undefined }): JSX.Element {
  const { t } = useTranslation();
  const { share } = props;
  return (
    <>
      <span className="film-waits__sign" aria-hidden="true">
        <span className="film-waits__spinner" />
        <span className="film-waits__hourglass">
          <WaitingIcon />
        </span>
      </span>
      {share === undefined ? null : (
        <span className="film-waits__bar" aria-hidden="true">
          <span className="film-waits__fill" style={{ width: `${String(share * 100)}%` }} />
        </span>
      )}
      <span className="film-waits__words">
        <Pill>{t("playerControls.gettingReady")}</Pill>
      </span>
    </>
  );
}
