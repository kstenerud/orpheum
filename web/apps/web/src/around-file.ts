// SPDX-License-Identifier: MPL-2.0

/**
 * What plays after a file and before it, as the server says it (GET /files/{id}/up-next): read
 * apart from the Up next offer (up-next.tsx), which draws over the player and loads it, so a
 * title's page, which names its next episode, does not wait on hls.js (the web client's
 * architecture audit of 2026-11-10, Playback 2).
 */

import type { components } from "@orpheum/sdk";

import { dataOrThrow } from "./problem.ts";
import { useServices } from "./services.tsx";
import { useResource } from "./use-resource.ts";

/** What plays after a file, as the server offers it (GET /files/{id}/up-next). */
export type UpNext = components["schemas"]["UpNextBody"];

/** What plays before a file, its previous in the sequence (GET /files/{id}/up-next, D407 §2). */
export type Previous = components["schemas"]["NextUpBody"];

/** What plays after fileId and before it, each undefined where nothing does. */
export interface AroundFile {
  readonly upNext: UpNext | undefined;
  readonly previous: Previous | undefined;
}

/**
 * What plays after fileId and before it, undefined while unknown; asked for once isAsked, nothing
 * before.
 */
export function useAroundFile(fileId: number, isAsked: boolean): AroundFile | undefined {
  const { api } = useServices();
  const [around] = useResource(
    `up-next/${String(fileId)}${isAsked ? "" : "/not-yet"}`,
    async (): Promise<AroundFile | undefined> => {
      if (!isAsked) {
        return undefined;
      }
      const answer = dataOrThrow(
        await api.GET("/files/{id}/up-next", { params: { path: { id: fileId } } }),
      );
      return { upNext: answer.upNext ?? undefined, previous: answer.previous ?? undefined };
    },
  );
  return around.state === "loaded" ? around.value : undefined;
}

/**
 * What plays after fileId, undefined while unknown and where nothing does; asked for once isAsked,
 * nothing before.
 */
export function useUpNext(fileId: number, isAsked = true): UpNext | undefined {
  return useAroundFile(fileId, isAsked)?.upNext;
}

/** What plays before fileId, undefined while unknown and where nothing does. */
export function usePrevious(fileId: number): Previous | undefined {
  return useAroundFile(fileId, true)?.previous;
}
