// SPDX-License-Identifier: MPL-2.0

import "./tv-controls.css";

import { useEffect, useRef, useState, type JSX, type RefObject } from "react";
import { useTranslation } from "react-i18next";

import { fillTheScreen } from "./film-fullscreen.ts";
import type { FilmParts } from "./film-player.tsx";
import type { Command } from "./input-buttons.ts";
import { useCommands } from "./input-layer.tsx";
import { answerPlayerCommand, seekStepMs } from "./player-commands.ts";
import { OverlayUpAndBack } from "./play-up.tsx";
import { PlayerControls, useVideoSource } from "./player-controls.tsx";

/**
 * How long the overlay stays after the last press while the film plays, in milliseconds: a TV
 * player's usual few seconds. A justified constant.
 */
export const overlayShownForMs = 5000;

/** What of the overlay shows: nothing, the scrub bar alone after a seek, or all of it. */
export type OverlayShown = "none" | "scrub" | "all";

/** What a press does to the overlay, and whether the overlay takes it from everything else. */
export interface OverlayAnswer {
  readonly shown: OverlayShown;
  readonly isTaken: boolean;
  /** A seek the press asks for, in ms: Left and Right while hidden or on the scrub bar. */
  readonly seekByMs?: number;
  /** Whether focus goes to play, as the whole overlay opens. */
  readonly isFocusToPlay?: boolean;
}

/**
 * What a press does to a television's player overlay, as Jellyfin's does (the owner, Fire TV,
 * 2026-10-25): while hidden or showing the scrub bar alone, Left and Right seek and show the scrub
 * bar, which goes by itself, and OK, Up and Down open the whole overlay with focus on play; while
 * all of it shows, Back hides it, Left and Right on the scrub bar seek, and every other press is
 * the overlay's controls' to move between and press. Play, pause and the seeking keys are the
 * player's everywhere.
 */
export function overlayAnswer(
  shown: OverlayShown,
  command: Command,
  isScrubBarFocused: boolean,
): OverlayAnswer {
  if (command === "playPause" || command === "seekBack" || command === "seekForward") {
    return { shown: shown === "all" ? "all" : "scrub", isTaken: false };
  }
  return shown === "all"
    ? answerWhileAllShows(command, isScrubBarFocused)
    : answerWhileHidden(shown, command);
}

/** A press while the overlay is hidden or shows the scrub bar alone. */
function answerWhileHidden(shown: "none" | "scrub", command: Command): OverlayAnswer {
  const seek = seekOfArrow(command);
  if (seek !== undefined) {
    return { shown: "scrub", isTaken: true, seekByMs: seek };
  }
  if (command === "select" || command === "up" || command === "down") {
    return { shown: "all", isTaken: true, isFocusToPlay: true };
  }
  return { shown: "none", isTaken: shown === "scrub" && command === "back" };
}

/** A press while all of the overlay shows. */
function answerWhileAllShows(command: Command, isScrubBarFocused: boolean): OverlayAnswer {
  if (command === "back") {
    return { shown: "none", isTaken: true };
  }
  const seek = seekOfArrow(command);
  if (seek !== undefined && isScrubBarFocused) {
    return { shown: "all", isTaken: true, seekByMs: seek };
  }
  return { shown: "all", isTaken: false };
}

/** The seek an arrow asks for on the scrub bar, in ms, or undefined for any other command. */
function seekOfArrow(command: Command): number | undefined {
  if (command === "left") {
    return -seekStepMs;
  }
  return command === "right" ? seekStepMs : undefined;
}

/**
 * A television's player overlay (tv-device.ts), over the film inside its stage so it stays in full
 * screen: play or pause, the scrub bar, where the film stands and when it ends. The browser's own
 * controls are in a shadow no remote reaches (the owner, Fire TV, 2026-10-25: "none of the controls
 * on the video player are selectable from the remote"). As the player opens it puts its stage full
 * screen, as a television's player is. A mouse's move, a click or a tap opens it too, as a desktop
 * player's controls do, so a cast's receiver on a computer keeps its controls (D338 §5).
 */
export function TvControls(props: FilmParts): JSX.Element {
  const { videoRef } = props;
  const playRef = useRef<HTMLButtonElement>(null);
  const scrubRef = useRef<HTMLInputElement>(null);
  const overlayRef = useRef<HTMLDivElement>(null);
  const source = useVideoSource(videoRef);
  const [shown, setShown] = useState<OverlayShown>("all");
  const notePress = useHiding({ isPaused: source.isPaused, shown, setShown, overlayRef, videoRef });
  useOpensAtThePointer(() => {
    setShown("all");
    notePress();
  });
  useFullscreenStage(playRef);
  useCommands(
    "document",
    (detail, event) => {
      if (detail.command === "select" && isOnAnotherControlOverTheFilm(overlayRef.current)) {
        return; // OK on what else the page draws over the film, its Up next, presses it
      }
      const isOnScrub = document.activeElement === scrubRef.current;
      const answer = overlayAnswer(shown, detail.command, isOnScrub);
      setShown(answer.shown);
      notePress();
      const video = videoRef.current;
      if (answer.seekByMs !== undefined && video !== null) {
        answerPlayerCommand(video, answer.seekByMs < 0 ? "seekBack" : "seekForward");
      }
      if (answer.isFocusToPlay === true) {
        requestAnimationFrame(() => playRef.current?.focus());
      }
      if (answer.isTaken) {
        event.preventDefault();
      }
    },
    { isCapture: true },
  );
  return (
    <div
      ref={overlayRef}
      className="tv-controls"
      data-shown={shown}
      data-focus-container=""
      data-player-controls=""
    >
      <OverlayUpAndBack />
      <PlayerControls
        source={source}
        controls={["play", "fullScreen", "timeline"]}
        isOverVideo
        playRef={playRef}
        timelineRef={scrubRef}
        hasArrowsAsCommands
        aside={
          <EndsAt leftMs={source.durationMs === 0 ? undefined : source.durationMs - source.atMs} />
        }
      />
    </div>
  );
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

/** When the film ends if it plays on from now, as the viewer's clock says it. */
function endsAt(leftMs: number): string {
  return new Date(Date.now() + leftMs).toLocaleTimeString([], {
    hour: "numeric",
    minute: "2-digit",
  });
}

/**
 * Hides the overlay overlayShownForMs after the last press while the film plays, the whole overlay
 * never while it is paused; as it hides, focus in the overlay moves to the video, so no hidden
 * control holds it, and what else over the film has focus, Up next, keeps it. Returns what notes a
 * press, which renews the time.
 */
function useHiding(hiding: {
  readonly isPaused: boolean;
  readonly shown: OverlayShown;
  readonly setShown: (shown: OverlayShown) => void;
  readonly overlayRef: RefObject<HTMLElement | null>;
  readonly videoRef: RefObject<HTMLVideoElement | null>;
}): () => void {
  const { isPaused, shown, setShown, overlayRef, videoRef } = hiding;
  const lastPressAtRef = useRef(0);
  useEffect(() => {
    if (shown === "none" || (isPaused && shown === "all")) {
      return undefined;
    }
    const timer = setInterval(() => {
      if (performance.now() - lastPressAtRef.current < overlayShownForMs) {
        return;
      }
      setShown("none");
      const video = videoRef.current;
      if (video !== null && overlayRef.current?.contains(document.activeElement) === true) {
        video.tabIndex = -1;
        video.focus();
      }
    }, 500);
    return () => {
      clearInterval(timer);
    };
  }, [isPaused, shown, setShown, overlayRef, videoRef]);
  return () => {
    lastPressAtRef.current = performance.now();
  };
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
 * Puts the stage the overlay is in full screen as the player opens, as a television's player is;
 * where the browser refuses, it fills the window (fillTheScreen).
 */
function useFullscreenStage(insideRef: RefObject<HTMLElement | null>): void {
  useEffect(() => {
    const stage = insideRef.current?.closest(".film-player__stage");
    if (stage instanceof HTMLElement) {
      fillTheScreen(stage, document);
    }
  }, [insideRef]);
}
