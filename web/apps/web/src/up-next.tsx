// SPDX-License-Identifier: MPL-2.0

import "./up-next.css";
import "./play-words.ts";

import {
  createContext,
  use,
  useEffect,
  useMemo,
  useRef,
  useState,
  type Context,
  type JSX,
  type ReactNode,
  type RefObject,
} from "react";
import { isNearItsEnd, type Ending } from "@orpheum/sdk";
import { useTranslation } from "react-i18next";

import { useAroundFile, type Previous, type UpNext } from "./around-file.ts";
import { moveBack, moveOnOrReady, type Receiving } from "./cast-receiver.tsx";
import { useFocusAsItShows, useTellsOffered, type OfferPlace } from "./convenience-area.tsx";
import { placeCode } from "./episode-place.ts";
import type { FilmParts, FilmPiece } from "./film-player.tsx";
import { OfferCard } from "./offer-card.tsx";
import type { MediaTracks } from "./input-commands.ts";
import { useCommands } from "./input-layer.tsx";
import { useAtTheEndWithNothingNext } from "./film-end.ts";
import { useLatest } from "./latest.ts";
import { OverTheFilm, PageControls } from "./page-controls.tsx";
import { overlayUpClass, PlayUpwardContext } from "./play-up.tsx";
import type { SequenceMoves } from "./player-controls.tsx";
import { Still } from "./poster.tsx";
import {
  noteAutoplayed,
  StillWatching,
  useAnyPressForgetsAutoplays,
  useIsAskingStillWatching,
} from "./still-watching.tsx";
import {
  creditsStartOf,
  segmentsOf,
  SegmentsContext,
  SkipOffer,
  SkipsCommercials,
  type MediaSegment,
} from "./segments.tsx";
import { useServices } from "./services.tsx";
import { parentOf } from "./upward.ts";
import { useVideoStanding, type StandingTaken, type VideoStanding } from "./video-standing.ts";
import { watchingOf } from "./watching.ts";

/**
 * How long autoplay counts down before it plays the next episode, in ms: long enough to read the
 * offer and press, as every service's countdown is. A justified constant. It starts at the start
 * of the film's credits, where a segment says where they are, else at its end, so a fixed lead never
 * cuts the programme's last seconds (D139, D339; plans/up-next.md §3, §9).
 */
export const upNextCountdownMs = 10_000;

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
 * What the Play page gives its Up next offer and its previous and next: what plays next and what
 * before, the person's autoplay, and the cast this device receives, while it does.
 */
export interface UpNextOffered {
  readonly upNext: UpNext | undefined;
  readonly previous?: Previous | undefined;
  /** Whether what plays next and before is known: the server answered, or it cannot be read. */
  readonly isNextKnown?: boolean;
  readonly isAutoplayOn: boolean;
  readonly receiving?: Receiving | undefined;
}

/** The Play page's Up next, which the offer drawn over its film reads. */
export const UpNextContext: Context<UpNextOffered | undefined> = createContext<
  UpNextOffered | undefined
>(undefined);

/**
 * What the Play page gives the offers over its film (PlayersOffers): its Up next (UpNextContext) and
 * the segments of its file (SegmentsContext), whose commercials skip themselves as video plays them.
 */
export function PlaysOffered(props: {
  readonly offered: UpNextOffered | undefined;
  readonly segments: readonly MediaSegment[] | null;
  readonly video: HTMLVideoElement | null;
  readonly children: ReactNode;
}): JSX.Element {
  const segments = useMemo(() => segmentsOf(props.segments), [props.segments]);
  return (
    <UpNextContext value={props.offered}>
      <SegmentsContext value={segments}>
        <SkipsCommercials video={props.video} segments={segments} />
        {props.children}
      </SegmentsContext>
    </UpNextContext>
  );
}

/**
 * What the Play page of fileId gives its Up next offer, the same object while nothing changes. What
 * plays next is asked for once its video has played: asked as the page opened, it was one more
 * request beside the first chunks, the playlists and the inits, on a link the first chunks need whole
 * (the efficiency audit's finding 4), for an offer shown at the film's end.
 */
export function useUpNextOffered(
  fileId: number,
  video: HTMLVideoElement | null,
  playing: Omit<UpNextOffered, "upNext" | "previous" | "isNextKnown">,
): UpNextOffered {
  const { isAutoplayOn, receiving } = playing;
  // A guest receiver asks it of its cast's film, the one it plays, which the core opens to it (D471 §1).
  const around = useAroundFile(fileId, useHasPlayed(video, fileId));
  const upNext = around?.upNext;
  const previous = around?.previous;
  const isNextKnown = around !== undefined;
  return useMemo(
    () => ({ upNext, previous, isNextKnown, isAutoplayOn, receiving }),
    [upNext, previous, isNextKnown, isAutoplayOn, receiving],
  );
}

/**
 * Whether the film is known to have nothing playing after it: no next episode, or one that cannot
 * be read now, which is not offered (plans/up-next.md §7); false while what plays next is unknown.
 */
export function useIsNothingNext(offered: UpNextOffered | undefined): boolean {
  const { next } = useSequenceMoves(offered);
  return offered?.isNextKnown === true && next === undefined;
}

/**
 * Leaves the player as its film ends with nothing playing after it, whatever the person's autoplay
 * (the owner, 2026-11-17), so no press at its end plays it again: for the page above it, its Up's
 * (OverlayUp, D367 §8), a film's page or an episode's own; back to that page's entry where it is
 * behind, else in the player's place, never a new entry, as Up leaves it. Nothing on a cast's
 * receiver, whose page has no Up (PlayUpwardContext): its cast is made ready instead
 * (cast-receiver.tsx).
 */
export function LeavesAtTheEnd(props: {
  readonly video: HTMLVideoElement | null;
  /** Whether nothing is known to play after the film (useIsNothingNext). */
  readonly isNothingNext: boolean;
}): null {
  useAtTheEndWithNothingNext(props.video, props.isNothingNext, useLeavesForThePageAbove());
  return null;
}

/**
 * The Play page's leaving for the page above it, its Up's (OverlayUp, D367 §8), as a film's end with
 * nothing next leaves it (the owner, 2026-11-17): back to that page's entry where it is behind, else
 * in the player's place, never a new entry. Undefined on a page with no Up, a cast's receiver's
 * (PlayUpwardContext).
 */
export function useLeavesForThePageAbove(): (() => void) | undefined {
  const { navigation } = useServices();
  const upward = use(PlayUpwardContext);
  const parent = upward === undefined ? undefined : parentOf(upward);
  return parent === undefined
    ? undefined
    : () => {
        navigation.up(parent.to, parent.isIt);
      };
}

/**
 * What the player's previous and next play (D407 §2, §10): the episode before and after this one,
 * each where there is one that can be read now, played the way Up next plays. On a cast's receiver,
 * a guest receiver among them, next moves the cast on, as its Up next does, and previous moves it
 * back, as its caster's ⏮ does (D471 §1).
 */
export function useSequenceMoves(offered: UpNextOffered | undefined): SequenceMoves {
  const { api, navigation } = useServices();
  const upNext = offered?.upNext?.episode;
  const previous = offered?.previous;
  const receiving = offered?.receiving;
  return useMemo(() => {
    const isPlayable = (episode: Previous | undefined): episode is Previous =>
      episode !== undefined && episode.unavailable === undefined;
    const playing = (episode: Previous | undefined): (() => void) | undefined => {
      if (!isPlayable(episode)) {
        return undefined;
      }
      return () => {
        navigation.replace({ screen: "play", fileId: episode.fileId, itemId: episode.itemId });
      };
    };
    if (receiving === undefined) {
      return { previous: playing(previous), next: playing(upNext) };
    }
    const movingOn = (): void => {
      moveOnOrReady(api, receiving);
    };
    const movingBack = (): void => {
      moveBack(api, receiving);
    };
    return {
      previous: isPlayable(previous) ? movingBack : undefined,
      next: isPlayable(upNext) ? movingOn : undefined,
    };
  }, [api, navigation, upNext, previous, receiving]);
}

/**
 * Answers the previous and next commands (a keyboard's media keys, the browser's media controls) by
 * playing the episode before or after this one, where there is one (useSequenceMoves); returns which
 * there are, so the browser's media controls offer only those (hearMediaSeeks).
 */
export function useSequenceCommands(offered: UpNextOffered): Required<MediaTracks> {
  const moves = useSequenceMoves(offered);
  useCommands("document", (detail, event) => {
    if (detail.command !== "previous" && detail.command !== "next") {
      return;
    }
    const move = moves[detail.command];
    if (move !== undefined) {
      event.preventDefault();
      move();
    }
  });
  const hasPrevious = moves.previous !== undefined;
  const hasNext = moves.next !== undefined;
  return useMemo(() => ({ hasPrevious, hasNext }), [hasPrevious, hasNext]);
}

/** What the person did to the offer: dismissed it, or stopped its countdown. */
export interface OfferChoice {
  readonly isDismissed: boolean;
  readonly isCancelled: boolean;
}

/** The offer as it first shows: neither dismissed nor stopped. */
const noChoice: OfferChoice = { isDismissed: false, isCancelled: false };

/**
 * Whether the film is near its end, and whether the offer shows and counts down: it shows from its
 * credits' start where the file has a credits segment, else within upNextLeadMs of the end, and on
 * the end screen, unless dismissed (isNearItsEnd); it counts from the credits' start, else once the
 * film has ended, with autoplay on, unless stopped.
 */
export function offerState(
  ending: Ending,
  choice: OfferChoice,
  isAutoplayOn: boolean,
): { readonly isNearTheEnd: boolean; readonly isShown: boolean; readonly isCounting: boolean } {
  const isNearTheEnd = isNearItsEnd(ending);
  const isShown = isNearTheEnd && !choice.isDismissed;
  return {
    isNearTheEnd,
    isShown,
    isCounting: isShown && isAtItsCredits(ending) && isAutoplayOn && !choice.isCancelled,
  };
}

/** Whether the film has ended, or plays its credits where a segment says where they start. */
function isAtItsCredits(ending: Ending): boolean {
  const { creditsStartMs } = ending;
  return ending.hasEnded || (creditsStartMs !== undefined && ending.atMs >= creditsStartMs);
}

/**
 * The Up next offer over the film, from the page's UpNextContext; nothing where nothing plays next.
 * On a cast's receiver it is shown and counts, but presses there are Take back control's: its film
 * is the cast's next episode, which the remote follows (D337's §9 answers, Q4). In the overlay's
 * convenience area it is told where it is (OfferPlace). As it shows, the next episode is warmed up
 * (useWarmsUpNext), so its start is fetched before it plays (the owner's request, 2026-11-11). It
 * shows and counts from the credits where the file has a credits segment (SegmentsContext; D139),
 * on a cast's receiver too, whose remote and Chromecast twin read the same segments, so all three
 * show and go together, as the player does locally (D546).
 */
export function UpNextOver(props: FilmParts & OfferPlace): JSX.Element | null {
  const offered = use(UpNextContext);
  const segments = use(SegmentsContext);
  const { next } = useSequenceMoves(offered);
  const upNext = offered?.upNext;
  const isHere = offered?.receiving === undefined;
  const warmUpNext = useWarmsUpNext(upNext?.episode, isHere);
  if (upNext === undefined || next === undefined) {
    return null; // a next episode that cannot be read now is not offered (plans/up-next.md §7)
  }
  return (
    <UpNextOffer
      key={upNext.episode.fileId}
      upNext={upNext}
      isAutoplayOn={offered?.isAutoplayOn ?? false}
      creditsStartMs={creditsStartOf(segments)}
      videoRef={props.videoRef}
      isOverlayFull={props.isOverlayFull}
      onOffered={props.onOffered}
      onShown={warmUpNext}
      onPlay={next}
    />
  );
}

/**
 * What warms up fileId, the next episode, as the Up next offer shows: its decision alone, nothing
 * preloaded beside the film playing, which keeps its playbacks to its end (Watching.warmUpNext).
 * Nothing where it plays on a cast's receiver, not here (isHere false), whose cast moves on to it
 * instead.
 */
function useWarmsUpNext(
  next: { readonly fileId: number; readonly itemId: number } | undefined,
  isHere: boolean,
): (() => void) | undefined {
  const { api, playableFormats } = useServices();
  const fileId = next?.fileId;
  const itemId = next?.itemId;
  return useMemo(() => {
    if (fileId === undefined || !isHere) {
      return undefined;
    }
    return () => {
      watchingOf(api).warmUpNext(fileId, playableFormats(), itemId);
    };
  }, [api, playableFormats, fileId, itemId, isHere]);
}

/**
 * What the Play page draws over its film: the player's full-screen overlay while full screen, the
 * Up next offer in its convenience area and ⏮ ⏭ in its row (D407 §3, page-controls.tsx), nothing
 * over the video otherwise (D407 §9). A television's or a cast's receiver's stage is put full
 * screen as it opens (D338 §5); a computer's film answers a mouse's click and double click.
 */
export function overTheFilm(hasTelevisionControls: boolean): FilmPiece {
  return hasTelevisionControls ? OverlayOpeningFullScreen : OverlayOnAComputer;
}

/** A television's overlay, the page's offers and ⏮ ⏭ in it, its stage full screen as it opens. */
function OverlayOpeningFullScreen(props: FilmParts): JSX.Element {
  const moves = useSequenceMoves(use(UpNextContext));
  return <OverTheFilm {...props} shouldOpenFullScreen offer={PlayersOffers} moves={moves} />;
}

/** A computer's and a phone's overlay, the page's offers and ⏮ ⏭ in it, and the film's clicks. */
function OverlayOnAComputer(props: FilmParts): JSX.Element {
  const moves = useSequenceMoves(use(UpNextContext));
  return <OverTheFilm {...props} hasPressesOnTheFilm offer={PlayersOffers} moves={moves} />;
}

/**
 * The player's controls under the film outside full screen, every device's (D407 §9,
 * page-controls.tsx), the page's offers among them as ordinary buttons.
 */
export function ControlsUnderTheFilm(props: FilmParts): JSX.Element | null {
  const moves = useSequenceMoves(use(UpNextContext));
  return <PageControls {...props} offer={PlayersOffers} moves={moves} />;
}

/**
 * The Play page's offers over its film, one at a time (D82; plans/media-segments.md): Skip intro,
 * recap, preview or credits while one of those segments plays (SkipOffer), and Up next or Still
 * watching near the end (UpNextOver), which takes the place of Skip credits on an episode with a next
 * one, and of any skip while it shows. The overlay is told an offer shows while either does. A cast's
 * receiver offers no skip: its presses are Take back control's (plans/up-next.md §8).
 */
export function PlayersOffers(props: FilmParts & OfferPlace): JSX.Element {
  const offered = use(UpNextContext);
  const segments = use(SegmentsContext);
  const { next } = useSequenceMoves(offered);
  const [isSkipOffered, setIsSkipOffered] = useState(false);
  const [isNextOffered, setIsNextOffered] = useState(false);
  useTellsOffered(isSkipOffered || isNextOffered, props.onOffered);
  const isSkipHere = offered?.receiving === undefined && !isNextOffered;
  return (
    <>
      {isSkipHere ? (
        <SkipOffer
          videoRef={props.videoRef}
          segments={segments}
          isCreditsLeftToUpNext={next !== undefined}
          onOffered={setIsSkipOffered}
        />
      ) : null}
      <UpNextOver {...props} onOffered={setIsNextOffered} />
    </>
  );
}

/** What the Up next offer is given: where it is, what it offers, and what it tells. */
interface UpNextOfferParts extends OfferPlace {
  readonly upNext: UpNext;
  readonly isAutoplayOn: boolean;
  /** Where the film's credits start, where a segment says so: the offer shows and counts from it. */
  readonly creditsStartMs?: number | undefined;
  /** Whether it is pressed here; not, it shows its words alone, with nothing to press. */
  readonly isPressable?: boolean;
  readonly videoRef: RefObject<HTMLVideoElement | null>;
  /** Told each time the offer comes to show, once each time. */
  readonly onShown?: (() => void) | undefined;
  readonly onPlay: () => void;
}

/**
 * The offer itself: "Up next: S1E3 · ‹name›" with its still, a button that plays it, near the end and
 * on the end screen; with autoplay on, counting down from the end and then playing it. Any press
 * stops the count and leaves the offer; Back or Escape puts it away, the film playing on, but where
 * all of the overlay shows, whose first Back hides it (D407 §5); so does the player's own Back's
 * first press, its next leaving (it has no ✕: the owner, 2026-11-23); a click elsewhere stops the
 * count. It takes focus as the film ends, and as it shows where nothing else has it, so OK presses
 * it.
 */
export function UpNextOffer(props: UpNextOfferParts): JSX.Element | null {
  const ending = useEnding(props.videoRef, props.creditsStartMs);
  const { isShown, isCounting, setChoice } = useOfferChoice(ending, props);
  useTellsShown(isShown, props.onShown);
  const playRef = useRef<HTMLButtonElement>(null);
  const isPressable = props.isPressable !== false;
  const { isAsking, secondsLeft } = useAutoplay(isCounting, isPressable, props.onPlay);
  const isAnswered = isPressable && !isAsking;
  useChoiceOfPresses({
    isShown: isShown && isAnswered,
    isCounting: isCounting && isAnswered,
    isBackLeft: props.isOverlayFull === true,
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
  return (
    <UpNextCard
      upNext={props.upNext}
      secondsLeft={isCounting ? secondsLeft : undefined}
      play={isPressable ? { onPlay: props.onPlay, playRef } : undefined}
    />
  );
}

/**
 * What the person did to the offer, and so whether it shows and counts, told to onOffered where
 * given; moved back from the end, the offer comes again as it nears it.
 */
function useOfferChoice(
  ending: Ending,
  { isAutoplayOn, onOffered }: Pick<OfferPlace, "onOffered"> & { readonly isAutoplayOn: boolean },
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
  useTellsOffered(isShown, onOffered);
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

/**
 * The Up next card (OfferCard, D538 §1): the next episode's still, "Up next: S1E3 · ‹name›", then
 * "Plays in 10 s" while the count runs, said once as it starts and not at each second, or "Play now"
 * where it is pressed; the card plays it, and has no ✕ (Back puts it away: UpNextOffer). Where
 * nothing here presses it (a cast's receiver), its words alone. The cast's remote's Up next is this
 * card, playing it there.
 */
export function UpNextCard(props: {
  readonly upNext: UpNext;
  /** The count's seconds left while it runs; none, no count. */
  readonly secondsLeft?: number | undefined;
  /** Its press, where it is pressed here. */
  readonly play?:
    | {
        readonly onPlay: () => void;
        readonly playRef?: RefObject<HTMLButtonElement | null>;
      }
    | undefined;
}): JSX.Element {
  const { t } = useTranslation();
  const { upNext, secondsLeft, play } = props;
  const { episode } = upNext;
  const place = placeCode(t, upNext.ordering, { ...episode, isOnlyGroup: upNext.isOnlyGroup });
  const isCounting = secondsLeft !== undefined;
  const countdown = isCounting ? t("upNext.countdown", { seconds: secondsLeft }) : undefined;
  return (
    <OfferCard
      words={t("upNext.offer", { place, name: episode.name ?? "" })}
      said={
        isCounting
          ? t("upNext.countdownSaid", { seconds: Math.round(upNextCountdownMs / 1000) })
          : undefined
      }
      detail={countdown ?? (play === undefined ? undefined : t("upNext.playNow"))}
      still={
        <Still sourceSha256={upNext.stillSha256} className="up-next__still" isNoPosterMarked />
      }
      onTake={play?.onPlay}
      takeRef={play?.playRef}
    />
  );
}

/** Tells onShown, where given, as the offer comes to show: once each time it does. */
function useTellsShown(isShown: boolean, onShown: (() => void) | undefined): void {
  const onShownRef = useLatest(onShown);
  useEffect(() => {
    if (isShown) {
      onShownRef.current?.();
    }
  }, [isShown, onShownRef]);
}

/**
 * The presses the offer hears while it shows: Back dismisses it, before anything else hears it,
 * unless isBackLeft, where all of the overlay shows and Back hides it (D407 §5); a press of the
 * player's own Back (OverlayUp) dismisses it in place of leaving, so the next one leaves; any other,
 * OK on its own button aside, stops its countdown; so does a click or tap anywhere but there.
 */
function useChoiceOfPresses(parts: {
  readonly isShown: boolean;
  readonly isCounting: boolean;
  readonly isBackLeft: boolean;
  readonly playRef: RefObject<HTMLButtonElement | null>;
  readonly onChoice: (change: (choice: OfferChoice) => OfferChoice) => void;
}): void {
  const { isShown, isCounting, isBackLeft, playRef, onChoice } = parts;
  useCommands(
    "document",
    (detail, event) => {
      if (!isShown) {
        return;
      }
      if (detail.command === "back" && !isBackLeft) {
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
    if (!isShown) {
      return undefined;
    }
    const onClick = (event: MouseEvent): void => {
      if (event.target instanceof Element && event.target.closest(`.${overlayUpClass}`) !== null) {
        event.preventDefault();
        event.stopPropagation();
        onChoice(() => ({ isDismissed: true, isCancelled: true }));
      }
    };
    document.addEventListener("click", onClick, true);
    return () => {
      document.removeEventListener("click", onClick, true);
    };
  }, [isShown, onChoice]);
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
  const onPlayRef = useLatest(onPlay);
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
  }, [isCounting, allSeconds, onPlayRef]);
  return secondsLeft;
}

/** The events at which the offer reads where the film stands. */
const endingEvents = ["loadedmetadata", "durationchange", "timeupdate", "seeked", "play", "ended"];

/**
 * The offer's taking of the video's reading, for a film whose credits start at creditsStartMs, where
 * known: drawn again only when it means something new.
 */
function endingTakingOf(creditsStartMs: number | undefined): StandingTaken<Ending> {
  return {
    take: (standing: VideoStanding) => ({
      atMs: standing.atMs,
      durationMs: standing.durationMs,
      hasEnded: standing.hasEnded,
      creditsStartMs,
    }),
    isSame: isSameEnding,
    initial: { atMs: 0, durationMs: 0, hasEnded: false, creditsStartMs },
    isReadAtOnce: false,
  };
}

/**
 * Where the film stands for the offer, its credits at creditsStartMs where known, as the video says
 * it at its own events, from the one reading every hearer of the video takes (useVideoStanding),
 * the bar's too: a video that plays says so within a timeupdate's quarter second.
 */
function useEnding(
  videoRef: RefObject<HTMLVideoElement | null>,
  creditsStartMs: number | undefined,
): Ending {
  const taking = useMemo(() => endingTakingOf(creditsStartMs), [creditsStartMs]);
  return useVideoStanding(videoRef, endingEvents, taking);
}

/**
 * Whether two readings mean the same to the offer: both far from the end and its credits, where it
 * shows nothing whatever the second (the web client's architecture audit of 2026-11-10,
 * Playback 8); or within a second, the same end.
 */
function isSameEnding(one: Ending, other: Ending): boolean {
  if (!isNearItsEnd(one) && !isNearItsEnd(other)) {
    return true;
  }
  return (
    Math.floor(one.atMs / 1000) === Math.floor(other.atMs / 1000) &&
    one.durationMs === other.durationMs &&
    one.hasEnded === other.hasEnded
  );
}
