// SPDX-License-Identifier: MPL-2.0

import type { Api } from "./client.ts";
import type { components } from "./gen/api.ts";
import type { PlayableFormats } from "./playable-formats.ts";
import { dataOrThrow } from "./problem.ts";

/** What a player asks the core of a film it is to play: POST /playback's body. */
export type PlaybackAsked = components["schemas"]["StartPlaybackInputBody"];

/** How the core decides a film plays for the player that asked. */
export type PlaybackDecision = components["schemas"]["StartPlaybackOutputBody"];

/**
 * Asks the core how a film plays for this player (D555 §3): the one way the Play page and the
 * Google Cast receiver ask, each saying in asked what it says of itself.
 */
export async function requestPlayback(api: Api, asked: PlaybackAsked): Promise<PlaybackDecision> {
  return dataOrThrow(await api.POST("/playback", { body: asked }));
}

/**
 * Asks how the film a Watch Together session has on plays for a viewer, by their membership's
 * credential (D246 §3): the same decision as POST /playback. The one way a guest's page and a
 * Chromecast taking a guest's part ask (D555 §3, D518 §1).
 */
export async function requestTogetherPlayback(
  api: Api,
  credential: string,
  playableFormats: PlayableFormats,
): Promise<PlaybackDecision> {
  return dataOrThrow(
    await api.POST("/together/playback", {
      headers: { authorization: `Bearer ${credential}` },
      body: { playableFormats },
    }),
  );
}
