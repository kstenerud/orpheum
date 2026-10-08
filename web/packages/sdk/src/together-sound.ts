// SPDX-License-Identifier: MPL-2.0

import type { TogetherConnection } from "./together-connection.ts";

/** The part of a video or audio element playing a film with or without its sound uses. */
export type SoundedMedia = Pick<
  HTMLMediaElement,
  "currentTime" | "muted" | "paused" | "ended" | "play" | "dispatchEvent"
>;

/**
 * The event a following player fires at its video when the browser would not start it with sound
 * and it plays muted instead: a page offers the sound at a tap (soundOnAtTap). iPhone Safari
 * refuses a play with sound that no recent tap started, and a guest's film starts long after their
 * Join (the owner's test, 2026-10-08, where her iPhone sat paused, showing each catch-up seek's
 * frame, and silent; docs/contributors/idiosyncrasies.md).
 */
export const playedMutedEvent = "orpheumplayedmuted";

/** Whether error is a browser refusing a play that no recent tap started. */
function isRefusedForWantOfATap(error: unknown): boolean {
  return (
    typeof error === "object" &&
    error !== null &&
    "name" in error &&
    error.name === "NotAllowedError"
  );
}

/** The media's position, in ms. */
function positionMsOf(media: SoundedMedia): number {
  return media.currentTime * 1000;
}

/**
 * Plays media. A play the browser refuses for want of a recent tap is played again muted, which
 * browsers allow, and said with playedMutedEvent and a note, so the page offers the sound at a tap
 * and the log says why the film was silent; any other refusal leaves it paused, for its caller to
 * try again.
 */
export function playOrPlayMuted(
  media: SoundedMedia,
  connection: Pick<TogetherConnection, "note">,
): void {
  media.play().catch((error: unknown) => {
    if (!isRefusedForWantOfATap(error) || media.muted) {
      return;
    }
    // eslint-disable-next-line no-param-reassign -- muting the element is how it may play
    media.muted = true;
    connection.note({ kind: "playedMuted", positionMs: positionMsOf(media) });
    media.dispatchEvent(new Event(playedMutedEvent));
    media.play().catch(() => undefined); // even muted, a phone saving power may refuse: the tap plays it
  });
}

/**
 * Turns a following player's sound on, and plays it if it is paused: for a page to call from the
 * person's tap, which lets the browser play with sound. The server's log is told.
 */
export function soundOnAtTap(
  media: SoundedMedia,
  connection: Pick<TogetherConnection, "note">,
): void {
  // eslint-disable-next-line no-param-reassign -- the tap's whole purpose: the element's sound on
  media.muted = false;
  connection.note({ kind: "unmuted", positionMs: positionMsOf(media) });
  if (media.paused && !media.ended) {
    media.play().catch(() => undefined); // the tap's own play; a refusal leaves it as it was
  }
}
