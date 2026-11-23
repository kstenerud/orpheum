// SPDX-License-Identifier: MPL-2.0

/**
 * Each thing that needs an admin in words, with the exact place it is fixed at (D538 §12): the
 * Needs attention page's lines and each page's Needs you say it so.
 */

import type { TFunction } from "i18next";

import type { AttentionItem } from "./attention-pages.ts";
import { dateTimeText } from "./date-time-text.ts";
import type { Route } from "./router.ts";
import { settingLabel } from "./server-settings.tsx";

/** One item's line: its words, and the exact place it is fixed at. */
export interface AttentionLine {
  readonly text: string;
  readonly to: Route;
}

/** The values an item's words name, each from the item; what it does not give, empty. */
type Values = Readonly<Record<string, string | number>>;

/** Where a kind's items are fixed. */
type Place = "library" | "identify" | "system" | "jobs" | "settings" | "update" | "network";

/** How a kind's line is made: where it leads, whether its words count, and any values of its own. */
interface KindLine {
  readonly place: Place;
  /** Whether its words say the item's count, with the catalogue's own words for one. */
  readonly isCounted?: true;
  /** Values its words name that the item's own fields do not say as they are. */
  readonly values?: (t: TFunction, item: AttentionItem) => Values;
}

/** Every kind the server sends (D538 §12), by the kind: the type checks that none is missing. */
const kindLines: Readonly<Record<AttentionItem["kind"], KindLine>> = {
  folderOffline: { place: "library" },
  diskOffline: { place: "library" },
  folderWaiting: { place: "library", isCounted: true },
  unread: { place: "library", isCounted: true },
  rescanGaveUp: { place: "library" },
  needsMatch: { place: "identify", isCounted: true },
  seriesReadWrongly: { place: "identify", isCounted: true },
  filmsWithoutSound: { place: "identify", isCounted: true },
  finding: {
    place: "system",
    values: (_t, item) => ({
      check: item.finding?.check ?? "",
      detail: item.finding?.detail ?? "",
    }),
  },
  workFailing: {
    place: "jobs",
    isCounted: true,
    values: (_t, item) => ({ hours: item.windowHours ?? 0 }),
  },
  destinationDown: {
    place: "system",
    values: (_t, item) => ({
      name: item.destination?.name ?? "",
      reason: item.destination?.reason ?? "",
    }),
  },
  certificateMissing: { place: "system" },
  unknownFailures: { place: "system", isCounted: true },
  machineLocked: { place: "system" },
  machineOutdated: { place: "system" },
  machineSilent: { place: "system" },
  keyRefused: { place: "settings" },
  keyMissing: { place: "settings" },
  secretNeedsReentry: {
    place: "settings",
    values: (t, item) => ({ name: settingLabel(t, "secretLabels", item.name ?? "") }),
  },
  updateAvailable: { place: "update" },
  securityFix: { place: "update" },
  updateCheckFailed: { place: "update" },
  composePinned: { place: "update" },
  composeNewer: { place: "update" },
  stoodAside: { place: "network" },
  copyRefused: { place: "network" },
  addressesExpired: { place: "network" },
  removed: { place: "network" },
  deadEnds: { place: "network", isCounted: true },
  protocolEnding: { place: "network" },
};

/**
 * The exact place an item is fixed at: a library's own page for a library's; Keys and passwords for
 * a key's, at the key where the item names it (D538 §11).
 */
function placeOf(place: Place, item: AttentionItem): Route {
  if (place === "settings") {
    return item.kind === "secretNeedsReentry" && item.name !== undefined
      ? { screen: "settings", page: "keys", setting: item.name }
      : { screen: "settings", page: "keys" };
  }
  if (place !== "library") {
    return { screen: place };
  }
  return item.libraryId === undefined
    ? { screen: "libraries" }
    : { screen: "library", libraryId: item.libraryId };
}

/** What an item's words can name, from its own fields. */
function valuesOf(item: AttentionItem, language: string): Values {
  return {
    libraryName: item.libraryName ?? "",
    rootPath: item.rootPath ?? "",
    nodeName: item.nodeName ?? "",
    mountPath: item.mountPath ?? "",
    name: item.name ?? "",
    version: item.version ?? "",
    detail: item.detail ?? "",
    until: item.until ?? "",
    since: item.since === undefined ? "" : dateTimeText(item.since, language),
  };
}

/** Whether the server's kind is one this app knows. */
function isKnown(kind: string): kind is AttentionItem["kind"] {
  return Object.hasOwn(kindLines, kind);
}

/**
 * An item's line, its words and where it is fixed; undefined for a kind this app does not know,
 * which a later server may send, and which is skipped (D432 §2).
 */
export function attentionLine(
  t: TFunction,
  item: AttentionItem,
  language: string,
): AttentionLine | undefined {
  if (!isKnown(item.kind)) {
    return undefined;
  }
  const kind = kindLines[item.kind];
  const values = { ...valuesOf(item, language), ...kind.values?.(t, item) };
  const key = `attention.${item.kind}`;
  const count = item.count ?? 0;
  if (kind.isCounted !== true) {
    return { text: t(key, values), to: placeOf(kind.place, item) };
  }
  const text = count === 1 ? t(`${key}One`, values) : t(key, { ...values, count });
  return { text, to: placeOf(kind.place, item) };
}
