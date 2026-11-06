// SPDX-License-Identifier: MPL-2.0

/**
 * A seek the person is still choosing, shown before it is made (D369 §1): a held or repeated arrow
 * moves where the player's timeline and time say, a held one by the time held, faster the longer it
 * goes on, and the film seeks
 * once, to the place shown, when the presses rest; a press on the timeline shows its place at once.
 * The shown place stays until the seek has landed. One mechanism for every player's source: a video
 * on this page (seekAheadOf) and a remote's film another device plays (useSeekAhead), whose drags
 * and steps are each one command once they rest (rule 11). A held Right on a Fire TV seeked the
 * video at every repeat of the key, ~150 seeks in a minute, the marker moving only as each landed
 * (the owner, 2026-10-29).
 */

import { seekTo } from "@orpheum/player";
import { useEffect, useState } from "react";

import { onCommandRelease, type CommandHold } from "./input-commands.ts";
import { useLatest } from "./latest.ts";

/**
 * How far a seek moves, in milliseconds: the usual step of a TV's player, the app having had no
 * keyboard seeking of its own to follow. A justified constant.
 */
export const seekStepMs = 10_000;

/** One stage of a held arrow's speed (heldSeekSpeeds). */
export interface HeldSeekSpeed {
  /** How long the arrow has been held when the stage starts, in milliseconds. */
  readonly afterMs: number;
  /** The least it moves in a second held, in milliseconds of film. */
  readonly msPerS: number;
  /** The part of the film's length it moves in a second held, where that is more. */
  readonly ofLengthPerS: number;
}

/**
 * How fast a held arrow moves the place shown, by wall time, not by the key's repeats, whose rate
 * differs eightfold between devices: a Fire TV's Silk passing Android's 50 ms repeats moved 25
 * minutes in a 4 s hold at 10/30/60 s a repeat (starts-chunks-remote-2026-10-29.md §3.1). Each stage
 * moves the larger of its seconds and its part of the film's length a second, as Leanback and Media3
 * scale by the length, so a 2-hour film is crossed in about 12 s at the top speed and a 22-minute
 * episode at 120 s a second; the stages accelerate as a TV player's scrubbing does (the owner,
 * 2026-10-29). Justified constants, the research's own guesses until the lab's timings (§3.1's test).
 */
export const heldSeekSpeeds: readonly HeldSeekSpeed[] = [
  { afterMs: 0, msPerS: 20_000, ofLengthPerS: 0.01 },
  { afterMs: 2000, msPerS: 60_000, ofLengthPerS: 0.03 },
  { afterMs: 4000, msPerS: 120_000, ofLengthPerS: 0.08 },
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
 * How long a held press, a key's or a controller's (CommandHold), may go without a repeat before the
 * seek is made anyway, in milliseconds: its release makes the seek seekRestMs later, and this covers
 * a release the page never hears. A justified constant.
 */
export const heldRestMs = 1500;

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

/**
 * How far an arrow held heldForMs has moved the place shown by its holding (heldSeekSpeeds), in
 * milliseconds of a film lengthMs long, its first press's seekStepMs not counted; a length not known
 * (Infinity) moves each stage's seconds.
 */
export function heldTravelMs(heldForMs: number, lengthMs: number): number {
  let travelledMs = 0;
  heldSeekSpeeds.forEach((speed, index) => {
    const endMs = heldSeekSpeeds[index + 1]?.afterMs ?? Infinity;
    const spanMs = Math.min(heldForMs, endMs) - speed.afterMs;
    if (spanMs > 0) {
      const byLength = Number.isFinite(lengthMs) ? speed.ofLengthPerS * lengthMs : 0;
      travelledMs += (spanMs / 1000) * Math.max(speed.msPerS, byLength);
    }
  });
  return travelledMs;
}

/** What a SeekAhead seeks and tells. */
export interface SeekAheadWays {
  /** Makes the seek, asked by what askedBy says where known (askedByOf). */
  readonly seek: (atMs: number, askedBy?: string) => void;
  /** Tells the place shown changed. */
  readonly changed: () => void;
  /** Where a hold's release is heard (onCommandRelease): the page's window. */
  readonly releases?: EventTarget | undefined;
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
  /**
   * The press held down, a key's or a controller's, while it is: when it went down, which way, and
   * when its last press or repeat moved the place shown.
   */
  #held: { readonly fromMs: number; readonly direction: -1 | 1; lastAtMs: number } | undefined;
  #timer: ReturnType<typeof setTimeout> | undefined;
  #stopHearingReleases: (() => void) | undefined;

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
   * towards direction, never before 0 nor past film.mostMs; the seek is made once the presses rest.
   * A press moves seekStepMs; a held one's repeat (press.hold "continue", from a remote's key and a
   * controller's stick alike, CommandHold) moves by the time held since (heldTravelMs). A held press
   * waits for its release, then seekRestMs; press.askedBy is what asked it (askedByOf), told with
   * the seek.
   */
  step(
    direction: -1 | 1,
    film: { readonly atMs: number; readonly mostMs: number },
    press: { readonly hold?: CommandHold | undefined; readonly askedBy?: string | undefined },
  ): void {
    const { atMs: fromMs, mostMs } = film;
    const { hold, askedBy } = press;
    const nowMs = this.#now();
    const held = this.#held;
    let moveMs = seekStepMs;
    if (hold === "continue" && held?.direction === direction) {
      moveMs =
        heldTravelMs(nowMs - held.fromMs, mostMs) -
        heldTravelMs(held.lastAtMs - held.fromMs, mostMs);
      held.lastAtMs = nowMs;
    } else {
      this.#held = hold === undefined ? undefined : { fromMs: nowMs, direction, lastAtMs: nowMs };
    }
    const baseMs = this.shownMs(fromMs);
    const toMs = Math.min(Math.max(baseMs + direction * moveMs, 0), mostMs);
    if (hold !== undefined) {
      this.#hearRelease();
    }
    this.aim(toMs, this.#stopHearingReleases === undefined ? seekRestMs : heldRestMs, askedBy);
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
    this.#stopHearingReleases?.();
    this.#stopHearingReleases = undefined;
  }

  #make(): void {
    clearTimeout(this.#timer);
    this.#timer = undefined;
    this.#stopHearingReleases?.();
    this.#stopHearingReleases = undefined;
    this.#held = undefined;
    const aimMs = this.#aimMs;
    if (aimMs === undefined) {
      return;
    }
    this.#madeAtMs = this.#now();
    this.#ways.seek(aimMs, this.#askedBy);
  }

  /** Waits for the hold's release (onCommandRelease) to rest seekRestMs. */
  #hearRelease(): void {
    const releases = this.#ways.releases;
    if (releases === undefined || this.#stopHearingReleases !== undefined) {
      return;
    }
    this.#stopHearingReleases = onCommandRelease(releases, () => {
      this.#stopHearingReleases?.();
      this.#stopHearingReleases = undefined;
      this.#held = undefined;
      if (this.#aimMs !== undefined && this.#madeAtMs === undefined) {
        clearTimeout(this.#timer);
        this.#timer = setTimeout(() => {
          this.#make();
        }, seekRestMs);
      }
    });
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
    releases: video.ownerDocument.defaultView ?? undefined,
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
  const seekRef = useLatest(seek);
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
