// SPDX-License-Identifier: MPL-2.0

import { useEffect, useRef, useState, type JSX } from "react";
import { useTranslation } from "react-i18next";

import { fillTheScreen, leaveFullScreenAsked } from "./film-fullscreen.ts";
import { IconButton } from "./icon-button.tsx";
import { FullScreenIcon, LeaveFullScreenIcon } from "./player-icons.tsx";

/**
 * The television player overlay's full screen, a control a remote moves to and presses like its
 * others (tv-controls.tsx): out of full screen it puts the player's stage full screen, from the press
 * the Fullscreen API needs, filling the window where the browser refuses (fillTheScreen); in full
 * screen it leaves, as the page's own asking (leaveFullScreenAsked), so a cast's receiver keeps its
 * cast. A Fire TV whose player opened out of full screen, its request made with no press, offered no
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
      label={isFullScreen ? t("tvControls.leaveFullScreen") : t("tvControls.fullScreen")}
      icon={isFullScreen ? <LeaveFullScreenIcon /> : <FullScreenIcon />}
      onClick={() => {
        if (document.fullscreenElement instanceof Element) {
          leaveFullScreenAsked(document);
          return;
        }
        const stage = buttonRef.current?.closest(".film-player__stage");
        if (stage instanceof HTMLElement) {
          fillTheScreen(stage, document);
        }
      }}
    />
  );
}

/** Whether the page is full screen now, as the browser says at each change. */
function useIsFullScreen(): boolean {
  const [isFullScreen, setIsFullScreen] = useState(
    () => document.fullscreenElement instanceof Element,
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
