// SPDX-License-Identifier: MPL-2.0

import type { components } from "@orpheum/sdk";

/** A title's detail, as the server sends it (title.tsx's TitleDetail). */
type TitleDetail = components["schemas"]["ItemBody"];

/** The file a title's Play goes to first, which its page warms up; undefined if none plays now. */
export function firstPlayableFile(title: TitleDetail): number | undefined {
  if (title.unavailable !== undefined) {
    return undefined;
  }
  return title.versions?.[0]?.files?.[0]?.id;
}
