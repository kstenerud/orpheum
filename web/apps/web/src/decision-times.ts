// SPDX-License-Identifier: MPL-2.0

/**
 * Each playback's decision times, kept from its asking until its reporting takes them: apart from
 * the Play page's reporting (play-state-video-events.ts), which loads the player, so a film's page,
 * which notes them as it warms its film up (watching.ts), does not wait on hls.js (the web client's
 * architecture audit of 2026-11-10, Playback 2).
 */

/**
 * When a playback's player asked how its file plays, and was answered, on the page's clock; and the
 * last steps of the film page's preload that loaded its first buffer, where one did (D260 §3).
 */
export interface DecisionTimes {
  readonly askedMs: number;
  readonly decidedMs: number;
  readonly preloadSteps?: string | undefined;
}

/** How many playbacks' decision times a tab keeps, the newest: more than a person starts at once. */
export const decisionTimesKept = 8;

/**
 * Each playback's decision times, by media token, until its reporting takes them, the newest
 * decisionTimesKept.
 */
const decisionTimesByToken = new Map<string, DecisionTimes>();

/**
 * Keeps when a playback's player asked how its file plays and was answered, askedMs and decidedMs
 * from performance.now(), so its reports can say how long it took to start playing; and what the
 * film page's preload did before it, where one did.
 */
export function noteDecisionTimes(mediaToken: string, times: DecisionTimes): void {
  decisionTimesByToken.set(mediaToken, times);
  // A film page's warm-up that is never played is never told: the oldest are let go of, so the
  // tab keeps a few at most (the M4 closing audit, 2026-10-08).
  for (const oldest of decisionTimesByToken.keys()) {
    if (decisionTimesByToken.size <= decisionTimesKept) {
      break;
    }
    decisionTimesByToken.delete(oldest);
  }
}

/** When the playback mediaToken names was asked for and decided, until its start is told. */
export function decisionTimesOf(mediaToken: string): DecisionTimes | undefined {
  return decisionTimesByToken.get(mediaToken);
}

/** Forgets when the playback mediaToken names was asked for and decided, once its start is told. */
export function forgetDecisionTimes(mediaToken: string): void {
  decisionTimesByToken.delete(mediaToken);
}
