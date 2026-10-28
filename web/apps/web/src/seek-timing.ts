// SPDX-License-Identifier: MPL-2.0

/**
 * How long the person's seeks take to play again, and what the start gate waited for before a start
 * or a seek played, for the play report (play-state.ts).
 */

import type { PlayReporter } from "@orpheum/player";
import {
  chunkLoadedOf,
  loadedAheadMsOf,
  playerLoadedChunkEvent,
  startGateOf,
  type components,
} from "@orpheum/sdk";

import { stepsOf } from "./steps-of.ts";

/** The video's position, in milliseconds. */
function positionMs(video: HTMLVideoElement): number {
  return video.currentTime * 1000;
}

/**
 * How much film the video had loaded ahead as it played, and, where its start gate waited before it
 * played, what it waited for and the next chunk's expected load set against it (start-gate.ts in the
 * SDK): what the start's and a seek's log lines say, so a reader sees the gate work.
 */
export function startGateFieldsOf(
  video: HTMLVideoElement,
): Pick<
  components["schemas"]["SeekBody"],
  "loadedAheadMs" | "waitedForAheadMs" | "nextChunkLoadMs" | "nextChunkLoadFrom"
> {
  const wait = startGateOf(video).lastWait();
  return {
    loadedAheadMs: loadedAheadMsOf(video),
    ...(wait === undefined ? {} : { waitedForAheadMs: wait.waitedForAheadMs }),
    ...(wait?.nextChunkLoadMs === undefined ? {} : { nextChunkLoadMs: wait.nextChunkLoadMs }),
    ...(wait === undefined ? {} : { nextChunkLoadFrom: wait.nextChunkLoadFrom }),
  };
}

/**
 * Times each of the person's seeks, from its last seeking (a drag seeks again and again) to playing,
 * or, for a paused video, to being able to play, and tells the reporter (the owner, 2026-10-12: a
 * jump that felt slow could not be measured): with the report the started event sends, or at once
 * when paused, with when its first chunks of picture and sound arrived, to say which it waited on;
 * a seek its start gate holds paused until enough is loaded is timed to its playing.
 * Its listeners go on the video before the started event's.
 */
export function timeSeeks(
  video: HTMLVideoElement,
  reporter: Pick<PlayReporter, "recordSeekReady" | "flush">,
): {
  readonly seeking: (fromMs: number) => void;
  readonly listeners: Record<string, (event: Event) => void>;
} {
  let pending:
    | { readonly fromMs: number; atMs: number; pictureAfterMs?: number; soundAfterMs?: number }
    | undefined;
  const ready = (): boolean => {
    if (pending === undefined || video.seeking) {
      return false;
    }
    reporter.recordSeekReady({
      fromPositionMs: Math.max(0, Math.round(pending.fromMs)),
      toPositionMs: Math.max(0, Math.round(positionMs(video))),
      readyAfterMs: Math.max(0, Math.round(performance.now() - pending.atMs)),
      ...(video.paused ? { isPaused: true } : {}),
      ...(pending.pictureAfterMs === undefined ? {} : { pictureAfterMs: pending.pictureAfterMs }),
      ...(pending.soundAfterMs === undefined ? {} : { soundAfterMs: pending.soundAfterMs }),
      ...stepsOf(video),
      ...startGateFieldsOf(video),
    });
    pending = undefined;
    return true;
  };
  return {
    listeners: {
      playing: () => {
        ready();
      },
      canplay: () => {
        if (video.paused && !startGateOf(video).isOwnPause() && ready()) {
          reporter.flush(positionMs(video));
        }
      },
      [playerLoadedChunkEvent]: (event: Event) => {
        const loaded = chunkLoadedOf(event);
        if (pending === undefined || loaded === undefined) {
          return;
        }
        const afterMs = Math.max(0, Math.round(performance.now() - pending.atMs));
        if (loaded.track === "video") {
          pending.pictureAfterMs ??= afterMs;
        } else {
          pending.soundAfterMs ??= afterMs;
        }
      },
    },
    seeking: (fromMs) => {
      const atMs = performance.now();
      if (pending === undefined) {
        pending = { fromMs, atMs };
      } else {
        pending.atMs = atMs;
      }
    },
  };
}
