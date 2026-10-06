// SPDX-License-Identifier: MPL-2.0

import "./i18n.ts";

import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";

import { ApiProblem } from "./problem.ts";
import { BrowseView, type Entry } from "./browse.tsx";
import { itemRoute } from "./router.ts";
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
    ).toContain("The server is starting up, or updating. It will answer in a moment.");
    expect(itemRoute("series", 4)).toEqual({ screen: "series-page", collectionId: 4 });
  });

  it("says a server down behind its proxy is not answering, and that the list asks again by itself", () => {
    const shown = renderBrowseView({
      state: "failed",
      cause: new ApiProblem({ code: "not_answering", message: "502" }),
      willAskAgain: true,
    });
    expect(shown).toContain("The server is not answering: it may be restarting");
    expect(shown).toContain("This page tries again by itself.");
    expect(shown).not.toContain("Something went wrong");
  });
});

describe("a title that cannot be played now (D241 §3)", () => {
  it("stays in the list, marked", () => {
    const markup = renderBrowseView({
      state: "loaded",
      value: {
        nextCursor: "",
        items: [
          {
            id: 10,
            name: "Metropolis",
            identificationState: "identified",
            libraryId: 1,
            addedAt: 1,
            unavailable: { reason: "disk_missing", nodeName: "devnas", path: "/media/usb" },
          },
        ],
      },
    });
    expect(markup).toContain("Metropolis");
    expect(markup).toContain("Cannot be played now");
    expect(markup).toContain("the disk at /media/usb on devnas is missing");
    expect(markup).toContain("poster-grid__card--unavailable");
  });
});
