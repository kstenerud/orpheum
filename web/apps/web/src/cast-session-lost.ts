// SPDX-License-Identifier: MPL-2.0

/**
 * The Google Cast session of a cast lost without the person's Stop (D553 §5): a hint that its
 * Chromecast may be gone, not an end. The Cast link (google-cast.ts's linkCastSession) tells it, and
 * the cast's remote reads it as its receiver not heard (cast-remote-heard.ts's useReceiverHeard)
 * while the cast's session shows no leader, so it reaches for the receiver and, unheard for the
 * remote's wait, gives it up (D554 §1, §4); the receiver heard leading overrides it and forgets it.
 */

import { useSyncExternalStore } from "react";

/** The cast whose Cast session was lost last, until its cast is linked to a session again. */
let lostCastId: number | undefined;
const listeners = new Set<() => void>();

function tellChanged(): void {
  for (const listener of listeners) {
    listener();
  }
}

/** Says the Cast session of the cast castId was lost, its device switched off or out of reach. */
export function tellCastSessionLost(castId: number): void {
  lostCastId = castId;
  tellChanged();
}

/**
 * Says the cast castId is lost no more: it has a Cast session again, linked anew, or its receiver is
 * heard leading its session after the loss (D554 §4).
 */
export function tellCastSessionLinked(castId: number): void {
  if (lostCastId === castId) {
    lostCastId = undefined;
    tellChanged();
  }
}

/** Whether the Cast session of the cast castId was lost (tellCastSessionLost). */
export function isCastSessionLost(castId: number): boolean {
  return lostCastId === castId;
}

function subscribe(listener: () => void): () => void {
  listeners.add(listener);
  return () => listeners.delete(listener);
}

/** Whether the Cast session of the cast castId was lost, re-rendering as that changes. */
export function useCastSessionLost(castId: number): boolean {
  const lost = (): boolean => isCastSessionLost(castId);
  return useSyncExternalStore(subscribe, lost, lost);
}
