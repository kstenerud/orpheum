// SPDX-License-Identifier: MPL-2.0

/**
 * The video an iPhone's Play page plays in, readied during the press that opened it. WebKit lets a
 * video play only in a press, or one that has once been played in a press (WebKit's lifting of its
 * restrictions at the first play in a gesture); the Play page asks its film to play once enough is
 * loaded, after the press, and Chrome on an iPhone refused it: the core's log said
 * pressLive=false lastPress=581 ms ago (the owner, staging, 2026-11-11: "In Chrome it went black
 * again"), where Safari had let it. A video played and at once paused in the press of a link to a Play
 * page is let play for good, and the Play page plays its film in that one (useVideoPressedForPlay):
 * every Play page after it too, Up next's among them, until the next such press readies another.
 */

import { useState } from "react";

import { routeOfHref } from "./router.ts";

/** The video readied at the last press of a link to a Play page; undefined before the first. */
let readied: HTMLVideoElement | undefined = undefined;

/** Stops following the presses, where followPressesOfPlay started. */
let stopFollowing: (() => void) | undefined = undefined;

/**
 * Readies a video at each press of a link to a Play page (readyAtThePress), from the app's start,
 * once, where isOn says the browser needs it: an iPhone's or an iPad's, whose every browser is
 * WebKit's. Elsewhere a press lets the page, not one video, play (Chromium's sticky activation).
 */
export function followPressesOfPlay(
  page: Document,
  isOn: boolean = isThisIos(navigator.userAgent),
): void {
  if (stopFollowing !== undefined || !isOn) {
    return;
  }
  const pressed = (event: Event): void => {
    readyAtThePress(page, event);
  };
  page.addEventListener("click", pressed, { capture: true });
  stopFollowing = () => {
    page.removeEventListener("click", pressed, { capture: true });
  };
}

/** Whether the user agent is an iPhone's, an iPad's or an iPod's, all of whose browsers are WebKit's. */
export function isThisIos(userAgent: string): boolean {
  return /\b(iPhone|iPad|iPod)\b/.test(userAgent);
}

/**
 * Plays and at once pauses a new video during a press of a link to a Play page, before the link is
 * followed, and keeps it for the Play page (useVideoPressedForPlay). The play has no film to play:
 * its pause rejects it, which is let go, the press it was played in being all it was for.
 */
function readyAtThePress(page: Document, event: Event): void {
  const link = event.target instanceof Element ? event.target.closest("a") : null;
  const href = link?.getAttribute("href") ?? null;
  if (href === null || routeOfHref(href).screen !== "play") {
    return;
  }
  const video = page.createElement("video");
  video.playsInline = true;
  video.play().catch(() => undefined);
  video.pause();
  readied = video;
}

/**
 * The video the Play page plays its film in: the one readied at the last press of a link to a Play
 * page (followPressesOfPlay), the same for as long as the page shows; undefined where none was, and
 * the player makes its own.
 */
export function useVideoPressedForPlay(): HTMLVideoElement | undefined {
  const [video] = useState(() => readied);
  return video;
}

/** Test-only: forgets what was followed and readied, as a new page load would. */
export function forgetVideoPressedForPlay(): void {
  readied = undefined;
  stopFollowing?.();
  stopFollowing = undefined;
}
