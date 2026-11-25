// SPDX-License-Identifier: MPL-2.0

import {
  requestTogetherPlayback as requestTogetherPlaybackOfTheCore,
  type Api,
  type MediaCredentials,
  type PlayableFormats,
} from "@orpheum/sdk";

import type { AroundFile } from "./around-file.ts";
import type { PlaybackDecision } from "./watching.ts";
import { dataOrThrow } from "./problem.ts";
import type { Session } from "./session.ts";
import { useServices } from "./services.tsx";
import { useResource, type Resource } from "./use-resource.ts";

/**
 * Asks how the film the host has on plays for a viewer, by their membership credential (D246 §3):
 * the same decision as POST /playback, and the viewer cookie the byte paths take. No click sends
 * it: the viewer's page asks each time the group's film changes, since a playback of the film
 * before stops once another is on. A viewer comes by a link, so is taken to be outside the house.
 */
export async function requestTogetherPlayback(
  api: Api,
  credential: string,
  asked: { readonly playableFormats: PlayableFormats },
): Promise<PlaybackDecision> {
  mediaViewer.credential = credential;
  return await requestTogetherPlaybackOfTheCore(api, credential, asked.playableFormats);
}

/**
 * What plays after the session's film and before it, in its host's ordering, for a viewer whose link
 * lets them put either on (GET /together/up-next, D471 §3): each undefined where nothing does.
 */
export async function requestTogetherAround(
  api: Api,
  headers: Readonly<Record<string, string>>,
): Promise<AroundFile> {
  const answer = dataOrThrow(await api.GET("/together/up-next", { headers }));
  return { upNext: answer.upNext ?? undefined, previous: answer.previous ?? undefined };
}

/** The membership whose playback this page asked for last, which its media loads are signed with. */

const mediaViewer: { credential: string | undefined } = { credential: undefined };

/**
 * The viewer's membership this page's media loads carry beside its own sign-in: on another origin
 * than the core's no viewer cookie reaches the core, and a guest signed in to an account of their own
 * found the film gone (staging, 2026-10-14). Undefined until the page plays as a viewer.
 */
export function viewerForMedia(): string | undefined {
  return mediaViewer.credential;
}

/** What the page's media loads are signed with, read at each load: its session, and its viewer's. */
export function mediaCredentialsOf(session: Pick<Session, "token">): () => MediaCredentials {
  return () => ({ token: session.token(), viewer: viewerForMedia() });
}

/**
 * How the group's film plays for this guest, as the server decides, asked for once per film: a
 * playback opened is this guest's alone, kept for no other reader (D478).
 */
export function useGroupFilmDecision(film: {
  readonly fileId: number;
  readonly credential: string;
}): Resource<PlaybackDecision> {
  const { api, playableFormats } = useServices();
  const [decision] = useResource(
    `together-play/${String(film.fileId)}`,
    () => requestTogetherPlayback(api, film.credential, { playableFormats: playableFormats() }),
    { isSingleUse: true },
  );
  return decision;
}
