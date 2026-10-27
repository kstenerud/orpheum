// SPDX-License-Identifier: MPL-2.0

import "./tv-controls.css";

import { useEffect, useRef, useState, type JSX, type RefObject } from "react";
import { seekTo } from "@orpheum/player";
import { useTranslation } from "react-i18next";

import { fillTheScreen } from "./film-fullscreen.ts";
import type { FilmParts } from "./film-player.tsx";
import { FullScreenButton } from "./full-screen-button.tsx";
import type { Command } from "./input-buttons.ts";
import { useCommands } from "./input-layer.tsx";
import { answerPlayerCommand, seekStepMs } from "./player-commands.ts";

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
  const stand = useStanding(videoRef);
  const [shown, setShown] = useState<OverlayShown>("all");
  const notePress = useHiding({ isPaused: stand.isPaused, shown, setShown, overlayRef, videoRef });
  useOpensAtThePointer(setShown, notePress);
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
      <div className="tv-controls__buttons">
        <PlayButton playRef={playRef} videoRef={videoRef} isPaused={stand.isPaused} />
        <EndsAt leftMs={stand.durationMs === 0 ? undefined : stand.durationMs - stand.atMs} />
        <FullScreenButton />
      </div>
      <ScrubBar
        scrubRef={scrubRef}
        atMs={stand.atMs}
        durationMs={stand.durationMs}
        onSeekTo={(ms) => {
          const video = videoRef.current;
          if (video !== null) {
            seekTo(video, ms);
          }
        }}
      />
    </div>
  );
}

/** When the film ends if it plays on from now, leftMs from its end; nothing while its length is not known. */
function EndsAt(props: { readonly leftMs: number | undefined }): JSX.Element {
  const { t } = useTranslation();
  const { leftMs } = props;
  return (
    <span className="tv-controls__ends">
      {leftMs === undefined ? "" : t("tvControls.endsAt", { time: endsAt(leftMs) })}
    </span>
  );
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

/** Play or pause, the overlay's first control, which focus goes to as the overlay opens. */
function PlayButton(props: {
  readonly playRef: RefObject<HTMLButtonElement | null>;
  readonly videoRef: RefObject<HTMLVideoElement | null>;
  readonly isPaused: boolean;
}): JSX.Element {
  const { t } = useTranslation();
  const { playRef, videoRef, isPaused } = props;
  return (
    <button
      ref={playRef}
      type="button"
      className="tv-controls__play"
      aria-label={isPaused ? t("tvControls.play") : t("tvControls.pause")}
      onClick={() => {
        const video = videoRef.current;
        if (video !== null) {
          answerPlayerCommand(video, "playPause");
        }
      }}
    >
      <span aria-hidden="true">{isPaused ? "▶" : "❚❚"}</span>
    </button>
  );
}

/**
 * The scrub bar, a control a remote moves to, whose Left and Right are commands the overlay answers
 * by seeking (arrowsAreCommandsAttribute), its own steps prevented with the key, and the film's
 * times. A range keeps its arrows from the input layer by default, and its own keydown's
 * preventDefault had the layer leave them alone too, so they did nothing (the owner's Fire TV,
 * 2026-10-27).
 */
function ScrubBar(props: {
  readonly scrubRef: RefObject<HTMLInputElement | null>;
  readonly atMs: number;
  readonly durationMs: number;
  readonly onSeekTo: (ms: number) => void;
}): JSX.Element {
  const { t } = useTranslation();
  const { scrubRef, atMs, durationMs, onSeekTo } = props;
  return (
    <div className="tv-controls__scrub-row">
      <input
        ref={scrubRef}
        type="range"
        className="tv-controls__scrub"
        min={0}
        max={Math.max(Math.round(durationMs / 1000), 1)}
        step={seekStepMs / 1000}
        value={Math.round(atMs / 1000)}
        aria-label={t("tvControls.where")}
        aria-valuetext={clockText(atMs)}
        data-focus-arrows-are-commands=""
        onChange={(event) => {
          onSeekTo(Number(event.currentTarget.value) * 1000);
        }}
      />
      <span className="tv-controls__time">
        {clockText(atMs)} / {clockText(durationMs)}
      </span>
    </div>
  );
}

/** A length of film as a player shows it: m:ss under an hour, h:mm:ss from one. */
export function clockText(ms: number): string {
  const seconds = Math.max(0, Math.floor(ms / 1000));
  const hours = Math.floor(seconds / 3600);
  const minutes = Math.floor((seconds % 3600) / 60);
  const rest = String(seconds % 60).padStart(2, "0");
  return hours === 0
    ? `${String(minutes)}:${rest}`
    : `${String(hours)}:${String(minutes).padStart(2, "0")}:${rest}`;
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
 * Opens the whole overlay at a mouse's move, or a click or a tap anywhere on the page, as a desktop
 * player shows its controls, and hides it as after a press (useHiding): a cast's receiver on a
 * computer showed no controls to its mouse (the owner, 2026-10-27). A move that did not change the
 * pointer's place is none (a browser may move it by itself as what lies under it changes), and a
 * click a key made (no count) is the key's, which the overlay answers as a press already. Focus
 * stays where it is.
 */
function useOpensAtThePointer(
  setShown: (shown: OverlayShown) => void,
  notePress: () => void,
): void {
  const openRef = useRef(notePress);
  useEffect(() => {
    openRef.current = () => {
      setShown("all");
      notePress();
    };
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

/** Where the film stands, its length, and whether it is paused, as the video says them. */
function useStanding(videoRef: RefObject<HTMLVideoElement | null>): {
  readonly atMs: number;
  readonly durationMs: number;
  readonly isPaused: boolean;
} {
  const [stand, setStand] = useState({ atMs: 0, durationMs: 0, isPaused: true });
  useEffect(() => {
    const video = videoRef.current;
    if (video === null) {
      return undefined;
    }
    const heard = (): void => {
      setStand({
        atMs: video.currentTime * 1000,
        durationMs: Number.isFinite(video.duration) ? video.duration * 1000 : 0,
        isPaused: video.paused,
      });
    };
    // Read at the video's own events, the first as soon as it knows its length.
    const events = ["loadedmetadata", "timeupdate", "durationchange", "play", "pause", "seeked"];
    for (const each of events) {
      video.addEventListener(each, heard);
    }
    return () => {
      for (const each of events) {
        video.removeEventListener(each, heard);
      }
    };
  }, [videoRef]);
  return stand;
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
