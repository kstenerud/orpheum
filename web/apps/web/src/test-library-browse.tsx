// SPDX-License-Identifier: MPL-2.0

// The Libraries page mounted for its tests, handed what the server and app.tsx hand it.

import type { JSX } from "react";

import { LibraryBrowse } from "./library-browse.tsx";
import { chipsOf } from "./list-filters.ts";
import { useRoute } from "./services.tsx";
import { BackgroundChangesContext, type Poller } from "./polling.ts";
import { mount, servedBy, type Answer, type Heard, type Mounted } from "./test-dom.tsx";
import { withServices } from "./test-services.tsx";

/** A poller that never asks: a list is read only as the test's steps ask. */
export const neverAsks: Poller = { start: () => () => undefined };

/** A library, as the server sends it. */
export function library(
  id: number,
  name: string,
  kind: "film" | "series" | "other" = "film",
): Record<string, unknown> {
  return {
    id,
    name,
    kind,
    metadataSource: kind === "other" ? "nfo" : "tmdb",
    changeDetection: "watch",
    isWritable: true,
    createdAt: 0,
  };
}

/** One film in a titles page, so .poster-grid draws: an empty page shows browse.empty instead. */
export const titlesPage = (): Answer => [
  200,
  {
    items: [
      {
        id: 1,
        name: "A film",
        kind: "film",
        identificationState: "identified",
        libraryId: 1,
        addedAt: 1,
      },
    ],
    nextCursor: "",
  },
];
export const lettersPage = (): Answer => [200, { letters: [] }];

/** Mounts the Libraries page with its state given by the address (app.tsx's own props). */
export async function mountBrowse(
  state: Partial<{
    library: number | "all" | undefined;
    view: "folders" | undefined;
    folder: string | undefined;
    sort: "name" | "added" | "recorded" | "fileName" | "released" | undefined;
    order: "ascending" | "descending" | undefined;
    query: string;
  }>,
  answers: Record<string, Answer | readonly Answer[]>,
  heard: Heard[] = [],
): Promise<Mounted> {
  // A films' or a series' library asks its genres as its Sort and filter opens (list-chips.tsx).
  const services = servedBy({ "GET /genres": [200, { genres: [] }], ...answers }, heard);
  return await mount(
    <BackgroundChangesContext value={neverAsks}>
      {withServices(
        <LibraryBrowse
          library={state.library}
          view={state.view}
          folder={state.folder}
          sort={state.sort}
          order={state.order}
          query={state.query ?? ""}
        />,
        services,
      )}
    </BackgroundChangesContext>,
  );
}

/** The Libraries page as app.tsx shows it: what it lists read from the address it replaces. */
export function RoutedLibraryBrowse(): JSX.Element {
  const route = useRoute();
  if (route.screen !== "browse") {
    return <p>{route.screen}</p>;
  }
  return (
    <LibraryBrowse
      library={route.library}
      view={route.view}
      folder={route.folder}
      sort={route.sort}
      order={route.order}
      query={route.query ?? ""}
      filters={chipsOf(route)}
    />
  );
}
