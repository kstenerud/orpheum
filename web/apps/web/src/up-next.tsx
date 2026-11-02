// SPDX-License-Identifier: MPL-2.0

import "./up-next.css";

import type { components } from "@orpheum/sdk";
import {
  createContext,
  use,
  useEffect,
  useMemo,
  useRef,
  useState,
  type JSX,
  type RefObject,
} from "react";
import { useTranslation } from "react-i18next";

import { moveOnOrReady, type Receiving } from "./cast-receiver.tsx";
import { placeCode } from "./episode-place.ts";
import type { FilmParts, FilmPiece } from "./film-player.tsx";
import { IconButton } from "./icon-button.tsx";
import { useCommands } from "./input-layer.tsx";
import { PageControls } from "./page-controls.tsx";
import { CloseIcon } from "./player-icons.tsx";
import { PlayerOverlay } from "./player-overlay.tsx";
import { Still } from "./poster.tsx";
import {
  noteAutoplayed,
  StillWatching,
  useAnyPressForgetsAutoplays,
  useIsAskingStillWatching,
} from "./still-watching.tsx";
import { dataOrThrow } from "./problem.ts";
import { useServices } from "./services.tsx";
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

/**
 * What plays after fileId, undefined while unknown and where nothing does; asked for once isAsked,
 * nothing before.
 */
export function useUpNext(fileId: number, isAsked = true): UpNext | undefined {
  const { api } = useServices();
  const [upNext] = useResource(
    `up-next/${String(fileId)}${isAsked ? "" : "/not-yet"}`,
    async () => {
      if (!isAsked) {
        return undefined;
      }
      const answer = dataOrThrow(
        await api.GET("/files/{id}/up-next", { params: { path: { id: fileId } } }),
      );
      return answer.upNext ?? undefined;
    },
  );
  return upNext.state === "loaded" ? upNext.value : undefined;
}

/**
 * Whether video has played fileId: true from its first playing, or a timeupdate while it plays,
 * for a video already playing when it is handed over.
 */
function useHasPlayed(video: HTMLVideoElement | null, fileId: number): boolean {
  const [playedFileId, setPlayedFileId] = useState<number | undefined>(undefined);
  useEffect(() => {
    if (video === null || playedFileId === fileId) {
      return undefined;
    }
    const heard = (): void => {
      if (!video.paused) {
        setPlayedFileId(fileId);
      }
    };
    video.addEventListener("playing", heard);
    video.addEventListener("timeupdate", heard);
    return () => {
      video.removeEventListener("playing", heard);
      video.removeEventListener("timeupdate", heard);
    };
  }, [video, fileId, playedFileId]);
  return playedFileId === fileId;
}

/**
 * What the Play page gives its Up next offer: what plays next, the person's autoplay, and the cast
 * this device receives, while it does.
 */
export interface UpNextOffered {
  readonly upNext: UpNext | undefined;
  readonly isAutoplayOn: boolean;
  readonly receiving?: Receiving | undefined;
}

/** The Play page's Up next, which the offer drawn over its film reads. */
export const UpNextContext = createContext<UpNextOffered | undefined>(undefined);

/**
 * What the Play page of fileId gives its Up next offer, the same object while nothing changes. What
 * plays next is asked for once its video has played: asked as the page opened, it was one more
 * request beside the first chunks, the playlists and the inits, on a link the first chunks need whole
 * (the efficiency audit's finding 4), for an offer shown at the film's end.
 */
export function useUpNextOffered(
  fileId: number,
  video: HTMLVideoElement | null,
  playing: Omit<UpNextOffered, "upNext">,
): UpNextOffered {
  const { isAutoplayOn, receiving } = playing;
  const upNext = useUpNext(fileId, useHasPlayed(video, fileId));
  return useMemo(() => ({ upNext, isAutoplayOn, receiving }), [upNext, isAutoplayOn, receiving]);
}

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

/**
 * The Up next offer over the film, from the page's UpNextContext; nothing where nothing plays next.
 * On a cast's receiver it is shown and counts, but presses there are Take back control's: its film
 * is the cast's next episode, which the remote follows (D337's §9 answers, Q4).
 */
export function UpNextOver(props: FilmParts): JSX.Element | null {
  const offered = use(UpNextContext);
  const { api, navigation } = useServices();
  const upNext = offered?.upNext;
  if (upNext === undefined || upNext.episode.unavailable !== undefined) {
    return null; // a next episode that cannot be read now is not offered (plans/up-next.md §7)
  }
  const receiving = offered?.receiving;
  return (
    <UpNextOffer
      key={upNext.episode.fileId}
      upNext={upNext}
      isAutoplayOn={offered?.isAutoplayOn ?? false}
      videoRef={props.videoRef}
      onPlay={() => {
        if (receiving === undefined) {
          navigation.replace({ screen: "play", fileId: upNext.episode.fileId });
        } else {
          moveOnOrReady(api, receiving);
        }
      }}
    />
  );
}

/**
 * What the Play page draws over its film: the player's own controls, every device's (D369 §2), a
 * full-screen overlay on a television or a cast's receiver (D338 §5, D407, player-overlay.tsx), its
 * stage put full screen as it opens, else a computer's and a phone's (page-controls.tsx); and the Up
 * next offer.
 */
export function overTheFilm(hasTelevisionControls: boolean): FilmPiece {
  return hasTelevisionControls ? OverlayAndUpNext : PageControlsAndUpNext;
}

/** The player's full-screen overlay (player-overlay.tsx), and the Up next offer over the film. */
function OverlayAndUpNext(props: FilmParts): JSX.Element {
  return (
    <>
      <PlayerOverlay {...props} shouldOpenFullScreen />
      <UpNextOver {...props} />
    </>
  );
}

/** A computer's and a phone's player controls (page-controls.tsx), and the Up next offer. */
function PageControlsAndUpNext(props: FilmParts): JSX.Element {
  return (
    <>
      <PageControls {...props} />
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
  /** Whether it is pressed here; not, it shows its words alone, with nothing to press. */
  readonly isPressable?: boolean;
  readonly videoRef: RefObject<HTMLVideoElement | null>;
  readonly onPlay: () => void;
}): JSX.Element | null {
  const ending = useEnding(props.videoRef);
  const { isShown, isCounting, setChoice } = useOfferChoice(ending, props.isAutoplayOn);
  const playRef = useRef<HTMLButtonElement>(null);
  const isPressable = props.isPressable !== false;
  const { isAsking, secondsLeft } = useAutoplay(isCounting, isPressable, props.onPlay);
  const isAnswered = isPressable && !isAsking;
  useChoiceOfPresses({
    isShown: isShown && isAnswered,
    isCounting: isCounting && isAnswered,
    playRef,
    onChoice: setChoice,
  });
  useFocusAsItShows(isShown && isAnswered, ending.hasEnded, playRef);
  const { navigation } = useServices();
  if (!isShown) {
    return null;
  }
  const dismiss = (): void => {
    setChoice({ isDismissed: true, isCancelled: true });
  };
  if (isAsking) {
    return (
      <StillWatching
        onContinue={props.onPlay}
        onStop={() => {
          dismiss();
          navigation.back();
        }}
      />
    );
  }
  const words = (
    <OfferWords upNext={props.upNext} secondsLeft={isCounting ? secondsLeft : undefined} />
  );
  if (!isPressable) {
    return (
      <div className="up-next">
        <div className="up-next__play">{words}</div>
      </div>
    );
  }
  return (
    <div className="up-next" data-focus-container="">
      <button ref={playRef} type="button" className="up-next__play" onClick={props.onPlay}>
        {words}
      </button>
      <DismissButton onDismiss={dismiss} />
    </div>
  );
}

/**
 * What the person did to the offer, and so whether it shows and counts; moved back from the end, the
 * offer comes again as it nears it.
 */
function useOfferChoice(
  ending: Ending,
  isAutoplayOn: boolean,
): {
  readonly isShown: boolean;
  readonly isCounting: boolean;
  readonly setChoice: (change: OfferChoice | ((choice: OfferChoice) => OfferChoice)) => void;
} {
  const [choice, setChoice] = useState<OfferChoice>(noChoice);
  const { isNearTheEnd, isShown, isCounting } = offerState(ending, choice, isAutoplayOn);
  // Adjusted as it renders.
  const [wasNear, setWasNear] = useState(isNearTheEnd);
  if (wasNear !== isNearTheEnd) {
    setWasNear(isNearTheEnd);
    if (!isNearTheEnd) {
      setChoice(noChoice);
    }
  }
  return { isShown, isCounting, setChoice };
}

/**
 * Autoplay's count while isCounting, and onPlay once it runs out; where presses answer it, a run of
 * autoplays with none between asks "Still watching?" instead of counting (D339).
 */
function useAutoplay(
  isCounting: boolean,
  isPressable: boolean,
  onPlay: () => void,
): { readonly isAsking: boolean; readonly secondsLeft: number } {
  useAnyPressForgetsAutoplays();
  const isAsking = useIsAskingStillWatching(isCounting && isPressable);
  const secondsLeft = useCountdown(isCounting && !isAsking, () => {
    if (isPressable) {
      noteAutoplayed();
    }
    onPlay();
  });
  return { isAsking, secondsLeft };
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
    <IconButton
      isOverVideo
      className="up-next__dismiss"
      label={t("upNext.dismiss")}
      icon={<CloseIcon />}
      onClick={props.onDismiss}
    />
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
