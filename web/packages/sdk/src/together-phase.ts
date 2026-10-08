// SPDX-License-Identifier: MPL-2.0

/**
 * Where a following player is in correcting its drift, as one state with its transitions: what the
 * follower once kept in five fields set and cleared in different places, one of which outlived the
 * move it was about and stopped a host's play 0.4 s later (the sync review, 2026-10-08). Each phase
 * holds only what it needs; nothing here touches a player.
 */

/** Where a following player is in correcting its drift. */
export type CorrectionPhase =
  /** Nothing under way: its drift is read and corrected as it comes. */
  | { readonly kind: "following" }
  /** Playing at another rate than the group's until untilMs (monotonic), to close a drift. */
  | { readonly kind: "nudging"; readonly untilMs: number }
  /**
   * Its rate just went back to the group's, behindMs behind: once its position settles, what that
   * cost it is learned (learnedLossMs).
   */
  | { readonly kind: "returning"; readonly behindMs: number }
  /**
   * It skipped to where it would be behindMs behind (ahead, so negative), correcting by seeks alone:
   * once it settles, where the skip really landed is learned (D268 §2).
   */
  | { readonly kind: "skipping"; readonly behindMs: number }
  /** It jumped ahead to atMs to catch up, and waits there, paused and loading, for the group. */
  | { readonly kind: "waitingAhead"; readonly atMs: number };

/** The phase a following player starts in. */
export const startingPhase: CorrectionPhase = { kind: "following" };

/** What happens to a following player that moves its phase. */
export type PhaseEvent =
  /** A nudge began, lasting until untilMs. */
  | { readonly kind: "nudged"; readonly untilMs: number }
  /**
   * Its rate changed: back to the group's, returnedBehindMs behind, or, undefined, to another rate
   * than the group's.
   */
  | { readonly kind: "rateChanged"; readonly returnedBehindMs: number | undefined }
  /** It sought somewhere: what a seek moves is no return's or earlier skip's cost. */
  | { readonly kind: "sought" }
  /** It skipped, correcting by seeks alone, to be behindMs behind. */
  | { readonly kind: "skipped"; readonly behindMs: number }
  /** It jumped ahead to atMs to wait for the group there. */
  | { readonly kind: "jumpedAhead"; readonly atMs: number }
  /** Settled, what its last return or skip cost it was learned. */
  | { readonly kind: "learned" }
  /** It stopped waiting ahead: the group arrived, passed it, sought away, or stopped. */
  | { readonly kind: "stoppedWaiting" };

/** The phase after event, from phase. */
export function nextPhase(phase: CorrectionPhase, event: PhaseEvent): CorrectionPhase {
  switch (event.kind) {
    case "nudged":
      return { kind: "nudging", untilMs: event.untilMs };
    case "rateChanged":
      // To another rate: a return not yet learned is no return now.
      return event.returnedBehindMs === undefined
        ? ending(phase, ["returning"])
        : { kind: "returning", behindMs: event.returnedBehindMs };
    case "skipped":
      return { kind: "skipping", behindMs: event.behindMs };
    case "jumpedAhead":
      return { kind: "waitingAhead", atMs: event.atMs };
    case "sought":
    case "learned":
      return ending(phase, ["returning", "skipping"]);
    case "stoppedWaiting":
      return ending(phase, ["waitingAhead"]);
  }
}

/** Following, if phase is one of kinds; else phase as it is. */
function ending(
  phase: CorrectionPhase,
  kinds: readonly CorrectionPhase["kind"][],
): CorrectionPhase {
  return kinds.includes(phase.kind) ? startingPhase : phase;
}
