// SPDX-License-Identifier: MPL-2.0

/**
 * The settings pages' words: each section's name and each server setting's and each key's or
 * password's label, description and details (locales/en-setting-labels.ts, locales/en-settings.ts),
 * added under `settings.` as the first of their modules loads, never with the first page (i18n.ts's
 * addWords; the owner, 2026-11-13: reverting either file in the shared catalogue re-ran nearly every
 * web test). Every module that asks for a `settings.sections.`, `settings.labels.`,
 * `settings.descriptions.`, `settings.details.`, `settings.secretLabels.` or
 * `settings.secretDescriptions.` word imports this.
 */

import { addWords } from "./i18n.ts";
import { enSettingLabels } from "./locales/en-setting-labels.ts";
import { enSettingsCatalogue } from "./locales/en-settings.ts";

/** The words this adds, as the catalogue holds them. */
export const settingsWords = { settings: { ...enSettingLabels, ...enSettingsCatalogue } };

addWords(settingsWords);
