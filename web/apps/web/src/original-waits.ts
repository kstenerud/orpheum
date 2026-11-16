// SPDX-License-Identifier: MPL-2.0

import { createContext, useEffect, useEffectEvent } from "react";

import { deviceDataCaps, isWithinCap, type DataCaps } from "./data-cap.ts";
import type { PlaybackDecision } from "./watching.ts";

/**
 * How long the link must carry a film's own file, without a break, before a film started on its
 * ladder in place of the original its person chose moves to the original: about a minute (D501 §1,
 * the research's long hold, competitors-2026-11-16.md §2). The move is a new delivery, a reload with
 * new buffers, so it is held far longer than the player's own step up between rungs.
 */
export const originalAfterLinkCarriesMs = 60_000;

/** How often a film waiting for the original looks at its link. */
export const linkLookEveryMs = 1_000;

/**
 * How many times a film's own file's average rate its link must carry for the film to move to it
 * (D501 §1): room for a busy scene's peak over the average, which a move at the average alone would
 * stall at (competitors-2026-11-16.md §2).
 */
export const linkOverTheFileAtLeast = 1.3;

/**
 * Whether a link of linkKbps, as the playback measures it, carries a film's own file of fileKbps (the
 * decision's fileBitRateKbps, every track of it): at least linkOverTheFileAtLeast times the file
 * (D501 §1). A link not measured yet carries nothing.
 */
export function linkCarriesTheOriginal(linkKbps: number | undefined, fileKbps: number): boolean {
  return linkKbps !== undefined && linkKbps >= fileKbps * linkOverTheFileAtLeast;
}

/**
 * How long after a film moved up to the original a move down from it, its player stalled there
 * (D247 §2), says the try failed (D501 §2): two minutes.
 */
export const triedFailsWithinMs = 120_000;

/**
 * A film's playback's one try at the original (D501 §2): whether the film moved up to it and has not
 * moved down since, and whether the moves up ended, a move down within triedFailsWithinMs of the move
 * up having failed the try. Kept for the film's playback on this device, across the decisions asked
 * for meanwhile. now reads the time in milliseconds.
 */
export class OriginalTries {
  readonly #now: () => number;
  #isOnTheOriginal = false;
  #isEnded = false;
  /** When the film last moved up to the original; undefined before a try. */
  #movedUpAtMs: number | undefined;

  constructor(now: () => number = () => Date.now()) {
    this.#now = now;
  }

  /** Whether the film moved up to the original and has not moved down from it since. */
  get isOnTheOriginal(): boolean {
    return this.#isOnTheOriginal;
  }

  /** Whether the moves up to the original ended for this playback, its try failed. */
  get isEnded(): boolean {
    return this.#isEnded;
  }

  /** The film moves up to the original: a move down within triedFailsWithinMs fails the try. */
  movedUp(): void {
    this.#isOnTheOriginal = true;
    this.#movedUpAtMs = this.#now();
  }

  /** The player moved down to a lower quality (D247 §2): from the original moved to, within the try's two minutes, it ends the moves up. */
  movedDown(): void {
    if (!this.#isOnTheOriginal) {
      return;
    }
    this.#isOnTheOriginal = false;
    if (this.#movedUpAtMs !== undefined && this.#now() - this.#movedUpAtMs < triedFailsWithinMs) {
      this.#isEnded = true;
    }
  }
}

/** The film's playback's try at the original (OriginalTries), given by the Play page; undefined where none is kept. */
export const OriginalTriesContext = createContext<OriginalTries | undefined>(undefined);

/**
 * Whether a film plays on its ladder waiting for the original its person chose: the decision says so
 * (isOriginalWaitingForTheLink) and the playback's moves up to it have not ended (D501 §2).
 */
export function isWaitingForTheOriginal(
  decision: Pick<PlaybackDecision, "isOriginalWaitingForTheLink">,
  tries: OriginalTries | undefined,
): boolean {
  return decision.isOriginalWaitingForTheLink === true && tries?.isEnded !== true;
}

/** A film on its ladder waiting for the original its person chose (the decision's isOriginalWaitingForTheLink). */
export interface OriginalWaiting {
  readonly fileKbps: number;
  /** The link's speed as the playback measures it, undefined until it can say (D247 §1). */
  readonly readLinkKbps: () => number | undefined;
  /** Whether the film's buffer is full (ChunkedPlayback.isAheadFull, @orpheum/player). */
  readonly readIsAheadFull: () => boolean;
}

/**
 * Calls onCarried once, for a film waiting for the original (waiting given), when its link has carried
 * the file (linkCarriesTheOriginal) at every look for originalAfterLinkCarriesMs, its buffer is full
 * and this device's data cap lets the file play (isWithinCap, D489): the page then asks for the
 * original from where the film stands (D501 §1), and tries is told of the move (D501 §2). A look the
 * link falls short at starts the hold again; a buffer not full, or a cap under the file, holds the
 * move back until it fills, or the cap is lifted. Nothing where tries says the moves up ended.
 */
export function useOriginalOnceTheLinkCarries(
  waiting: OriginalWaiting | undefined,
  onCarried: () => void,
  by: { readonly tries?: OriginalTries | undefined; readonly caps?: DataCaps } = {},
): void {
  const readLinkKbps = useEffectEvent(() => waiting?.readLinkKbps());
  const readIsAheadFull = useEffectEvent(() => waiting?.readIsAheadFull() === true);
  const { tries } = by;
  const carried = useEffectEvent(() => {
    tries?.movedUp();
    onCarried();
  });
  const isWaiting = waiting !== undefined && tries?.isEnded !== true;
  const fileKbps = waiting?.fileKbps ?? 0;
  const caps = by.caps ?? deviceDataCaps();
  useEffect(() => {
    if (!isWaiting) {
      return undefined;
    }
    let carriedMs = 0;
    const look = setInterval(() => {
      const isCarried = linkCarriesTheOriginal(readLinkKbps(), fileKbps);
      carriedMs = isCarried ? carriedMs + linkLookEveryMs : 0;
      const isHeld = carriedMs >= originalAfterLinkCarriesMs;
      if (isHeld && readIsAheadFull() && isWithinCap(fileKbps, caps.capKbps())) {
        clearInterval(look);
        carried();
      }
    }, linkLookEveryMs);
    return () => {
      clearInterval(look);
    };
  }, [isWaiting, fileKbps, caps]);
}
