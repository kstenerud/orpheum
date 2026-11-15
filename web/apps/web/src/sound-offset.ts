// SPDX-License-Identifier: MPL-2.0

/**
 * How far this device moves its sound from its picture, kept by the device itself (D492 §1): a
 * device is known to the core only by the id and proof its browser keeps (session.ts), so one that
 * forgets the core comes back as a new device, and what the core kept for the old one was no longer
 * its own (the owner, 2026-11-15). Every control drawn shows and applies the one value (D407 §8).
 */

import { longestSoundOffsetMs } from "@orpheum/player";
import { useSyncExternalStore } from "react";

import { browserStorage, type StringStorage } from "./browser-storage.ts";

/** The browser's key this device's sound offset is kept under, in ms: later when positive. */
export const soundOffsetKey = "orpheum.sound.offsetMs";

/**
 * The offset set in this page, which lasts as long as the page where the browser refuses to keep
 * it (a private window); undefined until the page sets one.
 */
const page: { ms: number | undefined } = { ms: undefined };

/** Who hears each offset set: every control drawn. */
const listeners = new Set<() => void>();

/** The offset kept in storage; undefined for none, or one not an offset this device may make. */
function keptIn(storage: StringStorage): number | undefined {
  let kept: unknown;
  try {
    kept = JSON.parse(storage.getItem(soundOffsetKey) ?? "null");
  } catch {
    return undefined;
  }
  return typeof kept === "number" &&
    Number.isInteger(kept) &&
    Math.abs(kept) <= longestSoundOffsetMs
    ? kept
    : undefined;
}

/** This device's offset, as set in this page or kept by its browser; undefined for none. */
function keptMs(storage: () => StringStorage): number | undefined {
  return page.ms ?? keptIn(storage());
}

/** This device's sound offset, in ms: 0, in step, for none. */
export function deviceSoundOffsetMs(storage: () => StringStorage = browserStorage): number {
  return keptMs(storage) ?? 0;
}

/** Keeps ms, within the bound either way, as this device's sound offset, telling every control. */
export function keepDeviceSoundOffsetMs(
  ms: number,
  storage: () => StringStorage = browserStorage,
): void {
  const kept = Math.round(Math.min(Math.max(ms, -longestSoundOffsetMs), longestSoundOffsetMs));
  page.ms = kept;
  storage().setItem(soundOffsetKey, JSON.stringify(kept));
  for (const listener of listeners) {
    listener();
  }
}

/**
 * Keeps the offset the core kept for this device before the device kept its own (GET /me's device,
 * D492 §1), once: where the device has none of its own and the core's is not in step, so the owner's
 * TV keeps the delay set on it. One the device has, a 0 among them, stands.
 */
export function carrySoundOffsetOver(
  coreMs: number | undefined,
  storage: () => StringStorage = browserStorage,
): void {
  if (coreMs !== undefined && coreMs !== 0 && keptMs(storage) === undefined) {
    keepDeviceSoundOffsetMs(coreMs, storage);
  }
}

function hear(listener: () => void): () => void {
  listeners.add(listener);
  return () => {
    listeners.delete(listener);
  };
}

function snapshot(): number {
  return deviceSoundOffsetMs();
}

/** This device's sound offset, drawn again as it is set. */
export function useDeviceSoundOffsetMs(): number {
  return useSyncExternalStore(hear, snapshot, snapshot);
}

/** Forgets the offset this page set, for a test, which clears the browser's storage itself. */
export function forgetSoundOffsetSetInThisPage(): void {
  page.ms = undefined;
}
