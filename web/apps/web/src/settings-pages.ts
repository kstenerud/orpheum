// SPDX-License-Identifier: MPL-2.0

/**
 * Settings' pages (D538 §11, pages-spec-3): the index at /settings, a page to each section at
 * /settings/<section>, and the keys and passwords at /settings/keys. Shell-safe: no words, no parts,
 * so the router reads it.
 */

import type { components } from "@orpheum/sdk";

/** The part of Settings a setting is listed in. */
export type SettingsSection = components["schemas"]["SettingBody"]["section"];

/** A page of Settings' own beneath its index: a section's, or the keys and passwords'. */
export type SettingsPage = SettingsSection | "keys";

/** The sections in the server's order (settings.Sections), which a test holds to the API document's. */
export const sectionOrder: readonly SettingsSection[] = [
  "general",
  "accounts",
  "libraries",
  "metadata",
  "playback",
  "slowLinks",
  "upload",
  "together",
  "nodes",
  "jobs",
  "remoteAccess",
  "backups",
  "diagnostics",
];

/** Settings' pages beneath its index, in the index's order: the keys and passwords first. */
export const settingsPages: readonly SettingsPage[] = ["keys", ...sectionOrder];

/** A page's part of its address: its name, its words joined by "-" (slowLinks, slow-links). */
export function settingsPagePath(page: SettingsPage): string {
  return page.replaceAll(/[A-Z]/gu, (letter) => `-${letter.toLowerCase()}`);
}
