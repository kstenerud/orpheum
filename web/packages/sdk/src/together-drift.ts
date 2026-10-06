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
 * that took transitMs (none when not given) to arrive: a playing group has moved on at its rate since it was sent (D66:
 * no clock is shared; the round trip bounds how old a message is). Without the transit, every
 * follower sat behind the group by its message's way from the server, 530 ms on a congested link
 * (M4's acceptance, 2026-10-06).
 */
export function expectedPositionMs(
  group: GroupView,
  at: { readonly receivedAtMs: number; readonly nowMs: number; readonly transitMs?: number },
): number {
  const { receivedAtMs, nowMs, transitMs = 0 } = at;
  return group.state === "Playing"
    ? group.positionMs + (nowMs - receivedAtMs + transitMs) * group.rate
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
 * drift is closed by the nudges that follow (D250 §1).
 */
export function driftCorrection(
  drift: { readonly expectedMs: number; readonly actualMs: number; readonly groupRate: number },
  tuning: SyncTuning,
): DriftCorrection {
  const behindMs = drift.expectedMs - drift.actualMs;
  if (Math.abs(behindMs) <= tuning.leaveDriftUpToMs) {
    return { kind: "none" };
  }
  if (Math.abs(behindMs) > tuning.seekDriftBeyondMs) {
    return { kind: "seek", toMs: drift.expectedMs };
  }
  const most = tuning.nudgeMostPercent / 100;
  const factor = Math.min(1 + most, Math.max(1 - most, 1 + behindMs / nudgeForMs));
  return { kind: "nudge", rate: drift.groupRate * factor };
}
