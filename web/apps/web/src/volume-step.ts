// SPDX-License-Identifier: MPL-2.0

/*
 * The volume's step, apart from the player's commands so that what the home page loads (the voice
 * of Watch Together) can step a volume without loading the player (home-imports.test.ts).
 */

import type { PlayerSound } from "./playback-target.ts";

/**
 * How far a press of + or − moves the volume: 5 % (D407 §10, the owner's Q12), the cast remote's
 * Volume down and up too (D562, which amends D548 §6's 10 %). A justified constant.
 */
export const volumeStep = 0.05;

/**
 * The volume a press of + (direction 1) or − (-1) leaves, from volume: a step along (volumeStep),
 * to the nearest hundredth so that steps add up evenly, kept within 0 and 1.
 */
export function volumeAfter(volume: number, direction: -1 | 1): number {
  const moved = Math.round((volume + direction * volumeStep) * 100) / 100;
  return Math.min(1, Math.max(0, moved));
}

/**
 * Moves sound a step towards direction (volumeAfter), a step up turning a muted film's sound on
 * first, as a player's volume key does: the one step of a film's volume wherever it is worked, on
 * the pad, at + and − over a castee, and in 🔈's row.
 */
export function stepSound(sound: PlayerSound, direction: -1 | 1): void {
  if (direction === 1 && sound.isMuted) {
    sound.setMuted(false);
  }
  sound.setVolume(volumeAfter(sound.volume, direction));
}
