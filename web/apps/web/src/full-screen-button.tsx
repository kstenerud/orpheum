// SPDX-License-Identifier: MPL-2.0

import { useEffect, useRef, useState, type JSX } from "react";
import { useTranslation } from "react-i18next";

import { toggleFullScreen } from "./film-fullscreen.ts";
import { IconButton } from "./icon-button.tsx";
import { FullScreenIcon, LeaveFullScreenIcon } from "./player-icons.tsx";

/**
 * The player bar's full screen, a control a remote moves to and presses like its others
 * (player-controls.tsx): out of full screen it puts the player's stage full screen, from the press
 * the Fullscreen API needs, filling the window where the browser refuses; in full screen it leaves,
 * as the page's own asking, so a cast's receiver keeps its cast (toggleFullScreen). A Fire TV whose player opened out of full screen, its request made with no press, offered no
 * way into it (the owner, 2026-10-27).
 */
export function FullScreenButton(): JSX.Element {
  const { t } = useTranslation();
  const buttonRef = useRef<HTMLButtonElement>(null);
  const isFullScreen = useIsFullScreen();
  return (
    <IconButton
      ref={buttonRef}
      isOverVideo
      className="full-screen-button"
      label={isFullScreen ? t("playerControls.leaveFullScreen") : t("playerControls.fullScreen")}
      icon={isFullScreen ? <LeaveFullScreenIcon /> : <FullScreenIcon />}
      onClick={() => {
        toggleFullScreen(buttonRef.current?.closest(".film-player__stage"), document);
      }}
    />
  );
}

/** Whether the page is full screen now, as the browser says at each change. */
function useIsFullScreen(): boolean {
  const [isFullScreen, setIsFullScreen] = useState(
    // A page drawn with no document, as a test draws it to markup, is not full screen.
    () => typeof document !== "undefined" && document.fullscreenElement instanceof Element,
  );
  useEffect(() => {
    const changed = (): void => {
      setIsFullScreen(document.fullscreenElement instanceof Element);
    };
    document.addEventListener("fullscreenchange", changed);
    return () => {
      document.removeEventListener("fullscreenchange", changed);
    };
  }, []);
  return isFullScreen;
}
