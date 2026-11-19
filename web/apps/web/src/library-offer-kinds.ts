// SPDX-License-Identifier: MPL-2.0

/**
 * The offer of a library for each folder of a media folder (D527 §3): which kind a folder's name
 * suggests, and the rows the offer shows. Pure, so the guess is tested without a page.
 */

import { folderInside } from "./folder-picker.tsx";
import type { Route, WelcomeStep } from "./router.ts";

/** The kind of a library an offered folder becomes. */
export type OfferKind = "film" | "series" | "other";

/** The kind a pop-up's chosen value names; "other" for anything else. */
export function kindOfChoice(value: string): OfferKind {
  if (value === "film" || value === "series") {
    return value;
  }
  return "other";
}

/** Folder names, in lower case, that hold films. */
const filmFolders: ReadonlySet<string> = new Set(["movies", "films", "film"]);

/** Folder names, in lower case, that hold series. */
const seriesFolders: ReadonlySet<string> = new Set(["tv", "tv shows", "shows", "series"]);

/** The kind a folder's name suggests, without regard to case; any other name is "other". */
export function guessKind(folderName: string): OfferKind {
  const name = folderName.trim().toLowerCase();
  if (filmFolders.has(name)) {
    return "film";
  }
  return seriesFolders.has(name) ? "series" : "other";
}

/** One folder offered as a library: ticked when its name suggests a kind, the kind changeable. */
export interface OfferRow {
  readonly nodeName: string;
  /** The folder's whole path on the node. */
  readonly path: string;
  /** The folder's name, which the library is named. */
  readonly name: string;
  readonly kind: OfferKind;
  readonly isTicked: boolean;
}

/** The rows the folders inside root offer: only a folder whose name suggests a kind starts ticked. */
export function offerRows(
  nodeName: string,
  root: string,
  folderNames: readonly string[],
): OfferRow[] {
  return folderNames.map((name) => {
    const kind = guessKind(name);
    return { nodeName, path: folderInside(root, name), name, kind, isTicked: kind !== "other" };
  });
}

/**
 * Whether the folders hold two kinds, films and series: one library of the whole folder takes one
 * kind, so it would hold one of them wrongly.
 */
export function holdsTwoKinds(rows: readonly Pick<OfferRow, "kind">[]): boolean {
  return rows.some((row) => row.kind === "film") && rows.some((row) => row.kind === "series");
}

/** The first-time page after step (undefined is the libraries page); home after the last. */
export function stepAfter(step: WelcomeStep | undefined): Route {
  return step === undefined ? { screen: "welcome", step: "discovery" } : { screen: "home" };
}
