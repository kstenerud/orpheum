// SPDX-License-Identifier: MPL-2.0

/**
 * A chunked player's word ahead of time that it will step down for its buffer's memory (D532 §1):
 * the picture it will step down to, by the stream its playlist names, and where in the film. The
 * core starts that quality's run from there before the player asks for any of it: a Fire TV
 * stepping between the film's own picture and 720p met 720p unmade at each step down, its run
 * stopped at the step up, and stalled with only the picture it had buffered (film 200 on staging,
 * 2026-11-21). It is the sdk's, not the player's, so a film's page reads it without loading hls.js
 * (home-imports.test.ts).
 */

/** The picture a player will step down to, as its playlist names the stream, and from where. */
export interface ForeseenStepDown {
  readonly pictureStream: string;
  readonly fromMs: number;
}

/**
 * The event the player fires at its video each time the step-down it foresees changes: its detail
 * the ForeseenStepDown, or null where it foresees none any longer.
 */
export const playerForesawStepDownEvent = "orpheumplayerforesawstepdown";

/** Tells video each step-down its player foresees, and when it foresees none (playerForesawStepDownEvent). */
export function stepDownForeseer(
  video: Pick<HTMLVideoElement, "dispatchEvent">,
): (step: ForeseenStepDown | undefined) => void {
  return (step) => {
    video.dispatchEvent(new CustomEvent(playerForesawStepDownEvent, { detail: step ?? null }));
  };
}

/** The step-down a playerForesawStepDownEvent says; undefined for none, or another event. */
export function foreseenStepDownOf(event: Event): ForeseenStepDown | undefined {
  const detail: unknown = event instanceof CustomEvent ? event.detail : undefined;
  if (typeof detail !== "object" || detail === null) {
    return undefined;
  }
  const { pictureStream, fromMs } = Object.fromEntries(Object.entries(detail));
  return typeof pictureStream === "string" && typeof fromMs === "number"
    ? { pictureStream, fromMs }
    : undefined;
}
