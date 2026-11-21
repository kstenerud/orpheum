// SPDX-License-Identifier: MPL-2.0

import {
  foreseenStepDownOf,
  playerForesawStepDownEvent,
  startGateOf,
  startGateWaitChangedEvent,
  type Api,
  type ForeseenStepDown,
} from "@orpheum/sdk";

import { isPausedAsAsked } from "./film-waits.tsx";
import { isPressKeepersPause } from "./start-refused.ts";

/**
 * What a player says it wants of its film's chunks (D309): where it plays from, and whether it is
 * paused. Its quality and its sound the server reads off the chunks it asks for, the first of a new
 * one saying it moved.
 */
export interface SaidInterest {
  readonly positionMs: number;
  readonly isPaused: boolean;
  /**
   * The sound it plays, by the stream its tracks name it: said by a player off the ladder that
   * switched its file's track in place, whose stall's offer then starts that sound.
   */
  readonly soundStream?: string;
  /**
   * The picture it will step down to for its buffer's memory, and where in the film, which the
   * server starts making ahead (D532 §1); absent for none, which ends one said before.
   */
  readonly stepDownPictureStream?: string;
  readonly stepDownFromMs?: number;
}

/**
 * Says what the video's player wants as it pauses, plays again and seeks, so the server finishes
 * only what a paused player keeps ahead, and stops the runs a seek left at once (D309 §2, §7).
 * Paused is said of a film paused as its person asked (isPausedAsAsked), never of one meant to play
 * that waits for enough to be loaded: a seek's, which its start gate pauses until then, was said
 * paused, so the core made it only the half minute ahead, and the run cut there paid FFmpeg's 5 to
 * 8 s open again (file 15497, production, 2026-11-17). Each step-down its player foresees is said
 * as it does, and with every word after while it stands (D532 §1). Returns the stopping.
 */
export function sayInterestOf(
  video: HTMLVideoElement,
  say: (said: SaidInterest) => void,
): () => void {
  const at = (): number => Math.round(video.currentTime * 1000);
  // Whether the film is paused, its start gate holding a seek, with the core told it plays.
  let isHeldSaidPlaying = false;
  let foreseen: ForeseenStepDown | undefined;
  const sayWith = (said: SaidInterest): void => {
    say(withForeseen(said, foreseen));
  };
  const listeners: Record<string, (event: Event) => void> = {
    pause: () => {
      // The end pauses the video too: nothing is left ahead to keep. The pause that keeps the
      // press for the start is not the person's, nor the start gate's own, holding a seek.
      if (isPressKeepersPause(video) || video.ended) {
        return;
      }
      if (startGateOf(video).isOwnPause()) {
        isHeldSaidPlaying = true;
        return;
      }
      sayWith({ positionMs: at(), isPaused: true });
    },
    playing: () => {
      isHeldSaidPlaying = false;
      sayWith({ positionMs: at(), isPaused: false });
    },
    seeking: () => {
      const isPaused = isPausedAsAsked(video);
      isHeldSaidPlaying = !isPaused && video.paused;
      sayWith({ positionMs: at(), isPaused });
    },
    // A held seek's wait given up, the film left paused (a press of Pause while it waits), is the
    // person's pause, which no pause event tells: the gate had paused it already.
    [startGateWaitChangedEvent]: () => {
      if (isHeldSaidPlaying && !video.ended && isPausedAsAsked(video)) {
        isHeldSaidPlaying = false;
        sayWith({ positionMs: at(), isPaused: true });
      }
    },
    [playerForesawStepDownEvent]: (event) => {
      foreseen = foreseenStepDownOf(event);
      if (!video.ended) {
        sayWith({ positionMs: at(), isPaused: isPausedAsAsked(video) });
      }
    },
  };
  for (const [event, listener] of Object.entries(listeners)) {
    video.addEventListener(event, listener);
  }
  return () => {
    for (const [event, listener] of Object.entries(listeners)) {
      video.removeEventListener(event, listener);
    }
  };
}

/** said, with the step-down foreseen where there is one (D532 §1). */
function withForeseen(said: SaidInterest, foreseen: ForeseenStepDown | undefined): SaidInterest {
  return foreseen === undefined
    ? said
    : { ...said, stepDownPictureStream: foreseen.pictureStream, stepDownFromMs: foreseen.fromMs };
}

/** Why a player let go of its playback (D309 §7): it left its page, or went on to another film. */
export type LeftFor = "left" | "replaced";

/**
 * Says to the server what the playback to.mediaToken names wants now, by to.headers where they carry a
 * Watch Together guest's membership (D247 §2); one that fails costs nothing.
 */
export function sendInterest(
  api: Api,
  to: {
    readonly mediaToken: string;
    readonly headers?: Readonly<Record<string, string>> | undefined;
  },
  said: SaidInterest,
): void {
  const { mediaToken, headers } = to;
  api
    .PUT("/playback/{token}/wants", {
      params: { path: { token: mediaToken } },
      body: said,
      ...(headers === undefined ? {} : { headers }),
    })
    .catch(() => undefined);
}

/**
 * Lets go of the playback mediaToken names, saying why (D258 §2, D309 §7). A leave that fails costs
 * only what the server keeps for the playback until it idles out.
 */
export function leavePlayback(api: Api, mediaToken: string, reason: LeftFor): void {
  api
    .DELETE("/playback/{token}", { params: { path: { token: mediaToken }, query: { reason } } })
    .catch(() => undefined);
}
