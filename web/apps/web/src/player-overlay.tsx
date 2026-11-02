// SPDX-License-Identifier: MPL-2.0

import "./player-overlay.css";

import { use, useEffect, useLayoutEffect, useRef, useState, type JSX, type RefObject } from "react";
import { useTranslation } from "react-i18next";

import { useIsVolumeSettable } from "./film-volume-slider.tsx";
import { fillTheScreen } from "./film-fullscreen.ts";
import type { FilmParts } from "./film-player.tsx";
import { isThisHandheld } from "./handheld.ts";
import { useCommands } from "./input-layer.tsx";
import { OverlayUp, PlayUpwardContext } from "./play-up.tsx";
import { answerPlayerCommand, seekBy } from "./player-commands.ts";
import {
  PlayerControls,
  useVideoSource,
  type PlayerControl,
  type PlayerSource,
} from "./player-controls.tsx";
import { PauseIcon } from "./player-icons.tsx";
import {
  controlsShown,
  hideAfter,
  overlayAnswer,
  type OverlayControl,
  type OverlayState,
  type PressContext,
} from "./player-overlay-state.ts";
import { PlayerTracksContext } from "./player-tracks.tsx";
import { Timeline } from "./timeline.tsx";
import { isTelevision } from "./tv-device.ts";

/**
 * The player's full-screen overlay (D407 §1–§7; plans/player-overlay.md): over the film inside its
 * stage, so it stays in full screen, three rows a remote walks by Up and Down, each a focus
 * container that remembers its last focused control (focus-memory.ts): the way up and what plays;
 * the scrub bar with where the film stands and its length; the buttons, ⏪ ▶ ⏩ at the start and full
 * screen and the sound at the end. Hidden, Left and Right seek and show the scrub bar alone, OK
 * plays, or pauses and shows the pause sign, and Up and Down show it all with focus on play
 * (player-overlay-state.ts); a mouse's move, a click or a tap shows it all, leaving focus where it is,
 * as a desktop player's controls show, so a cast's receiver on a computer keeps its controls (D338
 * §5). The browser's own controls are in a shadow no remote reaches (the owner, Fire TV,
 * 2026-10-25). shouldOpenFullScreen puts its stage full screen as the player opens, as a television's
 * player is, filling the window where the browser refuses.
 */
export function PlayerOverlay(
  props: FilmParts & { readonly shouldOpenFullScreen?: boolean | undefined },
): JSX.Element {
  const { videoRef, shouldOpenFullScreen } = props;
  const overlayRef = useRef<HTMLDivElement>(null);
  const playRef = useRef<HTMLButtonElement>(null);
  const scrubRef = useRef<HTMLInputElement>(null);
  const buttonsRef = useRef<HTMLDivElement>(null);
  const isTv = isTelevision(navigator.userAgent);
  const [state, setState] = useState<OverlayState>(() => openedWith(isTv));
  const isCovered = state.shown === "hidden" || state.shown === "convenience";
  const source = useVideoSource(videoRef, isCovered);
  const notePress = useOverlayHiding({
    state,
    setState,
    isPaused: source.isPaused,
    overlayRef,
    videoRef,
  });
  useOpensAtThePointer(() => {
    setState({ shown: "full", shownBy: "pointer" });
    notePress();
  });
  useOverlayPresses({
    state,
    setState,
    notePress,
    isTv,
    overlayRef,
    playRef,
    scrubRef,
    buttonsRef,
    videoRef,
  });
  useFullscreenStage(overlayRef, shouldOpenFullScreen === true);
  return (
    <>
      <span className="player-overlay-paused" hidden={state.shown !== "paused"} aria-hidden="true">
        <PauseIcon />
      </span>
      <div
        ref={overlayRef}
        className="player-overlay"
        data-shown={state.shown}
        data-focus-container=""
        data-player-controls=""
      >
        <OverlayRows
          source={source}
          isTv={isTv}
          playRef={playRef}
          scrubRef={scrubRef}
          buttonsRef={buttonsRef}
        />
      </div>
    </>
  );
}

/** The overlay as the player opens: all of it, shown by a remote on a television. */
function openedWith(isTv: boolean): OverlayState {
  return { shown: "full", shownBy: isTv ? "remote" : "pointer" };
}

/** What the overlay's parts hold: itself, play, the scrub bar, the buttons' row and the video. */
interface OverlayRefs {
  readonly overlayRef: RefObject<HTMLDivElement | null>;
  readonly playRef: RefObject<HTMLButtonElement | null>;
  readonly scrubRef: RefObject<HTMLInputElement | null>;
  readonly buttonsRef: RefObject<HTMLDivElement | null>;
  readonly videoRef: RefObject<HTMLVideoElement | null>;
}

/**
 * The overlay's three rows (D407 §1): the way up (the offer's place, Up next or Still watching, is
 * still the page's own beside the overlay until the convenience area comes, plan step 8); the scrub
 * bar; the buttons, those the device shows (controlsShown). Subtitles, audio and more come with
 * their menus (plan step 7), previous and next with sequences (step 9).
 */
function OverlayRows({
  source,
  isTv,
  playRef,
  scrubRef,
  buttonsRef,
}: {
  readonly source: PlayerSource;
  readonly isTv: boolean;
  readonly playRef: RefObject<HTMLButtonElement | null>;
  readonly scrubRef: RefObject<HTMLInputElement | null>;
  readonly buttonsRef: RefObject<HTMLDivElement | null>;
}): JSX.Element {
  const { t } = useTranslation();
  const shown = useControlsShown(isTv);
  return (
    <>
      <div className="player-overlay__row player-overlay__top" data-focus-container="">
        <OverlayUp />
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
          aside={
            <EndsAt
              leftMs={source.durationMs === 0 ? undefined : source.durationMs - source.atMs}
            />
          }
        />
      </div>
    </>
  );
}

/** The controls the overlay shows on this device (controlsShown). */
function useControlsShown(isTv: boolean): readonly OverlayControl[] {
  const isVolumeSettable = useIsVolumeSettable();
  const tracks = use(PlayerTracksContext);
  const upward = use(PlayUpwardContext);
  return controlsShown({
    isTelevision: isTv,
    isTouch: isThisHandheld(),
    isVolumeSettable,
    audioTrackCount: tracks?.audio.tracks.length ?? 0,
    hasPrevious: false,
    hasNext: false,
    isReceiver: upward === undefined,
  });
}

/** The bar's controls among those the overlay shows, which the bar draws today. */
function barControlsOf(shown: readonly OverlayControl[]): PlayerControl[] {
  const drawn: Partial<Record<OverlayControl, PlayerControl>> = {
    seekBack: "steps",
    play: "play",
    fullScreen: "fullScreen",
    sound: "sound",
  };
  return [...new Set(shown.flatMap((control) => drawn[control] ?? []))];
}

/**
 * Answers a press as the overlay's state says (overlayAnswer): showing or hiding, a seek, play or
 * pause, focus moved; a press taken is no one else's. Focus on an offer drawn over the film beside
 * the overlay, Up next or Still watching, leaves it OK and Back.
 */
function useOverlayPresses({
  state,
  setState,
  notePress,
  isTv,
  overlayRef,
  playRef,
  scrubRef,
  buttonsRef,
  videoRef,
}: OverlayRefs & {
  readonly state: OverlayState;
  readonly setState: (state: OverlayState) => void;
  readonly notePress: () => void;
  readonly isTv: boolean;
}): void {
  const askFocusToPlay = useFocusToPlayOnceDrawn(playRef, isTv);
  usePlayingLeavesPlay(playRef, videoRef, isTv);
  useCommands(
    "document",
    (detail, event) => {
      const video = videoRef.current;
      if (video === null) {
        return;
      }
      const parts = {
        overlay: overlayRef.current,
        scrub: scrubRef.current,
        buttons: buttonsRef.current,
      };
      const answer = overlayAnswer(state, detail.command, pressContext(parts, video, isTv));
      setState(answer.state);
      notePress();
      if (answer.seekByMs !== undefined) {
        seekBy(video, answer.seekByMs < 0 ? -1 : 1, detail); // its seek names the arrow pressed
      }
      if (answer.isPlayPause === true) {
        answerPlayerCommand(video, "playPause");
      }
      if (answer.focus === "play") {
        askFocusToPlay();
      } else if (answer.focus === "video") {
        // Down from the buttons (Q14), once the press is through.
        queueMicrotask(() => {
          focusTheVideo(video);
        });
      }
      if (answer.isTaken) {
        event.preventDefault();
      }
    },
    { isCapture: true },
  );
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

/** The overlay's parts as a press finds them: itself, the scrub bar and the buttons' row. */
interface OverlayParts {
  readonly overlay: HTMLElement | null;
  readonly scrub: HTMLElement | null;
  readonly buttons: HTMLElement | null;
}

/** What the overlay knows as a press comes: where focus is, and whether the film is paused. */
function pressContext(parts: OverlayParts, video: HTMLVideoElement, isTv: boolean): PressContext {
  const focused = document.activeElement;
  return {
    isPaused: video.paused,
    isTelevision: isTv,
    isOnScrub: focused !== null && focused === parts.scrub,
    isOnButtons: parts.buttons?.contains(focused) === true,
    isOnOffer: isOnAnotherControlOverTheFilm(parts.overlay),
  };
}

/** Moves focus to the video, which takes it from a script alone where it is no stop of its own. */
function focusTheVideo(video: HTMLVideoElement): void {
  if (!video.hasAttribute("tabindex")) {
    video.setAttribute("tabindex", "-1");
  }
  video.focus();
}

/**
 * Returns what asks focus onto play once the overlay it opens is drawn: after React's commit, not at
 * the next frame, which a Fire TV ran before the commit, play then without a box to take focus
 * (2026-11-02: OK opened the overlay with nothing focused). isAskedAtOnce asks it as the overlay
 * opens with the film: on a television focus is on a control while the overlay shows (D407 §6).
 */
function useFocusToPlayOnceDrawn(
  playRef: RefObject<HTMLButtonElement | null>,
  isAskedAtOnce: boolean,
): () => void {
  const [asked, setAsked] = useState(isAskedAtOnce ? 1 : 0);
  useEffect(() => {
    if (asked > 0) {
      playRef.current?.focus();
    }
  }, [asked, playRef]);
  return () => {
    setAsked((count) => count + 1);
  };
}

/** When the film ends if it plays on from now, leftMs from its end; nothing while its length is not known. */
function EndsAt(props: { readonly leftMs: number | undefined }): JSX.Element {
  const { t } = useTranslation();
  const { leftMs } = props;
  return <>{leftMs === undefined ? "" : t("playerControls.endsAt", { time: endsAt(leftMs) })}</>;
}

/**
 * Whether focus is on a control the page draws over the film beside the overlay, as Up next: in the
 * stage, neither the video nor in the overlay.
 */
function isOnAnotherControlOverTheFilm(overlay: HTMLElement | null): boolean {
  const focused = document.activeElement;
  const stage = overlay?.closest(".film-player__stage");
  return (
    focused instanceof HTMLElement &&
    !(focused instanceof HTMLVideoElement) &&
    stage?.contains(focused) === true &&
    overlay?.contains(focused) !== true
  );
}

/**
 * The clock time a film ends at, as the viewer's clock says it, made once: toLocaleTimeString built
 * a formatter at each call, 38 µs here against 1.8 µs for one kept, four times a second (the
 * efficiency audit's finding 7).
 */
const endsAtClock = new Intl.DateTimeFormat([], { hour: "numeric", minute: "2-digit" });

/** When the film ends if it plays on from now, as the viewer's clock says it. */
function endsAt(leftMs: number): string {
  return endsAtClock.format(new Date(Date.now() + leftMs));
}

/**
 * How often the overlay looks whether its time is up, in ms: a quarter of the scrub bar's 1.5 s, so it
 * goes near its time. A justified constant.
 */
const hidingCheckMs = 250;

/**
 * Hides the overlay its time after the last press (hideAfter), a seek landing counting as one, so the
 * scrub bar stays its time after the film shows the new place; as it hides, focus in the overlay
 * moves to the video, so no hidden control holds it, and what else over the film has focus, Up next,
 * keeps it. Returns what notes a press, which renews the time.
 */
function useOverlayHiding({
  state,
  setState,
  isPaused,
  overlayRef,
  videoRef,
}: {
  readonly state: OverlayState;
  readonly setState: (state: OverlayState) => void;
  readonly isPaused: boolean;
  readonly overlayRef: RefObject<HTMLElement | null>;
  readonly videoRef: RefObject<HTMLVideoElement | null>;
}): () => void {
  const lastPressAtRef = useRef(0);
  const isFocusIn = useIsFocusIn(overlayRef);
  const afterMs = hideAfter(state, { isPaused, isMenuOpen: false, isFocusIn });
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
  useFocusLeavesWhatHides(state, overlayRef, videoRef);
  return () => {
    lastPressAtRef.current = performance.now();
  };
}

/**
 * Moves focus out of the overlay to the video as all of it but the scrub bar hides, before the
 * browser drops it to the page's body with the hidden control (Firefox did, before an effect ran).
 */
function useFocusLeavesWhatHides(
  state: OverlayState,
  overlayRef: RefObject<HTMLElement | null>,
  videoRef: RefObject<HTMLVideoElement | null>,
): void {
  useLayoutEffect(() => {
    const video = videoRef.current;
    if (state.shown !== "full" && video !== null) {
      const focused = document.activeElement;
      if (focused === document.body || overlayRef.current?.contains(focused) === true) {
        focusTheVideo(video);
      }
    }
  }, [state.shown, overlayRef, videoRef]);
}

/** Whether focus is in the element overlayRef holds, as its focusin and focusout say. */
export function useIsFocusIn(overlayRef: RefObject<HTMLElement | null>): boolean {
  const [isFocusIn, setIsFocusIn] = useState(false);
  useEffect(() => {
    const overlay = overlayRef.current;
    if (overlay === null) {
      return undefined;
    }
    const focusedIn = (): void => {
      setIsFocusIn(true);
    };
    const focusedOut = (event: FocusEvent): void => {
      setIsFocusIn(event.relatedTarget instanceof Node && overlay.contains(event.relatedTarget));
    };
    overlay.addEventListener("focusin", focusedIn);
    overlay.addEventListener("focusout", focusedOut);
    return () => {
      overlay.removeEventListener("focusin", focusedIn);
      overlay.removeEventListener("focusout", focusedOut);
    };
  }, [overlayRef]);
  return isFocusIn;
}

/**
 * Opens a player's controls (open) at a mouse's move, or a click or a tap anywhere on the page, as a
 * desktop player shows its controls, the overlay hiding them again after its time: a cast's receiver
 * on a computer showed no controls to its mouse (the owner, 2026-10-27). A move that did not change
 * the pointer's place is none (a browser may move it by itself as what lies under it changes), and a
 * click a key made (no count) is the key's, which the overlay answers as a press already. Focus
 * stays where it is. The latest open is called each time.
 */
export function useOpensAtThePointer(open: () => void): void {
  const openRef = useRef(open);
  useEffect(() => {
    openRef.current = open;
  });
  useEffect(() => {
    let lastPlace: string | undefined;
    const moved = (event: PointerEvent): void => {
      if (event.pointerType !== "mouse") {
        return;
      }
      const place = `${String(event.screenX)},${String(event.screenY)}`;
      const isMoved = lastPlace !== undefined && place !== lastPlace;
      lastPlace = place;
      if (isMoved) {
        openRef.current();
      }
    };
    const clicked = (event: MouseEvent): void => {
      if (event.detail > 0) {
        openRef.current();
      }
    };
    document.addEventListener("pointermove", moved);
    document.addEventListener("click", clicked);
    return () => {
      document.removeEventListener("pointermove", moved);
      document.removeEventListener("click", clicked);
    };
  }, []);
}

/**
 * Puts the stage the overlay is in full screen as the player opens, where it is asked to (a
 * television's player); where the browser refuses, it fills the window (fillTheScreen).
 */
function useFullscreenStage(insideRef: RefObject<HTMLElement | null>, isAsked: boolean): void {
  useEffect(() => {
    const stage = insideRef.current?.closest(".film-player__stage");
    if (isAsked && stage instanceof HTMLElement) {
      fillTheScreen(stage, document);
    }
  }, [insideRef, isAsked]);
}
