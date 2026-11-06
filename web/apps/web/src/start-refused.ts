// SPDX-License-Identifier: MPL-2.0

import { noteForTheLog } from "@orpheum/sdk";

/** What startRefusedNote reads of the page as the browser refused its film's start. */
export interface PageAtRefusal {
  /** Whether a press is still fresh enough to let a video play (navigator.userActivation). */
  readonly isPressLive: boolean | undefined;
  /** Whether the page has had a press since it loaded. */
  readonly hasBeenPressed: boolean | undefined;
  readonly isFullScreen: boolean;
  /** How long ago the page's last press was, in ms; undefined where it had none. */
  readonly sincePressMs: number | undefined;
}

/**
 * The core log's line for a film the browser would not start: the owner's TCL TV, in TV Bro (an
 * Android WebView), opened its films paused when Level loudness was off and playing when it was on,
 * a press on the film's page before each (staging, 2026-11-06), and Chromium run with WebView's rule
 * (autoplay-policy=user-gesture-required) played them all. The refusal's name and the page's press
 * and full screen at that moment say which rule refused it.
 */
export function startRefusedNote(cause: unknown, page: PageAtRefusal): string {
  const refusal =
    cause instanceof Error ? `${cause.name}: ${cause.message}` : `not an error: ${String(cause)}`;
  const since = page.sincePressMs === undefined ? "none" : `${String(page.sincePressMs)} ms ago`;
  return (
    `the browser refused to start a film (${refusal}); pressLive=${String(page.isPressLive)}` +
    ` pressed=${String(page.hasBeenPressed)} lastPress=${since} fullScreen=${String(page.isFullScreen)}`
  );
}

/** The page's last press, by performance.now(); undefined until the first. */
let lastPressAtMs: number | undefined = undefined;

let isFollowingPresses = false;

/** Follows the page's presses, for startRefusedNote's lastPress: from the app's start, once. */
export function followPressesForRefusals(): void {
  if (isFollowingPresses) {
    return;
  }
  isFollowingPresses = true;
  const pressed = (): void => {
    lastPressAtMs = performance.now();
  };
  for (const kind of ["keydown", "pointerup", "click"]) {
    document.addEventListener(kind, pressed, { capture: true, passive: true });
  }
}

/** The page as it stands now, for startRefusedNote. */
export function pageAtRefusal(): PageAtRefusal {
  // Typed as always there, it is absent before Chromium 72 and in older WebViews.
  const activation = "userActivation" in navigator ? navigator.userActivation : undefined;
  return {
    isPressLive: activation?.isActive,
    hasBeenPressed: activation?.hasBeenActive,
    isFullScreen: document.fullscreenElement !== null,
    sincePressMs:
      lastPressAtMs === undefined ? undefined : Math.round(performance.now() - lastPressAtMs),
  };
}

/**
 * Tells the core's log of a film's start the browser refused (startRefusedNote), with the page as it
 * stands; any other failure of play() is the playback's own to report.
 */
export function noteIfStartRefused(cause: unknown): void {
  if (cause instanceof DOMException && cause.name === "NotAllowedError") {
    noteForTheLog(startRefusedNote(cause, pageAtRefusal()));
  }
}

/**
 * Lets the video start later on the press that opened it, before full screen uses the press up: the
 * owner's TCL TV, in TV Bro (an Android WebView), went full screen as the player opened, and the start
 * gate's play, about 3 s later once enough was loaded, was refused (pressLive=false, staging,
 * 2026-11-06), Level loudness on or off. A video played once during a press is unlocked for good in
 * Chromium, so it is played and at once paused while the press is live, before anything is loaded:
 * not the play paused and played again the start gate avoids, which costs a refill on Silk.
 */
export function keepThePressForTheStart(video: HTMLVideoElement): void {
  const activation = "userActivation" in navigator ? navigator.userActivation : undefined;
  if (!video.paused || activation?.isActive === false) {
    return;
  }
  // The pause right after rejects the play with AbortError: the unlock is all it was for.
  video.play().catch(() => undefined);
  pressKeepersPauses.add(video);
  video.pause();
}

/** The videos keepThePressForTheStart paused, until their pause event is taken (isPressKeepersPause). */
const pressKeepersPauses = new WeakSet<HTMLVideoElement>();

/**
 * Whether the video's pause, as its event comes, is keepThePressForTheStart's, which says nothing of
 * the person: it is taken, and the video's next pause is the person's. Told to the server as the
 * person's, it said the player paused at 0 before the resume moved it, and a paused player is made
 * only the half minute after where it paused: The Big Parade, resumed at 28 minutes with Level
 * loudness on, waited for a levelled sound never made (the owner's TCL TV, staging, 2026-11-06).
 */
export function isPressKeepersPause(video: HTMLVideoElement): boolean {
  return pressKeepersPauses.delete(video);
}
