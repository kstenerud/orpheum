// SPDX-License-Identifier: MPL-2.0

import type { Api, PlayableFormats } from "@orpheum/sdk";

import type { AudioOutput } from "./accounts.ts";
import type { PlaybackDecision } from "./play.tsx";
import { dataOrThrow } from "./problem.ts";
import { membershipHeaders } from "./together-storage.ts";

/**
 * Asks how the film the host has on plays for a viewer, by their membership credential (D246 §3):
 * the same decision as POST /playback, and the viewer cookie the byte paths take. No click sends
 * it: the viewer's page asks each time the group's film changes, since a playback of the film
 * before stops once another is on. A viewer comes by a link, so is taken to be outside the house.
 */
export async function requestTogetherPlayback(
  api: Api,
  credential: string,
  asked: { readonly playableFormats: PlayableFormats; readonly audioOutput: AudioOutput },
): Promise<PlaybackDecision> {
  return dataOrThrow(
    await api.POST("/together/playback", {
      headers: membershipHeaders(credential),
      body: {
        playableFormats: asked.playableFormats,
        network: "remote",
        audioOutput: asked.audioOutput,
      },
    }),
  );
}
