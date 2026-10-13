// SPDX-License-Identifier: MPL-2.0

import { healPageAudio } from "./page-audio.ts";
import type { TogetherConnection } from "./together-connection.ts";

/** The part of a video or audio element playing a film with or without its sound uses. */
export type SoundedMedia = Pick<
  HTMLMediaElement,
  "currentTime" | "muted" | "paused" | "ended" | "play" | "dispatchEvent"
>;

/**
 * How long after a tap for sound the player's sound is told again: whether it stayed on and the film
 * played on, which an iPhone that stayed silent after its tap could not say (the owner's VQ30,
 * 2026-10-12).
 */
export const soundToldAgainAfterMs = 2000;

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
 * From a tap: lets a video not yet playing play its films with sound for as long as it lives, by
 * turning its sound off and on again, which no one hears. iPhone Safari lifts its need of a tap for a
 * video's sound when that video's muting is changed during one, and for that one video only, across
 * every film it is given after (WebKit's HTMLMediaElement::setMutedInternal): a guest's tap before
 * the group's film started went to a video made only once it did, which still asked for its own (the
 * owner, 2026-10-13: "If the video hasn't been started yet, voice should still work, and then video
 * as well once it starts"). A video playing is left as it is: its sound is soundOnAtTap's.
 */
export function readyForSoundAtTap(media: Pick<HTMLMediaElement, "muted" | "paused">): void {
  if (!media.paused) {
    return;
  }
  /* eslint-disable no-param-reassign -- the tap's whole purpose: the element's muting changed */
  media.muted = true;
  media.muted = false;
  /* eslint-enable no-param-reassign -- only the muting changes */
}

/**
 * Turns a following player's sound on, and plays it if it is paused: for a page to call from the
 * person's tap, which lets the browser play with sound. The server's log is told, with the sound's
 * state then and soundToldAgainAfterMs later.
 */
export function soundOnAtTap(
  media: SoundedMedia & Pick<HTMLMediaElement, "volume">,
  connection: Pick<TogetherConnection, "note">,
): void {
  // eslint-disable-next-line no-param-reassign -- the tap's whole purpose: the element's sound on
  media.muted = false;
  // An iPhone's session may be stuck where the silent switch mutes it, saying playback all the while.
  healPageAudio();
  if (media.paused && !media.ended) {
    media.play().catch(() => undefined); // the tap's own play; a refusal leaves it as it was
  }
  connection.note({
    kind: "unmuted",
    positionMs: positionMsOf(media),
    soundState: soundStateOf(media),
  });
  setTimeout(() => {
    connection.note({
      kind: "unmuted",
      why: "after",
      positionMs: positionMsOf(media),
      soundState: soundStateOf(media),
    });
  }, soundToldAgainAfterMs);
}

/**
 * The player's sound as the server's log reads it: whether the element is muted, its volume, whether
 * it is paused, and the browser's audio session where it has one (Safari's navigator.audioSession).
 */
export function soundStateOf(media: Pick<HTMLMediaElement, "muted" | "volume" | "paused">): string {
  const session: unknown =
    typeof navigator === "undefined" ? undefined : Reflect.get(navigator, "audioSession");
  const sessionType: unknown =
    typeof session === "object" && session !== null ? Reflect.get(session, "type") : undefined;
  const type = typeof sessionType === "string" ? sessionType : "none";
  return `muted=${String(media.muted)} volume=${media.volume.toFixed(2)} paused=${String(media.paused)} session=${type}`;
}
