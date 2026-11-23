// SPDX-License-Identifier: MPL-2.0

import "./people-words.ts";

import type { TFunction } from "i18next";

import type { FormWords } from "./problem.ts";

/** How a refusal of a person's name or role is told, on the Users page and a person's own. */
export function personWords(t: TFunction): FormWords {
  return {
    fields: {
      "body.name": t("people.name"),
      "body.role": t("people.role"),
      "body.adminNote": t("people.adminNote"),
      "body.password": t("people.password"),
    },
    overrides: { conflict: t("people.nameTaken") },
    codeFields: { conflict: ["body.name"] },
  };
}

/** Whether a person has set their password yet, in words. */
export function registrationText(t: TFunction, account: { readonly hasPassword: boolean }): string {
  return t(account.hasPassword ? "people.registered" : "people.notRegistered");
}

/** The units a time since is said in, each with how many seconds make one, largest first. */
const sinceUnits: readonly (readonly [Intl.RelativeTimeFormatUnit, number])[] = [
  ["year", 365 * 24 * 3600],
  ["month", 30 * 24 * 3600],
  ["week", 7 * 24 * 3600],
  ["day", 24 * 3600],
  ["hour", 3600],
  ["minute", 60],
];

/** How long ago a moment was, in the locale's own words ("3 hours ago", "yesterday"). */
export function sinceText(epochMs: number, nowMs: number, locale: string): string {
  const seconds = Math.max(0, (nowMs - epochMs) / 1000);
  const [unit, size] = sinceUnits.find(([, each]) => seconds >= each) ?? ["minute", 60];
  return new Intl.RelativeTimeFormat(locale, { numeric: "auto" }).format(
    -Math.floor(seconds / size),
    unit,
  );
}

/**
 * A person's status on their row on Users (pages-spec-2): "Has not used their link yet" before
 * they have a password, "Watched 3 hours ago" once they have watched, nothing for one who never has.
 */
export function lastWatchedText(
  t: TFunction,
  account: { readonly hasPassword: boolean; readonly lastWatchedAt?: number | undefined },
  now: { readonly nowMs: number; readonly locale: string },
): string {
  if (!account.hasPassword) {
    return t("people.notRegistered");
  }
  return account.lastWatchedAt === undefined
    ? ""
    : t("people.watchedSince", { when: sinceText(account.lastWatchedAt, now.nowMs, now.locale) });
}
