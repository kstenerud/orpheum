// SPDX-License-Identifier: MPL-2.0

/**
 * What needs an admin, by the page it belongs to (D538 §12), as the server says it: what the menu's
 * marks (app-shell.tsx), the status tray's sign and the Needs attention page count from, so they
 * always agree. Each item's words are attention-lines.ts's, loaded with the pages that list them.
 */

import type { components } from "@orpheum/sdk";

import type { Route } from "./router.ts";

/** What needs an admin, by the page it belongs to and is fixed on (D538 §12). */
export type AttentionPages = components["schemas"]["AttentionPagesBody"];
/** One page's items, its problems first. */
export type AttentionPage = components["schemas"]["AttentionPageBody"];
/** One thing that needs an admin. */
export type AttentionItem = components["schemas"]["AttentionItemBody"];
/** A page things can need an admin on, by the attention API's name for it. */
export type AttentionPageKey = keyof AttentionPages;

/** The pages things can need an admin on, in the menu's order (app-shell.tsx's destinations). */
export const attentionPageOrder: readonly AttentionPageKey[] = [
  "titlesToCheck",
  "librarySetup",
  "settings",
  "system",
  "network",
  "update",
];

/**
 * Each page's name in the catalogue, under nav: the menu's own, so a page's section and its mark
 * name it as the menu does.
 */
export const attentionPageName: Readonly<
  Record<AttentionPageKey, "identify" | "libraries" | "settings" | "system" | "network" | "update">
> = {
  titlesToCheck: "identify",
  librarySetup: "libraries",
  settings: "settings",
  system: "system",
  network: "network",
  update: "update",
};

/** Where each page is, which its section's "Go to" leads to. */
export const attentionPageRoute: Readonly<Record<AttentionPageKey, Route>> = {
  titlesToCheck: { screen: "identify" },
  librarySetup: { screen: "libraries" },
  settings: { screen: "settings" },
  system: { screen: "system" },
  network: { screen: "network" },
  update: { screen: "update" },
};

/** A page's items, or none where nothing on it needs the admin, or the server sends no pages. */
export function itemsOf(pages: AttentionPages | undefined, key: AttentionPageKey): AttentionItem[] {
  return pages?.[key]?.items ?? [];
}

/**
 * Whether anything needs the admin on any page: what lights the tray's warning sign, where the
 * server says it by page (D538 §12).
 */
export function isAnythingNeeded(pages: AttentionPages): boolean {
  return attentionPageOrder.some((key) => itemCountOf(pages, key) > 0);
}

/** How many things need the admin on a page; 0 where nothing does. */
export function itemCountOf(pages: AttentionPages | undefined, key: AttentionPageKey): number {
  return pages?.[key]?.itemCount ?? 0;
}
