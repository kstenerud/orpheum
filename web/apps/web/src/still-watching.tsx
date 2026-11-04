// SPDX-License-Identifier: MPL-2.0

import { useEffect, useRef, useState, type JSX } from "react";
import { useTranslation } from "react-i18next";

import { commandEventName } from "./input-commands.ts";
import { isTelevision } from "./tv-device.ts";

/**
 * After how many episodes autoplay has played in a row, with no press between, the next one waits
 * on "Still watching?" instead of counting down (D339): the field's three, so a person asleep on
 * the sofa is not played a season. A justified constant: a press, any press, starts the run again,
 * and nothing else reads it.
 */
export const stillWatchingAfterAutoplays = 3;

/**
 * On a television, how long since the last press the run of autoplays must also have gone on before
 * "Still watching?" asks, in milliseconds: Netflix's rule on TVs and streaming devices, "after 3
 * episodes and 90 minutes of uninterrupted watching", where a sofa's viewing runs long and three
 * 11-minute episodes asked after half an hour (starts-chunks-remote-2026-10-29.md §4, S1). Elsewhere
 * the three autoplays alone ask, as Netflix does. A justified constant.
 */
export const stillWatchingOnATvAfterMs = 90 * 60_000;

/** The episodes autoplay has played in a row in this tab, with no press since. */
let autoplayedInARow = 0;

/** When the last press came (performance.now's clock); the tab's start until one does. */
let lastPressAtMs = 0;

/** Notes that autoplay played the next episode, with no one pressing anything. */
export function noteAutoplayed(): void {
  autoplayedInARow += 1;
}

/** Starts the run of autoplays again: someone pressed something, so someone is watching. */
export function forgetAutoplays(): void {
  autoplayedInARow = 0;
  lastPressAtMs = performance.now();
}

/**
 * Whether the next autoplay asks "Still watching?" instead of playing (D339): after
 * stillWatchingAfterAutoplays in a row, and on a television also stillWatchingOnATvAfterMs since
 * the last press.
 */
export function isStillWatchingDue(): boolean {
  if (autoplayedInARow < stillWatchingAfterAutoplays) {
    return false;
  }
  return (
    !isTelevision(navigator.userAgent) ||
    performance.now() - lastPressAtMs >= stillWatchingOnATvAfterMs
  );
}

/** The presses that say someone is watching: a remote's or a key's command, a key, a tap or a click. */
const pressEvents = [commandEventName, "keydown", "pointerdown"] as const;

/** Starts the run of autoplays again at any press on the page, while the player shows. */
export function useAnyPressForgetsAutoplays(): void {
  useEffect(() => {
    for (const each of pressEvents) {
      document.addEventListener(each, forgetAutoplays, true);
    }
    return () => {
      for (const each of pressEvents) {
        document.removeEventListener(each, forgetAutoplays, true);
      }
    };
  }, []);
}

/**
 * Whether the film's end asks "Still watching?" in place of autoplay's count, isCountDue saying it
 * would count: decided as the count would start, from the run so far, and kept until it would stop, so the presses that answer it do
 * not turn it back into a count.
 */
export function useIsAskingStillWatching(isCountDue: boolean): boolean {
  const [isAsking, setIsAsking] = useState(false);
  // Decided as the count would start or stop: adjusted as it renders.
  const [wasCountDue, setWasCountDue] = useState(false);
  if (wasCountDue !== isCountDue) {
    setWasCountDue(isCountDue);
    setIsAsking(isCountDue && isStillWatchingDue());
  }
  return isAsking && isCountDue;
}

/**
 * The "Still watching?" card, in Up next's place: Continue plays the next episode, focused so OK
 * presses it; Stop leaves the film.
 */
export function StillWatching(props: {
  readonly onContinue: () => void;
  readonly onStop: () => void;
}): JSX.Element {
  const { t } = useTranslation();
  const continueRef = useRef<HTMLButtonElement>(null);
  useEffect(() => {
    continueRef.current?.focus();
  }, []);
  return (
    <section className="up-next still-watching" aria-label={t("upNext.stillWatching")}>
      <div className="up-next__play still-watching__card" data-focus-container="">
        <p>{t("upNext.stillWatching")}</p>
        <button
          ref={continueRef}
          type="button"
          onClick={() => {
            forgetAutoplays();
            props.onContinue();
          }}
        >
          {t("upNext.continue")}
        </button>
        <button type="button" onClick={props.onStop}>
          {t("upNext.stop")}
        </button>
      </div>
    </section>
  );
}
