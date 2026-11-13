// SPDX-License-Identifier: MPL-2.0

import type { Api } from "@orpheum/sdk";

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
}

/**
 * Says what the video's player wants as it pauses, plays again and seeks, so the server finishes
 * only what a paused player keeps ahead, and stops the runs a seek left at once (D309 §2, §7).
 * Returns the stopping.
 */
export function sayInterestOf(
  video: HTMLVideoElement,
  say: (said: SaidInterest) => void,
): () => void {
  const at = (): number => Math.round(video.currentTime * 1000);
  const listeners: Record<string, () => void> = {
    pause: () => {
      // The end pauses the video too: nothing is left ahead to keep. The pause that keeps the
      // press for the start is not the person's.
      if (!isPressKeepersPause(video) && !video.ended) {
        say({ positionMs: at(), isPaused: true });
      }
    },
    playing: () => {
      say({ positionMs: at(), isPaused: false });
    },
    seeking: () => {
      say({ positionMs: at(), isPaused: video.paused });
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
