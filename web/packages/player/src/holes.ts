// SPDX-License-Identifier: MPL-2.0

import { playerMovedItselfEvent } from "@orpheum/sdk";

import { seekTo } from "./report.ts";

/** How often the position is looked at. */
const lookEveryMs = 500;

/**
 * How many looks a video must stand still in a hole before it is stepped over it: hls.js's own gap
 * controller looks every 100 ms and steps a hole it can at once, so one it has left for a second it
 * will not step. A justified constant.
 */
const stillLooksBeforeStep = 2;

/**
 * The widest hole stepped over, in seconds: hls.js's own bound for a hole it steps where its
 * maxBufferHole is smaller ("holeEnd - holeStart < 1", gap-controller.ts). A wider one is a stretch
 * still to load, not a hole.
 */
const widestHoleS = 1;

/** How far past a hole's end the video is put, as hls.js's skipBufferHolePadding does. */
const pastTheHoleS = 0.1;

/** The part of a video element a hole is found and stepped over by. */
export type HoleMedia = Pick<
  HTMLMediaElement,
  "currentTime" | "paused" | "seeking" | "ended" | "buffered" | "dispatchEvent"
>;

/**
 * Steps the video over a hole in what it has loaded when, asked to play and not seeking, it has stood
 * still in it for a second: hls.js steps over a hole only within a piece it still has, and after the
 * browser's bufferFullError and its flush it had none there, while a hole wider than its
 * maxBufferHole (0.1 s) it never nudges across, so the video stood still for good 0.12 s short of its
 * next picture (the owner's Fire TV, 2026-11-11: at 200.66 s, the next at 200.78 s; idiosyncrasies.md,
 * "hls.js leaves a hole a flush made"). The step is told first (playerMovedItselfEvent), as hls.js's
 * own are, so it is no jump, and noted. Returns the stopping.
 */
export function stepOverHoles(media: HoleMedia, note: (line: string) => void): () => void {
  let still: { positionS: number; looks: number } | undefined;
  const timer = setInterval(() => {
    const positionS = media.currentTime;
    if (media.paused || media.seeking || media.ended || still?.positionS !== positionS) {
      still = { positionS, looks: 0 };
      return;
    }
    still.looks += 1;
    const nextS = holeEndAt(media.buffered, positionS);
    if (still.looks < stillLooksBeforeStep || nextS === undefined) {
      return;
    }
    media.dispatchEvent(new CustomEvent(playerMovedItselfEvent, { detail: "hole" }));
    seekTo(media, (nextS + pastTheHoleS) * 1000);
    note(
      `stepped over a ${(nextS - positionS).toFixed(2)} s hole at ${positionS.toFixed(2)} s, stood still in it`,
    );
    still = undefined;
  }, lookEveryMs);
  return () => {
    clearInterval(timer);
  };
}

/**
 * Where the hole positionS stands in ends, the start of the next loaded range, where nothing is
 * loaded at positionS and that start is no further than widestHoleS; undefined otherwise.
 */
export function holeEndAt(
  buffered: Pick<TimeRanges, "length" | "start" | "end">,
  positionS: number,
): number | undefined {
  for (let index = 0; index < buffered.length; index++) {
    const startS = buffered.start(index);
    if (startS <= positionS && positionS < buffered.end(index)) {
      return undefined; // loaded where it stands: no hole
    }
    if (startS > positionS) {
      return startS - positionS <= widestHoleS ? startS : undefined;
    }
  }
  return undefined;
}
