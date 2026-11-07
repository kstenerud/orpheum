// SPDX-License-Identifier: MPL-2.0

import "./player-overlay.css";

import {
  use,
  useEffect,
  useLayoutEffect,
  useRef,
  useState,
  type ComponentProps,
  type JSX,
  type RefObject,
} from "react";
import { useTranslation } from "react-i18next";

import { CastSignOverTheFilmContext } from "./cast-sign-over-the-film.tsx";
import { ConvenienceArea, type OfferPiece } from "./convenience-area.tsx";
import { useIsVolumeSettable } from "./film-volume-slider.tsx";
import type { FilmParts } from "./film-player.tsx";
import { isThisHandheld } from "./handheld.ts";
import { useCommands } from "./input-layer.tsx";
import { useLatest } from "./latest.ts";
import { OverlayUp, PlayUpwardContext } from "./play-up.tsx";
import { answerPlayerCommand, seekBy, usePlayersControls } from "./player-commands.ts";
import {
  PlayerControls,
  useVideoSource,
  type PlayerControl,
  type PlayerSound,
  type PlayerSource,
  type SequenceMoves,
} from "./player-controls.tsx";
import { PauseIcon } from "./player-icons.tsx";
import { PlayerMenus } from "./player-menus.tsx";
import {
  afterPictureTap,
  controlsShown,
  hideAfter,
  overlayAnswer,
  shownWith,
  type OverlayAnswer,
  type OverlayControl,
  volumeShownForMs,
  type OverlayState,
  type PressContext,
} from "./player-overlay-state.ts";
import { useAnswersPictureTaps, useOpensAtThePointer, type SeekFeedback } from "./player-taps.ts";
import { PlayerTracksContext } from "./player-tracks.tsx";
import { Timeline } from "./timeline.tsx";
import { TogetherSignOverTheFilm } from "./together-over-the-film.tsx";
import { peopleOf, useSessionVoice } from "./together-voice.tsx";
import { isTelevision } from "./tv-device.ts";

/**
 * The player's full-screen overlay (D407 §1–§7; plans/player-overlay.md): over the film inside its
 * stage, so it stays in full screen, three rows a remote walks by Up and Down, each a focus
 * container that remembers its last focused control (focus-memory.ts): the way up and the
 * convenience area, where an offer (Up next, Still watching) shows, alone while the rest is hidden;
 * the scrub bar with where the film stands and its length; the buttons, ⏮ ⏪ ▶ ⏩ ⏭ at the start and
 * 🔈 💬 🎧 ⁝ ⤢ at the end. + and − move the volume from anywhere and show it a moment. Hidden, Left and Right seek and show the scrub bar alone, OK
 * plays, or pauses and shows the pause sign, and Up and Down show it all with focus on the scrub
 * bar (player-overlay-state.ts, D410); a mouse's move, a click or a tap shows it all, leaving focus where it is,
 * as a desktop player's controls show, so a cast's receiver on a computer keeps its controls (D338
 * §5). The browser's own controls are in a shadow no remote reaches (the owner, Fire TV,
 * 2026-10-25). Drawn in full screen alone (page-controls.tsx), which a television's player opens
 * with; outside it the controls are under the film (D407 §9).
 */
export function PlayerOverlay(
  props: FilmParts & {
    /** The offer its convenience area draws, Up next or Still watching (convenience-area.tsx). */
    readonly offer?: OfferPiece | undefined;
    /** What its previous and next play, each shown only where given (D407 §2). */
    readonly moves?: SequenceMoves | undefined;
  },
): JSX.Element {
  const { offer, moves, ...film } = props;
  const overlayRef = useRef<HTMLDivElement>(null);
  const playRef = useRef<HTMLButtonElement>(null);
  const scrubRef = useRef<HTMLInputElement>(null);
  const buttonsRef = useRef<HTMLDivElement>(null);
  const convenienceRef = useRef<HTMLDivElement>(null);
  const [hasOffer, setHasOffer] = useState(false);
  // Whether one of its menus is open, which holds it shown and takes its presses (D407 §6, §8).
  const [isMenuOpen, setIsMenuOpen] = useState(false);
  const refs = {
    overlayRef,
    playRef,
    scrubRef,
    buttonsRef,
    videoRef: film.videoRef,
    convenienceRef,
  };
  const told = { hasOffer, isMenuOpen };
  const { state, source, isTv, isVolumeShown, seekFeedback } = useOverlay(refs, told);
  const convenience = {
    film,
    offer,
    hasOffer,
    isOverlayFull: state.shown === "full",
    onOffered: setHasOffer,
    areaRef: convenienceRef,
  };
  return (
    <>
      <span className="player-overlay-paused" hidden={state.shown !== "paused"} aria-hidden="true">
        <PauseIcon />
      </span>
      <SeekFeedbackShown feedback={seekFeedback} />
      <VolumeShown sound={source.sound} isShown={isVolumeShown} />
      <div
        ref={overlayRef}
        className="player-overlay"
        data-shown={shownWith(state.shown, hasOffer)}
        data-offer={hasOffer ? "" : undefined}
        data-focus-container=""
        data-player-controls=""
      >
        <OverlayRows
          source={source}
          isTv={isTv}
          refs={refs}
          convenience={convenience}
          moves={moves}
          onMenuOpenChange={setIsMenuOpen}
        />
      </div>
    </>
  );
}

/**
 * What the overlay shows and how it answers: its state, what a mouse, a press and time do to it,
 * and the film it reads; it is the player's own controls for that film (usePlayersControls).
 */
function useOverlay(
  refs: OverlayRefs,
  told: {
    readonly hasOffer: boolean;
    readonly isMenuOpen: boolean;
  },
): {
  readonly state: OverlayState;
  readonly source: PlayerSource;
  readonly isTv: boolean;
  readonly isVolumeShown: boolean;
  readonly seekFeedback: SeekFeedback | undefined;
} {
  const { hasOffer, isMenuOpen } = told;
  const isTv = isTelevision(navigator.userAgent);
  const [state, setState] = useState<OverlayState>(() => openedWith(isTv));
  const isCovered = state.shown === "hidden" || state.shown === "convenience";
  const source = useVideoSource(refs.videoRef, isCovered);
  const notePress = useOverlayHiding({
    state,
    setState,
    isPaused: source.isPaused,
    isMenuOpen,
    ...refs,
  });
  const stateRef = useLatest(state);
  const isMenuOpenRef = useLatest(isMenuOpen);
  const { onPictureTap, feedback } = useAnswersPictureTaps(refs.videoRef, () => {
    setState(afterPictureTap(stateRef.current));
    notePress();
  });
  useOpensAtThePointer(
    () => {
      setState({ shown: "full", shownBy: "pointer" });
      notePress();
    },
    { isHeld: () => isMenuOpenRef.current, onPictureTap },
  );
  const volume = useVolumeShown();
  const presses = { state, setState, isTv, hasOffer, isMenuOpen, onVolumeShown: volume.show };
  useOverlayPresses({ ...presses, notePress, ...refs });
  usePlayersControls(refs.overlayRef, refs.videoRef);
  return { state, source, isTv, isVolumeShown: volume.isShown, seekFeedback: feedback };
}

/**
 * Whether the volume shows, and what shows it: from a press of + or − for volumeShownForMs after
 * the last (D407 §10), as the scrub bar shows alone after a seek.
 */
function useVolumeShown(): { readonly isShown: boolean; readonly show: () => void } {
  const [pressCount, setPressCount] = useState(0);
  const [isShown, setIsShown] = useState(false);
  useEffect(() => {
    if (pressCount === 0) {
      return undefined;
    }
    const timer = setTimeout(() => {
      setIsShown(false);
    }, volumeShownForMs);
    return () => {
      clearTimeout(timer);
    };
  }, [pressCount]);
  return {
    isShown,
    show: () => {
      setIsShown(true);
      setPressCount((count) => count + 1);
    },
  };
}

/**
 * A double-tap seek's "+10" or "−10", stacking as "+20", on the side tapped, fading shortly after the
 * last tap (D415 §5). It is aria-hidden: the seek is what a screen reader's user has, not a second
 * announcement; it takes no pointer and no focus.
 */
function SeekFeedbackShown(props: {
  readonly feedback: SeekFeedback | undefined;
}): JSX.Element | null {
  const { t } = useTranslation();
  const { feedback } = props;
  if (feedback === undefined) {
    return null;
  }
  const key =
    feedback.direction === 1 ? "playerControls.seekedForward" : "playerControls.seekedBack";
  return (
    <span
      className="player-overlay-seeked"
      data-side={feedback.direction === 1 ? "right" : "left"}
      data-shown={feedback.isShown ? "" : undefined}
      aria-hidden="true"
    >
      {t(key, { seconds: feedback.seconds })}
    </span>
  );
}

/** The volume + or − left, in the middle of the film a moment (useVolumeShown); muted is none. */
function VolumeShown(props: {
  readonly sound: PlayerSound | undefined;
  readonly isShown: boolean;
}): JSX.Element | null {
  const { t } = useTranslation();
  const { sound } = props;
  if (sound === undefined) {
    return null;
  }
  const percent = sound.isMuted ? 0 : Math.round(sound.volume * 100);
  return (
    <output className="player-overlay-volume" hidden={!props.isShown}>
      {props.isShown ? t("playerControls.volumeLevel", { percent }) : ""}
    </output>
  );
}

/**
 * The overlay as the player opens: hidden on every device, the picture alone with nothing ringed
 * (ui-audit-2026-11-04.md §3, the player: "no ring, controls hidden"); the first press shows it
 * (player-overlay-state.ts), a remote's on a television.
 */
function openedWith(isTv: boolean): OverlayState {
  return { shown: "hidden", shownBy: isTv ? "remote" : "pointer" };
}

/**
 * What the overlay's parts hold: itself, play, the scrub bar, the buttons' row, the video and the
 * convenience area.
 */
interface OverlayRefs {
  readonly overlayRef: RefObject<HTMLDivElement | null>;
  readonly playRef: RefObject<HTMLButtonElement | null>;
  readonly scrubRef: RefObject<HTMLInputElement | null>;
  readonly buttonsRef: RefObject<HTMLDivElement | null>;
  readonly videoRef: RefObject<HTMLVideoElement | null>;
  readonly convenienceRef: RefObject<HTMLDivElement | null>;
}

/** The tray's signs over the film, last in the top row: Watch Together's, then Cast's (D422, D423). */
function SignsOverTheFilm(props: {
  readonly onOpenChange: (isOpen: boolean) => void;
}): JSX.Element {
  const castSign = use(CastSignOverTheFilmContext);
  return (
    <>
      <TogetherSignOverTheFilm onOpenChange={props.onOpenChange} />
      {castSign(props.onOpenChange)}
    </>
  );
}

/**
 * The overlay's three rows (D407 §1): the way up and the convenience area (convenience-area.tsx);
 * the scrub bar; the buttons, those the device shows (controlsShown), the menus 🔈, Subtitles, Sound
 * track and ⁝ before full screen (player-menus.tsx), and previous and next where the sequence has
 * them (moves). Exported for the UI gallery, which draws the real rows over no film (ui-gallery.tsx);
 * what the overlay does with them, its showing, hiding and presses, is PlayerOverlay's.
 */
export function OverlayRows({
  source,
  isTv,
  refs,
  convenience,
  moves,
  onMenuOpenChange,
}: {
  readonly source: PlayerSource;
  readonly isTv: boolean;
  readonly refs: Pick<OverlayRefs, "playRef" | "scrubRef" | "buttonsRef" | "videoRef">;
  readonly convenience: Omit<ComponentProps<typeof ConvenienceArea>, "leftMs">;
  readonly moves: SequenceMoves | undefined;
  readonly onMenuOpenChange: (isOpen: boolean) => void;
}): JSX.Element {
  const { t } = useTranslation();
  const { playRef, scrubRef, buttonsRef, videoRef } = refs;
  const shown = useControlsShown(isTv, moves);
  const menus = { videoRef, shown, sound: source.sound, onOpenChange: onMenuOpenChange };
  return (
    <>
      <div className="player-overlay__row player-overlay__top" data-focus-container="">
        <OverlayUp />
        <ConvenienceArea
          {...convenience}
          leftMs={source.durationMs === 0 ? undefined : source.durationMs - source.atMs}
        />
        <SignsOverTheFilm onOpenChange={onMenuOpenChange} />
      </div>
      <div className="player-overlay__row player-overlay__scrub" data-focus-container="">
        <Timeline
          inputRef={scrubRef}
          label={t("playerControls.where")}
          atMs={source.atMs}
          durationMs={source.durationMs}
          isDisabled={!source.isReady}
          hasArrowsAsCommands
          onSeek={source.seekTo}
        />
      </div>
      <div
        ref={buttonsRef}
        className="player-overlay__row player-overlay__buttons"
        data-focus-container=""
      >
        <PlayerControls
          source={source}
          controls={barControlsOf(shown)}
          isOverVideo
          isOverlayRow
          playRef={playRef}
          moves={moves}
          menus={<PlayerMenus {...menus} />}
          videoRef={videoRef}
        />
      </div>
    </>
  );
}

/** The controls the overlay shows on this device (controlsShown), and the bar under the film. */
export function useControlsShown(
  isTv: boolean,
  moves: SequenceMoves | undefined,
): readonly OverlayControl[] {
  const tracks = use(PlayerTracksContext);
  const upward = use(PlayUpwardContext);
  const isVolumeSettable = useIsVolumeSettable();
  const voice = useSessionVoice();
  return controlsShown({
    isTelevision: isTv,
    isTouch: isThisHandheld(),
    isVolumeSettable,
    hasVoicesToMix: voice !== undefined && peopleOf(voice.snapshot).length > 0,
    audioTrackCount: tracks?.audio.tracks.length ?? 0,
    hasPrevious: moves?.previous !== undefined,
    hasNext: moves?.next !== undefined,
    isReceiver: upward === undefined,
  });
}

/**
 * The bar's controls among those the overlay shows, which the bar draws today: the sound is 🔈 among
 * the menus (player-menus.tsx), not the bar's mute and slider.
 */
export function barControlsOf(shown: readonly OverlayControl[]): PlayerControl[] {
  const drawn: Partial<Record<OverlayControl, PlayerControl>> = {
    previous: "previous",
    seekBack: "steps",
    play: "play",
    next: "next",
    fullScreen: "fullScreen",
  };
  return [...new Set(shown.flatMap((control) => drawn[control] ?? []))];
}

/**
 * Answers a press as the overlay's state says (overlayAnswer): showing or hiding, a seek, play or
 * pause, focus moved, the offer pressed; a press taken is no one else's. Focus on the offer in the
 * convenience area, Up next or Still watching, leaves it OK and Back while the rest is hidden. + and
 * − are the player's, the overlay only showing the volume they leave (onVolumeShown).
 */
function useOverlayPresses({
  state,
  setState,
  notePress,
  isTv,
  hasOffer,
  isMenuOpen,
  onVolumeShown,
  overlayRef,
  playRef,
  scrubRef,
  buttonsRef,
  videoRef,
  convenienceRef,
}: OverlayRefs & {
  readonly state: OverlayState;
  readonly setState: (state: OverlayState) => void;
  readonly notePress: () => void;
  readonly isTv: boolean;
  readonly hasOffer: boolean;
  readonly isMenuOpen: boolean;
  readonly onVolumeShown: () => void;
}): void {
  // Nothing is focused onto the overlay as the film opens: it opens hidden (openedWith).
  const askFocusToScrub = useFocusOnceDrawn(scrubRef, false);
  usePlayingLeavesPlay(playRef, videoRef, isTv);
  useMenuLeftAtFullScreenFocusesScrub(isMenuOpen, askFocusToScrub);
  useCommands(
    "document",
    (detail, event) => {
      const video = videoRef.current;
      if (video === null) {
        return;
      }
      const parts = overlayParts({ overlayRef, scrubRef, buttonsRef, convenienceRef }, hasOffer);
      const context = { ...pressContext(parts, video, isTv), isMenuOpen };
      const answer = overlayAnswer(state, detail.command, context);
      setState(answer.state);
      notePress();
      if (answer.isOfferPressed === true) {
        offerOf(parts.convenience)?.click();
      }
      if (answer.seekByMs !== undefined) {
        seekBy(video, answer.seekByMs < 0 ? -1 : 1, detail); // its seek names the arrow pressed
      }
      if (answer.isPlayPause === true) {
        answerPlayerCommand(video, "playPause");
      }
      if (answer.isVolumeShown === true) {
        onVolumeShown();
      }
      moveFocus(answer.focus, video, askFocusToScrub);
      if (answer.isTaken) {
        event.preventDefault();
      }
    },
    { isCapture: true },
  );
}

/**
 * Moves focus where a press's answer asks: to the scrub bar once drawn; to it at once, the press then
 * moving it on as if it had been there (D411); or to the video.
 */
function moveFocus(
  focus: OverlayAnswer["focus"],
  video: HTMLVideoElement,
  askFocusToScrub: (isAtOnce?: boolean) => void,
): void {
  if (focus === "scrub") {
    askFocusToScrub();
  } else if (focus === "fromScrub") {
    askFocusToScrub(true);
  } else if (focus === "video") {
    // Down from the buttons (Q14), once the press is through.
    queueMicrotask(() => {
      focusTheVideo(video);
    });
  }
}

/**
 * Moves focus to the scrub bar as full screen changes with a menu open: the menu closes first, its
 * focus going back to its button, then focus goes on to the scrub bar (D407 §8, D410): on a Fire TV, Silk's Back leaves
 * full screen, no key the page hears (idiosyncrasies.md, a modal dialog removed while open).
 */
function useMenuLeftAtFullScreenFocusesScrub(
  isMenuOpen: boolean,
  askFocusToScrub: () => void,
): void {
  const toldRef = useLatest({ isMenuOpen, askFocusToScrub });
  useEffect(() => {
    const changed = (): void => {
      if (toldRef.current.isMenuOpen) {
        toldRef.current.askFocusToScrub();
      }
    };
    document.addEventListener("fullscreenchange", changed);
    return () => {
      document.removeEventListener("fullscreenchange", changed);
    };
  }, [toldRef]);
}

/**
 * On a computer, a press of play that plays the film, OK's, Space's (player-commands.ts leaves Space
 * on a button to the button) or a mouse's, moves focus to the video, so the overlay that focus in it
 * held hides on its time (D407 §6, Q14). Heard as play's click comes to it, before its own answer
 * plays the film, so whether the film was paused reads true; focus moves once the click is through.
 */
function usePlayingLeavesPlay(
  playRef: RefObject<HTMLButtonElement | null>,
  videoRef: RefObject<HTMLVideoElement | null>,
  isTv: boolean,
): void {
  useEffect(() => {
    const play = playRef.current;
    if (isTv || play === null) {
      return undefined;
    }
    const pressed = (): void => {
      const video = videoRef.current;
      if (video?.paused === true) {
        queueMicrotask(() => {
          focusTheVideo(video);
        });
      }
    };
    play.addEventListener("click", pressed, true);
    return () => {
      play.removeEventListener("click", pressed, true);
    };
  }, [playRef, videoRef, isTv]);
}

/**
 * The overlay's parts as a press finds them: itself, the scrub bar, the buttons' row, and the
 * convenience area while an offer is in it.
 */
interface OverlayParts {
  readonly overlay: HTMLElement | null;
  readonly scrub: HTMLElement | null;
  readonly buttons: HTMLElement | null;
  readonly convenience: HTMLElement | null;
}

/** The overlay's parts as a press finds them, the convenience area only while it holds an offer. */
function overlayParts(
  refs: Pick<OverlayRefs, "overlayRef" | "scrubRef" | "buttonsRef" | "convenienceRef">,
  hasOffer: boolean,
): OverlayParts {
  return {
    overlay: refs.overlayRef.current,
    scrub: refs.scrubRef.current,
    buttons: refs.buttonsRef.current,
    convenience: hasOffer ? refs.convenienceRef.current : null,
  };
}

/** What the overlay knows as a press comes: where focus is, and whether the film is paused. */
function pressContext(parts: OverlayParts, video: HTMLVideoElement, isTv: boolean): PressContext {
  const focused = document.activeElement;
  return {
    isPaused: video.paused,
    isTelevision: isTv,
    isOnScrub: focused !== null && focused === parts.scrub,
    isOnButtons: parts.buttons?.contains(focused) === true,
    hasOffer: parts.convenience !== null,
    isOnOffer: parts.convenience?.contains(focused) === true,
    isFocusOutside: parts.overlay?.contains(focused) === false,
  };
}

/** The offer's own press in the convenience area: its first button, Up next's play or Continue. */
function offerOf(convenience: HTMLElement | null): HTMLButtonElement | null {
  return convenience?.querySelector("button") ?? null;
}

/** Moves focus to the video, which takes it from a script alone where it is no stop of its own. */
function focusTheVideo(video: HTMLVideoElement): void {
  if (!video.hasAttribute("tabindex")) {
    video.setAttribute("tabindex", "-1");
  }
  video.focus();
}

/**
 * Returns what asks focus onto the scrub bar (D410) once the overlay it opens is drawn: after React's
 * commit, not at the next frame, which a Fire TV ran before the commit, the control then without a
 * box to take focus (2026-11-02: OK opened the overlay with nothing focused). isAskedAtOnce asks it as
 * the overlay opens with the film: on a television focus is on a control while the overlay shows
 * (D407 §6). Asked with isAtOnce, it moves focus there now, the overlay shown already: the press
 * asking goes on to move it as if it had been there (D411).
 */
function useFocusOnceDrawn(
  scrubRef: RefObject<HTMLInputElement | null>,
  isAskedAtOnce: boolean,
): (isAtOnce?: boolean) => void {
  const [asked, setAsked] = useState(isAskedAtOnce ? 1 : 0);
  useEffect(() => {
    if (asked > 0) {
      scrubRef.current?.focus();
    }
  }, [asked, scrubRef]);
  return (isAtOnce = false) => {
    if (isAtOnce) {
      scrubRef.current?.focus(); // drawn already, the press then moving it on (D411)
    } else {
      setAsked((count) => count + 1);
    }
  };
}

/**
 * How often the overlay looks whether its time is up, in ms: a quarter of the scrub bar's 1.5 s, so it
 * goes near its time. A justified constant.
 */
const hidingCheckMs = 250;

/**
 * Hides the overlay its time after the last press (hideAfter), a seek landing counting as one, so the
 * scrub bar stays its time after the film shows the new place; as it hides, focus in the overlay
 * moves to the offer in its convenience area, or else the video, so no hidden control holds it
 * (useFocusLeavesWhatHides). A menu open holds it (D407 §6). Returns what notes a press, which
 * renews the time.
 */
function useOverlayHiding({
  state,
  setState,
  isPaused,
  isMenuOpen,
  overlayRef,
  videoRef,
  convenienceRef,
}: Pick<OverlayRefs, "overlayRef" | "videoRef" | "convenienceRef"> & {
  readonly state: OverlayState;
  readonly setState: (state: OverlayState) => void;
  readonly isPaused: boolean;
  readonly isMenuOpen: boolean;
}): () => void {
  const lastPressAtRef = useRef(0);
  const afterMs = hideAfter(state, { isPaused, isMenuOpen });
  // A menu closing counts as a press: the overlay's time starts again from it.
  useEffect(() => {
    if (!isMenuOpen) {
      lastPressAtRef.current = performance.now();
    }
  }, [isMenuOpen]);
  useEffect(() => {
    if (afterMs === undefined) {
      return undefined;
    }
    const video = videoRef.current;
    const landed = (): void => {
      lastPressAtRef.current = performance.now();
    };
    video?.addEventListener("seeked", landed);
    const timer = setInterval(() => {
      if (performance.now() - lastPressAtRef.current >= afterMs) {
        setState({ ...state, shown: "hidden" });
      }
    }, hidingCheckMs);
    return () => {
      video?.removeEventListener("seeked", landed);
      clearInterval(timer);
    };
  }, [afterMs, state, setState, videoRef]);
  useFocusLeavesWhatHides(state, { overlayRef, videoRef, convenienceRef });
  return () => {
    lastPressAtRef.current = performance.now();
  };
}

/**
 * Moves focus out of what of the overlay hides as all of it but the scrub bar and the convenience
 * area hides, before the browser drops it to the page's body with the hidden control (Firefox did,
 * before an effect ran): to the offer there while one is on, so OK presses it (D407 §3), else to the
 * video. Focus already on the offer stays.
 */
function useFocusLeavesWhatHides(
  state: OverlayState,
  refs: Pick<OverlayRefs, "overlayRef" | "videoRef" | "convenienceRef">,
): void {
  const { overlayRef, videoRef, convenienceRef } = refs;
  useLayoutEffect(() => {
    const video = videoRef.current;
    const focused = document.activeElement;
    const isOnOffer = convenienceRef.current?.contains(focused) === true;
    if (state.shown === "full" || video === null || isOnOffer) {
      return;
    }
    if (focused === document.body || overlayRef.current?.contains(focused) === true) {
      const offer = offerOf(convenienceRef.current);
      if (offer === null) {
        focusTheVideo(video);
      } else {
        offer.focus();
      }
    }
  }, [state.shown, overlayRef, videoRef, convenienceRef]);
}
