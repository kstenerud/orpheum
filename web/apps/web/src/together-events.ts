// SPDX-License-Identifier: MPL-2.0

import type { GroupEvent, GroupView, RosterMember } from "@orpheum/sdk";
import type { TFunction } from "i18next";

import { i18n } from "./i18n.ts";

/** A place in a film as a person reads it: 41:12, or 1:02:03 past the first hour. */
export function filmPositionText(positionMs: number, locale: string): string {
  const totalSeconds = Math.max(0, Math.floor(positionMs / 1000));
  const twoDigits = new Intl.NumberFormat(locale, { minimumIntegerDigits: 2, useGrouping: false });
  const plain = new Intl.NumberFormat(locale, { useGrouping: false });
  const hours = Math.floor(totalSeconds / 3600);
  const minutes = Math.floor((totalSeconds % 3600) / 60);
  const seconds = totalSeconds % 60;
  return hours === 0
    ? `${plain.format(minutes)}:${twoDigits.format(seconds)}`
    : `${plain.format(hours)}:${twoDigits.format(minutes)}:${twoDigits.format(seconds)}`;
}

/** The catalogue key, under together.event, of each event the server names by its code. */
const eventKeys: Readonly<Record<Exclude<GroupEvent, "seeked">, string>> = {
  played: "played",
  paused: "paused",
  changedItem: "changedItem",
  stopped: "stopped",
  buffering: "buffering",
  left: "dropped",
  everyoneReady: "everyoneReady",
  changedRate: "changedRate",
  continuedWithout: "continuedWithout",
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
  const at = filmPositionText(group.positionMs, i18n.language);
  if (group.event === "seeked") {
    const from = filmPositionText(group.seekedFromMs, i18n.language);
    return t("together.event.seeked", { name, from, to: at });
  }
  return t(`together.event.${eventKeys[group.event]}`, { name, at });
}
