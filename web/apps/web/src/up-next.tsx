// SPDX-License-Identifier: MPL-2.0

import "./up-next.css";

import type { components } from "@orpheum/sdk";
import { createContext, use, useEffect, useRef, useState, type JSX, type RefObject } from "react";
import { useTranslation } from "react-i18next";

import { placeCode } from "./episode-place.ts";
import type { FilmParts, FilmPiece } from "./film-player.tsx";
import { useCommands } from "./input-layer.tsx";
import { Still } from "./poster.tsx";
import { dataOrThrow } from "./problem.ts";
import { useServices } from "./services.tsx";
import { TvControls } from "./tv-controls.tsx";
import { useResource } from "./use-resource.ts";

/**
 * How long before the film's end the offer shows, in ms, where nothing says where its credits start
 * (plans/up-next.md §2): the length of a typical episode's end credits. A justified constant, until
 * detected credits place the offer (D82, D139).
 */
export const upNextLeadMs = 30_000;

/**
 * How long autoplay counts down before it plays the next episode, in ms: long enough to read the
 * offer and press, as every service's countdown is. A justified constant. It starts at the film's
 * end, so a fixed lead never cuts the programme's last seconds (D139; plans/up-next.md §3).
 */
export const upNextCountdownMs = 10_000;

/** What plays after a file, as the server offers it (GET /files/{id}/up-next). */
export type UpNext = components["schemas"]["UpNextBody"];

/** What plays after fileId, undefined while unknown and where nothing does. */
export function useUpNext(fileId: number): UpNext | undefined {
  const { api } = useServices();
  const [upNext] = useResource(`up-next/${String(fileId)}`, async () => {
    const answer = dataOrThrow(
      await api.GET("/files/{id}/up-next", { params: { path: { id: fileId } } }),
    );
    return answer.upNext ?? undefined;
  });
  return upNext.state === "loaded" ? upNext.value : undefined;
}

/** What the Play page gives its Up next offer: what plays next, and the person's autoplay. */
export interface UpNextOffered {
  readonly upNext: UpNext | undefined;
  readonly isAutoplayOn: boolean;
}

/** The Play page's Up next, which the offer drawn over its film reads. */
export const UpNextContext = createContext<UpNextOffered | undefined>(undefined);

/** Where the film stands for the offer: where it is, its length, and whether it has ended. */
export interface Ending {
  readonly atMs: number;
  readonly durationMs: number;
  readonly hasEnded: boolean;
}

/** What the person did to the offer: dismissed it, or stopped its countdown. */
export interface OfferChoice {
  readonly isDismissed: boolean;
  readonly isCancelled: boolean;
}

/** The offer as it first shows: neither dismissed nor stopped. */
const noChoice: OfferChoice = { isDismissed: false, isCancelled: false };

/**
 * Whether the film is near its end, and whether the offer shows and counts down: it shows within
 * upNextLeadMs of the end and on the end screen, unless dismissed; it counts once the film has
 * ended, with autoplay on, unless stopped.
 */
export function offerState(
  ending: Ending,
  choice: OfferChoice,
  isAutoplayOn: boolean,
): { readonly isNearTheEnd: boolean; readonly isShown: boolean; readonly isCounting: boolean } {
  const isNearTheEnd =
    ending.hasEnded || (ending.durationMs > 0 && ending.durationMs - ending.atMs <= upNextLeadMs);
  const isShown = isNearTheEnd && !choice.isDismissed;
  return {
    isNearTheEnd,
    isShown,
    isCounting: isShown && ending.hasEnded && isAutoplayOn && !choice.isCancelled,
  };
}

/** The Up next offer over the film, from the page's UpNextContext; nothing where nothing plays next. */
export function UpNextOver(props: FilmParts): JSX.Element | null {
  const offered = use(UpNextContext);
  const { navigation } = useServices();
  const upNext = offered?.upNext;
  if (upNext === undefined || upNext.episode.unavailable !== undefined) {
    return null; // a next episode that cannot be read now is not offered (plans/up-next.md §7)
  }
  return (
    <UpNextOffer
      key={upNext.episode.fileId}
      upNext={upNext}
      isAutoplayOn={offered?.isAutoplayOn ?? false}
      videoRef={props.videoRef}
      onPlay={() => {
        navigation.replace({ screen: "play", fileId: upNext.episode.fileId });
      }}
    />
  );
}

/**
 * What the Play page draws over its film: on a television its own bar (tv-controls.tsx), and the Up
 * next offer; on a cast's receiver nothing, which shows the film alone (D337 §3).
 */
export function overTheFilm(
  isOnTelevision: boolean,
  isReceiving: boolean,
): { readonly over?: FilmPiece } {
  if (isReceiving) {
    return {};
  }
  return { over: isOnTelevision ? TvControlsAndUpNext : UpNextOver };
}

/** A television's player overlay (tv-controls.tsx), and the Up next offer over the film. */
export function TvControlsAndUpNext(props: FilmParts): JSX.Element {
  return (
    <>
      <TvControls {...props} />
      <UpNextOver {...props} />
    </>
  );
}

/**
 * The offer itself: "Up next: S1E3 · ‹name›" with its still, a button that plays it, near the end and
 * on the end screen; with autoplay on, counting down from the end and then playing it. Any press
 * stops the count and leaves the offer; Back or Escape dismisses it, as does its ×; a click elsewhere
 * stops the count. It takes focus as the film ends, and as it shows where nothing else has it, so OK
 * presses it.
 */
export function UpNextOffer(props: {
  readonly upNext: UpNext;
  readonly isAutoplayOn: boolean;
  readonly videoRef: RefObject<HTMLVideoElement | null>;
  readonly onPlay: () => void;
}): JSX.Element | null {
  const ending = useEnding(props.videoRef);
  const [choice, setChoice] = useState<OfferChoice>(noChoice);
  const { isNearTheEnd, isShown, isCounting } = offerState(ending, choice, props.isAutoplayOn);
  // Moved back from the end, the offer comes again as it nears it: adjusted as it renders.
  const [wasNear, setWasNear] = useState(isNearTheEnd);
  if (wasNear !== isNearTheEnd) {
    setWasNear(isNearTheEnd);
    if (!isNearTheEnd) {
      setChoice(noChoice);
    }
  }
  const playRef = useRef<HTMLButtonElement>(null);
  const secondsLeft = useCountdown(isCounting, props.onPlay);
  useChoiceOfPresses({ isShown, isCounting, playRef, onChoice: setChoice });
  useFocusAsItShows(isShown, ending.hasEnded, playRef);
  if (!isShown) {
    return null;
  }
  return (
    <div className="up-next" data-focus-container="">
      <button ref={playRef} type="button" className="up-next__play" onClick={props.onPlay}>
        <OfferWords upNext={props.upNext} secondsLeft={isCounting ? secondsLeft : undefined} />
      </button>
      <DismissButton
        onDismiss={() => {
          setChoice({ isDismissed: true, isCancelled: true });
        }}
      />
    </div>
  );
}

/** What the offer says: the next episode's still, its place and name, and the count while it runs. */
function OfferWords(props: {
  readonly upNext: UpNext;
  readonly secondsLeft: number | undefined;
}): JSX.Element {
  const { t } = useTranslation();
  const { upNext, secondsLeft } = props;
  const { episode } = upNext;
  const place = placeCode(t, upNext.ordering, { ...episode, isOnlyGroup: upNext.isOnlyGroup });
  return (
    <>
      <Still sourceSha256={upNext.stillSha256} className="up-next__still" />
      <span className="up-next__words">
        <span>{t("upNext.offer", { place, name: episode.name ?? "" })}</span>
        {secondsLeft === undefined ? null : (
          <span className="up-next__count">{t("upNext.countdown", { seconds: secondsLeft })}</span>
        )}
      </span>
    </>
  );
}

/** The offer's ×, which dismisses it as Back does. */
function DismissButton(props: { readonly onDismiss: () => void }): JSX.Element {
  const { t } = useTranslation();
  return (
    <button
      type="button"
      className="up-next__dismiss"
      aria-label={t("upNext.dismiss")}
      onClick={props.onDismiss}
    >
      <span aria-hidden="true">{"×"}</span>
    </button>
  );
}

/**
 * The presses the offer hears while it shows: Back dismisses it, before anything else hears it; any
 * other, OK on its own button aside, stops its countdown; so does a click or tap anywhere but there.
 */
function useChoiceOfPresses(parts: {
  readonly isShown: boolean;
  readonly isCounting: boolean;
  readonly playRef: RefObject<HTMLButtonElement | null>;
  readonly onChoice: (change: (choice: OfferChoice) => OfferChoice) => void;
}): void {
  const { isShown, isCounting, playRef, onChoice } = parts;
  useCommands(
    "document",
    (detail, event) => {
      if (!isShown) {
        return;
      }
      if (detail.command === "back") {
        event.preventDefault();
        onChoice(() => ({ isDismissed: true, isCancelled: true }));
        return;
      }
      const isPressingPlay =
        detail.command === "select" && document.activeElement === playRef.current;
      if (isCounting && !isPressingPlay) {
        onChoice((choice) => ({ ...choice, isCancelled: true }));
      }
    },
    { isCapture: true },
  );
  useEffect(() => {
    if (!isCounting) {
      return undefined;
    }
    const onPointer = (event: Event): void => {
      if (event.target instanceof Node && playRef.current?.contains(event.target) === true) {
        return;
      }
      onChoice((choice) => ({ ...choice, isCancelled: true }));
    };
    document.addEventListener("pointerdown", onPointer, true);
    return () => {
      document.removeEventListener("pointerdown", onPointer, true);
    };
  }, [isCounting, playRef, onChoice]);
}

/**
 * Focuses the offer's button as the film ends, and as the offer shows where nothing but the video or
 * the page has focus, so a remote's OK presses it; never from another control.
 */
function useFocusAsItShows(
  isShown: boolean,
  hasEnded: boolean,
  playRef: RefObject<HTMLButtonElement | null>,
): void {
  useEffect(() => {
    const focused = document.activeElement;
    const isFree =
      focused === null || focused === document.body || focused instanceof HTMLVideoElement;
    if (isShown && (hasEnded || isFree)) {
      playRef.current?.focus();
    }
  }, [isShown, hasEnded, playRef]);
}

/**
 * The seconds autoplay has left while isCounting, one less each second from upNextCountdownMs, and
 * onPlay once they run out.
 */
function useCountdown(isCounting: boolean, onPlay: () => void): number {
  const allSeconds = Math.round(upNextCountdownMs / 1000);
  const [secondsLeft, setSecondsLeft] = useState(allSeconds);
  // A count started again starts from the top: adjusted as it renders.
  const [wasCounting, setWasCounting] = useState(isCounting);
  if (wasCounting !== isCounting) {
    setWasCounting(isCounting);
    setSecondsLeft(allSeconds);
  }
  const onPlayRef = useRef(onPlay);
  useEffect(() => {
    onPlayRef.current = onPlay;
  });
  useEffect(() => {
    if (!isCounting) {
      return undefined;
    }
    let left = allSeconds;
    const timer = setInterval(() => {
      left -= 1;
      setSecondsLeft(left);
      if (left <= 0) {
        clearInterval(timer);
        onPlayRef.current();
      }
    }, 1000);
    return () => {
      clearInterval(timer);
    };
  }, [isCounting, allSeconds]);
  return secondsLeft;
}

/**
 * Where the film stands for the offer, as the video says it at its own events: a video that plays
 * says so within a timeupdate's quarter second.
 */
function useEnding(videoRef: RefObject<HTMLVideoElement | null>): Ending {
  const [ending, setEnding] = useState<Ending>({ atMs: 0, durationMs: 0, hasEnded: false });
  useEffect(() => {
    const video = videoRef.current;
    if (video === null) {
      return undefined;
    }
    const heard = (): void => {
      const next = {
        atMs: video.currentTime * 1000,
        durationMs: Number.isFinite(video.duration) ? video.duration * 1000 : 0,
        hasEnded: video.ended,
      };
      setEnding((current) => (isSameEnding(current, next) ? current : next));
    };
    const events = ["loadedmetadata", "durationchange", "timeupdate", "seeked", "play", "ended"];
    for (const each of events) {
      video.addEventListener(each, heard);
    }
    return () => {
      for (const each of events) {
        video.removeEventListener(each, heard);
      }
    };
  }, [videoRef]);
  return ending;
}

/** Whether two readings mean the same to the offer: within a second, the same end. */
function isSameEnding(one: Ending, other: Ending): boolean {
  return (
    Math.floor(one.atMs / 1000) === Math.floor(other.atMs / 1000) &&
    one.durationMs === other.durationMs &&
    one.hasEnded === other.hasEnded
  );
}
