// SPDX-License-Identifier: MPL-2.0

import type { GroupView, RosterMember } from "@orpheum/sdk";
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

/** The server's name for a seek's event, with where it jumped from (internal/core/together). */
const seekedFrom = /^seeked from (\d+)$/u;

/**
 * The catalogue key of each event the server names, under together.event (the server's words are
 * its names for them, not a viewer's: internal/core/together/hub.go).
 */
const eventKeys: Readonly<Record<string, string>> = {
  played: "played",
  paused: "paused",
  "changed what plays": "changedItem",
  stopped: "stopped",
  buffering: "buffering",
  left: "dropped",
  "everyone is ready": "everyoneReady",
  "changed the rate": "changedRate",
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
  const seek = seekedFrom.exec(group.event);
  if (seek !== null) {
    const from = filmPositionText(Number(seek[1]), i18n.language);
    return t("together.event.seeked", { name, from, to: at });
  }
  const key = eventKeys[group.event];
  return key === undefined ? undefined : t(`together.event.${key}`, { name, at });
}
