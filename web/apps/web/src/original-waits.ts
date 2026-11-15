// SPDX-License-Identifier: MPL-2.0

import { useEffect, useEffectEvent } from "react";

import { deviceDataCaps, isWithinCap, type DataCaps } from "./data-cap.ts";

/**
 * How long the link must carry a film's own file, without a break, before a film started on its
 * ladder in place of the original its person chose moves to the original: the lead's proposal,
 * awaiting the owner (the 11-27 batch's "the original chosen on a link too slow for it"). It is the
 * hold the player waits on its link before its first step up (holdBeforeStepUpMsAt, @orpheum/player),
 * since this is that step taken to the top.
 */
export const originalAfterLinkCarriesMs = 30_000;

/** How often a film waiting for the original looks at its link. */
export const linkLookEveryMs = 1_000;

/**
 * Whether a link of linkKbps, as the playback measures it, carries a film's own file of fileKbps (the
 * decision's fileBitRateKbps, every track of it): by the rule its ladder chose its quality by, the
 * file within sharePercent of the link (the offer's linkSharePercent, D247 §6), as the core judges
 * the link it remembers (playback.go's linkCarries). A link not measured yet carries nothing.
 */
export function linkCarriesTheOriginal(
  linkKbps: number | undefined,
  fileKbps: number,
  sharePercent: number,
): boolean {
  return linkKbps !== undefined && fileKbps * 100 <= Math.round(linkKbps) * sharePercent;
}

/** A film on its ladder waiting for the original its person chose (the decision's isOriginalWaitingForTheLink). */
export interface OriginalWaiting {
  readonly fileKbps: number;
  readonly sharePercent: number;
  /** The link's speed as the playback measures it, undefined until it can say (D247 §1). */
  readonly readLinkKbps: () => number | undefined;
}

/**
 * Calls onCarried once, for a film waiting for the original (waiting given), when its link has carried
 * the file (linkCarriesTheOriginal) at every look for originalAfterLinkCarriesMs and this device's
 * data cap lets the file play (isWithinCap, D489): the page then asks for the original from where the
 * film stands (the owner, 2026-11-15: "Better to start on a lower rung and then go back up later").
 * A look the link falls short at starts the hold again; a cap under the file holds it back until
 * lifted.
 */
export function useOriginalOnceTheLinkCarries(
  waiting: OriginalWaiting | undefined,
  onCarried: () => void,
  caps: DataCaps = deviceDataCaps(),
): void {
  const readLinkKbps = useEffectEvent(() => waiting?.readLinkKbps());
  const carried = useEffectEvent(() => {
    onCarried();
  });
  const isWaiting = waiting !== undefined;
  const fileKbps = waiting?.fileKbps ?? 0;
  const sharePercent = waiting?.sharePercent ?? 0;
  useEffect(() => {
    if (!isWaiting) {
      return undefined;
    }
    let carriedMs = 0;
    const look = setInterval(() => {
      const isCarried = linkCarriesTheOriginal(readLinkKbps(), fileKbps, sharePercent);
      carriedMs = isCarried ? carriedMs + linkLookEveryMs : 0;
      if (carriedMs >= originalAfterLinkCarriesMs && isWithinCap(fileKbps, caps.capKbps())) {
        clearInterval(look);
        carried();
      }
    }, linkLookEveryMs);
    return () => {
      clearInterval(look);
    };
  }, [isWaiting, fileKbps, sharePercent, caps]);
}
