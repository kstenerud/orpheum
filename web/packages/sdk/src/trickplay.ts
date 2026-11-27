// SPDX-License-Identifier: MPL-2.0

import type { Api } from "./client.ts";
import type { components } from "./gen/api.ts";
import { dataOrThrow } from "./problem.ts";

/** A film's scrub-bar pictures as made so far (GET /files/{id}/trickplay; trickplay.md §5). */
export interface ScrubPictures {
  /** The time between pictures: picture n is of n·intervalMs. */
  readonly intervalMs: number;
  readonly pictureWidth: number;
  readonly pictureHeight: number;
  /** A sheet's pictures across and down, row by row. */
  readonly columns: number;
  readonly rows: number;
  /** The whole film's pictures, made or not. */
  readonly pictureCount: number;
  /** Each sheet made, in order; a stretch whose sheet is missing has no pictures yet. */
  readonly sheets: readonly ScrubSheet[];
}

/** One sheet made: its index from the film's start and its image's address. */
export type ScrubSheet = components["schemas"]["TrickplaySheetBody"];

/**
 * The scrub-bar pictures of the file fileId made so far; undefined while none is (the core's 404),
 * the scrub bar then showing the time alone.
 */
export async function readScrubPictures(
  api: Api,
  fileId: number,
): Promise<ScrubPictures | undefined> {
  const answer = await api.GET("/files/{id}/trickplay", { params: { path: { id: fileId } } });
  if (answer.response.status === 404) {
    return undefined;
  }
  const pictures = dataOrThrow(answer);
  return { ...pictures, sheets: pictures.sheets ?? [] };
}
