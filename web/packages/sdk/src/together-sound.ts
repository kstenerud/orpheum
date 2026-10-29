// SPDX-License-Identifier: MPL-2.0

import { healPageAudio } from "./page-audio.ts";
import type { Press, TogetherConnection } from "./together-connection.ts";
import type { PlayRefusal } from "./together-notes.ts";

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

/** How a refusal reads in a playRefused note: the error's name where it is one a browser gives. */
function refusalOf(error: unknown): PlayRefusal {
  const name: unknown =
    typeof error === "object" && error !== null && "name" in error ? error.name : undefined;
  return name === "NotAllowedError" || name === "AbortError" || name === "NotSupportedError"
    ? name
    : "other";
}

/** A play made as its page was shown again, which a refusal of any kind is not left at. */
export interface PlayAsShown {
  /**
   * Whether the player still wants to play as the refusal comes: its own pause, which refuses a
   * play under way, is no refusal to answer.
   */
  readonly isStillWanted: () => boolean;
  /** A refusal was noted and offered at a tap: once is enough. */
  readonly answered: () => void;
}

/**
 * Whether a following player's page was shown again and it has not played since: a refusal of any
 * kind of its play then, while it still wants to play, is noted and offered at a tap, once
 * (playOrPlayMuted's asShown), as a phone unlocked may not let its film play on (the owner,
 * 2026-10-29).
 */
export class ShownAgain {
  #isShown = false;

  /** Its page was shown again, for a follower (true), or hidden (false). */
  set(isShown: boolean): void {
    this.#isShown = isShown;
  }

  /** One of its player's events: once it plays, it is no longer just shown again. */
  heard(event: string): void {
    if (event === "playing") {
      this.#isShown = false;
    }
  }

  /** What a play passes playOrPlayMuted while shown again, isStillWanted its wish; else nothing. */
  play(isStillWanted: () => boolean): PlayAsShown | undefined {
    return this.#isShown
      ? {
          isStillWanted,
          answered: () => {
            this.#isShown = false;
          },
        }
      : undefined;
  }
}

/**
 * Plays media. A play the browser refuses for want of a recent tap is played again muted, which
 * browsers allow, and said with playedMutedEvent and a note, so the page offers the sound at a tap
 * and the log says why the film was silent; any other refusal leaves it paused, for its caller to
 * try again. A play as its page was shown again (asShown) whose refusal, of any kind, or whose video
 * stays paused though its play resolved, comes while it is still wanted, is noted (playRefused) and
 * answered the same way: a phone unlocked may not let its film play on, and its follower sat paused
 * with nothing to say why (the owner, 2026-10-29).
 */
export function playOrPlayMuted(
  media: SoundedMedia,
  connection: Pick<TogetherConnection, "note">,
  asShown?: PlayAsShown,
): void {
  const refusedAsShown = (why: PlayRefusal): boolean => {
    if (asShown?.isStillWanted() !== true) {
      return false;
    }
    asShown.answered();
    const page = pageActivation();
    connection.note({
      kind: "playRefused",
      why,
      positionMs: positionMsOf(media),
      ...(page === undefined ? {} : { page }),
    });
    playMuted(media, connection);
    return true;
  };
  media.play().then(
    () => {
      if (media.paused && !media.ended) {
        refusedAsShown("stayedPaused");
      }
    },
    (error: unknown) => {
      if (refusedAsShown(refusalOf(error)) || !isRefusedForWantOfATap(error) || media.muted) {
        return;
      }
      playMuted(media, connection);
    },
  );
}

/**
 * Plays media muted, which browsers allow without a tap, saying so with a note (where it was not
 * muted already) and playedMutedEvent, by which its page offers the tap that plays it with sound.
 */
function playMuted(media: SoundedMedia, connection: Pick<TogetherConnection, "note">): void {
  if (!media.muted) {
    // eslint-disable-next-line no-param-reassign -- muting the element is how it may play
    media.muted = true;
    const page = pageActivation();
    connection.note({
      kind: "playedMuted",
      positionMs: positionMsOf(media),
      ...(page === undefined ? {} : { page }),
    });
  }
  media.dispatchEvent(new Event(playedMutedEvent));
  media.play().catch(() => undefined); // even muted, a phone saving power may refuse: the tap plays it
}

/**
 * What the browser says of the page's user activation, for the server's log, undefined where it
 * says nothing (navigator.userActivation): whether the page was ever pressed (sticky, which lets a
 * Chromium browser play sound) and is now (transient, which full screen needs), how long since it
 * loaded and how the browser came to show it. A cast's receiver on a Fire TV was refused its sound
 * with nothing to say whether its page had been pressed or loaded again since (the owner,
 * 2026-10-27).
 */
export function pageActivation(): string | undefined {
  const activation: unknown =
    typeof navigator === "undefined" ? undefined : Reflect.get(navigator, "userActivation");
  if (typeof activation !== "object" || activation === null) {
    return undefined;
  }
  const hasBeenActive = Reflect.get(activation, "hasBeenActive") === true;
  const isActive = Reflect.get(activation, "isActive") === true;
  const entry = performance.getEntriesByType("navigation").at(0);
  const navigation = entry !== undefined && "type" in entry ? String(entry.type) : "unknown";
  return `hasBeenActive=${String(hasBeenActive)} isActive=${String(isActive)} loadedMsAgo=${String(Math.round(performance.now()))} navigation=${navigation}`;
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
 * state then and soundToldAgainAfterMs later, and by how the person pressed where the page says.
 */
export function soundOnAtTap(
  media: SoundedMedia & Pick<HTMLMediaElement, "volume">,
  connection: Pick<TogetherConnection, "note">,
  by?: Press,
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
    ...(by === undefined ? {} : { by }),
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
