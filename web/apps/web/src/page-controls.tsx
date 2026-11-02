// SPDX-License-Identifier: MPL-2.0

import "./page-controls.css";

import { useEffect, useRef, useState, type JSX, type RefObject } from "react";

import { pressOf } from "./cast-asked.tsx";
import type { OfferPiece } from "./convenience-area.tsx";
import { toggleFullScreen } from "./film-fullscreen.ts";
import type { FilmParts } from "./film-player.tsx";
import { useIsFullScreen } from "./full-screen-button.tsx";
import { useCommands } from "./input-layer.tsx";
import { answerPlayerCommand } from "./player-commands.ts";
import { PlayerControls, useVideoSource, type SequenceMoves } from "./player-controls.tsx";
import { PlayerOverlay, useIsFocusIn, useOpensAtThePointer } from "./player-overlay.tsx";
import { fullShownForMs } from "./player-overlay-state.ts";

/**
 * A computer's and a phone's player controls (D369 §2), in place of the browser's own: in full
 * screen, the browser's or the stage filling the window (useIsFullScreen), the player's overlay, as
 * a television's (D407 §1, §5); otherwise the bar (PageBar). The switch is made here, in the Play
 * page's own piece, not in the film player's stage, which Watch Together's guest shares (the plan's
 * §6 audit). A mouse's click on the film plays or pauses it, and a double click puts it full screen
 * or leaves (the two clicks' play and pause undoing each other), in either. The offer, Up next or
 * Still watching, is the overlay's convenience area's in full screen (D407 §3), and beside the bar
 * otherwise, until the controls move below the film (D407 §9, plans/player-overlay.md step 11).
 */
export function PageControls(
  props: FilmParts & {
    readonly offer?: OfferPiece | undefined;
    /** What the overlay's previous and next play (D407 §2). */
    readonly moves?: SequenceMoves | undefined;
  },
): JSX.Element {
  const { offer: Offer, moves, ...film } = props;
  const isFullScreen = useIsFullScreen();
  usePressesOnTheFilm(film.videoRef);
  if (isFullScreen) {
    return <PlayerOverlay {...film} offer={Offer} moves={moves} />;
  }
  return (
    <>
      <PageBar {...film} />
      {Offer === undefined ? null : <Offer {...film} />}
    </>
  );
}

/**
 * The player's bar outside full screen, along the film's bottom, inside its stage so it stays in full screen, with play or pause,
 * sound, full screen and the timeline. A mouse's move, a tap or a key in the player shows it, and
 * stillness hides it after fullShownForMs while the film plays, as the receiver's does; a key's
 * focus in it keeps it in view. The video itself is a stop for the arrows and Tab, where Left and
 * Right seek and OK and Space play and pause (player-commands.ts), and Down goes into the bar, to
 * play, as a television's overlay opens (the bar lies over the video, where no arrow's search from
 * it looks); a tap shows the bar.
 */
function PageBar(props: FilmParts): JSX.Element {
  const { videoRef } = props;
  const overlayRef = useRef<HTMLDivElement>(null);
  const playRef = useRef<HTMLButtonElement>(null);
  const [isShown, setIsShown] = useState(true);
  // A key's focus in the bar keeps it in view though hidden (page-controls.css), so it reads on.
  const isFocusIn = useIsFocusIn(overlayRef);
  const source = useVideoSource(videoRef, !isShown && !isFocusIn);
  useShownWhileActive({ isShown, setIsShown }, source.isPaused, overlayRef);
  useCommands(
    "document",
    (detail, event) => {
      if (detail.command === "down" && document.activeElement === videoRef.current) {
        event.preventDefault();
        playRef.current?.focus();
      }
    },
    { isCapture: true },
  );
  return (
    <div
      ref={overlayRef}
      className="page-controls"
      data-shown={isShown ? "all" : "none"}
      data-focus-container=""
      data-player-controls=""
    >
      <PlayerControls
        source={source}
        controls={["play", "sound", "fullScreen", "timeline"]}
        isOverVideo
        playRef={playRef}
        hasArrowsAsCommands
      />
    </div>
  );
}

/**
 * Whether the bar shows, and what shows it: shown as the player opens and while the film is paused,
 * it hides fullShownForMs after the last sign of someone there, a mouse's move, a click or a tap
 * anywhere (useOpensAtThePointer), a command while focus is in the player, or focus coming into the
 * bar. Hiding leaves focus where it is.
 */
function useShownWhileActive(
  shown: { readonly isShown: boolean; readonly setIsShown: (isShown: boolean) => void },
  isPaused: boolean,
  overlayRef: RefObject<HTMLElement | null>,
): void {
  const { isShown, setIsShown } = shown;
  const lastSeenAtRef = useRef(0);
  const show = (): void => {
    lastSeenAtRef.current = performance.now();
    setIsShown(true);
  };
  useOpensAtThePointer(show);
  const showRef = useRef(show);
  useEffect(() => {
    showRef.current = show;
  });
  useEffect(() => {
    const overlay = overlayRef.current;
    const focused = (): void => {
      showRef.current();
    };
    overlay?.addEventListener("focusin", focused);
    return () => {
      overlay?.removeEventListener("focusin", focused);
    };
  }, [overlayRef]);
  useCommands(
    "document",
    () => {
      const stage = overlayRef.current?.parentElement;
      if (stage?.contains(document.activeElement) === true) {
        show();
      }
    },
    { isCapture: true },
  );
  useEffect(() => {
    if (!isShown || isPaused) {
      return undefined;
    }
    const timer = setInterval(() => {
      if (performance.now() - lastSeenAtRef.current >= fullShownForMs) {
        setIsShown(false);
      }
    }, 500);
    return () => {
      clearInterval(timer);
    };
  }, [isShown, isPaused, setIsShown]);
}

/**
 * Makes the video a stop for the arrows and Tab, as the browser's own controls made it, and answers
 * a mouse's click on it by playing or pausing, a double click by full screen.
 */
function usePressesOnTheFilm(videoRef: RefObject<HTMLVideoElement | null>): void {
  useEffect(() => {
    const video = videoRef.current;
    if (video === null) {
      return undefined;
    }
    video.tabIndex = 0;
    const clicked = (event: MouseEvent): void => {
      if (pressOf(event) === "click") {
        answerPlayerCommand(video, "playPause");
      }
    };
    const doubleClicked = (): void => {
      toggleFullScreen(video.closest(".film-player__stage"), document);
    };
    video.addEventListener("click", clicked);
    video.addEventListener("dblclick", doubleClicked);
    return () => {
      video.removeEventListener("click", clicked);
      video.removeEventListener("dblclick", doubleClicked);
    };
  }, [videoRef]);
}
