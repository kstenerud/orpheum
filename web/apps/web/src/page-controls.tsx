// SPDX-License-Identifier: MPL-2.0

import "./page-controls.css";

import type { SkippedStretch } from "@orpheum/player";
import { useEffect, useRef, useState, type JSX, type RefObject } from "react";
import { useTranslation } from "react-i18next";

import { pressOf } from "./cast-start.tsx";
import { ConvenienceArea, type OfferPiece } from "./convenience-area.tsx";
import { fillTheScreen, toggleFullScreen } from "./film-fullscreen.ts";
import { keepThePressForTheStart } from "./start-refused.ts";
import type { FilmParts } from "./film-player.tsx";
import { useIsFullScreen } from "./full-screen-button.tsx";
import { useCommands } from "./input-layer.tsx";
import { OverlayUp } from "./play-up.tsx";
import { answerPlayerCommand, usePlayersControls } from "./player-commands.ts";
import { PlayerControls, useVideoSource, type SequenceMoves } from "./player-controls.tsx";
import { PlayerMenus } from "./player-menus.tsx";
import { barControlsOf, PlayerOverlay, useControlsShown } from "./player-overlay.tsx";
import { SkippedPopUp, SkippedSign, useSkippedStretches } from "./skipped-parts.tsx";
import { Timeline } from "./timeline.tsx";
import { isTelevision } from "./tv-device.ts";

/** What the player's controls draw beside the film's own parts: the offer, previous and next. */
interface ControlsGiven {
  /** The offer, Up next or Still watching (convenience-area.tsx). */
  readonly offer?: OfferPiece | undefined;
  /** What previous and next play, each shown only where given (D407 §2). */
  readonly moves?: SequenceMoves | undefined;
}

/**
 * The player's own controls over the film, every device's (D369 §2): the full-screen overlay while
 * the film shows full screen, the browser's or the stage filling the window (useIsFullScreen; D407
 * §1, §5), and nothing otherwise, the controls then lying under the film (PageControls, D407 §9).
 * shouldOpenFullScreen puts the stage full screen as the player opens, as a television's player is,
 * filling the window where the browser refuses; hasPressesOnTheFilm has a mouse's click on the film
 * play or pause it and a double click put it full screen or leave (the two clicks' play and pause
 * undoing each other). Over the film too, in and out of full screen, the pop-up that tells a long
 * stretch skipped (SkippedPopUp). The switch is made in the Play page's pieces, not in the film
 * player's stage, which Watch Together's guest shares (the plan's §6 audit).
 */
export function OverTheFilm(
  props: FilmParts &
    ControlsGiven & {
      readonly shouldOpenFullScreen?: boolean | undefined;
      readonly hasPressesOnTheFilm?: boolean | undefined;
    },
): JSX.Element | null {
  const { shouldOpenFullScreen, hasPressesOnTheFilm, offer, moves, ...film } = props;
  const isFullScreen = useIsFullScreen();
  useFullscreenStage(film.videoRef, shouldOpenFullScreen === true);
  usePressesOnTheFilm(film.videoRef, hasPressesOnTheFilm === true);
  return (
    <>
      {isFullScreen ? <PlayerOverlay {...film} offer={offer} moves={moves} /> : null}
      <SkippedPopUp videoRef={film.videoRef} />
    </>
  );
}

/**
 * The player's controls under the film, outside full screen on every device, a television's too
 * (D407 §9): nothing is drawn over the video then. As full screen ends unasked (a Fire TV's Back, a
 * computer's Escape, which the page never hears), focus goes to play, from where ⤢ returns and the
 * next Back goes to the film's page (D407 §7). In full screen, nothing: the overlay is the controls.
 * The stretches the player skipped are heard here, drawn or not, so the bar's sign names each.
 */
export function PageControls(props: FilmParts & ControlsGiven): JSX.Element | null {
  const { offer, moves, ...film } = props;
  const isFullScreen = useIsFullScreen();
  const playRef = useRef<HTMLButtonElement>(null);
  usePlayFocusedAsFullScreenEnds(isFullScreen, playRef);
  const skipped = useSkippedStretches(film.videoRef);
  return isFullScreen ? null : (
    <ControlsBar film={film} offer={offer} moves={moves} playRef={playRef} skipped={skipped} />
  );
}

/** Focuses play in playRef as full screen ends, once the controls under the film are drawn. */
function usePlayFocusedAsFullScreenEnds(
  isFullScreen: boolean,
  playRef: RefObject<HTMLButtonElement | null>,
): void {
  const wasFullScreenRef = useRef(isFullScreen);
  useEffect(() => {
    if (wasFullScreenRef.current && !isFullScreen) {
      playRef.current?.focus();
    }
    wasFullScreenRef.current = isFullScreen;
  }, [isFullScreen, playRef]);
}

/** A menu of the bar's opening or closing holds nothing: the bar under the film never hides. */
const noMenuHeld = (): void => undefined;

/**
 * The bar under the film (D407 §9): the timeline; the buttons, those the device shows
 * (controlsShown), ⏮ ⏪ ▶ ⏩ ⏭ leading and 🔈 ⤢ trailing; then the offers as ordinary buttons, Up next
 * or Still watching, or when the film ends while nothing is on offer (convenience-area.tsx), after
 * a television's ‹, its one way up (PlayTrail draws none there). Always shown. The subtitle and
 * audio menus and the rest stay under it on the page, as before D407 (play.tsx). The video is a
 * stop for the arrows and Tab, where Left and Right seek and OK and Space play and pause
 * (player-commands.ts), and Down goes into the bar, to play.
 */
function ControlsBar(props: {
  readonly film: FilmParts;
  readonly offer: OfferPiece | undefined;
  readonly moves: SequenceMoves | undefined;
  readonly playRef: RefObject<HTMLButtonElement | null>;
  /** The stretches the player stepped over in this viewing, told by a sign in the bar. */
  readonly skipped: readonly SkippedStretch[];
}): JSX.Element {
  const { t } = useTranslation();
  const { film, offer, moves, playRef, skipped } = props;
  const { videoRef } = film;
  const barRef = useRef<HTMLDivElement>(null);
  const source = useVideoSource(videoRef);
  const isTv = isTelevision(navigator.userAgent);
  const shown = useControlsShown(isTv, moves);
  usePlayersControls(barRef, videoRef);
  useDownFromTheFilm(videoRef, playRef);
  return (
    <div ref={barRef} className="page-controls" data-focus-container="" data-player-controls="">
      <Timeline
        label={t("playerControls.where")}
        atMs={source.atMs}
        durationMs={source.durationMs}
        isDisabled={!source.isReady}
        hasArrowsAsCommands
        onSeek={source.seekTo}
      />
      <PlayerControls
        source={source}
        controls={barControlsOf(shown)}
        isOverVideo
        isOverlayRow
        playRef={playRef}
        moves={moves}
        videoRef={videoRef}
        menus={
          <PlayerMenus
            videoRef={videoRef}
            shown={shown.filter((control) => control === "sound")}
            sound={source.sound}
            onOpenChange={noMenuHeld}
          />
        }
      />
      <BarOffers
        film={film}
        offer={offer}
        skipped={skipped}
        isTv={isTv}
        leftMs={source.durationMs === 0 ? undefined : source.durationMs - source.atMs}
      />
    </div>
  );
}

/**
 * The bar's end: the warning sign of what was skipped (SkippedSign), a television's ‹, and the
 * offers as ordinary buttons (ConvenienceArea).
 */
function BarOffers(props: {
  readonly film: FilmParts;
  readonly offer: OfferPiece | undefined;
  readonly skipped: readonly SkippedStretch[];
  readonly isTv: boolean;
  /** How long the film has left to play, undefined while its length is not known. */
  readonly leftMs: number | undefined;
}): JSX.Element {
  const areaRef = useRef<HTMLDivElement>(null);
  const [hasOffer, setHasOffer] = useState(false);
  return (
    <div className="page-controls__offers">
      <SkippedSign stretches={props.skipped} />
      {props.isTv ? <OverlayUp /> : null}
      {/* Back is left to the page, which it leaves for the film's (D407 §7). */}
      <ConvenienceArea
        film={props.film}
        offer={props.offer}
        hasOffer={hasOffer}
        isOverlayFull
        onOffered={setHasOffer}
        leftMs={props.leftMs}
        areaRef={areaRef}
      />
    </div>
  );
}

/** Takes Down on the video into the bar, to play, as a television's overlay opens on play. */
function useDownFromTheFilm(
  videoRef: RefObject<HTMLVideoElement | null>,
  playRef: RefObject<HTMLButtonElement | null>,
): void {
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
}

/**
 * Makes the video a stop for the arrows and Tab, as the browser's own controls made it, and answers
 * a mouse's click on it by playing or pausing, a double click by full screen, where isOn.
 */
function usePressesOnTheFilm(videoRef: RefObject<HTMLVideoElement | null>, isOn: boolean): void {
  useEffect(() => {
    const video = videoRef.current;
    if (video === null || !isOn) {
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
  }, [videoRef, isOn]);
}

/**
 * Puts the stage the film's video is in full screen as the player opens, where it is asked to (a
 * television's player), the video first keeping the press for its start (keepThePressForTheStart);
 * where the browser refuses, it fills the window (fillTheScreen).
 */
function useFullscreenStage(videoRef: RefObject<HTMLVideoElement | null>, isAsked: boolean): void {
  useEffect(() => {
    const video = videoRef.current;
    const stage = video?.closest(".film-player__stage");
    if (isAsked && video !== null && stage instanceof HTMLElement) {
      keepThePressForTheStart(video);
      fillTheScreen(stage, document);
    }
  }, [videoRef, isAsked]);
}
