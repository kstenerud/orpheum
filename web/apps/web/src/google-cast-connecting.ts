// SPDX-License-Identifier: MPL-2.0

/**
 * A cast to a Google Cast device being connected (D570): from the pick in Chrome's picker, its Cast
 * session made and the device named, until the cast exists and its receiver has taken it, the cast's
 * page then taking over (D569). The Play page says it with Cancel, its transport waiting
 * (chromecast-connecting.tsx); the cast's start tells it (cast-to.tsx's useStartsCasts), and the
 * live connection's sending ends it (chromecastSendingNow).
 */

import { useSyncExternalStore } from "react";

/** A Google Cast device being connected, by the name Chrome gives it, and its Cancel. */
export interface ChromecastConnecting {
  readonly name: string;
  /** Gives the cast up (D570 §3), the film carrying on here as it stands. */
  readonly cancel: () => void;
}

/** What a connect started (startChromecastConnecting) tells as it goes on. */
export interface ConnectingTold {
  /** The cast castId was made: the connect ends once its receiver takes it, or it is over untaken. */
  readonly made: (castId: number) => void;
  /** The connect ended with no cast: none picked after all, given up, or failed. */
  readonly end: () => void;
}

/** The connect now, the cast it made (castId) and whether the live connection said it sent. */
let now:
  | {
      readonly shown: ChromecastConnecting;
      readonly castId?: number;
      readonly isSent: boolean;
    }
  | undefined;

/** What the live connection said this device sends last, which may come before the cast is made. */
let sending: { readonly castId: number; readonly isAccepted: boolean } | undefined;

const listeners = new Set<() => void>();

function tellChanged(): void {
  for (const listener of listeners) {
    listener();
  }
}

/**
 * Says a Google Cast device named name is being connected, given up by cancel; what is answered
 * says how it goes on. Its Cancel ends the connect at once, then calls cancel.
 */
export function startChromecastConnecting(name: string, cancel: () => void): ConnectingTold {
  const endIfCurrent = (): void => {
    if (now?.shown === shown) {
      now = undefined;
      tellChanged();
    }
  };
  const shown: ChromecastConnecting = {
    name,
    cancel: () => {
      endIfCurrent();
      cancel();
    },
  };
  now = { shown, isSent: false };
  tellChanged();
  return {
    made: (castId) => {
      if (now?.shown === shown) {
        now = { shown, castId, isSent: false };
        heardSending();
      }
    },
    end: endIfCurrent,
  };
}

/**
 * Ends the connect whose cast the live connection says its receiver took (the cast's page takes over
 * in the same render, cast-to.tsx's PlayOrSendOn), or which it said sent and no longer does.
 */
function heardSending(): void {
  if (now?.castId === undefined) {
    return;
  }
  if (sending?.castId === now.castId) {
    if (sending.isAccepted) {
      now = undefined;
      tellChanged();
    } else {
      now = { ...now, isSent: true };
    }
  } else if (now.isSent) {
    now = undefined;
    tellChanged();
  }
}

/** Whether a Google Cast device is being connected now, read as a command comes. */
export function isChromecastConnecting(): boolean {
  return now !== undefined;
}

function subscribe(listener: () => void): () => void {
  listeners.add(listener);
  return () => listeners.delete(listener);
}

const shownNow = (): ChromecastConnecting | undefined => now?.shown;

/** The Google Cast device being connected, if one is, re-rendering as that changes. */
export function useChromecastConnecting(): ChromecastConnecting | undefined {
  return useSyncExternalStore(subscribe, shownNow, shownNow);
}

/**
 * Tells the connect what the live connection says this device sends now (castId, taken by its
 * receiver where isAccepted), or nothing: the connect ends as its cast is taken or over
 * (heardSending). google-cast.ts's useEndsTheConnectingOnceTaken tells it.
 */
export function chromecastSendingNow(castId: number | undefined, isAccepted: boolean): void {
  sending = castId === undefined ? undefined : { castId, isAccepted };
  heardSending();
}
