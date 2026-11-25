// SPDX-License-Identifier: MPL-2.0

import { useLayoutEffect, useRef, useState, type JSX } from "react";

import {
  followFillsWindow,
  isAStageFillingTheWindow,
  keepFillingTheWindow,
} from "./film-fullscreen.ts";
import { useLatest } from "./latest.ts";

/**
 * Whether a film filled the window as the Play page opened: the page before it was a Play page too,
 * which Next, Previous and Up next draw again for the other file. Read as the page is first drawn,
 * while the page it replaces is still on screen, and kept.
 */
export function useIsOpenedOverAFilm(): boolean {
  const [isOver] = useState(() => isAStageFillingTheWindow(document));
  return isOver;
}

/**
 * Keeps the window covered, black, as one Play page moves on to another's file, until the new page's
 * own film fills it (keepFillingTheWindow): the page drawn again has no stage until its playback
 * answers, and the shell's banner under it showed for that moment (the owner, 2026-11-14, VQ267 on
 * the Android phone: "whenever it switches to the next or previous video I see a brief flash of the
 * orpheum top bar"). The page's own stage fills the window once drawn, the page full screen still
 * (useWholeWindow, D555 §9), which lets this go; so does a playback that failed (isFailed), the
 * page then showing why.
 */
export function WindowHeldForTheFilm(props: { readonly isFailed: boolean }): JSX.Element | null {
  const [isLetGo, setIsLetGo] = useState(false);
  if (isLetGo || props.isFailed) {
    return null;
  }
  return (
    <StandInStage
      onTaken={() => {
        setIsLetGo(true);
      }}
    />
  );
}

/**
 * A black stage filling the window in the film's place, from the frame the page it replaces goes in,
 * until another stage fills the window (onTaken).
 */
function StandInStage(props: { readonly onTaken: () => void }): JSX.Element {
  const standInRef = useRef<HTMLDivElement>(null);
  const onTakenRef = useLatest(props.onTaken);
  useLayoutEffect(() => {
    const standIn = standInRef.current;
    if (standIn === null) {
      return undefined;
    }
    const letGo = keepFillingTheWindow(standIn);
    const stopFollowing = followFillsWindow(() => {
      if (isAStageFillingTheWindow(document, standIn)) {
        onTakenRef.current();
      }
    });
    return () => {
      stopFollowing();
      letGo();
    };
  }, [onTakenRef]);
  return <div ref={standInRef} className="film-player__stage" aria-hidden="true" />;
}
