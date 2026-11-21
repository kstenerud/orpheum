// SPDX-License-Identifier: MPL-2.0

import type { GroupEvent, GroupView, RosterMember } from "@orpheum/sdk";
import type { TFunction } from "i18next";

import { clockText } from "./date-time-text.ts";
import { i18n } from "./i18n.ts";

/** The catalogue key, under together.event, of each event the server names by its code. */
const eventKeys: Readonly<Record<Exclude<GroupEvent, "seeked">, string>> = {
  played: "played",
  paused: "paused",
  changedItem: "changedItem",
  stopped: "stopped",
  leaderPlays: "leaderPlays",
  changedRate: "changedRate",
  waitingFor: "waitingFor",
  everyoneLeft: "everyoneLeft",
  resumedOnReturn: "resumedOnReturn",
};

/**
 * The group's last change as a named event in the viewer's words, "Karl paused at 41:12" (D188,
 * watch-together.md), with the name its member chose; undefined for an event this client does not
 * know.
 */
export function groupEventText(
  t: TFunction,
  group: GroupView,
  roster: readonly RosterMember[],
): string | undefined {
  if (group.event === undefined) {
    return undefined;
  }
  const name =
    roster.find((member) => member.id === group.byMemberId)?.name ?? t("together.someone");
  const at = clockText(group.positionMs, i18n.language);
  if (group.event === "seeked") {
    const from = clockText(group.seekedFromMs, i18n.language);
    return t("together.event.seeked", { name, from, to: at });
  }
  if (group.event === "everyoneLeft" && group.isHeldPlaying) {
    return t("together.event.everyoneLeftWhilePlaying", { at }); // held, to play on (D375)
  }
  return t(`together.event.${eventKeys[group.event]}`, { name, at });
}
