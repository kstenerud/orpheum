// SPDX-License-Identifier: MPL-2.0

import { useRef, useSyncExternalStore, type JSX, type RefObject } from "react";
import { useTranslation } from "react-i18next";

import {
  followFillsWindow,
  isAStageFillingTheWindow,
  toggleFullScreen,
} from "./film-fullscreen.ts";
import { IconButton } from "./icon-button.tsx";
import { FullScreenIcon, LeaveFullScreenIcon } from "./player-icons.tsx";

/**
 * The player bar's full screen, a control a remote moves to and presses like its others
 * (player-controls.tsx): out of full screen it puts the player's stage full screen, from the press
 * the Fullscreen API needs, filling the window where the browser refuses; in full screen it leaves,
 * as the page's own asking, so a cast's receiver keeps its cast (toggleFullScreen). A Fire TV whose player opened out of full screen, its request made with no press, offered no
 * way into it (the owner, 2026-10-27). The stage is the one videoRef's video is in, as the bar under
 * the film lies outside it (D407 §9); with none given, the one the button is in.
 */
export function FullScreenButton(props: {
  readonly videoRef?: RefObject<HTMLVideoElement | null> | undefined;
}): JSX.Element {
  const { t } = useTranslation();
  const buttonRef = useRef<HTMLButtonElement>(null);
  const isFullScreen = useIsBrowserFullScreen();
  return (
    <IconButton
      ref={buttonRef}
      isOverVideo
      className="full-screen-button"
      label={isFullScreen ? t("playerControls.leaveFullScreen") : t("playerControls.fullScreen")}
      icon={isFullScreen ? <LeaveFullScreenIcon /> : <FullScreenIcon />}
      onClick={() => {
        const inStage = props.videoRef?.current ?? buttonRef.current;
        toggleFullScreen(inStage?.closest(".film-player__stage"), document);
      }}
    />
  );
}

/** Follows the browser's full screen and a stage's filling the window; returns the stopping. */
function followFullScreen(changed: () => void): () => void {
  document.addEventListener("fullscreenchange", changed);
  const stopFollowingFill = followFillsWindow(changed);
  return () => {
    document.removeEventListener("fullscreenchange", changed);
    stopFollowingFill();
  };
}

/** Whether the browser has put something on the page full screen. */
function isBrowserFullScreen(): boolean {
  return document.fullscreenElement instanceof Element;
}

/** Whether the film shows full screen: the browser's, or a stage filling the window it refused. */
function isShownFullScreen(): boolean {
  return isBrowserFullScreen() || isAStageFillingTheWindow(document);
}

/** A page drawn with no document, as a test draws it to markup, is not full screen. */
function isNotFullScreen(): boolean {
  return false;
}

/**
 * Whether the film shows full screen now: the browser's full screen, or a player's stage filling the
 * window where the browser refused it (fillTheScreen), which looks the same (plans/player-overlay.md
 * §2.1, D407).
 */
export function useIsFullScreen(): boolean {
  return useSyncExternalStore(followFullScreen, isShownFullScreen, isNotFullScreen);
}

/**
 * Whether the browser's full screen is on now, which the button's press leaves; a stage filling the
 * window is not, as its press asks for full screen (toggleFullScreen).
 */
function useIsBrowserFullScreen(): boolean {
  return useSyncExternalStore(followFullScreen, isBrowserFullScreen, isNotFullScreen);
}
