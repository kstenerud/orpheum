// SPDX-License-Identifier: MPL-2.0

/**
 * A following player's arithmetic: where its group is now, and what a drift from it calls for. No
 * player is touched here; together-follow.ts acts on the answers.
 */

import type { GroupView, SyncTuning } from "./together.ts";

/** How long one nudge of the rate lasts (watch-together.md, Sync: "for ~1 s"). */
export const nudgeForMs = 1000;

/**
 * Where a group is at the monotonic time nowMs, having been told it at receivedAtMs by a message
 * that took transitMs (none when not given) to arrive: a playing group has moved on at its rate
 * since it was sent (D66: no clock is shared; the round trip bounds how old a message is). A group
 * names no moment to start at since D371: it moves from when it was told. Without the transit, every
 * follower sat behind the group by its message's way from the server, 530 ms on a congested link
 * (M4's acceptance, 2026-10-06).
 */
export function expectedPositionMs(
  group: GroupView,
  at: { readonly receivedAtMs: number; readonly nowMs: number; readonly transitMs?: number },
): number {
  const sentAtMs = at.receivedAtMs - (at.transitMs ?? 0);
  return group.state === "Playing"
    ? group.positionMs + Math.max(0, at.nowMs - sentAtMs) * group.rate
    : group.positionMs;
}

/** What to do about the distance between where a player is and where the group is. */
export type DriftCorrection =
  | { readonly kind: "none" }
  | { readonly kind: "nudge"; readonly rate: number }
  | { readonly kind: "seek"; readonly toMs: number };

/**
 * The correction for a player at actualMs while the group is at expectedMs playing at groupRate:
 * none within tuning's leaveDriftUpToMs, a seek beyond its seekDriftBeyondMs, else a rate that
 * closes the gap over nudgeForMs, no further from the group's than its nudgeMostPercent: a larger
 * drift is closed by the nudges that follow (D250 §1). A player being nudged loses returnCostMs as
 * its rate goes back to the group's, as Safari does, some 220 ms each time: it is judged by where it
 * will be after that loss, so its nudge ends past the group by the loss and lands on it. Judged by
 * where it was, every nudge ended 220 ms behind, beyond the allowance, and an iPhone was nudged 40
 * times in two minutes, its sound stuttering at each (the owner's tests, 2026-10-08).
 */
export function driftCorrection(
  drift: { readonly expectedMs: number; readonly actualMs: number; readonly groupRate: number },
  tuning: SyncTuning,
  returnCostMs = 0,
): DriftCorrection {
  const behindMs = drift.expectedMs - drift.actualMs;
  if (Math.abs(behindMs) > tuning.seekDriftBeyondMs) {
    return { kind: "seek", toMs: drift.expectedMs };
  }
  const afterReturnMs = behindMs + returnCostMs;
  if (Math.abs(afterReturnMs) <= tuning.leaveDriftUpToMs) {
    return { kind: "none" };
  }
  const most = tuning.nudgeMostPercent / 100;
  const factor = Math.min(1 + most, Math.max(1 - most, 1 + afterReturnMs / nudgeForMs));
  return { kind: "nudge", rate: drift.groupRate * factor };
}

/** What to do, for a player that corrects by seeks alone, about its distance from the group. */
export type SeekOnlyCorrection =
  | { readonly kind: "none" }
  | { readonly kind: "skip"; readonly toMs: number }
  | { readonly kind: "seek"; readonly toMs: number };

/**
 * The correction for a player that corrects by seeks alone (D268 §2): none within tuning's
 * seekOnlyLeaveDriftUpToMs, a catch-up seek beyond its seekDriftBeyondMs, and between them a skip,
 * a small seek past the group by what its seeks have learned to lose, seekLossMs at groupRate, so it
 * lands on the group, as MediaSync's seek mode learns its overshoot.
 */
export function seekOnlyCorrection(
  drift: { readonly expectedMs: number; readonly actualMs: number; readonly groupRate: number },
  tuning: SyncTuning,
  seekLossMs: number,
): SeekOnlyCorrection {
  const offMs = Math.abs(drift.expectedMs - drift.actualMs);
  if (offMs > tuning.seekDriftBeyondMs) {
    return { kind: "seek", toMs: drift.expectedMs };
  }
  return offMs <= tuning.seekOnlyLeaveDriftUpToMs
    ? { kind: "none" }
    : { kind: "skip", toMs: drift.expectedMs + seekLossMs * drift.groupRate };
}

/** The most one move of a player is taken to lose it: beyond it, a stall. */
export const lossAtMostMs = 500;

/** What a player has learned one kind of its moves loses it, and from how many. */
export interface LearnedLoss {
  readonly costMs: number;
  readonly moves: number;
}

/**
 * What one kind of a player's moves loses it, learned from one more: where it was behind as it made
 * the move and where once its position settled, between 0 and lossAtMostMs. A rate going back to
 * the group's loses an iPhone some 220 ms (WebKit 163433); a skip lands where Safari's seek lands,
 * 100 to 500 ms off (hls.js#7616). The first move's loss is taken whole, the best there is, and each
 * one after averaged with what was learned (half and half), so a player that loses time learns it at
 * once and a wobble moves it little. A browser that loses nothing, Chrome or Firefox, learns about 0,
 * and nudges as before.
 */
export function learnedLossMs(
  learned: LearnedLoss,
  behindAtMoveMs: number,
  behindNowMs: number,
): LearnedLoss {
  const lostMs = Math.min(lossAtMostMs, Math.max(0, behindNowMs - behindAtMoveMs));
  const costMs = learned.moves === 0 ? lostMs : (learned.costMs + lostMs) / 2;
  return { costMs, moves: learned.moves + 1 };
}
