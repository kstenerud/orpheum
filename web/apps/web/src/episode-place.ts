// SPDX-License-Identifier: MPL-2.0

import type { TFunction } from "i18next";

/** The ordering every series has, which a person watches it in until they choose another (D159). */
export const orderingAired = "aired";

/** An episode's place in an ordering: its group, its number in it, and the group's name if any. */
export interface OrderingPlace {
  readonly groupNumber: number;
  readonly numberInGroup: number;
  /** The ordering's name for the group ("Disc 2"), where it names one. */
  readonly groupName?: string | undefined;
  /** Whether the ordering is one run, whose one group is no heading (an absolute order). */
  readonly isOnlyGroup?: boolean | undefined;
}

/**
 * How a place is labelled, in the ordering the person watches its series in (episode-orderings.md
 * §7 Q8): in aired "S1E3"; in another its group's name and the number in it, "Disc 2 · 3", or
 * "Episode 812" in an ordering of one run, or "Part 2 · 3" where the ordering names no group.
 */
export function placeCode(
  t: TFunction,
  ordering: string | undefined,
  place: OrderingPlace,
): string {
  if (ordering === undefined || ordering === orderingAired) {
    return t("title.episodeCode", { season: place.groupNumber, number: place.numberInGroup });
  }
  if (place.isOnlyGroup === true) {
    return t("ordering.episode", { number: place.numberInGroup });
  }
  return t("ordering.groupPlace", {
    group: groupName(t, ordering, { number: place.groupNumber, name: place.groupName }),
    number: place.numberInGroup,
  });
}

/**
 * The name of a group, as its heading says it: in aired "Season 2", or "Specials" for season 0; in
 * another ordering its own name for it ("Disc 2"), else "Part 2".
 */
export function groupName(
  t: TFunction,
  ordering: string | undefined,
  group: { readonly number: number; readonly name?: string | undefined },
): string {
  if (ordering === undefined || ordering === orderingAired) {
    return group.number === 0 ? t("title.specials") : t("title.season", { season: group.number });
  }
  return group.name ?? t("ordering.part", { number: group.number });
}
