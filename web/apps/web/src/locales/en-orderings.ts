// SPDX-License-Identifier: MPL-2.0

/**
 * The English catalogue's orderings part: a series' orderings, the choice of one, and an episode's
 * place in it (episode-orderings.md §4, §7 Q8), kept apart so the catalogue fits in one read (en.ts).
 */
export const enOrderings = {
  ordering: {
    label: "Order",
    aired: "Aired",
    option: "{{label}} ({{kind}}) · episodes here: {{count}}",
    optionListed: "{{label}} ({{kind}}) · episodes listed: {{count}}",
    failed: "The order could not be changed: {{why}}",
    fetching: "Fetching this order; its episodes show here once it is fetched.",
    part: "Part {{number}}",
    groupPlace: "{{group}} · {{number}}",
    episode: "Episode {{number}}",
    airedPlace: "Aired as S{{season}}E{{number}}",
    notInThisOrder: "Not in this order",
    notInThisOrderNote: "Not in the order you watch this series in",
  },
  orderingKind: {
    aired: "Aired",
    airedAlternate: "Original air date",
    absolute: "Absolute",
    dvd: "DVD",
    digital: "Digital",
    storyArc: "Story arc",
    production: "Production",
    tv: "TV",
    authored: "Your own",
  },
};
