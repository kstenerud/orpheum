// SPDX-License-Identifier: MPL-2.0

/**
 * The films whose own picture this device's player gave up as one it cannot play (D266), kept by the
 * device itself (D492 §3), so that each later start of one says so and is offered the twin, H.264 at
 * the picture's height, at once (D323 §2): Firefox on a Mac gave a film's own picture up anew at every
 * playback, 1.5 to 5 s each (the owner's test, 2026-10-12). The core remembers none: a device is
 * known to it only by what its browser keeps (session.ts), so what it kept for a device that forgot
 * it was no longer that device's (the owner, 2026-11-15).
 */

import { browserStorage, type StringStorage } from "./browser-storage.ts";

/** The browser's key the films are kept under: their file ids, the latest given up last. */
export const ownPicturesGivenUpKey = "orpheum.ownPicturesGivenUp";

/**
 * The most films kept, the longest given up longest ago dropped first: far more than one device
 * gives up between a browser's updates, which may come to play them, in a few hundred bytes.
 */
export const ownPicturesGivenUpMost = 64;

/** The file ids kept in storage, the latest last; none where what is kept is not such a list. */
function keptIn(storage: StringStorage): readonly number[] {
  let kept: unknown;
  try {
    kept = JSON.parse(storage.getItem(ownPicturesGivenUpKey) ?? "[]");
  } catch {
    return [];
  }
  return Array.isArray(kept)
    ? kept.filter((each): each is number => Number.isInteger(each) && each > 0)
    : [];
}

/** Whether this device's player gave fileId's own picture up. */
export function isOwnPictureGivenUp(
  fileId: number,
  storage: () => StringStorage = browserStorage,
): boolean {
  return keptIn(storage()).includes(fileId);
}

/** Keeps that this device's player gave fileId's own picture up, as the latest. */
export function keepOwnPictureGivenUp(
  fileId: number,
  storage: () => StringStorage = browserStorage,
): void {
  const kept = [...keptIn(storage()).filter((each) => each !== fileId), fileId];
  storage().setItem(ownPicturesGivenUpKey, JSON.stringify(kept.slice(-ownPicturesGivenUpMost)));
}
