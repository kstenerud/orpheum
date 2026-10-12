// SPDX-License-Identifier: MPL-2.0

/**
 * Keeps the screen awake while a film plays (D287): a phone left alone locked its screen mid-film,
 * and iOS paused the video as it did (the owner's tests, 2026-10-12). By the Screen Wake Lock API,
 * held while the video plays on a shown page, let go when it pauses or ends; the browser lets it go
 * itself when the page is hidden, and it is asked for again when the page is shown.
 */

/** A held wake lock: whether the browser has let it go, and letting it go. */
export type ScreenLock = Pick<WakeLockSentinel, "released" | "release">;

/** What gives screen locks: a browser's navigator.wakeLock. */
export interface ScreenLocks {
  request(type: "screen"): Promise<ScreenLock>;
}

/** The page a video is on: a browser's document. */
export type AwakePage = Pick<
  Document,
  "visibilityState" | "addEventListener" | "removeEventListener"
>;

/** The video kept awake for: whether it plays, and its events. */
export type AwakeMedia = Pick<
  HTMLMediaElement,
  "paused" | "ended" | "addEventListener" | "removeEventListener"
>;

/** The video's events after which whether it plays may have changed. */
const playEvents = ["play", "playing", "pause", "ended", "emptied"] as const;

/**
 * Holds a screen lock while media plays on page, asking locks for one; returns the stopping, which
 * lets it go. Without locks, as in a browser without the API or a page not served over HTTPS, it
 * does nothing; a refused request, as in low-power mode, is asked again at the next change.
 */
export function keepScreenAwake(
  media: AwakeMedia,
  page: AwakePage,
  locks: ScreenLocks | undefined,
): () => void {
  if (locks === undefined) {
    return () => undefined;
  }
  let held: ScreenLock | undefined;
  let isAsking = false;
  let isStopped = false;
  const wants = (): boolean =>
    !isStopped && !media.paused && !media.ended && page.visibilityState === "visible";
  const letGo = (): void => {
    const lock = held;
    held = undefined;
    if (lock !== undefined && !lock.released) {
      lock.release().catch(() => undefined);
    }
  };
  const look = (): void => {
    if (held?.released === true) {
      held = undefined; // let go by the browser as the page was hidden
    }
    if (!wants()) {
      letGo();
      return;
    }
    if (held !== undefined || isAsking) {
      return;
    }
    isAsking = true;
    locks.request("screen").then(
      (lock) => {
        isAsking = false;
        held = lock;
        if (!wants()) {
          letGo();
        }
      },
      () => {
        isAsking = false;
      },
    );
  };
  for (const event of playEvents) {
    media.addEventListener(event, look);
  }
  page.addEventListener("visibilitychange", look);
  look();
  return () => {
    isStopped = true;
    for (const event of playEvents) {
      media.removeEventListener(event, look);
    }
    page.removeEventListener("visibilitychange", look);
    letGo();
  };
}
