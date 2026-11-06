// SPDX-License-Identifier: MPL-2.0

import type { Beep } from "./lab-tone.ts";

/**
 * The sound timing test's clip (the owner's design for a sync clip, 2026-10-26, approved for the
 * test 2026-11-06): four seconds, a rising beep at the start of each, and a picture that flashes the
 * second's colour and number as its beep starts, a bar filling across the second between. Its sound
 * is a WAV made in the page (lab-tone.ts) and its picture is drawn from the element's clock, so the
 * test loads nothing and needs no clip on the server.
 */

/** How long the clip lasts before it loops, in milliseconds: one beep a second. */
export const soundTimingClipMs = 4000;

/** How long each beep and its flash last, in milliseconds: short, so their starts are seen together. */
export const soundTimingFlashMs = 100;

/**
 * The beeps' pitches, rising through the four seconds (an A major arpeggio), so which beep is which is
 * heard as the number says it.
 */
const pitchesHz = [440, 554, 659, 880] as const;

/** The clip's beeps, one at the start of each second. */
export const soundTimingBeeps: readonly Beep[] = pitchesHz.map((frequencyHz, index) => ({
  frequencyHz,
  atMs: index * 1000,
  lastsMs: soundTimingFlashMs,
}));

/** What the picture shows at a moment: the second's number from 1, its flash, and the bar's fill. */
export interface TimingPicture {
  readonly beat: number;
  readonly isFlash: boolean;
  /** How far across its second the bar has filled, from 0 to 1. */
  readonly fill: number;
}

/**
 * The picture at soundAtMs into the clip's sound, shown pictureLateByMs behind it: where the player
 * moves a film's sound earlier than its picture, which a clip played as its own file cannot, the
 * test holds its picture back by as much, so the two meet as a film's would.
 */
export function timingPictureAt(soundAtMs: number, pictureLateByMs: number): TimingPicture {
  const at =
    (((soundAtMs - pictureLateByMs) % soundTimingClipMs) + soundTimingClipMs) % soundTimingClipMs;
  const intoSecond = at % 1000;
  return {
    beat: Math.floor(at / 1000) + 1,
    isFlash: intoSecond < soundTimingFlashMs,
    fill: intoSecond / 1000,
  };
}
