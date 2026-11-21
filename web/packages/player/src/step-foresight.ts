// SPDX-License-Identifier: MPL-2.0

/**
 * A chunked player's word ahead of time that it will step down for its buffer's memory (D532 §1):
 * the picture it will step down to, by the stream its playlist names, and where in the film. The
 * core starts that quality's run from there before the player asks for any of it: a Fire TV stepping
 * between the film's own picture and 720p met 720p unmade at each step down, its run stopped at the
 * step up, and stalled with only the picture it had buffered (film 200 on staging, 2026-11-21).
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

/**
 * The stream a quality's playlist address names: the folder its stream.m3u8 is in, as the core's
 * master playlist writes it ("720p/stream.m3u8"); undefined for an address naming none.
 */
export function pictureStreamOf(uri: string | undefined): string | undefined {
  const parts = uri?.split(/[?#]/u)[0]?.split("/");
  if (parts === undefined || parts.length < 2 || parts.at(-1) !== "stream.m3u8") {
    return undefined;
  }
  const stream = parts.at(-2);
  return stream === "" ? undefined : stream;
}

/** Tells video each step-down its player foresees, and when it foresees none (playerForesawStepDownEvent). */
export function stepDownForeseer(
  video: Pick<HTMLVideoElement, "dispatchEvent">,
): (step: ForeseenStepDown | undefined) => void {
  return (step) => {
    video.dispatchEvent(new CustomEvent(playerForesawStepDownEvent, { detail: step ?? null }));
  };
}
