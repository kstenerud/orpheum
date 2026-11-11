// SPDX-License-Identifier: MPL-2.0

import { useEffect, useState, type JSX, type RefObject } from "react";
import { useTranslation } from "react-i18next";

import { useLatest } from "./latest.ts";
import { startRefusedEvent } from "./start-refused.ts";

/**
 * Whether the browser refused to start the film (startRefusedEvent) and it has not played since. As
 * it refuses, the overlay is shown whole (onRefused), the paused film holding it shown (hideAfter in player-overlay-state.ts),
 * and focus goes to Play once it is drawn, so one press starts the film: Chrome on an iPhone refuses
 * a start asked more than a moment after the press that opened the Play page, and the page showed
 * black, the controls hidden (the owner, staging, 2026-11-11: "In Chrome it went black again").
 */
export function useStartRefused(
  refs: {
    readonly videoRef: RefObject<HTMLVideoElement | null>;
    readonly playRef: RefObject<HTMLButtonElement | null>;
  },
  onRefused: () => void,
): boolean {
  const { videoRef, playRef } = refs;
  const [refusals, setRefusals] = useState(0);
  const [isRefused, setIsRefused] = useState(false);
  const onRefusedRef = useLatest(onRefused);
  useEffect(() => {
    const video = videoRef.current;
    if (video === null) {
      return undefined;
    }
    const refused = (): void => {
      onRefusedRef.current();
      setIsRefused(true);
      setRefusals((count) => count + 1);
    };
    const played = (): void => {
      setIsRefused(false);
    };
    video.addEventListener(startRefusedEvent, refused);
    video.addEventListener("play", played);
    return () => {
      video.removeEventListener(startRefusedEvent, refused);
      video.removeEventListener("play", played);
    };
  }, [videoRef, onRefusedRef]);
  useEffect(() => {
    if (refusals > 0) {
      playRef.current?.focus();
    }
  }, [refusals, playRef]);
  return isRefused;
}

/**
 * The line over a film the browser would not start (useStartRefused), saying why the picture stands
 * still and what starts it, drawn only then; an output, a status a screen reader says as it shows.
 */
export function StartRefusedShown(props: { readonly isShown: boolean }): JSX.Element | null {
  const { t } = useTranslation();
  return props.isShown ? (
    <output className="player-overlay-refused">{t("playerControls.pressPlayToStart")}</output>
  ) : null;
}
