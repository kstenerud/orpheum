// SPDX-License-Identifier: MPL-2.0

import { isNearItsEnd, type Ending } from "./up-next-lead.ts";

/**
 * Whether an episode before or after the film can be played now: there is one, and the core says
 * nothing keeps it from playing (D555 §3: the Play page's moves and a Chromecast's Up next alike).
 */
export function isPlayableNext(episode: { readonly unavailable?: unknown } | undefined): boolean {
  return episode !== undefined && episode.unavailable === undefined;
}

/** What the person did to the offer: dismissed it, or stopped its countdown. */
export interface OfferChoice {
  readonly isDismissed: boolean;
  readonly isCancelled: boolean;
}

/** The offer as it first shows: neither dismissed nor stopped. */
export const noOfferChoice: OfferChoice = { isDismissed: false, isCancelled: false };

/**
 * Whether the film is near its end, and whether Up next's offer shows and counts down (D555 §3: the
 * one choice the Play page, the web app's receiver and a cast's remote make by): it shows from its
 * credits' start where the file has a credits segment, else within upNextLeadMs of the end, and on
 * the end screen, unless dismissed (isNearItsEnd); it counts from the credits' start, else once the
 * film has ended, with autoplay on, unless stopped.
 */
export function offerState(
  ending: Ending,
  choice: OfferChoice,
  isAutoplayOn: boolean,
): { readonly isNearTheEnd: boolean; readonly isShown: boolean; readonly isCounting: boolean } {
  const isNearTheEnd = isNearItsEnd(ending);
  const isShown = isNearTheEnd && !choice.isDismissed;
  return {
    isNearTheEnd,
    isShown,
    isCounting: isShown && isAtItsCredits(ending) && isAutoplayOn && !choice.isCancelled,
  };
}

/** Whether the film has ended, or plays its credits where a segment says where they start. */
function isAtItsCredits(ending: Ending): boolean {
  const { creditsStartMs } = ending;
  return ending.hasEnded || (creditsStartMs !== undefined && ending.atMs >= creditsStartMs);
}
