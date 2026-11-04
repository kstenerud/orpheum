// SPDX-License-Identifier: MPL-2.0

/**
 * A seek the person is still choosing, shown before it is made (D369 §1): a held or repeated arrow
 * moves where the player's timeline and time say, faster the longer it goes on, and the film seeks
 * once, to the place shown, when the presses rest; a press on the timeline shows its place at once.
 * The shown place stays until the seek has landed. One mechanism for every player's source: a video
 * on this page (seekAheadOf) and a remote's film another device plays (useSeekAhead), whose drags
 * and steps are each one command once they rest (rule 11). A held Right on a Fire TV seeked the
 * video at every repeat of the key, ~150 seeks in a minute, the marker moving only as each landed
 * (the owner, 2026-10-29).
 */

import { seekTo } from "@orpheum/player";
import { useEffect, useRef, useState } from "react";

/**
 * How far a seek moves, in milliseconds: the usual step of a TV's player, the app having had no
 * keyboard seeking of its own to follow. A justified constant.
 */
export const seekStepMs = 10_000;

/**
 * How far each press of a held or repeated arrow moves, by how long its presses have gone on
 * without a rest: seekStepMs at first, then 30 s from 2 s on, then 60 s from 4 s on, the modest
 * acceleration of a TV player's scrubbing, so a long film is crossed in seconds while a short hold
 * stays precise (the owner, 2026-10-29). Justified constants.
 */
export const heldSeekSteps: readonly { readonly afterMs: number; readonly stepMs: number }[] = [
  { afterMs: 0, stepMs: seekStepMs },
  { afterMs: 2000, stepMs: 30_000 },
  { afterMs: 4000, stepMs: 60_000 },
];

/**
 * How long an arrow's presses rest before the seek is made, in milliseconds: longer than a held
 * key's or a controller's first repeat (stickRepeatFirstMs, 400 ms), and Kodi's 750 ms, so a person
 * pressing slowly, or a remote with lag, does not end a run early with a seek that loads chunks
 * (starts-chunks-remote-2026-10-29.md §3.2; it was 500 ms, the owner's "about half a second" of
 * 2026-10-29). A justified constant.
 */
export const seekRestMs = 750;

/**
 * How long a key held down may go without a repeat before the seek is made anyway, in
 * milliseconds: its release makes the seek seekRestMs later, and this covers a release the page
 * never hears. A justified constant.
 */
export const heldKeyRestMs = 1500;

/**
 * How near the film must come to the place shown for its seek to have landed, in milliseconds: a
 * second and a half, more than a timeline's step (a second) and the time a remote's position takes
 * to be told. A justified constant.
 */
export const landedWithinMs = 1500;

/**
 * How long the place shown waits for its seek to land, at most, in milliseconds: a seek refused or
 * moved elsewhere shows where the film is after it. A justified constant.
 */
export const landingShownForMs = 10_000;

/** The step a press moves after its presses have gone on heldForMs without a rest. */
export function heldStepMs(heldForMs: number): number {
  let stepMs = seekStepMs;
  for (const step of heldSeekSteps) {
    if (heldForMs >= step.afterMs) {
      stepMs = step.stepMs;
    }
  }
  return stepMs;
}

/** What a SeekAhead seeks and tells. */
export interface SeekAheadWays {
  /** Makes the seek, asked by what askedBy says where known (askedByOf). */
  readonly seek: (atMs: number, askedBy?: string) => void;
  /** Tells the place shown changed. */
  readonly changed: () => void;
  /** Where a held key's release is heard: the page's window. */
  readonly keys?: EventTarget | undefined;
  /** The clock, in ms. */
  readonly now?: () => number;
}

/**
 * A seek being chosen: the place it aims at, shown at once (shownMs), made once the presses rest,
 * and shown on until the film arrives there.
 */
export class SeekAhead {
  readonly #ways: SeekAheadWays;
  /** The place shown: the aim, waiting or made and not yet landed. */
  #aimMs: number | undefined;
  /** What asked the aim, as askedByOf says it; undefined where not told. */
  #askedBy: string | undefined;
  /** When the seek to the aim was made; undefined while it waits for the presses to rest. */
  #madeAtMs: number | undefined;
  /** When the steps of this run of presses began. */
  #runFromMs: number | undefined;
  #timer: ReturnType<typeof setTimeout> | undefined;
  #stopHearingKeys: (() => void) | undefined;

  constructor(ways: SeekAheadWays) {
    this.#ways = ways;
  }

  /**
   * Where to show the film when it stands at atNowMs: the aim while its seek waits or travels, the
   * film's own place once it came within landedWithinMs of the aim or landingShownForMs passed.
   */
  shownMs(atNowMs: number): number {
    const aimMs = this.#aimMs;
    if (aimMs === undefined) {
      return atNowMs;
    }
    const madeAtMs = this.#madeAtMs;
    if (
      madeAtMs !== undefined &&
      (Math.abs(atNowMs - aimMs) <= landedWithinMs || this.#now() - madeAtMs >= landingShownForMs)
    ) {
      this.#aimMs = undefined;
      this.#madeAtMs = undefined;
      return atNowMs;
    }
    return aimMs;
  }

  /**
   * Shows atMs and seeks there once no other aim has come for restMs; 0 seeks at once. askedBy is
   * what asked it (askedByOf), told with the seek.
   */
  aim(atMs: number, restMs: number, askedBy?: string): void {
    this.#aimMs = atMs;
    this.#askedBy = askedBy;
    this.#madeAtMs = undefined;
    clearTimeout(this.#timer);
    if (restMs <= 0) {
      this.#make();
    } else {
      this.#timer = setTimeout(() => {
        this.#make();
      }, restMs);
    }
    this.#ways.changed();
  }

  /**
   * An arrow's press: moves the place shown, from where the film stands (film.atMs) where none is,
   * by its step (heldStepMs) towards direction, never before 0 nor past film.mostMs; the seek is
   * made once the presses rest. A key's press (press.isKey) waits for the key's release, then
   * seekRestMs; press.askedBy is what asked it (askedByOf), told with the seek.
   */
  step(
    direction: -1 | 1,
    film: { readonly atMs: number; readonly mostMs: number },
    press: { readonly isKey: boolean; readonly askedBy?: string | undefined },
  ): void {
    const { atMs: fromMs, mostMs } = film;
    const { isKey, askedBy } = press;
    const nowMs = this.#now();
    this.#runFromMs ??= nowMs;
    const baseMs = this.shownMs(fromMs);
    const toMs = Math.min(
      Math.max(baseMs + direction * heldStepMs(nowMs - this.#runFromMs), 0),
      mostMs,
    );
    if (isKey) {
      this.#hearRelease();
    }
    this.aim(toMs, this.#stopHearingKeys === undefined ? seekRestMs : heldKeyRestMs, askedBy);
  }

  /** Makes a waiting seek now: play or pause pressed while choosing. */
  land(): void {
    if (this.#aimMs !== undefined && this.#madeAtMs === undefined) {
      this.#make();
    }
  }

  /** The film's seek landed: its own place shows. */
  landed(): void {
    if (this.#madeAtMs !== undefined) {
      this.#aimMs = undefined;
      this.#madeAtMs = undefined;
      this.#ways.changed();
    }
  }

  /** Stops waiting: the player is gone. */
  stop(): void {
    clearTimeout(this.#timer);
    this.#stopHearingKeys?.();
    this.#stopHearingKeys = undefined;
  }

  #make(): void {
    clearTimeout(this.#timer);
    this.#timer = undefined;
    this.#stopHearingKeys?.();
    this.#stopHearingKeys = undefined;
    this.#runFromMs = undefined;
    const aimMs = this.#aimMs;
    if (aimMs === undefined) {
      return;
    }
    this.#madeAtMs = this.#now();
    this.#ways.seek(aimMs, this.#askedBy);
  }

  /** Waits for the held key's release, or the page losing the keys, to rest seekRestMs. */
  #hearRelease(): void {
    const keys = this.#ways.keys;
    if (keys === undefined || this.#stopHearingKeys !== undefined) {
      return;
    }
    const released = (): void => {
      this.#stopHearingKeys?.();
      this.#stopHearingKeys = undefined;
      if (this.#aimMs !== undefined && this.#madeAtMs === undefined) {
        clearTimeout(this.#timer);
        this.#timer = setTimeout(() => {
          this.#make();
        }, seekRestMs);
      }
    };
    keys.addEventListener("keyup", released, true);
    keys.addEventListener("blur", released);
    this.#stopHearingKeys = () => {
      keys.removeEventListener("keyup", released, true);
      keys.removeEventListener("blur", released);
    };
  }

  #now(): number {
    return this.#ways.now?.() ?? performance.now();
  }
}

/**
 * What asked a seek, for the core's log: the command and where it came from, as
 * "<command>-<source>" (seekBack-media, right-key, timeline-bar), source "click" for a press with
 * none. A cast's receiver on a Fire TV stepped back 10 s twice, a step no one meant, and the log
 * could not say whether a key, a controller or the media session asked it (staging, 2026-10-29,
 * file 5680).
 */
export function askedByOf(command: string, source: string | undefined): string {
  return `${command}-${source ?? "click"}`;
}

/** The event a video is sent just before a seek asked of it (seekAheadOf), its detail askedByOf's. */
export const seekAskedEvent = "orpheum-seek-asked";

/** What asked a seek, from its seekAskedEvent; undefined for any other event. */
export function soughtByOf(event: Event): string | undefined {
  const detail: unknown = event instanceof CustomEvent ? event.detail : undefined;
  return typeof detail === "string" && /^[A-Za-z]+-[a-z]+$/u.test(detail) ? detail : undefined;
}

/** The event a video is sent as the place its player shows changes (seekAheadOf). */
export const seekAimedEvent = "orpheum-seek-aimed";

/** Each video's seek being chosen. */
const videoAheads = new WeakMap<HTMLVideoElement, SeekAhead>();

/**
 * The seek being chosen on video, one for every press and bar that seeks it: an arrow's command
 * (player-commands.ts) and its timeline (useVideoSource), so a press on one ends what the other
 * chose. It tells the video seekAimedEvent as the place shown changes, and takes the video's seeked
 * as its seek landed.
 */
export function seekAheadOf(video: HTMLVideoElement): SeekAhead {
  const known = videoAheads.get(video);
  if (known !== undefined) {
    return known;
  }
  const ahead = new SeekAhead({
    seek: (atMs, askedBy) => {
      if (askedBy !== undefined) {
        video.dispatchEvent(new CustomEvent(seekAskedEvent, { detail: askedBy }));
      }
      seekTo(video, atMs);
    },
    changed: () => {
      video.dispatchEvent(new Event(seekAimedEvent));
    },
    keys: video.ownerDocument.defaultView ?? undefined,
  });
  // Any seek landing ends the wait, a Watch Together follower's own undoing a seek too.
  video.addEventListener("seeked", () => {
    if (!video.seeking) {
      ahead.landed();
    }
  });
  videoAheads.set(video, ahead);
  return ahead;
}

/**
 * A seek being chosen for a film another device plays, its seek a command (a remote's): drawn again
 * as the place shown changes, stopped as the remote goes; the latest seek is made each time.
 */
export function useSeekAhead(seek: (atMs: number) => void): SeekAhead {
  const seekRef = useRef(seek);
  useEffect(() => {
    seekRef.current = seek;
  });
  const [, setChanges] = useState(0);
  const [ahead] = useState(
    () =>
      new SeekAhead({
        seek: (atMs) => {
          seekRef.current(atMs);
        },
        changed: () => {
          setChanges((count) => count + 1);
        },
      }),
  );
  useEffect(
    () => () => {
      ahead.stop();
    },
    [ahead],
  );
  return ahead;
}
