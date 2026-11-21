// SPDX-License-Identifier: MPL-2.0

/**
 * What plays after a file and before it, as the server says it (GET /files/{id}/up-next): read
 * apart from the Up next offer (up-next.tsx), which draws over the player and loads it, so the
 * cast's remote does not wait on hls.js (the web client's architecture audit of 2026-11-10,
 * Playback 2). The remote reads it once for its ⏮ and its Up next; a title's page reads none, its
 * own answer carrying it (D477 §2).
 */

import type { components } from "@orpheum/sdk";
import { createContext, use, type Context } from "react";

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
 * The title the Play page's file was opened from (its route's itemId), undefined where none was
 * named: one file's bytes filed both as a film and as an episode play as the episode from its
 * series, and what plays around it is the episode's (B0 question 20).
 */
export const OpenedAsContext: Context<number | undefined> = createContext<number | undefined>(
  undefined,
);

/** The title the Play page's file was opened from (OpenedAsContext). */
export function useOpenedAs(): number | undefined {
  return use(OpenedAsContext);
}

/**
 * What plays after fileId and before it, undefined while unknown; asked for once isAsked, nothing
 * before, as the title it was opened from (OpenedAsContext). Where it cannot be read, nothing plays
 * after it nor before, as a next episode that cannot be read now is not offered (plans/up-next.md §7).
 */
export function useAroundFile(fileId: number, isAsked: boolean): AroundFile | undefined {
  const { api } = useServices();
  const openedAs = use(OpenedAsContext);
  const asItem = openedAs === undefined ? "" : `/as/${String(openedAs)}`;
  const [around] = useResource(
    `up-next/${String(fileId)}${asItem}${isAsked ? "" : "/not-yet"}`,
    async (): Promise<AroundFile | undefined> => {
      if (!isAsked) {
        return undefined;
      }
      const query = openedAs === undefined ? {} : { query: { item: openedAs } };
      const answer = dataOrThrow(
        await api.GET("/files/{id}/up-next", { params: { path: { id: fileId }, ...query } }),
      );
      return { upNext: answer.upNext ?? undefined, previous: answer.previous ?? undefined };
    },
  );
  if (around.state === "failed") {
    return nothingAround;
  }
  return around.state === "loaded" ? around.value : undefined;
}

/** What is around a file whose sequence cannot be read: nothing. */
const nothingAround: AroundFile = { upNext: undefined, previous: undefined };
