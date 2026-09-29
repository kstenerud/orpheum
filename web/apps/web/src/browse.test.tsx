// SPDX-License-Identifier: MPL-2.0

import "./i18n.ts";

import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";

import { ApiProblem } from "./problem.ts";
import { BrowseView, entryRoute, type Entry } from "./browse.tsx";
import type { Page } from "./use-pages.ts";
import type { Resource } from "./use-resource.ts";
import { withServices } from "./test-services.tsx";

const ignore = (): void => undefined;

function renderBrowseView(loadedEntries: Resource<Page<Entry>>): string {
  return renderToStaticMarkup(
    withServices(
      <BrowseView
        kind="movie"
        loadedEntries={loadedEntries}
        isLoadingMore={false}
        onLoadMore={ignore}
      />,
    ),
  );
}

describe("browse", () => {
  it("shows each entry's poster, name and year, leading to its page", () => {
    const markup = renderBrowseView({
      state: "loaded",
      value: {
        nextCursor: "cursor",
        items: [
          {
            id: 10,
            name: "Metropolis",
            year: 1995,
            identificationState: "identified",
            libraryId: 1,
            addedAt: 1,
            posterSha256: "ab12",
          },
          {
            id: 11,
            name: "Untitled",
            identificationState: "unidentified",
            libraryId: 1,
            addedAt: 2,
          },
        ],
      },
    });
    expect(markup).toContain('href="/titles/10"');
    expect(markup).toContain('src="/api/v1/bytes/art/ab12/portrait-m.webp"');
    expect(markup).toContain(
      '<span class="poster-grid__name">Metropolis</span><span class="poster-grid__year">1995</span>',
    );
    expect(markup).toContain("No poster");
    expect(markup).toContain("Show more");
  });

  it("says what an empty list means, and a failure's cause", () => {
    const empty = renderBrowseView({ state: "loaded", value: { nextCursor: "", items: [] } });
    expect(empty).toContain("Once a library is scanned");
    expect(empty).not.toContain("Show more");
    expect(
      renderBrowseView({
        state: "failed",
        cause: new ApiProblem({ code: "migrating", message: "the server is migrating" }),
      }),
    ).toContain("The server is updating. Try again in a minute.");
    expect(entryRoute("series", 4)).toEqual({ screen: "series-page", id: 4 });
  });
});
