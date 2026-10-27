// SPDX-License-Identifier: MPL-2.0

import "./page-controls.css";

import { useEffect, useRef, useState, type JSX, type RefObject } from "react";

import { pressOf } from "./cast-asked.tsx";
import { toggleFullScreen } from "./film-fullscreen.ts";
import type { FilmParts } from "./film-player.tsx";
import { useCommands } from "./input-layer.tsx";
import { answerPlayerCommand } from "./player-commands.ts";
import { PlayerControls, useVideoSource } from "./player-controls.tsx";
import { overlayShownForMs, useOpensAtThePointer } from "./tv-controls.tsx";

/**
 * A computer's and a phone's player controls (D369 §2), in place of the browser's own: the player's
 * bar along the film's bottom, inside its stage so it stays in full screen, with play or pause,
 * sound, full screen and the timeline. A mouse's move, a tap or a key in the player shows it, and
 * stillness hides it after overlayShownForMs while the film plays, as the receiver's does; a key's
 * focus in it keeps it in view. The video itself is a stop for the arrows and Tab, where Left and
 * Right seek and OK and Space play and pause (player-commands.ts), and Down goes into the bar, to
 * play, as a television's overlay opens (the bar lies over the video, where no arrow's search from
 * it looks). A mouse's click on the film plays or pauses it, and a double click puts it full screen
 * or leaves (the two clicks' play and pause undoing each other); a tap shows the bar.
 */
export function PageControls(props: FilmParts): JSX.Element {
  const { videoRef } = props;
  const overlayRef = useRef<HTMLDivElement>(null);
  const playRef = useRef<HTMLButtonElement>(null);
  const source = useVideoSource(videoRef);
  const isShown = useShownWhileActive(source.isPaused, overlayRef);
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
  usePressesOnTheFilm(videoRef);
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
 * it hides overlayShownForMs after the last sign of someone there, a mouse's move, a click or a tap
 * anywhere (useOpensAtThePointer), a command while focus is in the player, or focus coming into the
 * bar. Hiding leaves focus where it is.
 */
function useShownWhileActive(
  isPaused: boolean,
  overlayRef: RefObject<HTMLElement | null>,
): boolean {
  const [isShown, setIsShown] = useState(true);
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
      if (performance.now() - lastSeenAtRef.current >= overlayShownForMs) {
        setIsShown(false);
      }
    }, 500);
    return () => {
      clearInterval(timer);
    };
  }, [isShown, isPaused]);
  return isShown;
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
