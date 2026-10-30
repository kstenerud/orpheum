// SPDX-License-Identifier: MPL-2.0

/**
 * The libraries a person reaches, as the banner's Libraries menu and a library's page read them
 * (D349 §9.1): one way to read them for both, and their order, the person's locale's alphabetical.
 */

import type { components } from "@orpheum/sdk";

import { useBackgroundChangesPoller } from "./polling.ts";
import { dataOrThrow } from "./problem.ts";
import { useServices } from "./services.tsx";
import { useResource, type Resource } from "./use-resource.ts";

/** A library, as the server sends it. */
export type LibraryBody = components["schemas"]["LibraryBody"];

/** The libraries the person reaches, in their locale's alphabetical order (D349 §9.1). */
export function sortedLibraries(loaded: readonly LibraryBody[]): LibraryBody[] {
  return [...loaded].sort((first, second) => first.name.localeCompare(second.name));
}

/** The libraries the person reaches, as the server lists them, read again as the server changes. */
export function useLibrariesReached(): Resource<readonly LibraryBody[]> {
  const { api } = useServices();
  const [libraries] = useResource(
    "libraries-reached",
    async () => dataOrThrow(await api.GET("/libraries")).libraries ?? [],
    useBackgroundChangesPoller(),
  );
  return libraries;
}
