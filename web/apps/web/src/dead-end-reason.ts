// SPDX-License-Identifier: MPL-2.0

/**
 * Why a page found its core but reached it by no way, as far as the page can tell (D398): whether
 * this network has IPv6, judged by the family the discovery saw the page's request from and how the
 * core's IPv6 names failed; each way tried and how it ended; and the cause its person is told. A phone
 * on mobile data in Australia was told only that its network could not reach a core with no IPv4 way
 * in (navigation-and-widgets-answers-2026-10-27.md, "Reaching production from outside" item 2).
 */

import { endingOf, type DeadEndReason, type DirectOutcome, type HasIpv6 } from "@orpheum/sdk";

import type { Target } from "./discovery-start.ts";

/** One way tried at a dead end: its kind, and how it ended. */
type Tried = DeadEndReason["tried"][number];

/** The kind of the target's name url: "lan", "ipv6" or "ipv4", or "direct" when it is none of them. */
function kindOf(target: Target, url: string): string {
  if (target.lanUrls.includes(url)) {
    return "lan";
  }
  if (target.families?.ipv6Urls.includes(url) === true) {
    return "ipv6";
  }
  return target.families?.ipv4Urls.includes(url) === true ? "ipv4" : "direct";
}

/**
 * Whether this network has IPv6 (D398 §1): yes when the discovery saw the page over IPv6, or one of
 * the core's IPv6 names was reached at all; no when the discovery saw it over IPv4 and every IPv6
 * name tried failed at once, as a name of a family the network has no route for does; else unknown,
 * as when an IPv6 name only went unanswered, which a router dropping it gives too.
 */
function hasIpv6Of(target: Target, tried: readonly Tried[]): HasIpv6 {
  const seenOver = target.families?.seenOver;
  const ipv6 = tried.filter((each) => each.kind === "ipv6");
  if (
    seenOver === "ipv6" ||
    ipv6.some((each) => each.ended === "answered" || each.ended === "refused")
  ) {
    return "yes";
  }
  return seenOver === "ipv4" &&
    ipv6.length > 0 &&
    ipv6.every((each) => each.ended === "unreachable")
    ? "no"
    : "unknown";
}

/**
 * Why nothing reached target, from what came of each name raced (reachCore's tellRaced): undefined
 * when the race was never decided. WebRTC is counted tried, and failed, once no name answered.
 */
export function deadEndReasonOf(
  target: Target,
  raced: readonly DirectOutcome[] | undefined,
): DeadEndReason {
  const named = (raced ?? []).map((each): Tried => ({
    kind: kindOf(target, each.url),
    ended: endingOf(each.outcome),
  }));
  const isWebrtcTried = raced !== undefined && named.every((each) => each.ended !== "answered");
  const tried = isWebrtcTried ? [...named, { kind: "webrtc", ended: "failed" }] : named;
  const ipv6 = hasIpv6Of(target, tried);
  const hasIpv4Way = (target.families?.ipv4Urls.length ?? 0) > 0;
  return { ipv6, tried, cause: ipv6 === "no" && !hasIpv4Way ? "noIpv6" : "unknown" };
}

/** The reason of a dead end the page knows nothing of: the discovery itself not reached. */
export const unknownDeadEnd: DeadEndReason = { ipv6: "unknown", tried: [], cause: "unknown" };
