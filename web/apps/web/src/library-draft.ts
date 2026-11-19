// SPDX-License-Identifier: MPL-2.0

import "./libraries-words.ts";

import type { components } from "@orpheum/sdk";
import type { TFunction } from "i18next";

import type { FormWords } from "./problem.ts";

/** A new library, as the server takes it. */
export type NewLibrary = components["schemas"]["CreateLibraryInputBody"];

/** The form's starting answers: read-only, told when to scan, sources as a person would expect. */
export function blankLibrary(): NewLibrary {
  return {
    name: "",
    kind: "film",
    metadataSource: "nfoThenTmdb",
    changeDetection: "manual",
    isWritable: false,
    // No machine yet: onAConnectedNode puts the first connected one here once they are known.
    nodeName: "",
    rootPaths: [],
  };
}

/**
 * The library the server is asked to create: the draft, but an other library is not asked where its
 * information comes from, as it takes NFO files only and is never looked up online (D349).
 */
export function libraryToCreate(draft: NewLibrary): NewLibrary {
  if (draft.kind !== "other") {
    return draft;
  }
  const rest: NewLibrary = { ...draft };
  delete rest.metadataSource;
  return rest;
}

/** How a refused library is told: by the fields the server names. */
export function libraryWords(t: TFunction): FormWords {
  return {
    fields: {
      "body.name": t("libraries.name"),
      "body.nodeName": t("libraries.node"),
      "body.rootPaths": t("libraries.roots"),
      "body.checkEveryHours": t("libraries.checkEvery"),
    },
  };
}
