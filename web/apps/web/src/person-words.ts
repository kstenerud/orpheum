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
