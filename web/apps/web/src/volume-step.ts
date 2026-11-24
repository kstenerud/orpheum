// SPDX-License-Identifier: MPL-2.0

/*
 * The volume's step, apart from the player's commands so that what the home page loads (the voice
 * of Watch Together) can step a volume without loading the player (home-imports.test.ts).
 */

/** How far a press of + or − moves the volume: 5 % (D407 §10, the owner's Q12). A justified constant. */
export const volumeStep = 0.05;

/**
 * How far the cast remote's Volume down and up move the film's level on the receiver: 10 % (D548
 * §6), ten presses from silence to full. A justified constant.
 */
export const remoteVolumeStep = 0.1;

/**
 * The volume a press of + (direction 1) or − (-1) leaves, from volume: a step along (volumeStep, or
 * step where given), to the nearest hundredth so that steps add up evenly, kept within 0 and 1.
 */
export function volumeAfter(volume: number, direction: -1 | 1, step: number = volumeStep): number {
  const moved = Math.round((volume + direction * step) * 100) / 100;
  return Math.min(1, Math.max(0, moved));
}
